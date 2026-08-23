// Package sse provides server-sent events helpers for rex routers.
package sse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abiiranathan/rex"
)

// Event represents a Server-Sent Event.
type Event struct {
	ID      string        // Event ID for client tracking
	Event   string        // Event type/name
	Data    any           // Event payload
	Retry   time.Duration // Reconnection time (converted to milliseconds)
	Comment string        // Optional comment (for keepalive)
}

// StreamOptions configures the SSE stream behavior.
type StreamOptions struct {
	// Headers to set on the response (merged with defaults)
	Headers map[string]string

	// Retry interval sent to client (0 to omit)
	Retry time.Duration

	// Enable automatic keepalive comments
	Keepalive bool

	// Keepalive interval (default: 15s)
	KeepaliveInterval time.Duration

	// Custom error handler
	OnError func(error)

	// Custom close handler
	OnClose func()
}

// DefaultOptions returns sensible defaults for SSE streaming.
func DefaultOptions() *StreamOptions {
	return &StreamOptions{
		Headers:           make(map[string]string),
		Keepalive:         true,
		KeepaliveInterval: 15 * time.Second,
	}
}

var (
	errNotFlusher = errors.New("streaming not supported: response writer does not implement http.Flusher")

	// keepaliveBytes is the wire form of the default keepalive comment.
	keepaliveBytes = []byte(": keepalive\n\n")
)

// bufPool recycles event-encoding buffers so steady-state streaming performs
// no heap allocations for event framing.
var bufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

// Stream sends events from the channel to the client with enhanced error handling
// and configuration options.
func Stream(c *rex.Context, ch <-chan any, opts *StreamOptions) error {
	return stream(c, c.Response, c.SetHeader, ch, opts)
}

// StreamWithContext is a convenience wrapper that uses the provided context
// for cancellation instead of rex.Context.
func StreamWithContext(ctx context.Context, w http.ResponseWriter, ch <-chan any, opts *StreamOptions) error {
	setHeader := func(k, v string) { w.Header().Set(k, v) }
	return stream(ctx, w, setHeader, ch, opts)
}

// stream is the shared implementation behind Stream and StreamWithContext.
// The setHeader function applies response headers so both rex contexts and
// plain ResponseWriters are supported without duplicating the loop.
func stream(ctx context.Context, w http.ResponseWriter, setHeader func(k, v string), ch <-chan any, opts *StreamOptions) error {
	if opts == nil {
		opts = DefaultOptions()
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		return errNotFlusher
	}

	// Set default SSE headers.
	setHeader("Content-Type", "text/event-stream")
	setHeader("Cache-Control", "no-cache, no-transform")
	setHeader("Connection", "keep-alive")
	setHeader("X-Accel-Buffering", "no")

	// Apply custom headers (can override defaults).
	for k, v := range opts.Headers {
		setHeader(k, v)
	}

	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Send initial retry directive if configured.
	if opts.Retry > 0 {
		buf := bufPool.Get().(*bytes.Buffer)
		appendRetry(buf, opts.Retry)
		_, err := w.Write(buf.Bytes())
		putBuf(buf)
		if err != nil {
			return err
		}
		flusher.Flush()
	}

	// Setup keepalive ticker.
	var keepaliveTicker *time.Ticker
	var keepaliveCh <-chan time.Time

	if opts.Keepalive {
		interval := opts.KeepaliveInterval
		if interval <= 0 {
			interval = 15 * time.Second
		}
		keepaliveTicker = time.NewTicker(interval)
		defer keepaliveTicker.Stop()
		keepaliveCh = keepaliveTicker.C
	}

	// Cleanup handler.
	defer func() {
		if opts.OnClose != nil {
			opts.OnClose()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-keepaliveCh:
			// Send keepalive comment.
			if _, err := w.Write(keepaliveBytes); err != nil {
				if opts.OnError != nil {
					opts.OnError(err)
				}
				return err
			}
			flusher.Flush()

		case msg, ok := <-ch:
			if !ok {
				// Channel closed gracefully.
				return nil
			}

			if err := writeMessage(w, msg); err != nil {
				if opts.OnError != nil {
					opts.OnError(err)
				}
				return err
			}
			flusher.Flush()
		}
	}
}

func putBuf(buf *bytes.Buffer) {
	const maxRetainedSize = 8 << 10 // don't retain huge buffers
	if buf.Cap() > maxRetainedSize {
		return
	}
	buf.Reset()
	bufPool.Put(buf)
}

