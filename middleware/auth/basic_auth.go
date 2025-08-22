package auth

import (
	"crypto/subtle"
	"fmt"
	"net/http"

	"github.com/abiiranathan/rex"
)

// Basic Auth middleware.
// If the username and password are not correct, a 401 status code is sent.
// The realm is the realm to display in the login box. Default is "Restricted".
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
