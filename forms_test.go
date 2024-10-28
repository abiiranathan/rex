package rex

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
)

type CustomStruct struct {
	Field1 string
	Field2 int
}

func (c *CustomStruct) FormScan(value interface{}) error {
	v, ok := value.(string)
	if !ok {
		return fmt.Errorf("value is not a string")
	}
	c.Field1 = v
	return nil
}

// Date in format YYYY-MM-DD
type Date time.Time

func (d *Date) FormScan(value interface{}) error {
	v, ok := value.(string)
	if !ok {
		return fmt.Errorf("value is not a string")
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return err
	}
	*d = Date(t)
	return nil
}

type customInt int // Kind is int

func TestSetField(t *testing.T) {
	tests := []struct {
		name      string
		fieldType reflect.Kind
		value     interface{}
		expected  interface{}
	}{

		{"String", reflect.String, "test", "test"},
		{"Int", reflect.Int, "123", 123},
		{"Uint", reflect.Uint, "123", uint(123)},
		{"Float32", reflect.Float32, "3.14", float32(3.14)},
		{"Bool True", reflect.Bool, "true", true},
		{"Bool True", reflect.Bool, "on", true},
		{"Bool True", reflect.Bool, "off", false},
		{"Bool False", reflect.Bool, "false", false},
		{"Slice", reflect.Slice, []string{"1", "2", "3"}, []string{"1", "2", "3"}},
		{"SliceWithCommaSeperatedString", reflect.Slice, "1, 2, 3", []string{"1", "2", "3"}},
		{"Time", reflect.Struct, "2022-02-22T12:00:00Z", time.Date(2022, 2, 22, 12, 0, 0, 0, time.UTC)},
		{"CustomStruct", reflect.Struct, "test", CustomStruct{Field1: "test"}},
		{"Date", reflect.Struct, "2022-02-22", Date(time.Date(2022, 2, 22, 0, 0, 0, 0, time.UTC))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fieldValue reflect.Value
			switch tt.fieldType {
			case reflect.String:
				fieldValue = reflect.ValueOf(new(string)).Elem()
			case reflect.Int:
				fieldValue = reflect.ValueOf(new(int)).Elem()
			case reflect.Uint:
				fieldValue = reflect.ValueOf(new(uint)).Elem()
			case reflect.Float32:
				fieldValue = reflect.ValueOf(new(float32)).Elem()
			case reflect.Bool:
				fieldValue = reflect.ValueOf(new(bool)).Elem()
			case reflect.Slice:
				fieldValue = reflect.ValueOf(new([]string)).Elem()
			case reflect.Struct:
				switch tt.name {
				case "Time":
					fieldValue = reflect.ValueOf(new(time.Time)).Elem()
				case "CustomStruct":
					fieldValue = reflect.ValueOf(&CustomStruct{}).Elem()
				case "Date":
					fieldValue = reflect.ValueOf(new(Date)).Elem()
				}
			}

			if err := setField(tt.name, fieldValue, tt.value); err != nil {
				t.Errorf("setField() error = %v", err)
				return
			}

			if !reflect.DeepEqual(fieldValue.Interface(), tt.expected) {
				t.Errorf("setField() = %#v, want %#v", fieldValue.Interface(), tt.expected)
			}
		})
	}
}

