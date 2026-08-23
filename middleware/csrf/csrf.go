// Package csrf provides CSRF protection middleware for rex routers.
//
// Tokens are stored in an HTTP-only cookie and must be echoed back by the
// client either as a form field ("csrf_token") or in the X-CSRF-Token header.
package csrf

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"

	"github.com/abiiranathan/rex"
)

const (
	formKeyName = "csrf_token" // CSRF token in form
	cookieName  = "csrf_token" // CSRF token in HTTP-only cookie
)

var (
	ErrMissingToken = rex.NewError(http.StatusForbidden, "missing CSRF token")
	ErrInvalidToken = rex.NewError(http.StatusForbidden, "invalid CSRF token")
)

// CreateToken generates a random CSRF token.
func CreateToken() (string, error) {
	tokenBytes := make([]byte, 32) // Generate 32 random bytes
	_, _ = rand.Read(tokenBytes)   // never returns an error
	return base64.StdEncoding.EncodeToString(tokenBytes), nil
}

// New returns middleware that sets and verifies CSRF tokens using cookies and forms.
// Set the token in forms using {{ .csrf_token }} in templates.
// If secureCookie is true, the token cookie is transmitted only over HTTPS.
//
// Validation failures are returned as *rex.Error values (HTTP 403), so they
// flow through the router's centralized error handling pipeline and can be
// customized via rex.Router.SetErrorHandler.
func New(secureCookie bool) rex.Middleware {
	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(ctx *rex.Context) error {
			req := ctx.Request
			resp := ctx.Response

			// Get or generate CSRF token.
			token, err := getOrCreateToken(req, resp, secureCookie)
			if err != nil {
				return rex.NewErrorWrap(
					http.StatusInternalServerError,
					"unable to create CSRF token",
					err,
				)
			}

			// Skip CSRF validation for safe methods (GET, HEAD, OPTIONS).
			if rex.IsSafeMethod(req.Method) {
				ctx.Set(formKeyName, token)
				return next(ctx)
			}

			// Validate CSRF token for non-safe methods.
			if !validateCSRFToken(req) {
				return ErrInvalidToken
			}

			// Set the CSRF token in the context.
			ctx.Set(formKeyName, token)
			return next(ctx)
		}
	}
}

// getOrCreateToken retrieves the token from the cookie or creates a new one.
func getOrCreateToken(req *http.Request, resp http.ResponseWriter, secureCookie bool) (string, error) {
	// Check if the CSRF token is already in the cookie.
	cookie, err := req.Cookie(cookieName)
	if err == nil {
		return cookie.Value, nil
	}

	// Generate a new CSRF token if not found in cookies.
	token, err := CreateToken()
	if err != nil {
		return "", err
	}

	// Set the new token in an HTTP-only, secure cookie.
	http.SetCookie(resp, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",          // Make cookie available across the site.
		HttpOnly: true,         // Prevent access via JavaScript.
		Secure:   secureCookie, // Use HTTPS only in prod. (set to false for local testing).
		SameSite: http.SameSiteLaxMode,
	})
	return token, nil
}

// validateCSRFToken checks the token from the form or request header against the cookie.
func validateCSRFToken(req *http.Request) bool {
	// Retrieve the CSRF token from the cookie.
	cookie, err := req.Cookie(cookieName)
	if err != nil {
		return false
	}

	// Retrieve the CSRF token from the request body or headers.
	token := req.FormValue(formKeyName)
	if token == "" {
		token = req.Header.Get("X-CSRF-Token")
		if token == "" {
			return false
		}
	}
	return subtleCompare(token, cookie.Value)
}

// subtleCompare performs a constant-time comparison of two strings to avoid timing attacks.
func subtleCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
