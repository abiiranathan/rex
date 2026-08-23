# rex

<img src="./rex.jpg" alt="logo" width="200"/>

**rex** is a minimalistic but robust HTTP router and micro-framework built on Go 1.22+'s enhanced `http.ServeMux`. It adds the batteries stdlib leaves out — middleware chains, groups, centralized error handling, form parsing with validation, template inheritance, app state, and a suite of production middlewares — without giving up `http.ServeMux` routing semantics.

```bash
go get -u github.com/abiiranathan/rex
```

---

## Quick start

```go
package main

import (
    "net/http"

    "github.com/abiiranathan/rex"
    "github.com/abiiranathan/rex/middleware/recovery"
)

type App struct{ Greeting string }

func main() {
    r := rex.NewRouter(rex.WithState(&App{Greeting: "Hello"}))
    r.Use(recovery.New())

    r.GET("/hello/{name}", func(c *rex.Context) error {
        app := c.State().(*App)
        return c.String(app.Greeting + ", " + c.Param("name") + "!\n")
    })

    r.GET("/api/hello", func(c *rex.Context) error {
        return c.JSON(rex.Map{"hello": "world"})
    })

    panic(r.ListenAndServe())
}
```

Handlers are just `func(c *rex.Context) error` — return an error and rex's centralized error handling produces consistent JSON/text/HTML responses (htmx-aware), logs it, and moves on.

---

## Why rex instead of net/http?

`net/http` (with 1.22 pattern routing) is excellent, and rex deliberately builds **on top of it** rather than replacing it. What you gain:

| Concern | net/http alone | rex |
|---|---|---|
| Handler errors | Manual `http.Error` everywhere | Return `error`; one place handles, formats & logs |
| Middleware | DIY `func(http.Handler) http.Handler` chains | First-class per-router / group / route middleware |
| Request data access | Parse query/forms by hand each time | Typed accessors (`c.QueryInt`, `ParamUint`, ...), cached parsing |
| Body binding + validation | Hand-rolled decode + validate | One call: `BodyParser` (JSON/XML/urlencoded/multipart) with `validator` tags and localized errors |
| Templates | Assemble layouts yourself | Base-layout inheritance, funcmaps, embedded FS helpers, pooled rendering |
| App state | Global vars or context keys | `WithState` injection with type-safe `GetState[T]` |
| Static files / SPA | `http.FileServer` wiring | `Static`, `StaticFS`, minified-variant serving, single-method SPA hosting with fallback |
| Graceful shutdown | Hand-rolled signal handling | `NewServer` with timeouts, HTTP/2 config, TLS helpers |
| Testing | `httptest` boilerplate | `test.Test(router, req)` helpers |
| Observability | Bring your own | slog logging (sync or async queue), request IDs, recovery, ETag, rate limiting |

And what you keep:

- Standard `http.ServeMux` pattern syntax (`GET /users/{id}`, `{path...}` wildcards) and precedence rules.
- Every handler converts to/from plain `net/http`: `r.WrapHandler`, `r.WrapMiddleware`, `r.ToHandler`. Existing stdlib middleware drops straight in.

### Compared to other frameworks

| | rex | chi | gin | echo | fiber |
|---|---|---|---|---|---|
| Router core | stdlib `ServeMux` | custom trie | custom radix | custom radix | fasthttp |
| `net/http` compatible handlers | native | yes | adapters needed | adapters needed | no (fasthttp) |
| Handlers return `error` | yes | no | no | yes | no |
| Centralized typed errors + validation translation | yes | no | partial | partial | partial |
| Form binding incl. multipart + custom types | yes | no | partial | partial | partial |
| Built-in template inheritance | yes | no | no | no | no |
| Async log queue | yes | no | no | no | no |

The honest trade-offs: rex's stdlib-based router is slower than gin/fiber's radix trees at extreme route counts, and fiber trades `net/http` compatibility for raw throughput. rex optimizes for correctness, stdlib alignment, and developer ergonomics over microsecond bragging rights.

---

## Features

