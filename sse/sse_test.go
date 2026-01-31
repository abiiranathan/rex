package sse

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abiiranathan/rex"
)

func TestStream_BasicMessages(t *testing.T) {
	r := rex.NewRouter()
	r.GET("/stream", func(c *rex.Context) error {
		ch := make(chan any, 3)
		ch <- "message 1"
		ch <- Event{Event: "ping", Data: "message 2"}
		ch <- map[string]string{"foo": "bar"}
		close(ch)
		return Stream(c, ch, nil)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if contentType := w.Header().Get("Content-Type"); contentType != "text/event-stream" {
		t.Errorf("Expected Content-Type text/event-stream, got %s", contentType)
	}

	body := w.Body.String()
	expectedLines := []string{
		"data: message 1\n\n",
		"event: ping\ndata: message 2\n\n",
		"data: {\"foo\":\"bar\"}\n\n",
	}

	for _, line := range expectedLines {
		if !strings.Contains(body, line) {
			t.Errorf("Expected body to contain %q, got body:\n%s", line, body)
		}
	}
}

func TestStream_WithOptions(t *testing.T) {
	r := rex.NewRouter()
	r.GET("/stream", func(c *rex.Context) error {
		ch := make(chan any, 1)
		ch <- "test message"
		close(ch)

		opts := &StreamOptions{
			Headers: map[string]string{
				"Access-Control-Allow-Origin": "*",
			},
			Retry:     5 * time.Second,
			Keepalive: false,
		}
		return Stream(c, ch, opts)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check custom header
	if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Errorf("Expected Access-Control-Allow-Origin *, got %s", origin)
	}

	// Check retry directive
	body := w.Body.String()
	if !strings.Contains(body, "retry: 5000\n") {
		t.Errorf("Expected retry directive, got body:\n%s", body)
	}
}

func TestStream_EventStructure(t *testing.T) {
	r := rex.NewRouter()
	r.GET("/stream", func(c *rex.Context) error {
		ch := make(chan any, 1)
		ch <- Event{
			ID:      "123",
			Event:   "update",
			Data:    map[string]int{"count": 42},
			Retry:   3 * time.Second,
			Comment: "test comment",
		}
		close(ch)
		return Stream(c, ch, nil)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream", nil)
	r.ServeHTTP(w, req)

	body := w.Body.String()

	// Check all event fields
	expectedParts := []string{
		": test comment\n\n",
		"id: 123\n",
		"event: update\n",
		"retry: 3000\n",
		"data: {\"count\":42}\n\n",
	}

	for _, part := range expectedParts {
		if !strings.Contains(body, part) {
			t.Errorf("Expected body to contain %q, got body:\n%s", part, body)
		}
	}
}

func TestStream_MultilineData(t *testing.T) {
	r := rex.NewRouter()
	r.GET("/stream", func(c *rex.Context) error {
		ch := make(chan any, 1)
		ch <- "line 1\nline 2\nline 3"
		close(ch)
		return Stream(c, ch, nil)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream", nil)
	r.ServeHTTP(w, req)

	body := w.Body.String()

	// Each line should be prefixed with "data: "
	expected := "data: line 1\ndata: line 2\ndata: line 3\n\n"
	if !strings.Contains(body, expected) {
		t.Errorf("Expected multiline data format, got body:\n%s", body)
	}
}

func TestStream_ContextCancel(t *testing.T) {
	r := rex.NewRouter()
	r.GET("/stream", func(c *rex.Context) error {
		ch := make(chan any)
		// Don't close channel, let context cancel
		return Stream(c, ch, nil)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream", nil)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)

	done := make(chan bool)
	go func() {
		r.ServeHTTP(w, req)
		done <- true
	}()

	// Cancel immediately
	cancel()

	select {
	case <-done:
		// Test passed - stream exited on context cancel
	case <-time.After(1 * time.Second):
		t.Fatal("Stream did not exit on context cancel")
	}
}

func TestStream_ChannelClose(t *testing.T) {
	r := rex.NewRouter()
	streamEnded := make(chan bool)

	r.GET("/stream", func(c *rex.Context) error {
		ch := make(chan any, 2)
		ch <- "message 1"
		ch <- "message 2"
		close(ch) // Close channel to end stream

		err := Stream(c, ch, nil)
		streamEnded <- true
		return err
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream", nil)

	go r.ServeHTTP(w, req)

	select {
	case <-streamEnded:
		// Test passed
	case <-time.After(1 * time.Second):
		t.Fatal("Stream did not exit on channel close")
	}
}

func TestStream_ErrorCallback(t *testing.T) {
	var errorCalled bool
	var mu sync.Mutex

	opts := &StreamOptions{
		OnError: func(err error) {
			mu.Lock()
			errorCalled = true
			mu.Unlock()
		},
		Keepalive: false,
	}

	// Create a failing writer
	failingWriter := &failingResponseWriter{
		ResponseRecorder: httptest.NewRecorder(),
		failAfter:        1,
	}

	ch := make(chan any, 3)
	ch <- "message 1"
	ch <- "message 2" // This should trigger error
	close(ch)

	ctx := rex.NewContext(failingWriter, httptest.NewRequest("GET", "/stream", nil), nil)
	_ = Stream(ctx, ch, opts)

	mu.Lock()
	defer mu.Unlock()
	if !errorCalled {
		t.Error("Expected error callback to be called")
	}
}

func TestStream_OnCloseCallback(t *testing.T) {
	var closeCalled bool
	var mu sync.Mutex

	opts := &StreamOptions{
		OnClose: func() {
			mu.Lock()
			closeCalled = true
			mu.Unlock()
		},
		Keepalive: false,
	}

	r := rex.NewRouter()
	r.GET("/stream", func(c *rex.Context) error {
		ch := make(chan any, 1)
		ch <- "test"
		close(ch)
		return Stream(c, ch, opts)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream", nil)
	r.ServeHTTP(w, req)

	mu.Lock()
	defer mu.Unlock()
	if !closeCalled {
		t.Error("Expected close callback to be called")
	}
}

func TestStreamWithContext(t *testing.T) {
	w := httptest.NewRecorder()
	ch := make(chan any, 2)
	ch <- "message 1"
	ch <- Event{Event: "test", Data: "message 2"}
	close(ch)

	ctx := context.Background()
	err := StreamWithContext(ctx, w, ch, nil)
	if err != nil {
		t.Fatalf("StreamWithContext failed: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "data: message 1\n\n") {
		t.Errorf("Expected message 1 in body, got:\n%s", body)
	}
	if !strings.Contains(body, "event: test\n") {
		t.Errorf("Expected event: test in body, got:\n%s", body)
	}
}

func TestStreamWithContext_Cancel(t *testing.T) {
	w := httptest.NewRecorder()
	ch := make(chan any) // Don't close, will cancel context

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error)
	go func() {
		done <- StreamWithContext(ctx, w, ch, nil)
	}()

	// Cancel after short delay
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled error, got: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("StreamWithContext did not exit on context cancel")
	}
}

func TestNewEvent_FluentAPI(t *testing.T) {
	event := NewEvent("test data").
		WithID("evt-123").
		WithEvent("notification").
		WithRetry(5 * time.Second).
		WithComment("important")

	if event.ID != "evt-123" {
		t.Errorf("Expected ID evt-123, got %s", event.ID)
	}
	if event.Event != "notification" {
		t.Errorf("Expected Event notification, got %s", event.Event)
	}
	if event.Retry != 5*time.Second {
		t.Errorf("Expected Retry 5s, got %v", event.Retry)
	}
	if event.Comment != "important" {
		t.Errorf("Expected Comment important, got %s", event.Comment)
	}
	if event.Data.(string) != "test data" {
		t.Errorf("Expected Data 'test data', got %v", event.Data)
	}
}

func TestSanitizeField(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"normal text", "normal text"},
		{"text\nwith\nnewlines", "textwithnewlines"},
		{"text\rwith\r\ncarriage", "textwithcarriage"},
		{"mixed\n\r\nchars", "mixedchars"},
	}

	for _, tt := range tests {
		result := sanitizeField(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeField(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.Keepalive != true {
		t.Error("Expected Keepalive to be true by default")
	}
	if opts.KeepaliveInterval != 15*time.Second {
		t.Errorf("Expected KeepaliveInterval 15s, got %v", opts.KeepaliveInterval)
	}
	if opts.Headers == nil {
		t.Error("Expected Headers to be initialized")
	}
}

func TestStream_ByteSliceData(t *testing.T) {
	r := rex.NewRouter()
	r.GET("/stream", func(c *rex.Context) error {
		ch := make(chan any, 1)
		ch <- []byte("raw bytes")
		close(ch)
		return Stream(c, ch, nil)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream", nil)
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "data: raw bytes\n\n") {
		t.Errorf("Expected byte slice to be written, got body:\n%s", body)
	}
}

func TestStream_NilData(t *testing.T) {
	r := rex.NewRouter()
	r.GET("/stream", func(c *rex.Context) error {
		ch := make(chan any, 1)
		ch <- Event{Event: "empty", Data: nil}
		close(ch)
		return Stream(c, ch, nil)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream", nil)
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "event: empty\n") {
		t.Errorf("Expected event field, got body:\n%s", body)
	}
	if !strings.Contains(body, "data: \n\n") {
		t.Errorf("Expected empty data field, got body:\n%s", body)
	}
}

func TestStream_ComplexJSON(t *testing.T) {
	type ComplexData struct {
		Name   string            `json:"name"`
		Values []int             `json:"values"`
		Meta   map[string]string `json:"meta"`
	}

	r := rex.NewRouter()
	r.GET("/stream", func(c *rex.Context) error {
		ch := make(chan any, 1)
		ch <- ComplexData{
			Name:   "test",
			Values: []int{1, 2, 3},
			Meta:   map[string]string{"key": "value"},
		}
		close(ch)
		return Stream(c, ch, nil)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream", nil)
	r.ServeHTTP(w, req)

	body := w.Body.String()

	// Extract JSON from data field
	lines := strings.Split(body, "\n")
	var jsonData string
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			jsonData = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	var result ComplexData
	if err := json.Unmarshal([]byte(jsonData), &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if result.Name != "test" {
		t.Errorf("Expected name 'test', got %s", result.Name)
	}
	if len(result.Values) != 3 {
		t.Errorf("Expected 3 values, got %d", len(result.Values))
	}
}

func TestStream_PointerEvent(t *testing.T) {
	r := rex.NewRouter()
	r.GET("/stream", func(c *rex.Context) error {
		ch := make(chan any, 1)
		ch <- &Event{Event: "pointer", Data: "test"}
		close(ch)
		return Stream(c, ch, nil)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream", nil)
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "event: pointer\n") {
		t.Errorf("Expected pointer event to work, got body:\n%s", body)
	}
}

func TestStream_Headers(t *testing.T) {
	r := rex.NewRouter()
	r.GET("/stream", func(c *rex.Context) error {
		ch := make(chan any, 1)
		ch <- "test"
		close(ch)
		return Stream(c, ch, nil)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream", nil)
	r.ServeHTTP(w, req)

	headers := map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-cache, no-transform",
		"Connection":        "keep-alive",
		"X-Accel-Buffering": "no",
	}

	for key, expected := range headers {
		if got := w.Header().Get(key); got != expected {
			t.Errorf("Header %s = %q, want %q", key, got, expected)
		}
	}
}

// Helper type for testing error handling
type failingResponseWriter struct {
	*httptest.ResponseRecorder
	writeCount int
	failAfter  int
}

func (f *failingResponseWriter) Write(b []byte) (int, error) {
	f.writeCount++
	if f.writeCount > f.failAfter {
		return 0, http.ErrHandlerTimeout
	}
	return f.ResponseRecorder.Write(b)
}

func (f *failingResponseWriter) Flush() {}

// Benchmark tests
func BenchmarkStream_SimpleMessages(b *testing.B) {
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/stream", nil)
		ctx := rex.NewContext(w, req, nil)

		ch := make(chan any, 100)
		for j := 0; j < 100; j++ {
			ch <- "message"
		}
		close(ch)

		_ = Stream(ctx, ch, &StreamOptions{Keepalive: false})
	}
}

func BenchmarkStream_JSONMessages(b *testing.B) {
	type Message struct {
		ID   int    `json:"id"`
		Text string `json:"text"`
	}

	for b.Loop() {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/stream", nil)
		ctx := rex.NewContext(w, req, nil)

		ch := make(chan any, 100)
		for j := range 100 {
			ch <- Message{ID: j, Text: "message"}
		}
		close(ch)

		_ = Stream(ctx, ch, &StreamOptions{Keepalive: false})
	}
}
