package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abiiranathan/rex"
	"github.com/abiiranathan/rex/middleware/auth"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
)

// Test the condition where a cookie signed with an old key is rejected after key rotation.
// Instead of rejecting, we need to expire the cookie on the client side so the login flow can be re-initiated.
func TestCookieRotation(t *testing.T) {
	// 1. Initialize with Key A
	keyA := securecookie.GenerateRandomKey(32)
	auth.InitializeCookieStore([][]byte{keyA}, User{})

	r := rex.NewRouter()
	sessionName := "rotation_test_session"

	config := auth.CookieConfig{
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
	r.Use(auth.Cookie(sessionName, config))

	r.GET("/protected", func(c *rex.Context) error {
		return c.String("Protected Content")
	})

	// 2. Obtain a valid cookie signed with Key A
	w := httptest.NewRecorder()

	// Manually creating a session to get the cookie string without a full login handler
	// Note: We can't easily access the store to create a cookie without a request context
	// So we'll use a login route helper.
	r.POST("/login", func(c *rex.Context) error {
		return auth.SetAuthState(c, User{Username: "test"})
	})

	req := httptest.NewRequest("POST", "/login", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Login failed: %d", w.Code)
	}

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("No cookies returned")
	}
	validCookie := cookies[0]

	// 3. Rotate Keys: Initialize with Key B only
	keyB := securecookie.GenerateRandomKey(32)
	auth.InitializeCookieStore([][]byte{keyB}, User{})

	// 4. Request /protected with Key A signed cookie
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/protected", nil)
	req.Header.Add("Cookie", validCookie.String())
	r.ServeHTTP(w, req)

	// 5. Expect 401 Unauthorized
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized after rotation, got %d", w.Code)
	}

	// 6. Expect Set-Cookie header to expire the cookie
	foundExpiration := false
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionName && c.MaxAge < 0 {
			foundExpiration = true
			break
		}
	}

	if !foundExpiration {
		t.Error("Expected Set-Cookie header to expire the invalid session cookie")
	}
}
