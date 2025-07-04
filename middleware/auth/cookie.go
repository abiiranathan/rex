// Package auth provides session-based authentication middleware for the Rex router.
// It uses secure cookie sessions to maintain authentication state and supports storing
// custom user state in the session.
// It also provide JWT and BasicAuth middleware.
// View the README for more information.
package auth

import (
	"context"
	"encoding/gob"
	"net/http"
	"time"

	"github.com/abiiranathan/rex"
	"github.com/gorilla/sessions"
	"github.com/pkg/errors"
)

var store *sessions.CookieStore
var ErrNotInitialized = errors.New("auth: Store not initialized, call auth.InitializeCookieStore first")

type cookieSkipped string
type cookieSession string

const cookieAuthIsSkipped = cookieSkipped("cookie_auth_skipped")
const sessionKey = cookieSession("cookie_session_key")

const (
	authKey  = "rex_authenticated"
	stateKey = "rex_auth_state"
)

// The cookie session name.
var rexSessionName string

type CookieConfig struct {
	// Cookie options.
	// Default: HttpOnly=true, SameSite=Strict(always), MaxAge=24hrs, Domain=/,secure=false
	Options *sessions.Options

	// Skip authentication for certain requests
	SkipAuth func(c *rex.Context) bool

	// Called when authentication fails
	ErrorHandler func(c *rex.Context) error
}

/*
InitializeCookieStore initializes cookie store with the provided secret/encryption key pairs.
Keys are defined in pairs to allow key rotation, but the common case is to
set a single authentication key and optionally an encryption key.

The first key in a pair is used for authentication and the second for encryption.
The encryption key can be set to nil or omitted in the last pair,
but the authentication key is required in all pairs.

It is recommended to use an authentication key with 32 or 64 bytes.
The encryption key, if set, must be either 16, 24, or 32 bytes to select AES-128, AES-192, or AES-256 modes.

userType is the struct instance that is registered with the gob encoder.
*/
func InitializeCookieStore(keyPairs [][]byte, userType any) {
	store = sessions.NewCookieStore(keyPairs...)
	gob.Register(userType)
}

// Cookie creates a new authentication middleware with the given configuration.
// Keys are defined in pairs to allow key rotation,
// but the common case is to set a single authentication key and optionally an encryption key.
//
// You MUST register the type of state you want to store in the session by calling
// auth.Register or gob.Register before using this middleware.
// Access the session with c.Get(auth.SessionKey). It will be nil if not logged in.
func Cookie(sessionName string, config CookieConfig) rex.Middleware {
	if sessionName == "" {
		panic("config.SessionName is required")
	}

	if config.ErrorHandler == nil {
		panic("you must provide the error handler")
	}

	if store == nil {
		panic(ErrNotInitialized)
	}

	// Update the global session name
	rexSessionName = sessionName

	// Set default options if not provided
	if config.Options == nil {
		config.Options = &sessions.Options{
			Path:     "/",
			MaxAge:   int((24 * time.Hour).Seconds()),
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		}
	} else {
		// Override security-critical options
		config.Options.HttpOnly = true
		config.Options.SameSite = http.SameSiteStrictMode

		if config.Options.MaxAge == 0 {
			config.Options.MaxAge = int((24 * time.Hour).Seconds())
		}

		if config.Options.Path == "" {
			config.Options.Path = "/"
		}
	}

	store.Options = config.Options

	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			handleError := func() error {
				if config.SkipAuth != nil && config.SkipAuth(c) {
					c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), cookieAuthIsSkipped, true))
					return next(c)
				}
				return config.ErrorHandler(c)
			}

			session, err := store.Get(c.Request, sessionName)
			if err != nil {
				return handleError()
			}

			if session.Values[authKey] != true {
				return handleError()
			}

			c.Set(sessionKey, session.Values[stateKey])
			return next(c)
		}
	}
}

// SetAuthState stores user state for this request.
// It could be the user object, userId or anything serializable into a cookie.
// This is typically called following user login.
func SetAuthState(c *rex.Context, state any) error {
	if store == nil {
		return ErrNotInitialized
	}

	session, err := store.Get(c.Request, rexSessionName)
	if err != nil {
		return err
	}

	session.Values[authKey] = true
	session.Values[stateKey] = state
	return session.Save(c.Request, c.Response)
}

// CookieValue returns the auth state for this request or nil if not logged in.
func CookieValue(c *rex.Context) (state any) {
	return c.GetOrEmpty(sessionKey)
}

// ClearAuthState deletes authentication state.
func ClearAuthState(c *rex.Context) error {
	if store == nil {
		return ErrNotInitialized
	}

	session, _ := store.Get(c.Request, rexSessionName)
	if session.IsNew {
		return nil
	}

	for k := range session.Values {
		delete(session.Values, k)
	}

	cookie, err := c.Request.Cookie(rexSessionName)
	if err != nil {
		return nil
	}

	cookie.MaxAge = -1
	http.SetCookie(c.Response, cookie)

	return session.Save(c.Request, c.Response)
}

// Returns true if JWT authentication was skipped.
func CookieAuthSkipped(r *http.Request) bool {
	value := r.Context().Value(cookieAuthIsSkipped)
	if skipped, ok := value.(bool); skipped && ok {
		return true
	}
	return false
}
