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
	"time"

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

// WrapHandler wraps an http.Handler to be used as a rex.HandlerFunc
func (r *Router) WrapHandler(h http.Handler) HandlerFunc {
	return func(c *Context) error {
		// Set the request context to the rex context (which implements context.Context)
		// This allows handlers to access locals via the context interface if needed.
		// Note: c.Request.Context() is c.ctx (parent).
		// We wrap it so c is the context.
		c.Request = c.Request.WithContext(c)

		h.ServeHTTP(c.Response, c.Request)

		// If an error was set in request context, return it
		// We can access it directly from locals if it was set via SetError (which now uses locals)
		// or check the fallback.
		if v, exists := c.locals[string(contextErrorKey)]; exists {
			if err, ok := v.(error); ok {
				return err
			}
		}

		// Check underlying context just in case
		// Note: contextErrorKey is contextError(string).
		// We use string(contextErrorKey) for locals map.
		errValue := c.Request.Context().Value(contextErrorKey)
		if err, isError := errValue.(error); isError {
			return err
		}
		// Propagate context error if any
		return c.err
	}
}

// Key for errors set inside http handlers without access to the context.
// Allows errors to be propagated via the request context.
type contextError string

// Key to store errors in context.
var contextErrorKey = contextError("context_error_key")

// SetError sets the error inside the request context
// that is retrieved and passed down the rex.Context.
// Useful for http handlers without access to rex.Context.
func SetError(r *http.Request, err error) {
	// If the context is already a *Context, set it directly in locals
	if ctx, ok := r.Context().(*Context); ok {
		ctx.Set(string(contextErrorKey), err)
		return
	}
	*r = *r.WithContext(context.WithValue(r.Context(), contextErrorKey, err))
}

// Convert rex.HandlerFunc to http.Handler.
func (r *Router) ToHandler(h HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := r.InitContext(w, req)
		defer r.PutContext(ctx)

		err := h(ctx)
		r.errorHandlerFunc(ctx, err)
	})
}

// WrapMiddleware wraps an http middleware to be used as a rex middleware.
func (router *Router) WrapMiddleware(middleware func(http.Handler) http.Handler) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c.router = router

				// The middleware receives 'w', which might be a wrapper around c.Response.
				// We need to temporarily set c.Response to 'w' so that 'next' writes to the correct writer.
				old := c.Response
				c.Response = w
				c.Request = r
				defer func() { c.Response = old }()

				c.err = next(c)
			})

			handler = middleware(handler)

			// ServeHTTP with the CURRENT response writer.
			// The middleware will wrap it and call our inner handler with the wrapped writer.
			handler.ServeHTTP(c.Response, c.Request)

			// If an error was set in request context, return it
			// We can access it directly from locals if it was set via SetError (which now uses locals)
			// or check the fallback.
			if v, exists := c.locals[string(contextErrorKey)]; exists {
				if err, ok := v.(error); ok {
					return err
				}
			}

			// Check underlying context just in case
			// Note: contextErrorKey is contextError(string).
			// We use string(contextErrorKey) for locals map.
			errValue := c.Request.Context().Value(contextErrorKey)

			if err, isError := errValue.(error); isError {
				return err
			}

			// Propagate context error if any
			return c.err
		}
	}
}

