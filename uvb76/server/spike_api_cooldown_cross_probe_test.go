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

// TestSpikeAPI_CrossProbeSuppression_OriginalSpikeInsideWindow tests the key scenario:
// An HTTP spike is suppressed by an ICMP anchor capture (cross-probe suppression).
// The ICMP anchor is NOT in the HTTP spikes list, but it IS inside the query window.
// Expected: anchor_visible=true, reason="retained_visible".
// This tests the fix for FAIL original_spike_missing_inside_window.
func TestSpikeAPI_CrossProbeSuppression_OriginalSpikeInsideWindow(t *testing.T) {
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

	// Step 1: Create ICMP anchor capture (simulating cross-probe scenario)
	// The ICMP spike/capture establishes cooldown for the peer.
	// ICMP threshold is 500ms warning, 2000ms critical
	icmpAnchorTime := now.Add(-30 * time.Second)
	icmpSpike := RecordSpikeForTest(st, "test-target", "icmp", 3000.0, icmpAnchorTime, true, nil, nil, nil, previousSamples)
	if icmpSpike == nil {
		t.Fatal("expected ICMP spike to be detected")
	}

	// Record successful ICMP capture and set anchor metadata with probe kind
	st.GetCaptureStore().AddCapture(icmpSpike.EventID, state.DiagCapture{
		Source:           "tovarisch-peer",
		CaptureStartedAt: icmpAnchorTime,
		Status:           state.DiagCaptureStatusOK,
		CaptureStatus:    state.CaptureStatusCaptured,
	})

	// Set the anchor metadata with probe kind for cross-probe suppression
	st.GetCaptureStore().SetLastCaptureAnchor("tovarisch-peer", state.CaptureCooldownAnchor{
		AnchorProbeKind: "icmp",
		AnchorCreatedAt:  icmpAnchorTime,
	})

	// Step 2: Create HTTP spike inside the query window
	// This HTTP spike will be suppressed by the ICMP anchor (cross-probe).
	httpSpikeTime := now.Add(-15 * time.Second) // 15s after ICMP anchor, still in cooldown
	httpSpike := RecordSpikeForTest(st, "test-target", "http", 2000.0, httpSpikeTime, true, nil, nil, nil, previousSamples)
	if httpSpike == nil {
		t.Fatal("expected HTTP spike to be detected")
	}

	// Step 3: Simulate suppressed HTTP capture with cross-probe metadata
	cooldownDecision := st.GetCaptureStore().EvaluateCooldown(httpSpikeTime, "tovarisch-peer", 90)
	cooldownInfo := state.BuildCooldownInfoFromDecision(cooldownDecision, "tovarisch-peer")
	if cooldownInfo != nil {
		// Set cross-probe suppression metadata (this is what the capture service does)
		cooldownInfo.SuppressedProbeKind = "http" // The suppressed spike is HTTP
		// AnchorProbeKind should already be "icmp" from the decision
		cooldownInfo.IsCrossProbeSuppression = cooldownInfo.AnchorProbeKind != "" &&
			cooldownInfo.SuppressedProbeKind != "" &&
			cooldownInfo.AnchorProbeKind != cooldownInfo.SuppressedProbeKind
	}

	capture := state.DiagCapture{
		Source:               "tovarisch-peer",
		CaptureStartedAt:     httpSpikeTime,
		Status:               state.DiagCaptureStatusOK,
		CaptureStatus:        state.CaptureStatusSkippedCooldown,
		SuppressedByCooldown: true,
		CooldownInfo:         cooldownInfo,
	}
	st.GetCaptureStore().AddCapture(httpSpike.EventID, capture)

	// Step 4: Call API for HTTP spikes (the suppressed probe kind)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&kind=http&include_captures=true", nil)
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

	// Find HTTP spike
	var foundHTTPSpike *state.SpikeEventWithCaptures
	for i := range resp.Spikes {
		if resp.Spikes[i].EventID == httpSpike.EventID {
			foundHTTPSpike = &resp.Spikes[i]
			break
		}
	}
	if foundHTTPSpike == nil {
		t.Fatal("HTTP spike not found in API response")
	}

	if len(foundHTTPSpike.Captures) == 0 {
		t.Fatal("expected capture for HTTP spike")
	}
	httpCapture := foundHTTPSpike.Captures[0]

	// CRITICAL: Verify cross-probe suppression metadata
	if !httpCapture.SuppressedByCooldown {
		t.Error("HTTP spike should be suppressed by cooldown")
	}

	if httpCapture.CooldownInfo == nil {
		t.Fatal("cooldown_info must be present for cross-probe suppressed spike")
	}

	if httpCapture.CooldownInfo.AnchorProbeKind != "icmp" {
		t.Errorf("AnchorProbeKind should be icmp, got %q", httpCapture.CooldownInfo.AnchorProbeKind)
	}

	if httpCapture.CooldownInfo.SuppressedProbeKind != "http" {
		t.Errorf("SuppressedProbeKind should be http, got %q", httpCapture.CooldownInfo.SuppressedProbeKind)
	}

	if !httpCapture.CooldownInfo.IsCrossProbeSuppression {
		t.Error("IsCrossProbeSuppression should be true")
	}

	// CRITICAL: For cross-probe suppression, anchor should be visible
	// The ICMP anchor IS inside the query window, even though it's not in HTTP spikes.
	if !httpCapture.CooldownInfo.AnchorVisible {
		t.Fatal("FAIL: Cross-probe anchor should be visible=true (ICMP anchor exists and is inside window)")
	}

	if httpCapture.CooldownInfo.AnchorVisibilityReason != "retained_visible" {
		t.Fatalf("FAIL: Cross-probe anchor should have reason='retained_visible', got %q",
			httpCapture.CooldownInfo.AnchorVisibilityReason)
	}

	t.Log("PASS: Cross-probe suppression correctly marks anchor as visible (anchor inside window)")
}

