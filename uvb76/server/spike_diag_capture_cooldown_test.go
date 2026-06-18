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

// TestSpikeCaptureAPI_CooldownSuppression tests that captures are suppressed within cooldown period.
func TestSpikeCaptureAPI_CooldownSuppression(t *testing.T) {
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

	// Create first spike event
	now := time.Now().UTC()
	previousSamples := make([]state.LatencySample, 25)
	for i := 0; i < 25; i++ {
		previousSamples[i] = state.LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		}
	}

	spike1 := st.DetectAndRecordSpike("test-target", "http", 1500.0, now.Add(-time.Second), true, nil, nil, nil, previousSamples)
	if spike1 == nil {
		t.Fatal("expected first spike to be detected")
	}

	// Trigger first capture and wait
	captureSvc.TriggerCapture(spike1.EventID, "test-target")
	waitForCaptures(st.GetCaptureStore(), spike1.EventID, 2*time.Second)

	// Create second spike immediately (within cooldown)
	spike2 := st.DetectAndRecordSpike("test-target", "http", 2000.0, now, true, nil, nil, nil, previousSamples)
	if spike2 == nil {
		t.Fatal("expected second spike to be detected")
	}

	// Trigger second capture
	captureSvc.TriggerCapture(spike2.EventID, "test-target")
	_ = waitForCaptures(st.GetCaptureStore(), spike2.EventID, 2*time.Second)

	// Call API
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&include_captures=true", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	// Parse response
	var resp SpikeResponseWithCaptures
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Count < 2 {
		t.Errorf("expected at least 2 spikes, got %d", resp.Count)
	}

	// Find second spike
	var foundSpike2 *state.SpikeEventWithCaptures
	for i := range resp.Spikes {
		if resp.Spikes[i].EventID == spike2.EventID {
			foundSpike2 = &resp.Spikes[i]
			break
		}
	}
	if foundSpike2 == nil {
		t.Fatal("second spike not found in API response")
	}

	// Second spike should have suppressed capture marker
	if len(foundSpike2.Captures) == 0 {
		t.Fatal("expected capture for second spike")
	}
	capture := foundSpike2.Captures[0]
	if !capture.SuppressedByCooldown {
		t.Error("expected second capture to be marked as suppressed by cooldown")
	}
	if capture.NetworkDiag != nil {
		t.Error("suppressed capture should not have network_diag")
	}

	// Verify first spike capture is still intact (not overwritten)
	captures1 := st.GetCaptureStore().GetCaptures(spike1.EventID)
	if len(captures1) == 0 {
		t.Error("first spike should still have capture")
	}
	if captures1[0].SuppressedByCooldown {
		t.Error("first spike capture should not be suppressed")
	}
}
