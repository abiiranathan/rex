package flash

import (
	"encoding/gob"
	"fmt"
	"net/http"
	"strings"

	"github.com/abiiranathan/rex"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/pkg/errors"
)

var flashMessageStore sessions.Store

const (
	sessionName      = "flash_messages_session"
	flashMessageKey  = "flash_message"
	flashMessageType = "flash_message_type"
)

// setFlashMessage sets a flash message for the given key
func setFlashMessage(c *rex.Context, key string, fm Flash) error {
	sess, _ := flashMessageStore.Get(c.Request, sessionName)

	sess.Values[key] = fm
	err := sess.Save(c.Request, c.Response)
	return errors.WithMessage(err, "error saving session")
}

func clearCookie(c *rex.Context) error {
	session, _ := flashMessageStore.Get(c.Request, sessionName)

	// Clear the session cookie
	for k := range session.Values {
		delete(session.Values, k)
	}

	cookie, err := c.Request.Cookie(sessionName)
	if err != nil {
		return nil
	}

	cookie.MaxAge = -1
	http.SetCookie(c.Response, cookie)

	err = session.Save(c.Request, c.Response)
	if err != nil {
		return errors.WithMessage(err, "error saving session")
	}
	return nil
}

// getFlashMessage retrieves and deletes a flash message for the given key
func getFlashMessage(c *rex.Context, key string) (Flash, error) {
	sess, _ := flashMessageStore.Get(c.Request, sessionName)
	if message, ok := sess.Values[key]; ok {
		_ = clearCookie(c)
		return message.(Flash), nil
	}
	return Flash{}, fmt.Errorf("no flash message found for key %s", key)
}

// Flash message types
type Flash struct {
	Message string // Message for the flash
	Type    string // success, info, warning, error
}

type FlashMessageType int

const (
	MessageSuccess FlashMessageType = iota
	MessageInfo
	MessageWarning
	MessageError
)

func init() {
	secret := securecookie.GenerateRandomKey(32)
	store := sessions.NewCookieStore(secret)
	store.Options.Secure = false  // Send both on http & https
	store.Options.HttpOnly = true // XSS protection. No access from JS
	store.Options.MaxAge = 0      // Session cookie is deleted when browser is closed

	flashMessageStore = store
	gob.Register(Flash{})
}

// Helper fucntion to set a flash message in session.
// default messageType is error.
func FlashMessage(c *rex.Context, message string, messageType ...FlashMessageType) error {
	m_type := "danger"

	if len(messageType) > 0 {
		switch messageType[0] {
		case MessageInfo:
			m_type = "info"
		case MessageSuccess:
			m_type = "success"
		case MessageWarning:
			m_type = "warning"
		default:
			m_type = "danger"
		}
	}

	err := setFlashMessage(c, flashMessageKey, Flash{
		Message: message,
		Type:    m_type,
	})
	return errors.WithMessage(err, "error setting flash message")
}

// FlashMessageMiddleware is a middleware that sets the flash message in the context.
// Flash messages are inserted into the context by calling FlashMessage function.
// The context variables passed are "flash_message" (string)
// and "flash_message_type" (string) [success,info,warning,danger]
func FlashMessageMiddleware() rex.Middleware {
	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			accept := c.AcceptHeader()

			// Only set flash messages for HTML requests
			if strings.HasPrefix(accept, "text/html") || strings.HasPrefix(accept, "*/*") {
				message, err := getFlashMessage(c, flashMessageKey)
				if message.Message != "" && err == nil {
					c.Set(flashMessageKey, message.Message)
					c.Set(flashMessageType, message.Type)
				}
			}
			return next(c)
		}
	}
}
