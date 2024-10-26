package cors_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abiiranathan/rex"
	"github.com/abiiranathan/rex/middleware/cors"
)

func TestCors(t *testing.T) {
	router := rex.NewRouter()
	host := "localhost:8080"

	router.Use(cors.New(cors.CORSOptions{
		AllowCredentials: true,
		Allowwebsockets:  true,
		AllowedOrigins:   []string{host},
		AllowedMethods:   []string{"OPTIONS", "GET"},
		AllowedHeaders:   []string{"Content-Type", "Host"},
		ExposedHeaders:   []string{"Content-Length", "Cache-Control"},
		MaxAge:           3600,
	}))

	res := "Hello World"

	router.GET("/", func(c *rex.Context) error {
		return c.String(res)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", host)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected 200 status, got %d", w.Result().StatusCode)
	}

	if w.Header().Get("Access-Control-Allow-Origin") != host {
		t.Errorf("expected Access-Control-Allow-Origin in response headers")
	}

	// test with unsupported origin
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "localhost:3030")
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 status, got %d", w.Result().StatusCode)
	}

}
