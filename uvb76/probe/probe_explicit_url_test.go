package probe

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// TestProbeClient_ExplicitProbeURL verifies that when a target has probe_url set,
// the probe uses that exact URL instead of base_url + /status.
func TestProbeClient_ExplicitProbeURL(t *testing.T) {
	// Create a server with two endpoints:
	// - /lab/probe returns 503 (the explicit probe URL)
	// - /status returns 200 (what would be used without explicit probe_url)
	var probedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probedPath = r.URL.Path
		if probedPath == "/lab/probe" {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(200)
		}
	}))
	defer server.Close()

	// Configuration with explicit probe_url pointing to /lab/probe
	// base_url is set to server.URL (which will append /status if used incorrectly)
	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:    30,
	}

	// Target with explicit probe_url set to /lab/probe
	targets := []*config.TargetConfig{
		{
			ID:       "explicit-probe-url-test",
			Name:     "Test Node",
			BaseURL:  server.URL, // Would result in /status if incorrectly used
			ProbeURL: server.URL + "/lab/probe", // Explicit probe URL
			Enabled:  true,
		},
	}

	st := state.NewManager()
	client := NewClient(httpCfg, st, targets)

	// Record some healthy samples first
	for i := 0; i < 25; i++ {
		st.RecordLatency("explicit-probe-url-test", 50.0, true)
	}

	// Probe - should use /lab/probe and get 503
	client.probeTarget(targets[0])

	// Verify the path that was actually probed
	if probedPath != "/lab/probe" {
		t.Errorf("Expected probe to hit /lab/probe, but got %s", probedPath)
	}

	// Verify a spike was created with http_probe_503 (not http_probe_404)
	spikes := st.GetSpikes("explicit-probe-url-test", "http", 10)
	if len(spikes) == 0 {
		t.Fatal("Expected a spike event to be created")
	}

	// Check spike has the expected http_probe_503 reason (not 404)
	found503Reason := false
	found404Reason := false
	for _, spike := range spikes {
		for _, reason := range spike.Reasons {
			if reason == "http_probe_503" {
				found503Reason = true
			}
			if reason == "http_probe_404" {
				found404Reason = true
			}
		}
	}

	if found404Reason {
		t.Errorf("Probe incorrectly hit /status (404) instead of /lab/probe (503)")
	}
	if !found503Reason {
		t.Errorf("Expected spike reason to contain 'http_probe_503', got reasons: %v", spikes[len(spikes)-1].Reasons)
	}
}

// TestProbeClient_ProbeTarget_UsesExplicitProbeURL verifies ProbeTarget method also respects probe_url.
func TestProbeClient_ProbeTarget_UsesExplicitProbeURL(t *testing.T) {
	var probedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probedPath = r.URL.Path
		w.WriteHeader(503)
	}))
	defer server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:    30,
	}

	targets := []*config.TargetConfig{
		{
			ID:       "probe-target-test",
			Name:     "Test Node",
			BaseURL:  server.URL,
			ProbeURL: server.URL + "/lab/probe",
			Enabled:  true,
		},
	}

	st := state.NewManager()
	client := NewClient(httpCfg, st, targets)

	result := client.ProbeTarget(targets[0])

	// Verify the path that was actually probed
	if probedPath != "/lab/probe" {
		t.Errorf("Expected ProbeTarget to hit /lab/probe, but got %s", probedPath)
	}

	// Verify the result reflects 503
	if result.Reachable {
		t.Error("Expected Reachable=false for 503 response")
	}
	if result.StatusCode != 503 {
		t.Errorf("Expected StatusCode=503, got %d", result.StatusCode)
	}
}
