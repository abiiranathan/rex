package rex

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/pkg/errors"
)

// Context represents the context of the current HTTP request.
//
// *Context is pooled and reset after the handler returns (see Router.PutContext).
// It is safe to pass as a context.Context to synchronous calls that complete
// within the handler (database queries, outbound HTTP requests you wait on,
// validator functions, etc.).
//
// Do NOT retain *Context or pass it into a goroutine, channel, or callback that
// may run after the handler returns — the underlying locals map and parent
// context get reset and reused for a different request, which is a data race.
// If you need to do async work, extract what you need first (e.g. copy values
// out of Locals(), or use context.WithoutCancel(c) / c.Request.Context() for
// cancellation semantics only) before spawning the goroutine.
type Context struct {
	Request        *http.Request       // Original Request object
	Response       http.ResponseWriter // Wrapped Writer
	rw             ResponseWriter
	ctx            context.Context // Parent Context
	router         *Router         // Instance of the Router.
	locals         map[string]any  // Overflow or materialized locals map
	query          url.Values      // Lazily parsed query parameters (cached)
	redirectOpts   RedirectOptions
	currentRoute   *route        // The current route.
	latency        time.Duration // Request latency tracked by router
	err            error         // Tracks any error encountered in middleware
	hasRedirect    bool
	contentTypeSet bool // Tracks if Content-Type header has been set
	recycled       bool // set true by reset(); guards against use-after-recycle
}

// NewContext creates a new Context instance for the given request and response.
// This is primarily useful for testing but can also be used when manually
// creating contexts outside of the normal routing flow.
// The writer is wrapped with rex's tracking ResponseWriter so StatusCode()
// and SetSkipBody() behave consistently with routed requests.
func NewContext(w http.ResponseWriter, r *http.Request, router *Router) *Context {
	c := &Context{
		Request: r,
		ctx:     r.Context(),
		router:  router,
		locals:  make(map[string]any, 16),
	}
	c.rw = ResponseWriter{writer: w, status: http.StatusOK}
	c.Response = &c.rw
	return c
}

// Deadline implements context.Context.
func (c *Context) Deadline() (deadline time.Time, ok bool) {
	if c.ctx == nil {
		return time.Time{}, false
	}
	return c.ctx.Deadline()
}

// Done implements context.Context.
func (c *Context) Done() <-chan struct{} {
	if c.ctx == nil {
		return nil
	}
	return c.ctx.Done()
}

// Err implements context.Context.
func (c *Context) Err() error {
	if c.ctx == nil {
		return nil
	}
	return c.ctx.Err()
}

// Value implements context.Context.
func (c *Context) Value(key any) any {
	if c.recycled {
		panic("rex: Context used after being returned to the pool — do not retain *Context past handler return")
	}

	if key, ok := key.(string); ok {
		if v, exists := c.Get(key); exists {
			return v
		}
	}
	if c.ctx == nil {
		return nil
	}
	return c.ctx.Value(key)
}

// SetHeader sets a header in the response
func (c *Context) SetHeader(key, value string) {
	c.Response.Header().Set(key, value)
}

// DelHeader deletes a header in the response
func (c *Context) DelHeader(key string) {
	c.Response.Header().Del(key)
}

// GetHeader returns the value of the request header
func (c *Context) GetHeader(key string) string {
	return c.Request.Header.Get(key)
}

// Status sets the status code of the response and returns the context
// allowing for chaining.
func (c *Context) Status(status int) *Context {
	if setter, ok := c.Response.(interface{ SetStatus(int) }); ok {
		setter.SetStatus(status)
		return c
	}

	c.Response.WriteHeader(status)
	return c
}

// JSON sends a JSON response.
// The payload is buffered so Content-Length can be set (avoiding chunked
// transfer encoding); the buffer is pooled to keep steady-state allocation low.
func (c *Context) JSON(data any) error {
	c.SetContentType(ContentTypeJSON)

	buf := respBufPool.Get().(*bytes.Buffer)
	buf.Reset()

	if err := json.NewEncoder(buf).Encode(data); err != nil {
		putRespBuf(buf)
		return err
	}

	c.Response.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	_, err := c.Response.Write(buf.Bytes())
	putRespBuf(buf)
	return err
}

// XML sends an XML response.
func (c *Context) XML(data any) error {
	c.SetContentType(ContentTypeXML)

	buf := respBufPool.Get().(*bytes.Buffer)
	buf.Reset()

	enc := xml.NewEncoder(buf)
	if err := enc.Encode(data); err != nil {
		putRespBuf(buf)
		return err
	}
	if err := enc.Flush(); err != nil {
		putRespBuf(buf)
		return err
	}

	c.Response.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	_, err := c.Response.Write(buf.Bytes())
	putRespBuf(buf)
	return err
}

