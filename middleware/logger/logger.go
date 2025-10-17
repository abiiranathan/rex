package logger

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"

	"github.com/abiiranathan/rex"
)

// LogFormat is the format of the log output, compatible with the new slog package.
type LogFormat int
type LogFlags int8

const (
	TextFormat LogFormat = iota + 1 // This is the default format
	JSONFormat                      // Log in JSON format
)

const (
	LOG_IP LogFlags = 1 << iota
	LOG_LATENCY
	LOG_USERAGENT
)

const StdLogFlags LogFlags = LOG_LATENCY | LOG_IP

// Config is a middleware that logs the request and response information.
type Config struct {
	// Output is the destination for the log output. If nil, os.Stderr is used.
	Output io.Writer

	// Format is the format of the log output. Default is TextFormat.
	Format LogFormat

	// Flags is the flags to be used for logging. Default is StdLogFlags.
	Flags LogFlags

	// Skip is a slice of paths that should not be logged.
	Skip []string

	// SkipIf is a function that can be used to skip logging based on the request.
	// If it returns true, the request will not be logged.
	SkipIf func(r *http.Request) bool

	// Options is the options to be passed to the slog.Handler.
	Options *slog.HandlerOptions

	// Callback is a function that can be used to modify the arguments passed to the logger.
	// Forexample the request_id, user_id etc. It MUST return an even number of arguments.
	Callback func(c *rex.Context, args ...any) []any
}

// DefaultConfig is the default logger used by the Logger middleware.
// It writes logs to os.Stderr with the TextFormat and StdLogFlags.
// The log level is set to Info.
var DefaultConfig = &Config{
	Output: os.Stderr,
	Format: TextFormat,
	Flags:  StdLogFlags,
	Options: &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: false,
	},
}

// New returns a new Logger middleware with the provided configuration.
// The logger needs access to status code and thus must apear before middleware wrapping the default
// response writer (like etags and Brotli)
func New(config *Config) rex.Middleware {
	if config == nil {
		config = DefaultConfig
	}

	if config.Output == nil {
		config.Output = os.Stderr
	}

	if config.Format == 0 {
		config.Format = TextFormat
	}

	if config.Options == nil {
		config.Options = &slog.HandlerOptions{
			Level:     slog.LevelInfo,
			AddSource: false,
		}
	}

	return config.Logger
}

// Logger is the middleware handler function for LoggerMiddleware.
func (l *Config) Logger(next rex.HandlerFunc) rex.HandlerFunc {
	return func(c *rex.Context) error {
		if slices.Contains(l.Skip, c.Request.URL.Path) {
			return next(c)
		}

		if l.SkipIf != nil && l.SkipIf(c.Request) {
			return next(c)
		}

		err := next(c)

		var logger *slog.Logger
		switch l.Format {
		case TextFormat:
			logger = slog.New(slog.NewTextHandler(l.Output, l.Options))
		case JSONFormat:
			logger = slog.New(slog.NewJSONHandler(l.Output, l.Options))
		default:
			logger = slog.New(slog.NewTextHandler(l.Output, l.Options))
		}

		args := []any{"status", c.StatusCode()}
		if l.Flags&LOG_LATENCY != 0 {
			args = append(args, "latency", c.Latency().String())
		}
		args = append(args, "method", c.Request.Method, "path", c.Request.URL.Path)

		if l.Flags&LOG_IP != 0 {
			ipAddr, _ := c.IP()
			args = append(args, "ip", ipAddr)
		}

		if l.Flags&LOG_USERAGENT != 0 {
			args = append(args, "user_agent", c.Request.UserAgent())
		}

		if l.Callback != nil {
			args = l.Callback(c, args...)

			if len(args)%2 != 0 {
				panic("Callback must return an even number of arguments")
			}
		}

		logger.Info("", args...)
		return err
	}
}
