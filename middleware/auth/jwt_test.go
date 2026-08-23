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

	userID, ok := claims["payload"]
	if !ok || userID != payload {
		t.Fatalf("expected payload %s, got %s", payload, userID)
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
		claims, err := auth.JwtClaims(c.Request)
		if err != nil {
			t.Fatalf("%s", err.Error())
		}

		id := claims["payload"]
		if id != payload {
			t.Errorf("expected payload to equal %s, got %v", payload, id)
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

// Test that JWT auth failures flow through the rex error pipeline as *rex.Error.
func TestJWTErrorPipeline(t *testing.T) {
	var captured error

	router := rex.NewRouter()
	router.Use(auth.JWT("test-secret", nil))
	router.SetErrorHandler(&jwtCapturingHandler{capture: &captured, status: http.StatusUnauthorized})
	router.GET("/protected", func(c *rex.Context) error { return nil })

	// Missing token
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.Code)
	}
	if captured == nil {
		t.Fatal("expected error to flow through the error pipeline")
	}
	rexErr, ok := captured.(*rex.Error)
	if !ok {
		t.Fatalf("expected *rex.Error, got %T", captured)
	}
	if rexErr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 error code, got %d", rexErr.Code)
	}

	// Invalid token
	req = httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.Code)
	}
	if captured == nil {
		t.Fatal("expected error to flow through the error pipeline")
	}
}

type jwtCapturingHandler struct {
	capture *error
	status  int
}

func (h *jwtCapturingHandler) Handle(c *rex.Context, err error) {
	*h.capture = err
	c.WriteHeader(h.status)
}
