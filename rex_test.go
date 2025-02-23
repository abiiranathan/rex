package rex_test

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/abiiranathan/rex"
)

func TestRouterServeHTTP(t *testing.T) {
	r := rex.NewRouter()

	r.GET("/test", func(c *rex.Context) error {
		return c.String("test")
	})

	r.GET("/test2", func(c *rex.Context) error {
		return c.String("test2")
	})

	r.GET("/test3", func(c *rex.Context) error {
		return c.String("test3")
	})

	r.POST("/test4", func(c *rex.Context) error {
		return c.String("test4")
	})

	r.PUT("/test5", func(c *rex.Context) error {
		return c.String("test5")
	})

	r.DELETE("/test6", func(c *rex.Context) error {
		return c.String("test6")
	})

	r.PATCH("/test7", func(c *rex.Context) error {
		return c.String("test7")
	})

	r.OPTIONS("/test8", func(c *rex.Context) error {
		return c.String("test8")
	})

	r.HEAD("/test9", func(c *rex.Context) error {
		return c.String("test9")
	})

	r.CONNECT("/test10", func(c *rex.Context) error {
		return c.String("test10")
	})

	r.TRACE("/test11", func(c *rex.Context) error {
		return c.String("test11")
	})

	tests := []struct {
		name     string
		method   string
		path     string
		expected string
	}{
		{"GET", "GET", "/test", "test"},
		{"GET", "GET", "/test2", "test2"},
		{"GET", "GET", "/test3", "test3"},
		{"POST", "POST", "/test4", "test4"},
		{"PUT", "PUT", "/test5", "test5"},
		{"DELETE", "DELETE", "/test6", "test6"},
		{"PATCH", "PATCH", "/test7", "test7"},
		{"OPTIONS", "OPTIONS", "/test8", "test8"},
		{"HEAD", "HEAD", "/test9", "test9"},
		{"CONNECT", "CONNECT", "/test10", "test10"},
		{"TRACE", "TRACE", "/test11", "test11"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			r.ServeHTTP(w, req)
			if w.Body.String() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, w.Body.String())
			}
		})
	}
}

