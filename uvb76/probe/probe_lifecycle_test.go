package probe

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

func TestProbeClient_StartStopCleanLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     1, // Fast interval for testing
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "lifecycle-test", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(httpCfg, st, targets)

	// Start
	client.Start()

	// Wait for at least one probe cycle
	time.Sleep(2500 * time.Millisecond)

	// Stop
	client.Stop()

	// Should have recorded some latency samples
	samples := st.GetRecentLatencySamples("lifecycle-test", 100)
	if len(samples) == 0 {
		t.Error("Expected latency samples after start/stop lifecycle")
	}
}

func TestProbeClient_StartDoesNothingWhenDisabled(t *testing.T) {
	enabled := false
	httpCfg := &config.HTTPProbeConfig{
		Enabled:             &enabled,
		IntervalSeconds:     1,
		TimeoutMilliseconds: 5000,
	}

	targets := []*config.TargetConfig{
		{ID: "disabled-start-test", Name: "Test", BaseURL: "http://localhost:8080", Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(httpCfg, st, targets)

	// Start should not do anything when disabled
	client.Start()

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Stop should be safe even though nothing was started
	client.Stop()

	// No latency should be recorded
	samples := st.GetRecentLatencySamples("disabled-start-test", 10)
	if len(samples) != 0 {
		t.Errorf("Expected 0 latency samples when disabled, got %d", len(samples))
	}
}
