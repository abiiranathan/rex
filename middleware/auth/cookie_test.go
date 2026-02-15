package auth_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/abiiranathan/rex"
	"github.com/abiiranathan/rex/middleware/auth"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
)

type User struct {
	Username string
	Password string
}

func errorCallback(c *rex.Context) error {
	c.WriteHeader(http.StatusUnauthorized)
	return nil
}

func skipAuth(c *rex.Context) bool {
	return c.Path() == "/login"
}

func TestCookieMiddleware(t *testing.T) {
	secretKey := securecookie.GenerateRandomKey(32)
	encryptionKey := securecookie.GenerateRandomKey(32)

	auth.InitializeCookieStore([][]byte{secretKey, encryptionKey}, User{})

	router := rex.NewRouter()
	router.Use(auth.Cookie("rex_session_name", auth.CookieConfig{
		Options: &sessions.Options{
			MaxAge:   int((24 * time.Hour).Seconds()),
			Secure:   false,
			SameSite: http.SameSiteStrictMode,
		},
		ErrorHandler: errorCallback,
		SkipAuth:     skipAuth,
	}))

	router.POST("/login", func(c *rex.Context) error {
		contentType := c.ContentType()
		if contentType != "application/x-www-form-urlencoded" && contentType != "multipart/form-data" {
			c.WriteHeader(http.StatusBadRequest)
			return nil
		}

		username := c.FormValue("username")
		password := c.FormValue("password")

		if username == "" || password == "" {
			c.WriteHeader(http.StatusBadRequest)
			return nil
		}

		// validate user credentials here

		// Set auth state
		u := User{username, password}
		err := auth.SetAuthState(c, u)
		if err != nil {
			return err
		}
		return c.HTML("Login successful")
	})

	router.GET("/", func(c *rex.Context) error {
		state := auth.CookieValue(c)
		if state == nil {
			t.Fatal("user is not authenticated")
		}

		res := fmt.Sprintf("Welcome home: %s", state.(User).Username)
		return c.String(res)
	})

	router.POST("/logout", func(c *rex.Context) error {
		auth.ClearAuthState(c)
		return c.String("Logout successful")
	})

	form := url.Values{
		"username": {"abiiranathan"},
		"password": {"supersecurepassword"},
	}
	body := form.Encode()

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected status code %d, got %d, body: %s\n", http.StatusOK, w.Result().StatusCode, w.Body.String())
	}

	hdr := w.Header()
	cookies, ok := hdr["Set-Cookie"]
	if !ok || len(cookies) == 0 {
		t.Fatalf("Set-Cookie header missing in response")
	}

	// Perform authenticated request
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("Cookie", cookies[0])
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Result().StatusCode)
	}

	expected := "Welcome home: abiiranathan"
	if expected != w.Body.String() {
		t.Fatalf("expected %q, got %s\n", expected, w.Body.String())
	}

	// Perform logout
	req = httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.Header.Add("Cookie", cookies[0])
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Result().StatusCode)
	}

	// Perform unauthenticated request
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status code %d, got %d", http.StatusUnauthorized, w.Result().StatusCode)
	}

}

func TestCookieSlidingWindowRefresh(t *testing.T) {
	secretKey := securecookie.GenerateRandomKey(32)
	encryptionKey := securecookie.GenerateRandomKey(32)

	auth.InitializeCookieStore([][]byte{secretKey, encryptionKey}, User{})

	// Use a short MaxAge so we can reason about the threshold easily.
	// refreshThreshold = maxAge / 2 = 4s
	maxAge := 8

	router := rex.NewRouter()
	router.Use(auth.Cookie("rex_session_name", auth.CookieConfig{
		Options: &sessions.Options{
			MaxAge:   maxAge,
			Secure:   false,
			SameSite: http.SameSiteStrictMode,
		},
		ErrorHandler: errorCallback,
		SkipAuth:     skipAuth,
	}))

	router.POST("/login", func(c *rex.Context) error {
		err := auth.SetAuthState(c, User{"testuser", "testpass"})
		if err != nil {
			return err
		}
		return c.String("ok")
	})

	router.GET("/protected", func(c *rex.Context) error {
		return c.String("ok")
	})

	// Helper: perform a request and return the cookies from the response.
	doRequest := func(method, path string, reqCookies []string) *httptest.ResponseRecorder {
		var req *http.Request
		if method == http.MethodPost && path == "/login" {
			form := url.Values{"username": {"testuser"}, "password": {"testpass"}}
			req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		for _, c := range reqCookies {
			req.Header.Add("Cookie", c)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// Login and grab the initial cookie.
	w := doRequest(http.MethodPost, "/login", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d", w.Code)
	}
	cookies := w.Header()["Set-Cookie"]
	if len(cookies) == 0 {
		t.Fatal("no Set-Cookie header after login")
	}
	firstCookie := cookies[0]

	// --- Invariant 1: No refresh before the threshold is crossed ---
	// An immediate follow-up request should NOT produce a new Set-Cookie,
	// because less than half of MaxAge has elapsed since the last save.
	w = doRequest(http.MethodGet, "/protected", []string{firstCookie})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if _, refreshed := w.Header()["Set-Cookie"]; refreshed {
		t.Error("cookie was refreshed too early: no Set-Cookie should be issued before the threshold")
	}

	// --- Invariant 2: Cookie IS refreshed once the threshold has elapsed ---
	// Sleep past the refresh threshold (maxAge/2 seconds).
	time.Sleep(time.Duration(maxAge/2+1) * time.Second)

	w = doRequest(http.MethodGet, "/protected", []string{firstCookie})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 after threshold sleep, got %d", w.Code)
	}
	refreshedCookies := w.Header()["Set-Cookie"]
	if len(refreshedCookies) == 0 {
		t.Fatal("cookie was NOT refreshed after the threshold elapsed: expected a new Set-Cookie header")
	}
	refreshedCookie := refreshedCookies[0]

	if refreshedCookie == firstCookie {
		t.Error("refreshed cookie is identical to original: session was not actually extended")
	}

	// --- Invariant 3: The refreshed cookie is still valid ---
	w = doRequest(http.MethodGet, "/protected", []string{refreshedCookie})
	if w.Code != http.StatusOK {
		t.Fatalf("refreshed cookie rejected: expected 200, got %d", w.Code)
	}

	// --- Invariant 4: Original cookie remains valid until MaxAge is truly exhausted ---
	// (It hasn't expired yet — only half the window elapsed so far.)
	// This confirms the refresh extends the session rather than invalidating the old one.
	w = doRequest(http.MethodGet, "/protected", []string{firstCookie})
	if w.Code != http.StatusOK {
		t.Fatalf("original cookie should still be valid before full MaxAge: got %d", w.Code)
	}

	// --- Invariant 5: Session expires if never refreshed ---
	// Sleep until the original MaxAge is fully exhausted.
	time.Sleep(time.Duration(maxAge/2+1) * time.Second)

	w = doRequest(http.MethodGet, "/protected", []string{firstCookie})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expired original cookie should be rejected: expected 401, got %d", w.Code)
	}

	// But the refreshed cookie should still work (its MaxAge was reset).
	w = doRequest(http.MethodGet, "/protected", []string{refreshedCookie})
	if w.Code != http.StatusOK {
		t.Errorf("refreshed cookie should still be valid: expected 200, got %d", w.Code)
	}
}
