package rex

import (
	"bufio"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

var (
	// StrictHome when set to true, only the root path will be matched
	StrictHome = false

	// NoTrailingSlash when set to true, trailing slashes will be removed
	NoTrailingSlash = true

	// name of the template content block
	contentBlock = "Content"
)

// HandlerFunc is the signature for route handlers that can return errors
type HandlerFunc func(c *Context) error

// Middleware function that takes a HandlerFunc and returns a HandlerFunc
type Middleware func(HandlerFunc) HandlerFunc

// Convert http.HandlerFunc to HandlerFunc
func WrapFunc(h http.HandlerFunc) HandlerFunc {
	return func(c *Context) error {
		// copy over context values
		for k, v := range c.locals {
			c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), k, v))
		}
		h(c.Response, c.Request)
		return nil
	}
}

// Convert http.Handler to HandlerFunc
func WrapHandler(h http.Handler) HandlerFunc {
	return func(c *Context) error {
		// copy over context values
		for k, v := range c.locals {
			c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), k, v))
		}
		h.ServeHTTP(c.Response, c.Request)
		return nil
	}
}

// Convert HandlerFunc to http.Handler. Note that this will not work with middlewares
// as the context is not passed to the handler and the router is not set(i.e c.router is nil).
func (h HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := &Context{
		Request:  r,
		Response: &ResponseWriter{writer: w},
		router:   nil,
		locals:   make(map[any]any),
	}

	if err := h(ctx); err != nil {
		log.Println(err)
	}
}

// Context represents the context of the current HTTP request
type Context struct {
	Request  *http.Request
	Response *ResponseWriter
	router   *Router
	locals   map[any]any
	mu       sync.RWMutex
}

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

// SetHeader sets a header in the response
func (c *Context) SetHeader(key, value string) {
	c.Response.Header().Set(key, value)
}

// GetHeader returns the status code of the response
func GetHeader(r *http.Request, key string) string {
	return r.Header.Get(key)
}

// SetStatus sets the status code of the response
func (c *Context) SetStatus(status int) error {
	c.Response.WriteHeader(status)
	return nil
}

// Router is the main router structure
type Router struct {
	mux               *http.ServeMux        // http.ServeMux
	routes            map[string]route      // map of routes
	globalMiddlewares []Middleware          // global middlewares
	errorHandler      func(*Context, error) // centralized error handler

	// Configuration for templates
	viewsFs            fs.FS              // Views embed.FS(Alternative to views if set)
	template           *template.Template // All parsed templates
	baseLayout         string             // Base layout for the templates(default is "")
	contentBlock       string             // Content block for the templates(default is "Content")
	errorTemplate      string             // Error template. Passed "error", "status", "status_text" in its context.
	passContextToViews bool               // Pass the request context to the views

	// groups
	groups map[string]*Group // Groups mapped to their prefix

	// Handler for 404 not found errors. Note that when this is called,
	// The request parameters are not available, since they are populated by the http.ServeMux
	// when the request is matched to a route. So calling r.PathValue() will return "".
	NotFoundHandler http.Handler
}

type route struct {
	prefix      string       // method + pattern
	handler     HandlerFunc  // handler function
	middlewares []Middleware // middlewares for the route
}

// Router option a function option for configuring the router.
type RouterOption func(*Router)

// NewRouter creates a new router with the given options.
// The router wraps the http.DefaultServeMux and adds routing and middleware
// capabilities.
func NewRouter(options ...RouterOption) *Router {
	r := &Router{
		mux:                http.NewServeMux(),
		routes:             make(map[string]route),
		passContextToViews: false,
		baseLayout:         "",
		contentBlock:       contentBlock,
		viewsFs:            nil,
		groups:             make(map[string]*Group),
		globalMiddlewares:  []Middleware{},
		template:           nil,
	}

	for _, option := range options {
		option(r)
	}
	return r
}

// Generic type for response data
type Map map[string]any

