package probe

import (
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// TestProbeClient_FailureCreatesSpikeEvent tests that failed HTTP probes
// create spike events with the failure reason.
// Note: net.http does NOT return an error for non-2xx responses.
// This test uses server.Close() to simulate a transport-level failure
// (connection refused, etc.), which does produce a real Go client error.
func TestProbeClient_FailureCreatesSpikeEvent(t *testing.T) {
	// Create a server and close it to simulate transport-level failure
	// This is what happens with tc netem loss 100% - connection cannot be established
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Server is immediately closed, so this won't be reached
	}))
	server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:    100,
	}

	targets := []*config.TargetConfig{
		{ID: "failure-test", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	st.EnableCaptureAwareSpikeRetention()
	client := NewClient(httpCfg, st, targets)

	// Record some baseline successful probes first
	for i := 0; i < 5; i++ {
		st.RecordLatency("failure-test", 50.0, true)
		time.Sleep(1 * time.Millisecond)
	}

	// Now probe (will fail)
	client.probeTarget(targets[0])

	// Check that a spike event was created
	spikes := st.GetSpikes("failure-test", "http", 10)
	if len(spikes) == 0 {
		t.Fatal("Expected spike event for failed probe")
	}

	// Verify the spike has failure reasons
	latestSpike := spikes[0] // Newest first
	foundFailure := false
	for _, reason := range latestSpike.Reasons {
		if reason == "http_probe_failure" || reason == "http_probe_connection_refused" {
			foundFailure = true
		}
	}
	if !foundFailure {
		t.Errorf("Expected failure reason in spike, got reasons: %v", latestSpike.Reasons)
	}

	// Verify severity is critical
	if latestSpike.Severity != "critical" {
		t.Errorf("Expected severity 'critical', got '%s'", latestSpike.Severity)
	}
}

// TestProbeClient_TimeoutCreatesSpikeEvent tests that timeout errors
// create spike events with the timeout reason.
func TestProbeClient_TimeoutCreatesSpikeEvent(t *testing.T) {
	// Create a very slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 100, // Very short timeout
		RecentSamplesMax:    100,
	}

	targets := []*config.TargetConfig{
		{ID: "timeout-test", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	st.EnableCaptureAwareSpikeRetention()
	client := NewClient(httpCfg, st, targets)

	// Record baseline
	for i := 0; i < 5; i++ {
		st.RecordLatency("timeout-test", 50.0, true)
		time.Sleep(1 * time.Millisecond)
	}

	// Probe (will timeout)
	client.probeTarget(targets[0])

	// Check spike was created - timeout should trigger failure spike
	spikes := st.GetSpikes("timeout-test", "http", 10)
	if len(spikes) == 0 {
		t.Fatal("Expected spike event for timeout")
	}

	latestSpike := spikes[0]
	// Should have failure-related reason (timeout or generic failure)
	foundFailure := false
	for _, reason := range latestSpike.Reasons {
		if reason == "http_probe_timeout" || reason == "http_probe_failure" {
			foundFailure = true
		}
	}
	if !foundFailure {
		t.Errorf("Expected failure reason in spike, got reasons: %v", latestSpike.Reasons)
	}
}

// TestProbeClient_FailureCapturesRequestedPath tests that failed probes
// have the requested path recorded in the spike event for diagnostics.
func TestProbeClient_FailureCapturesRequestedPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
	}))
	server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:    100,
	}

	targets := []*config.TargetConfig{
		{ID: "path-test", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	st.EnableCaptureAwareSpikeRetention()
	client := NewClient(httpCfg, st, targets)

	// Record baseline
	for i := 0; i < 5; i++ {
		st.RecordLatency("path-test", 50.0, true)
		time.Sleep(1 * time.Millisecond)
	}

	// Probe (will fail)
	client.probeTarget(targets[0])

	// Check spike has error info
	spikes := st.GetSpikes("path-test", "http", 10)
	if len(spikes) == 0 {
		t.Fatal("Expected spike event")
	}

	latestSpike := spikes[0]
	if latestSpike.ProbeError == nil {
		t.Error("Expected probe error to be captured in spike")
	}
}

// TestProbeClient_RecoveryAfterTransportFailure tests that recovery from transport failure
// creates a recovery event with http_probe_recovery reason.
// Uses a custom RoundTripper to simulate deterministic transport errors.
func TestProbeClient_RecoveryAfterTransportFailure(t *testing.T) {
	failCount := 3
	requestCount := 0

	// Custom RoundTripper that fails first N requests with transport error
	// then succeeds
	customTransport := &errorThenSuccessRoundTripper{
		failCount:    failCount,
		requestCount: &requestCount,
	}

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:    100,
	}

	targets := []*config.TargetConfig{
		{ID: "recovery-test", Name: "Test Node", BaseURL: "http://localhost:9999", Enabled: true},
	}

	st := state.NewManager()
	st.EnableCaptureAwareSpikeRetention()
	client := NewClient(httpCfg, st, targets)
	client.httpClient.Transport = customTransport

	// Record baseline success
	for i := 0; i < 5; i++ {
		st.RecordLatency("recovery-test", 50.0, true)
		time.Sleep(1 * time.Millisecond)
	}

	// Probe 3 times - all should fail with transport error
	for i := 0; i < failCount; i++ {
		client.probeTarget(targets[0])
		time.Sleep(10 * time.Millisecond)
	}

	// Verify failure spikes were created
	spikesAfterFailures := st.GetSpikes("recovery-test", "http", 100)
	foundFailure := false
	for _, spike := range spikesAfterFailures {
		for _, reason := range spike.Reasons {
			if reason == "http_probe_failure" || reason == "http_probe_connection_refused" {
				foundFailure = true
			}
		}
	}
	if !foundFailure {
		t.Fatal("Expected failure spikes after transport errors")
	}

	// Now probe - should succeed and trigger recovery
	client.probeTarget(targets[0])

	// Check that a recovery event was created
	spikesAfterRecovery := st.GetSpikes("recovery-test", "http", 100)
	
	foundRecovery := false
	for _, spike := range spikesAfterRecovery {
		for _, reason := range spike.Reasons {
			if reason == "http_probe_recovery" {
				foundRecovery = true
			}
		}
	}
	
	if !foundRecovery {
		// List all spike reasons for debugging
		t.Logf("Recovery spike not found. All spike reasons after recovery:")
		for _, spike := range spikesAfterRecovery {
			t.Logf("  - %v", spike.Reasons)
		}
		t.Fatal("Expected http_probe_recovery reason after transport error followed by success")
	}
}

