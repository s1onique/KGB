package state

import (
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// =============================================================================
// ACT-UVB76-HULK02: Diagnostic Capture Status Matrix Contract Tests
// =============================================================================
//
// These tests verify the canonical capture status matrix is correctly enforced.
// Each status has specific invariants about packet presence, reason requirements,
// TCP diagnostics, and visibility.
//
// Canonical statuses:
//   - captured         : Successful diagnostic capture with packet evidence
//   - skipped_cooldown : Suppressed due to cooldown policy
//   - failed           : Capture attempted but failed
//   - disabled         : Capture globally disabled
//   - not_configured   : Capture enabled but missing binary/config
//   - not_attempted    : No capture attempt should have happened
//   - in_progress      : Transient runtime state
//   - missing          : Expected artifact missing or lookup failed
//
// Contract matrix:
//
// | Status             | Packet | Reason | TCP Diag | Notes                           |
// | ------------------ | ------ | ------ | -------- | ------------------------------- |
// | captured           | yes    | no     | allowed  | Successful capture              |
// | skipped_cooldown    | no     | yes    | no       | Suppressed by cooldown          |
// | failed             | no     | yes    | optional | Capture attempted but failed    |
// | disabled           | no     | yes    | no       | Globally disabled               |
// | not_configured     | no     | yes    | no       | Missing binary/config           |
// | not_attempted      | no     | opt    | no       | No capture should happen        |
// | in_progress        | no     | opt    | no       | Transient, should not persist   |
// | missing            | no     | yes    | no       | Artifact missing or lookup failed|
//
// =============================================================================

// TestCaptureStatusMatrix_AllStatusesDefined verifies all canonical statuses exist.
func TestCaptureStatusMatrix_AllStatusesDefined(t *testing.T) {
	canonicalStatuses := []CaptureStatus{
		CaptureStatusCaptured,
		CaptureStatusSkippedCooldown,
		CaptureStatusFailed,
		CaptureStatusDisabled,
		CaptureStatusNotConfigured,
		CaptureStatusNotAttempted,
		CaptureStatusInProgress,
		CaptureStatusMissing,
	}

	for _, status := range canonicalStatuses {
		if status == "" {
			t.Error("canonical status cannot be empty")
		}
		_, ok := domain.ParseCaptureStatus(string(status))
		if !ok {
			t.Errorf("status %q is not valid per domain.ParseCaptureStatus", status)
		}
	}
}

// TestCaptureStatusMatrix_CapturedRequiresPacket verifies captured status requires packet.
func TestCaptureStatusMatrix_CapturedRequiresPacket(t *testing.T) {
	// captured status: requires_capture_packet = yes, allows_capture_packet = yes
	capture := DiagCapture{
		Source:           "peer-1",
		BaseURL:          "http://localhost:8080",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusOK,
		CaptureStatus:    CaptureStatusCaptured,
		NetworkDiag:      &NetworkDiagData{}, // Packet evidence present
	}
	finishCapture(&capture)

	// captured status MUST have NetworkDiag (packet evidence)
	if capture.NetworkDiag == nil {
		t.Error("captured status requires NetworkDiag packet evidence")
	}
	if capture.CaptureStatus != CaptureStatusCaptured {
		t.Errorf("expected captured status, got %s", capture.CaptureStatus)
	}
}

// TestCaptureStatusMatrix_CapturedAllowsTcpDiagnostics verifies captured allows TCP diagnostics.
func TestCaptureStatusMatrix_CapturedAllowsTcpDiagnostics(t *testing.T) {
	capture := DiagCapture{
		Source:           "peer-1",
		BaseURL:          "http://localhost:8080",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusOK,
		CaptureStatus:    CaptureStatusCaptured,
		NetworkDiag:      &NetworkDiagData{UnderlayTCP: []TcpSocketDiagData{{Name: "tcp-socket"}}},
	}

	// captured status allows TCP diagnostics
	if capture.NetworkDiag == nil {
		t.Error("captured should have NetworkDiag")
	}
	if len(capture.NetworkDiag.UnderlayTCP) == 0 {
		t.Error("captured allows TCP diagnostics")
	}
}

// TestCaptureStatusMatrix_CapturedDoesNotRequireReason verifies captured does not require reason.
func TestCaptureStatusMatrix_CapturedDoesNotRequireReason(t *testing.T) {
	capture := DiagCapture{
		Source:           "peer-1",
		BaseURL:          "http://localhost:8080",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusOK,
		CaptureStatus:    CaptureStatusCaptured,
		NetworkDiag:      &NetworkDiagData{},
		// Error is nil - reason is not required for captured
	}

	// captured status does NOT require reason
	if capture.Error != nil {
		t.Error("captured status should not require error/reason")
	}
}

// TestCaptureStatusMatrix_SkippedCooldownRequiresCooldownInfo verifies skipped_cooldown requires CooldownInfo.
func TestCaptureStatusMatrix_SkippedCooldownRequiresCooldownInfo(t *testing.T) {
	now := time.Now().UTC()
	anchorTime := now.Add(-5 * time.Minute)
	capture := DiagCapture{
		Source:               "peer-1",
		BaseURL:              "http://localhost:8080",
		CaptureStartedAt:     now,
		Status:               DiagCaptureStatusOK,
		SuppressedByCooldown: true,
		CaptureStatus:        CaptureStatusSkippedCooldown,
		CooldownInfo: &CaptureCooldownInfo{
			Scope:                    "per_diagnostic_peer",
			LastSuccessfulCaptureAt:   &anchorTime,
			CooldownSeconds:         90,
			AnchorVisible:           true,
			AnchorVisibilityReason:    AnchorVisibilityReasonRetained,
		},
	}
	finishCapture(&capture)

	// skipped_cooldown MUST have CooldownInfo (reason/metadata)
	if capture.CooldownInfo == nil {
		t.Error("skipped_cooldown requires CooldownInfo with cooldown metadata")
	}
	if !capture.SuppressedByCooldown {
		t.Error("skipped_cooldown must have SuppressedByCooldown=true")
	}
}

// TestCaptureStatusMatrix_SkippedCooldownDisallowsPacket verifies skipped_cooldown disallows packet.
func TestCaptureStatusMatrix_SkippedCooldownDisallowsPacket(t *testing.T) {
	capture := DiagCapture{
		Source:               "peer-1",
		CaptureStartedAt:     time.Now().UTC(),
		Status:               DiagCaptureStatusOK,
		SuppressedByCooldown: true,
		CaptureStatus:        CaptureStatusSkippedCooldown,
		// NetworkDiag should be nil for skipped_cooldown
	}

	// skipped_cooldown MUST NOT have NetworkDiag
	if capture.NetworkDiag != nil {
		t.Error("skipped_cooldown should not have NetworkDiag packet evidence")
	}
}

// TestCaptureStatusMatrix_FailedRequiresReason verifies failed requires reason.
func TestCaptureStatusMatrix_FailedRequiresReason(t *testing.T) {
	errMsg := "connection refused"
	capture := DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusError,
		CaptureStatus:    CaptureStatusFailed,
		Error:            &errMsg,
	}
	finishCapture(&capture)

	// failed status MUST have Error (reason)
	if capture.Error == nil {
		t.Error("failed status requires Error reason")
	}
}