// Router is the main router structure
type Router struct {
	mux               *http.ServeMux    // http.ServeMux
	routes            map[string]*route // map of routes
	globalMiddlewares []Middleware      // global middlewares

	errorHandlerFunc func(*Context, error) // centralized error handler
	errHandler       ErrorHandler          // Interface satisfying ErrorHandler, called inside errorHandlerFunc

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

	// loggerCallback returns an even number of user arguments to be added to the slog.Logger arguments
	loggerCallback func(c *Context) []any

	// skipLog skips logging the request if it returns true
	skipLog func(c *Context) bool
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

// WithLoggerCallback sets a callback function that should return an even number of
// additional user args to be appended to slog.Log arguments.
func WithLoggerCallback(callback func(c *Context) []any) RouterOption {
	return func(r *Router) {
		r.loggerCallback = callback
	}
}

// Skip logging this request if skipLog returns true.
func SkipLog(skipLog func(c *Context) bool) RouterOption {
	return func(r *Router) {
		r.skipLog = skipLog
	}
}

// GetLogger returns the *slog.Logger instance.
func (c *Context) GetLogger() *slog.Logger {
	return c.router.logger
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
		routes:             make(map[string]*route),
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
		errHandler: defaultErrorHandler,
		errorHandlerFunc: func(c *Context, err error) {
			// Log the error on exit to ensure that the correct status code is set.
			defer func() {
				// Skip logging if this returns true
				if c.router.skipLog != nil && c.router.skipLog(c) {
					return
				}

				args := make([]any, 0, 10)
				if err != nil {
					args = append(args, "error", err.Error())
				}

				args = append(args, "latency", c.Latency().String(), "method", c.Method(), "status", c.StatusCode(), "path", c.Path())

				if c.router.loggerCallback != nil {
					userArgs := c.router.loggerCallback(c)
					args = append(args, userArgs...)
				}

				if err != nil {
					c.router.logger.Error("", args...)
				} else {
					c.router.logger.Debug("", args...)
				}
			}()

			// We must return early if there is no error.
			if err == nil {
				return
			}

			var rexErr *Error
			if ve, ok := err.(validator.ValidationErrors); ok {
				rexErr = ValidationErr(c.TranslateErrors(ve))
			} else if fe, ok := err.(FormError); ok {
				rexErr = FormErr(fe)
			} else {
				// For generic errors, wrap it in our new Error struct.
				// Use existing status code from context if it's an error status, otherwise default to 500.
				var defaultStatusCode = http.StatusInternalServerError
				if c.StatusCode() >= http.StatusBadRequest && c.StatusCode() <= http.StatusNetworkAuthenticationRequired {
					defaultStatusCode = c.StatusCode()
				}
				rexErr = &Error{Code: defaultStatusCode, WrappedError: err}
			}
			c.router.errHandler.Handle(c, rexErr)
		},
	}

	// Create translator
	en := en.New()
	uni := ut.New(en, en)
	trans, _ := uni.GetTranslator("en")

	// Connect ut to our validator
	if err := en_translations.RegisterDefaultTranslations(r.validator, trans); err == nil {
		r.translator = trans
	} else {
		r.translator = nil
	}

	for _, option := range options {
		option(r)
	}

	if r.template != nil && r.baseLayout != "" && r.contentBlock == "" {
		panic("contentBlock is required when using a base layout")
	}
	return r
}

// RegisterValidation adds a validation with the given tag
//
// NOTES: - if the key already exists, the previous validation function will be replaced.
// - this method is not thread-safe it is intended that these all be registered prior
// to any validation
func (r *Router) RegisterValidation(tag string, fn validator.Func) error {
	return r.validator.RegisterValidation(tag, fn, true)
}

// RegisterValidationCtx does the same as RegisterValidation on accepts a
// FuncCtx validation allowing context.Context validation support.
func (r *Router) RegisterValidationCtx(tag string, fn validator.FuncCtx) error {
	return r.validator.RegisterValidationCtx(tag, fn, true)
}

// Set error handler for centralized error handling.
func (r *Router) SetErrorHandler(handler ErrorHandler) {
	if handler != nil {
		r.errHandler = handler
	}
}

// Global middleware
func (r *Router) Use(middlewares ...Middleware) {
	r.globalMiddlewares = append(r.globalMiddlewares, middlewares...)
}

// Pool for reusing context objects
var ctxPool = sync.Pool{
	New: func() any {
		return &Context{
			locals: make(map[string]any),
		}
	},
}

// Get a context from the pool
func (r *Router) getContext() *Context {
	return ctxPool.Get().(*Context)
}

// Put the context back in the pool.
func (r *Router) PutContext(c *Context) {
	c.reset()
	ctxPool.Put(c)
}

// Get context from the pool and initialize it with request and writer.
func (r *Router) InitContext(w http.ResponseWriter, req *http.Request) *Context {
	c := r.getContext()
	c.Request = req
	c.Response = &ResponseWriter{
		writer: w,
		status: http.StatusOK,
	}
	c.router = r
	c.ctx = req.Context() // Capture parent context
	return c
}

