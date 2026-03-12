// Package auth provides session-based authentication middleware for the Rex router.
// It uses secure cookie sessions to maintain authentication state and supports storing
// custom user state in the session.
// It also provide JWT and BasicAuth middleware.
// View the README for more information.
package auth

import (
	"encoding/gob"
	"errors"
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
}

// CookieConfig defines the behavior of the cookie authentication middleware.
type CookieConfig struct {
	// Cookie options.
	// Default: HttpOnly=true, SameSite=Strict(always), MaxAge=24hrs, Domain=/,secure=false
	Options *sessions.Options

	// Skip authentication for certain requests
	SkipAuth func(c *rex.Context) bool

	// Called when authentication fails
	ErrorHandler func(c *rex.Context) error
}

// DefaultErrorHandler returns HTTP 401 for unauthenticated requests.
func DefaultErrorHandler(c *rex.Context) error {
	c.WriteHeader(http.StatusUnauthorized)
	return nil
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
		config.Options = &sessions.Options{
			Path:     config.Options.Path,
			Domain:   config.Options.Domain,
			MaxAge:   config.Options.MaxAge,
			Secure:   config.Options.Secure,
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		}

		if config.Options.MaxAge <= 0 {
			config.Options.MaxAge = int((24 * time.Hour).Seconds())
		}

		if config.Options.Path == "" {
			config.Options.Path = "/"
		}
	}

	return config
}

// NewCookieAuth creates a cookie authentication instance with its own store and session name.
func NewCookieAuth(sessionName string, keyPairs [][]byte, userType any, config CookieConfig) (*CookieAuth, error) {
	if sessionName == "" {
		return nil, errors.New("sessionName is required")
	}
	if len(keyPairs) < 1 {
		return nil, errors.New("you must pass atleast one keyPair")
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
	return &CookieAuth{
		store:       store,
		sessionName: sessionName,
		config:      config,
		maxAge:      maxAge,
		refreshAge:  maxAge / 2,
	}, nil
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
			session, err := a.store.Get(c.Request, a.sessionName)
			if err != nil {
				a.expire(c)
				return a.unauthenticated(c, next)
			}

			if session.Values[authKey] != true {
				return a.unauthenticated(c, next)
			}

			now := time.Now()
			lastAccess, ok := session.Values[lastAccessKey].(time.Time)
			if !ok {
				return a.unauthenticated(c, next)
			}

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

			c.Set(sessionKey, session.Values[stateKey])
			return next(c)
		}
	}
}

// SetState stores authentication state for this instance.
func (a *CookieAuth) SetState(c *rex.Context, state any) error {
	if a == nil || a.store == nil {
		return ErrNotInitialized
	}

	session, err := a.store.Get(c.Request, a.sessionName)
	if err != nil {
		return err
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

	session, err := a.store.Get(c.Request, a.sessionName)
	if err == nil {
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
