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
// Spike API Cooldown Anchor Payload Invariant Tests
//
// These tests catch the "all-suppressed cooldown false green" scenario in the API:
// The API must never return a response where all visible retained spikes are
// skipped_cooldown with protected_capture_count=0, without exposing valid anchor metadata.
//
// The original screenshot bug showed:
//   - retained spikes > 0
//   - protected_by_captures = 0
//   - all visible rows: skipped: cooldown
//   - network_diag: suppressed
//   - no visible successful capture anchor
//
// The API must expose explicit cooldown anchor metadata in this case.
// =============================================================================

// recordSuppressedCooldownCapture is a test helper to record a suppressed cooldown capture.
func recordSuppressedCooldownCapture(cs *state.CaptureStore, eventID, peerName string, decision state.CaptureCooldownDecision) {
	capture := state.DiagCapture{
		Source:               peerName,
		CaptureStartedAt:     decision.DecisionNowAt,
		Status:               state.DiagCaptureStatusOK,
		CaptureStatus:        state.CaptureStatusSkippedCooldown,
		SuppressedByCooldown: true,
		CooldownInfo:         state.BuildCooldownInfoFromDecision(decision, peerName),
	}
	cs.AddCapture(eventID, capture)
}

// TestSpikeAPI_AllSuppressedFromStart_AnchorRequired tests the key invariant:
// If all visible spikes are skipped_cooldown with no protected captures,
// the API must expose cooldown anchor metadata.
func TestSpikeAPI_AllSuppressedFromStart_AnchorRequired(t *testing.T) {
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

	// First spike - successful capture at t=0
	spike1 := st.DetectAndRecordSpike("test-target", "http", 1500.0, now.Add(-time.Second), true, nil, nil, nil, previousSamples)
	if spike1 == nil {
		t.Fatal("expected first spike to be detected")
	}

	// Record successful capture for spike1
	st.GetCaptureStore().AddCapture(spike1.EventID, state.DiagCapture{
		Source:           "tovarisch-peer",
		CaptureStartedAt: now.Add(-time.Second),
		Status:           state.DiagCaptureStatusOK,
		CaptureStatus:    state.CaptureStatusCaptured,
	})

	// Create second spike immediately (within cooldown)
	spike2 := st.DetectAndRecordSpike("test-target", "http", 2000.0, now, true, nil, nil, nil, previousSamples)
	if spike2 == nil {
		t.Fatal("expected second spike to be detected")
	}

	// Simulate suppressed capture (cooldown active)
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

	// CRITICAL: Verify cooldown_info is present for skipped_cooldown
	if len(foundSpike2.Captures) == 0 {
		t.Fatal("expected capture for second spike")
	}
	capture := foundSpike2.Captures[0]

	if !capture.SuppressedByCooldown {
		t.Error("second spike should be suppressed by cooldown")
	}

	// CRITICAL: cooldown_info must be present
	if capture.CooldownInfo == nil {
		t.Error("skipped_cooldown capture MUST have cooldown_info (anchor metadata required)")
	}

	// CRITICAL: cooldown_info must have LastSuccessfulCaptureAt
	if capture.CooldownInfo.LastSuccessfulCaptureAt == nil {
		t.Error("cooldown_info must have last_successful_capture_at (the anchor timestamp)")
	}

	// CRITICAL: cooldown_info must have Scope
	if capture.CooldownInfo.Scope == "" {
		t.Error("cooldown_info must have cooldown_scope")
	}

	// CRITICAL: cooldown_info must have CaptureKey
	if capture.CooldownInfo.CaptureKey == "" {
		t.Error("cooldown_info must have cooldown_key (peer that started cooldown)")
	}

	t.Log("PASS: skipped_cooldown spike has anchor metadata in API response")
}

