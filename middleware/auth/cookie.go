// Package auth provides session-based authentication middleware for the Rex router.
// It uses secure cookie sessions to maintain authentication state and supports storing
// custom user state in the session.
// It also provide JWT and BasicAuth middleware.
// View the README for more information.
package auth

import (
	"encoding/gob"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/abiiranathan/rex"
	"github.com/gorilla/sessions"
)

// CtxKey identifies auth-related values stored in the request context.
type CtxKey string

// Context variables
// =======================
const (
	authSkipped   = CtxKey("cookie_auth_skipped")
	sessionKey    = "cookie_session_key"
	authKey       = "rex_authenticated"
	stateKey      = "rex_auth_state"
	lastAccessKey = "last_access"
)

// ErrNotInitialized is returned when a CookieAuth instance is nil or missing its store.
var ErrNotInitialized = errors.New("auth: cookie auth is not initialized")

// CookieAuth encapsulates session cookie authentication state and behavior.
type CookieAuth struct {
	store       *sessions.CookieStore
	sessionName string
	config      CookieConfig
	maxAge      time.Duration
	refreshAge  time.Duration

	// cacheKey is the per-request locals key under which the decoded session
	// is cached, so repeated operations in one request (middleware check,
	// SetState, Clear) never decrypt the cookie more than once.
	cacheKey string
}

// CookieConfig defines the behavior of the cookie authentication middleware.
type CookieConfig struct {
	// Cookie options.
	// Defaults: Path="/", HttpOnly=true, SameSite=Strict, Secure=false.
	//
	// Security notes:
	//   - HttpOnly is always enforced (true) and cannot be disabled; scripts
	//     should never read session cookies.
	//   - SameSite defaults to Strict. An explicitly set SameSite value
	//     (e.g. Lax for OAuth redirect flows) is honored.
	//   - MaxAge follows gorilla/sessions semantics: 0 means a browser-session
	//     cookie (no Max-Age attribute; the session ends when the browser
	//     closes, and time-based expiry/sliding refresh are skipped).
	//     A negative value is invalid for auth sessions and falls back to 24h.
	//     When Options is nil entirely, a 24h persistent cookie is used.
	Options *sessions.Options

	// Skip authentication for certain requests
	SkipAuth func(c *rex.Context) bool

	// Called when authentication fails
	ErrorHandler func(c *rex.Context) error
}

// DefaultErrorHandler returns a 401 error through rex's centralized error
// handling pipeline so it can be customized or rendered consistently.
func DefaultErrorHandler(c *rex.Context) error {
	return rex.NewError(http.StatusUnauthorized, "authentication required")
}

func normalizeCookieConfig(config CookieConfig) CookieConfig {
	if config.ErrorHandler == nil {
		config.ErrorHandler = DefaultErrorHandler
	}

	if config.Options == nil {
		config.Options = &sessions.Options{
			Path:     "/",
			MaxAge:   int((24 * time.Hour).Seconds()),
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		}
	} else {
		userOptions := config.Options
		sameSite := userOptions.SameSite
		if sameSite == http.SameSite(0) {
			sameSite = http.SameSiteStrictMode
		}

		config.Options = &sessions.Options{
			Path:     userOptions.Path,
			Domain:   userOptions.Domain,
			MaxAge:   userOptions.MaxAge,
			Secure:   userOptions.Secure,
			HttpOnly: true, // always enforced for safety
			SameSite: sameSite,
		}

		// Negative MaxAge is invalid for auth sessions (gorilla would emit
		// delete-cookies on save); fall back to a 24h persistent cookie.
		// Zero is gorilla's documented browser-session cookie and is kept.
		if config.Options.MaxAge < 0 {
			config.Options.MaxAge = int((24 * time.Hour).Seconds())
		}

		if config.Options.Path == "" {
			config.Options.Path = "/"
		}
	}

	return config
}

