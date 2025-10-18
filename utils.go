package rex

import (
	"net/http"
	"strings"
)

// Helper to check if the HTTP method is "safe" (no side effects).
func IsSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead ||
		method == http.MethodOptions || method == http.MethodTrace
}

// Returns true if v is true, 1 or on.
// This is case-insensitive.
// Otherwise returns off.
func ParseBool(v string) bool {
	v = strings.ToLower(v)
	return v == "true" || v == "1" || v == "on"
}

// Returns the first element in slice if len(elems) > 0,
// otherwise returns the default value.
func First[T any](elems []T, def T) T {
	if len(elems) > 0 {
		return elems[0]
	}
	return def
}
