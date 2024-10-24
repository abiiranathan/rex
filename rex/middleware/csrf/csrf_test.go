package csrf_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abiiranathan/rex/rex"
	"github.com/abiiranathan/rex/rex/middleware/csrf"
	"github.com/gorilla/sessions"
)

// test csrf.go

type user struct {
	Name string
	Age  int
}

func TestCSRF(t *testing.T) {
	router := rex.NewRouter()

	store := sessions.NewCookieStore([]byte("super secret token"))
	router.Use(csrf.New(store))

	router.GET("/csrf", func(c *rex.Context) error {
		_, err := c.Write([]byte("Hello CSRF"))
		return err
	})

	router.POST("/csrf", func(c *rex.Context) error {
		var u user
		err := c.BodyParser(&u)
		if err != nil {
			return c.WriteHeader(http.StatusBadRequest)
		}
		return c.JSON(u)
	})

	// create request
	req := httptest.NewRequest("GET", "/csrf", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// check if the response is 200, we/GET /csrf should not be blocked
	if w.Code != 200 {
		t.Errorf("GET /csrf failed: %d", w.Code)
	}

	token := w.Header().Get("X-CSRF-Token")

	// create request
	u := user{Name: "John Doe", Age: 25}

	b, _ := json.Marshal(u)
	body := bytes.NewReader(b)

	req = httptest.NewRequest("POST", "/csrf", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", token)

	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// TODO: Fix this test
	// // check if the response is 200, we/POST /csrf should not be blocked
	// if w.Code != 200 {
	// 	t.Errorf("POST /csrf failed: %d", w.Code)
	// }

	// // create request
	// req = httptest.NewRequest("POST", "/csrf", nil)
	// w = httptest.NewRecorder()
	// router.ServeHTTP(w, req)

	// // check if the response is 403, we/POST /csrf should be blocked
	// if w.Code != 403 {
	// 	t.Errorf("POST /csrf failed: %d", w.Code)
	// }
}
