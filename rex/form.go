package rex

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"mime/multipart"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	ContentTypeJSON          string = "application/json"
	ContentTypeXML           string = "application/xml"
	ContentTypeUrlEncoded    string = "application/x-www-form-urlencoded"
	ContentTypeMultipartForm string = "multipart/form-data"
	ContentTypeHTML          string = "text/html"
	ContentTypeCSV           string = "text/csv"
	ContentTypeText          string = "text/plain"
	ContentTypeEventStream   string = "text/event-stream"
)

// FormError represents an error encountered during body parsing.
type FormError struct {
	// The original error encountered.
	Err error
	// The kind of error encountered.
	Kind FormErrorKind

	// Struct field name causing error.
	Field string
}

// FormErrorKind represents the kind of error encountered during body parsing.
type FormErrorKind string

const (
	// InvalidContentType indicates an unsupported content type.
	InvalidContentType FormErrorKind = "invalid_content_type"
	// InvalidStructPointer indicates that the provided v is not a pointer to a struct.
	InvalidStructPointer FormErrorKind = "invalid_struct_pointer"
	// RequiredFieldMissing indicates that a required field was not found.
	RequiredFieldMissing FormErrorKind = "required_field_missing"
	// UnsupportedType indicates that an unsupported type was encountered.
	UnsupportedType FormErrorKind = "unsupported_type"
	// ParseError indicates that an error occurred during parsing.
	ParseError FormErrorKind = "parse_error"
)

// Error implements the error interface.
func (e FormError) Error() string {
	if wrappedError, ok := e.Err.(FormError); ok {
		return fmt.Sprintf("BodyParser error: field=%q kind=%s, err=%s", wrappedError.Field, wrappedError.Kind, wrappedError.Err)
	}
	return fmt.Sprintf("BodyParser error: field=%q kind=%s, err=%s", e.Field, e.Kind, e.Err)
}

var DefaultTimezone = time.UTC

// BodyParser parses the request body and stores the result in v.
// v must be a pointer to a struct.
// If timezone is provided, all date and time fields in forms are parsed with the provided location info.
// Otherwise rex.DefaultTimezone is used and defaults to UTC.
//
// Supported content types: application/json, application/x-www-form-urlencoded, multipart/form-data, application/xml
// For more robust form decoding we recommend using
// https://github.com/gorilla/schema package.
// Any form value can implement the FormScanner interface to implement custom form scanning.
// Struct tags are used to specify the form field name.
// If parsing forms, the default tag name is "form",
// followed by the "json" tag name, and then snake case of the field name.
func (c *Context) BodyParser(v interface{}, loc ...*time.Location) error {
	r := c.Request
	// Make sure v is a pointer to a struct
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return FormError{
			Err:   fmt.Errorf("v must be a pointer to a struct"),
			Kind:  InvalidStructPointer,
			Field: "",
		}
	}

	contentType := c.ContentType()
	timezone := DefaultTimezone
	if len(loc) > 0 && loc[0] != nil {
		timezone = loc[0]
	}

	if contentType == ContentTypeJSON {
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(v)
		if err != nil {
			return FormError{
				Err:  err,
				Kind: ParseError,
			}
		}
		return nil
	} else if contentType == ContentTypeUrlEncoded || contentType == ContentTypeMultipartForm {
		var form *multipart.Form
		var err error
		if contentType == ContentTypeMultipartForm {
			err = r.ParseMultipartForm(r.ContentLength)
			if err != nil {
				return FormError{
					Err:  err,
					Kind: ParseError,
				}
			}
			form = r.MultipartForm
		} else {
			err = r.ParseForm()
			if err != nil {
				return FormError{
					Err:  err,
					Kind: ParseError,
				}
			}
			form = &multipart.Form{
				Value: r.Form,
			}
		}

		data := make(map[string]interface{})

		for k, v := range form.Value {
			vLen := len(v)
			if vLen == 0 {
				continue // The struct will have the default value
			}

			if vLen == 1 {
				// skip empty values. Parsing "" to int, float, bool, etc causes errors.
				// Ignore to keep the default value of the struct field.
				// The user can check if the field is empty using the required tag.
				if v[0] == "" {
					continue
				}
				data[k] = v[0] // if there's only one value.
			} else {
				data[k] = v // array of values
			}
		}

		err = parseFormData(data, v, timezone)
		if err != nil {
			// propagate the error
			return err
		}
		return nil
	} else if contentType == ContentTypeXML {
		xmlDecoder := xml.NewDecoder(r.Body)
		err := xmlDecoder.Decode(v)
		if err != nil {
			return FormError{
				Err:  err,
				Kind: ParseError,
			}
		}
		return nil
	} else {
		return FormError{
			Err:  fmt.Errorf("unsupported content type: %s", contentType),
			Kind: InvalidContentType,
		}
	}
}

