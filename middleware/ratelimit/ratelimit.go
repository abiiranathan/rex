package ratelimit

import (
	"net/http"
	"time"

	"github.com/abiiranathan/rex"
)

// Config defines the configuration for the RateLimit middleware.
type Config struct {
	// Rate is the number of requests allowed per second.
	Rate float64

	// Capacity is the maximum burst size.
	Capacity float64

	// KeyFunc generates a key for the request (e.g. IP address).
	// Defaults to c.IP() if nil.
	KeyFunc func(c *rex.Context) string

	// Expiration is the duration after which an idle limiter is removed from memory.
	// Default: 1 minute.
	Expiration time.Duration

	// ErrorHandler is called when the limit is exceeded.
	// Default: returns 429 Too Many Requests.
	ErrorHandler func(c *rex.Context) error
}

// New creates a new RateLimit middleware with the given configuration.
func New(config Config) rex.Middleware {
	if config.Rate <= 0 {
		panic("ratelimit: Rate must be positive")
	}
	if config.Capacity <= 0 {
		panic("ratelimit: Capacity must be positive")
	}
	if config.KeyFunc == nil {
		config.KeyFunc = func(c *rex.Context) string {
			ip, _ := c.IP()
			return ip
		}
	}
	if config.Expiration == 0 {
		config.Expiration = time.Minute
	}
	if config.ErrorHandler == nil {
		config.ErrorHandler = func(c *rex.Context) error {
			c.WriteHeader(http.StatusTooManyRequests)
			return c.String("Too Many Requests")
		}
	}

	manager := NewManager(config.Rate, config.Capacity, config.Expiration)

	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			key := config.KeyFunc(c)
			if !manager.Allow(key) {
				return config.ErrorHandler(c)
			}
			return next(c)
		}
	}
}
