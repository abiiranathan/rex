package rex

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/pkg/errors"
)

// Context represents the context of the current HTTP request
type Context struct {
	Request      *http.Request       // Original Request object
	Response     http.ResponseWriter // Wrapped Writer
	router       *Router             // Instance of the Router.
	locals       map[any]any         // Per-request context data
	mu           sync.RWMutex        // Mutex for the locals map
	err          error               // Tracks any error encountered in middleware
	currentRoute *route              // The current route.
}

// SetHeader sets a header in the response
func (c *Context) SetHeader(key, value string) {
	if wrapped, ok := c.Response.(*ResponseWriter); ok {
		wrapped.writer.Header().Set(key, value)
	} else {
		c.Response.Header().Set(key, value)
	}
}

// DelHeader deletes a header in the response
func (c *Context) DelHeader(key string) {
	if wrapped, ok := c.Response.(*ResponseWriter); ok {
		wrapped.writer.Header().Del(key)
	} else {
		c.Response.Header().Del(key)
	}
}

// GetHeader returns the status code of the response
func (c *Context) GetHeader(key string) string {
	return c.Request.Header.Get(key)
}

// Status sets the status code of the response and returns the context
// allowing for chaining.
func (c *Context) Status(status int) *Context {
	c.Response.WriteHeader(status)
	return c
}

// Context helper methods
// JSON sends a JSON response
func (c *Context) JSON(data any) error {
	c.Response.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(c.Response).Encode(data)
}

// XML sends an XML response
func (c *Context) XML(data any) error {
	c.Response.Header().Set("Content-Type", "application/xml")
	return xml.NewEncoder(c.Response).Encode(data)
}

// String sends a string response
func (c *Context) String(text string) error {
	c.Response.Header().Set("Content-Type", "text/plain")
	_, err := c.Response.Write([]byte(text))
	return err
}

// Returns the header content type stripping everything after ; like
// charset or form boundary in multipart/form-data forms.
func (c *Context) ContentType() string {
	return strings.Split(c.Request.Header.Get("Content-Type"), ";")[0]
}

// Accepts returns the best match from the Accept header.
func (c *Context) AcceptHeader() string {
	accept := c.Request.Header.Get("Accept")

	// accept header may contain multiple values and encoding types
	return strings.Split(accept, ",")[0]
}

// Send HTML response.
func (c *Context) HTML(html string) error {
	c.Response.Header().Set("Content-Type", "text/html")
	_, err := c.Response.Write([]byte(html))
	return err
}

func (c *Context) WriteHeader(status int) {
	c.Response.WriteHeader(status)
}

// Write sends a raw response
func (c *Context) Write(data []byte) (int, error) {
	return c.Response.Write(data)
}

// Send sends a raw response and returns an error.
// This conveniently returns only the error from the response writer.
func (c *Context) Send(data []byte) error {
	_, err := c.Response.Write(data)
	return err
}

// Error sends an error response as plain text.
// You can optionally pass a content type.
// Status code is expected to be between 400 and 599.
func (c *Context) Error(err error, status int, contentType ...string) {
	if status < 400 || status > 599 {
		log.Println("status code must be between 400 and 599")
		return
	}

	if len(contentType) > 0 && contentType[0] != "" {
		c.Response.Header().Set("Content-Type", contentType[0])
	} else {
		c.Response.Header().Set("Content-Type", "text/plain")
	}

	c.Response.WriteHeader(status)
	_, _ = c.Response.Write([]byte(err.Error()))
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

// ParamUint returns the value of the parameter as an unsigned integer
func (c *Context) ParamUint(key string, defaults ...uint) uint {
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
	return uint(vInt)
}

// ParamInt64 returns the value of the parameter as an int64.
func (c *Context) ParamInt64(key string, defaults ...int64) int64 {
	v := c.Param(key)
	if v == "" && len(defaults) > 0 {
		return defaults[0]
	}

	vInt, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		if len(defaults) > 0 {
			return defaults[0]
		}
		return 0
	}
	return vInt
}

// Query returns the value of the query as a string.
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

