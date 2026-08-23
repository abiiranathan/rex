package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abiiranathan/rex"
	"github.com/abiiranathan/rex/middleware/auth"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
)

func newTestAuth(t *testing.T, sessionName string, config auth.CookieConfig) *auth.CookieAuth {
	t.Helper()
	if config.SkipAuth == nil {
		// Login routes must bypass authentication.
		config.SkipAuth = func(c *rex.Context) bool { return c.Path() == "/login" }
	}
	a, err := auth.NewCookieAuth(sessionName,
		[][]byte{securecookie.GenerateRandomKey(32), securecookie.GenerateRandomKey(32)},
		User{}, config)
	if err != nil {
		t.Fatalf("NewCookieAuth failed: %v", err)
	}
	return a
}

// TestEmptyKeysRejected ensures weak configuration fails fast.
func TestEmptyKeysRejected(t *testing.T) {
	if _, err := auth.NewCookieAuth("s", [][]byte{nil}, User{}, auth.CookieConfig{}); err == nil {
		t.Fatal("expected error for nil key")
	}
	if _, err := auth.NewCookieAuth("s", [][]byte{[]byte("")}, User{}, auth.CookieConfig{}); err == nil {
		t.Fatal("expected error for empty key")
	}
	if _, err := auth.NewCookieAuth("s", [][]byte{[]byte("k"), nil}, User{}, auth.CookieConfig{}); err == nil {
		t.Fatal("expected error for empty second key")
	}
}

