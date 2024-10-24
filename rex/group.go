package rex

import "net/http"

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

// GET request.
func (g *Group) Get(path string, handler HandlerFunc, middlewares ...Middleware) {
	g.router.Handle(http.MethodGet, g.prefix+path, handler, append(g.middlewares, middlewares...)...)
}

// POST request.
func (g *Group) Post(path string, handler HandlerFunc, middlewares ...Middleware) {
	g.router.Handle(http.MethodPost, g.prefix+path, handler, append(g.middlewares, middlewares...)...)
}

// PUT request.
func (g *Group) Put(path string, handler HandlerFunc, middlewares ...Middleware) {
	g.router.Handle(http.MethodPut, g.prefix+path, handler, append(g.middlewares, middlewares...)...)
}

// PATCH request.
func (g *Group) Patch(path string, handler HandlerFunc, middlewares ...Middleware) {
	g.router.Handle(http.MethodPatch, g.prefix+path, handler, append(g.middlewares, middlewares...)...)
}

// DELETE request.
func (g *Group) Delete(path string, handler HandlerFunc, middlewares ...Middleware) {
	g.router.Handle(http.MethodDelete, g.prefix+path, handler, append(g.middlewares, middlewares...)...)
}

// Creates a nested group with the given prefix and middleware.
func (g *Group) Group(prefix string, middlewares ...Middleware) *Group {
	return g.router.Group(g.prefix+prefix, append(g.middlewares, middlewares...)...)
}

// TODO: Add more methods for other HTTP methods and static files.
