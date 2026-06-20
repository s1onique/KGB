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

// =============================================================================
// Spike API Cooldown Retention Stats Tests
//
// Tests for retention statistics and protected capture counting.
// =============================================================================

// TestSpikeAPI_RetentionStats_ProtectedCaptureCount tests that protected_capture_count
// accurately reflects the number of protected (successful) captures.
func TestSpikeAPI_RetentionStats_ProtectedCaptureCount(t *testing.T) {
	// Create fake tovarisch server
	tovarischServer := httptest.NewServer(fakeTovarischHandler(validTovarischStatusJSON(), http.StatusOK))
	defer tovarischServer.Close()

	// Setup test server
	srv, st, _, _ := setupTestServer(t, tovarischServer.URL)

	// Create router with spike endpoint
	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/latency/spikes", http.HandlerFunc(srv.handleTargetLatencySpikes)).Methods(http.MethodGet)

	now := time.Now().UTC()
	previousSamples := make([]state.LatencySample, 25)
	for i := 0; i < 25; i++ {
		previousSamples[i] = state.LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		}
	}

	// First spike - successful capture
	spike1 := st.DetectAndRecordSpike("test-target", "http", 1500.0, now.Add(-time.Second), true, nil, nil, nil, previousSamples)
	if spike1 == nil {
		t.Fatal("expected first spike to be detected")
	}

	// Record successful capture
	st.GetCaptureStore().AddCapture(spike1.EventID, state.DiagCapture{
		Source:           "tovarisch-peer",
		CaptureStartedAt: now.Add(-time.Second),
		Status:           state.DiagCaptureStatusOK,
		CaptureStatus:    state.CaptureStatusCaptured,
	})

	// Second spike - suppressed (cooldown active)
	spike2 := st.DetectAndRecordSpike("test-target", "http", 2000.0, now, true, nil, nil, nil, previousSamples)
	if spike2 == nil {
		t.Fatal("expected second spike to be detected")
	}

	// Simulate suppressed capture
	cooldownDecision := st.GetCaptureStore().EvaluateCooldown(now, "tovarisch-peer", 90)
	recordSuppressedCooldownCapture(st.GetCaptureStore(), spike2.EventID, "tovarisch-peer", cooldownDecision)

	// Call API
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&include_captures=true", nil)
	token, _ := srv.sessions.GenerateToken("admin")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Parse response
	var resp SpikeResponseWithCaptures
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// CRITICAL: protected_capture_count must be at least 1 (spike1's capture)
	if resp.Retention.ProtectedCaptureCount < 1 {
		t.Errorf("Expected at least 1 protected capture, got %d", resp.Retention.ProtectedCaptureCount)
	}

	// Spike1 should be protected
	isProtected1, _ := st.GetCaptureStore().GetProtectionInfo(spike1.EventID)
	if !isProtected1 {
		t.Error("First spike (successful capture) should be protected")
	}

	// Spike2 should NOT be protected (suppressed)
	isProtected2, _ := st.GetCaptureStore().GetProtectionInfo(spike2.EventID)
	if isProtected2 {
		t.Error("Second spike (suppressed capture) should NOT be protected")
	}

	t.Log("PASS: Retention stats correctly count protected captures")
}
