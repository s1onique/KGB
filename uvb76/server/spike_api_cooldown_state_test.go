package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/state"
)

// =============================================================================
// Spike API Cooldown State Tests
//
// Tests for cooldown state evaluation: empty store, first spike, expired cooldown.
// These are pure state tests without full API round-trips.
// =============================================================================

// TestSpikeAPI_FirstSpike_NotSuppressed tests that the first spike after
// startup/start is never suppressed (no cooldown anchor exists).
func TestSpikeAPI_FirstSpike_NotSuppressed(t *testing.T) {
	// Create fake tovarisch server
	tovarischServer := httptest.NewServer(fakeTovarischHandler(validTovarischStatusJSON(), http.StatusOK))
	defer tovarischServer.Close()

	// Setup test server
	_, st, _, _ := setupTestServer(t, tovarischServer.URL)

	now := time.Now().UTC()
	previousSamples := make([]state.LatencySample, 25)
	for i := 0; i < 25; i++ {
		previousSamples[i] = state.LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		}
	}

	// First spike - no prior captures exist
	spike1 := st.DetectAndRecordSpike("test-target", "http", 1500.0, now, true, nil, nil, nil, previousSamples)
	if spike1 == nil {
		t.Fatal("expected first spike to be detected")
	}

	// Check cooldown evaluation - should NOT be in cooldown
	decision := st.GetCaptureStore().EvaluateCooldown(now, "tovarisch-peer", 90)
	if decision.IsInCooldown {
		t.Error("First spike must NOT be suppressed - no cooldown anchor exists")
	}

	t.Log("PASS: First spike is not suppressed (no cooldown anchor)")
}

// TestSpikeAPI_CooldownExpired_NotSuppressed tests that after cooldown expires,
// spikes are not suppressed even if prior captures exist.
func TestSpikeAPI_CooldownExpired_NotSuppressed(t *testing.T) {
	// Create fake tovarisch server
	tovarischServer := httptest.NewServer(fakeTovarischHandler(validTovarischStatusJSON(), http.StatusOK))
	defer tovarischServer.Close()

	// Setup test server
	_, st, _, _ := setupTestServer(t, tovarischServer.URL)

	cs := st.GetCaptureStore()

	// CRITICAL: Establish cooldown anchor by directly setting lastCapture
	// (AddCapture updates lastCapture to current time, not CaptureStartedAt)
	anchorTime := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	cs.AddCapture("anchor-spike", state.DiagCapture{
		Source:           "tovarisch-peer",
		CaptureStartedAt: anchorTime,
		Status:           state.DiagCaptureStatusOK,
		CaptureStatus:    state.CaptureStatusCaptured,
	})
	// Override lastCapture to use anchorTime (not current time)
	cs.SetLastCapture("tovarisch-peer", anchorTime)

	// Evaluate cooldown at t=100s (after 90s cooldown has expired from anchorTime)
	t100 := anchorTime.Add(100 * time.Second)
	decision := cs.EvaluateCooldown(t100, "tovarisch-peer", 90)

	// CRITICAL: After 100s with 90s cooldown, spike must NOT be suppressed
	if decision.IsInCooldown {
		t.Error("After 100s with 90s cooldown, spike must NOT be suppressed (cooldown expired)")
	}

	t.Log("PASS: Spike after cooldown expiry is not suppressed (anchor existed but expired)")
}

// TestSpikeAPI_CooldownActive_SkipsSpike tests that spikes within cooldown are skipped.
func TestSpikeAPI_CooldownActive_SkipsSpike(t *testing.T) {
	// Create fake tovarisch server
	tovarischServer := httptest.NewServer(fakeTovarischHandler(validTovarischStatusJSON(), http.StatusOK))
	defer tovarischServer.Close()

	// Setup test server
	_, st, _, _ := setupTestServer(t, tovarischServer.URL)

	now := time.Now().UTC()
	previousSamples := make([]state.LatencySample, 25)
	for i := 0; i < 25; i++ {
		previousSamples[i] = state.LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		}
	}

	// First spike at t=0
	spike1 := st.DetectAndRecordSpike("test-target", "http", 1500.0, now, true, nil, nil, nil, previousSamples)
	if spike1 == nil {
		t.Fatal("expected first spike to be detected")
	}

	// Set lastCapture to establish cooldown
	cs := st.GetCaptureStore()
	cs.AddCapture(spike1.EventID, state.DiagCapture{
		Source:           "tovarisch-peer",
		CaptureStartedAt: now,
		Status:           state.DiagCaptureStatusOK,
		CaptureStatus:    state.CaptureStatusCaptured,
	})

	// Second spike at t=30 (within 90s cooldown)
	t30 := now.Add(30 * time.Second)
	decision := cs.EvaluateCooldown(t30, "tovarisch-peer", 90)
	if !decision.IsInCooldown {
		t.Error("At t=30s with 90s cooldown, spike SHOULD be suppressed")
	}

	t.Log("PASS: Spike within cooldown is correctly suppressed")
}
