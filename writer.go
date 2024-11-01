package rex

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// ResponseWriter wraps http.ResponseWriter with additional functionality
type ResponseWriter struct {
	writer     http.ResponseWriter // The original http.ResponseWriter
	status     int                 // The status code of the response
	size       int                 // The size of the response sent so far
	statusSent bool                // If the status has been sent
	skipBody   bool                // If its a HEAD request, we should skip the body
	latency    time.Duration       // The latency of the response.
}

// ResponseWriter interface
func (rw *ResponseWriter) Header() http.Header {
	return rw.writer.Header()
}

// WriteHeader writes the status code to the response.
// Calling the method more than once will have no effect.
func (w *ResponseWriter) WriteHeader(status int) {
	if w.statusSent {
		return
	}
	w.status = status
	w.writer.WriteHeader(status)
	w.statusSent = true
}

// Write writes the data to the connection as part of an HTTP reply.
// Satisfies the io.Writer interface.
// Calling this with a HEAD request will only write the headers if they haven't been written yet.
func (w *ResponseWriter) Write(b []byte) (int, error) {
	if !w.statusSent {
		w.WriteHeader(http.StatusOK)
	}

	// If it's a HEAD request, we should skip the body
	if w.skipBody {
		return len(b), nil
	}

	size, err := w.writer.Write(b)
	w.size += size
	return size, err
}

func (w *ResponseWriter) Status() int {
	return w.status
}

func (w *ResponseWriter) Size() int {
	return w.size
}

// Implements the http.Flusher interface to allow an HTTP handler to flush buffered data to the client.
// This is useful for chunked responses and server-sent events.
func (w *ResponseWriter) Flush() {
	if f, ok := w.writer.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack lets the caller take over the connection.
func (w *ResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.writer.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("hijacking not supported")
}

// ReadFrom reads data from an io.Reader and writes it to the connection.
// All data is written in a single call to Write, so the data should be buffered.
// The return value is the number of bytes written and an error, if any.
func (w *ResponseWriter) ReadFrom(r io.Reader) (n int64, err error) {
	if !w.statusSent {
		// The status will be StatusOK if WriteHeader has not been called yet
		w.WriteHeader(http.StatusOK)
	}

	n, err = io.Copy(w.writer, r)
	w.size += int(n)
	return
}

// Satisfy http.ResponseController support (Go 1.20+)
// More about ResponseController: https://go.dev/ref/spec#ResponseController
func (w *ResponseWriter) Unwrap() http.ResponseWriter {
	return w.writer
}
