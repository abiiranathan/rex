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

// Assign middleware to route and return the route.
func (r *Router) With(midleware ...Middleware) *route {
	return &route{
		middlewares: midleware,
		router:      r,  // router reference
		prefix:      "", // when returned by Group, this is not an empty string
	}
}

// Register /GET method on pattern.
// You can optionally pass a custom validator to validate
// the rex.Map values during template rendering.
func (r *route) GET(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodGet, r.prefix+pattern, handler, false, r.middlewares...)
}

func (r *route) POST(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodPost, r.prefix+pattern, handler, false, r.middlewares...)
}

func (r *route) PUT(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodPut, r.prefix+pattern, handler, false, r.middlewares...)
}

func (r *route) PATCH(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodPatch, r.prefix+pattern, handler, false, r.middlewares...)
}

func (r *route) DELETE(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodDelete, r.prefix+pattern, handler, false, r.middlewares...)
}

// OPTIONS. This may not be necessary as registering GET request automatically registers OPTIONS.
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
