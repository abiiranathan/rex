package main

import (
	"embed"
	"log"
	"net/http"
	"text/template"

	"github.com/abiiranathan/rex"
	"github.com/abiiranathan/rex/middleware/auth"
	"github.com/abiiranathan/rex/middleware/logger"
	"github.com/gorilla/securecookie"
)

//go:embed templates
var viewsFS embed.FS

type User struct {
	Username string
	Password string
}

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

// Create a protected handler
func protectedHandler(c *rex.Context) error {
	state, _ := auth.GetAuthState(c.Request, c.Response)
	user := state.(User)
	return c.String("Hello %s", user.Username)
}

func authErrorCallback(c *rex.Context) error {
	return c.Redirect("/login")
}

func renderLoginPage(c *rex.Context) error {
	// if already logged in, redirect home
	if _, authenticated := auth.GetAuthState(c.Request, c.Response); authenticated {
		return c.Redirect("/")
	}

	c.SetHeader("cache-control", "no-cache")
	return c.Render("templates/login.html", rex.Map{})
}

func performLogin(c *rex.Context) error {
	var username, password string
	username = c.FormValue("username")
	password = c.FormValue("password")

	// auth verification here

	user := User{Username: username, Password: password}
	err := auth.SetAuthState(c.Request, c.Response, user)
	if err != nil {
		return err
	}
	return c.Redirect("/protected", http.StatusSeeOther)
}

func logout(c *rex.Context) error {
	auth.ClearAuthState(c.Request, c.Response)
	return c.Redirect("/login")
}

func APiRoutes(c *rex.Context) error {
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

	r.GET("/login", renderLoginPage)
	r.POST("/login", performLogin)

	// Routes below will require cookie auth.
	// if login routes are defined below, we define a skipFunc and ignore them.
	var secretKey = securecookie.GenerateRandomKey(64)
	auth.Register(User{})
	r.Use(auth.Cookie(authErrorCallback, nil, secretKey))

	r.GET("/", HomeHandler)
	r.GET("/about", AboutHandler)
	r.GET("/api", ApiHandler)
	r.GET("/api/routes", APiRoutes)
	r.GET("/doctor", NestedTemplate)
	r.GET("/protected", protectedHandler)
	r.POST("/logout", logout)

	log.Println("Server started on 0.0.0.0:8080")
	srv := rex.NewServer(":8080", r)
	defer srv.Shutdown()
	log.Fatalln(srv.ListenAndServe())
}
