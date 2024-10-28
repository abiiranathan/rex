package rex

import "net/http"

// Helper to check if the HTTP method is "safe" (no side effects).
func IsSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead ||
		method == http.MethodOptions || method == http.MethodTrace
}
