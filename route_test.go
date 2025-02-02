package rex_test

import (
	"log"
	"net/http/httptest"
	"testing"

	"github.com/abiiranathan/rex"
)

func LogURL(next rex.HandlerFunc) rex.HandlerFunc {
	return func(c *rex.Context) error {
		log.Println(c.URL())
		return next(c)
	}
}

func TestRouteMethods(t *testing.T) {
	r := rex.NewRouter()

	r.With(LogURL).GET("/test", func(c *rex.Context) error {
		return c.String("test")
	})

	r.With(LogURL).GET("/test2", func(c *rex.Context) error {
		return c.String("test2")
	})

	r.With(LogURL).GET("/test3", func(c *rex.Context) error {
		return c.String("test3")
	})

	r.With(LogURL).POST("/test4", func(c *rex.Context) error {
		return c.String("test4")
	})

	r.With(LogURL).PUT("/test5", func(c *rex.Context) error {
		return c.String("test5")
	})

	r.With(LogURL).DELETE("/test6", func(c *rex.Context) error {
		return c.String("test6")
	})

	r.With(LogURL).PATCH("/test7", func(c *rex.Context) error {
		return c.String("test7")
	})

	r.With(LogURL).OPTIONS("/test8", func(c *rex.Context) error {
		return c.String("test8")
	})

	r.With(LogURL).HEAD("/test9", func(c *rex.Context) error {
		return c.String("test9")
	})

	r.With(LogURL).CONNECT("/test10", func(c *rex.Context) error {
		return c.String("test10")
	})

	r.With(LogURL).TRACE("/test11", func(c *rex.Context) error {
		return c.String("test11")
	})

	tests := []struct {
		name     string
		method   string
		path     string
		expected string
	}{
		{"GET", "GET", "/test", "test"},
		{"GET", "GET", "/test2", "test2"},
		{"GET", "GET", "/test3", "test3"},
		{"POST", "POST", "/test4", "test4"},
		{"PUT", "PUT", "/test5", "test5"},
		{"DELETE", "DELETE", "/test6", "test6"},
		{"PATCH", "PATCH", "/test7", "test7"},
		{"OPTIONS", "OPTIONS", "/test8", "test8"},
		{"HEAD", "HEAD", "/test9", "test9"},
		{"CONNECT", "CONNECT", "/test10", "test10"},
		{"TRACE", "TRACE", "/test11", "test11"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			r.ServeHTTP(w, req)
			if w.Body.String() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, w.Body.String())
			}
		})
	}
}
