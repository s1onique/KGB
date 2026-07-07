package state

import (
	"time"
)

// =============================================================================
// ACT-UVB76-HULK02: Shared Test Helpers for Capture State Contracts
// =============================================================================
//
// These helpers are shared across capture state machine and projection tests.
//
// =============================================================================

// finishCapture sets CaptureFinishedAt and DurationMs.
func finishCapture(capture *DiagCapture) {
	if capture.CaptureFinishedAt == nil {
		now := time.Now().UTC()
		capture.CaptureFinishedAt = &now
		duration := capture.CaptureFinishedAt.Sub(capture.CaptureStartedAt).Milliseconds()
		capture.DurationMs = &duration
	}
}

// stringPtr returns a pointer to a string.
func stringPtr(s string) *string {
	return &s
}