// Set error handler for centralized error handling
func (r *Router) SetErrorHandler(handler func(*Context, error)) {
	r.errorHandler = handler
}

// Global middleware
func (r *Router) Use(middlewares ...Middleware) {
	r.globalMiddlewares = append(r.globalMiddlewares, middlewares...)
}

// Handle registers a new route with the given path and handler
func (r *Router) Handle(method, pattern string, handler HandlerFunc, middlewares ...Middleware) {
	if StrictHome && pattern == "/" {
		pattern = pattern + "{$}" // Match only the root pattern
	}

	// remove trailing slashes
	if NoTrailingSlash && pattern != "/" {
		pattern = strings.TrimSuffix(pattern, "/")
	}

	// Combine global and route-specific middlewares
	allMiddleware := append(r.globalMiddlewares, middlewares...)

	// Chain all middleware
	final := handler
	for i := len(allMiddleware) - 1; i >= 0; i-- {
		final = allMiddleware[i](final)
	}

	// Store the route
	routePattern := method + " " + pattern
	r.routes[routePattern] = route{
		prefix:      routePattern,
		handler:     final,
		middlewares: middlewares,
	}

	// Convert HandlerFunc to http.HandlerFunc for ServeMux
	r.mux.Handle(routePattern, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Only handle requests with matching method
		if req.Method != method {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		rw := &ResponseWriter{
			writer: w,
			status: http.StatusOK,
		}

		ctx := &Context{
			Request:  req,
			Response: rw,
			router:   r,
			locals:   make(map[any]any),
		}

		// Execute the handler and handle any errors
		if err := final(ctx); err != nil {
			r.errorHandler(ctx, err)
		}
	}))
}

// Common HTTP method handlers
func (r *Router) GET(pattern string, handler HandlerFunc, middlewares ...Middleware) {
	r.Handle(http.MethodGet, pattern, handler, middlewares...)
}

func (r *Router) POST(pattern string, handler HandlerFunc, middlewares ...Middleware) {
	r.Handle(http.MethodPost, pattern, handler, middlewares...)
}

func (r *Router) PUT(pattern string, handler HandlerFunc, middlewares ...Middleware) {
	r.Handle(http.MethodPut, pattern, handler, middlewares...)
}

func (r *Router) PATCH(pattern string, handler HandlerFunc, middlewares ...Middleware) {
	r.Handle(http.MethodPatch, pattern, handler, middlewares...)
}

func (r *Router) DELETE(pattern string, handler HandlerFunc, middlewares ...Middleware) {
	r.Handle(http.MethodDelete, pattern, handler, middlewares...)
}

// OPTIONS. This may not be necessary as registering GET request automatically registers OPTIONS.
func (r *Router) OPTIONS(pattern string, handler HandlerFunc, middlewares ...Middleware) {
	r.Handle(http.MethodOptions, pattern, handler, middlewares...)
}

// HEAD request.
func (r *Router) HEAD(pattern string, handler HandlerFunc, middlewares ...Middleware) {
	r.Handle(http.MethodHead, pattern, handler, middlewares...)
}

// TRACE http request.
func (r *Router) TRACE(pattern string, handler HandlerFunc, middlewares ...Middleware) {
	r.Handle(http.MethodTrace, pattern, handler, middlewares...)
}

// CONNECT http request.
func (r *Router) CONNECT(pattern string, handler HandlerFunc, middlewares ...Middleware) {
	r.Handle(http.MethodConnect, pattern, handler, middlewares...)
}

// ServeHTTP implements the http.Handler interface
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

// Context helper methods
// JSON sends a JSON response
func (c *Context) JSON(data interface{}) error {
	c.Response.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(c.Response).Encode(data)
}

// XML sends an XML response
func (c *Context) XML(data interface{}) error {
	c.Response.Header().Set("Content-Type", "application/xml")
	return xml.NewEncoder(c.Response).Encode(data)
}