// test 404
func TestRouterNotFound(t *testing.T) {
	r := rex.NewRouter()
	r.GET("/path", func(c *rex.Context) error {
		return c.String("test")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/notfound", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

// Use a derived type. Form processing should still pass.
type Age int

type User struct {
	Name string `form:"name"`
	Age  Age    `form:"age"`
}

// test sending and reading form data
func TestRouterUrlEncodedFormData(t *testing.T) {
	r := rex.NewRouter()
	r.POST("/urlencoded", func(c *rex.Context) error {
		u := User{}
		err := c.BodyParser(&u)
		if err != nil {
			return c.String(err.Error())
		}
		return c.String(u.Name)
	})

	form := url.Values{}
	form.Add("name", "Abiira Nathan")
	form.Add("age", "23")

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/urlencoded"+"?"+form.Encode(), nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "Abiira Nathan" {
		t.Errorf("expected Abiira Nathan, got %s", w.Body.String())
	}
}

// test sending and reading json data
func TestRouterJSONData(t *testing.T) {
	r := rex.NewRouter()

	r.POST("/json", func(c *rex.Context) error {
		u := User{}
		err := c.BodyParser(&u)
		if err != nil {
			return c.String(err.Error())
		}
		return c.JSON(u)
	})

	u := User{
		Name: "Abiira Nathan",
		Age:  23,
	}

	body, err := json.Marshal(u)
	if err != nil {
		t.Error(err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/json", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var u2 User
	json.NewDecoder(w.Body).Decode(&u2)

	if !reflect.DeepEqual(u, u2) {
		t.Errorf("expected %v, got %v", u, u2)
	}

}

func TestBodyParserDerivedTypes(t *testing.T) {
	r := rex.NewRouter()
	r.POST("/json", func(c *rex.Context) error {
		u := User{}
		err := c.BodyParser(&u)
		if err != nil {
			return c.String(err.Error())
		}
		return c.JSON(u)
	})

	u := User{
		Name: "Abiira Nathan",
		Age:  23,
	}

	body, err := json.Marshal(u)
	if err != nil {
		t.Error(err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/json", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var u2 User
	json.NewDecoder(w.Body).Decode(&u2)

	if !reflect.DeepEqual(u, u2) {
		t.Errorf("expected %v, got %v", u, u2)
	}

}

// multipart/form-data
func TestRouterMultipartFormData(t *testing.T) {
	r := rex.NewRouter()
	r.POST("/multipart", func(c *rex.Context) error {
		u := User{}
		err := c.BodyParser(&u)
		if err != nil {
			return c.String(err.Error())
		}
		return c.String(u.Name)
	})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("name", "Abiira Nathan")
	writer.WriteField("age", "23")
	writer.Close()

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/multipart", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "Abiira Nathan" {
		t.Errorf("expected Abiira Nathan, got %s", w.Body.String())
	}
}

// multipart/form-data with file
func TestRouterMultipartFormDataWithFile(t *testing.T) {
	r := rex.NewRouter()
	r.POST("/upload", func(c *rex.Context) error {
		c.Request.ParseMultipartForm(c.Request.ContentLength)
		_, fileHeader, err := c.Request.FormFile("file")
		if err != nil {
			return c.String(err.Error())
		}

		mpf, err := fileHeader.Open()
		if err != nil {
			return c.String(err.Error())
		}
		defer mpf.Close()

		buf := &bytes.Buffer{}
		_, err = buf.ReadFrom(mpf)
		if err != nil {
			return c.String(err.Error())
		}

		_, err = c.Write(buf.Bytes())
		return err
	})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", "test.txt")
	if err != nil {
		t.Error(err)
	}

	_, err = part.Write([]byte("hello world"))
	if err != nil {
		t.Error(err)
	}

	// close writer before creating request
	writer.Close()

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	data, err := io.ReadAll(w.Body)
	if err != nil {
		t.Error(err)
	}

	if string(data) != "hello world" {
		t.Errorf("expected hello world, got %s", string(data))
	}
}

type contextType string

const authContextKey contextType = "auth"

// test route middleware
func TestRouterMiddleware(t *testing.T) {
	r := rex.NewRouter()
	r.Use(func(hf rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			c.Set(authContextKey, "johndoe")
			return hf(c)
		}
	})

	r.GET("/middleware", func(c *rex.Context) error {
		auth, ok := c.Get(authContextKey)
		if !ok {
			c.WriteHeader(http.StatusUnauthorized)
			return c.String("no auth")
		}
		return c.String(auth.(string))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/middleware", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "johndoe" {
		t.Errorf("expected johndoe, got %s", w.Body.String())
	}
}

type XTestKey string

const xTestCtxKey XTestKey = "X-Test"

func TestWrapMiddleware(t *testing.T) {
	httpMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*r = *r.WithContext(context.WithValue(r.Context(), xTestCtxKey, "test"))
			next.ServeHTTP(w, r)
		})
	}

	r := rex.NewRouter()
	r.Use(r.WrapMiddleware(httpMiddleware))

	r.GET("/wrap", func(c *rex.Context) error {
		return c.String(c.Request.Context().Value(xTestCtxKey).(string))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/wrap", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "test" {
		t.Errorf("expected test, got %s", w.Body.String())
	}
}

func TestWrapMiddlewarePropagatesError(t *testing.T) {
	httpMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*r = *r.WithContext(context.WithValue(r.Context(), xTestCtxKey, "test"))
			next.ServeHTTP(w, r)
		})
	}

	r := rex.NewRouter()
	r.Use(r.WrapMiddleware(httpMiddleware))

	var msg = "This error should be propagated"

	r.GET("/", func(c *rex.Context) error {
		c.SetStatus(http.StatusBadRequest)
		return fmt.Errorf("%s", msg)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status %v, got %v", http.StatusBadRequest, w.Result().StatusCode)
	}

	// test: was overwritten
	if w.Body.String() != msg {
		t.Fatalf("expected body: %s, got: %s", msg, w.Body.String())
	}

}

type customResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *customResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *customResponseWriter) Status() int {
	return w.status
}

