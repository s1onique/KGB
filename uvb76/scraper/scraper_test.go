package scraper

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

func TestScraper_StoresSuccessfulSnapshot(t *testing.T) {
	// Create a test server that returns valid tovarisch status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"service": "tovarisch",
			"version": "0.1.1",
			"node_id": "test-node",
			"status":  "ok",
			"checks":  []interface{}{},
			"runtime": map[string]interface{}{
				"pid":     1234,
				"rss_kib": 1024,
			},
		})
	}))
	defer server.Close()

	cfg := &config.ScrapeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "test-1", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(cfg, st, targets)

	// Scrape immediately
	client.scrapeTarget(targets[0])

	snap := st.GetSnapshot("test-1")
	if snap == nil {
		t.Fatal("Expected snapshot to be stored")
	}
	if !snap.Reachable {
		t.Error("Expected reachable to be true")
	}
	if snap.Status != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", snap.Status)
	}
	if snap.NodeID != "test-node" {
		t.Errorf("Expected node_id 'test-node', got '%s'", snap.NodeID)
	}
}

func TestScraper_RecordsUnreachableTarget(t *testing.T) {
	// Create a test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
	}))
	server.Close() // Close immediately to make it unreachable

	cfg := &config.ScrapeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "test-1", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(cfg, st, targets)

	// Create a new URL for the closed server (will fail to connect)
	targets[0].BaseURL = "http://127.0.0.1:65432"

	// Scrape immediately - this will fail because server is closed
	client.scrapeTarget(targets[0])

	snap := st.GetSnapshot("test-1")
	if snap == nil {
		t.Fatal("Expected snapshot to be stored")
	}
	if snap.Reachable {
		t.Error("Expected reachable to be false for unreachable target")
	}
	if snap.Error == "" {
		t.Error("Expected error message for unreachable target")
	}
}

func TestScraper_SkipsDisabledTargets(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Server should not be called for disabled target")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.ScrapeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "test-1", Name: "Test Node", BaseURL: server.URL, Enabled: false},
	}

	st := state.NewManager()
	client := NewClient(cfg, st, targets)

	// Scrape all (only disabled targets)
	client.scrapeAll()

	// No snapshot should be stored for disabled target
	snap := st.GetSnapshot("test-1")
	if snap != nil {
		t.Error("Expected no snapshot for disabled target")
	}
}

func TestScraper_DoesNotCrashOnError(t *testing.T) {
	cfg := &config.ScrapeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	// Use a port that's unlikely to have anything listening
	targets := []*config.TargetConfig{
		{ID: "test-1", Name: "Test Node", BaseURL: "http://127.0.0.1:65432", Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(cfg, st, targets)

	// Should not panic
	client.scrapeTarget(targets[0])

	snap := st.GetSnapshot("test-1")
	if snap == nil {
		t.Fatal("Expected snapshot even on error")
	}
	// Connection refused error should result in Reachable=false
	if snap.Reachable && snap.Error == "" {
		t.Error("Expected either Reachable=false or Error message on connection failure")
	}
}

func TestScraper_ParsesTovarischStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TovarischStatus{
			Service: "tovarisch",
			Version: "0.1.1",
			NodeID:  "router-1",
			Status:  "warn",
			Checks: []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
				Detail string `json:"detail"`
			}{
				{Name: "process", Status: "ok", Detail: "running"},
				{Name: "wg_peers", Status: "warn", Detail: "no peers detected"},
			},
		})
	}))
	defer server.Close()

	cfg := &config.ScrapeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "test-1", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(cfg, st, targets)

	client.scrapeTarget(targets[0])

	snap := st.GetSnapshot("test-1")
	if snap == nil {
		t.Fatal("Expected snapshot")
	}
	if len(snap.Checks) != 2 {
		t.Errorf("Expected 2 checks, got %d", len(snap.Checks))
	}
	if snap.Checks[0].Name != "process" {
		t.Errorf("Expected first check name 'process', got '%s'", snap.Checks[0].Name)
	}
}

// Helper function for auth header
func basicAuth(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

// Test helper to validate a config can be created and is valid
func TestNewClient(t *testing.T) {
	cfg := &config.ScrapeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "test-1", Name: "Test", BaseURL: "http://localhost:8080", Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(cfg, st, targets)

	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	if client.httpClient == nil {
		t.Error("httpClient should not be nil")
	}
}

func TestNewClient_WithDisabledTargets(t *testing.T) {
	cfg := &config.ScrapeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "test-1", Name: "Test Enabled", BaseURL: "http://localhost:8080", Enabled: true},
		{ID: "test-2", Name: "Test Disabled", BaseURL: "http://localhost:8081", Enabled: false},
	}

	st := state.NewManager()
	client := NewClient(cfg, st, targets)

	if client == nil {
		t.Fatal("NewClient returned nil")
	}
}