// String sends a string response
func (c *Context) String(format string, values ...interface{}) error {
	c.Response.Header().Set("Content-Type", "text/plain")
	_, err := fmt.Fprintf(c.Response, format, values...)
	return err
}

// Returns the header content type stripping everything after ; like
// charset or form boundary in multipart/form-data forms.
func (c *Context) ContentType() string {
	return strings.Split(c.Request.Header.Get("Content-Type"), ";")[0]
}

// Send HTML response.
func (c *Context) HTML(html string) error {
	c.Response.Header().Set("Content-Type", "text/html")
	_, err := c.Response.Write([]byte(html))
	return err
}

func (c *Context) WriteHeader(status int) error {
	c.Response.WriteHeader(status)
	return nil
}

// Write sends a raw response
func (c *Context) Write(data []byte) (int, error) {
	return c.Response.Write(data)
}

// Param gets a path parameter value by name from the request.
// If the parameter is not found, it checks the redirect options.
func (c *Context) Param(name string) string {
	p := c.Request.PathValue(name)
	if p == "" {
		// check redirect params
		opts, ok := c.redirectOptions()
		if ok {
			p = opts.Params[name]
		}
	}
	return p
}

// paramInt returns the value of the parameter as an integer
// If the parameter is not found, it checks the redirect options.
func (c *Context) ParamInt(key string, defaults ...int) int {
	v := c.Param(key)
	if v == "" && len(defaults) > 0 {
		return defaults[0]
	}

	vInt, err := strconv.Atoi(v)
	if err != nil {
		if len(defaults) > 0 {
			return defaults[0]
		}
		return 0
	}
	return vInt
}

// Query returns the value of the query as a string.
// If the query is not found, it checks the redirect options.
func (c *Context) Query(key string, defaults ...string) string {
	v := c.Request.URL.Query().Get(key)
	if v == "" {
		// check redirect query params
		opts, ok := c.redirectOptions()
		if ok {
			v = opts.QueryParams[key]
		}
	}

	if v == "" && len(defaults) > 0 {
		return defaults[0]
	}
	return v
}

// queryInt returns the value of the query as an integer
// If the query is not found, it checks the redirect options.
func (c *Context) QueryInt(key string, defaults ...int) int {
	v := c.Query(key)
	if v == "" && len(defaults) > 0 {
		return defaults[0]
	}

	vInt, err := strconv.Atoi(v)
	if err != nil {
		if len(defaults) > 0 {
			return defaults[0]
		}
		return 0
	}
	return vInt
}

// Set stores a value in the context
func (c *Context) Set(key interface{}, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.locals[key] = value

	// Also set the value in the request context
	ctx := context.WithValue(c.Request.Context(), key, value)
	*c.Request = *c.Request.WithContext(ctx)

}

// Get retrieves a value from the context
func (c *Context) Get(key interface{}) (value interface{}, exists bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, exists = c.locals[key]
	return
}

// Locals returns the context values
func (c *Context) Locals() map[any]any {
	return c.locals
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

// Redirects the request to the given url.
// Default status code is 303 (http.StatusSeeOther)
func (c *Context) Redirect(url string, status ...int) error {
	var statusCode = http.StatusSeeOther
	if len(status) > 0 {
		statusCode = status[0]
	}
	http.Redirect(c.Response, c.Request, url, statusCode)
	return nil
}

// save file from multipart form to disk
func (c *Context) SaveFile(fh *multipart.FileHeader, dst string) error {
	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, src)
	return err
}

// chain of middlewares
func (r *Router) chain(middlewares []Middleware, handler HandlerFunc) HandlerFunc {
	if len(middlewares) == 0 {
		return handler
	}

	// wrap the handler with the last middleware
	wrapped := middlewares[len(middlewares)-1](handler)

	// wrap the handler with the remaining middlewares
	for i := len(middlewares) - 2; i >= 0; i-- {
		wrapped = middlewares[i](wrapped)
	}
	return wrapped
}