// writeMessage handles writing any message type to the SSE stream.
func writeMessage(w io.Writer, msg any) error {
	switch v := msg.(type) {
	case Event:
		return writeEvent(w, v)
	case *Event:
		return writeEvent(w, *v)
	default:
		return writeEventData(w, v)
	}
}

// writeEventData writes an encoded data field plus event terminator to w.
func writeEventData(w io.Writer, data any) error {
	buf := bufPool.Get().(*bytes.Buffer)
	defer putBuf(buf)

	buf.Reset()
	if err := appendData(buf, data); err != nil {
		return err
	}
	buf.WriteByte('\n') // end of event

	_, err := w.Write(buf.Bytes())
	return err
}

// writeEvent writes a structured Event to the stream.
// The entire event is encoded into a pooled buffer and written with a single
// Write call; framing performs no heap allocations in steady state.
func writeEvent(w io.Writer, event Event) error {
	buf := bufPool.Get().(*bytes.Buffer)
	defer putBuf(buf)

	if event.Comment != "" {
		buf.WriteString(": ")
		buf.WriteString(sanitizeField(event.Comment))
		buf.WriteString("\n\n")
	}

	if event.ID != "" {
		buf.WriteString("id: ")
		buf.WriteString(sanitizeField(event.ID))
		buf.WriteByte('\n')
	}

	if event.Event != "" {
		buf.WriteString("event: ")
		buf.WriteString(sanitizeField(event.Event))
		buf.WriteByte('\n')
	}

	if event.Retry > 0 {
		appendRetry(buf, event.Retry)
	}

	if err := appendData(buf, event.Data); err != nil {
		return err
	}
	buf.WriteByte('\n') // end of event

	_, err := w.Write(buf.Bytes())
	return err
}

// appendData encodes the data field into buf as one "data:" line per source
// line, per the SSE spec. String and []byte payloads skip JSON marshaling;
// other types are JSON-encoded once.
func appendData(buf *bytes.Buffer, data any) error {
	switch v := data.(type) {
	case nil:
		buf.WriteString("data: \n")
	case string:
		appendLines(buf, v)
	case []byte:
		appendByteLines(buf, v)
	default:
		jsonData, err := json.Marshal(v)
		if err != nil {
			return err
		}
		appendByteLines(buf, jsonData)
	}
	return nil
}

// appendLines writes s as "data:" lines, splitting on newlines without
// allocating an intermediate slice of lines.
func appendLines(buf *bytes.Buffer, s string) {
	for {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			buf.WriteString("data: ")
			buf.WriteString(s)
			buf.WriteByte('\n')
			return
		}
		buf.WriteString("data: ")
		buf.WriteString(s[:i])
		buf.WriteByte('\n')
		s = s[i+1:]
	}
}

// appendByteLines is appendLines for byte slices, avoiding a string copy.
func appendByteLines(buf *bytes.Buffer, b []byte) {
	for {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			buf.WriteString("data: ")
			buf.Write(b)
			buf.WriteByte('\n')
			return
		}
		buf.WriteString("data: ")
		buf.Write(b[:i])
		buf.WriteByte('\n')
		b = b[i+1:]
	}
}

// appendRetry encodes a retry field with duration converted to milliseconds.
func appendRetry(buf *bytes.Buffer, retry time.Duration) {
	var num [20]byte
	digits := strconv.AppendInt(num[:0], retry.Milliseconds(), 10)
	buf.WriteString("retry: ")
	buf.Write(digits)
	buf.WriteByte('\n')
}

// sanitizeField removes newlines from field values to prevent injection.
func sanitizeField(s string) string {
	if !strings.ContainsAny(s, "\n\r") {
		return s
	}
	var sb strings.Builder
	sb.Grow(len(s))
	for i := range len(s) {
		switch c := s[i]; c {
		case '\n', '\r':
		default:
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

// NewEvent creates a new Event with the given data.
func NewEvent(data any) Event {
	return Event{Data: data}
}

// WithID sets the event ID.
func (e Event) WithID(id string) Event {
	e.ID = id
	return e
}

// WithEvent sets the event type.
func (e Event) WithEvent(event string) Event {
	e.Event = event
	return e
}

// WithRetry sets the retry duration.
func (e Event) WithRetry(retry time.Duration) Event {
	e.Retry = retry
	return e
}

// WithComment sets a comment.
func (e Event) WithComment(comment string) Event {
	e.Comment = comment
	return e
}