func TestHandleSlice(t *testing.T) {
	tests := []struct {
		name       string
		fieldvalue reflect.Value
		value      interface{}
		expected   interface{}
	}{
		{"String", reflect.ValueOf(new([]string)).Elem(), "1, 2, 3", []string{"1", "2", "3"}},
		{"Int", reflect.ValueOf(new([]int)).Elem(), "1, 2, 3", []int{1, 2, 3}},
		{"Uint", reflect.ValueOf(new([]uint)).Elem(), "1, 2, 3", []uint{1, 2, 3}},
		{"Float32", reflect.ValueOf(new([]float32)).Elem(), "1.1, 2.2, 3.3", []float32{1.1, 2.2, 3.3}},
		{"Bool", reflect.ValueOf(new([]bool)).Elem(), "true, false, on", []bool{true, false, true}},
		{"Time", reflect.ValueOf(new([]time.Time)).Elem(), "2022-02-22T12:00:00Z, 2022-02-23T12:00:00Z", []time.Time{time.Date(2022, 2, 22, 12, 0, 0, 0, time.UTC), time.Date(2022, 2, 23, 12, 0, 0, 0, time.UTC)}},
		{"CustomStruct", reflect.ValueOf(new([]CustomStruct)).Elem(), "test1, test2", []CustomStruct{{Field1: "test1"}, {Field1: "test2"}}},
		{"Date", reflect.ValueOf(new([]Date)).Elem(), "2022-02-22, 2022-02-23", []Date{
			Date(time.Date(2022, 2, 22, 0, 0, 0, 0, time.UTC)),
			Date(time.Date(2022, 2, 23, 0, 0, 0, 0, time.UTC)),
		}},
		{"CustomInt", reflect.ValueOf(new([]customInt)).Elem(), "1, 2, 3", []customInt{1, 2, 3}},
		{"PointerToSlice", reflect.ValueOf(new(*[]string)).Elem(), "1, 2, 3", &[]string{"1", "2", "3"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := handleSlice(tt.name, tt.fieldvalue, tt.value, time.UTC); err != nil {
				t.Errorf("handleSlice() error = %v", err)
				return
			}

			if !reflect.DeepEqual(tt.fieldvalue.Interface(), tt.expected) {
				t.Errorf("handleSlice() = %v, want %v", tt.fieldvalue.Interface(), tt.expected)
			}
		})
	}

}

func TestSetFieldCustomInt(t *testing.T) {
	fieldValue := reflect.ValueOf(new(customInt)).Elem()

	if err := setField("int", fieldValue, "123"); err != nil {
		t.Errorf("setField() error = %v", err)
		return
	}

	if !reflect.DeepEqual(fieldValue.Interface(), customInt(123)) {
		t.Errorf("setField() = %v, want %v", fieldValue.Interface(), customInt(123))
	}
}

// test pointers
func TestSetFieldsPointer(t *testing.T) {
	// use pointer to string, int, float, bool, slice, struct, time.Time, and custom type
	var (
		str   *string
		i     *int
		ui    *uint
		f32   *float32
		b     *bool
		slice *[]string
		c     *CustomStruct
		d     *Date
	)

	str = new(string)
	i = new(int)
	ui = new(uint)
	f32 = new(float32)
	b = new(bool)
	slice = new([]string)
	c = &CustomStruct{}
	d = new(Date)

	tests := []struct {
		name     string
		fieldPtr interface{}
		value    interface{}
		expected interface{}
	}{
		{"String", str, "test", "test"},
		{"Int", i, "123", 123},
		{"Uint", ui, "123", uint(123)},
		{"Float32", f32, "3.14", float32(3.14)},
		{"Bool True", b, "true", true},
		{"Bool True", b, "on", true},
		{"Bool True", b, "off", false},
		{"Bool False", b, "false", false},
		{"Slice", slice, []string{"1", "2", "3"}, []string{"1", "2", "3"}},
		{"CustomStruct", c, "test", CustomStruct{Field1: "test"}},
		{"Date", d, "2022-02-22", Date(time.Date(2022, 2, 22, 0, 0, 0, 0, time.UTC))},

		// test nil
		{"Nil", new(string), "test", "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := setField(tt.name, reflect.ValueOf(tt.fieldPtr).Elem(), tt.value); err != nil {
				t.Errorf("setField() error = %v", err)
				return
			}

			if !reflect.DeepEqual(reflect.ValueOf(tt.fieldPtr).Elem().Interface(), tt.expected) {
				t.Errorf("setField() = %v, want %v", reflect.ValueOf(tt.fieldPtr).Elem().Interface(), tt.expected)
			}
		})
	}
}

