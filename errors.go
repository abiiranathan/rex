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

func (*errorHandler) ValidationErrors(c *Context, errs map[string]string) {
	accept := c.AcceptHeader()
	c.WriteHeader(http.StatusBadRequest)

	switch accept {
	case "application/json":
		c.JSON(errs)
	default:
		{
			var htmlReply strings.Builder
			htmlReply.WriteString(`<div class="rex_error">`)
			for _, value := range errs {
				htmlReply.WriteString(`<p class="rex_error_item">`)
				htmlReply.WriteString(value)
				htmlReply.WriteString("</p>")
			}
			htmlReply.WriteString("</div>")
			c.HTML(htmlReply.String())
		}
	}
}

func (*errorHandler) FormErrors(c *Context, err FormError) {
	accept := c.AcceptHeader()
	c.WriteHeader(http.StatusBadRequest)

	switch accept {
	case "application/json":
		c.JSON(err)
	default:
		{
			var htmlReply strings.Builder
			htmlReply.WriteString(`<div class="rex_error">`)
			htmlReply.WriteString(`<p class="rex_error_item">`)
			htmlReply.WriteString(err.Err.Error())
			htmlReply.WriteString("</p>")
			htmlReply.WriteString("</div>")
			c.HTML(htmlReply.String())
		}
	}
}

func (*errorHandler) GenericErrors(ctx *Context, err error) {
	// Render error template if defined.
	if ctx.router.errorTemplate != "" {
		ctx.RenderError(ctx.Response, err, http.StatusInternalServerError)
	} else {
		// Send raw string of the error
		ctx.WriteHeader(http.StatusInternalServerError)
		ctx.Write([]byte(err.Error()))
	}
}

// Default ErrorHandler for errors returned from handlers.
var defaultErrorHandler ErrorHandler = &errorHandler{}
