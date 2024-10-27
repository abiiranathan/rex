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

type user struct {
	username string
	password string
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

	router := rex.NewRouter()
	router.Use(auth.Cookie(errorCallback, skipAuth, secretKey, blockKey))

	router.POST("/login", func(c *rex.Context) error {
		username := c.FormValue("username")
		password := c.FormValue("password")

		// validate user credentials here

		// Set auth state
		u := user{username, password}
		err := auth.SetAuthState(c.Request, c.Response, u)
		if err != nil {
			return err
		}
		return c.JSON(u)
	})

	router.GET("/", func(c *rex.Context) error {
		state, authenticated := auth.GetAuthState(c.Request, c.Response)
		if !authenticated {
			t.Fatal("user is not authenticated")
		}
		return c.String("Welcome home: %s", state.(user).username)
	})

	router.POST("/logout", func(c *rex.Context) error {
		auth.ClearAuthState(c.Request, c.Response)
		return c.Redirect("/")
	})

	form := url.Values{
		"username": {"abiiranathan"},
		"password": {"sepersecurepassword"},
	}
	body := form.Encode()

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Result().StatusCode)
	}

	// hdr := w.Header()
	// cookies, ok := hdr["Set-Cookie"]
	// if !ok {
	// 	t.Fatalf("Set-Cookie header missing in response")
	// }

	// // Perform authenticated request
	// req = httptest.NewRequest(http.MethodGet, "/", nil)
	// req.Header.Add("Cookie", cookies[0])
	// w = httptest.NewRecorder()

	// router.ServeHTTP(w, req)

	// if w.Result().StatusCode != http.StatusOK {
	// 	t.Errorf("expected status code %d, got %d", http.StatusOK, w.Result().StatusCode)
	// }

	// expected := "Welcome abiiranathan"
	// if expected != w.Body.String() {
	// 	t.Fatalf("expected %q, got %s\n", expected, w.Body.String())
	// }

}
