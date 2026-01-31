package brotli

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/abiiranathan/rex"
	"github.com/andybalholm/brotli"
	"github.com/andybalholm/brotli/matchfinder"
)

type brotliWriter struct {
	http.ResponseWriter
	bw            *matchfinder.Writer
	status        int
	headerWritten bool
}

func (b *brotliWriter) WriteHeader(code int) {
	if b.headerWritten {
		return
	}
	// Only set headers if we are actually writing the response
	b.status = code
	if code != http.StatusNoContent && code != http.StatusNotModified {
		b.ResponseWriter.Header().Set("Content-Encoding", "br")
		b.ResponseWriter.Header().Del("Content-Length")
	}
	b.ResponseWriter.WriteHeader(code)
	b.headerWritten = true
}

func (b *brotliWriter) Write(p []byte) (int, error) {
	if !b.headerWritten {
		b.WriteHeader(http.StatusOK)
	}
	return b.bw.Write(p)
}

func (b *brotliWriter) Flush() {
	if flusher, ok := b.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (b *brotliWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := b.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("http.Hijacker interface is not supported")
}

// Brotli compression middleware.
func Brotli(skipPaths ...string) rex.Middleware {
	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			for _, path := range skipPaths {
				if strings.HasPrefix(c.Path(), path) {
					return next(c)
				}
			}

			if !strings.Contains(c.GetHeader("Accept-Encoding"), "br") {
				return next(c)
			}

			var bw *matchfinder.Writer
			var wb *brotliWriter

			restore := c.WrapWriter(func(w http.ResponseWriter) http.ResponseWriter {
				bw = brotli.NewWriterV2(w, 7)
				wb = &brotliWriter{ResponseWriter: w, bw: bw}
				return wb
			})

			defer restore()
			defer func() {
				if wb != nil && wb.headerWritten && (wb.status == http.StatusNoContent || wb.status == http.StatusNotModified) {
					return
				}
				if bw != nil {
					bw.Close()
				}
			}()

			return next(c)
		}
	}
}
