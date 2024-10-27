# rex.auth

Package auth provides session-based authentication middleware for the Rex router.
It uses secure cookie sessions to maintain authentication state and supports storing
custom user state in the session.

## Installation
```bash
go get -u github.com/abiiranathan/rex
```

Basic usage:
```go
	// First, register your user type for session storage
	auth.Register(User{})

	// Create the middleware with authentication key
	authMiddleware := auth.Cookie(auth.CookieConfig{
		KeyPairs: [][]byte{[]byte("your-32-byte-auth-key")},
		ErrorHandler: func(c *rex.Context) error {
			return c.Status(http.StatusUnauthorized).JSON(map[string]string{
				"error": "Unauthorized",
			})
		},
	})

	// Use the middleware in your router
	router := rex.NewRouter()
	router.Use(authMiddleware)
```
Login example:

```go
	router.Post("/login", func(c *rex.Context) error {
		user := &User{ID: 1, Name: "John"}
		if err := auth.SetAuthState(c, user); err != nil {
			return err
		}
		return c.JSON(user)
	})

```

Access authenticated user:

```go
	router.Get("/me", func(c *rex.Context) error {
		state, authenticated := auth.GetAuthState(c)
		if !authenticated {
			return c.Status(http.StatusUnauthorized)
		}
		user := state.(*User)
		return c.JSON(user)
	})

```

Logout example:

```go
	router.Post("/logout", func(c *rex.Context) error {
		return auth.ClearAuthState(c)
	})

```

Security Notes:
  - Cookie sessions are encrypted and authenticated using the provided key pairs
  - HttpOnly and SameSite=Strict are enforced for security
  - Default session duration is 24 hours
  - Use cryptographically secure random bytes for key pairs
  - For production, use https://pkg.go.dev/crypto/rand to generate keys

Key Generation Example:

```go
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(err)
	}
```

For key rotation, you can provide multiple key pairs:

```go
	authMiddleware := auth.Cookie(auth.CookieConfig{
		KeyPairs: [][]byte{
			[]byte("new-32-byte-auth-key"),
			[]byte("new-32-byte-encrypt-key"),
			[]byte("old-32-byte-auth-key"),
			[]byte("old-32-byte-encrypt-key"),
		},
		// ... other config
	})
```

Custom cookie options:

```go
	authMiddleware := auth.Cookie(auth.CookieConfig{
		KeyPairs: [][]byte{[]byte("your-32-byte-auth-key")},
		Options: &sessions.Options{
			Path:     "/",
			Domain:   "example.com",
			MaxAge:   3600,
			Secure:   true,
		},
		// ... other config
	})
```

Skip authentication for specific routes:

```go
	authMiddleware := auth.Cookie(auth.CookieConfig{
		KeyPairs: [][]byte{[]byte("your-32-or-64-byte-auth-key")},
		SkipAuth: func(req *http.Request) bool {
			return req.URL.Path == "/login" || req.URL.Path == "/signup"
		},
		// ... other config
	})

```

It also provides middleware for BasicAuth, JWT auth.
Oauth2 support is coming soon.