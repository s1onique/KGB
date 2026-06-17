// Package auth provides authentication middleware and session management for UVB-76.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
)

const (
	// SessionCookieName is the name of the session cookie.
	SessionCookieName = "uvb76_session"

	// SessionMaxAge is the session lifetime in seconds (24 hours).
	SessionMaxAge = 86400

	// SessionHeaderName is the alternative header-based session token (for API clients).
	SessionHeaderName = "X-Session-Token"
)

// Session represents an authenticated session.
type Session struct {
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SessionStore manages user sessions in memory.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	secret   []byte
}

// NewSessionStore creates a new session store with the given secret key.
func NewSessionStore(secret string) *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*Session),
		secret:   []byte(secret),
	}
}

// GenerateToken creates a new session token for the given username.
// Returns base64-encoded token suitable for transport/storage.
func (ss *SessionStore) GenerateToken(username string) (string, error) {
	// Generate random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	// Base64-encode for transport (cookie-safe)
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	now := time.Now()
	session := &Session{
		Username:  username,
		CreatedAt: now,
		ExpiresAt: now.Add(SessionMaxAge * time.Second),
	}

	ss.mu.Lock()
	ss.sessions[token] = session
	ss.mu.Unlock()

	return token, nil
}

// ValidateToken checks if a token is valid and returns the session if so.
func (ss *SessionStore) ValidateToken(token string) (*Session, bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	session, ok := ss.sessions[token]
	if !ok {
		return nil, false
	}

	// Check expiration
	if time.Now().After(session.ExpiresAt) {
		return nil, false
	}

	return session, true
}

// InvalidateToken removes a session token.
func (ss *SessionStore) InvalidateToken(token string) {
	ss.mu.Lock()
	delete(ss.sessions, token)
	ss.mu.Unlock()
}

// InvalidateAllForUser removes all sessions for a specific user.
func (ss *SessionStore) InvalidateAllForUser(username string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	for token, session := range ss.sessions {
		if session.Username == username {
			delete(ss.sessions, token)
		}
	}
}

// CleanupExpired removes expired sessions.
func (ss *SessionStore) CleanupExpired() {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	now := time.Now()
	for token, session := range ss.sessions {
		if now.After(session.ExpiresAt) {
			delete(ss.sessions, token)
		}
	}
}

// SignToken creates a signature for a token (HMAC-SHA256).
func (ss *SessionStore) SignToken(token string) string {
	mac := hmac.New(sha256.New, ss.secret)
	mac.Write([]byte(token))
	sig := mac.Sum(nil)
	return base64.URLEncoding.EncodeToString(sig)
}

// VerifyTokenSignature verifies the signature of a token.
func (ss *SessionStore) VerifyTokenSignature(token, signature string) bool {
	expectedSig := ss.SignToken(token)
	return subtle.ConstantTimeCompare([]byte(expectedSig), []byte(signature)) == 1
}

// GetSessionToken extracts session token from request (cookie or header).
func GetSessionToken(r *http.Request) string {
	// Check cookie first
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		return cookie.Value
	}

	// Fall back to header (for API clients)
	return r.Header.Get(SessionHeaderName)
}

// SetSessionCookie sets the session cookie on the response.
// The token is expected to be already base64-encoded from GenerateToken.
func SetSessionCookie(w http.ResponseWriter, token string, secure bool) {
	var cookie http.Cookie
	if secure {
		cookie = http.Cookie{
			Name:     SessionCookieName,
			Value:    token,
			Path:     "/",
			MaxAge:   SessionMaxAge,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		}
	} else {
		cookie = http.Cookie{
			Name:     SessionCookieName,
			Value:    token,
			Path:     "/",
			MaxAge:   SessionMaxAge,
			HttpOnly: true,
			Secure:   false,
			SameSite: http.SameSiteLaxMode,
		}
	}

	http.SetCookie(w, &cookie)
}

// ClearSessionCookie clears the session cookie.
// Pass secure=true in production (HTTPS), false in dev mode (HTTP).
func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	cookie := http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, &cookie)
}

// LoginRequest represents a login API request.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents a login API response.
type LoginResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// ErrorResponse represents an error API response.
type ErrorResponse struct {
	Error string `json:"error"`
}

// JSONError writes a JSON error response without WWW-Authenticate header.
func JSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}

// AuthenticatePasswordOnly validates password against stored hash.
// Note: Username validation must be done separately by the caller.
// Uses constant-time content comparison for equal-length values.
func AuthenticatePasswordOnly(password, storedHash string) bool {
	if password == "" || storedHash == "" {
		return false
	}
	valid, err := config.VerifyPassword(password, storedHash)
	return err == nil && valid
}

// AuthenticateFull validates both username and password against stored credentials.
// Uses constant-time comparison for both username and password to prevent timing attacks.
// Note: Go's ConstantTimeCompare timing depends on slice length, returns immediately on length mismatch.
func AuthenticateFull(username, password, expectedUsername, storedHash string) bool {
	// Validate username using constant-time comparison
	usernameMatch := subtle.ConstantTimeCompare([]byte(username), []byte(expectedUsername)) == 1

	// Validate password using constant-time comparison
	passwordValid, passwordErr := config.VerifyPassword(password, storedHash)

	// Combine results (both must pass)
	return usernameMatch && passwordErr == nil && passwordValid
}

// Base64URLDecode decodes a base64 URL-encoded string.
func Base64URLDecode(encoded string) ([]byte, error) {
	return base64.URLEncoding.DecodeString(encoded)
}
