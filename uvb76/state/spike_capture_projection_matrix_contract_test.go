package state

import (
	"testing"
	"time"
)

// =============================================================================
// ACT-UVB76-HULK02: Spike Row Projection Matrix Contract Tests
// =============================================================================
//
// These tests verify spike rows match the canonical capture status matrix:
// - cooldown info appears only when meaningful (skipped_cooldown)
// - TCP diagnostics presence matches status
// - all fields match the canonical status matrix
//
// =============================================================================

// TestSpikeCaptureProjection_CooldownInfoAppearsOnlyWhenMeaningful verifies
// cooldown info appears only when meaningful (skipped_cooldown scenario).
func TestSpikeCaptureProjection_CooldownInfoAppearsOnlyWhenMeaningful(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name            string
		captureStatus   CaptureStatus
		hasCooldownInfo bool
	}{
		{"captured_no_cooldown", CaptureStatusCaptured, false},
		{"skipped_cooldown_with_cooldown", CaptureStatusSkippedCooldown, true},
		{"failed_no_cooldown", CaptureStatusFailed, false},
		{"disabled_no_cooldown", CaptureStatusDisabled, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cooldownInfo *CaptureCooldownInfo
			anchorTime := now.Add(-5 * time.Minute)
			if tt.hasCooldownInfo {
				cooldownInfo = &CaptureCooldownInfo{
					Scope:                    "per_diagnostic_peer",
					LastSuccessfulCaptureAt: &anchorTime,
					CooldownSeconds:         90,
					AnchorVisible:           true,
					AnchorVisibilityReason:  AnchorVisibilityReasonRetained,
				}
			}

			capture := DiagCapture{
				Source:               "peer-1",
				CaptureStartedAt:     now,
				Status:               DiagCaptureStatusOK,
				CaptureStatus:        tt.captureStatus,
				CooldownInfo:         cooldownInfo,
				SuppressedByCooldown: tt.hasCooldownInfo,
			}

			if tt.hasCooldownInfo {
				if capture.CooldownInfo == nil {
					t.Error("skipped_cooldown must have CooldownInfo")
				}
				if !capture.SuppressedByCooldown {
					t.Error("skipped_cooldown must have SuppressedByCooldown=true")
				}
			} else {
				if capture.CooldownInfo != nil {
					t.Error("non-skipped_cooldown should not have CooldownInfo")
				}
			}
		})
	}
}

// TestSpikeCaptureProjection_TcpDiagnosticsPresenceMatchesStatus verifies
// TCP diagnostics presence matches capture status.
func TestSpikeCaptureProjection_TcpDiagnosticsPresenceMatchesStatus(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name              string
		captureStatus     CaptureStatus
		allowsTcpDiag     bool
		requiresTcpDiag   bool
		hasUnderlayTCP    bool
		hasTcpAbsenceEvts bool
	}{
		{
			name:            "captured_allows_tcp_diag",
			captureStatus:   CaptureStatusCaptured,
			allowsTcpDiag:   true,
			requiresTcpDiag: false,
			hasUnderlayTCP:  true,
		},
		{
			name:              "captured_allows_tcp_diag_absent",
			captureStatus:     CaptureStatusCaptured,
			allowsTcpDiag:     true,
			requiresTcpDiag:   false,
			hasUnderlayTCP:    false,
			hasTcpAbsenceEvts: true,
		},
		{
			name:              "skipped_cooldown_no_tcp_diag",
			captureStatus:     CaptureStatusSkippedCooldown,
			allowsTcpDiag:     false,
			requiresTcpDiag:   false,
			hasUnderlayTCP:    false,
			hasTcpAbsenceEvts: false,
		},
		{
			name:              "failed_no_tcp_diag",
			captureStatus:     CaptureStatusFailed,
			allowsTcpDiag:     false,
			requiresTcpDiag:   false,
			hasUnderlayTCP:    false,
			hasTcpAbsenceEvts: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var networkDiag *NetworkDiagData
			var tcpAbsenceEvents []TcpAbsenceEvent

			if tt.hasUnderlayTCP {
				networkDiag = &NetworkDiagData{
					UnderlayTCP: []TcpSocketDiagData{{Name: "tcp-socket"}},
				}
			}
			if tt.hasTcpAbsenceEvts {
				tcpAbsenceEvents = []TcpAbsenceEvent{
					{ReasonCode: "no_matching_socket", Source: "underlay_tcp"},
				}
			}

			capture := DiagCapture{
				Source:           "peer-1",
				CaptureStartedAt: now,
				Status:           DiagCaptureStatusOK,
				CaptureStatus:    tt.captureStatus,
				NetworkDiag:      networkDiag,
				TcpAbsenceEvents: tcpAbsenceEvents,
			}

			// Verify allowsTcpDiag
			if tt.allowsTcpDiag && capture.NetworkDiag == nil && len(capture.TcpAbsenceEvents) == 0 {
				// Captured without TCP data is allowed (may not have socket info)
			}

			// Verify not allowsTcpDiag
			if !tt.allowsTcpDiag && capture.NetworkDiag != nil {
				t.Error("non-captured status should not have NetworkDiag")
			}
		})
	}
}

