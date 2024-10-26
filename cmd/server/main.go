package main

import (
	"embed"
	"log"
	"net/http"
	"text/template"

	"github.com/abiiranathan/rex"
	"github.com/gorilla/sessions"
)

//go:embed templates
var viewsFS embed.FS

// base.html is automatically added to every template.
// {{ .Content }} is replaced with page contents.
// No need for {{ template "base.html" . }} in every page.
func HomeHandler(c *rex.Context) error {
	return c.Render("templates/home.html", rex.Map{
		"Title": "Home Page",
		"Body":  "Welcome to the home page",
	})
}

func AboutHandler(c *rex.Context) error {
	data := rex.Map{
		"Title": "About Page",
		"Body":  "Welcome to the about page",
	}
	return c.Render("templates/about.html", data)
}

func NestedTemplate(c *rex.Context) error {
	return c.Render("templates/doctor/doctor.html", rex.Map{})
}

func ApiHandler(c *rex.Context) error {
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

// For more persistent sessions, use a database store.
// e.g https://github.com/antonlindstrom/pgstore
var store = sessions.NewCookieStore([]byte("secret"))

// Create a protected handler
func protectedHandler(c *rex.Context) error {
	session, _ := store.Get(c.Request, "session-name")
	if session.Values["authenticated"] != true {
		return c.WriteHeader(http.StatusUnauthorized)
	}

	name := session.Values["user"]
	_, n := c.Write([]byte("Hello " + name.(string)))
	return n
}

func SessionMiddleware(next rex.HandlerFunc) rex.HandlerFunc {
	return func(c *rex.Context) error {
		session, _ := store.Get(c.Request, "session-name")
		if session.Values["authenticated"] != true {
			return c.Redirect("/login", http.StatusSeeOther)
		}
		return next(c)
	}
}

func loginGetHandler(c *rex.Context) error {
	return c.Render("templates/login.html", rex.Map{})
}

func main() {
	templ, err := rex.ParseTemplatesFS(viewsFS, "templates", template.FuncMap{}, ".html")
	if err != nil {
		panic(err)
	}

	r := rex.NewRouter(
		rex.WithTemplates(templ),
		rex.PassContextToViews(true),
		rex.BaseLayout("templates/base.html"),
		rex.ContentBlock("Content"),
	)

	r.GET("/", HomeHandler)
	r.GET("/about", AboutHandler)
	r.GET("/api", ApiHandler)
	r.GET("/doctor", NestedTemplate)

	// Create a basic auth middleware
	r.GET("/login", loginGetHandler)

	r.POST("/login", func(c *rex.Context) error {
		var username, password string
		username = c.Request.FormValue("username")
		password = c.Request.FormValue("password")

		if username == "admin" && password == "admin" {
			session, _ := store.Get(c.Request, "session-name")
			session.Values["authenticated"] = true
			session.Values["user"] = username
			session.Save(c.Request, c.Response)
			return c.Redirect("/protected", http.StatusSeeOther)
		}

		return c.WriteHeader(http.StatusUnauthorized)
	})

	r.GET("/protected", protectedHandler, SessionMiddleware)
	r.GET("/users/{username}", func(c *rex.Context) error {
		username := c.Param("username")
		return c.String("Hello %s", username)
	})

	srv := rex.NewServer(":8080", r)
	defer srv.Shutdown()

	log.Fatalln(srv.ListenAndServe())
}