func TestSnakecase(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Empty", "", ""},
		{"Lowercase", "test", "test"},
		{"Uppercase", "Test", "test"},
		{"CamelCase", "testString", "test_string"},
		{"CamelCase", "TestString", "test_string"},
		{"CamelCase", "testString123", "test_string123"},
		{"CamelCase", "TestString123", "test_string123"},
		{"CamelCase", "testString123Test", "test_string123_test"},
		{"CamelCase", "TestString123Test", "test_string123_test"},
		{"CamelCase", "testString123Test123", "test_string123_test123"},
		{"CamelCase", "TestString123Test123", "test_string123_test123"},
		{"CamelCase", "testString123Test123Test", "test_string123_test123_test"},
		{"CamelCase", "TestString123Test123Test", "test_string123_test123_test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SnakeCase(tt.input); got != tt.expected {
				t.Errorf("snakecase() = %v, want %v", got, tt.expected)
			}
		})
	}

}

func TestKebabCase(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Empty", "", ""},
		{"Lowercase", "example", "example"},
		{"Uppercase", "Example", "example"},
		{"CamelCase", "exampleString", "example-string"},
		{"CamelCase", "ExampleString", "example-string"},
		{"CamelCase", "exampleString123", "example-string123"},
		{"CamelCase", "ExampleString123", "example-string123"},
		{"CamelCase", "exampleStringWithNumbers123", "example-string-with-numbers123"},
		{"CamelCase", "ExampleStringWithNumbers123", "example-string-with-numbers123"},
		{"CamelCase", "exampleStringWithMultipleParts", "example-string-with-multiple-parts"},
		{"CamelCase", "ExampleStringWithMultipleParts", "example-string-with-multiple-parts"},
		{"CamelCase", "exampleStringWithNumbers123AndMore", "example-string-with-numbers123-and-more"},
		{"CamelCase", "ExampleStringWithNumbers123AndMore", "example-string-with-numbers123-and-more"},
		{"CamelCase", "exampleStringWithNumbers123AndEvenMoreParts", "example-string-with-numbers123-and-even-more-parts"},
		{"CamelCase", "ExampleStringWithNumbers123AndEvenMoreParts", "example-string-with-numbers123-and-even-more-parts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := KebabCase(tt.input); got != tt.expected {
				t.Errorf("kebabcase() = %v, want %v", got, tt.expected)
			}
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := KebabCase(tt.input); got != tt.expected {
				t.Errorf("kebabcase() = %v, want %v", got, tt.expected)
			}
		})
	}

}

// test multipart form with []int and []string
func TestSetFieldMultipartForm(t *testing.T) {
	type TestStruct struct {
		Ints    []int     // use default snake case tag(ints)
		Strings []string  `form:"strings,omitempty"` // field name must appear first.
		Floats  []float64 `form:"floats,omitempty"`
	}

	// send an actual form using httptest
	r := NewRouter()
	r.POST("/test", func(c *Context) error {
		var test TestStruct
		if err := c.BodyParser(&test); err != nil {
			t.Errorf("BodyParser() error = %v", err)
			return err
		}

		if !reflect.DeepEqual(test.Ints, []int{1, 2, 3}) {
			t.Errorf("BodyParser() = %v, want %v", test.Ints, []int{1, 2, 3})
		}

		if !reflect.DeepEqual(test.Strings, []string{"a", "b", "c"}) {
			t.Errorf("BodyParser() = %v, want %v", test.Strings, []string{"a", "b", "c"})
		}

		if !reflect.DeepEqual(test.Floats, []float64{1.1, 2.2, 3.3}) {
			t.Errorf("BodyParser() = %v, want %v", test.Floats, []float64{1.1, 2.2, 3.3})
		}

		_, err := c.Write([]byte("OK"))
		return err
	})

	// send a multipart form
	formData := url.Values{
		"ints":    {"1", "2", "3"},
		"strings": {"a", "b", "c"},
		"floats":  {"1.1", "2.2", "3.3"},
	}

	// Encode the form data
	formEncoded := formData.Encode()

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(formEncoded))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("BodyParser() status = %v, want %v", w.Code, http.StatusOK)
	}

}

