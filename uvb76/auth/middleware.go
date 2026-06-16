// Package auth provides authentication middleware for UVB-76.
package auth

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/s1onique/KGB/uvb76/config"
)

// BasicAuthMiddleware returns a handler that enforces HTTP Basic Auth.
func BasicAuthMiddleware(username, passwordHash string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Parse Basic auth header
			if !strings.HasPrefix(authHeader, "Basic ") {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			encoded := strings.TrimPrefix(authHeader, "Basic ")
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) != 2 {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			providedUser := parts[0]
			providedPass := parts[1]

			// Validate credentials
			if providedUser != username {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			ok, err := config.VerifyPassword(providedPass, passwordHash)
			if err != nil || !ok {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
