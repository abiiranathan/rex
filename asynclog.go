package rex

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
)

// DefaultLogQueueSize is the default number of log records that can be
// buffered by an AsyncLogHandler before records start being dropped.
const DefaultLogQueueSize = 4096

// asyncCore holds the state shared between all handlers derived from a single
// AsyncLogHandler (via WithAttrs/WithGroup).
type asyncCore struct {
	mu     sync.Mutex // serializes enqueue against shutdown, preventing lost records
	closed bool

	queue     chan slog.Record
	quit      chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once
	dropped   atomic.Uint64
}

func (core *asyncCore) enqueue(r slog.Record) {
	core.mu.Lock()
	if core.closed {
		// Shutting down: do not accept new work.
		core.mu.Unlock()
		core.dropped.Add(1)
		return
	}
	select {
	case core.queue <- r:
		core.mu.Unlock()
		return
	default:
		// Queue full: drop rather than block the caller.
		core.mu.Unlock()
		core.dropped.Add(1)
	}
}

// AsyncLogHandler is a thread-safe slog.Handler that delivers log records to
// a background goroutine through a bounded queue, so logging never blocks the
// request path.
//
// Behavior:
//   - Records are enqueued without blocking. If the queue is full, the record
//     is dropped and counted (see Dropped) — back-pressuring request handling
//     to write logs is usually worse than losing a log line.
//   - A single background goroutine drains the queue in FIFO order, preserving
//     per-record ordering.
//   - Close stops accepting new records, drains everything already queued,
//     then returns. Call it during graceful shutdown so no logs are lost.
//
// The zero value is not usable; construct with NewAsyncLogHandler.
type AsyncLogHandler struct {
	core    *asyncCore
	handler slog.Handler // underlying synchronous handler (owned exclusively by the worker)
}

// NewAsyncLogHandler wraps handler with asynchronous delivery using a queue of
// the given size. If queueSize <= 0, DefaultLogQueueSize is used.
func NewAsyncLogHandler(handler slog.Handler, queueSize int) *AsyncLogHandler {
	if queueSize <= 0 {
		queueSize = DefaultLogQueueSize
	}
	core := &asyncCore{
		queue: make(chan slog.Record, queueSize),
		quit:  make(chan struct{}),
	}
	a := &AsyncLogHandler{core: core, handler: handler}
	core.wg.Add(1)
	go core.run(a.handler)
	return a
}

// run drains the queue until Close is called, then drains any remaining
// buffered records before returning.
func (core *asyncCore) run(handler slog.Handler) {
	defer core.wg.Done()
	for {
		select {
		case r := <-core.queue:
			_ = handler.Handle(context.Background(), r)
		case <-core.quit:
			// Drain whatever was already accepted before shutdown.
			for {
				select {
				case r := <-core.queue:
					_ = handler.Handle(context.Background(), r)
				default:
					return
				}
			}
		}
	}
}

// Enabled implements slog.Handler.
func (a *AsyncLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return a.handler.Enabled(ctx, level)
}

// Handle enqueues the record for asynchronous processing.
// It never blocks: when the queue is full the record is dropped.
func (a *AsyncLogHandler) Handle(_ context.Context, r slog.Record) error {
	a.core.enqueue(r.Clone())
	return nil
}

// WithAttrs implements slog.Handler. The derived handler shares the same
// queue and lifecycle as the original.
func (a *AsyncLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &AsyncLogHandler{core: a.core, handler: a.handler.WithAttrs(attrs)}
}

// WithGroup implements slog.Handler. The derived handler shares the same
// queue and lifecycle as the original.
func (a *AsyncLogHandler) WithGroup(name string) slog.Handler {
	return &AsyncLogHandler{core: a.core, handler: a.handler.WithGroup(name)}
}

// Close stops accepting new records and waits until all previously accepted
// records have been written. It is safe to call multiple times and from
// multiple goroutines. Records logged after Close are counted in Dropped.
func (a *AsyncLogHandler) Close() {
	a.core.closeOnce.Do(func() {
		a.core.mu.Lock()
		a.core.closed = true
		a.core.mu.Unlock()

		close(a.core.quit)
	})
	a.core.wg.Wait()
}

// Dropped returns the number of records dropped because the queue was full.
func (a *AsyncLogHandler) Dropped() uint64 {
	return a.core.dropped.Load()
}

// Queued returns the approximate number of records waiting to be written.
func (a *AsyncLogHandler) Queued() int {
	return len(a.core.queue)
}
