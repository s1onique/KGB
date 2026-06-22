package diag

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// TestCaptureService_GhostSuppressionPrevention tests that the CaptureService
// with anchor validator wired does not suppress spikes when the anchor is evicted.
func TestCaptureService_GhostSuppressionPrevention(t *testing.T) {
	cfg := &config.DiagnosticsConfig{
		Enabled:          true,
		CaptureOnSpike:  true,
		CooldownSeconds: 90,
		TimeoutMs:       5000,
	}

	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	peer := &config.DiagPeerConfig{
		Name:   "tovarisch-peer",
		BaseURL: "http://localhost:8080",
	}
	cfg.Peers = []config.DiagPeerConfig{*peer}
	svc.targetPeers = cfg.TargetToDiagPeers()

	// Simulate successful capture at t=0 (without actual HTTP call)
	t0 := time.Date(2026, 6, 22, 14, 54, 0, 0, time.UTC)
	store.SetLastCaptureAnchor("tovarisch-peer", state.CaptureCooldownAnchor{
		AnchorCaptureID: "anchor-evicted-001",
		AnchorTargetID:  "target-1",
		AnchorProbeKind: "http",
		AnchorSource:    "tovarisch-peer",
		AnchorCreatedAt: t0,
	})

	// Wire anchor validator that returns false (anchor NOT retained)
	svc.SetAnchorValidator(func(targetID, probeKind string) []state.SpikeEvent {
		// Spike store returns empty - anchor was evicted
		return []state.SpikeEvent{}
	})

	// Spike at t=5s with anchor not in timeline
	now := t0.Add(5 * time.Second)

	// Evaluate cooldown with anchor validation
	decision := store.EvaluateCooldownWithAnchorValidation(now, "tovarisch-peer", cfg.CooldownSeconds, svc.anchorValidator)

	// CRITICAL: Even though lastCapture exists, suppressed anchor must NOT suppress
	if decision.IsInCooldown {
		t.Error("Ghost suppression: should NOT suppress when anchor spike is not retained")
	}

	// Verify cooldown was cleared
	anchor := store.GetLastCaptureAnchor("tovarisch-peer")
	if anchor.AnchorCaptureID != "" {
		t.Error("Stale anchor should have been cleared")
	}

	t.Log("PASS: Ghost suppression prevented when anchor is not retained")
}

// TestCaptureService_ValidAnchorAllowsSuppression tests that suppression IS allowed
// when the anchor spike is retained in the timeline.
func TestCaptureService_ValidAnchorAllowsSuppression(t *testing.T) {
	cfg := &config.DiagnosticsConfig{
		Enabled:          true,
		CaptureOnSpike:  true,
		CooldownSeconds: 90,
		TimeoutMs:       5000,
	}

	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	peer := &config.DiagPeerConfig{
		Name:   "tovarisch-peer",
		BaseURL: "http://localhost:8080",
	}
	cfg.Peers = []config.DiagPeerConfig{*peer}
	svc.targetPeers = cfg.TargetToDiagPeers()

	// Simulate successful capture at t=0
	t0 := time.Date(2026, 6, 22, 14, 54, 0, 0, time.UTC)
	store.SetLastCaptureAnchor("tovarisch-peer", state.CaptureCooldownAnchor{
		AnchorCaptureID: "anchor-retained-001",
		AnchorTargetID:  "target-1",
		AnchorProbeKind: "http",
		AnchorSource:    "tovarisch-peer",
		AnchorCreatedAt: t0,
	})

	// Wire anchor validator that returns true (anchor IS retained)
	svc.SetAnchorValidator(func(targetID, probeKind string) []state.SpikeEvent {
		return []state.SpikeEvent{
			{EventID: "anchor-retained-001"}, // Anchor spike is retained
		}
	})

	// Spike at t=5s with anchor in timeline
	now := t0.Add(5 * time.Second)

	// Evaluate cooldown with anchor validation
	decision := store.EvaluateCooldownWithAnchorValidation(now, "tovarisch-peer", cfg.CooldownSeconds, svc.anchorValidator)

	// CRITICAL: Valid anchor SHOULD suppress
	if !decision.IsInCooldown {
		t.Error("Valid anchor SHOULD suppress")
	}

	t.Log("PASS: Valid anchor allows suppression")
}

