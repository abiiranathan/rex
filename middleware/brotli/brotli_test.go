package brotli_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/abiiranathan/rex"
	"github.com/abiiranathan/rex/middleware/brotli"
	"github.com/abiiranathan/rex/middleware/logger"
	brotlipkg "github.com/andybalholm/brotli"
	"github.com/stretchr/testify/require"
)

func TestBrotliMiddleware(t *testing.T) {
	t.Parallel()
	r := rex.NewRouter()
	r.Use(logger.New(nil))

	r.Use(brotli.Brotli(-1))

	r.GET("/", func(c *rex.Context) error {
		return c.String("Hello World")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "br")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, w.Result().StatusCode, http.StatusOK)
	require.Equal(t, w.Header().Get("Content-Encoding"), "br")
	require.Equal(t, fmt.Sprintf("%x", w.Body.Bytes()),
		"0f05000080aaaaaaeaff74e5db910f373e5c586e04c000120205900fa6ece9")
	require.Equal(t, w.Body.Len(), 31)

}

func TestBrotliMiddlewareConcurrent(t *testing.T) {
	t.Parallel()
	r := rex.NewRouter()
	r.Use(logger.New(nil))
	r.Use(brotli.Brotli(-1))

	r.GET("/{id}", func(c *rex.Context) error {
		// Body content is derived from the request so each goroutine can
		// verify it got back its own response, not one recycled from the
		// writer pool mid-flight.
		return c.String(strings.Repeat("Hello World "+c.Param("id"), 50))
	})

	const goroutines = 100

	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for i := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			path := fmt.Sprintf("/%d", id)
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Accept-Encoding", "br")
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				errs <- fmt.Errorf("id %d: expected status 200, got %d", id, w.Code)
				return
			}
			if w.Header().Get("Content-Encoding") != "br" {
				errs <- fmt.Errorf("id %d: expected Content-Encoding br, got %q", id, w.Header().Get("Content-Encoding"))
				return
			}

			decompressed, err := decompressBrotli(w.Body.Bytes())
			if err != nil {
				errs <- fmt.Errorf("id %d: failed to decompress: %w", id, err)
				return
			}

			expected := strings.Repeat(fmt.Sprintf("Hello World %d", id), 50)
			if decompressed != expected {
				errs <- fmt.Errorf("id %d: body mismatch: got %d bytes, want %d bytes", id, len(decompressed), len(expected))
				return
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

// decompressBrotli decompresses Brotli-encoded data for test verification.
func decompressBrotli(data []byte) (string, error) {
	r := brotlipkg.NewReader(bytes.NewReader(data))
	out, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
