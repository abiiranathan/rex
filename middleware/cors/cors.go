package cors

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/abiiranathan/rex"
)

// CORSOptions is the configuration for the CORS middleware.
type CORSOptions struct {
	AllowedOrigins   []string // Origins that are allowed in the request, default is all origins
	AllowedMethods   []string // Methods that are allowed in the request
	AllowedHeaders   []string // Headers that are allowed in the request
	ExposedHeaders   []string // Headers that are exposed to the client
	AllowCredentials bool     // Allow credentials like cookies, authorization headers
	MaxAge           int      // Max age in seconds to cache preflight request
	Allowwebsockets  bool     // Allow websockets
}

// New creates a Cors middleware with default options.
// If opts argument is provided, it is used instead of defaults.
// All CORSOptions must be provided since there is no merging with defaults.
// If the origin is not allowed, a 403 status code is sent.
func New(opts ...CORSOptions) rex.Middleware {
	var options = CORSOptions{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{http.MethodGet, http.MethodOptions, http.MethodPost, http.MethodPut, http.MethodDelete},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: false,
		MaxAge:           5,
		Allowwebsockets:  false,
	}

	if len(opts) > 0 {
		options = opts[0]
	}

	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			origin := c.Request.Header.Get("Origin")

			if len(options.AllowedOrigins) > 0 {
				allowed := false
				for _, v := range options.AllowedOrigins {
					if v == origin || v == "*" {
						allowed = true
						break
					}
				}

				if !allowed {
					return c.WriteHeader(http.StatusForbidden)
				}
			}

			c.Response.Header().Set("Access-Control-Allow-Origin", origin)

			if len(options.AllowedMethods) > 0 {
				c.Response.Header().Set("Access-Control-Allow-Methods", joinStrings(options.AllowedMethods))
			}

			if len(options.AllowedHeaders) > 0 {
				c.Response.Header().Set("Access-Control-Allow-Headers", joinStrings(options.AllowedHeaders))
			}

			if len(options.ExposedHeaders) > 0 {
				c.Response.Header().Set("Access-Control-Expose-Headers", joinStrings(options.ExposedHeaders))
			}

			if options.AllowCredentials {
				c.Response.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if options.MaxAge > 0 {
				c.Response.Header().Set("Access-Control-Max-Age", fmt.Sprintf("%d", options.MaxAge))
			}

			if options.Allowwebsockets {
				c.Response.Header().Set("Access-Control-Allow-Websocket", "true")
			}

			if c.Request.Method == http.MethodOptions {
				c.Response.WriteHeader(http.StatusNoContent)
				return nil
			}

			return next(c)
		}
	}
}

func joinStrings(s []string) string {
	return strings.Join(s, ", ")
}
