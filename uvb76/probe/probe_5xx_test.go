package probe

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// ============================================================================
// HTTP Status Classification Tests
// ============================================================================

func TestClassifyHTTPProbeStatus_Success2xx(t *testing.T) {
	testCases := []struct {
		statusCode int
	}{
		{200},
		{201},
		{204},
		{299},
	}

	for _, tc := range testCases {
		result := ClassifyHTTPProbeStatus(tc.statusCode)
		if !result.Reachable {
			t.Errorf("Expected Reachable=true for status %d, got %v", tc.statusCode, result.Reachable)
		}
		if result.Reason != "" {
			t.Errorf("Expected empty Reason for status %d, got '%s'", tc.statusCode, result.Reason)
		}
	}
}

func TestClassifyHTTPProbeStatus_Success3xx(t *testing.T) {
	testCases := []struct {
		statusCode int
	}{
		{301},
		{302},
		{304},
		{399},
	}

	for _, tc := range testCases {
		result := ClassifyHTTPProbeStatus(tc.statusCode)
		if !result.Reachable {
			t.Errorf("Expected Reachable=true for status %d, got %v", tc.statusCode, result.Reachable)
		}
		if result.Reason != "" {
			t.Errorf("Expected empty Reason for status %d, got '%s'", tc.statusCode, result.Reason)
		}
	}
}

func TestClassifyHTTPProbeStatus_5xx(t *testing.T) {
	testCases := []struct {
		statusCode      int
		expectedReason  string
	}{
		{500, "http_probe_5xx"},
		{501, "http_probe_5xx"},
		{502, "http_probe_502"},
		{503, "http_probe_503"},
		{504, "http_probe_504"},
		{599, "http_probe_5xx"},
	}

	for _, tc := range testCases {
		result := ClassifyHTTPProbeStatus(tc.statusCode)
		if result.Reachable {
			t.Errorf("Expected Reachable=false for status %d, got %v", tc.statusCode, result.Reachable)
		}
		if result.Reason != tc.expectedReason {
			t.Errorf("Expected Reason='%s' for status %d, got '%s'", tc.expectedReason, tc.statusCode, result.Reason)
		}
	}
}

func TestClassifyHTTPProbeStatus_4xx(t *testing.T) {
	testCases := []struct {
		statusCode      int
		expectedReason  string
	}{
		{400, "http_probe_4xx"},
		{401, "http_probe_4xx"},
		{403, "http_probe_4xx"},
		{404, "http_probe_404"},
		{429, "http_probe_4xx"},
		{499, "http_probe_4xx"},
	}

	for _, tc := range testCases {
		result := ClassifyHTTPProbeStatus(tc.statusCode)
		if result.Reachable {
			t.Errorf("Expected Reachable=false for status %d, got %v", tc.statusCode, result.Reachable)
		}
		if result.Reason != tc.expectedReason {
			t.Errorf("Expected Reason='%s' for status %d, got '%s'", tc.expectedReason, tc.statusCode, result.Reason)
		}
	}
}

func TestClassifyHTTPProbeStatus_1xx(t *testing.T) {
	// 1xx are unexpected but should be classified as unhealthy
	result := ClassifyHTTPProbeStatus(100)
	if result.Reachable {
		t.Errorf("Expected Reachable=false for status 100, got %v", result.Reachable)
	}
	if !strings.Contains(result.Reason, "http_probe_unexpected_status_100") {
		t.Errorf("Expected Reason to contain 'http_probe_unexpected_status_100' for status 100, got '%s'", result.Reason)
	}
}

// ============================================================================
// HTTP 5xx Spike Integration Tests
// ============================================================================

