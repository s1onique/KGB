package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/s1onique/KGB/uvb76/config"
)

func TestSessionStore_GenerateAndValidateToken(t *testing.T) {
	store := NewSessionStore("test-secret")

	token, err := store.GenerateToken("admin")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	if token == "" {
		t.Error("Token should not be empty")
	}

	// Validate the token
	session, ok := store.ValidateToken(token)
	if !ok {
		t.Fatal("Token should be valid")
	}

	if session.Username != "admin" {
		t.Errorf("Expected username 'admin', got '%s'", session.Username)
	}
}

func TestSessionStore_InvalidateToken(t *testing.T) {
	store := NewSessionStore("test-secret")

	token, _ := store.GenerateToken("admin")

	// Validate before invalidation
	if _, ok := store.ValidateToken(token); !ok {
		t.Fatal("Token should be valid before invalidation")
	}

	// Invalidate
	store.InvalidateToken(token)

	// Validate after invalidation
	if _, ok := store.ValidateToken(token); ok {
		t.Fatal("Token should be invalid after invalidation")
	}
}

func TestGetSessionToken_FromCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "test-token"})

	token := GetSessionToken(req)
	if token != "test-token" {
		t.Errorf("Expected 'test-token', got '%s'", token)
	}
}

func TestGetSessionToken_FromHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(SessionHeaderName, "header-token")

	token := GetSessionToken(req)
	if token != "header-token" {
		t.Errorf("Expected 'header-token', got '%s'", token)
	}
}

func TestGetSessionToken_CookieTakesPrecedence(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "cookie-token"})
	req.Header.Set(SessionHeaderName, "header-token")

	token := GetSessionToken(req)
	if token != "cookie-token" {
		t.Errorf("Expected cookie token to take precedence, got '%s'", token)
	}
}

func TestAuthenticatePasswordOnly_ValidPassword(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	if !AuthenticatePasswordOnly("correct-password", hash) {
		t.Error("Should authenticate with correct password")
	}
}

func TestAuthenticatePasswordOnly_InvalidPassword(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	if AuthenticatePasswordOnly("wrong-password", hash) {
		t.Error("Should not authenticate with wrong password")
	}
}

func TestAuthenticatePasswordOnly_EmptyPassword(t *testing.T) {
	if AuthenticatePasswordOnly("", "sha256:aaa:bbb") {
		t.Error("Should not authenticate with empty password")
	}
}

func TestAuthenticateFull_ValidCredentials(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	if !AuthenticateFull("admin", "correct-password", "admin", hash) {
		t.Error("Should authenticate with correct username and password")
	}
}

func TestAuthenticateFull_InvalidUsername(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	if AuthenticateFull("wronguser", "correct-password", "admin", hash) {
		t.Error("Should not authenticate with wrong username")
	}
}

func TestAuthenticateFull_InvalidPassword(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	if AuthenticateFull("admin", "wrong-password", "admin", hash) {
		t.Error("Should not authenticate with wrong password")
	}
}

func TestJSONError(t *testing.T) {
	rec := httptest.NewRecorder()
	JSONError(rec, "test_error", http.StatusUnauthorized)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", rec.Code)
	}

	var resp ErrorResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error != "test_error" {
		t.Errorf("Expected 'test_error', got '%s'", resp.Error)
	}

	// Verify no WWW-Authenticate header
	if wwwAuth := rec.Header().Get("WWW-Authenticate"); wwwAuth != "" {
		t.Errorf("Should not have WWW-Authenticate header, got '%s'", wwwAuth)
	}
}

func TestLoginRequest_Parse(t *testing.T) {
	reqBody := `{"username":"admin","password":"secret"}`
	var req LoginRequest
	if err := json.Unmarshal([]byte(reqBody), &req); err != nil {
		t.Fatalf("Failed to parse login request: %v", err)
	}

	if req.Username != "admin" {
		t.Errorf("Expected username 'admin', got '%s'", req.Username)
	}
	if req.Password != "secret" {
		t.Errorf("Expected password 'secret', got '%s'", req.Password)
	}
}

func TestLoginResponse_Success(t *testing.T) {
	resp := LoginResponse{Success: true}
	data, _ := json.Marshal(resp)

	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)

	if parsed["success"] != true {
		t.Error("Expected success=true")
	}
}

func TestLoginResponse_Error(t *testing.T) {
	resp := LoginResponse{Success: false, Error: "invalid_credentials"}
	data, _ := json.Marshal(resp)

	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)

	if parsed["success"] != false {
		t.Error("Expected success=false")
	}
	if parsed["error"] != "invalid_credentials" {
		t.Errorf("Expected error 'invalid_credentials', got '%v'", parsed["error"])
	}
}
