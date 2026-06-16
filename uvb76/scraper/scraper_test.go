package scraper

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

// Latency Tests

func TestScraper_RecordsLatencyOnSuccess(t *testing.T) {
	// Create a test server with a small delay to ensure measurable latency
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"service": "tovarisch",
			"version": "0.1.1",
			"node_id": "test-node",
			"status":  "ok",
		})
	}))
	defer server.Close()

	cfg := &config.ScrapeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "latency-test-1", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(cfg, st, targets)

	// Scrape
	client.scrapeTarget(targets[0])

	// Check latency was recorded
	samples := st.GetRecentLatencySamples("latency-test-1", 10)
	if len(samples) != 1 {
		t.Fatalf("Expected 1 latency sample, got %d", len(samples))
	}
	if !samples[0].Reachable {
		t.Error("Expected reachable to be true for successful request")
	}
	if samples[0].LatencyMs <= 0 {
		t.Errorf("Expected positive latency, got %f", samples[0].LatencyMs)
	}
}

func TestScraper_RecordsLatencyOnFailure(t *testing.T) {
	// Create a server and close it to simulate failure
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
	}))
	server.Close()

	cfg := &config.ScrapeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "latency-test-2", Name: "Test Node", BaseURL: "http://127.0.0.1:65433", Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(cfg, st, targets)

	// Scrape (will fail)
	client.scrapeTarget(targets[0])

	// Check latency was still recorded even on failure
	samples := st.GetRecentLatencySamples("latency-test-2", 10)
	if len(samples) != 1 {
		t.Fatalf("Expected 1 latency sample on failure, got %d", len(samples))
	}
	if samples[0].Reachable {
		t.Error("Expected reachable to be false for failed request")
	}
}

func TestScraper_LatencySummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add small delay to ensure measurable latency
		time.Sleep(2 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"service": "tovarisch",
			"version": "0.1.1",
			"node_id": "test-node",
			"status":  "ok",
		})
	}))
	defer server.Close()

	cfg := &config.ScrapeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "latency-summary-test", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(cfg, st, targets)

	// Scrape multiple times
	client.scrapeTarget(targets[0])
	client.scrapeTarget(targets[0])
	client.scrapeTarget(targets[0])

	// Check summary
	summary := st.GetLatencySummary("latency-summary-test")
	if summary.SampleCount != 3 {
		t.Errorf("Expected 3 samples, got %d", summary.SampleCount)
	}
	// Note: Min/Max may be 0 if the request was very fast on localhost
	// But we should have 3 samples recorded
	if summary.MaxLatencyMs < 0 {
		t.Errorf("Expected non-negative max latency, got %f", summary.MaxLatencyMs)
	}
	if summary.AvgLatencyMs < 0 {
		t.Errorf("Expected non-negative avg latency, got %f", summary.AvgLatencyMs)
	}
	// Verify histogram structure exists
	if len(summary.Histogram.Buckets) == 0 {
		t.Error("Expected histogram buckets to be set")
	}
	if len(summary.Histogram.Counts) == 0 {
		t.Error("Expected histogram counts to be set")
	}
	// Total histogram count should match sample count
	total := int64(0)
	for _, c := range summary.Histogram.Counts {
		total += c
	}
	if total != 3 {
		t.Errorf("Expected 3 total in histogram, got %d", total)
	}
}

func TestScraper_LatencyIndependentPerTarget(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
	}))
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
	}))
	defer server1.Close()
	defer server2.Close()

	cfg := &config.ScrapeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "target-fast", Name: "Fast Target", BaseURL: server1.URL, Enabled: true},
		{ID: "target-slow", Name: "Slow Target", BaseURL: server2.URL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(cfg, st, targets)

	// Scrape both
	client.scrapeTarget(targets[0])
	client.scrapeTarget(targets[1])

	// Check summaries are independent
	summary1 := st.GetLatencySummary("target-fast")
	summary2 := st.GetLatencySummary("target-slow")

	if summary1.SampleCount != 1 {
		t.Errorf("Expected fast target to have 1 sample, got %d", summary1.SampleCount)
	}
	if summary2.SampleCount != 1 {
		t.Errorf("Expected slow target to have 1 sample, got %d", summary2.SampleCount)
	}

	// Fast target should have lower latency than slow target
	if summary1.MaxLatencyMs >= summary2.MaxLatencyMs {
		// This may not always hold due to timing variations, but generally should
		t.Logf("Note: Fast target latency (%f) >= Slow target latency (%f) - timing may vary", summary1.MaxLatencyMs, summary2.MaxLatencyMs)
	}
}

func TestScraper_LatencyRecordedOnTimeout(t *testing.T) {
	// Create a very slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
	}))
	defer server.Close()

	cfg := &config.ScrapeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 100, // Very short timeout
	}

	targets := []*config.TargetConfig{
		{ID: "timeout-test", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(cfg, st, targets)

	// Scrape (will timeout)
	client.scrapeTarget(targets[0])

	// Check snapshot shows unreachable
	snap := st.GetSnapshot("timeout-test")
	if snap == nil {
		t.Fatal("Expected snapshot to be stored")
	}
	if snap.Reachable {
		t.Error("Expected reachable to be false on timeout")
	}

	// Check latency was still recorded
	samples := st.GetRecentLatencySamples("timeout-test", 10)
	if len(samples) != 1 {
		t.Fatalf("Expected 1 latency sample on timeout, got %d", len(samples))
	}
	if samples[0].Reachable {
		t.Error("Expected reachable to be false for timeout request")
	}
}
