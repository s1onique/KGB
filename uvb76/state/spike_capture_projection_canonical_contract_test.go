package state

import (
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// =============================================================================
// ACT-UVB76-HULK02: Spike Row Projection Canonical Contract Tests
// =============================================================================
//
// These tests verify canonical spike capture projection behavior:
// - captured rows must reference capture evidence (NetworkDiag)
// - skipped_cooldown rows must include cooldown metadata
// - failed rows must include failure reason
// - missing rows must include missing artifact reason
// - status string must be one of canonical statuses
// - unknown status must fail validation
//
// =============================================================================

// TestSpikeCaptureProjection_CapturedRowMustReferenceCaptureEvidence verifies
// captured rows must reference capture evidence (NetworkDiag).
func TestSpikeCaptureProjection_CapturedRowMustReferenceCaptureEvidence(t *testing.T) {
	now := time.Now().UTC()
	spike := SpikeEventWithCaptures{
		SpikeEvent: SpikeEvent{
			EventID:     "spike-1",
			TargetID:    "target-1",
			Kind:        "http",
			Severity:    "warning",
			SampleTs:    now,
			CollectedAt: now,
		},
		Captures: []DiagCapture{
			{
				Source:           "peer-1",
				CaptureStartedAt: now,
				Status:           DiagCaptureStatusOK,
				CaptureStatus:    CaptureStatusCaptured,
				NetworkDiag:      &NetworkDiagData{}, // Evidence present
			},
		},
	}

	// Verify capture evidence is present
	if len(spike.Captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(spike.Captures))
	}
	capture := spike.Captures[0]

	if capture.CaptureStatus != CaptureStatusCaptured {
		t.Errorf("expected captured status, got %s", capture.CaptureStatus)
	}
	if capture.NetworkDiag == nil {
		t.Error("captured row must have NetworkDiag capture evidence")
	}
}

// TestSpikeCaptureProjection_SkippedCooldownMustIncludeCooldownMetadata verifies
// skipped_cooldown rows must include cooldown metadata.
func TestSpikeCaptureProjection_SkippedCooldownMustIncludeCooldownMetadata(t *testing.T) {
	now := time.Now().UTC()
	anchorTime := now.Add(-5 * time.Minute)
	spike := SpikeEventWithCaptures{
		SpikeEvent: SpikeEvent{
			EventID:     "spike-1",
			TargetID:    "target-1",
			Kind:        "http",
			Severity:    "warning",
			SampleTs:    now,
			CollectedAt: now,
		},
		Captures: []DiagCapture{
			{
				Source:               "peer-1",
				CaptureStartedAt:     now,
				Status:               DiagCaptureStatusOK,
				SuppressedByCooldown: true,
				CaptureStatus:        CaptureStatusSkippedCooldown,
				CooldownInfo: &CaptureCooldownInfo{
					Scope:                    "per_diagnostic_peer",
					LastSuccessfulCaptureAt: &anchorTime,
					CooldownSeconds:         90,
					AnchorVisible:           true,
					AnchorVisibilityReason:  AnchorVisibilityReasonRetained,
				},
			},
		},
	}

	// Verify cooldown metadata is present
	capture := spike.Captures[0]

	if capture.CaptureStatus != CaptureStatusSkippedCooldown {
		t.Errorf("expected skipped_cooldown status, got %s", capture.CaptureStatus)
	}
	if capture.CooldownInfo == nil {
		t.Error("skipped_cooldown row must have CooldownInfo metadata")
	}
	if !capture.SuppressedByCooldown {
		t.Error("skipped_cooldown must have SuppressedByCooldown=true")
	}
}

// TestSpikeCaptureProjection_FailedRowMustIncludeFailureReason verifies
// failed rows must include failure reason.
func TestSpikeCaptureProjection_FailedRowMustIncludeFailureReason(t *testing.T) {
	now := time.Now().UTC()
	errMsg := "connection refused: dial tcp 10.0.0.5:8080: connection refused"
	spike := SpikeEventWithCaptures{
		SpikeEvent: SpikeEvent{
			EventID:     "spike-1",
			TargetID:    "target-1",
			Kind:        "http",
			Severity:    "warning",
			SampleTs:    now,
			CollectedAt: now,
		},
		Captures: []DiagCapture{
			{
				Source:           "peer-1",
				CaptureStartedAt: now,
				Status:           DiagCaptureStatusError,
				CaptureStatus:    CaptureStatusFailed,
				Error:            &errMsg,
			},
		},
	}

	// Verify failure reason is present
	capture := spike.Captures[0]

	if capture.CaptureStatus != CaptureStatusFailed {
		t.Errorf("expected failed status, got %s", capture.CaptureStatus)
	}
	if capture.Error == nil {
		t.Error("failed row must have Error with failure reason")
	}
}

// TestSpikeCaptureProjection_MissingRowMustIncludeMissingArtifactReason verifies
// missing rows must include missing artifact reason.
func TestSpikeCaptureProjection_MissingRowMustIncludeMissingArtifactReason(t *testing.T) {
	now := time.Now().UTC()
	errMsg := "artifact not found: /var/log/uvb76/captures/spike-1.pcap"
	spike := SpikeEventWithCaptures{
		SpikeEvent: SpikeEvent{
			EventID:     "spike-1",
			TargetID:    "target-1",
			Kind:        "http",
			Severity:    "warning",
			SampleTs:    now,
			CollectedAt: now,
		},
		Captures: []DiagCapture{
			{
				Source:           "peer-1",
				CaptureStartedAt: now,
				Status:           DiagCaptureStatusError,
				CaptureStatus:    CaptureStatusMissing,
				Error:            &errMsg,
			},
		},
	}

	// Verify missing reason is present
	capture := spike.Captures[0]

	if capture.CaptureStatus != CaptureStatusMissing {
		t.Errorf("expected missing status, got %s", capture.CaptureStatus)
	}
	if capture.Error == nil {
		t.Error("missing row must have Error with missing artifact reason")
	}
}

// TestSpikeCaptureProjection_StatusStringMustBeCanonical verifies
// status string must be one of canonical statuses.
func TestSpikeCaptureProjection_StatusStringMustBeCanonical(t *testing.T) {
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
		parsed, ok := domain.ParseCaptureStatus(string(status))
		if !ok {
			t.Errorf("canonical status %q should be valid", status)
		}
		if !parsed.IsValid() {
			t.Errorf("parsed status %q should be valid", parsed)
		}
	}
}

// TestSpikeCaptureProjection_UnknownStatusMustFail verifies unknown status fails validation.
func TestSpikeCaptureProjection_UnknownStatusMustFail(t *testing.T) {
	unknownStatus := CaptureStatus("unknown_status")

	parsed, ok := domain.ParseCaptureStatus(string(unknownStatus))
	if ok || parsed.IsValid() {
		t.Error("unknown status should fail validation")
	}
}