// TestSpikeAPI_CooldownInfo_AllRequiredFields tests that cooldown_info
// in the API response contains all required fields.
func TestSpikeAPI_CooldownInfo_AllRequiredFields(t *testing.T) {
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

	// First spike
	spike1 := st.DetectAndRecordSpike("test-target", "http", 1500.0, now.Add(-time.Second), true, nil, nil, nil, previousSamples)
	if spike1 == nil {
		t.Fatal("expected first spike to be detected")
	}

	// Record successful capture to establish cooldown
	st.GetCaptureStore().AddCapture(spike1.EventID, state.DiagCapture{
		Source:           "tovarisch-peer",
		CaptureStartedAt: now.Add(-time.Second),
		Status:           state.DiagCaptureStatusOK,
		CaptureStatus:    state.CaptureStatusCaptured,
	})

	// Second spike (will be suppressed)
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

	if len(foundSpike2.Captures) == 0 {
		t.Fatal("expected capture for second spike")
	}
	capture := foundSpike2.Captures[0]

	if capture.CooldownInfo == nil {
		t.Fatal("cooldown_info must be present for skipped_cooldown")
	}

	// Required fields for anchor visibility:
	// 1. cooldown_scope
	if capture.CooldownInfo.Scope == "" {
		t.Error("cooldown_info.cooldown_scope must not be empty")
	}

	// 2. last_successful_capture_at (the anchor timestamp)
	if capture.CooldownInfo.LastSuccessfulCaptureAt == nil {
		t.Error("cooldown_info.last_successful_capture_at must not be nil")
	}

	// 3. next_capture_eligible_at (when cooldown expires)
	if capture.CooldownInfo.NextCaptureEligibleAt == nil {
		t.Error("cooldown_info.next_capture_eligible_at must not be nil")
	}

	// 4. remaining_cooldown_ms (explicit remaining time)
	if capture.CooldownInfo.RemainingCooldownMs == nil {
		t.Error("cooldown_info.remaining_cooldown_ms must not be nil")
	}

	// 5. cooldown_key (peer that started cooldown)
	if capture.CooldownInfo.CaptureKey == "" {
		t.Error("cooldown_info.cooldown_key must not be empty")
	}

	// 6. decision_now_at (proves when cooldown_info was computed)
	if capture.CooldownInfo.DecisionNowAt == nil {
		t.Error("cooldown_info.decision_now_at must not be nil")
	}

	t.Log("PASS: cooldown_info contains all required fields for anchor visibility")
}

