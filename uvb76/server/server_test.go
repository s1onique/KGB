package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

func TestHealthzEndpoint(t *testing.T) {
	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: "sha256:aaaaaaaaaaaaaaaa:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Targets:  []config.TargetConfig{},
	}
	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true) // devMode = true

	router := mux.NewRouter()
	router.Handle("/api/v1/healthz", http.HandlerFunc(srv.handleHealthz))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", resp["status"])
	}
}

func TestTargetsEndpoint_RequiresAuth(t *testing.T) {
	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: "sha256:aaaaaaaaaaaaaaaa:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Targets:  []config.TargetConfig{},
	}
	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.authMw)
	protected.Handle("/targets", http.HandlerFunc(srv.handleTargets))

	// Request without auth
	req := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 without auth, got %d", rec.Code)
	}
}

func TestTargetsEndpoint_AcceptsValidAuth(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Targets:  []config.TargetConfig{{ID: "test-1", Name: "Test", BaseURL: "http://localhost", Enabled: true}},
	}
	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.authMw)
	protected.Handle("/targets", http.HandlerFunc(srv.handleTargets))

	// Request with valid auth
	creds := base64.StdEncoding.EncodeToString([]byte("admin:correct-password"))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	req.Header.Set("Authorization", "Basic "+creds)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200 with valid auth, got %d", rec.Code)
	}

	var targets []config.TargetConfig
	json.NewDecoder(rec.Body).Decode(&targets)
	if len(targets) != 1 {
		t.Errorf("Expected 1 target, got %d", len(targets))
	}
}

func TestTargetsEndpoint_RejectsBadCredentials(t *testing.T) {
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
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.authMw)
	protected.Handle("/targets", http.HandlerFunc(srv.handleTargets))

	// Request with wrong password
	creds := base64.StdEncoding.EncodeToString([]byte("admin:wrong-password"))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	req.Header.Set("Authorization", "Basic "+creds)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 with bad credentials, got %d", rec.Code)
	}
}
