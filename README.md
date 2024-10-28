# rex

**rex** is a minimalistic yet robust HTTP router built on Go 1.22’s enhanced `http.ServeMux`. It offers a range of features for rapid web application development, including:

- **Middleware Support**: Apply middleware globally or to specific routes and groups.
- **Helper Methods**: Simplify defining routes and working with request/response handlers.
- **Template Rendering**: Includes automatic template inheritance (e.g., using a base template).
- **Body Parsing**: Supports decoding multiple content types:
  - JSON
  - XML
  - URL-encoded and multipart forms  
  Works with standard Go types, pointers, slices, and custom types implementing the `rex.FormScanner` interface.
- **SPA Support**:  
  Use `r.SPA` to serve a single-page application.
- **Route Grouping and Subgroups**:  
  Apply middleware to groups or individual routes for better organization.
- **Built-in Middleware**:
  - **Logging**: Uses Go’s `slog` package.
  - **Panic Recovery**: Gracefully handles panics.
  - **ETag Support**: For caching optimization.
  - **CORS Handling**: Cross-Origin Resource Sharing middleware.
  - **Seesion based cookie auth, Basic Auth & JWT Middleware**: Secure your routes with seesion, basic or token-based authentication.
- **Custom Middleware**:  
  Implement your own middleware by wrapping `rex.Handler`.
- **Static File Serving**:  
  Use `r.Static` to serve static files from a directory or `r.StaticFS` to serve files from a `http.FileSystem`.
  > Both of these method can serve the minified version of the files if present and rex.ServeMinifiedAssetsIfPresent is set to true.
You can also easily convert standard HTTP handlers to `rex` handlers:
- Use `rex.WrapHandler` to wrap a `http.Handler`.  
- Use `rex.WrapFunc` to wrap a `http.HandlerFunc`.  
- Centralize error handling and logging by overriding the routers error handler. The default error handler handles `rex.FormError` from `r.BodyParser` and validator.ValidationErrors if there is a validation error.
---

## Installation

```bash
go get -u github.com/abiiranathan/rex
```

---

## Example: Custom Type Implementing `FormScanner`

This example shows how to implement a custom type that satisfies the `FormScanner` interface.

```go
type Date time.Time // Date in format YYYY-MM-DD

// FormScan implements the FormScanner interface.
func (d *Date) FormScan(value interface{}) error {
	v, ok := value.(string)
	if !ok {
		return fmt.Errorf("date value is not a string")
	}

	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return fmt.Errorf("invalid date format")
	}
	*d = Date(t)
	return nil
}
```

---

## Rendering Templates

For a complete example of template rendering and router usage, see the example in [cmd/server/main.go](./cmd/server/main.go).

---

## Middleware

**rex** includes a few external libraries, used only in the middleware subpackage. Explore the middleware package for more details and usage examples.

---

## Tests

Run all tests with the following command:

```bash
go test -v ./...
```

---

## Benchmarks

Run benchmarks with memory profiling enabled:

```bash
go test -bench=. ./... -benchmem
```

---

## Contributing

Pull requests are welcome! For major changes, please open an issue to discuss your ideas first.  
Don’t forget to update tests as needed.

---

## License

This project is licensed under the [MIT License](https://choosealicense.com/licenses/mit/).