// String sends a string response
func (c *Context) String(text string) error {
	c.SetContentType(ContentTypeText)
	_, err := io.WriteString(c.Response, text)
	return err
}

// ContentType returns the request content type without parameters such as charset or multipart boundaries.
func (c *Context) ContentType() string {
	contentType := c.Request.Header.Get("Content-Type")
	if before, _, ok := strings.Cut(contentType, ";"); ok {
		return before
	}
	return contentType
}

// AcceptHeader returns the first media type from the Accept header.
func (c *Context) AcceptHeader() string {
	accept := c.Request.Header.Get("Accept")
	// accept header may contain multiple values and encoding types
	if before, _, ok := strings.Cut(accept, ","); ok {
		return before
	}
	return accept
}

// HTML sends an HTML response.
func (c *Context) HTML(html string) error {
	c.SetContentType(ContentTypeHTML)
	_, err := c.Response.Write([]byte(html))
	return err
}

// respBufPool recycles response serialization buffers.
var respBufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

func putRespBuf(buf *bytes.Buffer) {
	const maxRetainedSize = 64 << 10 // don't retain huge payloads
	if buf.Cap() > maxRetainedSize {
		return
	}
	buf.Reset()
	respBufPool.Put(buf)
}

// WriteHeader writes the response status code.
func (c *Context) WriteHeader(status int) {
	c.Response.WriteHeader(status)
}

// Write sends a raw response
func (c *Context) Write(data []byte) (int, error) {
	return c.Response.Write(data)
}

// Send sends a raw response and returns an error.
// This conveniently returns only the error from the response writer.
func (c *Context) Send(data []byte) error {
	_, err := c.Response.Write(data)
	return err
}

// Error sends an error response as plain text.
// You can optionally pass a content type.
// Status code is expected to be between 400 and 599.
func (c *Context) Error(err error, status int, contentType ...string) error {
	if status < 400 || status > 599 {
		return fmt.Errorf("invalid status code %d", status)
	}

	if len(contentType) > 0 && contentType[0] != "" {
		c.Response.Header().Set("Content-Type", contentType[0])
	} else {
		c.Response.Header().Set("Content-Type", "text/plain")
	}

	c.Status(status)
	_, writeErr := c.Response.Write([]byte(err.Error()))
	return writeErr
}

// Param gets a path parameter value by name from the request.
// If the parameter is not found, it checks the redirect options.
func (c *Context) Param(name string) string {
	p := c.Request.PathValue(name)
	if p == "" {
		// check redirect params
		opts, ok := c.redirectOptions()
		if ok {
			p = opts.Params[name]
		}
	}
	return p
}

// parseIntWithDefault parses s as a signed integer of the given bit size,
// returning defaults[0] (or 0) when s is empty or unparsable.
func parseIntWithDefault[T int | int64](s string, bitSize int, defaults ...T) T {
	if s == "" && len(defaults) > 0 {
		return defaults[0]
	}

	n, err := strconv.ParseInt(s, 10, bitSize)
	if err != nil {
		if len(defaults) > 0 {
			return defaults[0]
		}
		return 0
	}
	return T(n)
}

// parseUintWithDefault parses s as an unsigned integer, returning defaults[0]
// (or 0) when s is empty, negative, or unparsable.
func parseUintWithDefault(s string, defaults ...uint) uint {
	if s == "" && len(defaults) > 0 {
		return defaults[0]
	}

	n, err := strconv.ParseUint(s, 10, strconv.IntSize)
	if err != nil {
		if len(defaults) > 0 {
			return defaults[0]
		}
		return 0
	}
	return uint(n)
}

// ParamInt returns the value of the parameter as an integer.
// If the parameter is not found, it checks the redirect options.
func (c *Context) ParamInt(key string, defaults ...int) int {
	return parseIntWithDefault(c.Param(key), strconv.IntSize, defaults...)
}

// ParamUint returns the value of the parameter as an unsigned integer.
// Negative values or values exceeding the platform uint size return the default.
func (c *Context) ParamUint(key string, defaults ...uint) uint {
	return parseUintWithDefault(c.Param(key), defaults...)
}

// ParamInt64 returns the value of the parameter as an int64.
func (c *Context) ParamInt64(key string, defaults ...int64) int64 {
	return parseIntWithDefault(c.Param(key), 64, defaults...)
}

// queryValues returns the parsed query parameters, parsing them lazily on
// first access and caching the result so repeated Query* calls do not
// re-parse and re-allocate on every request.
func (c *Context) queryValues() url.Values {
	if c.query == nil {
		c.query = c.Request.URL.Query()
	}
	return c.query
}

// defaultTimezone returns the location used when parsing date/time values:
// the router's configured timezone (WithTimezone) if set, otherwise
// rex.DefaultTimezone.
func (c *Context) defaultTimezone() *time.Location {
	if c.router != nil && c.router.timezone != nil {
		return c.router.timezone
	}
	return DefaultTimezone
}

