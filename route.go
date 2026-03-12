package rex

import (
	"net/http"
)

type route struct {
	prefix      string       // method + pattern
	handler     HandlerFunc  // handler function
	middlewares []Middleware // middlewares for the route
	router      *Router
}

// With creates a route builder with the provided middleware.
func (r *Router) With(midleware ...Middleware) *route {
	return &route{
		middlewares: midleware,
		router:      r,  // router reference
		prefix:      "", // when returned by Group, this is not an empty string
	}
}

// GET registers a GET route on pattern.
func (r *route) GET(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodGet, r.prefix+pattern, handler, false, r.middlewares...)
}

// POST registers a POST route on pattern.
func (r *route) POST(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodPost, r.prefix+pattern, handler, false, r.middlewares...)
}

// PUT registers a PUT route on pattern.
func (r *route) PUT(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodPut, r.prefix+pattern, handler, false, r.middlewares...)
}

// PATCH registers a PATCH route on pattern.
func (r *route) PATCH(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodPatch, r.prefix+pattern, handler, false, r.middlewares...)
}

// DELETE registers a DELETE route on pattern.
func (r *route) DELETE(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodDelete, r.prefix+pattern, handler, false, r.middlewares...)
}

// OPTIONS registers an OPTIONS route on pattern.
func (r *route) OPTIONS(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodOptions, r.prefix+pattern, handler, false, r.middlewares...)
}

// HEAD request.
func (r *route) HEAD(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodHead, r.prefix+pattern, handler, false, r.middlewares...)
}

// TRACE http request.
func (r *route) TRACE(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodTrace, r.prefix+pattern, handler, false, r.middlewares...)
}

// CONNECT http request.
func (r *route) CONNECT(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodConnect, r.prefix+pattern, handler, false, r.middlewares...)
}
