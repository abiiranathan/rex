package security

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abiiranathan/rex"
)

func TestSecurityDefault(t *testing.T) {
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

	headers := w.Header()
	if headers.Get("X-XSS-Protection") != "1; mode=block" {
		t.Error("X-XSS-Protection mismatch")
	}
	if headers.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("X-Content-Type-Options mismatch")
	}
	if headers.Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Error("X-Frame-Options mismatch")
	}
	if headers.Get("Strict-Transport-Security") != "" {
		t.Error("HSTS should be disabled by default")
	}
}

func TestSecurityConfig(t *testing.T) {
	r := rex.NewRouter()
	config := Config{
		XSSProtection:         "0",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		HSTSMaxAge:            31536000,
		HSTSPreload:           true,
		ContentSecurityPolicy: "default-src 'self'",
		ReferrerPolicy:        "no-referrer",
	}
	r.Use(WithConfig(config))

	r.GET("/", func(c *rex.Context) error {
		return c.String("ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)

	headers := w.Header()
	if headers.Get("X-XSS-Protection") != "0" {
		t.Error("X-XSS-Protection mismatch")
	}
	if headers.Get("X-Frame-Options") != "DENY" {
		t.Error("X-Frame-Options mismatch")
	}
	if headers.Get("Strict-Transport-Security") != "max-age=31536000; includeSubDomains; preload" {
		t.Errorf("HSTS mismatch, got: %s", headers.Get("Strict-Transport-Security"))
	}
	if headers.Get("Content-Security-Policy") != "default-src 'self'" {
		t.Error("CSP mismatch")
	}
	if headers.Get("Referrer-Policy") != "no-referrer" {
		t.Error("Referrer-Policy mismatch")
	}
}
