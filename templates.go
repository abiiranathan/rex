package rex

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// BaseLayout sets the base layout template for the router.
// If set, this template will be used as the base layout for all views.
// The `contentBlock` variable will be replaced with the rendered content of the view.
//
// Example:
//
//	r := rex.NewRouter(rex.BaseLayout("layouts/base.html"))
func BaseLayout(baseLayout string) RouterOption {
	return func(r *Router) {
		r.baseLayout = baseLayout
	}
}

// ErrorTemplate sets the error template for the router.
// If set, this template will be used to render errors.
// It is passed "error", "status", "status_text" in its context.
func ErrorTemplate(errorTemplate string) RouterOption {
	return func(r *Router) {
		r.errorTemplate = errorTemplate
	}
}

// ContentBlock sets the name of the content block in the base layout template.
// This block will be replaced with the rendered content of the view.
// The default content block name is "content".
//
// Example:
//
//	r := rex.NewRouter(rex.ContentBlock("main"))
func ContentBlock(contentBlock string) RouterOption {
	return func(r *Router) {
		r.contentBlock = contentBlock
	}
}

// PassContextToViews enables or disables passing the router context to views.
// If enabled, the router context will be available as a variable named "ctx" in the views.
// This allows views to access information about the request and the router.
// The default value is `false`.
//
// Example:
//
//	r := rex.NewRouter(rex.PassContextToViews(true))
func PassContextToViews(passContextToViews bool) RouterOption {
	return func(r *Router) {
		r.passContextToViews = passContextToViews
	}
}

// WithTemplates sets the template for the router.
// This template will be used to render views.
//
// Example:
//
//	t := template.Must(template.ParseFiles("views/index.html"))
//	r := rex.NewRouter(rex.WithTemplates(t))
func WithTemplates(t *template.Template) RouterOption {
	return func(r *Router) {
		r.template = t
	}
}

// render error template with the given error and status code.
func (c *Context) renderErrorTemplate(err error, status ...int) error {
	c.SetHeader("Content-Type", "text/html")

	var statusCode = http.StatusInternalServerError
	if len(status) > 0 {
		statusCode = status[0]
	}

	c.Response.WriteHeader(statusCode)

	if c.router.errorTemplate != "" {
		return c.renderTemplate(c.router.errorTemplate, Map{
			"status":      statusCode,
			"status_text": http.StatusText(statusCode),
			"error":       err,
		})

	}

	_, err = c.Write([]byte(err.Error()))
	return err

}

// RenderError renders the error template with the given error and status code.
func (c *Context) RenderError(w http.ResponseWriter, err error, status ...int) error {
	return c.renderErrorTemplate(err, status...)
}

// builderPool is a pool of strings.Builder to avoid allocations.
var builderPool = sync.Pool{
	New: func() any {
		return new(strings.Builder)
	},
}

// renderTemplate renders the template with the given name and data.
func (c *Context) renderTemplate(name string, data Map) error {
	// Add extension only if necessary
	if filepath.Ext(name) == "" {
		name += ".html"
	}

	c.SetHeader("Content-Type", "text/html")

	if c.router.baseLayout != "" {
		// Get a builder from the pool
		builder := builderPool.Get().(*strings.Builder)

		defer func() {
			builder.Reset()
			builderPool.Put(builder)
		}()

		// Execute the template into the pooled builder
		if err := c.router.template.ExecuteTemplate(builder, name, data); err != nil {
			return err
		}

		// Update the data map with the rendered content
		data[c.router.contentBlock] = template.HTML(builder.String())

		// Reset the builder for reuse
		builder.Reset()

		// Execute the base template
		if err := c.router.template.ExecuteTemplate(builder, c.router.baseLayout, data); err != nil {
			return err
		}

		// Write the final content
		_, err := io.WriteString(c.Response, builder.String())
		return err
	} else {
		return c.router.template.ExecuteTemplate(c.Response, name, data)
	}
}

// Render the template tmpl with the data. If no template is configured, Render will panic.
// data is a map such that it can be extended with
// the request context keys if passContextToViews is set to true.
// If a file extension is missing, it will be appended as ".html".
func (c *Context) Render(name string, data Map) error {
	if c.router.template == nil {
		return fmt.Errorf("no template is configured: unable to render template %q", name)
	}

	// pass the request context to the views
	if c.router.passContextToViews {
		for k, v := range c.locals {
			data[fmt.Sprintf("%v", k)] = v
		}
	}
	return c.renderTemplate(name, data)
}

// Execute a standalone template without a layout.
func (c *Context) ExecuteTemplate(name string, data Map) error {
	if c.router.template == nil {
		return fmt.Errorf("no template is configured: unable to render template %q", name)
	}

	// pass the request context to the views
	if c.router.passContextToViews {
		for k, v := range c.locals {
			data[fmt.Sprintf("%v", k)] = v
		}
	}
	return c.router.template.ExecuteTemplate(c.Response, name, data)
}

// Template returns the template passed to the router.
func (c *Context) Template() (*template.Template, error) {
	if c.router.template == nil {
		return nil, fmt.Errorf("no template is configured")
	}
	return c.router.template, nil
}

// LookupTemplate returns the template with the given name.
func (c *Context) LookupTemplate(name string) (*template.Template, error) {
	if c.router.template == nil {
		return nil, fmt.Errorf("no template is configured")
	}
	return c.router.template.Lookup(name), nil
}

// ParseTemplates recursively parses all the templates in the given directory and returns a template.
// The funcMap is applied to all the templates. The suffix is used to filter the files.
// The default suffix is ".html".
// If you have a file system, you can use ParseTemplatesFS instead.
func ParseTemplates(rootDir string, funcMap template.FuncMap, suffix ...string) (*template.Template, error) {
	ext := ".html"
	if len(suffix) > 0 {
		ext = suffix[0]
	}

	cleanRoot := filepath.Clean(rootDir)
	pfx := len(cleanRoot) + 1
	root := template.New("")

	err := filepath.WalkDir(cleanRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && strings.HasSuffix(path, ext) {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			t := root.New(path[pfx:]).Funcs(funcMap)
			_, err = t.Parse(string(b))
			return err
		}
		return nil
	})

	return root, err
}

// ParseTemplatesFS parses all templates in a directory recursively from a given filesystem.
// It uses the specified `funcMap` to define custom template functions.
// The `suffix` argument can be used to specify a different file extension for the templates.
// The default file extension is ".html".
//
// Example:
//
//		t, err := rex.ParseTemplatesFS(
//	 	http.FS(http.Dir("templates")), "templates", template.FuncMap{
//						"now": func() time.Time { return time.Now() },
//			}, ".tmpl")
//
//		 if err != nil {
//		   log.Fatal(err)
//		 }
//
//		 r := rex.NewRouter(rex.WithTemplates(t))
func ParseTemplatesFS(root fs.FS, rootDir string, funcMap template.FuncMap, suffix ...string) (*template.Template, error) {
	ext := ".html"
	if len(suffix) > 0 {
		ext = suffix[0]
	}

	pfx := len(rootDir) + 1      // +1 for the trailing slash
	tmpl := template.New("root") // Create a new template

	err := fs.WalkDir(root, rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && strings.HasSuffix(path, ext) {
			if d != nil && d.IsDir() {
				return nil
			}

			b, err := fs.ReadFile(root, path)
			if err != nil {
				return err
			}

			t := tmpl.New(rootDir + "/" + path[pfx:]).Funcs(funcMap)
			_, err = t.Parse(string(b))

			return err
		}
		return nil
	})
	return tmpl, err
}

// Must unwraps the value and panics if the error is not nil.
func Must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
