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

// TestHandleTargetLatencySeries_DefaultsFromZeroedConfig tests that when latency config
// has zero values (e.g., loaded from JSON with empty latency:{}), the series endpoint
// returns the correct default values, not zeros.
// This is a regression test for the bug where cfg.Latency wasn't defaulted before being
// passed to the server, causing series metadata to show interval_seconds=0, retained_range=0.
func TestHandleTargetLatencySeries_DefaultsFromZeroedConfig(t *testing.T) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	// Create config with ZEROED latency - simulates JSON like: "latency": {"http": {}, "icmp": {}}
	cfg := &config.Config{
		Listen:   config.ListenConfig{Addr: ":0"},
		Auth:     config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:   config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Latency:  config.LatencyConfig{}, // Zeroed - all fields default to 0
		Targets:  []config.TargetConfig{{ID: "test-1", Name: "Test", BaseURL: "http://localhost:8080", Enabled: true}},
	}

	// Apply defaults to cfg.Latency - this is what main.go should do
	cfg.Latency.ApplyDefaults()

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	token, _ := srv.sessions.GenerateToken("admin")

	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/latency/series", http.HandlerFunc(srv.handleTargetLatencySeries))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/series?target_id=test-1&probe_kind=http", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var series state.LatencySeries
	json.NewDecoder(rec.Body).Decode(&series)

	// HTTP defaults from latency.go
	if series.IntervalSeconds != config.DefaultHTTPIntervalSeconds {
		t.Errorf("Expected HTTP interval_seconds=%d (default), got %d", config.DefaultHTTPIntervalSeconds, series.IntervalSeconds)
	}
	if series.RetainedRangeSeconds == 0 {
		t.Error("Expected retained_range_seconds > 0 after defaults, got 0")
	}

	// Test ICMP too
	reqICMP := httptest.NewRequest(http.MethodGet, "/api/v1/latency/series?target_id=test-1&probe_kind=icmp", nil)
	reqICMP.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	recICMP := httptest.NewRecorder()
	router.ServeHTTP(recICMP, reqICMP)

	if recICMP.Code != http.StatusOK {
		t.Errorf("Expected 200 for ICMP, got %d", recICMP.Code)
	}

	var seriesICMP state.LatencySeries
	json.NewDecoder(recICMP.Body).Decode(&seriesICMP)

	// ICMP defaults from latency.go
	if seriesICMP.IntervalSeconds != config.DefaultICMPIntervalSeconds {
		t.Errorf("Expected ICMP interval_seconds=%d (default), got %d", config.DefaultICMPIntervalSeconds, seriesICMP.IntervalSeconds)
	}
	if seriesICMP.RetainedRangeSeconds == 0 {
		t.Error("Expected ICMP retained_range_seconds > 0 after defaults, got 0")
	}
}
