package etag_test

import (
	"crypto/sha1"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abiiranathan/rex"
	"github.com/abiiranathan/rex/middleware/etag"
)

func TestEtag(t *testing.T) {
	router := rex.NewRouter()
	router.Use(etag.New())
	res := "Hello World!"

	router.GET("/", func(c *rex.Context) error {
		return c.String(res)
	})

	// test returning error
	router.GET("/error", func(c *rex.Context) error {
		return fmt.Errorf("some thing went wrong")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected 200 status, got %d", w.Result().StatusCode)
	}

	etagHeader := w.Header().Get("Etag")
	if etagHeader == "" {
		t.Error("expected a valid etag header, got empty string")
	}

	hash := sha1.New()
	hash.Write([]byte(res))
	b := hash.Sum(nil)
	expected := fmt.Sprintf(`"%x"`, b)
	// fmt.Printf("Etag: %s\n", expected)

	if expected != etagHeader {
		t.Fatalf("expected etag %x, got %s", expected, etagHeader)
	}

	req = httptest.NewRequest(http.MethodGet, "/error", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 status, got %d", w.Result().StatusCode)
	}

	fmt.Println(w.Body.String())
}