// TestSameSiteExplicitlyHonored verifies an explicit non-default SameSite is kept.
func TestSameSiteExplicitlyHonored(t *testing.T) {
	a := newTestAuth(t, "samesite_session", auth.CookieConfig{
		Options: &sessions.Options{
			Path:     "/",
			MaxAge:   3600,
			SameSite: http.SameSiteLaxMode, // explicit override must be honored
			Secure:   false,
		},
	})

	router := rex.NewRouter()
	router.Use(a.Middleware())
	router.POST("/login", func(c *rex.Context) error {
		return a.SetState(c, User{"u", "p"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	router.ServeHTTP(w, req)

	setCookie := w.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, "SameSite=Lax") {
		t.Fatalf("expected SameSite=Lax in Set-Cookie, got %q", setCookie)
	}
	if !strings.Contains(setCookie, "HttpOnly") {
		t.Fatalf("expected HttpOnly enforced, got %q", setCookie)
	}
}

// TestDefaultSameSiteIsStrict verifies the secure default.
func TestDefaultSameSiteIsStrict(t *testing.T) {
	a := newTestAuth(t, "strict_session", auth.CookieConfig{})

	router := rex.NewRouter()
	router.Use(a.Middleware())
	router.POST("/login", func(c *rex.Context) error {
		return a.SetState(c, User{"u", "p"})
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/login", nil))

	if sc := w.Header().Get("Set-Cookie"); !strings.Contains(sc, "SameSite=Strict") {
		t.Fatalf("expected SameSite=Strict default, got %q", sc)
	}
}

// TestSessionCookieMaxAgePreserved verifies gorilla semantics where
// MaxAge=0 means a browser-session cookie that skips sliding refresh.
func TestSessionCookieMaxAgePreserved(t *testing.T) {
	a := newTestAuth(t, "session_cookie", auth.CookieConfig{
		Options: &sessions.Options{
			Path:     "/",
			MaxAge:   0, // browser-session cookie (gorilla semantics)
			Secure:   false,
			SameSite: http.SameSiteStrictMode,
		},
	})

	router := rex.NewRouter()
	router.Use(a.Middleware())
	router.POST("/login", func(c *rex.Context) error {
		return a.SetState(c, User{"u", "p"})
	})
	router.GET("/me", func(c *rex.Context) error { return c.String("ok") })

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/login", nil))
	sc := w.Header().Get("Set-Cookie")

	// A browser-session cookie carries no Max-Age attribute.
	if strings.Contains(sc, "Max-Age=") {
		t.Fatalf("expected no Max-Age attribute for browser-session cookie, got %q", sc)
	}

	// Follow-up request must not trigger sliding refresh (no Set-Cookie).
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Add("Cookie", sc)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if _, hasSetCookie := w.Header()["Set-Cookie"]; hasSetCookie {
		t.Fatal("browser-session cookies should never be refreshed with Set-Cookie")
	}
}

// TestDefaultErrorHandlerUsesErrorPipeline verifies 401s flow through rex's
// centralized error handling as *rex.Error.
func TestDefaultErrorHandlerUsesErrorPipeline(t *testing.T) {
	var captured error
	a := newTestAuth(t, "pipeline_session", auth.CookieConfig{}) // default handler

	router := rex.NewRouter()
	router.Use(a.Middleware())
	router.SetErrorHandler(&captureHandler{capture: &captured})
	router.GET("/protected", func(c *rex.Context) error { return c.String("nope") })

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/protected", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if captured == nil {
		t.Fatal("expected error through pipeline")
	}
	rexErr, ok := captured.(*rex.Error)
	if !ok || rexErr.Code != http.StatusUnauthorized {
		t.Fatalf("expected *rex.Error 401, got %T", captured)
	}
}

type captureHandler struct {
	capture *error
}

func (h *captureHandler) Handle(c *rex.Context, err error) {
	*h.capture = err
	c.WriteHeader(http.StatusUnauthorized)
}

// TestSetStateOverwritesTamperedCookie ensures login succeeds even when the
// client presents an undecodable session cookie.
func TestSetStateOverwritesTamperedCookie(t *testing.T) {
	a := newTestAuth(t, "tamper_session", auth.CookieConfig{})

	router := rex.NewRouter()
	router.Use(a.Middleware())
	router.GET("/me", func(c *rex.Context) error {
		v := a.Value(c)
		u, _ := v.(User)
		return c.String(u.Username)
	})
	router.POST("/login", func(c *rex.Context) error {
		return a.SetState(c, User{"fresh", "p"})
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/login", nil))
	validCookie := w.Header().Get("Set-Cookie")

	// Tamper with the signed value: flip one byte mid-value (stays valid
	// base64 so the header parses, but breaks the HMAC).
	parts := strings.SplitN(validCookie, "=", 2)
	val := []byte(parts[1])
	mid := len(val) / 2
	if val[mid] == 'A' {
		val[mid] = 'B'
	} else {
		val[mid] = 'A'
	}
	tamperedCookie := parts[0] + "=" + string(val)

	// Login again while presenting the tampered cookie must succeed.
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.Header.Add("Cookie", tamperedCookie)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected login to overwrite tampered session, got %d: %s", w.Code, w.Body.String())
	}

	// The response carries both an expire cookie (for the rejected session)
	// and the fresh session cookie; pick the real one.
	var fresh string
	for _, sc := range w.Header()["Set-Cookie"] {
		if !strings.Contains(sc, "Expires=Thu, 01 Jan 1970") {
			fresh = sc
		}
	}
	if fresh == "" {
		t.Fatal("expected fresh Set-Cookie after re-login")
	}

	// The fresh cookie authenticates.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Add("Cookie", fresh)
	router.ServeHTTP(w, req)

	if w.Body.String() != "fresh" {
		t.Fatalf("expected fresh user, got %q", w.Body.String())
	}
}

// TestStateUpdateReusesCachedSession verifies SetState works on an
// already-authenticated route (exercising the per-request session cache).
func TestStateUpdateReusesCachedSession(t *testing.T) {
	a := newTestAuth(t, "update_session", auth.CookieConfig{})

	router := rex.NewRouter()
	router.Use(a.Middleware())
	router.POST("/login", func(c *rex.Context) error {
		return a.SetState(c, User{"first", "p"})
	})
	router.POST("/update", func(c *rex.Context) error {
		// Middleware already decoded the session for this request; SetState
		// reuses the cached copy instead of decrypting the cookie again.
		return a.SetState(c, User{"second", "p"})
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/login", nil))
	cookie := w.Header().Get("Set-Cookie")

	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/update", nil)
	req.Header.Add("Cookie", cookie)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on update, got %d: %s", w.Code, w.Body.String())
	}

	updated := w.Header().Get("Set-Cookie")
	if updated == "" {
		t.Fatal("expected refreshed Set-Cookie after state update")
	}

	// New state must be visible.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/update", nil)
	req.Header.Add("Cookie", updated)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("updated cookie rejected: got %d", w.Code)
	}
}

// BenchmarkAuthMiddleware measures an authenticated request through the
// middleware (dominated by securecookie HMAC/AES verification).
func BenchmarkAuthMiddleware(b *testing.B) {
	a := newTestAuth(&testing.T{}, "bench_session", auth.CookieConfig{})
	router := rex.NewRouter()
	router.Use(a.Middleware())
	router.POST("/login", func(c *rex.Context) error { return a.SetState(c, User{"u", "p"}) })
	router.GET("/protected", func(c *rex.Context) error { return c.String("ok") })

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/login", nil))
	cookie := w.Header().Get("Set-Cookie")

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Add("Cookie", cookie)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}
