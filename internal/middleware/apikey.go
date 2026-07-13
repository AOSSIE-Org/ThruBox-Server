package middleware

import (
	"crypto/subtle"
	"net/http"
)

// APIKeyAuth returns an HTTP middleware that enforces API key authentication.
// If the apiKey is empty, the middleware is a no-op passthrough.
// When enabled, all requests must include a valid X-API-Key header.
// The health endpoint (/health) is always excluded from API key checks.
func APIKeyAuth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// No API key configured — pass through
			if apiKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Health endpoint is always accessible
			if r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			// Check the X-API-Key header using constant-time comparison
			provided := r.Header.Get("X-API-Key")
			if subtle.ConstantTimeCompare([]byte(provided), []byte(apiKey)) != 1 {
				http.Error(w, "unauthorized: invalid or missing API key", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
