/*
Package rex (go router) implements a minimalistic but robust http router based on the standard go 1.22
enhanced routing capabilities in the `http.ServeMux`.

It adds features like middleware support, helper methods for defining routes,
template rendering with automatic template inheritance (of a base template).

It also has a BodyParser that decodes json, xml, url-encoded and multipart forms
based on content type. Form parsing supports all standard go types(and their pointers)
and slices of standard types.
It also supports custom types that implement the `rex.FormScanner` interface.

rex supports single page application routing with a dedicated method `r.SPAHandler`
that serves the index.html file for all routes that do not match a file or directory in the root directory of the SPA.

The router also supports route groups and subgroups with middleware
that can be applied to the entire group or individual routes.
It has customizable built-in middleware for logging using the slog package,
panic recovery, etag, cors, basic auth and jwt middlewares.

More middlewares can be added by implementing the Middleware type,
a standard function that wraps rex.Handler.
*/
package rex

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	en_translations "github.com/go-playground/validator/v10/translations/en"
)

var (
	// StrictHome when set to true, only the root path will be matched
	StrictHome = true

	// NoTrailingSlash when set to true, trailing slashes will be removed
	NoTrailingSlash = true

	// name of the template content block
	contentBlock = "Content"

	// Serve minified files if present instead of original file.
	// This applies to StaticFS, Static functions.
	ServeMinified = false

	// MinExtensions is the slice of file extensions for which minified files are served.
	MinExtensions = []string{".js", ".css"}
)

// HandlerFunc is the signature for route handlers that can return errors
type HandlerFunc func(c *Context) error

// Middleware function that takes a HandlerFunc and returns a HandlerFunc
type Middleware func(HandlerFunc) HandlerFunc

// Generic type for response data
type Map map[string]any

// WrapHandler wraps an http.Handler to be used as a HandlerFunc while preserving router access
func (r *Router) WrapHandler(h http.Handler) HandlerFunc {
	return func(c *Context) error {
		// copy over context values
		for k, v := range c.locals {
			c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), k, v))
		}
		h.ServeHTTP(c.Response, c.Request)
		return nil
	}
}

// Convert HandlerFunc to http.Handler with router access
func (r *Router) ToHTTPHandler(h HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := r.initContext(w, req)
		defer r.putContext(ctx)

		if err := h(ctx); err != nil {
			r.errorHandler(ctx, err)
		}
	})
}

// WrapMiddleware wraps an http middleware to be used as a rex middleware.
func (router *Router) WrapMiddleware(middleware func(http.Handler) http.Handler) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if c.router == nil {
					c.router = router
				}
				next(c)
			})

			handler = middleware(handler)
			handler.ServeHTTP(c.Response, c.Request)
			return nil
		}
	}
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

	// Validator instance
	validator *validator.Validate

	// universal translator
	translator ut.Translator

	// Logger
	logger *slog.Logger
}

type route struct {
	prefix      string       // method + pattern
	handler     HandlerFunc  // handler function
	middlewares []Middleware // middlewares for the route
}

// Router option a function option for configuring the router.
type RouterOption func(*Router)

// Replace the default slog.Logger with a custom logger.
func WithLogger(logger *slog.Logger) RouterOption {
	if logger == nil {
		panic("logger cannot be nil")
	}

	return func(r *Router) {
		r.logger = logger
	}
}

// defaultErrorHandler is the default error handler for the router.
// It handles errors centrally and logs and writes the error to the response.
// The logger can be replaced with a custom logger using WithLogger option.
// It also handles validation errors and form errors.
// The default error handler can be replaced with a custom error handler using SetErrorHandler.
func defaultErrorHandler(ctx *Context, err error) {
	// Log the error
	ctx.router.logger.Debug("ERROR", "error", err, "status", ctx.Response.Status(), "path", ctx.Request.URL.Path)

	if ve, ok := err.(validator.ValidationErrors); ok {
		HandleValidationErrors(ctx, ve)
		return
	}

	if fe, ok := err.(FormError); ok {
		HandleFormErrors(ctx, fe)
		return
	}

	ctx.WriteHeader(http.StatusInternalServerError)
	ctx.Write([]byte(err.Error()))
}

