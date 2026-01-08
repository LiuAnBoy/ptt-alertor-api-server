package auth

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/julienschmidt/httprouter"
)

type contextKey string

const (
	UserContextKey contextKey = "user"
)

// ErrorResponse represents an API error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// JWTAuth middleware validates JWT token
func JWTAuth(next httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		tokenString, err := ExtractTokenFromHeader(r)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: err.Error()})
			return
		}

		claims, err := ValidateToken(tokenString)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: err.Error()})
			return
		}

		// Add claims to context
		ctx := context.WithValue(r.Context(), UserContextKey, claims)
		next(w, r.WithContext(ctx), ps)
	}
}

// RequireRole middleware checks if user has required role
func RequireRole(role string, next httprouter.Handle) httprouter.Handle {
	return JWTAuth(func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		claims := GetUserFromContext(r.Context())
		if claims == nil {
			writeJSON(w, http.StatusUnauthorized, ErrorResponse{Error: "未授權"})
			return
		}

		if claims.Role != role && claims.Role != "admin" {
			writeJSON(w, http.StatusForbidden, ErrorResponse{Error: "禁止存取"})
			return
		}

		next(w, r, ps)
	})
}

// RequireAdmin middleware checks if user is admin
func RequireAdmin(next httprouter.Handle) httprouter.Handle {
	return RequireRole("admin", next)
}

// GetUserFromContext gets user claims from context
func GetUserFromContext(ctx context.Context) *Claims {
	claims, ok := ctx.Value(UserContextKey).(*Claims)
	if !ok {
		return nil
	}
	return claims
}
