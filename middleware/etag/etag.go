// Package etag provides ETag middleware for rex routers.
package etag

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"fmt"
	"hash"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/abiiranathan/rex"
)

// writerPool reuses etagResponseWriter instances to avoid per-request heap allocation.
var writerPool = sync.Pool{
	New: func() any {
		ew := &etagResponseWriter{}
		ew.hash = sha1.New()
		return ew
	},
}

// etagResponseWriter intercepts writes to buffer the body and compute a hash
// simultaneously, deferring the actual write until the ETag is known.
type etagResponseWriter struct {
	http.ResponseWriter
	buf         bytes.Buffer
	hash        hash.Hash
	status      int
	wroteHeader bool
}

func (e *etagResponseWriter) reset(w http.ResponseWriter) {
	e.ResponseWriter = w
	e.buf.Reset()
	e.hash.Reset()
	e.status = http.StatusOK
	e.wroteHeader = false
}

// WriteHeader captures the status code. For 200 responses we defer the actual
// WriteHeader call until we know the ETag; for all others we pass through immediately.
func (e *etagResponseWriter) WriteHeader(code int) {
	if e.wroteHeader {
		return
	}
	e.wroteHeader = true
	e.status = code
	if code != http.StatusOK {
		e.ResponseWriter.WriteHeader(code)
	}
}

// Write fans out to both the hash and the buffer for 200 responses.
// Non-200 writes bypass buffering entirely.
func (e *etagResponseWriter) Write(p []byte) (int, error) {
	if !e.wroteHeader {
		e.wroteHeader = true
		e.status = http.StatusOK
	}
	if e.status != http.StatusOK {
		return e.ResponseWriter.Write(p)
	}
	// Write to hash and buf in two calls — avoids io.MultiWriter allocation
	// and keeps the two writes on the same cache lines.
	n, err := e.buf.Write(p)
	if err != nil {
		return n, err
	}
	_, _ = e.hash.Write(p) // sha1.Write never returns an error
	return n, nil
}

// Unwrap lets http.ResponseController reach the underlying writer (Go 1.20+).
func (e *etagResponseWriter) Unwrap() http.ResponseWriter {
	return e.ResponseWriter
}

// Flush forwards to the underlying writer if it supports flushing.
// Note: flushing mid-response means we can no longer compute an ETag for the
// full body, so callers should not mix streaming with ETag middleware.
func (e *etagResponseWriter) Flush() {
	if f, ok := e.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack supports WebSocket upgrades on the underlying connection.
func (e *etagResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := e.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// SkipFunc reports whether ETag processing should be skipped for a request.
type SkipFunc func(r *http.Request) bool

// New returns middleware that computes and validates ETags for cacheable GET/HEAD
// responses. It buffers the response body only for 200 OK responses; all other
// status codes are passed through without buffering or hashing.
//
// Optional SkipFunc predicates short-circuit ETag processing when any returns true.
func New(skip ...SkipFunc) rex.Middleware {
	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			// Fast-path: skip non-cacheable requests without touching the pool.
			if !isCacheable(c, skip) {
				return next(c)
			}

			ew := writerPool.Get().(*etagResponseWriter)
			ew.reset(c.Response)

			// Swap the writer on the context; restore unconditionally on exit.
			original := c.Response
			c.Response = ew
			err := next(c)
			c.Response = original

			// Return the writer to the pool before any early returns so it
			// is available to other goroutines as soon as possible.
			status := ew.status
			etag := fmt.Sprintf(`"%x"`, ew.hash.Sum(nil))
			body := ew.buf.Bytes()

			// Guard against oversized buffers persisting in the pool.
			if ew.buf.Cap() > 512<<10 { // 512 KB
				// Discard the oversized writer; the pool will allocate a fresh one
				// via New() when next needed. Putting a replacement here is unnecessary
				// and was masking the fact that we're intentionally discarding ew.
			} else {
				writerPool.Put(ew)
			}

			if err != nil {
				return err
			}

			// Non-200 responses were already written through; nothing left to do.
			if status != http.StatusOK {
				return nil
			}

			// Validate conditional request headers before committing the response.
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

			// Commit: write ETag, status, then body.
			c.SetHeader("ETag", etag)
			original.WriteHeader(http.StatusOK)
			_, err = original.Write(body)
			return err
		}
	}
}

// isCacheable returns true when ETag processing should run for this request.
func isCacheable(c *rex.Context, skip []SkipFunc) bool {
	method := c.Method()
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	if strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
		return false
	}
	if strings.Contains(c.GetHeader("Accept"), "text/event-stream") {
		return false
	}
	for _, s := range skip {
		if s(c.Request) {
			return false
		}
	}
	return true
}
