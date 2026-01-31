package rex

import (
	"fmt"
	"net/http"
)

// Error defines a structured error for use within Rex.
type Error struct {
	Code           int               `json:"-"` // HTTP status code for the error
	Message        string            `json:"-"` // General error message, can be translated
	Fields         map[string]string `json:"-"` // Field-specific validation errors
	FormKind       FormErrorKind     `json:"-"` // Specific kind of form error
	FormField      string            `json:"-"` // Name of the form field that caused the error
	WrappedError   error             `json:"-"` // Original error for internal logging/debugging
	TranslationKey string            `json:"-"` // Key for looking up translated messages
}

// ErrorResponse represents the uniform JSON error response structure.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains the actual error information.
type ErrorDetail struct {
	Message    string            `json:"message"`              // Human-readable error message
	Code       string            `json:"code,omitempty"`       // Error code for programmatic handling
	Details    map[string]any    `json:"details,omitempty"`    // Additional error context
	Validation *ValidationDetail `json:"validation,omitempty"` // Validation-specific errors
	Form       *FormDetail       `json:"form,omitempty"`       // Form-specific errors
}

// ValidationDetail contains field-level validation errors.
type ValidationDetail struct {
	Fields map[string]string `json:"fields"` // Field name -> error message
}

// FormDetail contains form-specific error information.
type FormDetail struct {
	Kind  FormErrorKind `json:"kind"`            // Type of form error
	Field string        `json:"field,omitempty"` // Field that caused the error
}

// Error makes Error implement the error interface.
func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}

	// Prioritize form errors if present
	if e.FormKind != "" {
		msg := fmt.Sprintf("Form error: Kind=%s", e.FormKind)
		if e.FormField != "" {
			msg = fmt.Sprintf("%s, Field=%s", msg, e.FormField)
		}
		if e.WrappedError != nil {
			msg = fmt.Sprintf("%s, Err=%s", msg, e.WrappedError.Error())
		}
		return msg
	}

	if e.WrappedError != nil {
		return e.WrappedError.Error()
	}
	return "An unexpected error occurred"
}

// ToResponse converts the Error to a uniform ErrorResponse structure.
func (e *Error) ToResponse() ErrorResponse {
	detail := ErrorDetail{Message: e.Error()}

	// Add validation details if present
	if len(e.Fields) > 0 {
		detail.Validation = &ValidationDetail{
			Fields: e.Fields,
		}
	}

	// Add form details if present
	if e.FormKind != "" {
		detail.Form = &FormDetail{
			Kind:  e.FormKind,
			Field: e.FormField,
		}
	}

	return ErrorResponse{Error: detail}
}

// ValidationErr creates a new validation error.
func ValidationErr(fields map[string]string) *Error {
	return &Error{
		Code:    http.StatusBadRequest,
		Message: "Validation failed",
		Fields:  fields,
	}
}

// FormErr creates a new form error.
func FormErr(formErr FormError) *Error {
	return &Error{
		Code:         http.StatusBadRequest,
		Message:      formErr.Err.Error(),
		FormKind:     formErr.Kind,
		FormField:    formErr.Field,
		WrappedError: formErr.Err,
	}
}

// NewError creates a new generic error with a given message and optional status code.
func NewError(code int, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// NewErrorWrap creates a new generic error wrapping an existing error.
func NewErrorWrap(code int, message string, err error) *Error {
	return &Error{
		Code:         code,
		Message:      message,
		WrappedError: err,
	}
}

// ErrorHandler interface allows for custom error handling.
type ErrorHandler interface {
	Handle(c *Context, err error)
}

type errorHandler struct{}

func (*errorHandler) Handle(c *Context, err error) {
	rexErr, ok := err.(*Error)
	if !ok {
		rexErr = &Error{
			Code:         http.StatusInternalServerError,
			Message:      "Internal server error",
			WrappedError: err,
		}
	}

	statusCode := rexErr.Code
	if statusCode == 0 {
		statusCode = http.StatusInternalServerError
	}

	accept := c.AcceptHeader()
	contentType := c.ContentType()

	c.WriteHeader(statusCode)

	if accept == ContentTypeJSON || contentType == ContentTypeJSON {
		// Always return uniform error response structure
		c.JSON(rexErr.ToResponse())
	} else {
		// HTML response when there is no error template or htmx request
		isHtmx := c.Request.Header.Get("HX-Request") == "true"
		if isHtmx || c.router.errorTemplate == "" {
			c.Write([]byte(rexErr.Error()))
		} else {
			// Render error using the configured template
			c.RenderError(c.Response, rexErr, statusCode)
		}
	}
}

// Default ErrorHandler for errors returned from handlers.
var defaultErrorHandler ErrorHandler = &errorHandler{}