func SnakeCase(s string) string {
	var res strings.Builder
	for i, r := range s {
		if i > 0 && 'A' <= r && r <= 'Z' {
			res.WriteRune('_')
		}
		res.WriteRune(r)
	}
	return strings.ToLower(res.String())
}

// Parses the form data and stores the result in v.
// Default tag name is "form". You can specify a different tag name using the tag argument.
// Forexample "query" tag name will parse the form data using the "query" tag.
func parseFormData(data map[string]interface{}, v interface{}, timezone *time.Location, tag ...string) error {
	var tagName string = "form"
	if len(tag) > 0 {
		tagName = tag[0]
	}

	rv := reflect.ValueOf(v).Elem()
	rt := rv.Type()

	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		tag := field.Tag.Get(tagName)
		if tag == "" {
			// try json tag name and fallback to snake case
			tag = field.Tag.Get("json")
			if tag == "" {
				tag = SnakeCase(field.Name)
			}
		}

		tagList := strings.Split(tag, ",")
		for i := range tagList {
			tagList[i] = strings.TrimSpace(tagList[i])
		}

		// Take tag name to be the first in the tagList
		tag = tagList[0]

		required := slices.Contains(tagList, "required") || field.Tag.Get("required") == "true"
		value, ok := data[tag]
		if !ok {
			if required {
				return FormError{
					Err:   fmt.Errorf("field '%s' is required", tag),
					Kind:  RequiredFieldMissing,
					Field: field.Name,
				}
			}
			continue
		}

		// set the value
		fieldVal := rv.Field(i)
		if err := setField(field.Name, fieldVal, value, timezone); err != nil {
			return FormError{
				Err:   err,
				Kind:  ParseError,
				Field: field.Name,
			}
		}
	}
	return nil
}

func setField(name string, fieldVal reflect.Value, value interface{}, timezone ...*time.Location) error {
	tz := DefaultTimezone
	if len(timezone) > 0 {
		tz = timezone[0]
	}

	// Dereference pointer if the field is a pointer
	if fieldVal.Kind() == reflect.Ptr {
		// Create a new value of the underlying type
		if fieldVal.IsNil() {
			fieldVal.Set(reflect.New(fieldVal.Type().Elem()))
		}
		fieldVal = fieldVal.Elem()
	}

	switch fieldVal.Kind() {
	case reflect.String:
		fieldVal.SetString(value.(string))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v, err := strconv.ParseInt(value.(string), 10, 64)
		if err != nil {
			return err
		}
		fieldVal.SetInt(v)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v, err := strconv.ParseUint(value.(string), 10, 64)
		if err != nil {
			return err
		}
		fieldVal.SetUint(v)
	case reflect.Float32, reflect.Float64:
		v, err := strconv.ParseFloat(value.(string), 64)
		if err != nil {
			return err
		}
		fieldVal.SetFloat(v)
	case reflect.Bool:
		v, err := strconv.ParseBool(value.(string))
		if err != nil {
			// try parsing on/off since html forms use on/off for checkboxes
			if value.(string) == "on" {
				v = true
			} else if value.(string) == "off" {
				v = false
			} else {
				return err
			}
		}
		fieldVal.SetBool(v)
	case reflect.Slice:
		// Handle slice types
		return handleSlice(name, fieldVal, value, tz)
	case reflect.Struct:
		if fieldVal.Type() == reflect.TypeOf(time.Time{}) {
			t, err := ParseTime(value.(string), tz)
			if err != nil {
				return err
			}
			fieldVal.Set(reflect.ValueOf(t))
		} else {
			// Check if the field implements the FormScanner interface
			if scanner, ok := fieldVal.Addr().Interface().(FormScanner); ok {
				return scanner.FormScan(value)
			}
			return FormError{
				Err:   fmt.Errorf("unsupported type: %v: %v, a custom struct must implement rex.FormScanner interface", fieldVal.Kind(), value),
				Kind:  UnsupportedType,
				Field: name,
			}
		}
	default:
		// check if the field implements the FormScanner interface (even if it's a pointer)
		if fieldVal.Kind() == reflect.Ptr {
			if fieldVal.Elem().Kind() == reflect.Struct {
				if scanner, ok := fieldVal.Interface().(FormScanner); ok {
					return scanner.FormScan(value)
				}
			}
		} else if fieldVal.Kind() == reflect.Struct {
			// Check if the field implements the FormScanner interface
			if scanner, ok := fieldVal.Addr().Interface().(FormScanner); ok {
				return scanner.FormScan(value)
			}
		} else {
			return FormError{
				Err:   fmt.Errorf("unsupported type: %s, a custom struct must implement rex.FormScanner interface", fieldVal.Kind()),
				Kind:  UnsupportedType,
				Field: name,
			}
		}
	}

	return nil
}