// queryInt returns the value of the query as an int64
func (c *Context) QueryInt64(key string, defaults ...int64) int64 {
	v := c.Query(key)
	if v == "" && len(defaults) > 0 {
		return defaults[0]
	}

	vInt, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		if len(defaults) > 0 {
			return defaults[0]
		}
		return 0
	}
	return vInt
}

// QueryUInt returns the value of the query as an unsigned integer
func (c *Context) QueryUInt(key string, defaults ...uint) uint {
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
	return uint(vInt)
}

// Set stores a value in the context
func (c *Context) Set(key any, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.locals[key] = value

	// Also set the value in the request context
	ctx := context.WithValue(c.Request.Context(), key, value)
	*c.Request = *c.Request.WithContext(ctx)

}

// Get retrieves a value from the context
func (c *Context) Get(key any) (value any, exists bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, exists = c.locals[key]
	return
}

// MustGet retrieves a value from the context or panics if the key does not exist.
func (c *Context) MustGet(key any) any {
	value, exists := c.Get(key)
	if !exists {
		panic("key not found")
	}
	return value
}

// GetOrEmpty retrieves a value from the context or returns nil if the key does not exist.
// This better when you want to type-cast the value to a specific type without checking for existence.
func (c *Context) GetOrEmpty(key any) any {
	value, exists := c.Get(key)
	if !exists {
		return nil
	}
	return value
}

// Locals returns the context values
func (c *Context) Locals() map[any]any {
	return c.locals
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

// Returns English translated errors for validation errors in map[string]string.
func (c *Context) TranslateErrors(errs validator.ValidationErrors) map[string]string {
	return errs.Translate(c.router.translator)
}

// Returns the form value by key.
func (c *Context) FormValue(key string) string {
	return c.Request.FormValue(key)
}

// Returns the form value by key as an integer.
// If the value is not found or cannot be converted to an integer, it returns the default value.
func (c *Context) FormValueInt(key string, defaults ...int) int {
	v := c.FormValue(key)
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

// Returns the form value by key as an unsigned integer.
// If the value is not found or cannot be converted to an unsigned integer, it returns the default value.
func (c *Context) FormValueUInt(key string, defaults ...uint) uint {
	v := c.FormValue(key)
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
	return uint(vInt)
}

func (c *Context) FormFile(key string) (multipart.File, *multipart.FileHeader, error) {
	return c.Request.FormFile(key)
}

func (c *Context) FormFiles(key string, maxMemory ...int64) ([]*multipart.FileHeader, error) {
	var memory int64 = 10 << 20 // 10 MB
	if len(maxMemory) > 0 {
		memory = maxMemory[0]
	}
	err := c.Request.ParseMultipartForm(memory)
	if err != nil {
		return nil, err
	}

	if files, ok := c.Request.MultipartForm.File[key]; ok {
		return files, nil
	}

	return nil, fmt.Errorf("no files match the given key")
}

// save file from multipart form to disk
func (c *Context) SaveFile(fh *multipart.FileHeader, target string) error {
	src, err := fh.Open()
	if err != nil {
		return errors.Wrap(err, "failed to open multipart.FileHeader)")
	}
	defer src.Close()

	out, err := os.Create(target)
	if err != nil {
		return errors.Wrap(err, "failed to create file")
	}
	defer out.Close()

	_, err = io.Copy(out, src)
	return err
}

// Returns the status code of the response.
func (c *Context) StatusCode() int {
	if wrapped, ok := c.Response.(*ResponseWriter); ok {
		return wrapped.status
	} else {
		return 0
	}
}

// Latency returns the duration of the request including the time it took to write the response,
// execute the middleware and the handler.
func (c *Context) Latency() time.Duration {
	if wrapped, ok := c.Response.(*ResponseWriter); ok {
		return wrapped.latency
	} else {
		return 0
	}
}

// Returns the *rex.Router instance.
func (c *Context) Router() *Router {
	return c.router
}

// Path returns the request path.
func (c *Context) Path() string {
	return c.Request.URL.Path
}

// Method returns the request method.
func (c *Context) Method() string {
	return c.Request.Method
}

// Host returns the request host.
func (c *Context) Host() string {
	return c.Request.Host
}

// URL returns the request URL.
func (c *Context) URL() string {
	return c.Request.URL.String()
}
