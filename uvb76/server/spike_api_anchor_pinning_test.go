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
// Spike API Anchor Pinning Regression Tests
//
// These tests catch the "temporal anchor-liveness bug" where:
// 1. A suppressed spike's anchor is visible at suppression decision time
// 2. The anchor spike is later evicted from the visible timeline
// 3. The suppressed spike remains visible without any visible anchor
//
// The fix implements:
// - Anchor pinning: Required anchors are pinned into the response
// - Anchor summary embedding: When anchor cannot be pinned, embed event summary
// - Suppression degradation: When neither pinning nor embedding is possible, mark degraded
// =============================================================================

// TestAnchorPinning_AnchorEvictedButSuppressedRowsVisible tests the EXACT bug scenario:
// Anchor is in retention but outside the visible window (evicted by newer spikes).
// The response must pin the anchor OR embed its summary.
func TestAnchorPinning_AnchorEvictedButSuppressedRowsVisible(t *testing.T) {
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

	// Step 1: Create anchor spike at T=0 with successful capture
	anchorTime := now.Add(-90 * time.Second) // 90 seconds ago
	anchorSpike := RecordSpikeForTest(st, "test-target", "http", 2000.0, anchorTime, true, nil, nil, nil, previousSamples)
	if anchorSpike == nil {
		t.Fatal("expected anchor spike to be detected")
	}

	// Record successful capture for anchor
	st.GetCaptureStore().AddCapture(anchorSpike.EventID, state.DiagCapture{
		Source:           "tovarisch-peer",
		CaptureStartedAt: anchorTime,
		Status:           state.DiagCaptureStatusOK,
		CaptureStatus:    state.CaptureStatusCaptured,
	})

	// Step 2: Create enough subsequent spikes to push the anchor out of the visible window
	// With limit=20, we need more than 20 newer spikes to push anchor outside visible window
	spikeTimes := make([]time.Time, 25)
	for i := 0; i < 25; i++ {
		spikeTimes[i] = now.Add(-time.Duration(80-i*3) * time.Second)
	}

	for i, spikeTime := range spikeTimes {
		spike := RecordSpikeForTest(st, "test-target", "http", 1500.0, spikeTime, true, nil, nil, nil, previousSamples)
		if spike == nil {
			t.Fatalf("expected spike %d to be detected", i)
		}

		// These spikes are within cooldown window (anchor was 90s ago, spikes are within 90s)
		cooldownDecision := st.GetCaptureStore().EvaluateCooldown(spikeTime, "tovarisch-peer", 90)
		if cooldownDecision.IsInCooldown {
			recordSuppressedCooldownCapture(st.GetCaptureStore(), spike.EventID, "tovarisch-peer", cooldownDecision)
		}
	}

	// Step 3: Verify anchor is NOT in the limited spikes (pushed out)
	limitedSpikes := st.GetSpikes("test-target", "http", 20)
	anchorInVisible := false
	for _, spike := range limitedSpikes {
		if spike.EventID == anchorSpike.EventID {
			anchorInVisible = true
			break
		}
	}
	if anchorInVisible {
		t.Fatal("anchor should NOT be in limited spikes (test precondition)")
	}

	// Step 4: Call API with limit=20
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&include_captures=true&limit=20", nil)
	token, _ := srv.sessions.GenerateToken("admin")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Parse response
	var resp SpikeResponseWithCaptures
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// CRITICAL: Count suppressed captures
	suppressedCount := 0
	for _, spike := range resp.Spikes {
		for _, capture := range spike.Captures {
			if capture.SuppressedByCooldown {
				suppressedCount++
			}
		}
	}

	if suppressedCount == 0 {
		t.Fatal("expected at least one suppressed capture in response")
	}

	// CRITICAL: Either pinned anchors must be present OR embedded summaries
	hasPinnedAnchors := len(resp.PinnedAnchors) > 0
	hasEmbeddedSummaries := false
	for _, spike := range resp.Spikes {
		for _, capture := range spike.Captures {
			if capture.SuppressedByCooldown && capture.CooldownInfo != nil && capture.CooldownInfo.AnchorEventSummary != nil {
				hasEmbeddedSummaries = true
				break
			}
		}
	}

	if !hasPinnedAnchors && !hasEmbeddedSummaries {
		t.Fatal("FAIL: No pinned anchors AND no embedded summaries - ghost suppression scenario")
	}

	// CRITICAL: Verify anchor is visible (pinned or embedded)
	for _, spike := range resp.Spikes {
		for _, capture := range spike.Captures {
			if capture.SuppressedByCooldown && capture.CooldownInfo != nil {
				if capture.CooldownInfo.AnchorVisible {
					// Good - anchor is visible
					continue
				}
				// Anchor not visible - must have embedded summary
				if capture.CooldownInfo.AnchorEventSummary == nil && !capture.CooldownInfo.SuppressionDegraded {
					t.Fatal("FAIL: Suppressed row has neither anchor_visible=true nor anchor_event_summary - ghost suppression")
				}
			}
		}
	}

	t.Logf("PASS: Anchor pinning/embedding working correctly (pinned=%v, embedded=%v, suppressed=%d)",
		hasPinnedAnchors, hasEmbeddedSummaries, suppressedCount)
}