func (w *customResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// Test Wrap middleware with custom http.ResponseWriter
func TestWrapMiddlewareWithCustomResponseWriter(t *testing.T) {
	logger := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cw := &customResponseWriter{ResponseWriter: w}
			next.ServeHTTP(cw, r)

			fmt.Printf("%s %s %d\n", r.Method, r.URL.Path, cw.Status())
		})
	}

	r := rex.NewRouter()
	r.Use(r.WrapMiddleware(logger))

	r.GET("/wrap", func(c *rex.Context) error {
		return c.String("test")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/wrap", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "test" {
		t.Errorf("expected test, got %s", w.Body.String())
	}

}

const msgKey contextType = "message"

// test chaining of middlewares
func TestRouterChainMiddleware(t *testing.T) {
	r := rex.NewRouter()

	r.Use(func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			c.Set(msgKey, "first")
			return next(c)
		}
	})

	r.Use(func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			message, ok := c.Get(msgKey)
			if !ok {
				c.WriteHeader(http.StatusInternalServerError)
				return c.String("no message")
			}

			c.Set(msgKey, message.(string)+" second")
			return next(c)
		}
	})

	r.With(func(next rex.HandlerFunc) rex.HandlerFunc {
		return func(c *rex.Context) error {
			message, ok := c.Get(msgKey)
			if !ok {
				c.WriteHeader(http.StatusInternalServerError)
				return c.String("no message")
			}

			c.Set(msgKey, message.(string)+" third")
			return next(c)
		}
	}).GET("/chain", func(c *rex.Context) error {
		message, ok := c.Get(msgKey)
		if !ok {
			c.WriteHeader(http.StatusInternalServerError)
			return c.String("no message")
		}

		return c.String(message.(string))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/chain", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "first second third" {
		t.Errorf("expected first second third, got %s", w.Body.String())
	}
}

