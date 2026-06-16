// Package auth provides authentication middleware for UVB-76.
package auth

import (
	"net/http"

	"github.com/s1onique/KGB/uvb76/config"
)

const (
	// WWWAuthenticateHeader is the WWW-Authenticate header value for Basic Auth.
	// Per RFC 9110, a server returning 401 must include this header.
	WWWAuthenticateHeader = `Basic realm="uvb76", charset="UTF-8"`
)

// BasicAuthMiddleware returns a handler that enforces HTTP Basic Auth.
func BasicAuthMiddleware(username, passwordHash string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Use r.BasicAuth() to extract credentials per Go stdlib patterns.
			// Returns "" if no Authorization header is present.
			providedUser, providedPass, ok := r.BasicAuth()
			if !ok {
				w.Header().Set("WWW-Authenticate", WWWAuthenticateHeader)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Validate username
			if providedUser != username {
				w.Header().Set("WWW-Authenticate", WWWAuthenticateHeader)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Validate password using constant-time comparison
			valid, err := config.VerifyPassword(providedPass, passwordHash)
			if err != nil || !valid {
				w.Header().Set("WWW-Authenticate", WWWAuthenticateHeader)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
