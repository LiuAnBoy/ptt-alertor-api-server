package middleware

import (
	"net/http"
	"os"
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

	// Allow DASHBOARD_URL
	dashboardURL := os.Getenv("DASHBOARD_URL")
	if dashboardURL != "" && origin == dashboardURL {
		return true
	}

	return false
}
