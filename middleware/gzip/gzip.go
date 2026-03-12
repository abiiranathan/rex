// Package gzip provides gzip compression middleware for rex routers.
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
	status        int
	headerWritten bool
}

// WriteHeader writes the response status code and gzip headers.
func (g *gzipWriter) WriteHeader(code int) {
	if g.headerWritten {
		return
	}
	g.status = code
	if code != http.StatusNoContent && code != http.StatusNotModified {
		g.ResponseWriter.Header().Set("Content-Encoding", "gzip")
		g.ResponseWriter.Header().Del("Content-Length")
	}
	g.headerWritten = true
	g.ResponseWriter.WriteHeader(code)
}

// Write compresses p and writes it to the underlying response.
func (g *gzipWriter) Write(p []byte) (int, error) {
	if !g.headerWritten {
		code := g.status
		if code == 0 {
			code = http.StatusOK
		}
		g.WriteHeader(code)
	}
	return g.gw.Write(p)
}

// SetStatus records a status code without committing headers yet.
func (g *gzipWriter) SetStatus(code int) {
	if g.headerWritten {
		return
	}
	g.status = code
}

// Flush flushes any buffered response data.
func (g *gzipWriter) Flush() {
	if g.gw != nil {
		g.gw.Flush()
	}
	if flusher, ok := g.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack implements http.Hijacker when supported by the underlying writer.
func (g *gzipWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := g.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("http.Hijacker interface is not supported")
}

// Gzip returns middleware that compresses responses with gzip when the client supports it.
func Gzip(skipPaths ...string) rex.Middleware {
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

			var (
				gw      *gzip.Writer
				wb      *gzipWriter
				created bool
			)
			restore := c.WrapWriter(func(w http.ResponseWriter) http.ResponseWriter {
				z, err := gzip.NewWriterLevel(w, gzip.DefaultCompression)
				if err != nil {
					return w
				}
				gw = z
				created = true
				wb = &gzipWriter{ResponseWriter: w, gw: z}
				return wb
			})
			defer restore()
			if created {
				defer func() {
					if wb != nil && wb.headerWritten && (wb.status == http.StatusNoContent || wb.status == http.StatusNotModified) {
						return
					}
					gw.Close()
				}()
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
			var wb *gzipWriter

			restore := c.WrapWriter(func(w http.ResponseWriter) http.ResponseWriter {
				z, _ := gzip.NewWriterLevel(w, level)
				gw = z
				wb = &gzipWriter{ResponseWriter: w, gw: z}
				return wb
			})
			defer restore()
			defer func() {
				if wb != nil && wb.headerWritten && (wb.status == http.StatusNoContent || wb.status == http.StatusNotModified) {
					return
				}
				gw.Close()
			}()

			return next(c)
		}
	}
}
