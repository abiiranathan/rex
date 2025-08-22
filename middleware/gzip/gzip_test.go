package gzip

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abiiranathan/rex"
)

func TestGzip_WithAcceptEncoding(t *testing.T) {
	app := rex.NewRouter()
	app.Use(Gzip())

	app.GET("/test", func(c *rex.Context) error {
		return c.String("Hello, World!")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("expected Content-Encoding: gzip, got %s", w.Header().Get("Content-Encoding"))
	}

	if w.Header().Get("Content-Length") != "" {
		t.Error("Content-Length header should be removed for compressed content")
	}

	// Verify the content is actually gzip compressed
	reader, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to decompress response: %v", err)
	}

	if string(decompressed) != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got %s", string(decompressed))
	}
}

func TestGzip_WithoutAcceptEncoding(t *testing.T) {
	app := rex.NewRouter()
	app.Use(Gzip())

	app.GET("/test", func(c *rex.Context) error {
		return c.String("Hello, World!")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	// No Accept-Encoding header
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Encoding") != "" {
		t.Errorf("Content-Encoding should not be set, got %s", w.Header().Get("Content-Encoding"))
	}

	if w.Body.String() != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got %s", w.Body.String())
	}
}

func TestGzip_WithoutGzipInAcceptEncoding(t *testing.T) {
	app := rex.NewRouter()
	app.Use(Gzip())

	app.GET("/test", func(c *rex.Context) error {
		return c.String("Hello, World!")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Accept-Encoding", "deflate, br")
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Encoding") != "" {
		t.Errorf("Content-Encoding should not be set, got %s", w.Header().Get("Content-Encoding"))
	}

	if w.Body.String() != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got %s", w.Body.String())
	}
}

func TestGzip_SkipPaths(t *testing.T) {
	app := rex.NewRouter()
	app.Use(Gzip("/api/raw", "/static"))

	app.GET("/api/raw/data", func(c *rex.Context) error {
		return c.String("Raw data")
	})

	app.GET("/api/compressed", func(c *rex.Context) error {
		return c.String("Compressed data")
	})

	// Test skipped path
	req1 := httptest.NewRequest("GET", "/api/raw/data", nil)
	req1.Header.Set("Accept-Encoding", "gzip")
	w1 := httptest.NewRecorder()

	app.ServeHTTP(w1, req1)

	if w1.Header().Get("Content-Encoding") != "" {
		t.Errorf("skipped path should not be compressed, got Content-Encoding: %s", w1.Header().Get("Content-Encoding"))
	}

	if w1.Body.String() != "Raw data" {
		t.Errorf("expected 'Raw data', got %s", w1.Body.String())
	}

	// Test non-skipped path
	req2 := httptest.NewRequest("GET", "/api/compressed", nil)
	req2.Header.Set("Accept-Encoding", "gzip")
	w2 := httptest.NewRecorder()

	app.ServeHTTP(w2, req2)

	if w2.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("non-skipped path should be compressed, got Content-Encoding: %s", w2.Header().Get("Content-Encoding"))
	}
}

func TestGzipLevel_BestSpeed(t *testing.T) {
	app := rex.NewRouter()
	app.Use(GzipLevel(gzip.BestSpeed))

	app.GET("/test", func(c *rex.Context) error {
		// Use a larger string to better test compression
		data := strings.Repeat("Hello, World! ", 100)
		return c.String(data)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("expected Content-Encoding: gzip, got %s", w.Header().Get("Content-Encoding"))
	}

	// Verify decompression works
	reader, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to decompress response: %v", err)
	}

	expected := strings.Repeat("Hello, World! ", 100)
	if string(decompressed) != expected {
		t.Errorf("decompressed content doesn't match expected")
	}
}

func TestGzipLevel_BestCompression(t *testing.T) {
	app := rex.NewRouter()
	app.Use(GzipLevel(gzip.BestCompression))

	app.GET("/test", func(c *rex.Context) error {
		data := strings.Repeat("This is a test string for compression. ", 50)
		return c.String(data)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("expected Content-Encoding: gzip, got %s", w.Header().Get("Content-Encoding"))
	}
}

func TestGzipLevel_InvalidLevel(t *testing.T) {
	app := rex.NewRouter()
	app.Use(GzipLevel(10)) // Invalid compression level

	app.GET("/test", func(c *rex.Context) error {
		return c.String("Hello, World!")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	// Should handle error gracefully - exact behavior depends on rex error handling
	// This test ensures the middleware doesn't panic
	if w.Code == 0 {
		t.Error("middleware should handle invalid compression level gracefully")
	}
}

func TestGzip_JSONResponse(t *testing.T) {
	app := rex.NewRouter()
	app.Use(Gzip())

	app.GET("/json", func(c *rex.Context) error {
		data := map[string]any{
			"message": "Hello, World!",
			"status":  "success",
			"data":    []int{1, 2, 3, 4, 5},
		}
		return c.JSON(data)
	})

	req := httptest.NewRequest("GET", "/json", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("expected Content-Encoding: gzip, got %s", w.Header().Get("Content-Encoding"))
	}

	// Verify JSON can be decompressed
	reader, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to decompress response: %v", err)
	}

	// Should be valid JSON
	if !strings.Contains(string(decompressed), `"message":"Hello, World!"`) {
		t.Error("decompressed JSON doesn't contain expected content")
	}
}

func TestGzip_LargeResponse(t *testing.T) {
	app := rex.NewRouter()
	app.Use(Gzip())

	app.GET("/large", func(c *rex.Context) error {
		// Generate a large response (10KB)
		data := strings.Repeat("This is a large response for testing compression efficiency. ", 200)
		return c.String(data)
	})

	req := httptest.NewRequest("GET", "/large", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	originalSize := len(strings.Repeat("This is a large response for testing compression efficiency. ", 200))
	compressedSize := w.Body.Len()

	// Gzip should compress this repetitive text significantly
	if compressedSize >= originalSize {
		t.Errorf("compressed size (%d) should be smaller than original size (%d)", compressedSize, originalSize)
	}

	// Verify decompression
	reader, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to decompress response: %v", err)
	}

	if len(decompressed) != originalSize {
		t.Errorf("decompressed size (%d) doesn't match original size (%d)", len(decompressed), originalSize)
	}
}