// TestCaptureService_CrossProbeSuppressionWithAnchor tests that cross-probe
// suppression works correctly when anchor is retained.
func TestCaptureService_CrossProbeSuppressionWithAnchor(t *testing.T) {
	cfg := &config.DiagnosticsConfig{
		Enabled:          true,
		CaptureOnSpike:  true,
		CooldownSeconds: 90,
		TimeoutMs:       5000,
	}

	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	peer := &config.DiagPeerConfig{
		Name:   "tovarisch-peer",
		BaseURL: "http://localhost:8080",
	}
	cfg.Peers = []config.DiagPeerConfig{*peer}
	svc.targetPeers = cfg.TargetToDiagPeers()

	// Simulate ICMP anchor capture at t=0
	t0 := time.Date(2026, 6, 22, 14, 54, 0, 0, time.UTC)
	store.SetLastCaptureAnchor("tovarisch-peer", state.CaptureCooldownAnchor{
		AnchorCaptureID: "icmp-anchor-001",
		AnchorTargetID:  "target-1",
		AnchorProbeKind: "icmp", // ICMP anchor
		AnchorSource:    "tovarisch-peer",
		AnchorCreatedAt: t0,
	})

	// Wire anchor validator that returns true (anchor IS retained)
	svc.SetAnchorValidator(func(targetID, probeKind string) []state.SpikeEvent {
		return []state.SpikeEvent{
			{EventID: "icmp-anchor-001"},
		}
	})

	// HTTP spike at t=5s (cross-probe scenario)
	now := t0.Add(5 * time.Second)

	// Evaluate cooldown with anchor validation
	decision := store.EvaluateCooldownWithAnchorValidation(now, "tovarisch-peer", cfg.CooldownSeconds, svc.anchorValidator)

	// CRITICAL: ICMP anchor SHOULD suppress HTTP spike (cross-probe suppression allowed)
	if !decision.IsInCooldown {
		t.Error("Cross-probe suppression SHOULD suppress when anchor is retained")
	}

	t.Log("PASS: Cross-probe suppression works with retained anchor")
}

// TestCaptureService_NoValidatorFallsBackToBasicCooldown tests that without
// anchor validator, the service falls back to basic cooldown behavior.
func TestCaptureService_NoValidatorFallsBackToBasicCooldown(t *testing.T) {
	cfg := &config.DiagnosticsConfig{
		Enabled:          true,
		CaptureOnSpike:  true,
		CooldownSeconds: 5,
		TimeoutMs:       5000,
	}

	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	peer := &config.DiagPeerConfig{
		Name:   "tovarisch-peer",
		BaseURL: "http://localhost:8080",
	}
	cfg.Peers = []config.DiagPeerConfig{*peer}
	svc.targetPeers = cfg.TargetToDiagPeers()

	// Simulate successful capture at t=0
	t0 := time.Date(2026, 6, 22, 14, 54, 0, 0, time.UTC)
	store.SetLastCapture("tovarisch-peer", t0)

	// NO anchor validator set - should fall back to basic cooldown

	// Spike at t=3s (within 5s cooldown)
	now := t0.Add(3 * time.Second)
	decision := store.EvaluateCooldown(now, "tovarisch-peer", cfg.CooldownSeconds)

	// Should be in cooldown (fallback behavior)
	if !decision.IsInCooldown {
		t.Error("Fallback to basic cooldown should suppress")
	}

	t.Log("PASS: No validator falls back to basic cooldown")
}

// TestCaptureService_SetAnchorValidator_WiresCorrectly tests that SetAnchorValidator
// properly wires the validator function.
func TestCaptureService_SetAnchorValidator_WiresCorrectly(t *testing.T) {
	cfg := &config.DiagnosticsConfig{
		Enabled:          true,
		CaptureOnSpike:  true,
		CooldownSeconds: 90,
		TimeoutMs:       5000,
	}

	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	// Verify validator is nil initially
	if svc.anchorValidator != nil {
		t.Error("Validator should be nil before SetAnchorValidator is called")
	}

	// Set anchor validator
	svc.SetAnchorValidator(func(targetID, probeKind string) []state.SpikeEvent {
		return []state.SpikeEvent{{EventID: "test-spike"}}
	})

	// Verify validator is set
	if svc.anchorValidator == nil {
		t.Error("Validator should be set after SetAnchorValidator is called")
	}

	// Verify spikeStoreGetter is set
	if svc.spikeStoreGetter == nil {
		t.Error("spikeStoreGetter should be set after SetAnchorValidator is called")
	}

	// Verify validator works
	anchor := state.CaptureCooldownAnchor{
		AnchorCaptureID: "test-spike",
		AnchorTargetID:  "target-1",
		AnchorProbeKind: "http",
	}
	result := svc.anchorValidator(anchor)
	if !result.IsValid {
		t.Error("Validator should return true for retained spike")
	}

	t.Log("PASS: SetAnchorValidator wires correctly")
}

