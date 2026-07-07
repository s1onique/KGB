package state

import (
	"testing"
	"time"
)

// =============================================================================
// ACT-UVB76-HULK02: Capture State Machine Invariant Contract Tests
// =============================================================================
//
// These tests verify capture state machine invariants:
// - failed capture cannot masquerade as skipped_cooldown
// - disabled cannot masquerade as not_configured
//
// =============================================================================

// TestCaptureStateMachine_FailedDoesNotMasqueradeAsSkippedCooldown verifies
// failed capture cannot masquerade as skipped_cooldown.
func TestCaptureStateMachine_FailedDoesNotMasqueradeAsSkippedCooldown(t *testing.T) {
	store := NewCaptureStore()

	errMsg := "connection refused"
	capture := DiagCapture{
		Source:               "peer-1",
		CaptureStartedAt:     time.Now().UTC(),
		Status:               DiagCaptureStatusError,
		SuppressedByCooldown: false, // NOT suppressed
		CaptureStatus:        CaptureStatusFailed,
		Error:                &errMsg,
	}
	finishCapture(&capture)
	store.AddCapture("event-1", capture)

	captures := store.GetCaptures("event-1")
	if captures[0].CaptureStatus == CaptureStatusSkippedCooldown {
		t.Error("failed status cannot masquerade as skipped_cooldown")
	}
	if captures[0].SuppressedByCooldown {
		t.Error("failed capture should not have SuppressedByCooldown=true")
	}
}

// TestCaptureStateMachine_DisabledDoesNotMasqueradeAsNotConfigured verifies
// disabled cannot masquerade as not_configured.
func TestCaptureStateMachine_DisabledDoesNotMasqueradeAsNotConfigured(t *testing.T) {
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
	if captures[0].CaptureStatus == CaptureStatusNotConfigured {
		t.Error("disabled status cannot masquerade as not_configured")
	}
	if captures[0].Status != DiagCaptureStatusDisabled {
		t.Error("disabled must have DiagCaptureStatusDisabled")
	}
}