func TestProbeClient_HTTP503CreatesSpikeEvent(t *testing.T) {
	// Create a server that returns 503 Service Unavailable
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:    30,
	}

	targets := []*config.TargetConfig{
		{ID: "http-503-test", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(httpCfg, st, targets)

	// Record some healthy samples first to establish baseline
	for i := 0; i < 25; i++ {
		st.RecordLatency("http-503-test", 50.0, true)
	}

	// Probe - should return 503
	client.probeTarget(targets[0])

	// Check that the sample was recorded as unreachable
	samples := st.GetRecentLatencySamples("http-503-test", 10)
	if len(samples) < 1 {
		t.Fatal("Expected at least 1 latency sample")
	}

	// The last sample should be unreachable (HTTP 503)
	lastSample := samples[len(samples)-1]
	if lastSample.Reachable {
		t.Error("Expected last sample to be unreachable for HTTP 503 response")
	}

	// Check that a spike event was created
	spikes := st.GetSpikes("http-503-test", "http", 10)
	if len(spikes) == 0 {
		t.Fatal("Expected a spike event to be created for HTTP 503")
	}

	// Check spike has the expected reason
	found503Reason := false
	for _, spike := range spikes {
		for _, reason := range spike.Reasons {
			if reason == "http_probe_503" {
				found503Reason = true
				break
			}
		}
		if found503Reason {
			break
		}
	}

	if !found503Reason {
		t.Errorf("Expected spike reason to contain 'http_probe_503', got reasons: %v", spikes[len(spikes)-1].Reasons)
	}

	// Check spike severity is critical
	if spikes[len(spikes)-1].Severity != "critical" {
		t.Errorf("Expected severity 'critical' for HTTP 503 spike, got '%s'", spikes[len(spikes)-1].Severity)
	}

	// Check HTTP status is recorded in spike
	if spikes[len(spikes)-1].HTTPStatus == nil || *spikes[len(spikes)-1].HTTPStatus != 503 {
		t.Errorf("Expected HTTPStatus=503 in spike, got %v", spikes[len(spikes)-1].HTTPStatus)
	}
}

func TestProbeClient_HTTP502CreatesSpikeEvent(t *testing.T) {
	// Create a server that returns 502 Bad Gateway
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:    30,
	}

	targets := []*config.TargetConfig{
		{ID: "http-502-test", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(httpCfg, st, targets)

	// Record some healthy samples first
	for i := 0; i < 25; i++ {
		st.RecordLatency("http-502-test", 50.0, true)
	}

	// Probe - should return 502
	client.probeTarget(targets[0])

	// Check that a spike event was created
	spikes := st.GetSpikes("http-502-test", "http", 10)
	if len(spikes) == 0 {
		t.Fatal("Expected a spike event to be created for HTTP 502")
	}

	// Check spike has the expected reason
	found502Reason := false
	for _, spike := range spikes {
		for _, reason := range spike.Reasons {
			if reason == "http_probe_502" {
				found502Reason = true
				break
			}
		}
		if found502Reason {
			break
		}
	}

	if !found502Reason {
		t.Errorf("Expected spike reason to contain 'http_probe_502', got reasons: %v", spikes[len(spikes)-1].Reasons)
	}
}

func TestProbeClient_HTTP200DoesNotCreate5xxSpike(t *testing.T) {
	// Create a healthy server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
	}))
	defer server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:    30,
	}

	targets := []*config.TargetConfig{
		{ID: "http-200-test", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(httpCfg, st, targets)

	// Record some healthy samples first
	for i := 0; i < 25; i++ {
		st.RecordLatency("http-200-test", 50.0, true)
	}

	// Probe - should return 200 OK
	client.probeTarget(targets[0])

	// Check that the sample was recorded as reachable
	samples := st.GetRecentLatencySamples("http-200-test", 10)
	lastSample := samples[len(samples)-1]
	if !lastSample.Reachable {
		t.Error("Expected last sample to be reachable for HTTP 200 response")
	}

	// No spike should be created for healthy response
	spikes := st.GetSpikes("http-200-test", "http", 10)
	// There might be a latency spike from normal variation, but no 5xx reason
	for _, spike := range spikes {
		for _, reason := range spike.Reasons {
			if strings.Contains(reason, "http_probe_5") || strings.Contains(reason, "http_probe_4") {
				t.Errorf("Did not expect 5xx/4xx spike reason for HTTP 200, got: %s", reason)
			}
		}
	}
}

func TestProbeClient_HTTP404CreatesSpikeEvent(t *testing.T) {
	// Create a server that returns 404 Not Found
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:    30,
	}

	targets := []*config.TargetConfig{
		{ID: "http-404-test", Name: "Test Node", BaseURL: server.URL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(httpCfg, st, targets)

	// Record some healthy samples first
	for i := 0; i < 25; i++ {
		st.RecordLatency("http-404-test", 50.0, true)
	}

	// Probe - should return 404
	client.probeTarget(targets[0])

	// Check that a spike event was created
	spikes := st.GetSpikes("http-404-test", "http", 10)
	if len(spikes) == 0 {
		t.Fatal("Expected a spike event to be created for HTTP 404")
	}

	// Check spike has the expected reason
	found404Reason := false
	for _, spike := range spikes {
		for _, reason := range spike.Reasons {
			if reason == "http_probe_404" {
				found404Reason = true
				break
			}
		}
		if found404Reason {
			break
		}
	}

	if !found404Reason {
		t.Errorf("Expected spike reason to contain 'http_probe_404', got reasons: %v", spikes[len(spikes)-1].Reasons)
	}
}

func TestProbeClient_TransportErrorCreatesDistinctSpike(t *testing.T) {
	// Create a server and capture its URL before closing (transport error)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	closedURL := server.URL
	server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     60,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:    30,
	}

	targets := []*config.TargetConfig{
		{ID: "transport-error-test", Name: "Test Node", BaseURL: closedURL, Enabled: true},
	}

	st := state.NewManager()
	client := NewClient(httpCfg, st, targets)

	// Record some healthy samples first
	for i := 0; i < 25; i++ {
		st.RecordLatency("transport-error-test", 50.0, true)
	}

	// Probe - should fail with connection error
	client.probeTarget(targets[0])

	// Check that a spike event was created
	spikes := st.GetSpikes("transport-error-test", "http", 10)
	if len(spikes) == 0 {
		t.Fatal("Expected a spike event to be created for transport error")
	}

	// Check spike does NOT have http_probe_5xx reason (it's a transport error, not HTTP error)
	latestSpike := spikes[len(spikes)-1]
	for _, reason := range latestSpike.Reasons {
		if strings.Contains(reason, "http_probe_5") {
			t.Errorf("Did not expect 5xx spike reason for transport error, got: %s", reason)
		}
	}

	// Transport errors should have a generic http_probe_failure reason
	foundFailureReason := false
	for _, reason := range latestSpike.Reasons {
		if strings.Contains(reason, "http_probe_failure") || strings.Contains(reason, "http_probe_connection") {
			foundFailureReason = true
			break
		}
	}
	if !foundFailureReason {
		t.Errorf("Expected transport error spike to have failure-related reason, got: %v", latestSpike.Reasons)
	}
}
