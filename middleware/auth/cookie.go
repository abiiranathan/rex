package auth

import (
	"encoding/gob"
	"net/http"

	"github.com/abiiranathan/rex"
	"github.com/gorilla/sessions"
)

var store *sessions.CookieStore

const (
	authKey     = "authenticated"
	stateKey    = "auth_state"
	sessionName = "auth_session"
)

// Keys are defined in pairs to allow key rotation,
// but the common case is to set a single authentication key and optionally an encryption key.
// The errorCallback is called if the request is not authenticated.
//
// If skip is not nil and returns true, authentication is skipped. This is useful
// for pages where auth is not required e.g login page.
//
// You MUST register the type of state you want to store in the session by calling
// auth.Register or gob.Register.
func Cookie(errorCallback func(c *rex.Context) error, skip func(req *http.Request) bool, keyPairs ...[]byte) rex.Middleware {
	if len(keyPairs) == 0 {
		panic("you must provide at least 1 key")
	}

	if errorCallback == nil {
		panic("you must provide the error callback")
	}

	store = sessions.NewCookieStore(keyPairs...)
	store.Options.HttpOnly = true
	store.Options.Secure = true
	store.Options.SameSite = http.SameSiteStrictMode
	store.Options.MaxAge = 24 * 60 * 60 // 24 hours

	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			if skip != nil && skip(c.Request) {
				return next(c)
			}

			session, err := store.Get(c.Request, sessionName)
			if err != nil {
				return errorCallback(c)
			}

			if session.Values[authKey] != true {
				return errorCallback(c)
			}
			return next(c)
		}
	}
}

// Register registers this type with GOB encoding.
// Otherwise you will get a panic trying to serialize your custom types.
// See gob.Register.
// Example usage: auth.Register(User{})
func Register(value any) {
	gob.Register(value)
}

// SetAuthState stores user state for this request.
// It could the user object, userId or any thing serializable into a cookie.
// This cab be called following user login.
func SetAuthState(c *rex.Context, state any) error {
	session, _ := store.Get(c.Request, sessionName)
	session.Values[authKey] = true
	session.Values[stateKey] = state
	return session.Save(c.Request, c.Response)
}

// Returns the auth state for this request.
func GetAuthState(c *rex.Context) (state any, authenticated bool) {
	session, _ := store.Get(c.Request, sessionName)
	if session.IsNew {
		return nil, false
	}

	state = session.Values[stateKey]
	return state, state != nil && session.Values[authKey] == true
}

// ClearAuthState deletes authentication state.
func ClearAuthState(c *rex.Context) error {
	// remove cookie from store
	session, _ := store.Get(c.Request, sessionName)
	if session.IsNew {
		return nil
	}

	for k := range session.Values {
		delete(session.Values, k)
	}

	cookie, err := c.Request.Cookie(sessionName)
	if err != nil {
		return nil
	}

	// Expire the cookie now.
	cookie.MaxAge = -1

	// Set the cookie to cause it to expire.
	http.SetCookie(c.Response, cookie)

	return session.Save(c.Request, c.Response)
}
