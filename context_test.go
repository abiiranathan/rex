package rex

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLocals(t *testing.T) {
	t.Parallel()

	r := NewRouter()

	r.GET("/test", func(c *Context) error {
		c.Set("key", "value")
		return nil
	})

	r.Use(func(hf HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			value, ok := c.Get("key")
			if !ok {
				t.Error("key not found")
			}
			if value != "value" {
				t.Error("value is not correct")
			}

			locals := c.Locals()
			if len(locals) != 1 {
				t.Error("locals length is not correct")
			}

			return hf(c)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Error("status code is not OK")
	}
}

func TestIP(t *testing.T) {
	t.Parallel()

	r := NewRouter()

	r.GET("/test", func(c *Context) error {
		ip, err := c.IP()

		if err != nil {
			t.Error(err)
		}

		expected := "127.0.0.1"

		if ip != expected {
			t.Errorf("expected IP %s, got %s", expected, ip)
		}
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
}

func TestQueryIntWithDefault(t *testing.T) {
	t.Parallel()

	r := NewRouter()

	r.GET("/test", func(c *Context) error {
		v := c.QueryInt("key", 123)
		if v != 123 {
			t.Error("value is not correct")
		}
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/test?key=abc", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Error("status code is not OK")
	}
}

func TestQueryIntWithoutDefault(t *testing.T) {
	t.Parallel()

	r := NewRouter()

	r.GET("/test", func(c *Context) error {
		v := c.QueryInt("key")
		if v != 0 {
			t.Error("value is not correct")
		}
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/test?key=abc", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Error("status code is not OK")
	}
}

func TestParamIntWithDefault(t *testing.T) {
	t.Parallel()

	r := NewRouter()

	r.GET("/test/{key}", func(c *Context) error {
		v := c.ParamInt("key", 123)
		if v != 123 {
			t.Error("value is not correct")
		}
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/test/abc", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Error("status code is not OK")
	}
}

func TestParamIntWithoutDefault(t *testing.T) {
	t.Parallel()

	r := NewRouter()

	r.GET("/test/{key}", func(c *Context) error {
		v := c.ParamInt("key")
		if v != 1234 {
			t.Error("expected: 1234, got:", v)
			return fmt.Errorf("expected: 1234, got: %d", v)
		}
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/test/1234", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Error("status code is not OK")
	}
}

func TestRedirect(t *testing.T) {
	t.Parallel()

	r := NewRouter()

	r.GET("/test", func(c *Context) error {
		return c.Redirect("/new")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Error("status code is not SeeOther")
	}
}

func TestFormValue(t *testing.T) {
	t.Parallel()

	r := NewRouter()

	r.POST("/test", func(c *Context) error {
		v := c.FormValue("key")
		if v != "value" {
			t.Error("value is not correct")
		}
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Form = map[string][]string{
		"key": {"value"},
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Error("status code is not OK")
	}
}

func TestFormFileOperations(t *testing.T) {
	t.Parallel()

	r := NewRouter()

	r.POST("/test", func(c *Context) error {
		f, fh, err := c.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		if f == nil {
			t.Fatal("file is nil")
		}

		// validate the file
		b, err := io.ReadAll(f)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != "hello" {
			t.Error("file content is not correct")
		}

		// Test FormFiles
		files, err := c.FormFiles("file")
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 1 {
			t.Error("files length is not correct")
		}

		// Test SaveFile
		err = c.SaveFile(fh, filepath.Join(t.TempDir(), fh.Filename))
		if err != nil {
			t.Fatal(err)
		}
		return nil
	})

	// create a temp file
	dirname := t.TempDir()
	filename := dirname + "/upload.txt"
	os.WriteFile(filename, []byte("hello"), 0644)

	buf := new(bytes.Buffer)
	writer := multipart.NewWriter(buf)

	// use the same file from the fixtures
	mw, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Error(err)
	}
	file, err := os.Open(filename)
	if err != nil {
		t.Error(err)
	}
	defer file.Close()

	_, err = io.Copy(mw, file)
	if err != nil {
		t.Error(err)
	}

	// close the writer before sending the request
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/test", buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Error("status code is not OK")
	}
}
