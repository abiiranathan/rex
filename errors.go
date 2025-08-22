package rex

import (
	"net/http"
	"strings"
)

// Interface that allows for rex to call user-defined functions with errors
// returns from handlers.
type ErrorHandler interface {
	// Passes translated validation errors in map[string]string.
	// The keys are field names. Values are the error messages.
	ValidationErrors(c *Context, errs map[string]string)

	// Passes FormError as a result of calling c.BodyParser() when parsing the form.
	FormErrors(c *Context, err FormError)

	// Generic errors that are not ValidationErrors or FormErrors are passed here
	// to the caller.
	GenericErrors(c *Context, err error)
}

type errorHandler struct{}

func writeHtmlError(c *Context, errs map[string]string) {
	var htmlReply strings.Builder
	htmlReply.WriteString(`<div class="rex_error">`)
	for _, value := range errs {
		htmlReply.WriteString(`<p class="rex_error_item">`)
		htmlReply.WriteString(value)
		htmlReply.WriteString("</p>")
	}
	htmlReply.WriteString("</div>")
	_ = c.HTML(htmlReply.String())
}

func (*errorHandler) ValidationErrors(c *Context, errs map[string]string) {
	accept := c.AcceptHeader()
	contentType := c.ContentType()
	c.WriteHeader(http.StatusBadRequest)
	if accept == ContentTypeJSON || contentType == ContentTypeJSON {
		_ = c.JSON(errs)
	} else {
		writeHtmlError(c, errs)
	}
}

func (*errorHandler) FormErrors(c *Context, err FormError) {
	accept := c.AcceptHeader()
	contentType := c.ContentType()

	c.WriteHeader(http.StatusBadRequest)

	if accept == ContentTypeJSON || contentType == ContentTypeJSON {
		_ = c.JSON(err)
	} else {
		var htmlReply strings.Builder
		htmlReply.WriteString(`<div class="rex_error">`)
		htmlReply.WriteString(`<p class="rex_error_item">`)
		htmlReply.WriteString(err.Err.Error())
		htmlReply.WriteString("</p>")
		htmlReply.WriteString("</div>")
		_ = c.HTML(htmlReply.String())
	}
}

func (*errorHandler) GenericErrors(ctx *Context, err error) {
	statusCode := ctx.StatusCode()
	if statusCode <= http.StatusBadRequest {
		statusCode = http.StatusInternalServerError
	}

	accept := ctx.AcceptHeader()
	contentType := ctx.ContentType()

	if accept == ContentTypeJSON || contentType == ContentTypeJSON {
		ctx.WriteHeader(statusCode)
		_ = ctx.JSON(Map{"error": err.Error()})
	} else {
		isHtmx := ctx.Request.Header.Get("HX-Request") == "true"
		if isHtmx || ctx.router.errorTemplate == "" {
			ctx.WriteHeader(statusCode)
			_, _ = ctx.Write([]byte(err.Error()))
		} else {
			_ = ctx.RenderError(ctx.Response, err, statusCode)
		}
	}
}

// Default ErrorHandler for errors returned from handlers.
var defaultErrorHandler ErrorHandler = &errorHandler{}
