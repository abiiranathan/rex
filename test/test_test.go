package test_test

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/abiiranathan/rex"
	"github.com/abiiranathan/rex/test"
)

func TestTestHandler_WithTemplate(t *testing.T) {
	// Create a dummy handler that renders a template
	handler := func(c *rex.Context) error {
		return c.Render("index.html", rex.Map{"Name": "World"})
	}

	// Create templates using helper
	tmpl := test.TemplateFromString(map[string]string{
		"index.html": "Hello {{.Name}}",
	})

	// Create request
	req := test.NewRequest("GET", "/", nil)

	// Run TestHandler
	resp, err := test.TestHandler(handler, req, rex.WithTemplates(tmpl))
	if err != nil {
		t.Fatalf("TestHandler failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "Hello World" {
		t.Errorf("expected body 'Hello World', got '%s'", string(body))
	}
}

func TestTestHandler_WithParam(t *testing.T) {
	// Handler using param
	handler := func(c *rex.Context) error {
		return c.String("Hello " + c.Param("name"))
	}

	req := test.NewRequest("GET", "/hello/rex", nil)
	test.SetPathValue(req, "name", "rex")

	resp, err := test.TestHandler(handler, req)
	if err != nil {
		t.Fatalf("TestHandler failed: %v", err)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "Hello rex" {
		t.Errorf("expected 'Hello rex', got '%s'", string(body))
	}
}

func TestTest_Integration(t *testing.T) {
	r := rex.NewRouter()
	r.GET("/ping", func(c *rex.Context) error {
		return c.String("pong")
	})

	req := test.NewRequest("GET", "/ping", nil)
	resp, err := test.Test(r, req)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong" {
		t.Errorf("expected 'pong', got '%s'", string(body))
	}
}

func TestTestHandler_Error(t *testing.T) {
	handler := func(c *rex.Context) error {
		return fmt.Errorf("oops")
	}

	req := test.NewRequest("GET", "/", nil)
	_, err := test.TestHandler(handler, req)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.Error() != "oops" {
		t.Errorf("expected error 'oops', got '%v'", err)
	}
}
