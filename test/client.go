package test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// Client is a high-level, in-memory HTTP test client for rex routers
// (or any http.Handler). It removes httptest boilerplate from handler
// tests: requests are built fluently, dispatched without a real server,
// and responses carry self-failing assertions when constructed with a
// *testing.T.
//
// Basic usage:
//
//	c := test.NewClient(t, router)
//	resp := c.Get("/users/42").Send()
//	resp.AssertStatus(200)
//
//	var user User
//	require.NoError(t, resp.JSON(&user))
//
// Posting JSON or forms:
//
//	c.PostJSON("/users", User{Name: "ada"}).AssertStatus(201)
//	c.Post("/users").Form(url.Values{"name": {"ada"}}).Send()
//
// The zero value is not usable; construct with NewClient.
type Client struct {
	t   testing.TB
	app http.Handler

	// headers applied to every outgoing request.
	headers http.Header
}

// NewClient creates a Client that dispatches requests to app in memory.
// If t is non-nil, request-build errors and failed assertions are reported
// through it automatically. A *rex.Router is an http.Handler and can be
// passed directly.
func NewClient(t testing.TB, app http.Handler) *Client {
	return &Client{
		t:       t,
		app:     app,
		headers: make(http.Header),
	}
}

// Header sets a default header applied to every request made by this client.
func (c *Client) Header(key, value string) *Client {
	c.headers.Set(key, value)
	return c
}

// Get starts a GET request against path.
func (c *Client) Get(path string) *Request { return c.newRequest(http.MethodGet, path) }

// Post starts a POST request against path.
func (c *Client) Post(path string) *Request { return c.newRequest(http.MethodPost, path) }

// Put starts a PUT request against path.
func (c *Client) Put(path string) *Request { return c.newRequest(http.MethodPut, path) }

// Patch starts a PATCH request against path.
func (c *Client) Patch(path string) *Request { return c.newRequest(http.MethodPatch, path) }

// Delete starts a DELETE request against path.
func (c *Client) Delete(path string) *Request { return c.newRequest(http.MethodDelete, path) }

// Head starts a HEAD request against path.
func (c *Client) Head(path string) *Request { return c.newRequest(http.MethodHead, path) }

// Options starts an OPTIONS request against path.
func (c *Client) Options(path string) *Request { return c.newRequest(http.MethodOptions, path) }

// PostJSON sends a POST with v marshaled as the JSON body in one call.
func (c *Client) PostJSON(path string, v any) *Response {
	return c.Post(path).JSON(v).Send()
}

// PutJSON sends a PUT with v marshaled as the JSON body in one call.
func (c *Client) PutJSON(path string, v any) *Response {
	return c.Put(path).JSON(v).Send()
}

// newRequest builds a Request. It never returns nil so callers can keep
// chaining after errors; errors surface at Send time.
func (c *Client) newRequest(method, target string) *Request {
	req := httptest.NewRequest(method, target, nil)
	for key, values := range c.headers {
		for _, v := range values {
			req.Header.Add(key, v)
		}
	}
	return &Request{t: c.t, app: c.app, req: req}
}

// Request is a chainable builder for an outgoing test request.
type Request struct {
	t   testing.TB
	app http.Handler
	req *http.Request
	err error

	bodyBuffer      *bytes.Buffer
	multipartWriter *multipart.Writer
}

// fail records an error (reported at Send, or immediately via t).
func (r *Request) fail(err error) *Request {
	if r.err == nil {
		r.err = err
	}
	return r
}

// Header sets a request header, overriding any client default.
func (r *Request) Header(key, value string) *Request {
	r.req.Header.Set(key, value)
	return r
}

// Cookie adds a cookie to the request.
func (r *Request) Cookie(cookie *http.Cookie) *Request {
	if cookie == nil {
		return r.fail(errors.New("cookie must not be nil"))
	}
	r.req.AddCookie(cookie)
	return r
}

// Query adds a query parameter, preserving existing parameters.
func (r *Request) Query(key, value string) *Request {
	q := r.req.URL.Query()
	q.Add(key, value)
	r.req.URL.RawQuery = q.Encode()
	return r
}

// PathValue sets a path parameter on the request for handlers that read
// c.Param directly (e.g. when testing handlers without route registration).
func (r *Request) PathValue(key, value string) *Request {
	r.req.SetPathValue(key, value)
	return r
}

// JSON marshals v as the request body with an application/json Content-Type.
func (r *Request) JSON(v any) *Request {
	data, err := json.Marshal(v)
	if err != nil {
		return r.fail(err)
	}
	r.req.Header.Set("Content-Type", "application/json")
	r.setBody(data)
	return r
}

// Form sends vals as an application/x-www-form-urlencoded body.
func (r *Request) Form(vals url.Values) *Request {
	r.req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.setBody([]byte(vals.Encode()))
	return r
}

// File adds an in-memory file upload; Send builds a multipart/form-data body
// containing all files and form fields added this way.
func (r *Request) File(field, filename string, content []byte) *Request {
	if r.multipartWriter == nil {
		body := &bytes.Buffer{}
		r.bodyBuffer = body
		r.multipartWriter = multipart.NewWriter(body)
	}
	fw, err := r.multipartWriter.CreateFormFile(field, filename)
	if err != nil {
		return r.fail(err)
	}
	if _, err := fw.Write(content); err != nil {
		return r.fail(err)
	}
	return r
}