// Test multipart form with ignores empty values
func TestSetFieldMultipartFormEmpty(t *testing.T) {
	type TestStruct struct {
		Field1 string
		Field2 int
	}

	// send an actual form using httptest
	r := NewRouter()
	r.POST("/test", func(c *Context) error {
		test := TestStruct{}
		if err := c.BodyParser(&test); err != nil {
			t.Errorf("BodyParser() error = %v", err)
			return err
		}

		if test.Field1 != "" {
			t.Errorf("BodyParser() = %q, want %q", test.Field1, "")
		}

		if test.Field2 != 0 {
			t.Errorf("BodyParser() = %v, want %v", test.Field2, 0)
		}
		_, err := c.Write([]byte("OK"))
		return err
	})

	// send a multipart form
	formData := url.Values{
		"field1": {""},
		"field2": {""},
	}

	// Encode the form data
	formEncoded := formData.Encode()

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(formEncoded))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("BodyParser() status = %v, want %v", w.Code, http.StatusOK)
	}
}

// empty form fields raise an error if required
func TestSetFieldMultipartFormRequired(t *testing.T) {
	type TestStruct struct {
		Field1 string  `form:"field1,required"`
		Field2 int     `form:"field2,required"`
		Fields float32 `form:"fields" required:"true"` // required tag is also allowed.
	}

	// send an actual form using httptest
	r := NewRouter()
	r.POST("/test", func(c *Context) error {
		var test TestStruct
		err := c.BodyParser(&test)
		if err == nil {
			t.Errorf("BodyParser() error = %v, want %v", err, "required field field2 is empty")
			return err
		}

		if !errors.As(err, &FormError{}) {
			t.Errorf("expected FormError, got %T", err)
		}

		var f_err FormError = err.(FormError)
		if f_err.Kind != RequiredFieldMissing {
			t.Errorf("expected FormError.Kind of RequiredFieldMissing, got %v", f_err.Kind)
		}

		_, err = c.Write([]byte("OK"))
		return err
	})

	// send a multipart form
	formData := url.Values{
		"field1": {"test"},
	}

	// Encode the form data
	formEncoded := formData.Encode()

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(formEncoded))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("BodyParser() status = %v, want %v", w.Code, http.StatusOK)
	}
}

