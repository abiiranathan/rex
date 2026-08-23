// Package logger provides request logging middleware for rex routers.
package logger

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"sync"

	"github.com/abiiranathan/rex"
)

// LogFormat is the format of the log output, compatible with the new slog package.
type LogFormat int

// LogFlags controls which request attributes are added to log output.
type LogFlags int8

const (
	TextFormat LogFormat = iota + 1 // This is the default format
	JSONFormat                      // Log in JSON format
)

const (
	LogIP LogFlags = 1 << iota
	LogLatency
	LogUserAgent
)

// StdLogFlags is the default set of log fields included by the middleware.
const StdLogFlags LogFlags = LogLatency | LogIP

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

	// Async delivers log records through a bounded background queue so log
	// writes never block request handling. When the queue is full, records
	// are dropped (see Dropped). Call Close during graceful shutdown to
	// flush queued records. Default: false (synchronous logging).
	Async bool

	// QueueSize is the number of records buffered when Async is enabled.
	// Default: rex.DefaultLogQueueSize.
	QueueSize int

	loggerOnce   sync.Once
	logger       *slog.Logger
	asyncHandler *rex.AsyncLogHandler
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

// builtin returns the middleware's slog.Logger, constructing it exactly once.
// The handler is built here (not per request) so the encoding machinery is
// reused across requests.
func (l *Config) builtin() *slog.Logger {
	l.loggerOnce.Do(func() {
		var handler slog.Handler
		switch l.Format {
		case JSONFormat:
			handler = slog.NewJSONHandler(l.Output, l.Options)
		default:
			handler = slog.NewTextHandler(l.Output, l.Options)
		}

		if l.Async {
			l.asyncHandler = rex.NewAsyncLogHandler(handler, l.QueueSize)
			handler = l.asyncHandler
		}
		l.logger = slog.New(handler)
	})
	return l.logger
}

// Close flushes and stops the background log queue when Async is enabled.
// Call it during graceful shutdown so buffered records are not lost. It is
// safe to call when async logging was never enabled or multiple times.
func (l *Config) Close() {
	if l.asyncHandler != nil {
		l.asyncHandler.Close()
	}
}

// Dropped returns the number of log records dropped because the async queue
// was full. Returns 0 when Async is not enabled.
func (l *Config) Dropped() uint64 {
	if l.asyncHandler == nil {
		return 0
	}
	return l.asyncHandler.Dropped()
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

		args := []any{"status", c.StatusCode()}
		if l.Flags&LogLatency != 0 {
			args = append(args, "latency", slog.DurationValue(c.Latency()))
		}
		args = append(args, "method", c.Request.Method, "path", c.Request.URL.Path)

		if l.Flags&LogIP != 0 {
			ipAddr, _ := c.IP()
			args = append(args, "ip", ipAddr)
		}

		if l.Flags&LogUserAgent != 0 {
			args = append(args, "user_agent", c.Request.UserAgent())
		}

		if l.Callback != nil {
			args = l.Callback(c, args...)

			if len(args)%2 != 0 {
				panic("Callback must return an even number of arguments")
			}
		}

		l.builtin().Info("", args...)
		return err
	}
}