// FormField adds a regular field to a multipart body started by File.
func (r *Request) FormField(field, value string) *Request {
	if r.multipartWriter == nil {
		body := &bytes.Buffer{}
		r.bodyBuffer = body
		r.multipartWriter = multipart.NewWriter(body)
	}
	if err := r.multipartWriter.WriteField(field, value); err != nil {
		return r.fail(err)
	}
	return r
}

// Raw sets the request body and content type verbatim.
func (r *Request) Raw(contentType string, body []byte) *Request {
	r.req.Header.Set("Content-Type", contentType)
	r.setBody(body)
	return r
}

// setBody replaces the request body while preserving method semantics.
func (r *Request) setBody(data []byte) {
	r.bodyBuffer = bytes.NewBuffer(data)
}

// finalize prepares the request body just before dispatch.
func (r *Request) finalize() error {
	if r.err != nil {
		return r.err
	}

	switch {
	case r.multipartWriter != nil:
		if err := r.multipartWriter.Close(); err != nil {
			return err
		}
		r.req.Body = io.NopCloser(r.bodyBuffer)
		r.req.Header.Set("Content-Type", r.multipartWriter.FormDataContentType())
		r.req.ContentLength = int64(r.bodyBuffer.Len())
	case r.bodyBuffer != nil:
		r.req.Body = io.NopCloser(r.bodyBuffer)
		r.req.ContentLength = int64(r.bodyBuffer.Len())
	}
	return nil
}

// Send dispatches the request to the handler and returns the response.
// Build errors (e.g. JSON marshal failures) are reported here: through t
// when the client was created with one, otherwise returned on the Response.
func (r *Request) Send() *Response {
	if err := r.finalize(); err != nil {
		if r.t != nil {
			r.t.Helper()
			r.t.Errorf("test client: building %s %s: %v", r.req.Method, r.req.URL.Path, err)
		}
		return &Response{t: r.t, recorder: httptest.NewRecorder(), err: err}
	}

	recorder := httptest.NewRecorder()
	r.app.ServeHTTP(recorder, r.req)
	return &Response{t: r.t, recorder: recorder}
}

// Response wraps a recorded HTTP response with convenience accessors and
// self-failing assertions. Assertion methods report failures through the
// client's testing.TB and return the response for chaining.
type Response struct {
	t        testing.TB
	recorder *httptest.ResponseRecorder
	err      error
}

// Err returns any error recorded while building or dispatching the request.
func (resp *Response) Err() error { return resp.err }

// Code returns the response status code.
func (resp *Response) Code() int { return resp.recorder.Code }

// Body returns the raw response body bytes.
func (resp *Response) Body() []byte { return resp.recorder.Body.Bytes() }

// BodyString returns the response body as a string.
func (resp *Response) BodyString() string { return resp.recorder.Body.String() }

// Header returns the first value of a response header.
func (resp *Response) Header(key string) string {
	return resp.recorder.Header().Get(key)
}

// Cookies returns the cookies set by the handler via Set-Cookie headers.
func (resp *Response) Cookies() []*http.Cookie {
	return resp.recorder.Result().Cookies()
}

// Cookie returns the named cookie set by the handler, or nil.
func (resp *Response) Cookie(name string) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// JSON decodes the response body into v.
func (resp *Response) JSON(v any) error {
	if resp.err != nil {
		return resp.err
	}
	return json.Unmarshal(resp.Body(), v)
}

// ContentType returns the response Content-Type header.
func (resp *Response) ContentType() string {
	return resp.Header("Content-Type")
}

func (resp *Response) fail(format string, args ...any) *Response {
	if resp.t != nil {
		resp.t.Helper()
		resp.t.Errorf("test client: "+format, args...)
	}
	return resp
}

// AssertStatus fails the test unless the status code matches, and returns
// the response for further chaining.
func (resp *Response) AssertStatus(code int) *Response {
	if resp.recorder.Code != code {
		return resp.fail("expected status %d, got %d (body: %q)",
			code, resp.recorder.Code, truncateBody(resp.BodyString()))
	}
	return resp
}

// AssertHeader fails the test unless the header contains the expected value.
func (resp *Response) AssertHeader(key, value string) *Response {
	if got := resp.Header(key); got != value {
		return resp.fail("expected header %s=%q, got %q", key, value, got)
	}
	return resp
}

// AssertBodyContains fails the test unless the body contains substr.
func (resp *Response) AssertBodyContains(substr string) *Response {
	if !bytes.Contains(resp.Body(), []byte(substr)) {
		return resp.fail("expected body to contain %q, got %q", substr, truncateBody(resp.BodyString()))
	}
	return resp
}

// AssertJSON decodes the body into v and fails the test on decode errors,
// returning the response for chaining.
func (resp *Response) AssertJSON(v any) *Response {
	if err := resp.JSON(v); err != nil {
		return resp.fail("failed to decode JSON response: %v", err)
	}
	return resp
}

func truncateBody(s string) string {
	const max = 512
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
