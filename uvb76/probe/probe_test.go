package probe

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

func TestProbeClient_RecordsLatencyOnSuccess(t *testing.T) {
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

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "probe-test-1", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(httpCfg, st, targets)

	// Probe
	client.probeTarget(targets[0])

	// Check latency was recorded
	samples := st.GetRecentLatencySamples("probe-test-1", 10)
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

func TestProbeClient_RecordsLatencyOnFailure(t *testing.T) {
	// Create a server and close it to simulate failure
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
	}))
	server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "probe-test-2", Name: "Test Node", BaseURL: "http://127.0.0.1:65434", Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(httpCfg, st, targets)

	// Probe (will fail)
	client.probeTarget(targets[0])

	// Check latency was still recorded even on failure
	samples := st.GetRecentLatencySamples("probe-test-2", 10)
	if len(samples) != 1 {
		t.Fatalf("Expected 1 latency sample on failure, got %d", len(samples))
	}
	if samples[0].Reachable {
		t.Error("Expected reachable to be false for failed request")
	}
}

func TestProbeClient_LatencySummary(t *testing.T) {
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

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "probe-summary-test", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(httpCfg, st, targets)

	// Probe multiple times
	client.probeTarget(targets[0])
	client.probeTarget(targets[0])
	client.probeTarget(targets[0])

	// Check summary
	summary := st.GetLatencySummary("probe-summary-test")
	if summary.SampleCount != 3 {
		t.Errorf("Expected 3 samples, got %d", summary.SampleCount)
	}
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

func TestProbeClient_LatencyIndependentPerTarget(t *testing.T) {
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

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "target-fast", Name: "Fast Target", BaseURL: server1.URL, Enabled: true},
		{ID: "target-slow", Name: "Slow Target", BaseURL: server2.URL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(httpCfg, st, targets)

	// Probe both
	client.probeTarget(targets[0])
	client.probeTarget(targets[1])

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

func TestProbeClient_LatencyRecordedOnTimeout(t *testing.T) {
	// Create a very slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
	}))
	defer server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 100, // Very short timeout
	}

	targets := []*config.TargetConfig{
		{ID: "timeout-test", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(httpCfg, st, targets)

	// Probe (will timeout)
	client.probeTarget(targets[0])

	// Check latency was still recorded
	samples := st.GetRecentLatencySamples("timeout-test", 10)
	if len(samples) != 1 {
		t.Fatalf("Expected 1 latency sample on timeout, got %d", len(samples))
	}
	if samples[0].Reachable {
		t.Error("Expected reachable to be false for timeout request")
	}
}

func TestProbeClient_DisabledDoesNotProbe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Server should not be called when disabled")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	enabled := false
	httpCfg := &config.HTTPProbeConfig{
		Enabled:             &enabled,
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "disabled-test", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(httpCfg, st, targets)

	// Should not probe since disabled
	client.probeAll()

	// No latency should be recorded
	samples := st.GetRecentLatencySamples("disabled-test", 10)
	if len(samples) != 0 {
		t.Errorf("Expected 0 latency samples when disabled, got %d", len(samples))
	}
}

func TestProbeClient_IsEnabled(t *testing.T) {
	// Test with enabled = nil (default)
	httpCfg1 := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}
	targets := []*config.TargetConfig{
		{ID: "test-1", Name: "Test", BaseURL: "http://localhost:8080", Enabled: true},
	}
	st := state.NewManager()
	client1 := NewClient(httpCfg1, st, targets)
	if !client1.IsEnabled() {
		t.Error("Expected enabled when Enabled is nil")
	}

	// Test with enabled = true
	enabled := true
	httpCfg2 := &config.HTTPProbeConfig{
		Enabled:             &enabled,
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}
	client2 := NewClient(httpCfg2, st, targets)
	if !client2.IsEnabled() {
		t.Error("Expected enabled when Enabled is true")
	}

	// Test with enabled = false
	disabled := false
	httpCfg3 := &config.HTTPProbeConfig{
		Enabled:             &disabled,
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}
	client3 := NewClient(httpCfg3, st, targets)
	if client3.IsEnabled() {
		t.Error("Expected disabled when Enabled is false")
	}
}

func TestProbeClient_ProbeTargetReturnsDetailedResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
	}))
	defer server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "detailed-test", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(httpCfg, st, targets)

	result := client.ProbeTarget(targets[0])

	if result.TargetID != "detailed-test" {
		t.Errorf("Expected target ID 'detailed-test', got '%s'", result.TargetID)
	}
	if result.LatencyMs <= 0 {
		t.Errorf("Expected positive latency, got %f", result.LatencyMs)
	}
	if !result.Reachable {
		t.Error("Expected reachable to be true for successful probe")
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", result.StatusCode)
	}
	if result.Error != "" {
		t.Errorf("Expected no error, got '%s'", result.Error)
	}
	if result.Timestamp.IsZero() {
		t.Error("Expected non-zero timestamp")
	}
}

func TestProbeClient_ProbeTargetHandlesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "error-test", Name: "Test Node", BaseURL: "http://127.0.0.1:65435", Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(httpCfg, st, targets)

	result := client.ProbeTarget(targets[0])

	if result.TargetID != "error-test" {
		t.Errorf("Expected target ID 'error-test', got '%s'", result.TargetID)
	}
	// Latency should be non-negative (connection may fail very fast)
	if result.LatencyMs < 0 {
		t.Errorf("Expected non-negative latency for failed probe, got %f", result.LatencyMs)
	}
	if result.Reachable {
		t.Error("Expected reachable to be false for failed probe")
	}
	if result.Error == "" {
		t.Error("Expected error message for failed probe")
	}
}

func TestProbeClient_SkipsDisabledTargets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Server should not be called for disabled target")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "disabled-target", Name: "Test Node", BaseURL: server.URL, Enabled: false},
	}

	st := state.NewManager()
	client := NewClient(httpCfg, st, targets)

	// Probe all (only disabled targets)
	client.probeAll()

	// No latency should be recorded for disabled target
	samples := st.GetRecentLatencySamples("disabled-target", 10)
	if len(samples) != 0 {
		t.Errorf("Expected 0 latency samples for disabled target, got %d", len(samples))
	}
}

func TestProbeClient_DoesNotUpdateSnapshots(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"service": "tovarisch",
			"version": "0.1.1",
			"node_id": "test-node",
			"status":  "ok",
		})
	}))
	defer server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "snapshot-test", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(httpCfg, st, targets)

	// Probe
	client.probeTarget(targets[0])

	// Latency should be recorded
	samples := st.GetRecentLatencySamples("snapshot-test", 10)
	if len(samples) != 1 {
		t.Errorf("Expected 1 latency sample, got %d", len(samples))
	}

	// But snapshot should NOT be stored (probe doesn't update snapshots)
	snap := st.GetSnapshot("snapshot-test")
	if snap != nil {
		t.Error("Probe should not update snapshots - expected nil snapshot")
	}
}
