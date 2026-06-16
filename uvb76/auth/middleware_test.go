package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/s1onique/KGB/uvb76/config"
)

func TestBasicAuthMiddleware_RejectsMissingCredentials(t *testing.T) {
	mw := BasicAuthMiddleware("admin", "sha256:abc:def")

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", rec.Code)
	}

	// Verify WWW-Authenticate header is present (per RFC 9110)
	if wwwAuth := rec.Header().Get("WWW-Authenticate"); wwwAuth == "" {
		t.Error("Expected WWW-Authenticate header to be set on 401")
	}
}

func TestBasicAuthMiddleware_RejectsBadCredentials(t *testing.T) {
	// Create a valid hash for "correct-password"
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	mw := BasicAuthMiddleware("admin", hash)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Next handler should not be called")
	}))

	// Use wrong password (BasicAuth uses base64 encoding internally)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.SetBasicAuth("admin", "wrong-password")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", rec.Code)
	}

	// Verify WWW-Authenticate header is present
	if wwwAuth := rec.Header().Get("WWW-Authenticate"); wwwAuth != WWWAuthenticateHeader {
		t.Errorf("Expected WWW-Authenticate %q, got %q", WWWAuthenticateHeader, wwwAuth)
	}
}

func TestBasicAuthMiddleware_AcceptsValidCredentials(t *testing.T) {
	// Create a valid hash for "correct-password"
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	mw := BasicAuthMiddleware("admin", hash)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Use correct password
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.SetBasicAuth("admin", "correct-password")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	// Successful auth should NOT have WWW-Authenticate header
	if wwwAuth := rec.Header().Get("WWW-Authenticate"); wwwAuth != "" {
		t.Errorf("Expected no WWW-Authenticate on 200, got %q", wwwAuth)
	}
}

func TestBasicAuthMiddleware_RejectsInvalidAuthScheme(t *testing.T) {
	mw := BasicAuthMiddleware("admin", "sha256:abc:def")

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", rec.Code)
	}

	// Verify WWW-Authenticate header is present
	if wwwAuth := rec.Header().Get("WWW-Authenticate"); wwwAuth != WWWAuthenticateHeader {
		t.Errorf("Expected WWW-Authenticate %q, got %q", WWWAuthenticateHeader, wwwAuth)
	}
}

func TestBasicAuthMiddleware_RejectsWrongUsername(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	mw := BasicAuthMiddleware("admin", hash)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Next handler should not be called")
	}))

	// Use wrong username
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.SetBasicAuth("wronguser", "correct-password")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", rec.Code)
	}

	// Verify WWW-Authenticate header is present
	if wwwAuth := rec.Header().Get("WWW-Authenticate"); wwwAuth != WWWAuthenticateHeader {
		t.Errorf("Expected WWW-Authenticate %q, got %q", WWWAuthenticateHeader, wwwAuth)
	}
}

func TestBasicAuthMiddleware_WWWAuthenticateHeaderValue(t *testing.T) {
	mw := BasicAuthMiddleware("admin", "sha256:abc:def")

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	wwwAuth := rec.Header().Get("WWW-Authenticate")
	if wwwAuth != `Basic realm="uvb76", charset="UTF-8"` {
		t.Errorf("Expected WWW-Authenticate 'Basic realm=\"uvb76\", charset=\"UTF-8\"', got %q", wwwAuth)
	}
}
