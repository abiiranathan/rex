package logger

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abiiranathan/rex"
)

func setupRouterWithLogger(t *testing.T, cfg *Config) (*rex.Router, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	if cfg == nil {
		cfg = &Config{}
	}
	cfg.Output = &buf
	r := rex.NewRouter()
	r.Use(New(cfg))
	r.GET("/hello", func(c *rex.Context) error {
		return c.String("ok")
	})
	return r, &buf
}

func TestLogger_TextFormat_Basic(t *testing.T) {
	r, buf := setupRouterWithLogger(t, &Config{Format: TextFormat})
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", w.Code)
	}
	out := buf.String()
	if !strings.Contains(out, "status=") || !strings.Contains(out, "method=") || !strings.Contains(out, "path=") {
		t.Fatalf("expected basic keys in text log, got: %s", out)
	}

	t.Log(out)
}

func TestLogger_JSONFormat_Basic(t *testing.T) {
	r, buf := setupRouterWithLogger(t, &Config{Format: JSONFormat})
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", w.Code)
	}
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	var entry map[string]any
	if err := dec.Decode(&entry); err != nil {
		t.Fatalf("expected JSON log, decode error: %v; raw: %s", err, buf.String())
	}

	t.Log(entry)
	if int(entry["status"].(float64)) != http.StatusOK {
		t.Fatalf("expected status %d, got %v", http.StatusOK, entry["status"])
	}

	if entry["method"] != http.MethodGet {
		t.Fatalf("expected method %s, got %s", http.MethodGet, entry["method"])
	}
	if entry["path"] != "/hello" {
		t.Fatalf("expected path %s, got %s", "/hello", entry["path"])
	}
	if entry["latency"] == 0 {
		t.Fatalf("expected latency, got 0")
	}
	if entry["user_agent"] == "" {
		t.Fatalf("expected user_agent, got empty")
	}
	if entry["ip"] == "" {
		t.Fatalf("expected ip, got empty")
	}
}

func TestLogger_SkipPath(t *testing.T) {
	cfg := &Config{Skip: []string{"/hello"}}
	r, buf := setupRouterWithLogger(t, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	r.ServeHTTP(w, req)
	if buf.Len() != 0 {
		t.Fatalf("expected no logs for skipped path, got: %s", buf.String())
	}
	t.Log(buf.String())
}

func TestLogger_SkipIf(t *testing.T) {
	cfg := &Config{SkipIf: func(r *http.Request) bool { return r.URL.Path == "/hello" }}
	r, buf := setupRouterWithLogger(t, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	r.ServeHTTP(w, req)
	if buf.Len() != 0 {
		t.Fatalf("expected no logs for SkipIf, got: %s", buf.String())
	}
	t.Log(buf.String())
}

func TestLogger_Flags_IP_UserAgent_Latency(t *testing.T) {
	cfg := &Config{Format: TextFormat, Flags: LOG_IP | LOG_USERAGENT | LOG_LATENCY}
	r, buf := setupRouterWithLogger(t, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	req.Header.Set("User-Agent", "logger-test")
	req.Header.Set("X-Real-Ip", "203.0.113.9")
	r.ServeHTTP(w, req)
	out := buf.String()
	t.Log(out)

	if !strings.Contains(out, "user_agent=logger-test") {
		t.Fatalf("expected user_agent in log, got: %s", out)
	}
	if !strings.Contains(out, "ip=203.0.113.9") {
		t.Fatalf("expected ip in log, got: %s", out)
	}
	if !strings.Contains(out, "latency=") {
		t.Fatalf("expected latency in log, got: %s", out)
	}
}

func TestLogger_Callback_AppendsArgs(t *testing.T) {
	cfg := &Config{Format: TextFormat, Callback: func(c *rex.Context, args ...any) []any {
		return append(args, "request_id", "abc123")
	}}

	r, buf := setupRouterWithLogger(t, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	r.ServeHTTP(w, req)
	out := buf.String()
	if !strings.Contains(out, "request_id=abc123") {
		t.Fatalf("expected request_id from callback, got: %s", out)
	}
}

func TestLogger_Callback_OddArgs_Panics(t *testing.T) {
	cfg := &Config{Format: TextFormat, Callback: func(c *rex.Context, args ...any) []any {
		// Append an odd key without value to trigger panic
		return append(args, "odd-key-only")
	}}
	r, _ := setupRouterWithLogger(t, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic for odd number of args from callback")
		}
	}()
	r.ServeHTTP(w, req)
}
