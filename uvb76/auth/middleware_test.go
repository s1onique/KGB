package auth

import (
	"encoding/base64"
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
}

func TestBasicAuthMiddleware_RejectsBadCredentials(t *testing.T) {
	// Create a valid hash for "correct-password"
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	mw := BasicAuthMiddleware("admin", hash)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Next handler should not be called")
	}))

	// Use wrong password
	creds := base64.StdEncoding.EncodeToString([]byte("admin:wrong-password"))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Basic "+creds)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", rec.Code)
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
	creds := base64.StdEncoding.EncodeToString([]byte("admin:correct-password"))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Basic "+creds)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
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
}

func TestBasicAuthMiddleware_RejectsWrongUsername(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	mw := BasicAuthMiddleware("admin", hash)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Next handler should not be called")
	}))

	// Use wrong username
	creds := base64.StdEncoding.EncodeToString([]byte("wronguser:correct-password"))
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Basic "+creds)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", rec.Code)
	}
}
