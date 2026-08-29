package rex

import (
	"bytes"
	"context"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
)

// TestAsyncLogHandlerDeliversAllRecords verifies that every record accepted
// before Close is written by the background worker.
func TestAsyncLogHandlerDeliversAllRecords(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	syncHandler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	async := NewAsyncLogHandler(syncHandler, 128)

	logger := slog.New(async)
	const n = 500
	for i := range n {
		logger.Info("hello", "i", i)
	}

	async.Close()

	written := strings.Count(buf.String(), "hello")
	dropped := int(async.Dropped())

	// Every record is either written or explicitly dropped under burst load.
	if written+dropped != n {
		t.Fatalf("expected %d accounted records, got %d written + %d dropped", n, written, dropped)
	}
}

func TestAsyncLogHandlerCloseIsIdempotent(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	async := NewAsyncLogHandler(slog.NewTextHandler(&buf, nil), 16)
	async.Close()
	async.Close()
	async.Close()

	slog.New(async).Info("after close")
	async.Close()

	// Records after Close are dropped, not panicked on.
	if async.Dropped() != 1 {
		t.Fatalf("expected 1 dropped record, got %d", async.Dropped())
	}
}

func TestAsyncLogHandlerConcurrentUse(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	async := NewAsyncLogHandler(slog.NewJSONHandler(&buf, nil), 1024)

	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			logger := slog.New(async.WithGroup("worker"))
			for i := range 100 {
				logger.Info("msg", "g", g, "i", i)
			}
		})
	}
	wg.Wait()
	async.Close()

	if got := strings.Count(buf.String(), `"msg":"msg"`); got != 800 {
		t.Fatalf("expected 800 records, got %d", got)
	}
}

func TestAsyncLogHandlerDropOnFullQueue(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	// A blocking handler keeps the worker busy so the queue fills up.
	blocking := handlerFunc(func(_ context.Context, r slog.Record) error {
		<-release
		return nil
	})

	async := NewAsyncLogHandler(blocking, 4)
	logger := slog.New(async)

	// Fill the queue (4) and overflow it a few times.
	for range 20 {
		logger.Info("x")
	}

	if async.Queued() > 5 {
		t.Fatalf("queue should be bounded, got %d queued", async.Queued())
	}

	close(release)
	async.Close()

	if async.Dropped() == 0 {
		t.Fatal("expected some records to be dropped")
	}
}

type handlerFunc func(context.Context, slog.Record) error

func (f handlerFunc) Enabled(context.Context, slog.Level) bool        { return true }
func (f handlerFunc) Handle(ctx context.Context, r slog.Record) error { return f(ctx, r) }
func (f handlerFunc) WithAttrs([]slog.Attr) slog.Handler              { return f }
func (f handlerFunc) WithGroup(string) slog.Handler                   { return f }

// TestRouterAsyncLogging verifies the WithAsyncLogging router option end to end.
func TestRouterAsyncLogging(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		r := NewRouter(WithAsyncLogging(64))
		r.GET("/logged", func(c *Context) error { return c.String("ok") })

		req := httptest.NewRequest("GET", "/logged", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		// Wait until every goroutine in the bubble is durably blocked; the
		// background worker only blocks once the queue is fully drained.
		synctest.Wait()
		r.CloseLogQueue()

		if dropped := r.LogQueueDropped(); dropped != 0 {
			t.Fatalf("expected 0 dropped, got %d", dropped)
		}

		// Safe to call again / without async enabled.
		r.CloseLogQueue()
		NewRouter().CloseLogQueue()
	})
}
