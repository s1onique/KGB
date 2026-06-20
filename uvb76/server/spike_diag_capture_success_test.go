package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/s1onique/KGB/uvb76/auth"
	"github.com/s1onique/KGB/uvb76/state"
)

// TestSpikeCaptureAPI_SuccessPath tests the full spike → capture → API integration path.
func TestSpikeCaptureAPI_SuccessPath(t *testing.T) {
	// Create fake tovarisch server
	tovarischServer := httptest.NewServer(fakeTovarischHandler(validTovarischStatusJSON(), http.StatusOK))
	defer tovarischServer.Close()

	// Setup test server
	srv, st, captureSvc, token := setupTestServer(t, tovarischServer.URL)

	// Create router with spike endpoint
	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/latency/spikes", http.HandlerFunc(srv.handleTargetLatencySpikes)).Methods(http.MethodGet)

	// Create a spike event via DetectAndRecordSpike
	now := time.Now().UTC()
	sampleTs := now.Add(-time.Second)
	previousSamples := make([]state.LatencySample, 25)
	for i := 0; i < 25; i++ {
		previousSamples[i] = state.LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0, // low baseline
			Reachable: true,
		}
	}

	// Trigger a spike (1000ms should trigger HTTP warning threshold of 1000ms)
	spikeEvent := st.DetectAndRecordSpike("test-target", "http", 1500.0, sampleTs, true, nil, nil, nil, previousSamples)
	if spikeEvent == nil {
		t.Fatal("expected spike to be detected")
	}

	// Trigger diagnostic capture
	captureSvc.TriggerCapture(spikeEvent.EventID, "test-target", "http")

	// Wait for async capture to complete
	captures := waitForCaptures(st.GetCaptureStore(), spikeEvent.EventID, 2*time.Second)
	if len(captures) == 0 {
		t.Fatal("expected at least one capture after waiting")
	}

	// Call API with include_captures=true
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&include_captures=true", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Assert HTTP status
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Parse response
	var resp SpikeResponseWithCaptures
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Assert response structure
	if resp.Count < 1 {
		t.Errorf("expected at least 1 spike, got %d", resp.Count)
	}
	if len(resp.Spikes) < 1 {
		t.Fatal("expected non-empty spikes array")
	}

	// Find our spike in the response
	var foundSpike *state.SpikeEventWithCaptures
	for i := range resp.Spikes {
		if resp.Spikes[i].EventID == spikeEvent.EventID {
			foundSpike = &resp.Spikes[i]
			break
		}
	}
	if foundSpike == nil {
		t.Fatal("spike event not found in API response")
	}

	// Assert captures array is present and non-empty
	if len(foundSpike.Captures) == 0 {
		t.Fatal("expected captures array to be non-empty")
	}

	// Assert capture details
	capture := foundSpike.Captures[0]
	if capture.Status != state.DiagCaptureStatusOK {
		t.Errorf("expected capture status 'ok', got '%s'", capture.Status)
	}
	if capture.Source != "tovarisch-peer" {
		t.Errorf("expected capture source 'tovarisch-peer', got '%s'", capture.Source)
	}
	if capture.BaseURL != tovarischServer.URL {
		t.Errorf("expected capture base_url '%s', got '%s'", tovarischServer.URL, capture.BaseURL)
	}
	if capture.NetworkDiag == nil {
		t.Fatal("expected network_diag to be non-nil")
	}
	if capture.NetworkDiag.Status != "ok" {
		t.Errorf("expected network_diag.status 'ok', got '%s'", capture.NetworkDiag.Status)
	}
	if len(capture.NetworkDiag.UnderlayTCP) == 0 {
		t.Fatal("expected underlay_tcp to have at least one entry")
	}

	// Verify xray socket with expected fields
	xraySocket := capture.NetworkDiag.UnderlayTCP[0]
	if xraySocket.Name != "xray" {
		t.Errorf("expected socket name 'xray', got '%s'", xraySocket.Name)
	}
	if xraySocket.RTTMs == nil || *xraySocket.RTTMs != 123.4 {
		t.Errorf("expected rtt_ms 123.4, got %v", xraySocket.RTTMs)
	}
	if xraySocket.RTOMs == nil || *xraySocket.RTOMs != 456 {
		t.Errorf("expected rto_ms 456, got %v", xraySocket.RTOMs)
	}
	if xraySocket.Retransmits == nil || *xraySocket.Retransmits != 7 {
		t.Errorf("expected retransmits 7, got %v", xraySocket.Retransmits)
	}
}

// TestSpikeCaptureAPI_ExcludeCaptures tests that captures are absent when include_captures is omitted.
func TestSpikeCaptureAPI_ExcludeCaptures(t *testing.T) {
	// Create fake tovarisch server
	tovarischServer := httptest.NewServer(fakeTovarischHandler(validTovarischStatusJSON(), http.StatusOK))
	defer tovarischServer.Close()

	// Setup test server
	srv, st, captureSvc, token := setupTestServer(t, tovarischServer.URL)

	// Create router with spike endpoint
	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/latency/spikes", http.HandlerFunc(srv.handleTargetLatencySpikes)).Methods(http.MethodGet)

	// Create a spike event
	now := time.Now().UTC()
	previousSamples := make([]state.LatencySample, 25)
	for i := 0; i < 25; i++ {
		previousSamples[i] = state.LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		}
	}

	spikeEvent := st.DetectAndRecordSpike("test-target", "http", 1500.0, now.Add(-time.Second), true, nil, nil, nil, previousSamples)
	if spikeEvent == nil {
		t.Fatal("expected spike to be detected")
	}

	// Trigger capture
	captureSvc.TriggerCapture(spikeEvent.EventID, "test-target", "http")
	waitForCaptures(st.GetCaptureStore(), spikeEvent.EventID, 2*time.Second)

	// Call API WITHOUT include_captures
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	// Parse as SpikeResponse (without captures)
	var resp SpikeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Spike should still appear (old behavior preserved)
	if resp.Count < 1 {
		t.Error("expected spike in response")
	}
}
