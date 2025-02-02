package rex

import "net/http"

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
		router:      r,
	}
}

// Common HTTP method handlers
func (r *route) GET(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodGet, pattern, handler, false, r.middlewares...)
}

func (r *route) POST(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodPost, pattern, handler, false, r.middlewares...)
}

func (r *route) PUT(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodPut, pattern, handler, false, r.middlewares...)
}

func (r *route) PATCH(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodPatch, pattern, handler, false, r.middlewares...)
}

func (r *route) DELETE(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodDelete, pattern, handler, false, r.middlewares...)
}

// OPTIONS. This may not be necessary as registering GET request automatically registers OPTIONS.
func (r *route) OPTIONS(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodOptions, pattern, handler, false, r.middlewares...)
}

// HEAD request.
func (r *route) HEAD(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodHead, pattern, handler, false, r.middlewares...)
}

// TRACE http request.
func (r *route) TRACE(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodTrace, pattern, handler, false, r.middlewares...)
}

// CONNECT http request.
func (r *route) CONNECT(pattern string, handler HandlerFunc) {
	r.router.handle(http.MethodConnect, pattern, handler, false, r.middlewares...)
}