// NewCookieAuth creates a cookie authentication instance with its own store and session name.
// keyPairs are gorilla securecookie keys: the first authenticates with HMAC-SHA256,
// the second (optional) encrypts with AES-256. Keys should be 32 or 64 bytes of
// cryptographic randomness (see securecookie.GenerateRandomKey); empty keys are rejected.
func NewCookieAuth(sessionName string, keyPairs [][]byte, userType any, config CookieConfig) (*CookieAuth, error) {
	if sessionName == "" {
		return nil, errors.New("sessionName is required")
	}
	if len(keyPairs) < 1 {
		return nil, errors.New("you must pass atleast one keyPair")
	}
	for i, key := range keyPairs {
		if len(key) == 0 {
			return nil, fmt.Errorf("keyPairs[%d] must not be empty", i)
		}
	}
	if userType == nil {
		return nil, errors.New("userType must not be nil")
	}

	gob.Register(userType)
	gob.Register(time.Time{})

	config = normalizeCookieConfig(config)
	store := sessions.NewCookieStore(keyPairs...)
	store.Options = config.Options

	maxAge := time.Duration(config.Options.MaxAge) * time.Second
	var refreshAge time.Duration // zero disables sliding refresh
	if maxAge > 0 {
		refreshAge = maxAge / 2
	}

	return &CookieAuth{
		store:       store,
		sessionName: sessionName,
		config:      config,
		maxAge:      maxAge,
		refreshAge:  refreshAge,
		cacheKey:    "rex_auth_cached_session:" + sessionName,
	}, nil
}

// getSession returns the decoded session for this request, reusing the copy
// cached by an earlier call (middleware check, SetState, Clear) within the
// same request so the cookie is verified at most once.
func (a *CookieAuth) getSession(c *rex.Context) (*sessions.Session, error) {
	if cached, ok := c.Get(a.cacheKey); ok {
		if session, ok := cached.(*sessions.Session); ok {
			return session, nil
		}
	}

	session, err := a.store.Get(c.Request, a.sessionName)
	if err != nil {
		return nil, err
	}
	c.Set(a.cacheKey, session)
	return session, nil
}

func (a *CookieAuth) unauthenticated(c *rex.Context, next rex.HandlerFunc) error {
	if a.config.SkipAuth != nil && a.config.SkipAuth(c) {
		c.Set(string(authSkipped), true)
		return next(c)
	}
	return a.config.ErrorHandler(c)
}

func (a *CookieAuth) expire(c *rex.Context) {
	http.SetCookie(c.Response, &http.Cookie{
		Name:     a.sessionName,
		Path:     a.config.Options.Path,
		Domain:   a.config.Options.Domain,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: a.config.Options.HttpOnly,
		Secure:   a.config.Options.Secure,
		SameSite: a.config.Options.SameSite,
	})
}

// Middleware returns the cookie authentication middleware for this instance.
func (a *CookieAuth) Middleware() rex.Middleware {
	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			session, err := a.getSession(c)
			if err != nil {
				a.expire(c)
				return a.unauthenticated(c, next)
			}

			if session.Values[authKey] != true {
				return a.unauthenticated(c, next)
			}

			lastAccess, ok := session.Values[lastAccessKey].(time.Time)
			if !ok {
				return a.unauthenticated(c, next)
			}

			// Browser-session cookies (maxAge <= 0) skip time-based expiry;
			// their lifetime ends with the browser session.
			if a.maxAge > 0 {
				now := time.Now()
				sessionAge := now.Sub(lastAccess)
				if sessionAge > a.maxAge {
					return a.unauthenticated(c, next)
				}

				if sessionAge > a.refreshAge {
					session.Values[lastAccessKey] = now
					session.Options = a.config.Options
					if err := session.Save(c.Request, c.Response); err != nil {
						return err
					}
				}
			}

			c.Set(sessionKey, session.Values[stateKey])
			return next(c)
		}
	}
}

// SetState stores authentication state for this instance.
// A tampered or undecodable existing session cookie does not fail the call;
// a fresh session replaces it (login overwrites bad state).
func (a *CookieAuth) SetState(c *rex.Context, state any) error {
	if a == nil || a.store == nil {
		return ErrNotInitialized
	}

	session, err := a.getSession(c)
	if err != nil {
		session = sessions.NewSession(a.store, a.sessionName)
		c.Set(a.cacheKey, session)
	}

	session.Values[authKey] = true
	session.Values[stateKey] = state
	session.Values[lastAccessKey] = time.Now()
	session.Options = a.config.Options
	return session.Save(c.Request, c.Response)
}

// Value returns the auth state for this request or nil if not logged in.
func (a *CookieAuth) Value(c *rex.Context) any {
	return c.GetOrEmpty(sessionKey)
}

// Clear deletes authentication state for this instance.
func (a *CookieAuth) Clear(c *rex.Context) {
	if a == nil || a.store == nil {
		return
	}

	if session, err := a.getSession(c); err == nil {
		clear(session.Values)
	}
	a.expire(c)
}

// Skipped reports whether this request skipped cookie authentication.
func (a *CookieAuth) Skipped(c *rex.Context) bool {
	value, ok := c.Get(string(authSkipped))
	if !ok {
		return false
	}
	skipped, ok := value.(bool)
	return ok && skipped
}
