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
	writer     http.ResponseWriter
	status     int
	size       int
	statusSent bool
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
