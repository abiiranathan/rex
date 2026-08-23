package rex_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/abiiranathan/rex"
	"github.com/stretchr/testify/require"
)

func TestWithStrictHome(t *testing.T) {
	r := rex.NewRouter(rex.WithStrictHome(false))
	r.GET("/", func(c *rex.Context) error { return c.String("home") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/subpath", nil)
	r.ServeHTTP(w, req)

	// Without strict home, "/" acts as a prefix match per http.ServeMux.
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestWithNoTrailingSlashDisabled(t *testing.T) {
	r := rex.NewRouter(rex.WithNoTrailingSlash(false))
	r.GET("/path/", func(c *rex.Context) error { return c.String("ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/path/", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestParamUintRejectsNegative(t *testing.T) {
	r := rex.NewRouter()
	r.GET("/items/{id}", func(c *rex.Context) error {
		return c.JSON(rex.Map{"id": c.ParamUint("id", 42)})
	})

	// A negative param must yield the default, not a wrapped positive value.
	req := httptest.NewRequest(http.MethodGet, "/items/-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Contains(t, w.Body.String(), `"id":42`)

	req = httptest.NewRequest(http.MethodGet, "/items/7", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Contains(t, w.Body.String(), `"id":7`)
}

func TestQueryUIntRejectsNegative(t *testing.T) {
	r := rex.NewRouter()
	r.GET("/search", func(c *rex.Context) error {
		return c.JSON(rex.Map{"page": c.QueryUInt("page", 1)})
	})

	req := httptest.NewRequest(http.MethodGet, "/search?page=-5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Contains(t, w.Body.String(), `"page":1`)
}

func TestIPUntrustedProxyHeadersIgnored(t *testing.T) {
	r := rex.NewRouter()
	r.GET("/ip", func(c *rex.Context) error {
		ip, err := c.IP()
		require.NoError(t, err)
		return c.String(ip)
	})

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	// Spoofed headers from an untrusted peer must be ignored.
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	req.Header.Set("X-Real-Ip", "203.0.113.9")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, "192.0.2.1", w.Body.String())
}

func TestIPTrustedProxyHeadersHonored(t *testing.T) {
	r := rex.NewRouter(rex.WithTrustProxy("192.0.2.0/24"))
	r.GET("/ip", func(c *rex.Context) error {
		ip, err := c.IP()
		require.NoError(t, err)
		return c.String(ip)
	})

	t.Run("xff rightmost untrusted wins", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ip", nil)
		req.RemoteAddr = "192.0.2.5:1234"
		req.Header.Set("X-Forwarded-For", "203.0.113.9, 198.51.100.7")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, "198.51.100.7", w.Body.String())
	})

	t.Run("spoofed xff from untrusted client is ignored", func(t *testing.T) {
		// Client 198.51.100.7 (untrusted) tries to spoof via XFF; the trusted
		// proxy appends the real client IP at the end.
		req := httptest.NewRequest(http.MethodGet, "/ip", nil)
		req.RemoteAddr = "192.0.2.5:1234"
		req.Header.Set("X-Forwarded-For", "8.8.8.8, 198.51.100.7")
		// All XFF entries here: walking right to left, 198.51.100.7 is
		// untrusted so it wins over the spoofed 8.8.8.8.
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, "198.51.100.7", w.Body.String())
	})

	t.Run("all xff entries trusted falls back to x-real-ip", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ip", nil)
		req.RemoteAddr = "192.0.2.5:1234"
		req.Header.Set("X-Forwarded-For", "192.0.2.6, 192.0.2.7")
		req.Header.Set("X-Real-Ip", "203.0.113.9")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, "203.0.113.9", w.Body.String())
	})
}

func TestIPTrustedBareIP(t *testing.T) {
	// Bare IPs should be accepted by WithTrustProxy without CIDR notation.
	r := rex.NewRouter(rex.WithTrustProxy("192.0.2.1", "::1"))
	r.GET("/ip", func(c *rex.Context) error {
		ip, _ := c.IP()
		return c.String(ip)
	})

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "192.0.2.1:9999"
	req.Header.Set("X-Real-Ip", "203.0.113.9")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, "203.0.113.9", w.Body.String())
}

func TestWithTrustProxyPanicsOnInvalidCIDR(t *testing.T) {
	require.Panics(t, func() {
		rex.NewRouter(rex.WithTrustProxy("not-a-cidr"))
	})
}

func TestServeMinifiedOption(t *testing.T) {
	dirname := t.TempDir()

	write := func(name, content string) {
		require.NoError(t, os.WriteFile(dirname+"/"+name, []byte(content), 0644))
	}
	write("app.js", "original")
	write("app.min.js", "minified")
	write("style.css", "original-css")

	r := rex.NewRouter(
		rex.WithServeMinified(true),
		rex.WithMinExtensions([]string{".js"}),
	)
	r.StaticFS("/static/", http.Dir(dirname))

	// Minified variant is served when present...
	req := httptest.NewRequest(http.MethodGet, "/static/app.js", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "minified", w.Body.String())

	// ...and the original is served when no minified variant exists.
	req = httptest.NewRequest(http.MethodGet, "/static/style.css", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, "original-css", w.Body.String())

	// Disabled by default.
	r2 := rex.NewRouter()
	r2.StaticFS("/static/", http.Dir(dirname))
	req = httptest.NewRequest(http.MethodGet, "/static/app.js", nil)
	w = httptest.NewRecorder()
	r2.ServeHTTP(w, req)
	require.Equal(t, "original", w.Body.String())
}