// NewRouter creates a new router with the given options.
// The router wraps the http.DefaultServeMux and adds routing and middleware
// capabilities.
// The router uses slog for logging. The default log level is Error with JSON formatting.
// The router also performs automatic body parsing and struct validation
// with the go-playground/validator/v10 package.
func NewRouter(options ...RouterOption) *Router {
	r := &Router{
		mux:                http.NewServeMux(),
		routes:             make(map[string]route),
		passContextToViews: false,
		baseLayout:         "",
		contentBlock:       contentBlock,
		viewsFs:            nil,
		template:           nil,
		groups:             make(map[string]*Group),
		globalMiddlewares:  []Middleware{},
		validator:          validator.New(validator.WithRequiredStructEnabled()),
		logger: slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			AddSource: false,
			Level:     slog.LevelError,
		})),

		// Global error handler function.
		errorHandler: defaultErrorHandler,
	}

	// Create translator
	en := en.New()
	uni := ut.New(en, en)
	trans, _ := uni.GetTranslator("en")

	// Connect ut to our validator
	en_translations.RegisterDefaultTranslations(r.validator, trans)
	r.translator = trans

	for _, option := range options {
		option(r)
	}
	return r
}

// RegisterValidation adds a validation with the given tag
//
// NOTES: - if the key already exists, the previous validation function will be replaced.
// - this method is not thread-safe it is intended that these all be registered prior
// to any validation
func (r *Router) RegisterValidation(tag string, fn validator.Func) {
	r.validator.RegisterValidation(tag, fn, true)
}

// RegisterValidationCtx does the same as RegisterValidation on accepts a
// FuncCtx validation allowing context.Context validation support.
func (r *Router) RegisterValidationCtx(tag string, fn validator.FuncCtx) {
	r.validator.RegisterValidationCtx(tag, fn, true)
}

// Set error handler for centralized error handling
func (r *Router) SetErrorHandler(handler func(*Context, error)) {
	r.errorHandler = handler
}

// Global middleware
func (r *Router) Use(middlewares ...Middleware) {
	r.globalMiddlewares = append(r.globalMiddlewares, middlewares...)
}

// Pool for reusing context objects
var ctxPool = sync.Pool{
	New: func() any {
		return &Context{
			locals: make(map[any]any),
		}
	},
}

// Get a context from the pool
func (r *Router) getContext() *Context {
	return ctxPool.Get().(*Context)
}

// Put the context back in the pool
func (r *Router) putContext(c *Context) {
	c.reset()
	ctxPool.Put(c)
}

// Init context
func (r *Router) initContext(w http.ResponseWriter, req *http.Request) *Context {
	c := r.getContext()
	c.Request = req
	c.Response = &ResponseWriter{
		writer: w,
		status: http.StatusOK,
	}
	c.router = r
	return c
}

// Reset the context
func (c *Context) reset() {
	c.Request = nil
	c.Response = nil
	c.router = nil
	c.locals = make(map[any]any)
}

// Handle registers a new route with the given path and handler
func (r *Router) Handle(method, pattern string, handler HandlerFunc, is_static bool, middlewares ...Middleware) {
	if StrictHome && pattern == "/" {
		pattern = pattern + "{$}" // Match only the root pattern
	}

	// remove trailing slashes if not a static route
	if !is_static {
		if NoTrailingSlash && pattern != "/" {
			pattern = strings.TrimSuffix(pattern, "/")
		}
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
		var skipBody bool
		// Only handle requests with matching method
		if req.Method != method {
			// Allow HEAD requests for GET routes
			// OPTIONS/TRACE and other methods are not allowed and must be defined explicitly.
			allowed := method == http.MethodGet && req.Method == http.MethodHead
			if !allowed {
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
				return
			}
			skipBody = true
		}

		ctx := r.initContext(w, req)
		defer r.putContext(ctx)

		ctx.Response.skipBody = skipBody

		// Execute the handler and handle any errors
		if err := final(ctx); err != nil {
			r.errorHandler(ctx, err)
		}
	}))
}

// Common HTTP method handlers
func (r *Router) GET(pattern string, handler HandlerFunc, middlewares ...Middleware) {
	r.Handle(http.MethodGet, pattern, handler, false, middlewares...)
}

func (r *Router) POST(pattern string, handler HandlerFunc, middlewares ...Middleware) {
	r.Handle(http.MethodPost, pattern, handler, false, middlewares...)
}

func (r *Router) PUT(pattern string, handler HandlerFunc, middlewares ...Middleware) {
	r.Handle(http.MethodPut, pattern, handler, false, middlewares...)
}

func (r *Router) PATCH(pattern string, handler HandlerFunc, middlewares ...Middleware) {
	r.Handle(http.MethodPatch, pattern, handler, false, middlewares...)
}

func (r *Router) DELETE(pattern string, handler HandlerFunc, middlewares ...Middleware) {
	r.Handle(http.MethodDelete, pattern, handler, false, middlewares...)
}

// OPTIONS. This may not be necessary as registering GET request automatically registers OPTIONS.
func (r *Router) OPTIONS(pattern string, handler HandlerFunc, middlewares ...Middleware) {
	r.Handle(http.MethodOptions, pattern, handler, false, middlewares...)
}

// HEAD request.
func (r *Router) HEAD(pattern string, handler HandlerFunc, middlewares ...Middleware) {
	r.Handle(http.MethodHead, pattern, handler, false, middlewares...)
}

