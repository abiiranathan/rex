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

			// Create gzip writer
			gw, err := gzip.NewWriterLevel(c.Response, gzip.DefaultCompression)
			if err != nil {
				return err
			}

			// Create wrapper
			grw := &gzipWriter{
				ResponseWriter: c.Response,
				gw:             gw,
				headerWritten:  false,
			}

			// Replace the response writer
			originalWriter := c.Response
			c.Response = grw

			// Call next handler
			err = next(c)

			// Ensure gzip writer is properly closed
			if closeErr := gw.Close(); closeErr != nil && err == nil {
				err = closeErr
			}

			// Restore original writer
			c.Response = originalWriter

			return err
		}
	}
}

// GzipLevel creates a gzip middleware with a specific compression level.
func GzipLevel(level int, skipPaths ...string) rex.Middleware {
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

			c.SetHeader("Content-Encoding", "gzip")

			// No content Length on compressed data.
			c.DelHeader("Content-Length")

			gw, err := gzip.NewWriterLevel(c.Response, level)
			if err != nil {
				return err
			}
			defer gw.Close()

			grw := &gzipWriter{
				ResponseWriter: c.Response,
				gw:             gw,
			}

			originalWriter := c.Response
			c.Response = grw
			defer func() { c.Response = originalWriter }()
			return next(c)
		}
	}
}