// State returns the shared application state injected via WithState/SetState,
// or nil when none was configured. Type-assert the result or prefer the
// type-safe rex.GetState[T] helper.
func (c *Context) State() any {
	if c.router == nil {
		return nil
	}
	return c.router.state
}

// GetState returns the application state as T. It reports false when no state
// was configured or the stored value does not match T.
func (c *Context) GetState[T any]() (T, bool) {
	var zero T
	if c.router == nil {
		return zero, false
	}
	state, ok := c.router.state.(T)
	return state, ok
}

// Query returns the value of the query as a string.
func (c *Context) Query(key string, defaults ...string) string {
	v := c.queryValues().Get(key)
	if v == "" {
		// check redirect query params
		opts, ok := c.redirectOptions()
		if ok {
			v = opts.QueryParams[key]
		}
	}

	if v == "" && len(defaults) > 0 {
		return defaults[0]
	}
	return v
}

// QueryInt returns the value of the query as an integer.
func (c *Context) QueryInt(key string, defaults ...int) int {
	return parseIntWithDefault(c.Query(key), strconv.IntSize, defaults...)
}

// QueryInt64 returns the value of the query as an int64.
func (c *Context) QueryInt64(key string, defaults ...int64) int64 {
	return parseIntWithDefault(c.Query(key), 64, defaults...)
}

// QueryUInt returns the value of the query as an unsigned integer.
// Negative values or values exceeding the platform uint size return the default.
func (c *Context) QueryUInt(key string, defaults ...uint) uint {
	return parseUintWithDefault(c.Query(key), defaults...)
}

// Set stores a value in the context
func (c *Context) Set(key string, value any) {
	if c.locals == nil {
		c.locals = make(map[string]any)
	}
	c.locals[key] = value
}

// Get retrieves a value from the context
func (c *Context) Get(key string) (value any, exists bool) {
	if c.locals == nil {
		return nil, false
	}
	value, exists = c.locals[key]
	return
}

// Locals returns the context values
func (c *Context) Locals() map[string]any {
	if c.locals == nil {
		c.locals = make(map[string]any)
	}
	return c.locals
}

// MustGet retrieves a value from the context or panics if the key does not exist.
func (c *Context) MustGet(key string) any {
	value, exists := c.Get(key)
	if !exists {
		panic("key not found: " + key)
	}
	return value
}

// GetOrEmpty retrieves a value from the context or returns nil if the key does not exist.
// This better when you want to type-cast the value to a specific type without checking for existence.
func (c *Context) GetOrEmpty(key string) any {
	value, exists := c.Get(key)
	if !exists {
		return nil
	}
	return value
}

// Redirect redirects the request to the given URL.
// The default status code is 303 (http.StatusSeeOther).
func (c *Context) Redirect(url string, status ...int) error {
	http.Redirect(c.Response, c.Request, url, First(status, http.StatusSeeOther))
	return nil
}

// IP returns the client's IP address.
//
// By default (no trusted proxies configured via the WithTrustProxy router
// option), proxy headers are never trusted and the remote address of the
// direct TCP peer is returned. This prevents clients from spoofing their IP
// by setting X-Forwarded-For or X-Real-Ip headers.
//
// When trusted proxies are configured and the request arrives from a trusted
// proxy, the client IP is resolved by walking X-Forwarded-For from right to
// left and returning the first untrusted address, falling back to X-Real-Ip,
// then to the remote address.
func (c *Context) IP() (string, error) {
	ip, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		ip = strings.TrimSpace(c.Request.RemoteAddr)
	}
	remoteIP := net.ParseIP(ip)
	if remoteIP == nil {
		return "", errors.New("invalid remote address")
	}

	router := c.router
	if router == nil || !router.isTrustedProxy(remoteIP) {
		return normalizeLoopback(remoteIP), nil
	}

	// Walk X-Forwarded-For right-to-left without allocating a []string.
	xff := c.Request.Header.Get("X-Forwarded-For")
	for xff != "" {
		var field string
		if idx := strings.LastIndexByte(xff, ','); idx >= 0 {
			field, xff = xff[idx+1:], xff[:idx]
		} else {
			field, xff = xff, ""
		}
		parsed := net.ParseIP(strings.TrimSpace(field))
		if parsed == nil {
			continue
		}
		if !router.isTrustedProxy(parsed) {
			return normalizeLoopback(parsed), nil
		}
	}

	if real := strings.TrimSpace(c.Request.Header.Get("X-Real-Ip")); real != "" {
		if parsed := net.ParseIP(real); parsed != nil && !router.isTrustedProxy(parsed) {
			return normalizeLoopback(parsed), nil
		}
	}

	return normalizeLoopback(remoteIP), nil
}