// Serve static assests at prefix in the directory dir.
// e.g r.Static("/static", "static").
// This method will strip the prefix from the URL path.
// To serve minified assets(JS and CSS) if present, call rex.ServeMinifiedAssetsIfPresent=true.
// To enable caching, provide maxAge seconds for cache duration.
func (r *Router) Static(prefix, dir string, maxAge ...int) {
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	cacheDuration := 0
	if len(maxAge) > 0 {
		cacheDuration = maxAge[0]
	}

	var handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		path := filepath.Join(dir, strings.TrimPrefix(req.URL.Path, prefix))

		setCacheHeaders := func() {
			if cacheDuration > 0 {
				// Set cache control headers with the specified maxAge
				w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(cacheDuration))
			}
		}

		if ServeMinifiedAssetsIfPresent {
			stat, err := os.Stat(path)
			if err != nil || stat.IsDir() {
				http.NotFound(w, req)
				return
			}

			if strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".css") {
				// Check for the minified version of the file
				minifiedPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".min" + filepath.Ext(path)
				if filePathExists(minifiedPath) {
					http.ServeFile(w, req, minifiedPath)
					setCacheHeaders()
					return
				}
			}
		}

		setCacheHeaders()

		http.ServeFile(w, req, path)

	})

	r.mux.Handle(prefix, r.chain(r.globalMiddlewares, WrapHandler(handler)))
}

func filePathExists(name string) bool {
	stat, err := os.Stat(name)
	return err == nil && !stat.IsDir()
}

// Wrapper around http.ServeFile.
func (r *Router) File(path, file string) {
	var hf http.HandlerFunc = func(w http.ResponseWriter, req *http.Request) {
		http.ServeFile(w, req, file)
	}
	handler := r.chain(r.globalMiddlewares, WrapFunc(hf))
	r.GET(path, handler)
}

func (r *Router) FileFS(fs http.FileSystem, prefix, path string) {
	r.GET(prefix, WrapHandler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		f, err := fs.Open(path)
		if err != nil {
			http.NotFound(w, req)
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil || stat.IsDir() {
			http.NotFound(w, req)
			return
		}

		w.WriteHeader(http.StatusOK)
		http.ServeContent(w, req, path, stat.ModTime(), f)
	})))
}

// Serve favicon.ico from the file system fs at path.
func (r *Router) FaviconFS(fs http.FileSystem, path string) {
	var handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		f, err := fs.Open(path)
		if err != nil {
			http.NotFound(w, req)
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil || stat.IsDir() {
			http.NotFound(w, req)
			return
		}

		var data = make([]byte, stat.Size())
		_, err = f.Read(data)
		if err != nil {
			http.NotFound(w, req)
			return
		}

		w.Header().Set("Content-Type", "image/x-icon")
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
		w.Header().Set("Content-Disposition", "inline; filename=favicon.ico")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})

	r.GET("/favicon.ico", WrapHandler(handler))
}

type minifiedFS struct {
	http.FileSystem
}

func (mfs *minifiedFS) Open(name string) (http.File, error) {
	// Check if the requested file is a .js or .css file
	if strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".css") {
		// Check for the minified version of the file
		minifiedName := strings.TrimSuffix(name, filepath.Ext(name)) + ".min" + filepath.Ext(name)

		// Return minified file if available.
		if f, err := mfs.FileSystem.Open(minifiedName); err == nil {
			return f, nil
		}
	}

	// If no minified version is found, serve the original file
	return mfs.FileSystem.Open(name)
}

// Serve minified Javascript and CSS if present instead of original file.
// This applies to StaticFS, Static functions.
// e.g /static/js/main.js will serve /static/js/main.min.js if present.
// Default is false.
// This is important since we maintain the same script sources in our templates/html.
var ServeMinifiedAssetsIfPresent = false

