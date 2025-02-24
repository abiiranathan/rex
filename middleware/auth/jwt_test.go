package auth_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abiiranathan/rex"
	"github.com/abiiranathan/rex/middleware/auth"
)

func TestCreateJWTToken(t *testing.T) {
	payload := "userId"
	duration := time.Minute * 30

	token, err := auth.CreateJWTToken("supersecret", payload, duration)
	if err != nil {
		t.Error(err)
	}

	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		t.Fatalf("invalid JWT token: %s\n", token)
	}
}

func TestVerifyToken(t *testing.T) {
	payload := "userId"
	duration := time.Minute * 30
	secret := "supersecret"

	token, err := auth.CreateJWTToken(secret, payload, duration)
	if err != nil {
		t.Fatal(err)
	}

	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		t.Fatalf("invalid JWT token: %s\n", token)
	}

	claims, err := auth.VerifyJWToken(secret, token)
	if err != nil {
		t.Fatal(err)
	}

	userId, ok := claims["payload"]
	if !ok || userId != payload {
		t.Fatalf("expected payload %s, got %s", payload, userId)
	}

	// Expired token
	token, err = auth.CreateJWTToken(secret, payload, time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}

	claims, err = auth.VerifyJWToken(secret, token)
	if err == nil {
		t.Fatalf("expected error for expired token, got nil")
	}

	fmt.Println(err, claims)

}

func TestJWTMiddleware(t *testing.T) {
	payload := "userId"
	duration := time.Minute * 30
	secret := "supersecret"

	token, err := auth.CreateJWTToken(secret, payload, duration)
	if err != nil {
		t.Fatal(err)
	}

	router := rex.NewRouter()
	router.Use(auth.JWT(secret, nil))

	router.GET("/", func(c *rex.Context) error {
		id := auth.GetPayload(c.Request)

		if id != payload {
			t.Errorf("expected payload to equal %s, got %s", payload, id)
		}

		return c.String("Hello")
	})

	// Request without auth
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	expected := http.StatusUnauthorized

	if w.Result().StatusCode != expected {
		t.Errorf("expected status code %d, got %d", expected, w.Result().StatusCode)
	}

	// Pass the correct authorization
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)
	expected = http.StatusOK

	if w.Result().StatusCode != expected {
		t.Errorf("expected status code %d, got %d", expected, w.Result().StatusCode)
	}

	// Invalid token
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", "invalid token"))
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)
	expected = http.StatusUnauthorized

	if w.Result().StatusCode != expected {
		t.Errorf("expected status code %d, got %d", expected, w.Result().StatusCode)
	}

}
