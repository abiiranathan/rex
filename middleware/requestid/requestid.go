package requestid

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/abiiranathan/rex"
)

const (
	// HeaderKey is the default header key for request ID.
	HeaderKey = "X-Request-ID"
)

// Config defines the config for RequestID middleware.
type Config struct {
	// Generator defines a function to generate an ID.
	// Defaults to a random 32-character hex string.
	Generator func() string

	// Header is the header key to set.
	// Defaults to "X-Request-ID".
	Header string
}

// New creates a new RequestID middleware with the default configuration.
func New() rex.Middleware {
	return WithConfig(Config{})
}

// WithConfig creates a new RequestID middleware with the given configuration.
func WithConfig(config Config) rex.Middleware {
	if config.Generator == nil {
		config.Generator = randomID
	}
	if config.Header == "" {
		config.Header = HeaderKey
	}

	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			// Check if header already exists
			rid := c.GetHeader(config.Header)
			if rid == "" {
				rid = config.Generator()
				c.Request.Header.Set(config.Header, rid)
			}

			// Set header in response
			c.SetHeader(config.Header, rid)

			return next(c)
		}
	}
}

// randomID generates a random 32-character hex string.
func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
