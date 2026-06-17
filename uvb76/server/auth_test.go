package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/s1onique/KGB/uvb76/auth"
	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// Auth endpoint tests

func TestLoginEndpoint_Success(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Targets:  []config.TargetConfig{},
	}
	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)

	router := mux.NewRouter()
	router.Handle("/api/v1/auth/login", http.HandlerFunc(srv.handleLogin))

	loginReq := auth.LoginRequest{Username: "admin", Password: "correct-password"}
	body, _ := json.Marshal(loginReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200 for successful login, got %d", rec.Code)
	}

	var resp auth.LoginResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if !resp.Success {
		t.Error("Expected success=true")
	}

	// Should set session cookie
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == auth.SessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Error("Expected session cookie to be set")
	}
}

func TestLoginEndpoint_BadCredentials(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Targets:  []config.TargetConfig{},
	}
	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)

	router := mux.NewRouter()
	router.Handle("/api/v1/auth/login", http.HandlerFunc(srv.handleLogin))

	loginReq := auth.LoginRequest{Username: "admin", Password: "wrong-password"}
	body, _ := json.Marshal(loginReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for bad credentials, got %d", rec.Code)
	}

	var resp auth.LoginResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Success {
		t.Error("Expected success=false")
	}
	if resp.Error != "invalid_credentials" {
		t.Errorf("Expected error 'invalid_credentials', got '%s'", resp.Error)
	}

	// Should NOT have WWW-Authenticate header
	if wwwAuth := rec.Header().Get("WWW-Authenticate"); wwwAuth != "" {
		t.Errorf("Should not have WWW-Authenticate header, got '%s'", wwwAuth)
	}
}

func TestLoginEndpoint_WrongUsernameWithCorrectPassword(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Targets:  []config.TargetConfig{},
	}
	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)

	router := mux.NewRouter()
	router.Handle("/api/v1/auth/login", http.HandlerFunc(srv.handleLogin))

	// Use wrong username but correct password
	loginReq := auth.LoginRequest{Username: "wronguser", Password: "correct-password"}
	body, _ := json.Marshal(loginReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for wrong username, got %d", rec.Code)
	}

	var resp auth.LoginResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Success {
		t.Error("Expected success=false for wrong username")
	}
	if resp.Error != "invalid_credentials" {
		t.Errorf("Expected error 'invalid_credentials', got '%s'", resp.Error)
	}
}

func TestLogoutEndpoint(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Targets:  []config.TargetConfig{},
	}
	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)

	router := mux.NewRouter()
	router.Handle("/api/v1/auth/logout", http.HandlerFunc(srv.handleLogout))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200 for logout, got %d", rec.Code)
	}

	// Should clear session cookie
	cookies := rec.Result().Cookies()
	for _, c := range cookies {
		if c.Name == auth.SessionCookieName && c.MaxAge == -1 {
			return // Found the clearing cookie
		}
	}
	t.Error("Expected session cookie to be cleared")
}

func TestAuthCheckEndpoint_Authenticated(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Targets:  []config.TargetConfig{},
	}
	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)

	// Generate a valid token
	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	router.Handle("/api/v1/auth/check", http.HandlerFunc(srv.handleAuthCheck))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/check", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200 for authenticated check, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["authenticated"] != true {
		t.Error("Expected authenticated=true")
	}
}

func TestAuthCheckEndpoint_Unauthenticated(t *testing.T) {
	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: "sha256:aaaaaaaaaaaaaaaa:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Targets:  []config.TargetConfig{},
	}
	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)

	router := mux.NewRouter()
	router.Handle("/api/v1/auth/check", http.HandlerFunc(srv.handleAuthCheck))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/check", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for unauthenticated check, got %d", rec.Code)
	}
}

// Admin UI endpoint tests

func TestAdminEndpoint_UnauthenticatedReturns200(t *testing.T) {
	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: "sha256:aaaaaaaaaaaaaaaa:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Targets:  []config.TargetConfig{},
	}
	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)

	router := mux.NewRouter()
	router.Handle("/", http.HandlerFunc(srv.handleAdmin))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Should return 200 HTML, not 401
	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200 for unauthenticated admin, got %d", rec.Code)
	}

	// Should not have WWW-Authenticate header
	if wwwAuth := rec.Header().Get("WWW-Authenticate"); wwwAuth != "" {
		t.Errorf("Should not have WWW-Authenticate header, got '%s'", wwwAuth)
	}
}
