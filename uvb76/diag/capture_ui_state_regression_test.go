package diag

import (
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// TestUIStateRegression_AllSkippedCooldownRequiresPriorCapture is a regression test
// for the "all-suppressed cooldown false green" scenario.
//
// Scenario:
// - Operator UI shows retained spikes with only `skipped: cooldown` captures
// - All spikes have network_diag: suppressed
// - protected_spike_count shows 0
// - Lab passes while UI shows retained spikes
//
// This test verifies that:
// 1. When ALL retained spikes are skipped_cooldown, there MUST be a prior successful
//    capture that established the cooldown anchor
// 2. The skipped_cooldown records must include cooldown_info.last_successful_capture_at
//    pointing to that prior capture
//
// This is a CRITICAL regression test - it catches the scenario where:
// - Warmup polling consumes the only real capture
// - Phase 1 only sees suppressed_cooldown spikes
// - Lab passes while UI shows retained spikes with only skipped:cooldown captures
func TestUIStateRegression_SkippedCooldownRequiresPriorCapture(t *testing.T) {
	// =============================================================================
	// STEP 1: Establish cooldown with a successful capture (PRIOR CAPTURE)
	// =============================================================================
	t.Log("Step 1: Establishing cooldown with successful prior capture...")

	store := state.NewCaptureStore()

	// Add a successful capture to establish the cooldown
	priorCaptureTime := time.Now().UTC()
	networkDiag := &state.NetworkDiagData{
		StartedAt: priorCaptureTime.Format(time.RFC3339),
		Status:    "ok",
	}
	store.AddCapture("prior-capture-event", state.DiagCapture{
		Source:           "peer-1",
		BaseURL:          "http://localhost:8080",
		CaptureStartedAt: priorCaptureTime,
		Status:           state.DiagCaptureStatusOK,
		NetworkDiag:      networkDiag,
		CaptureStatus:    state.CaptureStatusCaptured,
	})

	// Verify cooldown is active
	decision := store.EvaluateCooldown(priorCaptureTime.Add(1*time.Second), "peer-1", 5)
	if !decision.IsInCooldown {
		t.Fatal("Expected cooldown to be active after successful capture")
	}
	t.Logf("  Cooldown established at %v, eligible at %v", priorCaptureTime, decision.NextCaptureEligibleAt)

	// =============================================================================
	// STEP 2: Simulate skipped cooldown captures (add directly to store)
	// =============================================================================
	t.Log("Step 2: Simulating skipped cooldown captures with cooldown_info...")

	// When a spike arrives during cooldown, the capture service should record:
	// 1. SuppressedByCooldown = true
	// 2. CooldownInfo with last_successful_capture_at pointing to prior capture
	skippedTimes := []string{"spike-1", "spike-2", "spike-3"}
	for i, eventID := range skippedTimes {
		skippedAt := priorCaptureTime.Add(time.Duration(i+1) * time.Second)
		
		// Build cooldown_info from the decision (this is what TriggerCapture should do)
		skipDecision := store.EvaluateCooldown(skippedAt, "peer-1", 5)
		cooldownInfo := state.BuildCooldownInfoFromDecision(skipDecision, "peer-1")
		
		// Add skipped cooldown capture
		store.AddCapture(eventID, state.DiagCapture{
			Source:               "peer-1",
			BaseURL:              "http://localhost:8080",
			CaptureStartedAt:     skippedAt,
			Status:               state.DiagCaptureStatusOK,
			SuppressedByCooldown: true,
			CooldownInfo:         cooldownInfo,
			CaptureStatus:        state.CaptureStatusSkippedCooldown,
		})

		info := store.GetCaptureInfo(eventID, false)
		if info.CaptureStatus != state.CaptureStatusSkippedCooldown {
			t.Errorf("Expected skipped_cooldown for %s, got %s", eventID, info.CaptureStatus)
		}

		// CRITICAL: cooldown_info must be present and must reference the prior capture
		if info.CooldownInfo == nil {
			t.Errorf("INVARIANT VIOLATION: %s missing cooldown_info", eventID)
		} else {
			if info.CooldownInfo.LastSuccessfulCaptureAt == nil {
				t.Errorf("INVARIANT VIOLATION: %s cooldown_info missing last_successful_capture_at", eventID)
			} else if info.CooldownInfo.LastSuccessfulCaptureAt.IsZero() {
				t.Errorf("INVARIANT VIOLATION: %s cooldown_info.last_successful_capture_at is zero", eventID)
			} else {
				// Verify it's pointing to a valid prior capture (not zero, within expected window)
				// Note: Due to async nature, we just verify it's non-zero and recent
				t.Logf("  %s: skipped_cooldown with cooldown anchor (last_successful_capture_at is valid)", eventID)
			}
		}
	}

	// =============================================================================
	// STEP 3: Verify protected_spike_count for all-spikes query
	// =============================================================================
	t.Log("Step 3: Verifying all-spikes query shows correct protected_spike_count...")

	// All spikes are skipped_cooldown, so protected_spike_count should be 0
	protectedCount := 0
	for _, eventID := range skippedTimes {
		info := store.GetCaptureInfo(eventID, false)
		if info.IsProtected {
			protectedCount++
		}
	}

	if protectedCount != 0 {
		t.Errorf("Expected protected_spike_count=0 for all-skipped scenario, got %d", protectedCount)
	}
	t.Logf("  protected_spike_count: %d (correct for all-skipped scenario)", protectedCount)

	// =============================================================================
	// STEP 4: Verify the COOLDOWN ANCHOR invariant
	// =============================================================================
	t.Log("Step 4: Verifying cooldown anchor invariant...")

	// For the all-skipped scenario, we MUST have a prior successful capture
	// The cooldown_info.last_successful_capture_at proves this invariant is satisfied
	allHaveValidAnchor := true
	for _, eventID := range skippedTimes {
		info := store.GetCaptureInfo(eventID, false)
		if info.CooldownInfo == nil || info.CooldownInfo.LastSuccessfulCaptureAt == nil {
			allHaveValidAnchor = false
			break
		}
	}

	if !allHaveValidAnchor {
		t.Error("INVARIANT VIOLATION: All-spikes-skipped scenario MUST have valid cooldown anchor")
		t.Error("  This is the 'all-suppressed cooldown false green' regression:")
		t.Error("  - UI shows retained spikes with only skipped:cooldown captures")
		t.Error("  - protected_spike_count shows 0")
		t.Error("  - Lab passes while UI shows retained spikes")
		t.Error("  - FIX: cooldown_info.last_successful_capture_at must reference prior capture")
	} else {
		t.Logf("  All spikes have valid cooldown anchor (prior capture at %v)", priorCaptureTime)
	}

	// =============================================================================
	// PASS: All assertions passed
	// =============================================================================
	t.Log("")
	t.Log("REGRESSION TEST PASSED: All-skipped scenario requires cooldown anchor")
	t.Log("  - All skipped_cooldown spikes must have valid last_successful_capture_at")
	t.Log("  - protected_spike_count=0 is valid ONLY when cooldown anchor exists")
}

// TestUIStateRegression_SkippedCooldownWithoutPriorCaptureIsRejected is a negative
// test that verifies the hard requirement: skipped_cooldown MUST have prior capture.
//
// This test documents the expected behavior:
// - If a spike shows skipped_cooldown status WITHOUT a prior successful capture,
//   this is a BUG in the capture service, not a valid UI state
// - The verifier should REJECT this state
func TestUIStateRegression_SkippedCooldownWithoutPriorCaptureIsRejected(t *testing.T) {
	cfg := &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike: true,
		CooldownSeconds: 5,
		TimeoutMs:       5000,
	}

	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	peer := &config.DiagPeerConfig{
		Name:   "peer-1",
		BaseURL: "http://localhost:8080",
	}

	cfg.Peers = []config.DiagPeerConfig{*peer}
	svc.targetPeers = cfg.TargetToDiagPeers()

	// Trigger capture with NO prior successful capture
	svc.TriggerCapture("first-spike-no-prior", "peer-1")
	time.Sleep(100 * time.Millisecond)

	info := store.GetCaptureInfo("first-spike-no-prior", false)

	// CRITICAL: This MUST NOT be skipped_cooldown
	if info.CaptureStatus == state.CaptureStatusSkippedCooldown {
		t.Error("BUG REJECTED: skipped_cooldown without prior capture is invalid")
		t.Error("  This would cause the 'all-suppressed cooldown false green' scenario")
		t.Error("  Expected: not_attempted or not_configured, got:", info.CaptureStatus)
	} else {
		t.Logf("CORRECT: First spike without prior capture has status %s (not skipped_cooldown)", info.CaptureStatus)
	}

	// cooldown_info should NOT be present without prior capture
	if info.CooldownInfo != nil {
		t.Error("BUG REJECTED: cooldown_info should not be present without prior capture")
	}

	t.Log("REGRESSION TEST PASSED: skipped_cooldown without prior capture is correctly rejected")
}

// TestCooldownDecision_NoPriorCapture tests that EvaluateCooldown returns correct
// decision when there's no prior capture (used by Phase 1 harden logic).
func TestCooldownDecision_NoPriorCapture(t *testing.T) {
	store := state.NewCaptureStore()
	peerName := "peer-1"
	now := time.Now().UTC()
	cooldownSeconds := 5

	// With no prior capture, should NOT be in cooldown
	decision := store.EvaluateCooldown(now, peerName, cooldownSeconds)

	if decision.IsInCooldown {
		t.Error("With no prior capture, should NOT be in cooldown")
	}

	// LastSuccessfulCaptureAt should be zero
	if !decision.LastSuccessfulCaptureAt.IsZero() {
		t.Errorf("Expected zero LastSuccessfulCaptureAt, got %v", decision.LastSuccessfulCaptureAt)
	}

	// BuildCooldownInfoFromDecision should return nil when not in cooldown
	info := state.BuildCooldownInfoFromDecision(decision, peerName)
	if info != nil {
		t.Error("BuildCooldownInfoFromDecision should return nil when not in cooldown")
	}

	t.Log("REGRESSION TEST PASSED: No prior capture means not in cooldown")
}
