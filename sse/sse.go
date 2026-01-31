package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

// Stream sends events from the channel to the client with enhanced error handling
// and configuration options.
func Stream(c *rex.Context, ch <-chan any, opts *StreamOptions) error {
	if opts == nil {
		opts = DefaultOptions()
	}

	w := c.Response
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported: response writer does not implement http.Flusher")
	}

	// Set default SSE headers
	defaultHeaders := map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-cache, no-transform",
		"Connection":        "keep-alive",
		"X-Accel-Buffering": "no", // Disable nginx buffering
	}

	// Apply default headers
	for k, v := range defaultHeaders {
		c.SetHeader(k, v)
	}

	// Apply custom headers (can override defaults)
	for k, v := range opts.Headers {
		c.SetHeader(k, v)
	}

	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Send initial retry directive if configured
	if opts.Retry > 0 {
		if err := writeRetry(w, opts.Retry); err != nil {
			return err
		}
		flusher.Flush()
	}

	// Setup keepalive ticker
	var keepaliveTicker *time.Ticker
	var keepaliveCh <-chan time.Time

	if opts.Keepalive {
		keepaliveTicker = time.NewTicker(opts.KeepaliveInterval)
		defer keepaliveTicker.Stop()
		keepaliveCh = keepaliveTicker.C
	}

	// Cleanup handler
	defer func() {
		if opts.OnClose != nil {
			opts.OnClose()
		}
	}()

	for {
		select {
		case <-c.Done():
			return c.Err()

		case <-keepaliveCh:
			// Send keepalive comment
			if err := writeComment(w, "keepalive"); err != nil {
				if opts.OnError != nil {
					opts.OnError(err)
				}
				return err
			}
			flusher.Flush()

		case msg, ok := <-ch:
			if !ok {
				// Channel closed gracefully
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

// StreamWithContext is a convenience wrapper that uses the provided context
// for cancellation instead of rex.Context.
func StreamWithContext(ctx context.Context, w http.ResponseWriter, ch <-chan any, opts *StreamOptions) error {
	if opts == nil {
		opts = DefaultOptions()
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported: response writer does not implement http.Flusher")
	}

	// Set headers
	defaultHeaders := map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-cache, no-transform",
		"Connection":        "keep-alive",
		"X-Accel-Buffering": "no",
	}

	for k, v := range defaultHeaders {
		w.Header().Set(k, v)
	}
	for k, v := range opts.Headers {
		w.Header().Set(k, v)
	}

	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	if opts.Retry > 0 {
		if err := writeRetry(w, opts.Retry); err != nil {
			return err
		}
		flusher.Flush()
	}

	var keepaliveTicker *time.Ticker
	var keepaliveCh <-chan time.Time

	if opts.Keepalive {
		keepaliveTicker = time.NewTicker(opts.KeepaliveInterval)
		defer keepaliveTicker.Stop()
		keepaliveCh = keepaliveTicker.C
	}

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
			if err := writeComment(w, "keepalive"); err != nil {
				if opts.OnError != nil {
					opts.OnError(err)
				}
				return err
			}
			flusher.Flush()

		case msg, ok := <-ch:
			if !ok {
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

// writeMessage handles writing any message type to the SSE stream.
func writeMessage(w io.Writer, msg any) error {
	switch v := msg.(type) {
	case Event:
		return writeEvent(w, v)
	case *Event:
		return writeEvent(w, *v)
	case string:
		return writeData(w, v)
	case []byte:
		return writeData(w, v)
	default:
		return writeData(w, v)
	}
}

// writeEvent writes a structured Event to the stream.
func writeEvent(w io.Writer, event Event) error {
	if event.Comment != "" {
		if err := writeComment(w, event.Comment); err != nil {
			return err
		}
	}

	if event.ID != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", sanitizeField(event.ID)); err != nil {
			return err
		}
	}

	if event.Event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", sanitizeField(event.Event)); err != nil {
			return err
		}
	}

	if event.Retry > 0 {
		if err := writeRetry(w, event.Retry); err != nil {
			return err
		}
	}

	return writeData(w, event.Data)
}

// writeData writes the data field, handling multiline data correctly.
func writeData(w io.Writer, data any) error {
	var content string

	switch v := data.(type) {
	case string:
		content = v
	case []byte:
		content = string(v)
	case nil:
		content = ""
	default:
		jsonData, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("failed to marshal data: %w", err)
		}
		content = string(jsonData)
	}

	// Handle multiline data correctly per SSE spec
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}

	// End of event
	_, err := fmt.Fprint(w, "\n")
	return err
}

// writeComment writes a comment line (for keepalive or debugging).
func writeComment(w io.Writer, comment string) error {
	_, err := fmt.Fprintf(w, ": %s\n\n", comment)
	return err
}

// writeRetry writes a retry field with duration converted to milliseconds.
func writeRetry(w io.Writer, retry time.Duration) error {
	ms := retry.Milliseconds()
	_, err := fmt.Fprintf(w, "retry: %d\n", ms)
	return err
}

// sanitizeField removes newlines from field values to prevent injection.
func sanitizeField(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\n", ""), "\r", "")
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
