package requestid

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abiiranathan/rex"
)

func TestRequestID(t *testing.T) {
	r := rex.NewRouter()
	r.Use(New())

	r.GET("/", func(c *rex.Context) error {
		return c.String("ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	rid := w.Header().Get(HeaderKey)
	if rid == "" {
		t.Error("Expected X-Request-ID header")
	}

	if len(rid) != 32 {
		t.Errorf("Expected 32 chars ID, got %d", len(rid))
	}
}

func TestRequestIDConfig(t *testing.T) {
	r := rex.NewRouter()
	r.Use(WithConfig(Config{
		Header: "X-Trace-ID",
		Generator: func() string {
			return "custom-id"
		},
	}))

	r.GET("/", func(c *rex.Context) error {
		return c.String("ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)

	rid := w.Header().Get("X-Trace-ID")
	if rid != "custom-id" {
		t.Errorf("Expected custom-id, got %s", rid)
	}
}

func TestRequestIDExisting(t *testing.T) {
	r := rex.NewRouter()
	r.Use(New())

	r.GET("/", func(c *rex.Context) error {
		return c.String("ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(HeaderKey, "existing-id")
	r.ServeHTTP(w, req)

	rid := w.Header().Get(HeaderKey)
	if rid != "existing-id" {
		t.Errorf("Expected existing-id, got %s", rid)
	}
}