// TestCaptureService_TriggerCapture_WithRetainedCapturedAnchor_Suppresses tests that
// suppression IS allowed when the anchor spike is retained and the anchor capture exists.
func TestCaptureService_TriggerCapture_WithRetainedCapturedAnchor_Suppresses(t *testing.T) {
	// Create fake tovarisch server that returns OK
	tovarischServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"network_diag":{"started_at":"2026-06-22T14:54:00Z","status":"ok","interfaces":[],"routes":[],"underlay_tcp":[],"events":[]}}`))
	}))
	defer tovarischServer.Close()

	cfg := &config.DiagnosticsConfig{
		Enabled:          true,
		CaptureOnSpike:  true,
		CooldownSeconds: 90,
		TimeoutMs:       5000,
	}

	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	// Create peer with target-1 in its Targets list
	peer := &config.DiagPeerConfig{
		Name:   "tovarisch-peer",
		BaseURL: tovarischServer.URL,
		Targets: []string{"target-1"},
	}
	cfg.Peers = []config.DiagPeerConfig{*peer}
	svc.targetPeers = cfg.TargetToDiagPeers()

	// Simulate successful capture at t=0 with anchor spike retained
	anchorCaptureID := "anchor-spike-001"
	t0 := time.Now().Add(-5 * time.Second)
	store.SetLastCaptureAnchor("tovarisch-peer", state.CaptureCooldownAnchor{
		AnchorCaptureID: anchorCaptureID,
		AnchorTargetID:  "target-1",
		AnchorProbeKind: "http",
		AnchorSource:    "tovarisch-peer",
		AnchorCreatedAt: t0,
	})

	// Also add a capture record for the anchor spike so ValidateAnchorWithCaptureStatus passes
	store.AddCapture(anchorCaptureID, state.DiagCapture{
		Status: state.DiagCaptureStatusOK,
	})

	// Use SetAnchorValidatorWithCaptureStatus which checks both spike AND capture status
	svc.SetAnchorValidatorWithCaptureStatus(
		func(targetID, probeKind string) []state.SpikeEvent {
			return []state.SpikeEvent{{EventID: anchorCaptureID}}
		},
		func(eventID string) []state.DiagCapture {
			return store.GetCaptures(eventID)
		},
	)

	// Trigger capture - should suppress because anchor is retained and capture is OK
	svc.TriggerCapture("new-spike-001", "target-1", "http")

	// Allow async capture to complete
	time.Sleep(100 * time.Millisecond)

	// Check capture
	captures := store.GetCaptures("new-spike-001")
	if len(captures) == 0 {
		t.Fatal("Expected at least one capture record")
	}

	capture := captures[0]
	
	// CRITICAL: Suppression MUST happen when anchor is retained and capture is OK
	if !capture.SuppressedByCooldown {
		t.Error("Should suppress when anchor spike is retained and capture is OK")
	}
	
	t.Log("PASS: TriggerCapture suppressed correctly when anchor is retained")
}

// TestCaptureService_TriggerCapture_WithMissingAnchor_AttemptsCapture tests that capture
// IS attempted when the anchor spike is missing (evicted) from the timeline.
func TestCaptureService_TriggerCapture_WithMissingAnchor_AttemptsCapture(t *testing.T) {
	// Create fake tovarisch server that returns OK
	tovarischServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"network_diag":{"started_at":"2026-06-22T14:54:00Z","status":"ok","interfaces":[],"routes":[],"underlay_tcp":[],"events":[]}}`))
	}))
	defer tovarischServer.Close()

	cfg := &config.DiagnosticsConfig{
		Enabled:          true,
		CaptureOnSpike:  true,
		CooldownSeconds: 90,
		TimeoutMs:       5000,
	}

	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	// Create peer with target-1 in its Targets list so the mapping works
	peer := &config.DiagPeerConfig{
		Name:   "tovarisch-peer",
		BaseURL: tovarischServer.URL,
		Targets: []string{"target-1"}, // IMPORTANT: map target-1 to this peer
	}
	cfg.Peers = []config.DiagPeerConfig{*peer}
	svc.targetPeers = cfg.TargetToDiagPeers()

	// Simulate successful capture at t=0 but anchor spike is MISSING (evicted)
	anchorCaptureID := "evicted-anchor-spike-001"
	t0 := time.Now().Add(-5 * time.Second)
	store.SetLastCaptureAnchor("tovarisch-peer", state.CaptureCooldownAnchor{
		AnchorCaptureID: anchorCaptureID,
		AnchorTargetID:  "target-1",
		AnchorProbeKind: "http",
		AnchorSource:    "tovarisch-peer",
		AnchorCreatedAt: t0,
	})

	// Set anchor validator that does NOT find the anchor (spike was evicted)
	svc.SetAnchorValidator(func(targetID, probeKind string) []state.SpikeEvent {
		return []state.SpikeEvent{} // Empty - anchor was evicted
	})

	// Trigger capture - should attempt capture because anchor is missing
	svc.TriggerCapture("new-spike-001", "target-1", "http")

	// Allow async capture to complete
	time.Sleep(100 * time.Millisecond)

	// Check capture
	captures := store.GetCaptures("new-spike-001")
	if len(captures) == 0 {
		t.Fatal("Expected at least one capture record")
	}

	capture := captures[0]
	
	// CRITICAL: Capture MUST NOT be suppressed when anchor is missing (evicted)
	// This is the ghost suppression fix - if anchor is evicted, allow capture
	if capture.SuppressedByCooldown {
		t.Error("Should NOT suppress when anchor spike is missing (ghost suppression prevention)")
	}
	
	// Verify the capture was attempted (should have OK status or similar)
	if capture.Status != state.DiagCaptureStatusOK {
		t.Errorf("Expected OK status, got %v", capture.Status)
	}
	
	t.Log("PASS: TriggerCapture attempted capture when anchor spike was missing")
}