// test render with a base layout
func TestRouterRenderWithBaseLayout(t *testing.T) {
	templ, err := rex.ParseTemplates(
		"cmd/server/templates",
		template.FuncMap{"upper": strings.ToUpper},
		".html",
	)

	if err != nil {
		panic(err)
	}

	options := []rex.RouterOption{
		rex.BaseLayout("base.html"),
		rex.WithTemplates(templ),
		rex.PassContextToViews(true),
		rex.ContentBlock("Content"),
	}

	r := rex.NewRouter(options...)

	r.GET("/home_page", func(c *rex.Context) error {
		return c.Render("home.html", rex.Map{
			"Title": "Home Page",
			"Body":  "Welcome to the home page",
		})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/home_page", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

}

func CopyDir(src, dst string) error {
	// create the destination directory
	err := os.MkdirAll(dst, 0755)
	if err != nil {
		return err
	}

	// get a list of all the files in the source directory
	files, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	// copy each file to the destination directory
	for _, file := range files {
		srcFile := filepath.Join(src, file.Name())
		dstFile := filepath.Join(dst, file.Name())

		// if the file is a directory, copy it recursively
		if file.IsDir() {
			err = CopyDir(srcFile, dstFile)
			if err != nil {
				return err
			}
		} else {
			// copy the file
			input, err := os.ReadFile(srcFile)
			if err != nil {
				return err
			}
			err = os.WriteFile(dstFile, input, 0644)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func TestRouterStatic(t *testing.T) {
	dirname, err := os.MkdirTemp("", "static")
	if err != nil {
		t.Fatalf("could not create temp dir: %v", err)
	}
	defer os.RemoveAll(dirname)

	file := filepath.Join(dirname, "test.txt")
	err = os.WriteFile(file, []byte("hello world"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	r := rex.NewRouter()
	r.Static("/static", dirname)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/static/notfound.txt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/static/test.txt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	data, err := io.ReadAll(w.Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "hello world" {
		t.Errorf("expected hello world, got %s", string(data))
	}

}

func TestRouterStaticFS(t *testing.T) {
	dirname, err := os.MkdirTemp("", "assests")
	if err != nil {
		t.Fatalf("could not create temp dir: %v", err)
	}
	defer os.RemoveAll(dirname)

	file := filepath.Join(dirname, "test.txt")
	err = os.WriteFile(file, []byte("hello world"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	r := rex.NewRouter()
	r.StaticFS("/static", http.Dir(dirname))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/static/notfound.txt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/static/test.txt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	data, err := io.ReadAll(w.Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "hello world" {
		t.Errorf("expected hello world, got %s", string(data))
	}
}

func TestRouterFile(t *testing.T) {
	// create a temporary directory for the views
	dirname, err := os.MkdirTemp("", "static")
	if err != nil {
		t.Fatalf("could not create temp dir: %v", err)
	}
	defer os.RemoveAll(dirname)

	file := filepath.Join(dirname, "test.txt")
	err = os.WriteFile(file, []byte("hello world"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	r := rex.NewRouter()
	r.File("/static/test.txt", file)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/static/test.txt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	data, err := io.ReadAll(w.Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "hello world" {
		t.Errorf("expected hello world, got %s", string(data))
	}
}

// test rex.Redirect
func TestRouterRedirect(t *testing.T) {
	r := rex.NewRouter()

	r.GET("/redirect1", func(c *rex.Context) error {
		return c.Redirect("/redirect2", http.StatusFound)
	})

	r.GET("/redirect2", func(c *rex.Context) error {
		return c.String("redirect2")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/redirect1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected status 302, got %d", w.Code)
	}

	// test redirect with params and query
	r.GET("/redirect3", func(c *rex.Context) error {
		return c.RedirectRoute("/redirect/{name}", rex.RedirectOptions{
			Status:      http.StatusFound,
			Params:      map[string]string{"name": "redirect3"},
			QueryParams: map[string]string{"name": "abiira"},
		})
	})

	r.GET("/redirect/{name}", func(c *rex.Context) error {
		nameParam := c.Param("name") // Loaded from the redirect route params
		nameQuery := c.Query("name") // Loaded from the redirect query params

		if nameParam != "redirect3" {
			t.Errorf("expected redirect3, got %s", nameParam)
		}

		if nameQuery != "abiira" {
			t.Errorf("expected abiira, got %s", nameQuery)
		}

		return c.String("redirect3")
	})

	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/redirect3", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected status 302, got %d", w.Code)
	}

	body := w.Body.String()
	if body != "redirect3" {
		t.Errorf("expected redirect3, got %s", body)
	}

}

// test redirect route
func TestRouterRedirectRoute(t *testing.T) {
	r := rex.NewRouter()
	r.GET("/redirect_route1", func(c *rex.Context) error {
		return c.RedirectRoute("/redirect_route2", rex.RedirectOptions{Status: http.StatusFound})
	})

	r.GET("/redirect_route2", func(c *rex.Context) error {
		status := c.Status()
		if status != http.StatusFound {
			t.Errorf("expected status 302, got %d", status)
		}
		return c.String("redirect_route2")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/redirect_route1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected status 302, got %d", w.Code)
	}

}

// test Query
func TestRouterQuery(t *testing.T) {
	r := rex.NewRouter()
	r.GET("/query", func(c *rex.Context) error {
		return c.String(c.Query("name", "default"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/query?name=abiira", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "abiira" {
		t.Errorf("expected abiira, got %s", w.Body.String())
	}
}

// test QueryInt
func TestRouterQueryInt(t *testing.T) {
	r := rex.NewRouter()
	r.GET("/queryint", func(c *rex.Context) error {
		return c.String(strconv.Itoa(c.QueryInt("age", 0)))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/queryint?age=23", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "23" {
		t.Errorf("expected 23, got %s", w.Body.String())
	}
}

// test ParamInt
func TestRouterParamInt(t *testing.T) {
	r := rex.NewRouter()
	r.GET("/paramint/{age}", func(c *rex.Context) error {
		return c.String(c.Param("age"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/paramint/30", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "30" {
		t.Errorf("expected 30, got %s", w.Body.String())
	}
}

// Write a benchmark test for the router
func BenchmarkRouter(b *testing.B) {
	r := rex.NewRouter()
	r.GET("/benchmark", func(c *rex.Context) error {
		return c.String("Hello World!")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/benchmark", nil)

	for i := 0; i < b.N; i++ {
		r.ServeHTTP(w, req)
	}
}

// bench mark full request/response cycle
func BenchmarkRouterFullCycle(b *testing.B) {
	r := rex.NewRouter()
	r.GET("/benchmark-cycle", func(c *rex.Context) error {
		return c.String("Hello World!")
	})

	ts := httptest.NewServer(r)
	defer ts.Close()

	for i := 0; i < b.N; i++ {
		res, err := http.Get(ts.URL + "/benchmark-cycle")
		if err != nil {
			b.Fatal(err)
		}
		if res.StatusCode != http.StatusOK {
			b.Fatalf("expected status 200, got %d", res.StatusCode)
		}
	}
}

func TestRouterExecuteTemplate(t *testing.T) {
	templ, err := rex.ParseTemplates("cmd/server/templates",
		template.FuncMap{"upper": strings.ToUpper}, ".html")

	if err != nil {
		panic(err)
	}

	r := rex.NewRouter(rex.WithTemplates(templ))

	r.GET("/template", func(c *rex.Context) error {
		data := rex.Map{
			"Title": "Template",
			"Body":  "Welcome to the template page",
		}

		err := c.ExecuteTemplate("home.html", data)
		if err != nil {
			t.Errorf("execute template failed")
			return err
		}

		// Test lookup template
		templ, err = c.LookupTemplate("home.html")
		if err != nil {
			t.Errorf("expected to find home.html template")
			return err
		}

		out := new(bytes.Buffer)
		err = templ.Execute(out, map[string]any{
			"Title": "Template",
			"Body":  "Named Template",
		})

		if err != nil {
			t.Errorf("execute template failed")
			return err
		}

		if !strings.Contains(out.String(), "Named Template") {
			t.Errorf("expected 'Named Template' in templated page, got %s", out.String())
		}

		return nil
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/template", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// check body
	if !strings.Contains(w.Body.String(), "Welcome to the template page") {
		t.Errorf("expected Welcome to the template page, got %s", w.Body.String())
	}

}

func TestRouterFileFS(t *testing.T) {
	dirname, err := os.MkdirTemp("", "assets")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dirname)

	file := filepath.Join(dirname, "test.txt")
	err = os.WriteFile(file, []byte("hello world"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	r := rex.NewRouter()
	r.FileFS(http.Dir(dirname), "/static", "test.txt")

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/static", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	data, err := io.ReadAll(w.Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "hello world" {
		t.Errorf("expected hello world, got %s", string(data))
	}
}

func TestRouterFaviconFS(t *testing.T) {
	dirname, err := os.MkdirTemp("", "assets")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dirname)

	file := filepath.Join(dirname, "favicon.ico")
	err = os.WriteFile(file, []byte("hello world"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	r := rex.NewRouter()
	r.FaviconFS(http.Dir(dirname), "favicon.ico")

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/favicon.ico", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	data, err := io.ReadAll(w.Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "hello world" {
		t.Errorf("expected hello world, got %s", string(data))
	}
}

// Test serve minified files if available
func TestRouterServeMinifiedAssets(t *testing.T) {
	dirname, err := os.MkdirTemp("", "assets")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dirname)

	file := filepath.Join(dirname, "test.txt")
	err = os.WriteFile(file, []byte("hello world"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	minFile := filepath.Join(dirname, "test.min.txt")
	err = os.WriteFile(minFile, []byte("minified"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	rex.ServeMinified = true
	rex.MinExtensions = append(rex.MinExtensions, ".txt")

	r := rex.NewRouter()

	r.StaticFS("/static", http.Dir(dirname))

	w := httptest.NewRecorder()

	// we should get the minified file
	req := httptest.NewRequest("GET", "/static/test.txt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	data, err := io.ReadAll(w.Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "minified" {
		t.Errorf("expected minified, got %s", string(data))
	}
}

func TestRouterServeMinifiedAssetsStatic(t *testing.T) {
	dirname, err := os.MkdirTemp("", "assets")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dirname)

	file := filepath.Join(dirname, "test.txt")
	err = os.WriteFile(file, []byte("hello world"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	minFile := filepath.Join(dirname, "test.min.txt")
	err = os.WriteFile(minFile, []byte("minified"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	rex.ServeMinified = true
	rex.MinExtensions = append(rex.MinExtensions, ".txt")

	r := rex.NewRouter()

	r.Static("/static", dirname, 60)

	w := httptest.NewRecorder()

	// we should get the minified file
	req := httptest.NewRequest("GET", "/static/test.txt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	data, err := io.ReadAll(w.Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "minified" {
		t.Errorf("expected minified, got %s", string(data))
	}
}

func TestRegisteredRoutes(t *testing.T) {
	r := rex.NewRouter()

	r.GET("/test", func(c *rex.Context) error {
		return c.String("test")
	})

	r.GET("/test2", func(c *rex.Context) error {
		return c.String("test2")
	})

	r.GET("/test3", func(c *rex.Context) error {
		return c.String("test3")
	})

	r.POST("/test4", func(c *rex.Context) error {
		return c.String("test4")
	})

	// test registered routes
	routes := r.RegisteredRoutes()
	if len(routes) != 4 {
		t.Errorf("expected 4 routes, got %d", len(routes))
	}

	for _, route := range routes {
		if !strings.HasPrefix(route.Path, "/test") {
			t.Errorf("expected path to start with /test, got %s", route.Path)
		}
	}
}

func TestSPAHandler(t *testing.T) {
	temp := t.TempDir()

	// Create a simple SPA with 4 files.
	// - index.html
	// - app.js
	// - app.css
	index := filepath.Join(temp, "index.html")
	err := os.WriteFile(index, []byte(`<html><head><link rel="stylesheet" href="/app.css"></head><body><script src="/app.js"></script></body></html>`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	appjs := filepath.Join(temp, "app.js")
	err = os.WriteFile(appjs, []byte(`console.log("Hello World!")`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	appcss := filepath.Join(temp, "app.css")
	err = os.WriteFile(appcss, []byte(`body { background-color: #f0f0f0; }`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	r := rex.NewRouter()

	options := []rex.SPAOption{
		rex.WithCacheControl("public max-age=3600"),
		rex.WithSkipFunc(func(r *http.Request) bool {
			// Return 404 for this pattern.
			return r.URL.Path == "/non-existent-file"
		}),
	}

	r.SPA("/", "index.html", http.Dir(temp), options...)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	// fetch js
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/app.js", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	// fetch css
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/app.css", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	// fetch non-existent file
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/non-existent", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 202, got %d", w.Code)
	}

	// we expect content of index.html
	data, err := io.ReadAll(w.Body)
	if err != nil {
		t.Fatal(err)
	}

	fileContent, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != string(fileContent) {
		t.Errorf("expected %s, got %s", string(fileContent), string(data))
	}

	// non-existent file with extension should return 404
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/non-existent.js", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}

	// test with skip func
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/non-existent-file", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}

}

//go:embed cmd/server/templates
var templates embed.FS

func TestCreateFileSystem(t *testing.T) {
	fs := rex.CreateFileSystem(templates, "cmd/server/templates")
	if fs == nil {
		t.Fatal("expected a file system")
	}

	_, err := fs.Open("home.html")
	if err != nil {
		t.Error(err)
	}
}

func BenchmarkRequestsPerSecond(b *testing.B) {
	// Setup router
	r := rex.NewRouter()
	r.GET("/benchmark", func(c *rex.Context) error {
		return c.String("Hello World!")
	})

	// Create test server
	ts := httptest.NewServer(r)
	defer ts.Close()

	// Create HTTP client with timeouts
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	// Reset timer to exclude setup time
	b.ResetTimer()

	// Run parallel benchmark
	b.SetParallelism(4)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			res, err := client.Get(ts.URL + "/benchmark")
			if err != nil {
				b.Fatal(err)
			}
			if res.StatusCode != http.StatusOK {
				b.Fatalf("expected status 200, got %d", res.StatusCode)
			}

			// Make sure to read and close the body to reuse connections
			_, err = io.ReadAll(res.Body)
			res.Body.Close()
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// Benchmark and calculate requests per second
func TestRequestsPerSecond(t *testing.T) {
	result := testing.Benchmark(BenchmarkRequestsPerSecond)

	// Calculate requests per second
	requestsPerSecond := float64(result.N) / result.T.Seconds()

	t.Logf("Requests per second: %.2f", requestsPerSecond)
	t.Logf("Total requests: %d", result.N)
	t.Logf("Total time: %s", result.T)
	t.Logf("Memory allocations: %d", result.AllocsPerOp())
	t.Logf("Bytes allocated per op: %d", result.AllocedBytesPerOp())
}