// TRACE http request.
func (r *Router) TRACE(pattern string, handler HandlerFunc, middlewares ...Middleware) {
	r.Handle(http.MethodTrace, pattern, handler, false, middlewares...)
}

// CONNECT http request.
func (r *Router) CONNECT(pattern string, handler HandlerFunc, middlewares ...Middleware) {
	r.Handle(http.MethodConnect, pattern, handler, false, middlewares...)
}

// ServeHTTP implements the http.Handler interface
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
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

func staticHandler(prefix, dir string, cacheDuration int) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		path := filepath.Join(dir, strings.TrimPrefix(req.URL.Path, prefix))
		ext := filepath.Ext(path)

		setCacheHeaders := func() {
			if cacheDuration > 0 {
				// Set cache control headers with the specified maxAge
				w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(cacheDuration))
			}
		}

		if ServeMinified && slices.Contains(MinExtensions, ext) {
			stat, err := os.Stat(path)
			if err != nil || stat.IsDir() {
				http.NotFound(w, req)
				return
			}

			// TODO: Allow user to customize the minified extension based on the file type
			// This will allow for serving minified files with different extensions.
			// e.g .min.js, .min.css, .tar.gz, .br etc.
			minifiedPath := strings.TrimSuffix(path, ext) + ".min" + ext

			// Check for the minified version of the file
			stat, err = os.Stat(minifiedPath)
			if err == nil && !stat.IsDir() {
				http.ServeFile(w, req, minifiedPath)
				setCacheHeaders()
				return
			}
		}

		setCacheHeaders()

		http.ServeFile(w, req, path)
	}

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

	handler := r.WrapHandler(staticHandler(prefix, dir, cacheDuration))
	r.Handle(http.MethodGet, prefix, handler, true)
}

// Wrapper around http.ServeFile but applies global middleware to the handler.
func (r *Router) File(path, file string) {
	var hf http.HandlerFunc = func(w http.ResponseWriter, req *http.Request) {
		http.ServeFile(w, req, file)
	}

	handler := r.chain(r.globalMiddlewares, r.WrapHandler(hf))
	r.GET(path, handler)
}

func (r *Router) FileFS(fs http.FileSystem, prefix, path string) {
	r.GET(prefix, r.WrapHandler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
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

	r.GET("/favicon.ico", r.WrapHandler(handler))
}

type minifiedFS struct {
	http.FileSystem
}

func (mfs *minifiedFS) Open(name string) (http.File, error) {
	ext := filepath.Ext(name)

	if slices.Contains(MinExtensions, ext) {
		minifiedName := strings.TrimSuffix(name, filepath.Ext(name)) + ".min" + filepath.Ext(name)
		if f, err := mfs.FileSystem.Open(minifiedName); err == nil {
			return f, nil
		}
	}

	// serve the original file
	return mfs.FileSystem.Open(name)
}

// Like Static but for http.FileSystem.
// Use this to serve embedded assets with go/embed.
//
// mux.StaticFS("/static", http.FS(staticFs))
//
// To enable caching, provide maxAge seconds for cache duration.
func (r *Router) StaticFS(prefix string, fs http.FileSystem, maxAge ...int) {
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	cacheDuration := 0
	if len(maxAge) > 0 {
		cacheDuration = maxAge[0]
	}

	if ServeMinified {
		fs = &minifiedFS{fs}
	}

	// Create file server for the http.FileSystem
	var handler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		if cacheDuration > 0 {
			// Set cache control headers with the specified maxAge
			w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(cacheDuration))
		}

		http.FileServer(fs).ServeHTTP(w, r)
	}

	// Apply global middleware
	finalHandler := r.WrapHandler(http.StripPrefix(prefix, handler))
	r.Handle(http.MethodGet, prefix, finalHandler, true)
}

type RedirectOptions struct {
	Status      int               // status code to use for the redirect
	Params      map[string]string // query parameters to add to the redirect URL
	QueryParams map[string]string // query parameters to add to the redirect URL
}

var defaultRedirectOptions = RedirectOptions{
	Status:      http.StatusSeeOther,
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
	Method  string `json:"method,omitempty"` // Http method.
	Path    string `json:"path,omitempty"`   // Registered pattern.
	Handler string `json:"-"`                // Function name for the handler.
}

// RegisteredRoutes returns a list of registered routes in a slice of RouteInfo.
func (r *Router) RegisteredRoutes() []RouteInfo {
	var routes []RouteInfo
	for _, route := range r.routes {
		parts := strings.SplitN(route.prefix, " ", 2)
		routes = append(routes, RouteInfo{Method: parts[0], Path: parts[1], Handler: getFuncName(route.handler)})
	}
	return routes
}

func getFuncName(f interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}
