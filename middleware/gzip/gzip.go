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
	gw *gzip.Writer
}

func (g *gzipWriter) WriteHeader(code int) {
	g.ResponseWriter.Header().Del("Content-Length")
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipWriter) Write(p []byte) (int, error) {
	return g.gw.Write(p)
}

func (g *gzipWriter) Flush() {
	g.gw.Flush()
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

// Gzip compression middleware.
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

			c.SetHeader("Content-Encoding", "gzip")

			// No content Length on compressed data.
			c.DelHeader("Content-Length")

			gw, err := gzip.NewWriterLevel(c.Response, gzip.DefaultCompression)
			if err != nil {
				return err
			}
			defer gw.Close()

			grw := &gzipWriter{ResponseWriter: c.Response, gw: gw}

			originalWriter := c.Response
			c.Response = grw
			err = next(c)
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
