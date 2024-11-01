package rex_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/abiiranathan/rex"
)

// test group GET, POST, PUT, PATCH, DELETE
func TestRouterGroupMethods(t *testing.T) {
	r := rex.NewRouter()
	admin := r.Group("/admin")

	admin.GET("/home", func(c *rex.Context) error {
		return c.String("test")
	})

	admin.POST("/home", func(c *rex.Context) error {
		return c.String("test")
	})

	admin.PUT("/home", func(c *rex.Context) error {
		return c.String("test")
	})

	admin.PATCH("/home", func(c *rex.Context) error {
		return c.String("test")
	})

	admin.DELETE("/home", func(c *rex.Context) error {
		return c.String("test")
	})

	// test /admin/home GET
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/admin/home", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "test" {
		t.Errorf("expected test, got %s", w.Body.String())
	}

	// test /admin/home POST
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/admin/home", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "test" {
		t.Errorf("expected test, got %s", w.Body.String())
	}

	// test /admin/home PUT
	w = httptest.NewRecorder()
	req = httptest.NewRequest("PUT", "/admin/home", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "test" {
		t.Errorf("expected test, got %s", w.Body.String())
	}

	// test /admin/home PATCH
	w = httptest.NewRecorder()
	req = httptest.NewRequest("PATCH", "/admin/home", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "test" {
		t.Errorf("expected test, got %s", w.Body.String())
	}

	// test /admin/home DELETE
	w = httptest.NewRecorder()
	req = httptest.NewRequest("DELETE", "/admin/home", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "test" {
		t.Errorf("expected test, got %s", w.Body.String())
	}
}

func TestGroupMiddleware(t *testing.T) {
	r := rex.NewRouter()
	admin := r.Group("/admin")

	admin.Use(func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			c.Set("admin", "admin middleware")
			return next(c)
		}
	})

	admin.GET("/test", func(c *rex.Context) error {
		admin, ok := c.Get("admin")
		if !ok {
			c.WriteHeader(http.StatusInternalServerError)
			return c.String("no admin")
		}
		return c.String(admin.(string))
	})

	// test /admin/test
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/admin/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "admin middleware" {
		t.Errorf("expected admin middleware, got %s", w.Body.String())
	}
}

// Test route groups
func TestRouterGroup(t *testing.T) {
	r := rex.NewRouter()
	admin := r.Group("/admin")

	admin.GET("/home", func(c *rex.Context) error {
		return c.String("test")
	})

	admin.GET("/users", func(c *rex.Context) error {
		return c.String("test2")
	})

	// test /admin/test
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/admin/home", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "test" {
		t.Errorf("expected test, got %s", w.Body.String())
	}

	// test /admin/test2
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/admin/users", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "test2" {
		t.Errorf("expected test2, got %s", w.Body.String())
	}
}

// test groups with middleware
func TestRouterGroupMiddleware(t *testing.T) {
	r := rex.NewRouter()
	admin := r.Group("/admin", func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			c.Set("admin", "admin middleware")
			return next(c)
		}
	})

	admin.GET("/test", func(c *rex.Context) error {
		admin, ok := c.Get("admin")
		if !ok {
			c.WriteHeader(http.StatusInternalServerError)
			return c.String("no admin")
		}
		return c.String(admin.(string))
	})

	// test /admin/test
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/admin/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "admin middleware" {
		t.Errorf("expected admin middleware, got %s", w.Body.String())
	}
}

// test nested groups
func TestRouterNestedGroup(t *testing.T) {
	r := rex.NewRouter()
	admin := r.Group("/admin")
	users := admin.Group("/users")

	users.GET("/test", func(c *rex.Context) error {
		return c.String("test")
	})

	// test /admin/users/test
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/admin/users/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "test" {
		t.Errorf("expected test, got %s", w.Body.String())
	}
}

// Test static files for a group
func TestRouterGroupStatic(t *testing.T) {
	dirname, err := os.MkdirTemp("", "static")
	if err != nil {
		t.Fatalf("could not create temp dir: %v", err)
	}
	defer os.RemoveAll(dirname)

	file := filepath.Join(dirname, "test.txt")
	err = os.WriteFile(file, []byte("hello world"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	r := rex.NewRouter()
	admin := r.Group("/admin")
	admin.Static("/static", dirname)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/admin/static/notfound.txt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/admin/static/test.txt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	data, err := io.ReadAll(w.Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "hello world" {
		t.Errorf("expected hello world, got %s", string(data))
	}
}

// test group Static with file system
func TestRouterGroupStaticFS(t *testing.T) {
	dirname, err := os.MkdirTemp("", "static")
	if err != nil {
		t.Fatalf("could not create temp dir: %v", err)
	}
	defer os.RemoveAll(dirname)

	file := filepath.Join(dirname, "test.txt")
	err = os.WriteFile(file, []byte("hello world"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	r := rex.NewRouter()

	admin := r.Group("/admin")
	admin.StaticFs("/static", http.Dir(dirname), 3600)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/admin/static/notfound.txt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/admin/static/test.txt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	data, err := io.ReadAll(w.Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "hello world" {
		t.Errorf("expected hello world, got %s", string(data))
	}
}
