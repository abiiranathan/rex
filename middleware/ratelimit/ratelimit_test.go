package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abiiranathan/rex"
)

func TestRateLimit(t *testing.T) {
	// 5 requests per second, burst 5
	config := Config{
		Rate:       5,
		Capacity:   5,
		Expiration: time.Minute,
	}

	r := rex.NewRouter()
	r.Use(New(config))

	r.GET("/", func(c *rex.Context) error {
		return c.String("ok")
	})

	// Perform 5 allowed requests
	for i := range 5 {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Request %d failed: %d", i, w.Code)
		}
	}

	// Perform 1 blocked request
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Expected 429, got %d", w.Code)
	}
}

func TestRateLimitRecovery(t *testing.T) {
	// 10 request per second (1 per 100ms)
	config := Config{
		Rate:     10,
		Capacity: 1,
	}

	r := rex.NewRouter()
	r.Use(New(config))

	r.GET("/", func(c *rex.Context) error {
		return c.String("ok")
	})

	// Consume capacity
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatal("First request failed")
	}

	// Immediate next request should fail (capacity 1 exhausted)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatal("Expected limit reached")
	}

	// Wait for refill (100ms + buffer)
	time.Sleep(150 * time.Millisecond)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Expected recovery, got %d", w.Code)
	}
}

func TestRateLimitCustomKey(t *testing.T) {
	config := Config{
		Rate:     1,
		Capacity: 1,
		KeyFunc: func(c *rex.Context) string {
			return c.GetHeader("X-API-Key")
		},
	}

	r := rex.NewRouter()
	r.Use(New(config))

	r.GET("/", func(c *rex.Context) error {
		return c.String("ok")
	})

	// Client A
	reqA := httptest.NewRequest("GET", "/", nil)
	reqA.Header.Set("X-API-Key", "A")
	wA := httptest.NewRecorder()
	r.ServeHTTP(wA, reqA)
	if wA.Code != http.StatusOK {
		t.Error("Client A failed")
	}

	// Client B (should perform independently)
	reqB := httptest.NewRequest("GET", "/", nil)
	reqB.Header.Set("X-API-Key", "B")
	wB := httptest.NewRecorder()
	r.ServeHTTP(wB, reqB)
	if wB.Code != http.StatusOK {
		t.Error("Client B failed")
	}

	// Client A again (blocked)
	wA2 := httptest.NewRecorder()
	r.ServeHTTP(wA2, reqA)
	if wA2.Code != http.StatusTooManyRequests {
		t.Error("Client A should be blocked")
	}
}
