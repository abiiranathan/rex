package brotli_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abiiranathan/rex"
	"github.com/abiiranathan/rex/middleware/brotli"
	"github.com/abiiranathan/rex/middleware/logger"
	"github.com/stretchr/testify/require"
)

func TestBrotliMiddleware(t *testing.T) {
	r := rex.NewRouter()
	r.Use(logger.New(nil))

	r.Use(brotli.Brotli())

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
