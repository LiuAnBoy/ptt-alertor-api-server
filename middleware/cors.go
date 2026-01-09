package middleware

import (
	"net/http"
	"net/url"
	"os"
	"strings"
)

// CORS handles Cross-Origin Resource Sharing
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Check if origin is allowed
		if isOriginAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Requested-With")
		w.Header().Set("Access-Control-Max-Age", "86400")

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// isOriginAllowed checks if the origin is allowed
func isOriginAllowed(origin string) bool {
	if origin == "" {
		return false
	}

	// Always allow localhost for development
	if origin == "http://localhost:3000" {
		return true
	}

	// Allow origins matching ALLOWED_DOMAIN suffix (e.g., "luan.com.tw")
	allowedDomain := os.Getenv("ALLOWED_DOMAIN")
	if allowedDomain != "" {
		parsed, err := url.Parse(origin)
		if err == nil {
			host := parsed.Hostname()
			// Match exact domain or subdomain
			if host == allowedDomain || strings.HasSuffix(host, "."+allowedDomain) {
				return true
			}
		}
	}

	return false
}
