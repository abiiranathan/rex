package test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/abiiranathan/rex"
)

type user struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func newTestRouter(t *testing.T) *rex.Router {
	t.Helper()
	r := rex.NewRouter()

	r.GET("/users/{id}", func(c *rex.Context) error {
		return c.JSON(user{ID: c.ParamInt("id"), Name: "ada"})
	})

	r.GET("/search", func(c *rex.Context) error {
		return c.String("page=" + c.Query("page", "0") + ";q=" + c.Query("q", ""))
	})

	r.POST("/users", func(c *rex.Context) error {
		var u user
		if err := c.BodyParser(&u); err != nil {
			return err
		}
		c.WriteHeader(http.StatusCreated)
		return c.JSON(u)
	})

	r.PUT("/users/{id}", func(c *rex.Context) error {
		var u user
		if err := c.BodyParser(&u); err != nil {
			return err
		}
		u.ID = c.ParamInt("id")
		return c.JSON(u)
	})

	r.POST("/upload", func(c *rex.Context) error {
		f, fh, err := c.FormFile("file")
		if err != nil {
			return err
		}
		defer f.Close()
		note := c.FormValue("note")
		return c.String("file:" + fh.Filename + ";note:" + note)
	})

	r.GET("/authz", func(c *rex.Context) error {
		if c.GetHeader("Authorization") != "Bearer tok" {
			return rex.NewError(http.StatusUnauthorized, "no token")
		}
		return c.String("authorized")
	})

	r.GET("/cookie-echo", func(c *rex.Context) error {
		ck, err := c.Request.Cookie("session")
		if err != nil {
			return rex.NewError(http.StatusBadRequest, "missing cookie")
		}
		return c.String(ck.Value)
	})

	r.GET("/set-cookie", func(c *rex.Context) error {
		http.SetCookie(c.Response, &http.Cookie{Name: "sid", Value: "abc123"})
		return c.String("ok")
	})

	r.POST("/form-login", func(c *rex.Context) error {
		user := c.FormValue("username")
		pass := c.FormValue("password")
		if user == "" || pass == "" {
			return rex.NewError(http.StatusBadRequest, "missing fields")
		}
		return c.String("welcome " + user)
	})

	return r
}

func TestClientGetJSON(t *testing.T) {
	t.Parallel()
	c := NewClient(t, newTestRouter(t))

	var u user
	resp := c.Get("/users/42").Send()
	resp.AssertStatus(200).AssertJSON(&u).AssertHeader("Content-Type", "application/json")

	if u.ID != 42 || u.Name != "ada" {
		t.Fatalf("unexpected user: %+v", u)
	}
}

func TestClientQueryParams(t *testing.T) {
	t.Parallel()
	c := NewClient(t, newTestRouter(t))

	c.Get("/search").Query("page", "3").Query("q", "golang").
		Send().
		AssertStatus(200).
		AssertBodyContains("page=3;q=golang")
}

func TestClientPostJSON(t *testing.T) {
	t.Parallel()
	c := NewClient(t, newTestRouter(t))

	var created user
	c.PostJSON("/users", user{Name: "grace"}).
		AssertStatus(201).
		AssertJSON(&created)

	if created.Name != "grace" || created.ID != 0 {
		t.Fatalf("unexpected echo: %+v", created)
	}
}

func TestClientPutJSON(t *testing.T) {
	t.Parallel()
	c := NewClient(t, newTestRouter(t))

	var updated user
	c.Put("/users/7").JSON(map[string]string{"name": "linus"}).
		Send().
		AssertStatus(200).
		AssertJSON(&updated)

	if updated.ID != 7 || updated.Name != "linus" {
		t.Fatalf("unexpected update: %+v", updated)
	}
}

func TestClientFormPost(t *testing.T) {
	t.Parallel()
	c := NewClient(t, newTestRouter(t))

	c.Post("/form-login").Form(url.Values{
		"username": {"ken"},
		"password": {"s3cret"},
	}).Send().AssertStatus(200).AssertBodyContains("welcome ken")
}

func TestClientMultipartUpload(t *testing.T) {
	t.Parallel()
	c := NewClient(t, newTestRouter(t))

	c.Post("/upload").
		File("file", "notes.txt", []byte("hello upload")).
		FormField("note", "attached").
		Send().
		AssertStatus(200).
		AssertBodyContains("file:notes.txt;note:attached")
}

func TestClientHeadersAndCookies(t *testing.T) {
	t.Parallel()
	r := newTestRouter(t)
	c := NewClient(t, r)

	// Per-request header.
	c.Get("/authz").Header("Authorization", "Bearer tok").
		Send().AssertStatus(200).AssertBodyContains("authorized")

	// Client default header.
	authed := NewClient(t, r).Header("Authorization", "Bearer tok")
	authed.Get("/authz").Send().AssertStatus(200)

	// Cookie on request.
	c.Get("/cookie-echo").Cookie(&http.Cookie{Name: "session", Value: "xyz"}).
		Send().AssertBodyContains("xyz")

	// Cookies from response.
	resp := c.Get("/set-cookie").Send().AssertStatus(200)
	if got := resp.Cookie("sid"); got == nil || got.Value != "abc123" {
		t.Fatalf("expected sid cookie, got %v", got)
	}
}

func TestClientPathValueForIsolatedHandlers(t *testing.T) {
	t.Parallel()
	// Handler tested in isolation, without route registration.
	h := func(c *rex.Context) error { return c.String("id=" + c.Param("id")) }

	w := httptest.NewRecorder()
	req := NewRequest("GET", "/", nil)
	SetPathValue(req, "id", "99")

	ctx := rex.NewContext(w, req, nil)
	if err := h(ctx); err != nil {
		t.Fatal(err)
	}
	if w.Body.String() != "id=99" {
		t.Fatalf("got %q", w.Body.String())
	}
}

// failingTB captures failures instead of aborting the test binary.
type failingTB struct {
	*testing.T
	failures []string
}

func (f *failingTB) Errorf(fm string, a ...any) {
	f.failures = append(f.failures, fmt.Sprintf(fm, a...))
}

func (f *failingTB) Fatalf(fm string, a ...any) { f.Errorf(fm, a...) }

func TestClientSelfFailingAssertions(t *testing.T) {
	t.Parallel()

	tb := &failingTB{T: t}
	c := NewClient(tb, newTestRouter(t))

	resp := c.Get("/users/1").Send().AssertStatus(500)

	if len(tb.failures) == 0 {
		t.Fatal("expected assertion failure to be reported through testing.TB")
	}
	if resp.Code() != 200 {
		t.Fatalf("response still usable after failed assert: %d", resp.Code())
	}

	// Build errors surface too.
	bad := c.Get("/x").JSON(make(chan int)).Send()
	if bad.Err() == nil && len(tb.failures) < 2 {
		t.Fatal("expected marshal error to be reported")
	}
}