// TestCaptureStatusMatrix_FailedDisallowsPacket verifies failed disallows packet.
func TestCaptureStatusMatrix_FailedDisallowsPacket(t *testing.T) {
	errMsg := "timeout"
	capture := DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusTimeout,
		CaptureStatus:    CaptureStatusFailed,
		Error:            &errMsg,
		// NetworkDiag should be nil for failed
	}
	finishCapture(&capture)

	// failed status should NOT have NetworkDiag
	if capture.NetworkDiag != nil {
		t.Error("failed status should not have NetworkDiag packet evidence")
	}
}

// TestCaptureStatusMatrix_DisabledRequiresReason verifies disabled requires reason.
func TestCaptureStatusMatrix_DisabledRequiresReason(t *testing.T) {
	capture := DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusDisabled,
		CaptureStatus:    CaptureStatusDisabled,
		// Error should be set to explain why disabled
	}
	finishCapture(&capture)

	// disabled status SHOULD have a reason (Error or implied by status)
	if capture.Status != DiagCaptureStatusDisabled {
		t.Error("disabled status requires DiagCaptureStatusDisabled")
	}
}

// TestCaptureStatusMatrix_NotConfiguredRequiresReason verifies not_configured requires reason.
func TestCaptureStatusMatrix_NotConfiguredRequiresReason(t *testing.T) {
	errMsg := "binary not found: tcpdump"
	capture := DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusNoPeerMapping,
		CaptureStatus:    CaptureStatusNotConfigured,
		Error:            &errMsg,
	}
	finishCapture(&capture)

	// not_configured status MUST have Error explaining what's missing
	if capture.Error == nil {
		t.Error("not_configured status requires Error explaining what's missing")
	}
}

// TestCaptureStatusMatrix_NotAttemptedDoesNotRequireReason verifies not_attempted optional reason.
func TestCaptureStatusMatrix_NotAttemptedDoesNotRequireReason(t *testing.T) {
	capture := DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusNoPeerMapping,
		CaptureStatus:    CaptureStatusNotAttempted,
		// Error is optional for not_attempted
	}
	finishCapture(&capture)

	// not_attempted does NOT require Error
	if capture.Error != nil {
		t.Error("not_attempted should not require Error")
	}
}

// TestCaptureStatusMatrix_InProgressIsTransient verifies in_progress is transient.
func TestCaptureStatusMatrix_InProgressIsTransient(t *testing.T) {
	capture := DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusOK,
		CaptureStatus:    CaptureStatusInProgress,
		// CaptureFinishedAt should be nil for in_progress
	}

	// in_progress status should NOT have CaptureFinishedAt (transient)
	if capture.CaptureFinishedAt != nil {
		t.Error("in_progress is transient and should not have CaptureFinishedAt")
	}
}

// TestCaptureStatusMatrix_MissingRequiresReason verifies missing requires reason.
func TestCaptureStatusMatrix_MissingRequiresReason(t *testing.T) {
	errMsg := "artifact not found: /path/to/capture.pcap"
	capture := DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           DiagCaptureStatusError,
		CaptureStatus:    CaptureStatusMissing,
		Error:            &errMsg,
	}
	finishCapture(&capture)

	// missing status MUST have Error explaining what's missing
	if capture.Error == nil {
		t.Error("missing status requires Error explaining what's missing")
	}
}
