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
	"strings"

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
	if e.written {
		return // Prevent multiple calls
	}
	e.status = code
	e.written = true

	// Don't call e.ResponseWriter.WriteHeader(code) yet for 200 OK
	// For non-200, we should write through immediately
	if code != http.StatusOK {
		e.ResponseWriter.WriteHeader(code)
	}
}

func (e *etagResponseWriter) Write(p []byte) (int, error) {
	if !e.written {
		e.status = http.StatusOK
		e.written = true
	}

	// If status is not 200, write directly to the underlying response writer
	if e.status != http.StatusOK {
		return e.ResponseWriter.Write(p)
	}

	// For 200 OK, buffer the response
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

func (e *etagResponseWriter) Status() int {
	return e.status
}

// Create a new etag middleware.
// The middleware will ignore Server-Sent events and websocket requests by default.
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

			if c.Method() != http.MethodGet && c.Method() != http.MethodHead {
				skipEtag = true
			}

			// Skip WebSocket upgrade requests
			if strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
				skipEtag = true
			}

			// Skip Server-Sent Events (SSE) requests
			if strings.Contains(c.GetHeader("Accept"), "text/event-stream") {
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

			restore := c.WrapWriter(func(w http.ResponseWriter) http.ResponseWriter {
				ew.ResponseWriter = w
				return ew
			})

			err := next(c)
			restore()

			// Return error after restoring the response writer
			if err != nil {
				return err
			}

			// If status is not 200 OK, the response was already written through
			if ew.status != http.StatusOK {
				return nil
			}

			// For 200 OK responses, apply ETag logic
			etag := fmt.Sprintf(`"%x"`, ew.hash.Sum(nil))
			c.SetHeader("ETag", etag)

			// Check If-None-Match
			ifNoneMatch := c.GetHeader("If-None-Match")
			if ifNoneMatch == etag {
				c.WriteHeader(http.StatusNotModified)
				return nil
			}

			// Check If-Match
			ifMatch := c.GetHeader("If-Match")
			if ifMatch != "" && ifMatch != etag {
				c.WriteHeader(http.StatusPreconditionFailed)
				return nil
			}

			// Write the buffered 200 OK response
			// IMPORTANT: Write header first, then copy buffer to the ORIGINAL ResponseWriter
			ew.ResponseWriter.WriteHeader(http.StatusOK)
			_, err = ew.buf.WriteTo(ew.ResponseWriter)
			return err
		}
	}

}