// Like Static but for http.FileSystem.
// Use this to serve embedded assets with go/embed.
//
//	mux.StaticFS("/static", http.FS(staticFs))
//
// To enable caching, provide maxAge seconds for cache duration.
func (r *Router) StaticFS(prefix string, fs http.FileSystem, maxAge ...int) {
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	if ServeMinifiedAssetsIfPresent {
		fs = &minifiedFS{fs}
	}

	cacheDuration := 0
	if len(maxAge) > 0 {
		cacheDuration = maxAge[0]
	}

	// Create file server for the http.FileSystem
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cacheDuration > 0 {
			// Set cache control headers with the specified maxAge
			w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(cacheDuration))
		}
		http.FileServer(fs).ServeHTTP(w, r)
	})

	// Apply global middleware
	finalHandler := r.chain(r.globalMiddlewares, WrapHandler(handler))
	r.mux.Handle(prefix, finalHandler)
}

// =========== SPA handling ===========
// creates a new http.FileSystem from the embed.FS
func buildFS(frontendFS fs.FS, root string) http.FileSystem {
	fsys, err := fs.Sub(frontendFS, root)
	if err != nil {
		panic(err)
	}
	return http.FS(fsys)
}

// SPAOptions for customizing the cache control and index file.
type SPAOptions struct {
	CacheControl     string           // default is empty, example: "public, max-age=31536000"
	ResponseModifier http.HandlerFunc // allows fo modifying request/response
	Skip             []string         // skip these routes and return 404 if they match
	Index            string           // default is index.html
}

// Serves Single Page applications like svelte-kit, react etc.
// frontendFS is any interface that satisfies fs.FS, like embed.FS,
// http.Dir() wrapping a directory etc.
// path is the mount point: likely "/".
// buildPath is the path to build output containing your entry point html file.
// The default entrypoint is "index.html" that is served for all unmatched routes.
// You can change the entrypoint with options. Passed options override all defaults.
func (r *Router) SPAHandler(frontendFS fs.FS, path string, buildPath string, options ...SPAOptions) {
	var (
		indexFile    = "index.html"
		cacheControl string
		skip         []string
		resModifier  http.HandlerFunc = nil
	)

	if len(options) > 0 {
		option := options[0]

		cacheControl = option.CacheControl
		skip = option.Skip

		if option.Index != "" {
			indexFile = option.Index
		}
		resModifier = option.ResponseModifier
	}

	indexFp, err := frontendFS.Open(filepath.Join(buildPath, indexFile))
	if err != nil {
		panic(err)
	}

	index, err := io.ReadAll(indexFp)
	if err != nil {
		panic("Unable to read contents of " + indexFile)
	}

	// Apply global middleware
	fsHandler := http.FileServer(buildFS(frontendFS, buildPath))
	handler := r.chain(r.globalMiddlewares, WrapHandler(fsHandler))

	r.mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
		// check skip.
		for _, s := range skip {
			if s == req.URL.Path {
				http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
				return
			}
		}

		baseName := filepath.Base(req.URL.Path)
		if req.URL.Path == "/" {
			baseName = indexFile
		}

		// open the file from the embed.FS
		f, err := frontendFS.Open(filepath.Join(buildPath, baseName))
		if err != nil {
			if os.IsNotExist(err) {
				// Could be an invalid API request
				if filepath.Ext(req.URL.Path) != "" {
					http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
					return
				}

				// Send the html content type.
				w.Header().Set("Content-Type", "text/html")

				// set cache control headers if specified by user.
				if cacheControl != "" {
					w.Header().Set("Cache-Control", cacheControl)
				}

				w.WriteHeader(http.StatusAccepted)

				// Allow user to modify response.
				if resModifier != nil {
					resModifier(w, req)
				}

				// send index.html
				w.Write(index)
			} else {
				// IO Error
				http.Error(w, "500 internal server error", http.StatusInternalServerError)
			}
			return
		} else {
			// we found the file, send it if not a directory.
			defer f.Close()
			stat, err := f.Stat()
			if err != nil {
				http.Error(w, "500 internal server error", http.StatusInternalServerError)
				return
			}

			if stat.IsDir() {
				http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
				return
			}

			// The file system handler knows how to serve JS/CSS and other assets with the correct
			// content type.
			handler.ServeHTTP(w, req)
		}
	})
}