// normalizeLoopback maps IPv6 loopback (::1) to IPv4 loopback for convenience.
func normalizeLoopback(ip net.IP) string {
	if ip.Equal(net.IPv6loopback) {
		return "127.0.0.1"
	}
	return ip.String()
}

// TranslateErrors returns English translations for validation errors.
// If no translator is available, the raw validation tags are returned
// keyed by field name instead of panicking.
func (c *Context) TranslateErrors(errs validator.ValidationErrors) map[string]string {
	if c.router == nil || c.router.translator == nil {
		out := make(map[string]string, len(errs))
		for _, e := range errs {
			out[e.Field()] = e.Tag()
		}
		return out
	}
	return errs.Translate(c.router.translator)
}

// FormValue returns the form value for key.
func (c *Context) FormValue(key string) string {
	return c.Request.FormValue(key)
}

// FormValueInt returns the form value for key as an integer.
// If the value is not found or cannot be converted to an integer, it returns the default value.
func (c *Context) FormValueInt(key string, defaults ...int) int {
	return parseIntWithDefault(c.FormValue(key), strconv.IntSize, defaults...)
}

// FormValueUInt returns the form value for key as an unsigned integer.
// If the value is not found, negative, or cannot be converted to an unsigned
// integer, it returns the default value.
func (c *Context) FormValueUInt(key string, defaults ...uint) uint {
	return parseUintWithDefault(c.FormValue(key), defaults...)
}

// FormFile returns the first uploaded file for key.
func (c *Context) FormFile(key string) (multipart.File, *multipart.FileHeader, error) {
	return c.Request.FormFile(key)
}

// FormFiles returns all uploaded files for key after parsing the multipart form.
// The default max memory is the router's configured max memory (WithMaxMemory),
// or DefaultMaxMemory if not set. It can be overridden per call.
func (c *Context) FormFiles(key string, maxMemory ...int64) ([]*multipart.FileHeader, error) {
	var memory int64 = DefaultMaxMemory
	if c.router != nil && c.router.maxMemory > 0 {
		memory = c.router.maxMemory
	}
	if len(maxMemory) > 0 {
		memory = maxMemory[0]
	}
	err := c.Request.ParseMultipartForm(memory)
	if err != nil {
		return nil, err
	}

	if files, ok := c.Request.MultipartForm.File[key]; ok {
		return files, nil
	}

	return nil, fmt.Errorf("no files match the given key")
}

// SaveFile saves a multipart file to target.
func (c *Context) SaveFile(fh *multipart.FileHeader, target string) error {
	src, err := fh.Open()
	if err != nil {
		return errors.Wrap(err, "failed to open multipart.FileHeader")
	}
	defer src.Close()

	out, err := os.Create(target)
	if err != nil {
		return errors.Wrap(err, "failed to create file")
	}
	defer out.Close()

	_, err = io.Copy(out, src)
	return err
}

// StatusCode returns the status code written to the response.
func (c *Context) StatusCode() int {
	if wrapped, ok := c.Response.(*ResponseWriter); ok {
		return wrapped.StatusCode()
	}
	return http.StatusOK
}

// SetContentType sets the Content-Type header for the response.
func (c *Context) SetContentType(contentType string) {
	if c.contentTypeSet {
		return
	}
	c.Response.Header().Set("Content-Type", contentType)
	c.contentTypeSet = true
}

// Latency returns the duration of the request including the time it took to write the response,
// execute the middleware and the handler.
func (c *Context) Latency() time.Duration {
	return c.latency
}

// setLatency sets the latency; used by router after handler completes
func (c *Context) setLatency(d time.Duration) { c.latency = d }

// WrapWriter applies a function to wrap the underlying writer safely
// and returns a restore function to revert to the previous writer.
func (c *Context) WrapWriter(fn func(http.ResponseWriter) http.ResponseWriter) (restore func()) {
	old := c.Response
	c.Response = fn(old)
	return func() { c.Response = old }
}

// SetSkipBody toggles writing of the response body (for HEAD requests)
func (c *Context) SetSkipBody(enabled bool) {
	if rw, ok := c.Response.(*ResponseWriter); ok {
		rw.SetSkipBody(enabled)
	}
}

// SkipBody indicates if the response body should be skipped
func (c *Context) SkipBody() bool {
	if rw, ok := c.Response.(*ResponseWriter); ok {
		return rw.SkipBody()
	}
	return false
}

// Router returns the router associated with the request.
func (c *Context) Router() *Router {
	return c.router
}

// Path returns the request path.
func (c *Context) Path() string {
	return c.Request.URL.Path
}

// Method returns the request method.
func (c *Context) Method() string {
	return c.Request.Method
}

// Host returns the request host.
func (c *Context) Host() string {
	return c.Request.Host
}

// URL returns the request URL.
func (c *Context) URL() string {
	return c.Request.URL.String()
}
