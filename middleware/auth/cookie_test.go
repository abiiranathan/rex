package auth_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/abiiranathan/rex"
	"github.com/abiiranathan/rex/middleware/auth"
	"github.com/gorilla/securecookie"
)

type User struct {
	Username string
	Password string
}

func errorCallback(c *rex.Context) error {
	return c.WriteHeader(http.StatusUnauthorized)
}

func skipAuth(req *http.Request) bool {
	return req.URL.Path == "/login"
}

func TestCookieMiddleware(t *testing.T) {
	secretKey := securecookie.GenerateRandomKey(32)
	blockKey := securecookie.GenerateRandomKey(32)

	auth.Register(User{})

	router := rex.NewRouter()
	router.Use(auth.Cookie(errorCallback, skipAuth, secretKey, blockKey))

	router.POST("/login", func(c *rex.Context) error {
		contentType := c.ContentType()
		if contentType != "application/x-www-form-urlencoded" && contentType != "multipart/form-data" {
			return c.WriteHeader(http.StatusBadRequest)
		}

		username := c.FormValue("username")
		password := c.FormValue("password")

		if username == "" || password == "" {
			return c.WriteHeader(http.StatusBadRequest)
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
		state, authenticated := auth.GetAuthState(c)
		if !authenticated {
			t.Fatal("user is not authenticated")
		}
		return c.String("Welcome home: %s", state.(User).Username)
	})

	router.POST("/logout", func(c *rex.Context) error {
		auth.ClearAuthState(c)
		return c.Redirect("/")
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

}