// TestSpikeAPI_CrossProbeSuppression_AnchorMissingFromAnchorProbeSet tests the negative case:
// An HTTP spike is suppressed by an ICMP anchor, but the ICMP anchor is NOT in the
// ICMP spike/capture set (simulating the anchor being evicted or never captured).
// Expected: anchor_visible=false, reason="outside_filter_window".
func TestSpikeAPI_CrossProbeSuppression_AnchorMissingFromAnchorProbeSet(t *testing.T) {
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

	// Step 1: Create ICMP anchor capture OUTSIDE the query window
	// This simulates the anchor being evicted from retention.
	// Anchor at T=-30s, cooldown=90s, HTTP spike at T=-15s (still in cooldown)
	// But we DON'T create an ICMP spike, so anchor is not in ICMP spike list.
	icmpAnchorTime := now.Add(-30 * time.Second) // 30 seconds ago
	st.GetCaptureStore().SetLastCaptureAnchor("tovarisch-peer", state.CaptureCooldownAnchor{
		AnchorProbeKind: "icmp",
		AnchorCreatedAt:  icmpAnchorTime,
	})

	// Step 2: Create HTTP spike inside the query window
	httpSpikeTime := now.Add(-15 * time.Second)
	httpSpike := RecordSpikeForTest(st, "test-target", "http", 2000.0, httpSpikeTime, true, nil, nil, nil, previousSamples)
	if httpSpike == nil {
		t.Fatal("expected HTTP spike to be detected")
	}

	// Step 3: Simulate suppressed HTTP capture with cross-probe metadata
	cooldownDecision := st.GetCaptureStore().EvaluateCooldown(httpSpikeTime, "tovarisch-peer", 90)
	cooldownInfo := state.BuildCooldownInfoFromDecision(cooldownDecision, "tovarisch-peer")
	if cooldownInfo != nil {
		cooldownInfo.SuppressedProbeKind = "http"
		cooldownInfo.IsCrossProbeSuppression = cooldownInfo.AnchorProbeKind != "" &&
			cooldownInfo.SuppressedProbeKind != "" &&
			cooldownInfo.AnchorProbeKind != cooldownInfo.SuppressedProbeKind
	}

	capture := state.DiagCapture{
		Source:               "tovarisch-peer",
		CaptureStartedAt:     httpSpikeTime,
		Status:               state.DiagCaptureStatusOK,
		CaptureStatus:        state.CaptureStatusSkippedCooldown,
		SuppressedByCooldown: true,
		CooldownInfo:         cooldownInfo,
	}
	st.GetCaptureStore().AddCapture(httpSpike.EventID, capture)

	// Step 4: Call API for HTTP spikes
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&kind=http&include_captures=true", nil)
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

	// Find HTTP spike
	var foundHTTPSpike *state.SpikeEventWithCaptures
	for i := range resp.Spikes {
		if resp.Spikes[i].EventID == httpSpike.EventID {
			foundHTTPSpike = &resp.Spikes[i]
			break
		}
	}
	if foundHTTPSpike == nil {
		t.Fatal("HTTP spike not found in API response")
	}

	httpCapture := foundHTTPSpike.Captures[0]

	// Verify cooldown info is present
	if httpCapture.CooldownInfo == nil {
		t.Fatal("cooldown_info must be present for cross-probe suppressed spike")
	}

	// CRITICAL: For cross-probe suppression with anchor OUTSIDE window, anchor should be hidden
	if httpCapture.CooldownInfo.AnchorVisible {
		t.Fatal("FAIL: Cross-probe anchor should be visible=false when ICMP anchor is outside window")
	}

	if httpCapture.CooldownInfo.AnchorVisibilityReason == "retained_visible" {
		t.Fatal("FAIL: Cross-probe anchor should NOT have reason='retained_visible' when anchor is outside window")
	}

	t.Log("PASS: Cross-probe suppression correctly marks anchor as hidden when anchor is outside window")
}
