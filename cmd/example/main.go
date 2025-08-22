package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"
	"text/template"
	"time"

	"github.com/abiiranathan/rex"
	"github.com/abiiranathan/rex/middleware/cors"
	"github.com/abiiranathan/rex/middleware/csrf"
	"github.com/abiiranathan/rex/middleware/logger"
	"github.com/abiiranathan/rex/middleware/recovery"
	"github.com/gorilla/sessions"
)

//go:embed static/*
var static embed.FS

func main() {
	t, err := rex.ParseTemplatesFS(static, "static", template.FuncMap{})
	if err != nil {
		panic(err)
	}

	// Create a new router
	rex.NoTrailingSlash = true
	rex.ServeMinified = true

	mux := rex.NewRouter(
		rex.WithTemplates(t),
		rex.PassContextToViews(true),
	)

	mux.Use(recovery.New(recovery.WithStackTrace(true)))
	mux.Use(logger.New(logger.DefaultConfig))
	// mux.Use(etag.New())
	mux.Use(cors.New())

	// Create a cookie store.
	var store = sessions.NewCookieStore([]byte("secret key"))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   0,
		Domain:   "localhost",
		Secure:   false,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}

	mux.Use(csrf.New(store, false))
	mux.StaticFS("/static", http.FS(static))

	mux.GET("/test/{id}/", func(c *rex.Context) error {
		return c.Redirect("/redirect")
	})

	mux.GET("/redirect", func(c *rex.Context) error {
		return c.String("Redirected")
	})

	mux.GET("/api", func(c *rex.Context) error {
		c.WriteHeader(http.StatusBadRequest)
		return c.JSON(map[string]any{"error": "This is an error"})
	})

	mux.GET("/", func(c *rex.Context) error {
		return c.Render("static/index.html", rex.Map{})
	})

	mux.POST("/login", func(c *rex.Context) error {
		username := c.Request.FormValue("username")
		password := c.Request.FormValue("password")

		// log the csrf token
		fmt.Println(c.Request.FormValue("csrf_token"))
		res := fmt.Sprintf("Username: %s, Password: %s", username, password)
		return c.String(res)
	})

	mux.FaviconFS(http.FS(static), "static/favicon.ico")

	opts := []rex.ServerOption{
		rex.WithReadTimeout(time.Second * 10),
		rex.WithWriteTimeout(time.Second * 15),
	}

	server, err := rex.NewServer(":8000", mux, opts...)
	if err != nil {
		log.Fatalln(err)
	}
	defer server.Shutdown()

	log.Printf("Listening on %v\n", server.Addr)
	log.Fatalln(server.ListenAndServe())
}