- **Middleware support**: apply globally, per group, or per route (`r.With(...).Use(...)`).
- **Route groups and subgroups** with their own middleware.
- **Centralized error handling**: structured `rex.Error` types, content negotiation (JSON/text/HTML), customizable via `SetErrorHandler`.
- **Body parsing**: JSON, XML, url-encoded and multipart forms; standard types, pointers, slices, `time.Time`, and custom types implementing `rex.FormScanner`.
- **Validation**: `go-playground/validator/v10` with English translations out of the box; register custom tags.
- **Application state**: shared dependency injection like Rust routers.
- **Template rendering** with base-layout inheritance and pass-through of request context to views.
- **SPA support**: `r.SPA(pattern, index, fs)` serves your frontend with history-API fallback.
- **Graceful server**: HTTP/2 options, TLS loading, read/write/idle timeouts.

### Application state (like axum's `with_state`)

```go
type App struct{ DB *sql.DB }

r := rex.NewRouter(rex.WithState(&App{DB: db}))

r.GET("/users/{id}", func(c *rex.Context) error {
    app, ok := rex.GetState[*App](c)
    if !ok {
        return rex.NewError(500, "state not configured")
    }
    _ = app.DB
    return nil
})
```

State is shared across all requests — make it safe for concurrent use, exactly like an `Arc<T>` in Rust.

---

## Built-in middleware

| Package | Purpose |
|---|---|
| `middleware/logger` | slog request logging, text/JSON, sync or async queue |
| `middleware/recovery` | panic recovery with stack traces, pluggable handlers |
| `middleware/requestid` | `X-Request-ID` generation/propagation |
| `middleware/security` | secure headers (XSS, nosniff, frame, HSTS, CSP, Referrer-Policy) |
| `middleware/cors` | configurable CORS |
| `middleware/csrf` | cookie+form CSRF tokens, constant-time comparison |
| `middleware/etag` | SHA1 ETags with If-None-Match/If-Match |
| `middleware/gzip` | gzip compression with levels & skip paths |
| `middleware/brotli` | brotli compression with documented level tradeoffs |
| `middleware/ratelimit` | token-bucket limiter, per-key, shared managers |
| `middleware/auth` | cookie sessions (key rotation, sliding refresh), Basic Auth, JWT |
| `middleware/flash` | flash messages via cookies |
| `sse` | server-sent events streaming with keepalives |

---

## Performance

Measured with the repo's benchmarks (`go test -bench=. -benchmem .`) on linux/amd64; indicative relative figures rather than absolute guarantees.

| Scenario | ns/op | B/op | allocs/op |
|---|---|---|---|
| Raw `http.ServeMux` (floor) | 224 | 88 | 3 |
| rex minimal route | 3,679* | 473 | 7 |
| rex + 3 query params parsed | 3,769 | 465 | 7 |
| rex JSON response | 3,979 | 497 | 8 |
| rex with logger middleware (async) | 5,723 | 1,051 | 16 |
| rex with async router logging | 1,273 | 465 | 5 |

\* The default router logs every request synchronously (~2.5µs of that figure is the stdout write). Enable `rex.WithAsyncLogging()` to move logging off the request path entirely.

Design notes: contexts and encoding buffers are pooled; query parsing is cached per request; SSE event encoding is allocation-free in steady state (~0.3 allocs/message); JSON responses set exact Content-Length to avoid chunked encoding.

---

## Rendering templates

For a complete example of template rendering and router usage, see the example in [cmd/server/main.go](./cmd/server/main.go).

### Custom types implementing FormScanner

```go
type Date time.Time // Date in format YYYY-MM-DD

// FormScan implements the FormScanner interface.
func (d *Date) FormScan(value any) error {
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

## Asynchronous logging

Move the router's internal request/error logging off the request path with a bounded background queue:

```go
r := rex.NewRouter(rex.WithAsyncLogging(0)) // 0 = default queue size (4096)
defer r.CloseLogQueue()                     // flush during graceful shutdown
```

Records are enqueued without blocking; when the queue is full they are dropped (never slowing down requests). The drop count is available via `r.LogQueueDropped()`. `rex.AsyncLogHandler` is also available as a standalone `slog.Handler`. The [logger middleware](./middleware/logger) supports the same behavior via its `Async` config field.

---

## Tests

Run all tests with the following command:

```bash
go test -v ./...
```

Run benchmarks with memory profiling enabled:

```bash
go test -bench=. ./... -benchmem
```

---

## Contributing

Pull requests are welcome! For major changes, please open an issue to discuss your ideas first.
Don't forget to update tests as needed.

---

## License

MIT — see [LICENSE](./LICENSE).
