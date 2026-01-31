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
		return
	}
	e.status = code
	e.written = true
	if code != http.StatusOK {
		e.ResponseWriter.WriteHeader(code)
	}
}

func (e *etagResponseWriter) Write(p []byte) (int, error) {
	if !e.written {
		e.status = http.StatusOK
		e.written = true
	}

	if e.status != http.StatusOK {
		return e.ResponseWriter.Write(p)
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

func (e *etagResponseWriter) Status() int {
	return e.status
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

			if c.Method() != http.MethodGet && c.Method() != http.MethodHead {
				skipEtag = true
			}

			if strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
				skipEtag = true
			}

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
			restore() // Restore c.Response to original

			if err != nil {
				return err
			}

			if ew.status != http.StatusOK {
				return nil
			}

			etag := fmt.Sprintf(`"%x"`, ew.hash.Sum(nil))
			c.SetHeader("ETag", etag)

			ifNoneMatch := c.GetHeader("If-None-Match")
			if ifNoneMatch == etag {
				c.WriteHeader(http.StatusNotModified)
				return nil
			}

			ifMatch := c.GetHeader("If-Match")
			if ifMatch != "" && ifMatch != etag {
				c.WriteHeader(http.StatusPreconditionFailed)
				return nil
			}

			// Write buffered response to original writer
			// Note: We need to write the header to the ORIGINAL writer now.
			// ew.ResponseWriter is likely the original writer (or the one below etag).
			ew.ResponseWriter.WriteHeader(http.StatusOK)
			_, err = ew.buf.WriteTo(ew.ResponseWriter)
			return err
		}
	}
}
