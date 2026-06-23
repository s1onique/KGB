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

// TestAnchorPinning_ResponseTimeVerification tests that the response does not
// violate the invariant: no normal skipped_cooldown row lacks anchor provenance.
func TestAnchorPinning_ResponseTimeVerification(t *testing.T) {
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

	// Create anchor
	anchorTime := now.Add(-100 * time.Second)
	anchorSpike := RecordSpikeForTest(st, "test-target", "http", 2500.0, anchorTime, true, nil, nil, nil, previousSamples)
	if anchorSpike == nil {
		t.Fatal("expected anchor spike")
	}
	st.GetCaptureStore().AddCapture(anchorSpike.EventID, state.DiagCapture{
		Source:           "tovarisch-peer",
		CaptureStartedAt: anchorTime,
		Status:           state.DiagCaptureStatusOK,
		CaptureStatus:    state.CaptureStatusCaptured,
	})

	// Create suppressed spikes
	for i := 0; i < 30; i++ {
		spikeTime := now.Add(-time.Duration(90-i*2) * time.Second)
		spike := RecordSpikeForTest(st, "test-target", "http", 1500.0, spikeTime, true, nil, nil, nil, previousSamples)
		if spike == nil {
			continue
		}
		cooldownDecision := st.GetCaptureStore().EvaluateCooldown(spikeTime, "tovarisch-peer", 90)
		if cooldownDecision.IsInCooldown {
			recordSuppressedCooldownCapture(st.GetCaptureStore(), spike.EventID, "tovarisch-peer", cooldownDecision)
		}
	}

	// Call API
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&include_captures=true&limit=20", nil)
	token, _ := srv.sessions.GenerateToken("admin")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp SpikeResponseWithCaptures
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// INVARIANT VERIFICATION: For every skipped_cooldown row:
	// either anchor_visible=true OR anchor_event_summary != nil OR suppression_degraded=true
	violationCount := 0
	for _, spike := range resp.Spikes {
		for _, capture := range spike.Captures {
			if capture.SuppressedByCooldown && capture.CooldownInfo != nil {
				hasAnchor := capture.CooldownInfo.AnchorVisible
				hasSummary := capture.CooldownInfo.AnchorEventSummary != nil
				isDegraded := capture.CooldownInfo.SuppressionDegraded

				if !hasAnchor && !hasSummary && !isDegraded {
					violationCount++
					t.Errorf("INVARIANT VIOLATION: Suppressed row %s has neither anchor nor summary nor degraded flag",
						spike.EventID)
				}
			}
		}
	}

	if violationCount > 0 {
		t.Errorf("FAIL: %d suppressed rows violate anchor provenance invariant", violationCount)
	} else {
		t.Log("PASS: All suppressed rows satisfy anchor provenance invariant")
	}
}
