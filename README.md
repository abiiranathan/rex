# rex

Package **rex** (go router) implements a minimalistic but robust http router based on the standard go 1.22 enhanced routing capabilities in the `http.ServeMux`. It adds features like middleware support, helper methods for defining routes, template rendering with automatic template inheritance (of a base template).

It also has a BodyParser that decodes json, xml, url-encoded and multipart forms
based on content type. Form parsing supports all standard go types(and their pointers)
and slices of standard types. 
It also supports custom types that implement the `rex.FormScanner` interface.

**rex** supports single page application routing with a dedicated method `r.SPAHandler` that serves the index.html file for all routes that do not match a file or directory in the root directory of the SPA.

The router also supports route groups and subgroups with middleware that can be applied to the entire group or individual routes.

It has customizable built-in middleware for logging using the slog package, panic recovery, etag, cors, basic auth and jwt middlewares.

More middlewares can be added by implementing the Middleware type, a standard function that wraps rex.Handler.

To convert a standard http.Handler to a HandlerFunc, use the `rex.WrapHandler` function.
To convert a standard http.HandlerFunc to a HandlerFunc, use the `rex.WrapFunc` function.

See the middleware package for examples.

## Installation

```bash
go get -u github.com/abiiranathan/rex
```

### Example of a custom type that implements the FormScanner interface
```go
type FormScanner interface {
	FormScan(value interface{}) error
}

type Date time.Time // Date in format YYYY-MM-DD

// FormScan implements the FormScanner interface
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


## Rendering Templates
See the [example](./cmd/server/main.go) for a complete example of how to use the rex package.

> Only a few external libraries are used in the middleware subpackage.

## Tests
    
```bash
go test -v ./...
```

## Benchmarks

```bash
go test -bench=. ./... -benchmem
```

## Contributing

Pull requests are welcome. For major changes, please open an issue first to discuss what you would like to change.
Please make sure to update tests as appropriate.

## License

[MIT](https://choosealicense.com/licenses/mit/)