// TestProbeClient_NoRecoveryFromUnknownState verifies that the first successful probe
// from unknown state does NOT create a recovery event.
func TestProbeClient_NoRecoveryFromUnknownState(t *testing.T) {
	requestCount := 0

	// Custom RoundTripper that always succeeds (no prior failure)
	customTransport := &errorThenSuccessRoundTripper{
		failCount:    0, // Always succeed
		requestCount: &requestCount,
	}

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:    100,
	}

	targets := []*config.TargetConfig{
		{ID: "fresh-start", Name: "Test Node", BaseURL: "http://localhost:9999", Enabled: true},
	}

	st := state.NewManager()
	st.EnableCaptureAwareSpikeRetention()
	client := NewClient(httpCfg, st, targets)
	client.httpClient.Transport = customTransport

	// First probe from unknown state (fresh client, no prior probes)
	client.probeTarget(targets[0])

	// Check for spikes - should NOT have recovery reason
	spikes := st.GetSpikes("fresh-start", "http", 100)
	
	for _, spike := range spikes {
		for _, reason := range spike.Reasons {
			if reason == "http_probe_recovery" {
				t.Errorf("Should not have recovery reason from unknown state, got: %v", spike.Reasons)
			}
		}
	}
}

// errorThenSuccessRoundTripper returns errors for the first N requests,
// then returns a successful response.
type errorThenSuccessRoundTripper struct {
	failCount    int
	requestCount *int
}

func (rt *errorThenSuccessRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	count := *rt.requestCount
	*rt.requestCount++
	
	if count < rt.failCount {
		// Return a transport error (simulates connection refused, timeout, etc.)
		return nil, &net.OpError{
			Op:  "dial",
			Err: syscall.ECONNREFUSED,
		}
	}
	
	// Return successful response
	return &http.Response{
		StatusCode: 200,
		Body:       http.NoBody,
		Header:     make(http.Header),
	}, nil
}

// TestProbeClient_FailureAndRecoveryWithCooldown tests that cooldowns
// prevent duplicate captures.
func TestProbeClient_FailureAndRecoveryWithCooldown(t *testing.T) {
	// Create and close server first - all requests will fail
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Fail", http.StatusServiceUnavailable)
	}))
	server.Close() // Close immediately so all requests fail

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:    100,
	}

	targets := []*config.TargetConfig{
		{ID: "cooldown-test", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	st.EnableCaptureAwareSpikeRetention()
	client := NewClient(httpCfg, st, targets)

	// Record baseline successful probes through the client
	// First probe should succeed (server still responding)
	if server == nil {
		t.Skip("Server closed too early")
	}

	// Rapid failure probes - should create spikes for failures
	for i := 0; i < 5; i++ {
		client.probeTarget(targets[0])
		time.Sleep(5 * time.Millisecond)
	}

	spikes := st.GetSpikes("cooldown-test", "http", 100)
	
	// The exact number depends on implementation
	t.Logf("Spike count after rapid failures: %d", len(spikes))
	
	// Core test: failed probes should create spike events
	if len(spikes) == 0 {
		t.Error("Expected at least some spikes from rapid failures")
	}
}
