// package csrf_test

// import (
// 	"net/http"
// 	"net/http/httptest"
// 	"net/url"
// 	"strings"
// 	"testing"

// 	"github.com/abiiranathan/rex"
// 	"github.com/abiiranathan/rex/middleware/csrf"
// 	"github.com/gorilla/sessions"
// 	"github.com/stretchr/testify/require"
// )

// // Mock session store for testing.
// var store = sessions.NewCookieStore([]byte("test-secret"))

// // Middleware test helper to simulate an HTTP request.
// func testMiddleware(method, url string, body string, cookie *http.Cookie, handler http.Handler) *httptest.ResponseRecorder {
// 	req := httptest.NewRequest(method, url, strings.NewReader(body))
// 	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

// 	if cookie != nil {
// 		req.AddCookie(cookie)
// 	}

// 	resp := httptest.NewRecorder()
// 	handler.ServeHTTP(resp, req)
// 	return resp
// }

// // Test that the CSRF token is generated and set in the cookie.
// func TestCSRFTokenGeneration(t *testing.T) {
// 	handler := csrf.New(store, true)(rex.HandlerFunc(func(ctx *rex.Context) error {
// 		_, err := ctx.Response.Write([]byte("OK"))
// 		require.NoError(t, err)
// 		return nil
// 	}))

// 	resp := testMiddleware(http.MethodGet, "/", "", nil, handler)

// 	// Check that the cookie is set.
// 	cookie := resp.Result().Cookies()
// 	require.NotEmpty(t, cookie, "Expected a CSRF cookie to be set")

// 	// Verify the cookie properties.
// 	csrfCookie := cookie[0]
// 	require.Equal(t, "csrf_token", csrfCookie.Name)
// 	require.True(t, csrfCookie.HttpOnly, "Cookie should be HTTP-only")
// 	require.True(t, csrfCookie.Secure, "Cookie should be secure (use HTTPS)")
// }

// // Helper function to create a POST request with form values and cookie.
// func testPOSTRequestWithForm(urlPath string, formData url.Values, cookie *http.Cookie, handler http.Handler) *httptest.ResponseRecorder {
// 	// Encode the form data.
// 	body := strings.NewReader(formData.Encode())

// 	// Create a new POST request.
// 	req := httptest.NewRequest(http.MethodPost, urlPath, body)
// 	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

// 	// Add the CSRF cookie to the request if provided.
// 	if cookie != nil {
// 		req.AddCookie(cookie)
// 	}

// 	// Create a response recorder to capture the response.
// 	resp := httptest.NewRecorder()
// 	handler.ServeHTTP(resp, req)
// 	return resp
// }

// // Test that the CSRF token is validated correctly.
// func TestCSRFTokenValidationSuccess(t *testing.T) {
// 	// Create a valid CSRF token and set it in the cookie.
// 	token, err := csrf.CreateToken()
// 	require.NoError(t, err)

// 	cookie := &http.Cookie{Name: "csrf_token", Value: token}

// 	// Define the handler with CSRF middleware.
// 	handler := csrf.New(store, false)(rex.HandlerFunc(func(ctx *rex.Context) error {
// 		_, err := ctx.Response.Write([]byte("OK"))
// 		require.NoError(t, err)
// 		return nil
// 	}))

// 	// Create form data with the CSRF token.
// 	formData := url.Values{}
// 	formData.Set("csrf_token", token)

// 	// Simulate a POST request with the form data.
// 	resp := testPOSTRequestWithForm("/submit", formData, cookie, handler)

// 	// Check if the response status is 200 OK.
// 	require.Equal(t, http.StatusOK, resp.Code, "Expected 200 OK response")
// }

// // Test that CSRF token validation fails when the token is missing.
// func TestCSRFTokenValidationFailure_MissingToken(t *testing.T) {
// 	handler := csrf.New(store, false)(rex.HandlerFunc(func(ctx *rex.Context) error {
// 		_, err := ctx.Response.Write([]byte("OK"))
// 		require.NoError(t, err)
// 		return nil
// 	}))

// 	// Simulate a POST request without a CSRF token.
// 	resp := testMiddleware(http.MethodPost, "/submit", "", nil, handler)

// 	require.Equal(t, http.StatusForbidden, resp.Code, "Expected 403 Forbidden response")
// }

// // Test that CSRF validation fails if the token in the request doesn't match the cookie.
// func TestCSRFTokenValidationFailure_InvalidToken(t *testing.T) {
// 	// Create a valid token and a mismatched token for the request.
// 	validToken, err := csrf.CreateToken()
// 	require.NoError(t, err)

// 	mismatchedToken, err := csrf.CreateToken()
// 	require.NoError(t, err)

// 	cookie := &http.Cookie{Name: "csrf_token", Value: validToken}

// 	handler := csrf.New(store, false)(rex.HandlerFunc(func(ctx *rex.Context) error {
// 		_, err := ctx.Response.Write([]byte("OK"))
// 		require.NoError(t, err)
// 		return nil
// 	}))

// 	// Simulate a POST request with the mismatched token.
// 	body := "csrf_token=" + mismatchedToken
// 	resp := testMiddleware(http.MethodPost, "/submit", body, cookie, handler)

// 	require.Equal(t, http.StatusForbidden, resp.Code, "Expected 403 Forbidden response")
// }

// // Test that safe HTTP methods (GET, HEAD, OPTIONS) bypass CSRF validation.
// func TestSafeMethodsBypassCSRFValidation(t *testing.T) {
// 	handler := csrf.New(store, false)(rex.HandlerFunc(func(ctx *rex.Context) error {
// 		_, err := ctx.Response.Write([]byte("OK"))
// 		require.NoError(t, err)
// 		return nil
// 	}))

// 	// Test each safe method.
// 	safeMethods := []string{http.MethodGet, http.MethodHead, http.MethodOptions}
// 	for _, method := range safeMethods {
// 		t.Run(method, func(t *testing.T) {
// 			resp := testMiddleware(method, "/", "", nil, handler)
// 			require.Equal(t, http.StatusOK, resp.Code, "Expected 200 OK response")
// 		})
// 	}
// }

package csrf_test

// TODO: fix tests
