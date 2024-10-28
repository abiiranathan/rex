package rex

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
)

// ResponseWriter wraps http.ResponseWriter with additional functionality
type ResponseWriter struct {
	writer     http.ResponseWriter // The original http.ResponseWriter
	status     int                 // The status code of the response
	size       int                 // The size of the response sent so far
	statusSent bool                // If the status has been sent
	skipBody   bool                // If its a HEAD request, we should skip the body
}

// ResponseWriter interface
func (rw *ResponseWriter) Header() http.Header {
	return rw.writer.Header()
}

func (w *ResponseWriter) WriteHeader(status int) {
	if w.statusSent {
		return
	}
	w.status = status
	w.writer.WriteHeader(status)
	w.statusSent = true
}

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

// Implement additional interfaces
func (w *ResponseWriter) Flush() {
	if f, ok := w.writer.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *ResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.writer.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("hijacking not supported")
}

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
func (w *ResponseWriter) Unwrap() http.ResponseWriter {
	return w.writer
}
