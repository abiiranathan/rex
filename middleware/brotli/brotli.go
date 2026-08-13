// Package brotli provides Brotli compression middleware for rex routers.
package brotli

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/abiiranathan/rex"
	"github.com/andybalholm/brotli"
	"github.com/andybalholm/brotli/matchfinder"
)

// brotliWriter wraps http.ResponseWriter, transparently compressing the
// response body with Brotli and deferring header writes so that
// Content-Encoding can be set (or skipped) correctly for the actual status
// code.
type brotliWriter struct {
	http.ResponseWriter
	bw            *matchfinder.Writer
	status        int
	headerWritten bool
}

// WriteHeader writes the response status code and compression headers.
// Calling this more than once has no effect.
func (b *brotliWriter) WriteHeader(code int) {
	if b.headerWritten {
		return
	}
	// Only set headers if we are actually writing the response.
	b.status = code
	if code != http.StatusNoContent && code != http.StatusNotModified {
		b.ResponseWriter.Header().Set("Content-Encoding", "br")
		b.ResponseWriter.Header().Del("Content-Length")
	}
	b.ResponseWriter.WriteHeader(code)
	b.headerWritten = true
}

// Write compresses p and writes it to the underlying response.
func (b *brotliWriter) Write(p []byte) (int, error) {
	if !b.headerWritten {
		code := b.status
		if code == 0 {
			code = http.StatusOK
		}
		b.WriteHeader(code)
	}
	return b.bw.Write(p)
}

// SetStatus records a status code without committing headers yet.
func (b *brotliWriter) SetStatus(code int) {
	if b.headerWritten {
		return
	}
	b.status = code
}

// Flush flushes any buffered response data.
func (b *brotliWriter) Flush() {
	if flusher, ok := b.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack implements http.Hijacker when supported by the underlying writer.
func (b *brotliWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := b.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("http.Hijacker interface is not supported")
}

// reset prepares a pooled brotliWriter, including its compressor, for reuse
// against a new underlying ResponseWriter.
func (b *brotliWriter) reset(w http.ResponseWriter, level int) {
	b.ResponseWriter = w
	b.status = 0
	b.headerWritten = false
	if b.bw == nil {
		b.bw = brotli.NewWriterV2(w, level)
		return
	}
	b.bw.Reset(w)
}

// writerPool reuses brotliWriter instances, including their underlying
// matchfinder.Writer compressors, to avoid per-request heap allocation.
var writerPool = sync.Pool{
	New: func() any {
		return &brotliWriter{}
	},
}

// Brotli returns middleware that compresses responses with Brotli when the
// client supports it (signaled via the Accept-Encoding request header).
// Paths matching any of skipPaths by prefix bypass compression entirely.
//
// compressionLevel controls the trade-off between CPU cost and compression
// ratio, and must be between 0 and 11 inclusive. Values outside this range
// are clamped to the nearest valid level. If compressionLevel is 0 pass a
// negative value or -1 is not accepted as "use default" — pass 6 explicitly,
// or use DefaultCompressionLevel.
//
// Brotli's cost curve is not linear: each additional level does
// significantly more work (larger match-search window, deeper context
// modeling, static dictionary lookups), so the "right" level depends on
// your workload and should be benchmarked against representative payloads
// rather than assumed. As a starting point:
//
//   - Levels 1-4: fastest, lowest CPU cost, weakest compression ratio.
//     Suitable for latency-sensitive, small, or frequently-changing dynamic
//     responses (e.g. JSON API payloads) where the encoding cost must stay
//     well below the cost of generating the response itself.
//   - Levels 5-7: balanced middle ground. Reasonable ratio improvement over
//     the low end at moderate additional CPU cost. A common default for
//     general-purpose dynamic HTTP compression; this package defaults to 6.
//   - Levels 8-9: noticeably higher CPU cost for incremental ratio gains
//     (diminishing returns). Only worth it if compression happens
//     infrequently relative to how often the compressed bytes are served
//     (e.g. semi-static responses cached downstream).
//   - Levels 10-11: maximum compression, substantial CPU cost. Intended for
//     compressing static assets once at build time (e.g. pre-generating
//     .br files for JS/CSS bundles), not for per-request dynamic
//     compression. Using these levels on the request path will materially
//     increase response latency under load.
//
// Because the cost/ratio trade-off depends on payload size, redundancy, and
// request rate, benchmark with your own representative payloads before
// choosing a non-default level for production traffic.
func Brotli(compressionLevel int, skipPaths ...string) rex.Middleware {
	const (
		minCompressionLevel     = 0
		maxCompressionLevel     = 11
		DefaultCompressionLevel = 6
	)

	level := compressionLevel
	switch {
	case level < minCompressionLevel:
		level = minCompressionLevel
	case level > maxCompressionLevel:
		level = maxCompressionLevel
	}

	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			// If the response is already encoded (e.g. an outer middleware or a
			// pre-set header signals pre-compressed content), don't wrap it again —
			// double-compressing would corrupt the response for the client, which
			// only reverses a single Content-Encoding transformation.
			//This only catches the case where Content-Encoding was set
			// before this middleware's turn — i.e. by an outer middleware or a static pre-set header.
			if c.Response.Header().Get("Content-Encoding") != "" {
				return next(c)
			}

			for _, path := range skipPaths {
				if strings.HasPrefix(c.Path(), path) {
					return next(c)
				}
			}

			// Brotli not supported.
			if !strings.Contains(c.GetHeader("Accept-Encoding"), "br") {
				return next(c)
			}

			wb := writerPool.Get().(*brotliWriter)

			restore := c.WrapWriter(func(w http.ResponseWriter) http.ResponseWriter {
				wb.reset(w, level)
				return wb
			})

			defer restore()
			defer func() {
				// Non-committing statuses (204/304) never had a Brotli
				// stream opened for the client, so there is nothing to
				// close on the compressor.
				skip := wb.headerWritten && (wb.status == http.StatusNoContent || wb.status == http.StatusNotModified)
				if !skip {
					_ = wb.bw.Close()
				}
				// Drop the reference to the underlying ResponseWriter so a
				// future borrower doesn't retain this request's writer
				// between uses. The compressor (bw) is intentionally kept
				// so its buffers are reused, not the ResponseWriter.
				wb.ResponseWriter = nil
				writerPool.Put(wb)
			}()

			return next(c)
		}
	}
}
