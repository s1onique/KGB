package state

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// =============================================================================
// ACT-UVB76-HULK02: Spike Row Projection JSON Contract Tests
// =============================================================================
//
// These tests verify JSON serialization preserves canonical capture status values.
//
// =============================================================================

// TestSpikeCaptureProjection_JSONSerializationPreservesStatus verifies
// JSON serialization preserves canonical status values.
func TestSpikeCaptureProjection_JSONSerializationPreservesStatus(t *testing.T) {
	now := time.Now().UTC()
	anchorTime := now.Add(-5 * time.Minute)

	tests := []struct {
		name   string
		capture DiagCapture
	}{
		{
			name: "captured_roundtrip",
			capture: DiagCapture{
				Source:           "peer-1",
				CaptureStartedAt: now,
				Status:           DiagCaptureStatusOK,
				CaptureStatus:    CaptureStatusCaptured,
				NetworkDiag:      &NetworkDiagData{},
			},
		},
		{
			name: "skipped_cooldown_roundtrip",
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
					AnchorVisible:           true,
					AnchorVisibilityReason:  AnchorVisibilityReasonRetained,
				},
			},
		},
		{
			name: "failed_roundtrip",
			capture: DiagCapture{
				Source:           "peer-1",
				CaptureStartedAt: now,
				Status:           DiagCaptureStatusError,
				CaptureStatus:    CaptureStatusFailed,
				Error:            stringPtr("connection refused"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Serialize to JSON
			data, err := json.Marshal(tt.capture)
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}

			// Deserialize back
			var result DiagCapture
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			// Verify status preserved
			if result.CaptureStatus != tt.capture.CaptureStatus {
				t.Errorf("status mismatch: got %s, want %s", result.CaptureStatus, tt.capture.CaptureStatus)
			}

			// Verify canonical status
			parsed, ok := domain.ParseCaptureStatus(string(result.CaptureStatus))
			if !ok || !parsed.IsValid() {
				t.Errorf("deserialized status %q is not canonical", result.CaptureStatus)
			}
		})
	}
}
