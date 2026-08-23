package rex_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abiiranathan/rex"
	"github.com/abiiranathan/rex/middleware/logger"
)

// noOpWriter is a minimal ResponseWriter so benchmarks measure rex's overhead,
// not httptest.NewRecorder's per-iteration header clone and buffer growth.
type noOpWriter struct {
	header http.Header
	status int
	size   int
}

func newNoOpWriter() *noOpWriter { return &noOpWriter{header: make(http.Header, 6)} }

func (w *noOpWriter) Header() http.Header    { return w.header }
func (w *noOpWriter) WriteHeader(status int) { w.status = status }
func (w *noOpWriter) Write(b []byte) (int, error) {
	w.size += len(b)
	return len(b), nil
}

var _ http.ResponseWriter = (*noOpWriter)(nil)

const benchBody = "Hello World!"

func BenchmarkRawMux(b *testing.B) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /benchmark", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(benchBody))
	})

	req := httptest.NewRequest(http.MethodGet, "/benchmark", nil)
	b.ReportAllocs()
	for b.Loop() {
		w := newNoOpWriter()
		mux.ServeHTTP(w, req)
	}
}

// BenchmarkRexRouter measures the minimal rex request path.
func BenchmarkRexRouter(b *testing.B) {
	r := rex.NewRouter()
	r.GET("/benchmark", func(c *rex.Context) error {
		return c.String(benchBody)
	})

	req := httptest.NewRequest(http.MethodGet, "/benchmark", nil)
	b.ReportAllocs()
	for b.Loop() {
		w := newNoOpWriter()
		r.ServeHTTP(w, req)
	}
}

// BenchmarkRexRouterWithMiddleware measures the path with a passthrough middleware.
func BenchmarkRexRouterWithMiddleware(b *testing.B) {
	r := rex.NewRouter()
	r.Use(func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error { return next(c) }
	})
	r.GET("/benchmark", func(c *rex.Context) error {
		return c.String(benchBody)
	})

	req := httptest.NewRequest(http.MethodGet, "/benchmark", nil)
	b.ReportAllocs()
	for b.Loop() {
		w := newNoOpWriter()
		r.ServeHTTP(w, req)
	}
}

// BenchmarkRexQueryAccess measures repeated query access on one request
// (previously each call re-parsed the query string).
func BenchmarkRexQueryAccess(b *testing.B) {
	r := rex.NewRouter()
	r.GET("/q", func(c *rex.Context) error {
		page := c.QueryInt("page", 1)
		size := c.QueryInt("size", 20)
		sort := c.Query("sort")
		_, _, _ = page, size, sort
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/q?page=1&size=20&sort=name", nil)
	w := newNoOpWriter()
	b.ReportAllocs()
	for b.Loop() {
		r.ServeHTTP(w, req)
	}
}

// BenchmarkRexJSON measures a JSON response round trip.
func BenchmarkRexJSON(b *testing.B) {
	type payload struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	r := rex.NewRouter()
	r.GET("/json", func(c *rex.Context) error {
		return c.JSON(payload{ID: 1, Name: "rex"})
	})

	req := httptest.NewRequest(http.MethodGet, "/json", nil)
	b.ReportAllocs()
	for b.Loop() {
		w := newNoOpWriter()
		r.ServeHTTP(w, req)
	}
}

// BenchmarkRexAsyncLogging measures the request path when the router's
// internal logging goes through the async queue (log write cost excluded).
func BenchmarkRexAsyncLogging(b *testing.B) {
	r := rex.NewRouter(rex.WithAsyncLogging(0))
	defer r.CloseLogQueue()
	r.GET("/benchmark", func(c *rex.Context) error {
		return c.String(benchBody)
	})

	req := httptest.NewRequest(http.MethodGet, "/benchmark", nil)
	b.ReportAllocs()
	for b.Loop() {
		w := newNoOpWriter()
		r.ServeHTTP(w, req)
	}
}

// BenchmarkLoggerMiddlewareSync measures a request through the logger
// middleware with synchronous logging.
func BenchmarkLoggerMiddlewareSync(b *testing.B) {
	cfg := logger.New(&logger.Config{
		Output: io.Discard,
		Format: logger.TextFormat,
		Flags:  logger.StdLogFlags,
	})
	r := rex.NewRouter()
	r.Use(cfg)
	r.GET("/benchmark", func(c *rex.Context) error { return c.String(benchBody) })

	req := httptest.NewRequest(http.MethodGet, "/benchmark", nil)
	b.ReportAllocs()
	for b.Loop() {
		w := newNoOpWriter()
		r.ServeHTTP(w, req)
	}
}

// BenchmarkLoggerMiddlewareAsync measures the same path with the async queue
// (the background writer cost is not attributed to the request).
func BenchmarkLoggerMiddlewareAsync(b *testing.B) {
	conf := &logger.Config{
		Output:    io.Discard,
		Format:    logger.TextFormat,
		Flags:     logger.StdLogFlags,
		Async:     true,
		QueueSize: 4096,
	}
	defer conf.Close()

	r := rex.NewRouter()
	r.Use(logger.New(conf))
	r.GET("/benchmark", func(c *rex.Context) error { return c.String(benchBody) })

	req := httptest.NewRequest(http.MethodGet, "/benchmark", nil)
	b.ReportAllocs()
	for b.Loop() {
		w := newNoOpWriter()
		r.ServeHTTP(w, req)
	}
}
