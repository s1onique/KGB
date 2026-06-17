package auth

import (
	"context"
	"net/http"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const usernameContextKey contextKey = "username"

// SessionAuthMiddleware returns a handler that enforces session-based authentication.
// Unlike BasicAuthMiddleware, this does NOT emit WWW-Authenticate headers.
// Instead, unauthenticated requests receive a JSON 401 response.
func SessionAuthMiddleware(sessions *SessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get session token from cookie or header
			// Token is base64-encoded from GenerateToken, so it's the key directly
			token := GetSessionToken(r)
			if token == "" {
				JSONError(w, "authentication_required", http.StatusUnauthorized)
				return
			}

			// Validate session token
			session, ok := sessions.ValidateToken(token)
			if !ok {
				JSONError(w, "authentication_required", http.StatusUnauthorized)
				return
			}

			// Store username in request context for downstream handlers
			ctx := context.WithValue(r.Context(), usernameContextKey, session.Username)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SetUsernameContext stores the username in the request context.
func SetUsernameContext(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, usernameContextKey, username)
}

// GetUsernameFromContext retrieves the username from the request context.
func GetUsernameFromContext(ctx context.Context) string {
	if username, ok := ctx.Value(usernameContextKey).(string); ok {
		return username
	}
	return ""
}