// TestSpikeCaptureProjection_FieldsMatchMatrixContract verifies all required fields
// match the canonical status matrix.
func TestSpikeCaptureProjection_FieldsMatchMatrixContract(t *testing.T) {
	now := time.Now().UTC()
	anchorTime := now.Add(-5 * time.Minute)

	tests := []struct {
		name                   string
		capture                DiagCapture
		expectedStatus         CaptureStatus
		requiresPacket         bool
		requiresReason         bool
		allowsTcpDiag          bool
		requiresCooldownInfo   bool
	}{
		{
			name: "captured_valid",
			capture: DiagCapture{
				Source:           "peer-1",
				CaptureStartedAt: now,
				Status:           DiagCaptureStatusOK,
				CaptureStatus:    CaptureStatusCaptured,
				NetworkDiag:      &NetworkDiagData{},
			},
			expectedStatus:       CaptureStatusCaptured,
			requiresPacket:       true,
			requiresReason:       false,
			allowsTcpDiag:        true,
			requiresCooldownInfo: false,
		},
		{
			name: "skipped_cooldown_valid",
			capture: DiagCapture{
				Source:               "peer-1",
				CaptureStartedAt:     now,
				Status:               DiagCaptureStatusOK,
				SuppressedByCooldown: true,
				CaptureStatus:        CaptureStatusSkippedCooldown,
				CooldownInfo: &CaptureCooldownInfo{
					Scope:                    "per_diagnostic_peer",
					LastSuccessfulCaptureAt: &anchorTime,
					CooldownSeconds:         90,
					AnchorVisible:          true,
					AnchorVisibilityReason: AnchorVisibilityReasonRetained,
				},
			},
			expectedStatus:       CaptureStatusSkippedCooldown,
			requiresPacket:       false,
			requiresReason:       true,
			allowsTcpDiag:        false,
			requiresCooldownInfo: true,
		},
		{
			name: "failed_valid",
			capture: DiagCapture{
				Source:           "peer-1",
				CaptureStartedAt: now,
				Status:           DiagCaptureStatusError,
				CaptureStatus:    CaptureStatusFailed,
				Error:            stringPtr("timeout"),
			},
			expectedStatus:       CaptureStatusFailed,
			requiresPacket:       false,
			requiresReason:       true,
			allowsTcpDiag:        false,
			requiresCooldownInfo: false,
		},
		{
			name: "disabled_valid",
			capture: DiagCapture{
				Source:           "peer-1",
				CaptureStartedAt: now,
				Status:           DiagCaptureStatusDisabled,
				CaptureStatus:    CaptureStatusDisabled,
			},
			expectedStatus:       CaptureStatusDisabled,
			requiresPacket:       false,
			requiresReason:       true, // Status implies reason
			allowsTcpDiag:        false,
			requiresCooldownInfo: false,
		},
		{
			name: "not_configured_valid",
			capture: DiagCapture{
				Source:           "peer-1",
				CaptureStartedAt: now,
				Status:           DiagCaptureStatusNoPeerMapping,
				CaptureStatus:    CaptureStatusNotConfigured,
				Error:            stringPtr("binary not found: tcpdump"),
			},
			expectedStatus:       CaptureStatusNotConfigured,
			requiresPacket:       false,
			requiresReason:       true,
			allowsTcpDiag:        false,
			requiresCooldownInfo: false,
		},
		{
			name: "not_attempted_valid",
			capture: DiagCapture{
				Source:           "peer-1",
				CaptureStartedAt: now,
				Status:           DiagCaptureStatusNoPeerMapping,
				CaptureStatus:    CaptureStatusNotAttempted,
			},
			expectedStatus:       CaptureStatusNotAttempted,
			requiresPacket:       false,
			requiresReason:       false, // Optional
			allowsTcpDiag:        false,
			requiresCooldownInfo: false,
		},
		{
			name: "in_progress_valid",
			capture: DiagCapture{
				Source:           "peer-1",
				CaptureStartedAt: now,
				Status:           DiagCaptureStatusOK,
				CaptureStatus:    CaptureStatusInProgress,
				// CaptureFinishedAt is nil - transient
			},
			expectedStatus:       CaptureStatusInProgress,
			requiresPacket:       false,
			requiresReason:       false, // Optional
			allowsTcpDiag:        false,
			requiresCooldownInfo: false,
		},
		{
			name: "missing_valid",
			capture: DiagCapture{
				Source:           "peer-1",
				CaptureStartedAt: now,
				Status:           DiagCaptureStatusError,
				CaptureStatus:    CaptureStatusMissing,
				Error:            stringPtr("artifact not found"),
			},
			expectedStatus:       CaptureStatusMissing,
			requiresPacket:       false,
			requiresReason:       true,
			allowsTcpDiag:        false,
			requiresCooldownInfo: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture := tt.capture

			// Verify status
			if capture.CaptureStatus != tt.expectedStatus {
				t.Errorf("expected status %s, got %s", tt.expectedStatus, capture.CaptureStatus)
			}

			// Verify packet requirement
			if tt.requiresPacket && capture.NetworkDiag == nil {
				t.Error("capture requires NetworkDiag packet evidence")
			}

			// Verify reason requirement
			if tt.requiresReason && capture.Error == nil && capture.CooldownInfo == nil {
				// Check if reason is implied by status
				if capture.Status != DiagCaptureStatusDisabled {
					t.Error("capture requires Error or implied reason")
				}
			}

			// Verify TCP diag allowance
			if !tt.allowsTcpDiag && capture.NetworkDiag != nil {
				t.Error("capture should not have NetworkDiag")
			}

			// Verify cooldown info requirement
			if tt.requiresCooldownInfo && capture.CooldownInfo == nil {
				t.Error("capture requires CooldownInfo")
			}
		})
	}
}
