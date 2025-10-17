package gzip

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/abiiranathan/rex"
)

type gzipWriter struct {
	http.ResponseWriter
	gw            *gzip.Writer
	headerWritten bool
}

func (g *gzipWriter) WriteHeader(code int) {
	if !g.headerWritten {
		// Only set gzip headers when we actually write content
		g.ResponseWriter.Header().Set("Content-Encoding", "gzip")
		g.ResponseWriter.Header().Del("Content-Length")
		g.headerWritten = true
	}
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipWriter) Write(p []byte) (int, error) {
	if !g.headerWritten {
		// Ensure headers are written before first write
		g.WriteHeader(http.StatusOK)
	}
	return g.gw.Write(p)
}

func (g *gzipWriter) Flush() {
	if g.gw != nil {
		g.gw.Flush()
	}
	if flusher, ok := g.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (g *gzipWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := g.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("http.Hijacker interface is not supported")
}

func Gzip(skipPaths ...string) rex.Middleware {
	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			// Skip if path is in skipPaths
			for _, path := range skipPaths {
				if strings.HasPrefix(c.Path(), path) {
					return next(c)
				}
			}

			// Skip if client doesn't accept gzip
			if !strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
				return next(c)
			}

			var (
				gw      *gzip.Writer
				created bool
			)
			restore := c.WrapWriter(func(w http.ResponseWriter) http.ResponseWriter {
				z, err := gzip.NewWriterLevel(w, gzip.DefaultCompression)
				if err != nil {
					// Fallback: do not wrap; keep original writer
					return w
				}
				gw = z
				created = true
				return &gzipWriter{ResponseWriter: w, gw: z}
			})
			defer restore()
			if created {
				defer gw.Close()
			}

			return next(c)
		}
	}
}

// GzipLevel creates a gzip middleware with a specific compression level.
func GzipLevel(level int, skipPaths ...string) rex.Middleware {
	if level < gzip.HuffmanOnly || level > gzip.BestCompression {
		panic(fmt.Errorf("gzip: invalid compression level: %d", level))
	}

	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			for _, path := range skipPaths {
				if strings.HasPrefix(c.Path(), path) {
					return next(c)
				}
			}

			if !strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
				return next(c)
			}

			var gw *gzip.Writer
			restore := c.WrapWriter(func(w http.ResponseWriter) http.ResponseWriter {
				z, _ := gzip.NewWriterLevel(w, level)
				gw = z
				return &gzipWriter{ResponseWriter: w, gw: z}
			})
			defer restore()
			defer gw.Close()

			return next(c)
		}
	}
}