type RedirectOptions struct {
	Status      int               // status code to use for the redirect
	Params      map[string]string // query parameters to add to the redirect URL
	QueryParams map[string]string // query parameters to add to the redirect URL
}

var defaultRedirectOptions = RedirectOptions{
	Params:      make(map[string]string),
	QueryParams: make(map[string]string),
}

type redirectContextType string

const redirectContextKey = redirectContextType("redirect")

// RedirectRoute redirects the request to the given route.
// The pathname is the name of the route to redirect to.
// The options are the redirect options like status code, query parameters etc.
func (c *Context) RedirectRoute(pathname string, options ...RedirectOptions) error {
	var opts RedirectOptions
	if len(options) > 0 {
		opts = options[0]
		if opts.Status == 0 {
			opts.Status = defaultRedirectOptions.Status
		}

		if opts.Params == nil {
			opts.Params = defaultRedirectOptions.Params
		}
		if opts.QueryParams == nil {
			opts.QueryParams = defaultRedirectOptions.QueryParams
		}
	} else {
		opts = defaultRedirectOptions
	}

	// find the mathing route
	var handler HandlerFunc

	for _, route := range c.router.routes {
		// we can only redirect to /GET routes
		if route.prefix[:3] != http.MethodGet {
			continue
		}

		// split prefix into method and path
		parts := strings.Split(route.prefix, " ")
		name := strings.TrimSpace(parts[1])
		if name == pathname {
			handler = route.handler
			break
		}
	}

	if handler == nil {
		c.Response.WriteHeader(http.StatusNotFound)
		return fmt.Errorf("route not found")
	}

	c.Response.WriteHeader(opts.Status)
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), redirectContextKey, opts))
	return handler(c)
}

// Returns the redirect options set in the context when RedirectRoute is called.
func (c *Context) redirectOptions() (RedirectOptions, bool) {
	opts, ok := c.Request.Context().Value(redirectContextKey).(RedirectOptions)
	if !ok {
		return defaultRedirectOptions, false
	}
	return opts, true
}

// RouteInfo contains information about a registered route.
type RouteInfo struct {
	Method string // Http method.
	Path   string // Registered pattern.
	Name   string // Function name for the handler.
}

// RegisteredRoutes returns a list of registered routes in a slice of RouteInfo.
func (r *Router) RegisteredRoutes() []RouteInfo {
	var routes []RouteInfo
	for _, route := range r.routes {
		parts := strings.SplitN(route.prefix, " ", 2)
		routes = append(routes, RouteInfo{Method: parts[0], Path: parts[1], Name: getFuncName(route.handler)})
	}
	return routes
}

func getFuncName(f interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}

// IP returns the client's IP address.
// It tries to get the IP from the X-Forwarded-For header first, then falls back to the X-Real-Ip header.
// If both headers are not set, it returns the remote address from the request.
func (c *Context) IP() (string, error) {
	ips := c.Request.Header.Get("X-Forwarded-For")
	splitIps := strings.Split(ips, ",")

	if len(splitIps) > 0 {
		// get last IP in list since ELB prepends other user defined IPs,
		// meaning the last one is the actual client IP.
		netIP := net.ParseIP(splitIps[len(splitIps)-1])
		if netIP != nil {
			return netIP.String(), nil
		}
	}

	// Try to get the IP from the X-Real-Ip header.
	ip := c.Request.Header.Get("X-Real-Ip")
	if ip != "" {
		return ip, nil
	}

	ip, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return "", err
	}

	netIP := net.ParseIP(ip)
	if netIP != nil {
		ip := netIP.String()
		if ip == "::1" {
			return "127.0.0.1", nil
		}
		return ip, nil
	}
	return "", errors.New("IP not found")
}
