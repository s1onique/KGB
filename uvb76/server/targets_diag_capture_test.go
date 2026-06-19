package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/s1onique/KGB/uvb76/auth"
	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// TestTargetsEndpoint_ExposesEffectiveCaptureURL verifies that GET /api/v1/targets
// includes diagnostic capture info (diagnostic_peer_name, diagnostic_base_url,
// effective_capture_url) when a diagnostics peer targets that target.
func TestTargetsEndpoint_ExposesEffectiveCaptureURL(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Diagnostics: config.DiagnosticsConfig{
			Enabled:        true,
			CaptureOnSpike: true,
			Peers: []config.DiagPeerConfig{
				{
					Name:    "tovarisch-lab",
					BaseURL: "http://10.88.76.2:8317",
					Targets: []string{"lab-tovarisch"},
				},
			},
		},
		Targets: []config.TargetConfig{
			{ID: "lab-tovarisch", Name: "Lab Tovarisch", BaseURL: "http://10.88.76.2:8317", Enabled: true},
		},
	}

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/targets", http.HandlerFunc(srv.handleTargets))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var targets []TargetInfo
	if err := json.NewDecoder(rec.Body).Decode(&targets); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(targets) != 1 {
		t.Fatalf("Expected 1 target, got %d", len(targets))
	}

	tgt := targets[0]
	if tgt.ID != "lab-tovarisch" {
		t.Errorf("Expected ID 'lab-tovarisch', got %q", tgt.ID)
	}

	// Verify diagnostic capture info is exposed
	if tgt.DiagnosticPeerName != "tovarisch-lab" {
		t.Errorf("Expected DiagnosticPeerName 'tovarisch-lab', got %q", tgt.DiagnosticPeerName)
	}

	if tgt.DiagnosticBaseURL != "http://10.88.76.2:8317" {
		t.Errorf("Expected DiagnosticBaseURL 'http://10.88.76.2:8317', got %q", tgt.DiagnosticBaseURL)
	}

	// Verify effective_capture_url equals DiagPeerStatusURL(peer.BaseURL)
	expectedCaptureURL := config.DiagPeerStatusURL("http://10.88.76.2:8317")
	if tgt.EffectiveCaptureURL != expectedCaptureURL {
		t.Errorf("Expected EffectiveCaptureURL %q, got %q", expectedCaptureURL, tgt.EffectiveCaptureURL)
	}

	// Verify the URL has the correct path
	if tgt.EffectiveCaptureURL != "http://10.88.76.2:8317/status.json?include=network_diag" {
		t.Errorf("Expected effective_capture_url with /status.json?include=network_diag, got %q", tgt.EffectiveCaptureURL)
	}
}

// TestTargetsEndpoint_OriginOnlyBaseURLProducesCanonicalStatusPath verifies that
// an origin-only base_url (without trailing path) produces /status.json?include=network_diag.
func TestTargetsEndpoint_OriginOnlyBaseURLProducesCanonicalStatusPath(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Diagnostics: config.DiagnosticsConfig{
			Enabled:        true,
			CaptureOnSpike: true,
			Peers: []config.DiagPeerConfig{
				{
					Name:    "peer-1",
					BaseURL: "http://localhost:8080",
					Targets: []string{"target-1"},
				},
			},
		},
		Targets: []config.TargetConfig{
			{ID: "target-1", Name: "Target 1", BaseURL: "http://localhost:8080", Enabled: true},
		},
	}

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/targets", http.HandlerFunc(srv.handleTargets))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec.Code)
	}

	var targets []TargetInfo
	if err := json.NewDecoder(rec.Body).Decode(&targets); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	expectedURL := "http://localhost:8080/status.json?include=network_diag"
	if targets[0].EffectiveCaptureURL != expectedURL {
		t.Errorf("Expected %q, got %q", expectedURL, targets[0].EffectiveCaptureURL)
	}
}

// TestTargetsEndpoint_FootgunBaseURLWithStatusPath shows that base_url containing /status
// produces /status/status.json (legacy/footgun behavior that should be documented).
func TestTargetsEndpoint_FootgunBaseURLWithStatusPath(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Diagnostics: config.DiagnosticsConfig{
			Enabled:        true,
			CaptureOnSpike: true,
			Peers: []config.DiagPeerConfig{
				{
					Name:    "peer-1",
					BaseURL: "http://localhost:8080/status", // Footgun: includes /status
					Targets: []string{"target-1"},
				},
			},
		},
		Targets: []config.TargetConfig{
			{ID: "target-1", Name: "Target 1", BaseURL: "http://localhost:8080/status", Enabled: true},
		},
	}

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/targets", http.HandlerFunc(srv.handleTargets))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec.Code)
	}

	var targets []TargetInfo
	if err := json.NewDecoder(rec.Body).Decode(&targets); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// The DiagPeerStatusURL function produces /status/status.json when base_url contains /status
	// This is the documented footgun behavior - users should use origin-only base_url
	expectedURL := "http://localhost:8080/status/status.json?include=network_diag"
	if targets[0].EffectiveCaptureURL != expectedURL {
		t.Errorf("Expected footgun URL %q, got %q", expectedURL, targets[0].EffectiveCaptureURL)
	}
}

// TestTargetsEndpoint_TargetWithoutDiagnosticsPeerHasNoCaptureInfo verifies that
// targets without a diagnostics peer do not include diagnostic capture fields.
func TestTargetsEndpoint_TargetWithoutDiagnosticsPeerHasNoCaptureInfo(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Diagnostics: config.DiagnosticsConfig{
			Enabled:        true,
			CaptureOnSpike: true,
			Peers: []config.DiagPeerConfig{
				{
					Name:    "peer-1",
					BaseURL: "http://host1:8080",
					Targets: []string{"target-1"}, // Only targets target-1
				},
			},
		},
		Targets: []config.TargetConfig{
			{ID: "target-1", Name: "Target 1", BaseURL: "http://host1:8080", Enabled: true},
			{ID: "target-2", Name: "Target 2", BaseURL: "http://host2:8080", Enabled: true}, // No diagnostics peer
		},
	}

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/targets", http.HandlerFunc(srv.handleTargets))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec.Code)
	}

	var targets []TargetInfo
	if err := json.NewDecoder(rec.Body).Decode(&targets); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(targets) != 2 {
		t.Fatalf("Expected 2 targets, got %d", len(targets))
	}

	// target-1 has a diagnostics peer - should have capture info
	if targets[0].DiagnosticPeerName != "peer-1" {
		t.Errorf("Expected target-1 to have DiagnosticPeerName 'peer-1', got %q", targets[0].DiagnosticPeerName)
	}
	if targets[0].EffectiveCaptureURL == "" {
		t.Error("Expected target-1 to have EffectiveCaptureURL")
	}

	// target-2 has no diagnostics peer - should NOT have capture info
	if targets[1].DiagnosticPeerName != "" {
		t.Errorf("Expected target-2 to have empty DiagnosticPeerName, got %q", targets[1].DiagnosticPeerName)
	}
	if targets[1].EffectiveCaptureURL != "" {
		t.Errorf("Expected target-2 to have empty EffectiveCaptureURL, got %q", targets[1].EffectiveCaptureURL)
	}
}