// TestSpikeAPI_AllVisibleRowsSuppressed_RequiresVisibleOrExplicitAnchor tests the
// EXACT screenshot invariant:
//   - all visible rows = skipped_cooldown
//   - protected_capture_count = 0
//   - no successful captured row visible
//
// The API must expose explicit anchor metadata (anchor_visible=false,
// anchor_visibility_reason != "retained_visible") for UI explanation.
func TestSpikeAPI_AllVisibleRowsSuppressed_RequiresVisibleOrExplicitAnchor(t *testing.T) {
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

	// CRITICAL: Create the exact screenshot bug scenario.
	// Timeline: Anchor at T=0, spikes at T=10s, T=30s, T=50s, but anchor spike is evicted.
	// At query time T=60s, all visible spikes are suppressed but anchor is not visible.

	// Step 1: Establish cooldown anchor at T=0 (anchor spike will be "evicted" from visible rows)
	anchorTime := now.Add(-60 * time.Second) // 60 seconds ago
	st.GetCaptureStore().AddCapture("anchor-evicted", state.DiagCapture{
		Source:           "tovarisch-peer",
		CaptureStartedAt: anchorTime,
		Status:           state.DiagCaptureStatusOK,
		CaptureStatus:    state.CaptureStatusCaptured,
	})

	// Step 2: Create skipped cooldown spikes AFTER the anchor (realistic timeline)
	// Spike at T=-50s (10s after anchor), spike at T=-30s, spike at T=-10s
	previousSamples := make([]state.LatencySample, 25)
	for i := 0; i < 25; i++ {
		previousSamples[i] = state.LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		}
	}

	// Create skipped cooldown spikes - each one occurs after anchor (positive elapsed time)
	skippedOffsets := []time.Duration{-50 * time.Second, -30 * time.Second, -10 * time.Second}
	for _, offset := range skippedOffsets {
		spikeTime := now.Add(offset)
		spike := st.DetectAndRecordSpike("test-target", "http", 2000.0, spikeTime, true, nil, nil, nil, previousSamples)
		if spike == nil {
			t.Fatal("expected skipped spike to be detected")
		}

		// Evaluate cooldown at spike time - anchor (60s ago) is still active
		// This is realistic: spike arrives, cooldown is evaluated, capture is suppressed
		cooldownDecision := st.GetCaptureStore().EvaluateCooldown(spikeTime, "tovarisch-peer", 90)
		recordSuppressedCooldownCapture(st.GetCaptureStore(), spike.EventID, "tovarisch-peer", cooldownDecision)
	}

	// Step 3: Call API
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&include_captures=true&limit=20", nil)
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

	// Count visible capture statuses
	hasSuccessfulCapture := false
	allSkippedCooldown := true
	skippedCooldownCaptures := 0

	for _, spike := range resp.Spikes {
		if len(spike.Captures) == 0 {
			continue
		}
		capture := spike.Captures[0]
		if capture.CaptureStatus == state.CaptureStatusCaptured {
			hasSuccessfulCapture = true
			allSkippedCooldown = false
		}
		if capture.CaptureStatus == state.CaptureStatusSkippedCooldown {
			skippedCooldownCaptures++
		}
		if capture.CaptureStatus != state.CaptureStatusSkippedCooldown {
			allSkippedCooldown = false
		}
	}

	// CRITICAL ASSERTIONS: This test CANNOT pass unless the exact bug state is present.

	// Require: no visible successful captured rows
	if hasSuccessfulCapture {
		t.Fatal("FAIL: test precondition violated - successful capture is visible; " +
			"expected all visible rows to be skipped_cooldown")
	}

	// Require: at least one skipped cooldown capture exists
	if skippedCooldownCaptures == 0 {
		t.Fatal("FAIL: test precondition violated - expected at least one skipped_cooldown capture")
	}

	// Require: all visible captures are skipped cooldown
	if !allSkippedCooldown {
		t.Fatal("FAIL: test precondition violated - expected all visible capture rows to be skipped_cooldown")
	}

	// Require: protected_capture_count must be 0 (no visible protected captures)
	if resp.Retention.ProtectedCaptureCount != 0 {
		t.Fatalf("FAIL: expected protected_capture_count=0 for all-visible-suppressed scenario, got %d",
			resp.Retention.ProtectedCaptureCount)
	}

	// CRITICAL: Every skipped cooldown capture MUST have cooldown_info with hidden anchor metadata
	for _, spike := range resp.Spikes {
		if len(spike.Captures) == 0 {
			continue
		}
		capture := spike.Captures[0]

		// Only check skipped cooldown captures
		if capture.CaptureStatus != state.CaptureStatusSkippedCooldown {
			continue
		}

		// cooldown_info must be present
		if capture.CooldownInfo == nil {
			t.Fatal("FAIL: skipped_cooldown capture needs cooldown_info (anchor metadata required)")
		}

		// Hidden anchor needs last_successful_capture_at
		if capture.CooldownInfo.LastSuccessfulCaptureAt == nil {
			t.Fatal("FAIL: hidden anchor needs last_successful_capture_at timestamp")
		}

		// anchor_visible must be false when anchor is not in response
		if capture.CooldownInfo.AnchorVisible {
			t.Fatal("FAIL: hidden anchor must set anchor_visible=false")
		}

		// anchor_visibility_reason must NOT be "retained_visible" (that would be misleading)
		if capture.CooldownInfo.AnchorVisibilityReason == "" {
			t.Fatal("FAIL: hidden anchor needs non-empty anchor_visibility_reason")
		}
		if capture.CooldownInfo.AnchorVisibilityReason == "retained_visible" {
			t.Fatal("FAIL: hidden anchor cannot have reason='retained_visible' (anchor is NOT visible)")
		}
	}

	t.Log("PASS: All-suppressed response correctly marks hidden anchor with anchor_visible=false and non-retained visibility reason")
}
