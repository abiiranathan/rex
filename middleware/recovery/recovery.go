package recovery

import (
	"errors"
	"log"
	"net/http"
	"runtime/debug"

	"github.com/abiiranathan/rex"
)

// Panic recovery middleware.
// If stack trace is true, a stack trace will be logged.
// If errorHandler is passed, it will be called with the error. No response will be sent to the client.
// Otherwise the error will be logged and sent with a 500 status code.
func New(stackTrace bool, errorHandler ...func(err error)) rex.Middleware {
	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			defer func() {
				if r := recover(); r != nil {
					err, ok := r.(error)
					if !ok {
						// must be a string
						err = errors.New(r.(string))
					}

					if len(errorHandler) > 0 {
						errorHandler[0](err)
					} else {
						if stackTrace {
							log.Println(string(debug.Stack()))
						} else {
							log.Println(err)
						}

						c.WriteHeader(http.StatusInternalServerError)
						c.Write([]byte(err.Error()))
					}
				}
			}()

			return next(c)
		}
	}
}
