// Description: CSRF protection middleware for Go web servers.
// The CSRF middleware generates and validates tokens to prevent cross-site request forgery attacks.
// The CSRF token is set in an HTTP-only cookie(to prevent access via JavaScript) and and a hidden form field.
// The middleware checks the token in the form or request headers against the cookie.
// The CSRF token is generated using 32 random bytes encoded in base64.
// Access to the token is provided in the context using the key "csrf_token".
package csrf

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/abiiranathan/rex"
	"github.com/gorilla/sessions"
)

const (
	formKeyName = "csrf_token"   // CSRF token in form
	sessionName = "csrf_session" // Session name for CSRF token storage
	cookieName  = "csrf_token"   // CSRF token in HTTP-only cookie
)

var (
	ErrMissingToken = errors.New("missing CSRF token")
	ErrInvalidToken = errors.New("invalid CSRF token")
	httpsCookie     = true
)

// Generates a random CSRF token.
func CreateToken() (string, error) {
	tokenBytes := make([]byte, 32) // Generate 32 random bytes
	_, err := rand.Read(tokenBytes)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(tokenBytes), nil
}

// Middleware sets and verifies CSRF tokens using HTTP-only cookies and forms.
// Set the CSRF token in the form using {{ .csrf_token }} in the template.
// If secureCookie is true, the csrf token is transmitted only in a secure context (https).
func New(store sessions.Store, secureCookie bool) rex.Middleware {
	httpsCookie = secureCookie

	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(ctx *rex.Context) error {
			req := ctx.Request
			resp := ctx.Response

			// Get or generate CSRF token.
			token, err := getOrCreateToken(req, resp)
			if err != nil {
				return fmt.Errorf("unable to create CSRF token: %v", err)
			}

			// Skip CSRF validation for safe methods (GET, HEAD, OPTIONS).
			if rex.IsSafeMethod(req.Method) {
				ctx.Set(formKeyName, token)
				return next(ctx)
			}

			// Validate CSRF token for non-safe methods.
			if !validateCSRFToken(req) {
				http.Error(resp, "Forbidden: CSRF token validation failed", http.StatusForbidden)
				return nil
			}

			// Continue with the next handler.
			_reqContext := context.WithValue(req.Context(), formKeyName, token)
			*req = *req.WithContext(_reqContext)
			return next(ctx)
		}
	}
}

// getOrCreateToken retrieves the token from the cookie or creates a new one.
func getOrCreateToken(req *http.Request, resp http.ResponseWriter) (string, error) {
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
		Path:     "/",         // Make cookie available across the site.
		HttpOnly: true,        // Prevent access via JavaScript.
		Secure:   httpsCookie, // Use HTTPS only in prod. (set to false for local testing).
		SameSite: http.SameSiteLaxMode,
	})
	return token, nil
}

// validateCSRFToken checks the token from the form or request header against the cookie.
func validateCSRFToken(req *http.Request) bool {
	// Retrieve the CSRF token from the cookie.
	cookie, err := req.Cookie(cookieName)
	if err != nil {
		log.Println("CSRF token not found in cookie:", err)
		return false
	}

	// Retrieve the CSRF token from the request body or headers.
	token := req.FormValue(formKeyName)
	if token == "" {
		token = req.Header.Get("X-CSRF-Token")
		if token == "" {
			log.Println("CSRF token not found in form or headers")
			return false
		}
	}

	// Compare tokens.
	if subtleCompare(token, cookie.Value) {
		return true
	}

	log.Println("CSRF token mismatch")
	return false
}

// subtleCompare performs a constant-time comparison of two strings to avoid timing attacks.
func subtleCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
