package rex_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/abiiranathan/rex"
	"github.com/abiiranathan/rex/middleware/auth"
	"github.com/abiiranathan/rex/middleware/brotli"
	"github.com/abiiranathan/rex/middleware/etag"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
)

// User struct for auth
type ChainUser struct {
	Username string
}

func TestChainedMiddleware(t *testing.T) {
	// Initialize Auth Store
	secretKey := securecookie.GenerateRandomKey(32)
	auth.InitializeCookieStore([][]byte{secretKey}, ChainUser{})

	r := rex.NewRouter()

	// Apply Middlewares in order: Auth -> Brotli -> ETag
	// Flow:
	// Request -> Brotli (sets up writer) -> ETag (sets up writer) -> Auth -> Handler
	// Response: Handler writes -> ETag buffers -> ETag writes to Brotli -> Brotli compresses -> Response

	r.Use(brotli.Brotli())
	r.Use(etag.New())

	// Auth config
	authConfig := auth.CookieConfig{
		Options: &sessions.Options{
			Path:     "/",
			MaxAge:   3600,
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		},
		ErrorHandler: func(c *rex.Context) error {
			c.WriteHeader(http.StatusUnauthorized)
			return c.String("Unauthorized")
		},
		SkipAuth: func(c *rex.Context) bool {
			return c.Path() == "/login"
		},
	}
	r.Use(auth.Cookie("rex_chain_session", authConfig))

	// Routes
	r.POST("/login", func(c *rex.Context) error {
		u := ChainUser{Username: "testuser"}
		return auth.SetAuthState(c, u)
	})

	r.GET("/data", func(c *rex.Context) error {
		// Verify we are authenticated
		user := auth.CookieValue(c)
		if user == nil {
			return fmt.Errorf("not authenticated")
		}
		return c.String("some secret data needing compression and etag")
	})

	// 1. Login
	w := httptest.NewRecorder()
	form := url.Values{}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Login failed: %d", w.Code)
	}

	cookie := w.Result().Cookies()[0]

	// 2. Get Data (First Request)
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/data", nil)
	req.Header.Add("Cookie", cookie.String())
	req.Header.Set("Accept-Encoding", "br") // Request Brotli
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Get data failed: %d %s", w.Code, w.Body.String())
	}

	// Check Brotli
	if w.Header().Get("Content-Encoding") != "br" {
		t.Errorf("Expected Content-Encoding: br, got %s", w.Header().Get("Content-Encoding"))
	}

	// Check ETag
	etagVal := w.Header().Get("ETag")
	if etagVal == "" {
		t.Error("Expected ETag header")
	}

	// 3. Get Data (Second Request with ETag)
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/data", nil)
	req2.Header.Add("Cookie", cookie.String())
	req2.Header.Set("Accept-Encoding", "br")
	req2.Header.Set("If-None-Match", etagVal)
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusNotModified {
		t.Errorf("Expected 304 Not Modified, got %d", w2.Code)
	}

	if w2.Body.Len() > 0 {
		t.Errorf("Expected empty body for 304, got %d bytes", w2.Body.Len())
	}
}
