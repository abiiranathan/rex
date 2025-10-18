package rex

import (
	"net/http"
)

// Group is a collection of routes with a common prefix.
type Group struct {
	prefix      string       // Group prefix
	middlewares []Middleware // Middlewares specific to this group
	router      *Router      // The router
}

// Group creates a new group with the given prefix and options.
func (r *Router) Group(prefix string, middlewares ...Middleware) *Group {
	group := &Group{
		prefix:      prefix,
		middlewares: middlewares,
		router:      r,
	}

	r.groups[prefix] = group
	return group
}

// Use adds middlewares to the group.
func (g *Group) Use(middlewares ...Middleware) {
	g.middlewares = append(g.middlewares, middlewares...)
}

// Create a route and apply this middleware to it.
func (g *Group) With(middlewares ...Middleware) *route {
	return &route{
		prefix:      g.prefix,
		middlewares: append(g.middlewares, middlewares...),
		router:      g.router,
	}
}

// GET request.
func (g *Group) GET(path string, handler HandlerFunc) {
	g.router.handle(http.MethodGet, g.prefix+path, handler, false, g.middlewares...)
}

// POST request.
func (g *Group) POST(path string, handler HandlerFunc) {
	g.router.handle(http.MethodPost, g.prefix+path, handler, false, g.middlewares...)
}

// PUT request.
func (g *Group) PUT(path string, handler HandlerFunc) {
	g.router.handle(http.MethodPut, g.prefix+path, handler, false, g.middlewares...)
}

// PATCH request.
func (g *Group) PATCH(path string, handler HandlerFunc) {
	g.router.handle(http.MethodPatch, g.prefix+path, handler, false, g.middlewares...)
}

// DELETE request.
func (g *Group) DELETE(path string, handler HandlerFunc) {
	g.router.handle(http.MethodDelete, g.prefix+path, handler, false, g.middlewares...)
}

// Creates a nested group with the given prefix and middleware.
func (g *Group) Group(prefix string, middleware ...Middleware) *Group {
	return g.router.Group(g.prefix+prefix, append(g.middlewares, middleware...)...)
}

// Static serves files from the given file system root.
func (g *Group) Static(prefix, dir string, maxAge ...int) {
	g.router.Static(g.prefix+prefix, dir, maxAge...)
}

// StaticFs serves files from the given file system.
func (g *Group) StaticFs(prefix string, fs http.FileSystem, maxAge ...int) {
	g.router.StaticFS(g.prefix+prefix, fs, maxAge...)
}
