package recovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"

	"github.com/abiiranathan/rex"
)

// RecoveryConfig holds configuration for the recovery middleware
type RecoveryConfig struct {
	// Whether to include stack trace in logs
	StackTrace bool

	// Custom error handler - if provided, it handles the error completely
	ErrorHandler func(c *rex.Context, err error)

	// Custom logger - if not provided, uses standard log package
	Logger Logger

	// Whether to send error details to client (disable in production)
	ExposeErrors bool

	// Custom error message for production
	ProductionMessage string
}

// Logger interface for custom logging
type Logger interface {
	Printf(format string, v ...any)
}

// defaultLogger wraps the standard log package
type defaultLogger struct{}

func (d defaultLogger) Printf(format string, v ...any) {
	log.Printf(format, v...)
}

// Option defines a function type for configuring RecoveryConfig
type Option func(*RecoveryConfig)

// WithStackTrace configures whether to log stack traces
func WithStackTrace(enabled bool) Option {
	return func(c *RecoveryConfig) {
		c.StackTrace = enabled
	}
}

// WithErrorHandler sets a custom error handler
func WithErrorHandler(handler func(c *rex.Context, err error)) Option {
	return func(c *RecoveryConfig) {
		c.ErrorHandler = handler
	}
}

// WithLogger sets a custom logger
func WithLogger(logger Logger) Option {
	return func(c *RecoveryConfig) {
		c.Logger = logger
	}
}

// WithExposeErrors configures whether to expose error details to clients
func WithExposeErrors(expose bool) Option {
	return func(c *RecoveryConfig) {
		c.ExposeErrors = expose
	}
}

// WithProductionMessage sets a custom error message for production
func WithProductionMessage(message string) Option {
	return func(c *RecoveryConfig) {
		c.ProductionMessage = message
	}
}

// New creates a new recovery middleware with the given options
func New(opts ...Option) rex.Middleware {
	// Default configuration
	config := RecoveryConfig{
		StackTrace:        false,
		Logger:            defaultLogger{},
		ExposeErrors:      false,
		ProductionMessage: "Internal Server Error",
	}

	// Apply options
	for _, opt := range opts {
		opt(&config)
	}

	return func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			defer func() {
				if r := recover(); r != nil {
					err := convertPanicToError(r)

					// Log the error
					logError(config, err)

					// Handle the error
					if config.ErrorHandler != nil {
						config.ErrorHandler(c, err)
						return
					}

					// Default error handling
					sendErrorResponse(c, err, config)
				}
			}()

			return next(c)
		}
	}
}

// convertPanicToError safely converts a panic value to an error
func convertPanicToError(r interface{}) error {
	switch v := r.(type) {
	case error:
		return v
	case string:
		return errors.New(v)
	case fmt.Stringer:
		return errors.New(v.String())
	default:
		return fmt.Errorf("panic: %+v", v)
	}
}

// logError logs the error with optional stack trace
func logError(config RecoveryConfig, err error) {
	if config.StackTrace {
		config.Logger.Printf("Panic recovered: %v\nStack trace:\n%s",
			err, string(debug.Stack()))
	} else {
		config.Logger.Printf("Panic recovered: %v\n", err)
	}
}

// sendErrorResponse sends an appropriate error response to the client
func sendErrorResponse(c *rex.Context, err error, config RecoveryConfig) {
	// Determine response message
	var message string
	if config.ExposeErrors {
		message = err.Error()
	} else {
		message = config.ProductionMessage
	}

	// Try to determine if client expects JSON
	contentType := c.Request.Header.Get("Content-Type")
	accept := c.AcceptHeader()

	wantsJSON := contentType == "application/json" || accept == "application/json"

	// Set status code
	c.WriteHeader(http.StatusInternalServerError)

	if wantsJSON {
		// Send JSON response
		response := map[string]any{"error": message}
		c.SetHeader("Content-Type", "application/json")
		data, _ := json.Marshal(response)
		_, _ = c.Write(data)
	} else {
		// Send plain text response
		c.SetHeader("Content-Type", "text/plain")
		_, _ = c.Write([]byte(message))
	}
}
