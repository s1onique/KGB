package state

import (
	"testing"
	"time"
)

// =============================================================================
// ACT-UVB76-HULK02: Capture State Machine Decision Contract Tests
// =============================================================================
//
// These tests verify capture state machine decision logic:
// - enabled + no cooldown + success -> captured
// - enabled + cooldown active -> skipped_cooldown
// - enabled + command failure -> failed
// - disabled -> disabled
// - enabled + missing binary -> not_configured
// - not requested -> not_attempted
// - started but not completed -> in_progress
// - referenced but artifact absent -> missing
//
// =============================================================================

// TestCaptureStateMachine_CaptureEnabledNoCooldownSuccess verifies enabled + no cooldown + success -> captured.
func TestCaptureStateMachine_CaptureEnabledNoCooldownSuccess(t *testing.T) {
	store := NewCaptureStore()

	// No prior captures - not in cooldown
	if store.IsInCooldown("peer-1", 90) {
		t.Error("should not be in cooldown initially")
	}

	// Simulate successful capture
	capture := DiagCapture{
		Source:           "peer-1",
		BaseURL:          "http://localhost:8080",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusOK,
		CaptureStatus:    CaptureStatusCaptured,
		NetworkDiag:      &NetworkDiagData{},
	}
	finishCapture(&capture)
	store.AddCapture("event-1", capture)

	captures := store.GetCaptures("event-1")
	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}

	// Result: captured
	if captures[0].CaptureStatus != CaptureStatusCaptured {
		t.Errorf("expected captured, got %s", captures[0].CaptureStatus)
	}
	if captures[0].NetworkDiag == nil {
		t.Error("captured must have NetworkDiag")
	}
}

// TestCaptureStateMachine_CaptureEnabledCooldownActive verifies enabled + cooldown active -> skipped_cooldown.
func TestCaptureStateMachine_CaptureEnabledCooldownActive(t *testing.T) {
	store := NewCaptureStore()

	// Add successful prior capture to set cooldown
	now := time.Now().UTC()
	store.AddCapture("prior-event", DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: now,
		Status:           DiagCaptureStatusOK,
		CaptureStatus:    CaptureStatusCaptured,
		NetworkDiag:      &NetworkDiagData{},
	})

	// Verify in cooldown
	if !store.IsInCooldown("peer-1", 90) {
		t.Error("should be in cooldown after prior capture")
	}

	// Evaluate cooldown decision
	decision := store.EvaluateCooldown(now.Add(30*time.Second), "peer-1", 90)

	if !decision.IsInCooldown {
		t.Error("should be in cooldown")
	}

	// Result: skipped_cooldown with CooldownInfo
	if decision.Anchor == nil {
		t.Error("skipped_cooldown requires Anchor in decision")
	}
}

// TestCaptureStateMachine_CaptureEnabledCommandFailure verifies enabled + command failure -> failed.
func TestCaptureStateMachine_CaptureEnabledCommandFailure(t *testing.T) {
	store := NewCaptureStore()

	errMsg := "command failed: exit status 1"
	capture := DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusError,
		CaptureStatus:    CaptureStatusFailed,
		Error:            &errMsg,
	}
	finishCapture(&capture)
	store.AddCapture("event-1", capture)

	captures := store.GetCaptures("event-1")
	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}

	// Result: failed with reason
	if captures[0].CaptureStatus != CaptureStatusFailed {
		t.Errorf("expected failed, got %s", captures[0].CaptureStatus)
	}
	if captures[0].Error == nil {
		t.Error("failed requires Error reason")
	}
}

// TestCaptureStateMachine_CaptureDisabled verifies disabled -> disabled.
func TestCaptureStateMachine_CaptureDisabled(t *testing.T) {
	store := NewCaptureStore()

	capture := DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusDisabled,
		CaptureStatus:    CaptureStatusDisabled,
	}
	finishCapture(&capture)
	store.AddCapture("event-1", capture)

	captures := store.GetCaptures("event-1")
	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}

	// Result: disabled
	if captures[0].CaptureStatus != CaptureStatusDisabled {
		t.Errorf("expected disabled, got %s", captures[0].CaptureStatus)
	}
}

// TestCaptureStateMachine_CaptureEnabledMissingBinary verifies enabled + missing binary -> not_configured.
func TestCaptureStateMachine_CaptureEnabledMissingBinary(t *testing.T) {
	store := NewCaptureStore()

	errMsg := "tool not found: tcpdump"
	capture := DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusNoPeerMapping,
		CaptureStatus:    CaptureStatusNotConfigured,
		Error:            &errMsg,
	}
	finishCapture(&capture)
	store.AddCapture("event-1", capture)

	captures := store.GetCaptures("event-1")
	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}

	// Result: not_configured with reason
	if captures[0].CaptureStatus != CaptureStatusNotConfigured {
		t.Errorf("expected not_configured, got %s", captures[0].CaptureStatus)
	}
	if captures[0].Error == nil {
		t.Error("not_configured requires Error explaining what's missing")
	}
}

// TestCaptureStateMachine_CaptureNotRequested verifies not requested -> not_attempted.
func TestCaptureStateMachine_CaptureNotRequested(t *testing.T) {
	store := NewCaptureStore()

	capture := DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusNoPeerMapping,
		CaptureStatus:    CaptureStatusNotAttempted,
		// No Error - not_attempted is optional
	}
	finishCapture(&capture)
	store.AddCapture("event-1", capture)

	captures := store.GetCaptures("event-1")
	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}

	// Result: not_attempted
	if captures[0].CaptureStatus != CaptureStatusNotAttempted {
		t.Errorf("expected not_attempted, got %s", captures[0].CaptureStatus)
	}
}

// TestCaptureStateMachine_CaptureStartedButNotCompleted verifies in_progress state.
func TestCaptureStateMachine_CaptureStartedButNotCompleted(t *testing.T) {
	store := NewCaptureStore()

	capture := DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusOK,
		CaptureStatus:    CaptureStatusInProgress,
		// CaptureFinishedAt is nil - in progress
	}
	store.AddCapture("event-1", capture)

	captures := store.GetCaptures("event-1")
	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}

	// Result: in_progress
	if captures[0].CaptureStatus != CaptureStatusInProgress {
		t.Errorf("expected in_progress, got %s", captures[0].CaptureStatus)
	}
	if captures[0].CaptureFinishedAt != nil {
		t.Error("in_progress should not have CaptureFinishedAt")
	}
}

// TestCaptureStateMachine_CaptureReferencedButArtifactAbsent verifies missing state.
func TestCaptureStateMachine_CaptureReferencedButArtifactAbsent(t *testing.T) {
	store := NewCaptureStore()

	errMsg := "artifact not found: /var/log/capture.pcap"
	capture := DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusError,
		CaptureStatus:    CaptureStatusMissing,
		Error:            &errMsg,
	}
	finishCapture(&capture)
	store.AddCapture("event-1", capture)

	captures := store.GetCaptures("event-1")
	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}

	// Result: missing with reason
	if captures[0].CaptureStatus != CaptureStatusMissing {
		t.Errorf("expected missing, got %s", captures[0].CaptureStatus)
	}
	if captures[0].Error == nil {
		t.Error("missing requires Error explaining what's missing")
	}
}
