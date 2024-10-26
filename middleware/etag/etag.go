package etag

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"fmt"
	"hash"
	"io"
	"net"
	"net/http"

	"github.com/abiiranathan/rex"
)

type etagResponseWriter struct {
	http.ResponseWriter              // the original ResponseWriter
	buf                 bytes.Buffer // buffer to store the response body
	hash                hash.Hash    // hash to calculate the ETag
	w                   io.Writer    // multiwriter to write to both the buffer and the hash
	status              int          // status code of the response
	written             bool         // whether the header has been written
}

func (e *etagResponseWriter) WriteHeader(code int) {
	e.status = code
	e.written = true
	// Don't actually write the header yet, we'll do that later
}

func (e *etagResponseWriter) Write(p []byte) (int, error) {
	if !e.written {
		// If WriteHeader was not explicitly called, we need to set the status
		e.status = http.StatusOK
		e.written = true
	}
	return e.w.Write(p)
}

func (e *etagResponseWriter) Flush() {
	if f, ok := e.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (e *etagResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := e.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func New(skip ...func(r *http.Request) bool) rex.Middleware {
	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			var skipEtag bool
			for _, s := range skip {
				if s(c.Request) {
					skipEtag = true
					break
				}
			}

			if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
				skipEtag = true
			}

			if skipEtag {
				return next(c)
			}

			ew := &etagResponseWriter{
				ResponseWriter: c.Response,
				buf:            bytes.Buffer{},
				hash:           sha1.New(),
				status:         http.StatusOK,
			}
			ew.w = io.MultiWriter(&ew.buf, ew.hash)

			next.ServeHTTP(ew, c.Request)

			if ew.status != http.StatusOK {
				// For non-200 responses, write the status and body without ETag
				c.WriteHeader(ew.status)
				_, err := ew.buf.WriteTo(c.Response)
				return err
			}

			etag := fmt.Sprintf(`"%x"`, ew.hash.Sum(nil))
			c.Response.Header().Set("ETag", etag)

			// Check If-None-Match and If-Match headers and return 304 or 412 if needed
			ifNoneMatch := c.Request.Header.Get("If-None-Match")
			if ifNoneMatch == etag {
				return c.WriteHeader(http.StatusNotModified)
			}

			// If-Match is not supported for GET requests
			ifMatch := c.Request.Header.Get("If-Match")
			if ifMatch != "" && ifMatch != etag {
				// If-Match header is present and doesn't match the ETag
				return c.WriteHeader(http.StatusPreconditionFailed)
			}

			// Write the status and body for 200 OK responses
			c.WriteHeader(ew.status)
			_, err := ew.buf.WriteTo(c.Response)
			return err
		}
	}
}
