package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/s1onique/KGB/uvb76/auth"
	"github.com/s1onique/KGB/uvb76/state"
)

// TestSpikeCaptureAPI_TovarischError tests that spike appears even when tovarisch returns error.
func TestSpikeCaptureAPI_TovarischError(t *testing.T) {
	// Create fake tovarisch server that returns 500
	tovarischServer := httptest.NewServer(fakeTovarischHandler("", http.StatusInternalServerError))
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
	captureSvc.TriggerCapture(spikeEvent.EventID, "test-target")
	captures := waitForCaptures(st.GetCaptureStore(), spikeEvent.EventID, 2*time.Second)

	// Call API
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&include_captures=true", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	// Spike should appear
	var resp SpikeResponseWithCaptures
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Count < 1 {
		t.Error("expected spike in response")
	}

	// Capture should have error status
	if len(captures) == 0 {
		t.Fatal("expected capture to be stored")
	}
	capture := captures[0]
	if capture.Status != state.DiagCaptureStatusError {
		t.Errorf("expected capture status 'error', got '%s'", capture.Status)
	}
	if capture.Error == nil {
		t.Error("expected error message to be set")
	}
	if capture.Error != nil {
		// Verify error is safe (no newlines)
		if strings.Contains(*capture.Error, "\n") || strings.Contains(*capture.Error, "\r") {
			t.Error("error message should not contain newlines")
		}
		// Error should be truncated if too long
		if len(*capture.Error) > 200 {
			t.Error("error message should be truncated to 200 chars")
		}
	}
}

// TestSpikeCaptureAPI_TovarischInvalidJSON tests that spike appears when tovarisch returns invalid JSON.
func TestSpikeCaptureAPI_TovarischInvalidJSON(t *testing.T) {
	// Create fake tovarisch server that returns invalid JSON
	invalidJSON := `{invalid json that will not parse`
	tovarischServer := httptest.NewServer(fakeTovarischHandler(invalidJSON, http.StatusOK))
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
	captureSvc.TriggerCapture(spikeEvent.EventID, "test-target")
	captures := waitForCaptures(st.GetCaptureStore(), spikeEvent.EventID, 2*time.Second)

	// Call API
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&include_captures=true", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	// Spike should appear
	var resp SpikeResponseWithCaptures
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Count < 1 {
		t.Error("expected spike in response")
	}

	// Capture should have error status
	if len(captures) == 0 {
		t.Fatal("expected capture to be stored")
	}
	capture := captures[0]
	if capture.Status != state.DiagCaptureStatusError {
		t.Errorf("expected capture status 'error', got '%s'", capture.Status)
	}
	if capture.Error == nil {
		t.Error("expected error message for parse failure")
	}
}
