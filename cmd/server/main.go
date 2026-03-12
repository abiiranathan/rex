package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"
	"text/template"
	"time"

	"github.com/abiiranathan/rex"
	"github.com/abiiranathan/rex/middleware/auth"
	"github.com/abiiranathan/rex/middleware/logger"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
)

//go:embed templates
var viewsFS embed.FS

// User represents the authenticated session user in the example server.
type User struct {
	Username string
	Password string
}

// HomeHandler renders the home page template.
func HomeHandler(c *rex.Context) error {
	return c.Render("templates/home.html", rex.Map{
		"Title": "Home Page",
		"Body":  "Welcome to the home page",
	})
}

// AboutHandler renders the about page template.
func AboutHandler(c *rex.Context) error {
	data := rex.Map{
		"Title": "About Page",
		"Body":  "Welcome to the about page",
	}
	return c.Render("templates/about.html", data)
}

// NestedTemplate renders the nested doctor template.
func NestedTemplate(c *rex.Context) error {
	return c.Render("templates/doctor/doctor.html", rex.Map{})
}

// APIHandler returns example JSON data.
func APIHandler(c *rex.Context) error {
	todos := []struct {
		Title     string
		Completed bool
		Author    string
	}{
		{
			Title:     "Working on my portfolio",
			Completed: true,
			Author:    "Abiira Nathan",
		},
		{
			Title:     "Adding route groups in rex",
			Completed: false,
			Author:    "Abiira Nathan",
		},
	}

	return c.JSON(todos)
}

// Create a protected handler
func protectedHandler(cookieAuth *auth.CookieAuth) rex.HandlerFunc {
	return func(c *rex.Context) error {
		state := cookieAuth.Value(c)
		user := state.(User)
		res := fmt.Sprintf("Hello %s", user.Username)
		return c.String(res)
	}
}

func authErrorCallback(c *rex.Context) error {
	return c.Redirect("/login")
}

func renderLoginPage(cookieAuth *auth.CookieAuth) rex.HandlerFunc {
	return func(c *rex.Context) error {
		// if already logged in, redirect home
		if v := cookieAuth.Value(c); v != nil {
			return c.Redirect("/")
		}

		c.SetHeader("cache-control", "no-cache")
		return c.Render("templates/login.html", rex.Map{})
	}
}

func performLogin(cookieAuth *auth.CookieAuth) rex.HandlerFunc {
	return func(c *rex.Context) error {
		var username, password string
		username = c.FormValue("username")
		password = c.FormValue("password")

		// auth verification here

		user := User{Username: username, Password: password}
		err := cookieAuth.SetState(c, user)
		if err != nil {
			return err
		}
		return c.Redirect("/protected", http.StatusSeeOther)
	}
}

func logout(cookieAuth *auth.CookieAuth) rex.HandlerFunc {
	return func(c *rex.Context) error {
		cookieAuth.Clear(c)
		return c.Redirect("/login")
	}
}

// APIRoutes returns the registered routes as JSON.
func APIRoutes(c *rex.Context) error {
	return c.JSON(c.Router().RegisteredRoutes())
}

func main() {
	templ := rex.Must(rex.ParseTemplatesFS(viewsFS, "templates", template.FuncMap{}, ".html"))

	routerOPtions := []rex.RouterOption{
		rex.WithTemplates(templ),
		rex.BaseLayout("templates/base.html"),
		rex.ContentBlock("Content"),
		rex.PassContextToViews(true),
	}

	r := rex.NewRouter(routerOPtions...)
	r.Use(logger.New(nil))

	// Routes below will require cookie auth.
	// if login routes are defined below, we define a skipFunc and ignore them.
	var secretKey = securecookie.GenerateRandomKey(64)
	cookieAuth, err := auth.NewCookieAuth("rex_session_name", [][]byte{secretKey}, User{}, auth.CookieConfig{
		Options: &sessions.Options{
			MaxAge:   int((24 * time.Hour).Seconds()),
			Secure:   false,
			SameSite: http.SameSiteStrictMode,
		},
		ErrorHandler: authErrorCallback,
		SkipAuth: func(c *rex.Context) bool {
			return c.Path() == "/login"
		},
	})
	if err != nil {
		log.Fatalln("Failed to initialize cookie auth:", err)
	}
	r.Use(cookieAuth.Middleware())

	r.GET("/login", renderLoginPage(cookieAuth))
	r.POST("/login", performLogin(cookieAuth))

	r.GET("/", HomeHandler)
	r.GET("/about", AboutHandler)
	r.GET("/api", APIHandler)
	r.GET("/api/routes", APIRoutes)
	r.GET("/doctor", NestedTemplate)
	r.GET("/protected", protectedHandler(cookieAuth))
	r.POST("/logout", logout(cookieAuth))

	log.Println("Server started on 0.0.0.0:8080")
	srv, err := rex.NewServer(":8080", r)
	if err != nil {
		log.Fatalln(err)
	}
	defer srv.Shutdown()
	log.Fatalln(srv.ListenAndServe())
}
