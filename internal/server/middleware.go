package server

import (
	"net/http"

	"dezuxk/internal/security"
)

// RequireBearer returns a middleware that validates the HTTP Authorization Bearer token
// against the configured server API key using constant-time comparison.
func RequireBearer(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if apiKey == "" {
				next.ServeHTTP(w, r)
				return
			}
			token := security.BearerToken(r)
			if token == "" || !security.TokenEqual(apiKey, token) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized","message":"invalid or missing api key"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
