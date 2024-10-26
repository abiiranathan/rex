package rex

import (
	"log"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"
)

func HandleValidationErrors(c *Context, errs validator.ValidationErrors) {
	log.Println("handling validation errors")

	accept := strings.Split(c.Request.Header.Get("Accept"), ";")[0]
	c.WriteHeader(http.StatusBadRequest)

	switch accept {
	case "application/json":
		c.JSON(errs.Translate(c.router.translator))
	default:
		{
			var htmlReply strings.Builder
			htmlReply.WriteString(`<div class="rex_error">`)
			for _, value := range errs.Translate(c.router.translator) {
				htmlReply.WriteString(`<p class="rex_error_item">`)
				htmlReply.WriteString(value)
				htmlReply.WriteString("</p>")
			}
			htmlReply.WriteString("</div>")
			c.HTML(htmlReply.String())
		}
	}
}

func HandleFormErrors(c *Context, err FormError) {
	log.Println("handling form errors")
	accept := strings.Split(c.Request.Header.Get("Accept"), ";")[0]
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
