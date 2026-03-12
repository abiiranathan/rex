package rex

import (
	"net/http"
	"strings"
)

// IsSafeMethod reports whether method is an HTTP safe method.
func IsSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead ||
		method == http.MethodOptions || method == http.MethodTrace
}

// ParseBool reports whether v is one of the accepted true values.
// It treats "true", "1", and "on" as true, case-insensitively.
func ParseBool(v string) bool {
	v = strings.ToLower(v)
	return v == "true" || v == "1" || v == "on"
}

// First returns the first element in elems or def when elems is empty.
func First[T any](elems []T, def T) T {
	if len(elems) > 0 {
		return elems[0]
	}
	return def
}
