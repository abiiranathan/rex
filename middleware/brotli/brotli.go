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
	bw *matchfinder.Writer
}

func (b *brotliWriter) WriteHeader(code int) {
	b.ResponseWriter.Header().Del("Content-Length")
	b.ResponseWriter.WriteHeader(code)
}

func (b *brotliWriter) Write(p []byte) (int, error) {
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

			c.SetHeader("Content-Encoding", "br")

			// No content Length on compressed data.
			c.DelHeader("Content-Length")

			var bw *matchfinder.Writer
			restore := c.WrapWriter(func(w http.ResponseWriter) http.ResponseWriter {
				bw = brotli.NewWriterV2(w, 7)
				return &brotliWriter{ResponseWriter: w, bw: bw}
			})

			defer restore()
			defer bw.Close()

			return next(c)
		}
	}
}
