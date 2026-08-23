package logger

import (
	"bytes"
	"strings"
	"testing"

	"github.com/abiiranathan/rex"
	"net/http"
	"net/http/httptest"
)

// TestAsyncLoggerDeliversAllRequests verifies that with Async enabled, every
// request is logged exactly once and Close flushes the queue.
func TestAsyncLoggerDeliversAllRequests(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	conf := &Config{
		Output:    &buf,
		Format:    TextFormat,
		Flags:     StdLogFlags,
		Async:     true,
		QueueSize: 256,
	}

	r := rex.NewRouter()
	r.Use(New(conf))
	r.GET("/hello", func(c *rex.Context) error { return c.String("ok") })

	const n = 100
	for range n {
		req := httptest.NewRequest(http.MethodGet, "/hello", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}

	// Close blocks until every queued record has been written.
	conf.Close()

	if got := strings.Count(buf.String(), "path=/hello"); got != n {
		t.Fatalf("expected %d log lines, got %d", n, got)
	}
	if conf.Dropped() != 0 {
		t.Fatalf("expected 0 dropped, got %d", conf.Dropped())
	}
}

// TestSyncModeStillDefault ensures Async defaults to false.
func TestSyncModeStillDefault(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	conf := &Config{Output: &buf, Format: JSONFormat, Flags: StdLogFlags}

	r := rex.NewRouter()
	r.Use(New(conf))
	r.GET("/hello", func(c *rex.Context) error { return c.String("ok") })

	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	if !strings.Contains(buf.String(), `"status":200`) {
		t.Fatalf("expected synchronous log line, got: %s", buf.String())
	}
	if conf.asyncHandler != nil {
		t.Fatal("async handler should not be created when Async is false")
	}
}

// TestLatencyLoggedAsDuration verifies the slog.Duration latency value.
func TestLatencyLoggedAsDuration(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	conf := &Config{Output: &buf, Format: TextFormat, Flags: LogLatency | LogIP | LogUserAgent}

	r := rex.NewRouter(rex.WithTrustProxy("192.0.2.1"))
	r.Use(New(conf))
	r.GET("/hello", func(c *rex.Context) error { return c.String("ok") })

	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	req.Header.Set("X-Real-Ip", "203.0.113.9")
	r.ServeHTTP(httptest.NewRecorder(), req)

	out := buf.String()
	for _, want := range []string{"latency=", "status=200", "ip=203.0.113.9"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in log, got: %s", want, out)
		}
	}
}