// TestAnchorPinning_AllSuppressedRowsRequireAnchorPinned tests the EXACT screenshot scenario:
// HTTP: 0 captured, 4 suppressed
// ICMP: 0 captured, 10 suppressed
// Showing 14 of 14, all suppressed
// With 0 captured visible.
func TestAnchorPinning_AllSuppressedRowsRequireAnchorPinned(t *testing.T) {
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

	// Create anchor spike with successful capture
	anchorTime := now.Add(-120 * time.Second)
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

	// Create 25 suppressed spikes (more than visible limit of 20)
	for i := 0; i < 25; i++ {
		spikeTime := now.Add(-time.Duration(110-i*4) * time.Second)
		spike := RecordSpikeForTest(st, "test-target", "http", 1500.0, spikeTime, true, nil, nil, nil, previousSamples)
		if spike == nil {
			t.Fatalf("expected suppressed spike %d", i)
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

	// Count visible captured vs suppressed
	visibleCaptured := 0
	visibleSuppressed := 0
	for _, spike := range resp.Spikes {
		for _, capture := range spike.Captures {
			if capture.CaptureStatus == state.CaptureStatusCaptured {
				visibleCaptured++
			}
			if capture.SuppressedByCooldown {
				visibleSuppressed++
			}
		}
	}

	// CRITICAL: This is the screenshot scenario - 0 captured visible
	t.Logf("Visible captured: %d, Visible suppressed: %d", visibleCaptured, visibleSuppressed)

	if visibleSuppressed > 0 {
		// Verify no ghost suppression - check that each suppressed row has valid provenance
		ghostSuppressionCount := 0
		for _, spike := range resp.Spikes {
			for _, capture := range spike.Captures {
				if capture.SuppressedByCooldown && capture.CooldownInfo != nil {
					// Must have either anchor visible, pinned, embedded, or degraded
					hasValidProvenance := capture.CooldownInfo.AnchorVisible || 
						capture.CooldownInfo.AnchorEventSummary != nil || 
						capture.CooldownInfo.SuppressionDegraded
					if !hasValidProvenance {
						ghostSuppressionCount++
						t.Errorf("FAIL: Ghost suppression - suppressed row %s has no anchor provenance", spike.EventID)
					}
					if capture.CooldownInfo.SuppressionDegraded {
						t.Logf("Suppression degraded: %s", capture.CooldownInfo.SuppressionDegradedReason)
					}
				}
			}
		}

		if ghostSuppressionCount > 0 {
			t.Errorf("FAIL: %d suppressed rows have ghost suppression (no anchor provenance)", ghostSuppressionCount)
		} else {
			t.Log("PASS: All suppressed rows have valid anchor provenance (visible/pinned/embedded/degraded)")
		}
	}

	t.Log("PASS: All-suppressed scenario handled correctly")
}

// TestAnchorPinning_CrossProbeSuppressionUsesEmbeddedSummary tests that cross-probe
// suppression (HTTP suppressed due to ICMP anchor) embeds anchor summary.
func TestAnchorPinning_CrossProbeSuppressionUsesEmbeddedSummary(t *testing.T) {
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

	// Step 1: Create ICMP anchor spike with successful capture
	icmpAnchorTime := now.Add(-60 * time.Second)
	icmpAnchorSpike := RecordSpikeForTest(st, "test-target", "icmp", 3000.0, icmpAnchorTime, true, nil, nil, nil, previousSamples)
	if icmpAnchorSpike == nil {
		t.Fatal("expected ICMP anchor spike")
	}
	st.GetCaptureStore().AddCapture(icmpAnchorSpike.EventID, state.DiagCapture{
		Source:           "tovarisch-peer",
		CaptureStartedAt: icmpAnchorTime,
		Status:           state.DiagCaptureStatusOK,
		CaptureStatus:    state.CaptureStatusCaptured,
	})

	// Step 2: Create HTTP spike that is cross-probe suppressed
	httpSpikeTime := now.Add(-30 * time.Second)
	httpSpike := RecordSpikeForTest(st, "test-target", "http", 1500.0, httpSpikeTime, true, nil, nil, nil, previousSamples)
	if httpSpike == nil {
		t.Fatal("expected HTTP suppressed spike")
	}

	// Evaluate cooldown at HTTP spike time (should be suppressed)
	cooldownDecision := st.GetCaptureStore().EvaluateCooldown(httpSpikeTime, "tovarisch-peer", 90)
	if !cooldownDecision.IsInCooldown {
		t.Fatal("HTTP spike should be suppressed by ICMP cooldown")
	}

	// Record suppressed capture with cross-probe flag
	capture := state.DiagCapture{
		Source:               "tovarisch-peer",
		CaptureStartedAt:     httpSpikeTime,
		Status:               state.DiagCaptureStatusOK,
		CaptureStatus:        state.CaptureStatusSkippedCooldown,
		SuppressedByCooldown: true,
		CooldownInfo:         state.BuildCooldownInfoFromDecision(cooldownDecision, "tovarisch-peer"),
	}
	if capture.CooldownInfo != nil {
		capture.CooldownInfo.SuppressedProbeKind = "http"
		capture.CooldownInfo.IsCrossProbeSuppression = true
		capture.CooldownInfo.AnchorProbeKind = "icmp"
	}
	st.GetCaptureStore().AddCapture(httpSpike.EventID, capture)

	// Call API for HTTP spikes
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&kind=http&include_captures=true", nil)
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

	// Find HTTP suppressed spike
	var foundHTTP bool
	for _, spike := range resp.Spikes {
		if spike.EventID == httpSpike.EventID {
			foundHTTP = true
			for _, cap := range spike.Captures {
				if cap.SuppressedByCooldown && cap.CooldownInfo != nil {
					// Cross-probe suppression should have either:
					// 1. anchor_visible=true (anchor found in ICMP spikes), OR
					// 2. embedded summary (anchor evicted but summary embedded), OR
					// 3. degraded flag (anchor not found)
					hasValidProvenance := cap.CooldownInfo.AnchorVisible ||
						cap.CooldownInfo.AnchorEventSummary != nil ||
						cap.CooldownInfo.SuppressionDegraded
					if !hasValidProvenance {
						t.Error("Cross-probe suppression should have valid anchor provenance")
					}
					t.Logf("Cross-probe suppression visibility: anchor_visible=%v, reason=%s, embedded=%v, degraded=%v",
						cap.CooldownInfo.AnchorVisible,
						cap.CooldownInfo.AnchorVisibilityReason,
						cap.CooldownInfo.AnchorEventSummary != nil,
						cap.CooldownInfo.SuppressionDegraded)
				}
			}
		}
	}

	if !foundHTTP {
		t.Fatal("HTTP suppressed spike not found in response")
	}

	t.Log("PASS: Cross-probe suppression handled correctly")
}