// Reset the context
func (c *Context) reset() {
	c.Request = nil
	c.Response = nil
	c.router = nil
	c.currentRoute = nil
	c.ctx = nil
	// clear map but preserve capacity
	for k := range c.locals {
		delete(c.locals, k)
	}
}

// handle registers a new route with the given path and handler
func (r *Router) handle(method, pattern string, handler HandlerFunc, is_static bool, middlewares ...Middleware) *route {
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
	newRoute := &route{
		prefix:      routePattern,
		handler:     final,
		middlewares: middlewares,
		router:      r,
	}

	r.routes[routePattern] = newRoute

	r.mux.HandleFunc(routePattern, func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()

		ctx := r.InitContext(w, req)
		defer r.PutContext(ctx)

		// Track the current route.
		ctx.currentRoute = newRoute

		// Inject rex.Context into the request so it's available downstream
		ctx.Request = ctx.Request.WithContext(ctx)

		var skipBody bool
		if req.Method != method {
			// Allow HEAD requests for GET routes as this is allowed by the new Go 1.22 router.
			allowed := method == http.MethodGet && req.Method == http.MethodHead
			if !allowed {
				ctx.WriteHeader(http.StatusMethodNotAllowed)
				r.errorHandlerFunc(ctx, fmt.Errorf("method not allowed"))
				return
			}

			// Skip the body for HEAD requests
			skipBody = true
		}

		// Router logic
		ctx.SetSkipBody(skipBody)

		// Execute the handler and handle any errors
		err := final(ctx)

		end := time.Now()

		latency := end.Sub(start)
		ctx.setLatency(latency)

		// Call the error handler if an error is returned or not
		// This allows the errorHandler to handle errors that are not returned by the handler.
		// e.g. errors that occur in the middleware.
		// Also logging should be done in the errorHandler because the correct status code is set there.
		r.errorHandlerFunc(ctx, err)
	})

	return newRoute
}

// Common HTTP method handlers
func (r *Router) GET(pattern string, handler HandlerFunc) {
	r.handle(http.MethodGet, pattern, handler, false)
}

func (r *Router) POST(pattern string, handler HandlerFunc) {
	r.handle(http.MethodPost, pattern, handler, false)
}

func (r *Router) PUT(pattern string, handler HandlerFunc) {
	r.handle(http.MethodPut, pattern, handler, false)
}

func (r *Router) PATCH(pattern string, handler HandlerFunc) {
	r.handle(http.MethodPatch, pattern, handler, false)
}

func (r *Router) DELETE(pattern string, handler HandlerFunc) {
	r.handle(http.MethodDelete, pattern, handler, false)
}

// OPTIONS. This may not be necessary as registering GET request automatically registers OPTIONS.
func (r *Router) OPTIONS(pattern string, handler HandlerFunc) {
	r.handle(http.MethodOptions, pattern, handler, false)
}

// HEAD request.
func (r *Router) HEAD(pattern string, handler HandlerFunc) {
	r.handle(http.MethodHead, pattern, handler, false)
}

// TRACE http request.
func (r *Router) TRACE(pattern string, handler HandlerFunc) {
	r.handle(http.MethodTrace, pattern, handler, false)
}

// CONNECT http request.
func (r *Router) CONNECT(pattern string, handler HandlerFunc) {
	r.handle(http.MethodConnect, pattern, handler, false)
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
	r.handle(http.MethodGet, prefix, handler, true)
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
		_, _ = w.Write(data)
	})

	r.GET("/favicon.ico", r.WrapHandler(handler))
}

type minifiedFS struct {
	http.FileSystem
}

func (mfs *minifiedFS) Open(name string) (http.File, error) {
	ext := filepath.Ext(name)
	if slices.Contains(MinExtensions, ext) {
		minifiedName := fmt.Sprintf("%s.min%s", strings.TrimSuffix(name, ext), ext)
		if f, err := mfs.FileSystem.Open(minifiedName); err == nil {
			return f, nil
		}
		// Fallback to serving original file instead or error-ing.
	}
	// serve the original file
	return mfs.FileSystem.Open(name)
}

// Like Static but for http.FileSystem.
// Example:
//
//	app.StaticFS("/static", rex.CreateFileSystem(embedfs, "static"), 3600)
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
	r.handle(http.MethodGet, prefix, finalHandler, true)
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

func getFuncName(f any) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}
