package test

import (
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/abiiranathan/rex"
)

// Test executes the request against the router and returns the response.
// It mimics the behavior of the real server by using the router's ServeHTTP method.
func Test(r *rex.Router, req *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Result(), nil
}

// TestHandler executes a specific handler in isolation with a temporary router context.
// It allows testing handlers that require router features like templates without registering a route.
//
// If the handler returns an error, it is returned directly. The response in the recorder
// contains whatever was written to the ResponseWriter up to that point.
//
// Example:
//
//	func TestMyHandler(t *testing.T) {
//	    req := httptest.NewRequest("GET", "/", nil)
//	    resp, err := test.TestHandler(MyHandler, req, rex.WithTemplates(tmpl))
//	    // ...
//	}
func TestHandler(h rex.HandlerFunc, req *http.Request, options ...rex.RouterOption) (*http.Response, error) {
	// Create a temporary router to provide context (templates, etc.)
	r := rex.NewRouter(options...)

	w := httptest.NewRecorder()

	// Initialize context
	ctx := r.InitContext(w, req)
	defer r.PutContext(ctx)

	// Execute handler
	err := h(ctx)
	if err != nil {
		return w.Result(), err
	}

	return w.Result(), nil
}

// NewRequest creates a new HTTP request.
// It's a wrapper around httptest.NewRequest but handy for consistency.
func NewRequest(method, target string, body io.Reader) *http.Request {
	return httptest.NewRequest(method, target, body)
}

// SetPathValue sets a path value on the request.
// This is useful for testing handlers that rely on path parameters (c.Param).
// Note: This requires Go 1.22+.
func SetPathValue(req *http.Request, key, value string) {
	req.SetPathValue(key, value)
}

// TemplateFromString creates a *template.Template from a string map.
// Keys are template names (e.g. "index.html") and values are the content.
// Useful for quick tests without creating files.
func TemplateFromString(templates map[string]string) *template.Template {
	if len(templates) == 0 {
		return nil
	}

	var root *template.Template

	for name, content := range templates {
		if root == nil {
			root = template.Must(template.New(name).Parse(content))
		} else {
			_ = template.Must(root.New(name).Parse(content))
		}
	}
	return root
}