// Parses the form value and stores the result fieldVal.
// value should be a slice of strings.
func handleSlice(name string, fieldVal reflect.Value, value any, timezone *time.Location) error {
	var valueSlice []string
	var ok bool
	valueSlice, ok = value.([]string)
	if !ok {
		// Check if its a string and split it and clean it
		if v, ok := value.(string); ok {
			valueSlice = strings.Split(v, ",")
			for i := range valueSlice {
				valueSlice[i] = strings.TrimSpace(valueSlice[i])
			}
		} else {
			return FormError{
				Err:   fmt.Errorf("unsupported slice type: %T with value: %v", value, value),
				Kind:  UnsupportedType,
				Field: name,
			}
		}
	}

	sliceLen := len(valueSlice)
	if sliceLen == 0 {
		return nil // Use a zero value slice
	}

	// If we have a pointer to a slice, call handleSlice recursively
	if fieldVal.Kind() == reflect.Ptr {
		// We can't call of reflect.Value.Type on zero Value
		if fieldVal.IsNil() {
			fieldVal.Set(reflect.New(fieldVal.Type().Elem()))
		}
		fieldVal = fieldVal.Elem()
		if fieldVal.Kind() == reflect.Slice {
			return handleSlice(name, fieldVal, valueSlice, timezone)
		}
	}

	slice := reflect.MakeSlice(fieldVal.Type(), sliceLen, sliceLen)

	// get the kind of the slice element
	elemKind := fieldVal.Type().Elem().Kind()
	switch elemKind {
	case reflect.String:
		for i, v := range valueSlice {
			slice.Index(i).SetString(v)
		}
		fieldVal.Set(slice)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		for i, v := range valueSlice {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return err
			}
			slice.Index(i).SetInt(n)
		}
		fieldVal.Set(slice)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		for i, v := range valueSlice {
			n, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return err
			}
			slice.Index(i).SetUint(n)
		}
		fieldVal.Set(slice)
	case reflect.Float32, reflect.Float64:
		for i, v := range valueSlice {
			n, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return err
			}
			slice.Index(i).SetFloat(n)
		}
		fieldVal.Set(slice)
	case reflect.Bool:
		for i, v := range valueSlice {
			n, err := strconv.ParseBool(v)
			if err != nil {
				// try parsing on/off since html forms use on/off for checkboxes
				if v == "on" {
					n = true
				} else if v == "off" {
					n = false
				} else {
					return err
				}
			}
			slice.Index(i).SetBool(n)
		}
		fieldVal.Set(slice)
	case reflect.Struct:
		// could be time.Time
		if fieldVal.Type().Elem() == reflect.TypeOf(time.Time{}) {
			for i, v := range valueSlice {
				t, err := ParseTime(v, timezone)
				if err != nil {
					return err
				}
				slice.Index(i).Set(reflect.ValueOf(t))
			}
			fieldVal.Set(slice)
		} else {
			// Check if the slice element implements the FormScanner interface
			_, ok := reflect.New(fieldVal.Type().Elem()).Interface().(FormScanner)
			if !ok {
				return FormError{
					Err:   fmt.Errorf("unsupported slice element type: %s", fieldVal.Type().Elem().Kind()),
					Kind:  UnsupportedType,
					Field: name,
				}
			}

			for i, v := range valueSlice {
				// Create a new instance of the slice element
				elem := reflect.New(fieldVal.Type().Elem()).Elem()

				// Scan the form value into the slice element
				if err := setField(name, elem, v, timezone); err != nil {
					return err
				}

				// Set the element in the slice
				slice.Index(i).Set(elem)
			}

			fieldVal.Set(slice)
		}
	default:
		elemType := fieldVal.Type().Elem()
		if elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
		}

		// Check if the slice element implements the FormScanner interface
		_, ok := reflect.New(elemType).Interface().(FormScanner)
		if !ok {
			return FormError{
				Err:   fmt.Errorf("unsupported slice element type: %s", elemType.Kind()),
				Kind:  UnsupportedType,
				Field: name,
			}
		}

		for i, v := range valueSlice {
			// Create a new instance of the slice element
			elem := reflect.New(elemType).Elem()

			// Scan the form value into the slice element
			if err := setField(name, elem, v, timezone); err != nil {
				return err
			}

			// Set the element in the slice
			slice.Index(i).Set(elem)
		}

		fieldVal.Set(slice)

	}
	return nil
}

