package auth

import (
	"net/http"

	"github.com/abiiranathan/rex"
	"github.com/gorilla/sessions"
	"github.com/pkg/errors"
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
// If skip is not nil and returns true, authentication is skipped. This is useful
// for pages where auth is not required e.g login page.
func Cookie(errorCallback func(c *rex.Context) error, skip func(req *http.Request) bool, keyPairs ...[]byte) rex.Middleware {
	if len(keyPairs) == 0 {
		panic("you must provide at least 1 key")
	}

	if errorCallback == nil {
		panic("you must provide the error callback")
	}

	store = sessions.NewCookieStore(keyPairs...)

	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			if skip != nil && skip(c.Request) {
				return nil
			}

			session, err := store.Get(c.Request, sessionName)
			if err != nil {
				return errors.WithMessage(err, "unable to get session")
			}

			if session.Values[authKey] != true {
				return errorCallback(c)
			}
			return next(c)
		}
	}
}

// SetAuthState stores user state for this request.
// It could the user object, userId or any thing serializable into a cookie.
// This cab be called following user login.
func SetAuthState(req *http.Request, w http.ResponseWriter, state any) error {
	session, err := store.Get(req, sessionName)
	if err != nil {
		return errors.WithMessage(err, "unable to get session")
	}

	session.Values[authKey] = true
	session.Values[stateKey] = state
	return session.Save(req, w)
}

// Returns the auth state for this request.
func GetAuthState(req *http.Request) (state any, authenticated bool) {
	session, err := store.Get(req, sessionName)
	if err != nil {
		return nil, false
	}

	state = session.Values[stateKey]
	return state, session.Values[authKey] == true
}

// ClearAuthState deletes authentication state.
func ClearAuthState(req *http.Request) error {
	session, err := store.Get(req, sessionName)
	if err != nil {
		return err
	}

	delete(session.Values, authKey)
	delete(session.Values, stateKey)
	return nil
}
