package rex

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
)

// TestInitContextZeroesFields exercises InitContext directly on a dirty,
// recycled context to prove no state leaks between requests.
func TestInitContextZeroesFields(t *testing.T) {
	t.Parallel()
	r := NewRouter()

	c := r.InitContext(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	c.err = errors.New("old")
	c.latency = 42 * time.Second
	c.contentTypeSet = true
	c.Set("key", "value")
	r.PutContext(c)

	c2 := r.InitContext(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if c2.err != nil {
		t.Error("err not cleared")
	}
	if c2.latency != 0 {
		t.Error("latency not cleared")
	}
	if c2.contentTypeSet {
		t.Error("contentTypeSet not cleared")
	}
	if _, exists := c2.Get("key"); exists {
		t.Error("locals not cleared")
	}
}

// TestPooledRequestsNoStaleError runs failing and succeeding requests through
// the pool repeatedly; a stale c.err or contentTypeSet would surface as a
// wrong status here.
func TestPooledRequestsNoStaleError(t *testing.T) {
	t.Parallel()
	r := NewRouter()
	r.GET("/fail", func(c *Context) error { return errors.New("boom") })
	r.GET("/ok", func(c *Context) error {
		c.SetContentType("text/custom")
		return c.String("ok")
	})

	// Generate failures first so pool slots carry dirty state.
	for range 3 {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/fail", nil))
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 for /fail, got %d", w.Code)
		}
	}

	for range 5 {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ok", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("stale state detected: expected 200 for /ok, got %d", w.Code)
		}
		if ct := w.Header().Get("Content-Type"); ct != "text/custom" {
			t.Fatalf("unexpected content type %q", ct)
		}
	}
}

// TestRedirectRouteAppliesStatusAfterTarget verifies redirect semantics
// without superfluous WriteHeader calls.
func TestRedirectRouteAppliesStatusAfterTarget(t *testing.T) {
	t.Parallel()
	r := NewRouter()
	r.GET("/target", func(c *Context) error {
		// Writes a body without sending a status explicitly.
		return c.String("redirected body")
	})
	r.GET("/start", func(c *Context) error {
		return c.RedirectRoute("/target")
	})
	r.GET("/target-explicit", func(c *Context) error {
		c.WriteHeader(http.StatusAccepted) // handler's own status must win
		return c.String("accepted")
	})
	r.GET("/start-explicit", func(c *Context) error {
		return c.RedirectRoute("/target-explicit")
	})

	// Body-writing target still receives the redirect status (303).
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/start", nil))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if w.Body.String() != "redirected body" {
		t.Fatalf("expected target body, got %q", w.Body.String())
	}

	// Target's explicit status takes precedence.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/start-explicit", nil))
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
}

// TestTranslateErrorsFallbackWithoutRouter ensures no panic when the context
// has no router/translator.
func TestTranslateErrorsFallbackWithoutRouter(t *testing.T) {
	t.Parallel()
	c := NewContext(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil), nil)

	out := c.TranslateErrors(validator.ValidationErrors{})
	if out == nil || len(out) != 0 {
		t.Fatalf("expected empty map, got %v", out)
	}
}

// TestJSONSetsContentLength verifies buffered JSON responses carry Content-Length.
func TestJSONSetsContentLength(t *testing.T) {
	t.Parallel()
	r := NewRouter()
	r.GET("/json", func(c *Context) error {
		return c.JSON(Map{"hello": "world"})
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/json", nil))

	body := w.Body.String()
	if cl := w.Header().Get("Content-Length"); cl != strconv.Itoa(len(body)) {
		t.Fatalf("expected Content-Length %d, got %q", len(body), cl)
	}
	if ct := w.Header().Get("Content-Type"); ct != ContentTypeJSON {
		t.Fatalf("unexpected content type %q", ct)
	}
	if strings.TrimSpace(body) != `{"hello":"world"}` {
		t.Fatalf("unexpected body %q", body)
	}
}

// TestSetContentTypePrecedence verifies response helpers honor a previously
// set content type within the same request.
func TestSetContentTypePrecedence(t *testing.T) {
	t.Parallel()
	r := NewRouter()
	r.GET("/ct", func(c *Context) error {
		c.SetContentType("application/problem+json")
		return c.JSON(Map{"ok": true}) // must not override the custom type
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ct", nil))
	if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("expected custom content type preserved, got %q", ct)
	}
}

// TestWithTimezone verifies the router timezone applies to form parsing.
func TestWithTimezone(t *testing.T) {
	t.Parallel()
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("tzdata unavailable")
	}

	type form struct {
		When time.Time `form:"when"`
	}

	r := NewRouter(WithTimezone(ny))
	r.POST("/tz", func(c *Context) error {
		var f form
		if err := c.BodyParser(&f); err != nil {
			return err
		}
		_, offset := f.When.In(ny).Zone()
		if offset != -4*3600 { // EDT (UTC-4) in June
			t.Fatalf("expected EDT offset, got %d", offset)
		}
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/tz", strings.NewReader("when=2024-06-01T00:00:00"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestNewContextTracksStatus verifies NewContext wraps the writer so
// StatusCode works outside routing.
func TestNewContextTracksStatus(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c := NewContext(w, httptest.NewRequest("GET", "/", nil), nil)

	c.WriteHeader(http.StatusTeapot)
	if c.StatusCode() != http.StatusTeapot {
		t.Fatalf("expected 418, got %d", c.StatusCode())
	}
	if w.Code != http.StatusTeapot {
		t.Fatalf("recorder expected 418, got %d", w.Code)
	}
}
