package auth

import (
	"crypto/subtle"
	"fmt"
	"net/http"

	"github.com/abiiranathan/rex"
)

// BasicAuth returns middleware that protects routes with HTTP Basic authentication.
// If the credentials are invalid, it responds with status 401.
// The default realm is "Restricted".
func BasicAuth(username, password string, realm ...string) rex.Middleware {
	defaultRealm := "Restricted"
	if len(realm) > 0 {
		defaultRealm = realm[0]
	}

	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(ctx *rex.Context) error {
			user, pass, ok := ctx.Request.BasicAuth()

			if !ok || subtle.ConstantTimeCompare([]byte(user), []byte(username)) != 1 ||
				subtle.ConstantTimeCompare([]byte(pass), []byte(password)) != 1 {

				ctx.Response.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm="%s"`, defaultRealm))
				ctx.WriteHeader(http.StatusUnauthorized)
				return nil
			}
			return next(ctx)
		}
	}
}