// FormScanner is an interface for types that can scan form values.
// It is used to implement custom form scanning for types that are not supported by default.
type FormScanner interface {
	// FormScan scans the form value and stores the result in the receiver.
	FormScan(value interface{}) error
}

// QueryParser parses the query string and stores the result in v.
func (c *Context) QueryParser(v interface{}, tag ...string) error {
	var tagName string = "query"
	if len(tag) > 0 {
		tagName = tag[0]
	}

	// Make sure v is a pointer to a struct
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return FormError{
			Err:  fmt.Errorf("v must be a pointer to a struct"),
			Kind: InvalidStructPointer,
		}
	}

	data := c.Request.URL.Query()
	dataMap := make(map[string]interface{}, len(data))
	for k, v := range data {
		if len(v) == 1 {
			dataMap[k] = v[0] // if there's only one value.
		} else {
			dataMap[k] = v // array of values or empty array
		}
	}
	return parseFormData(dataMap, v, time.UTC, tagName)
}

// Parse time from string using specified timezone. If timezone is nil,
// UTC is used. Supported time formats are tried in order.
/*
	timeFormats := []string{
		time.RFC3339,                    // "2006-01-02T15:04:05Z07:00" (default go time)
		"2006-01-02T15:04:05.000Z07:00", // RFC 3339 format with milliseconds and a UTC timezone offset(JSON)
		"2006-01-02T15:04",              // HTML datetime-local format without seconds
		"2006-01-02T15:04:05",           // "2006-01-02T15:04:05"
		time.DateTime,                   // Custom format for "YYYY-MM-DD HH:MM:SS"
		time.DateOnly,                   // "2006-01-02" (html date format)
		time.TimeOnly,                   // "HH:MM:SS" (html time format)
	}
*/
func ParseTime(v string, timezone *time.Location) (time.Time, error) {
	// Define a list of time formats to try
	timeFormats := []string{
		time.RFC3339,                    // "2006-01-02T15:04:05Z07:00" (default go time)
		"2006-01-02T15:04:05.000Z07:00", // RFC 3339 format with milliseconds and a UTC timezone offset(JSON)
		"2006-01-02T15:04",              // HTML datetime-local format without seconds
		"2006-01-02T15:04:05",           // "2006-01-02T15:04:05"
		time.DateTime,                   // Custom format for "YYYY-MM-DD HH:MM:SS"
		time.DateOnly,                   // "2006-01-02" (html date format)
		time.TimeOnly,                   // "HH:MM:SS" (html time format)
	}

	loc := DefaultTimezone
	if timezone != nil {
		loc = timezone
	}

	var parsedTime time.Time
	var err error

	// Try parsing the time with each format
	for _, format := range timeFormats {
		parsedTime, err = time.ParseInLocation(format, v, loc)
		if err == nil {
			break // Stop if we successfully parsed the time
		}
	}

	if err != nil { // If no format matched
		return time.Time{}, err
	}
	return parsedTime, nil
}

func ParseTimeFormat(value string, format string, timezone ...string) (time.Time, error) {
	tz := "UTC"
	if len(timezone) > 0 {
		tz = timezone[0]
	}

	loc := time.UTC
	var parsedTime time.Time
	var err error

	if tz != "" {
		loc, err = time.LoadLocation(tz)
		if err != nil {
			return time.Time{}, err
		}
	}

	parsedTime, err = time.ParseInLocation(format, value, loc)
	if err != nil {
		return time.Time{}, err
	}
	return parsedTime, nil
}
