package diag

import (
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// TestCaptureService_DoesNotRecordSkippedCooldownWithoutPriorCapture verifies that
// recordSuppressedCapture never records skipped_cooldown status when there is no
// prior successful capture. This is a critical invariant: skipped_cooldown requires
// a valid prior successful capture to exist.
//
// This test verifies BOTH raw storage AND API projection (GetCaptureInfo).
func TestCaptureService_DoesNotRecordSkippedCooldownWithoutPriorCapture(t *testing.T) {
	cfg := &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike: true,
		CooldownSeconds: 60,
		TimeoutMs:       5000,
	}
	
	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)
	
	peer := &config.DiagPeerConfig{
		Name:   "peer-1",
		BaseURL: "http://localhost:8080",
	}
	
	// Prime the targetPeers map so peer lookup works
	cfg.Peers = []config.DiagPeerConfig{*peer}
	svc.targetPeers = cfg.TargetToDiagPeers()
	
	// FIRST SPIKE: Trigger capture with NO prior successful capture.
	// The CaptureStore has lastCapture["peer-1"] = zero time (never set).
	// recordSuppressedCapture should NOT record skipped_cooldown.
	svc.TriggerCapture("event-1", "peer-1")
	
	// Give async goroutine time to complete
	time.Sleep(100 * time.Millisecond)
	
	// Check 1: Raw storage should not have skipped_cooldown
	captures := store.GetCaptures("event-1")
	if len(captures) == 0 {
		t.Fatal("expected at least one capture record")
	}
	
	capture := captures[len(captures)-1]
	
	// The critical invariant: skipped_cooldown must NOT be set without a prior capture
	if capture.CaptureStatus == state.CaptureStatusSkippedCooldown {
		t.Errorf("INVARIANT VIOLATION: raw capture has skipped_cooldown without prior successful capture. "+
			"capture_status=%s, suppressed=%v, cooldown_info=%v",
			capture.CaptureStatus, capture.SuppressedByCooldown, capture.CooldownInfo)
	}
	
	// SuppressedByCooldown flag should NOT be set for non-cooldown events
	if capture.SuppressedByCooldown {
		t.Error("INVARIANT VIOLATION: SuppressedByCooldown should not be set for non-cooldown events")
	}
	
	// The correct behavior: should be not_attempted
	if capture.CaptureStatus == state.CaptureStatusNotAttempted {
		t.Log("CORRECT: not_attempted recorded when no prior capture exists")
	}
	
	// Check 2: API projection (GetCaptureInfo) should also not return skipped_cooldown
	info := store.GetCaptureInfo("event-1", false)
	if info.CaptureStatus == state.CaptureStatusSkippedCooldown {
		t.Fatalf("INVARIANT VIOLATION: GetCaptureInfo projected no-source capture as skipped_cooldown. "+
			"capture_status=%s, cooldown_info=%v", info.CaptureStatus, info.CooldownInfo)
	}
	if info.CooldownInfo != nil {
		t.Fatalf("INVARIANT VIOLATION: GetCaptureInfo returned cooldown_info without prior successful capture: %+v", info.CooldownInfo)
	}
	
	t.Logf("PASS: Both raw storage and API projection verified. status=%s, cooldown_info=%v", info.CaptureStatus, info.CooldownInfo)
}