// test application xml
func TestBodyParserXML(t *testing.T) {
	type TestStruct struct {
		Field1 string `xml:"field1"`
		Field2 int    `xml:"field2"`
	}

	r := NewRouter()
	r.POST("/submit", func(c *Context) error {
		var test TestStruct
		if err := c.BodyParser(&test); err != nil {
			t.Errorf("BodyParser() error = %v", err)
			return err
		}

		if test.Field1 != "test" {
			t.Errorf("BodyParser() = %v, want %v", test.Field1, "test")
		}

		if test.Field2 != 123 {
			t.Errorf("BodyParser() = %v, want %v", test.Field2, 123)
		}

		_, err := c.Write([]byte("OK"))
		return err
	})

	// send an XML
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
	<testStruct>
		<field1>test</field1>
		<field2>123</field2>
	</testStruct>`

	req := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(xmlData))
	req.Header.Set("Content-Type", "application/xml")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("BodyParser() status = %v, want %v", w.Code, http.StatusOK)
	}
}

// test QueryParser
func TestQueryParser(t *testing.T) {
	type TestStruct struct {
		Field1 string `query:"field1"`
		Field2 int    `query:"field2"`
	}

	r := NewRouter()
	r.GET("/submit", func(c *Context) error {
		var test TestStruct
		if err := c.QueryParser(&test); err != nil {
			t.Errorf("QueryParser() error = %v", err)
			return err
		}

		if test.Field1 != "test" {
			t.Errorf("QueryParser() = %v, want %v", test.Field1, "test")
		}

		if test.Field2 != 123 {
			t.Errorf("QueryParser() = %v, want %v", test.Field2, 123)
		}

		_, err := c.Write([]byte("OK"))
		return err
	})

	// send a query
	req := httptest.NewRequest(http.MethodGet, "/submit?field1=test&field2=123", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("QueryParser() status = %v, want %v", w.Code, http.StatusOK)
	}
}

// test query parser with slice
func TestQueryParserSlice(t *testing.T) {
	type TestStruct struct {
		Ints    []int     `query:"ints"`
		Strings []string  `query:"strings"`
		Floats  []float64 `query:"floats"`
	}

	r := NewRouter()
	r.GET("/submitslice", func(c *Context) error {
		var test TestStruct
		if err := c.QueryParser(&test); err != nil {
			t.Errorf("QueryParser() error = %v", err)
			return err
		}

		if !reflect.DeepEqual(test.Ints, []int{1, 2, 3}) {
			t.Errorf("QueryParser() = %v, want %v", test.Ints, []int{1, 2, 3})
		}

		if !reflect.DeepEqual(test.Strings, []string{"a", "b", "c"}) {
			t.Errorf("QueryParser() = %v, want %v", test.Strings, []string{"a", "b", "c"})
		}

		if !reflect.DeepEqual(test.Floats, []float64{1.1, 2.2, 3.3}) {
			t.Errorf("QueryParser() = %v, want %v", test.Floats, []float64{1.1, 2.2, 3.3})
		}

		_, err := c.Write([]byte("OK"))
		return err
	})

	// send a query
	req := httptest.NewRequest(http.MethodGet, "/submitslice?ints=1&ints=2&ints=3&strings=a&strings=b&strings=c&floats=1.1&floats=2.2&floats=3.3", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("QueryParser() status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestParseTime(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		timezone    string
		expected    time.Time
		shouldError bool
	}{
		{
			name:     "RFC3339",
			input:    "2024-08-20T10:11:09Z",
			timezone: "UTC",
			expected: time.Date(2024, 8, 20, 10, 11, 9, 0, time.UTC),
		},
		{
			name:     "RFC3339 with milliseconds",
			input:    "2024-08-20T10:11:09.851Z",
			timezone: "UTC",
			expected: time.Date(2024, 8, 20, 10, 11, 9, 851000000, time.UTC),
		},
		{
			name:     "HTML datetime-local without seconds",
			input:    "2024-08-20T10:11",
			timezone: "America/New_York",
			expected: time.Date(2024, 8, 20, 10, 11, 0, 0, time.FixedZone("EDT", -4*60*60)),
		},
		{
			name:     "Full date and time",
			input:    "2024-08-20T10:11:09",
			timezone: "America/New_York",
			expected: time.Date(2024, 8, 20, 10, 11, 9, 0, time.FixedZone("EDT", -4*60*60)),
		},
		{
			name:     "Custom format with space",
			input:    "2024-08-20 10:11:09",
			timezone: "America/New_York",
			expected: time.Date(2024, 8, 20, 10, 11, 9, 0, time.FixedZone("EDT", -4*60*60)),
		},
		{
			name:     "Date only",
			input:    "2024-08-20",
			timezone: "America/New_York",
			expected: time.Date(2024, 8, 20, 0, 0, 0, 0, time.FixedZone("EDT", -4*60*60)),
		},
		{
			name:     "Time only",
			input:    "10:11:09",
			timezone: "Africa/Kampala",
			expected: time.Date(0, 1, 1, 10, 11, 9, 0, time.FixedZone("LMT", 2*3600+27*60)),
		},
		{
			name:        "Invalid format",
			input:       "Invalid format",
			timezone:    "UTC",
			shouldError: true,
		},
		{
			name:     "With TIMEZONE set",
			input:    "2024-08-20T10:11:09",
			timezone: "America/New_York",
			expected: time.Date(2024, 8, 20, 10, 11, 9, 0, time.FixedZone("EDT", -4*60*60)),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			loc, err := time.LoadLocation(tc.timezone)
			if err != nil {
				t.Errorf("unable to load timezone: %s\n", tc.timezone)
				return
			}

			result, err := ParseTime(tc.input, loc)
			if tc.shouldError {
				if err == nil {
					t.Errorf("expected error, got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				if tc.name == "Time only" {
					// Time only is wierd. Always seems to be represented as LMT.
					// When compared with .Equal it fails!!
					// I don't know if this is not a bug in go
					if result.String() != tc.expected.String() {
						t.Errorf("expected %v, got %v", tc.expected, result)
					}
				} else {
					if !result.Equal(tc.expected) {
						t.Errorf("expected %v, got %v", tc.expected, result)
					}
				}
			}
		})
	}
}

func TestParseTimeFormat(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		format      string
		timezone    string
		expected    time.Time
		shouldError bool
	}{
		{
			name:     "RFC3339",
			input:    "2024-08-20T10:11:09Z",
			format:   time.RFC3339,
			expected: time.Date(2024, 8, 20, 10, 11, 9, 0, time.UTC),
		},
		{
			name:     "Custom format with milliseconds",
			input:    "2024-08-20T10:11:09.851",
			format:   "2006-01-02T15:04:05.000",
			expected: time.Date(2024, 8, 20, 10, 11, 9, 851000000, time.UTC),
		},
		{
			name:        "Invalid format",
			input:       "Invalid format",
			format:      "2006-01-02T15:04:05",
			timezone:    "UTC",
			shouldError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseTimeFormat(tc.input, tc.format, tc.timezone)
			if tc.shouldError {
				if err == nil {
					t.Errorf("expected error, got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if !result.Equal(tc.expected) {
					t.Errorf("expected %v, got %v", tc.expected, result)
				}
			}
		})
	}
}

func TestValidation(t *testing.T) {
	type User struct {
		Email    string `validate:"required,email"`
		Username string `validate:"min=10"`
		Password string `validate:"required,min=10"`
	}

	r := NewRouter()

	r.POST("/test", func(c *Context) error {
		test := User{}
		err := c.BodyParser(&test)
		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			t.Fatalf("expected validation errors")
		}

		if len(ve) != 3 {
			t.Errorf("expected 3 validation errors, got %d", len(ve))
		}

		// The error handler will translate these errors and render them in html
		return err
	})

	// send a multipart form
	formData := url.Values{
		"email":    {"invalid_email"},
		"username": {""},
		"password": {"password"},
	}

	// Encode the form data
	formEncoded := formData.Encode()

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(formEncoded))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("BodyParser() status = %v, want %v", w.Code, http.StatusBadRequest)
	}
}

func TestCorrectValidation(t *testing.T) {
	type User struct {
		Email    string    `validate:"required,email"`
		Username string    `validate:"required,min=6"`
		Password string    `validate:"required,min=6"`
		Hoby     string    `validate:"oneof=Football TV Movies"`
		Scores   []float32 `validate:"len=3"`
	}

	r := NewRouter()

	r.POST("/test", func(c *Context) error {
		test := User{}
		err := c.BodyParser(&test)
		ve, ok := err.(validator.ValidationErrors)
		if ok {
			t.Fatalf("did not expect validation errors: %v", ve)
		}

		// The error handler will translate these errors and render them in html
		return err
	})

	// send a multipart form
	formData := url.Values{
		"email":    {"example@co.ug"},
		"username": {"johndoe"},
		"password": {"password"},
		"hoby":     {"Football"},
		"scores":   {"98.5", "89", "100"},
	}

	// Encode the form data
	formEncoded := formData.Encode()

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(formEncoded))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("BodyParser() status = %v, want %v", w.Code, http.StatusOK)
	}

	fmt.Println(w.Body.String())

}

func TestErrorHandlerFormErrors(t *testing.T) {
	type User struct {
		Email    string `required:"true"`
		Username string `required:"true"`
		Password string `required:"true"`
	}

	r := NewRouter()
	r.POST("/test", func(c *Context) error {
		user := User{}
		err := c.BodyParser(&user)
		return err
	})

	r.GET("/error", func(c *Context) error {
		return fmt.Errorf("random error")
	})

	// send a multipart form
	formData := url.Values{
		"email":    {""},
		"username": {""},
		"password": {""},
	}

	// Encode the form data
	formEncoded := formData.Encode()

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(formEncoded))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("BodyParser() status = %v, want %v", w.Code, http.StatusBadRequest)
	}

	fmt.Println("Body:", w.Body.String())

	// random non-form error
	req = httptest.NewRequest(http.MethodGet, "/error", strings.NewReader(formEncoded))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("BodyParser() status = %v, want %v", w.Code, http.StatusBadRequest)
	}

}
