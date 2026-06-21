// Package verifier provides self-test fixtures for the ICMP OS ping soak lab.
package verifier

import (
	"testing"
)

func TestVerifyDaemonStatusAttempsZero(t *testing.T) {
	result := VerifyDaemonStatus(Fixtures["attempts_zero"].RawStatus)
	
	if result.OK != Fixtures["attempts_zero"].ExpectedOK {
		t.Errorf("expected OK=%v, got %v", Fixtures["attempts_zero"].ExpectedOK, result.OK)
	}
	if result.ICMPExercised != Fixtures["attempts_zero"].ExpectedExercised {
		t.Errorf("expected ICMPExercised=%v, got %v", Fixtures["attempts_zero"].ExpectedExercised, result.ICMPExercised)
	}
	if result.EvidenceSource != "daemon-status" {
		t.Errorf("expected EvidenceSource='daemon-status', got %q", result.EvidenceSource)
	}
	if result.Reason != Fixtures["attempts_zero"].ExpectedReason {
		t.Errorf("expected Reason=%q, got %q", Fixtures["attempts_zero"].ExpectedReason, result.Reason)
	}
	if result.DaemonAttempts != Fixtures["attempts_zero"].ExpectedAttempts {
		t.Errorf("expected DaemonAttempts=%d, got %d", Fixtures["attempts_zero"].ExpectedAttempts, result.DaemonAttempts)
	}
}

func TestVerifyDaemonStatusAttempsPositive(t *testing.T) {
	result := VerifyDaemonStatus(Fixtures["attempts_positive"].RawStatus)
	
	if result.OK != Fixtures["attempts_positive"].ExpectedOK {
		t.Errorf("expected OK=%v, got %v", Fixtures["attempts_positive"].ExpectedOK, result.OK)
	}
	if result.ICMPExercised != Fixtures["attempts_positive"].ExpectedExercised {
		t.Errorf("expected ICMPExercised=%v, got %v", Fixtures["attempts_positive"].ExpectedExercised, result.ICMPExercised)
	}
	if result.EvidenceSource != "daemon-status" {
		t.Errorf("expected EvidenceSource='daemon-status', got %q", result.EvidenceSource)
	}
	if result.Reason != Fixtures["attempts_positive"].ExpectedReason {
		t.Errorf("expected Reason=%q, got %q", Fixtures["attempts_positive"].ExpectedReason, result.Reason)
	}
	if result.DaemonAttempts != Fixtures["attempts_positive"].ExpectedAttempts {
		t.Errorf("expected DaemonAttempts=%d, got %d", Fixtures["attempts_positive"].ExpectedAttempts, result.DaemonAttempts)
	}
}

func TestVerifyDaemonStatusMissingTelemetry(t *testing.T) {
	result := VerifyDaemonStatus(Fixtures["missing_telemetry"].RawStatus)
	
	if result.OK != Fixtures["missing_telemetry"].ExpectedOK {
		t.Errorf("expected OK=%v, got %v", Fixtures["missing_telemetry"].ExpectedOK, result.OK)
	}
	if result.ICMPExercised != Fixtures["missing_telemetry"].ExpectedExercised {
		t.Errorf("expected ICMPExercised=%v, got %v", Fixtures["missing_telemetry"].ExpectedExercised, result.ICMPExercised)
	}
	if result.EvidenceSource != "daemon-status" {
		t.Errorf("expected EvidenceSource='daemon-status', got %q", result.EvidenceSource)
	}
	if result.Reason != Fixtures["missing_telemetry"].ExpectedReason {
		t.Errorf("expected Reason=%q, got %q", Fixtures["missing_telemetry"].ExpectedReason, result.Reason)
	}
}

func TestVerifyDaemonStatusMalformedJSON(t *testing.T) {
	result := VerifyDaemonStatus(Fixtures["malformed_json"].RawStatus)
	
	if result.OK != Fixtures["malformed_json"].ExpectedOK {
		t.Errorf("expected OK=%v, got %v", Fixtures["malformed_json"].ExpectedOK, result.OK)
	}
	if result.ICMPExercised != Fixtures["malformed_json"].ExpectedExercised {
		t.Errorf("expected ICMPExercised=%v, got %v", Fixtures["malformed_json"].ExpectedExercised, result.ICMPExercised)
	}
	if result.Reason != Fixtures["malformed_json"].ExpectedReason {
		t.Errorf("expected Reason=%q, got %q", Fixtures["malformed_json"].ExpectedReason, result.Reason)
	}
}

func TestVerifyDaemonStatusEmptyResponse(t *testing.T) {
	result := VerifyDaemonStatus(Fixtures["empty_response"].RawStatus)
	
	if result.OK != Fixtures["empty_response"].ExpectedOK {
		t.Errorf("expected OK=%v, got %v", Fixtures["empty_response"].ExpectedOK, result.OK)
	}
	if result.ICMPExercised != Fixtures["empty_response"].ExpectedExercised {
		t.Errorf("expected ICMPExercised=%v, got %v", Fixtures["empty_response"].ExpectedExercised, result.ICMPExercised)
	}
	if result.Reason != Fixtures["empty_response"].ExpectedReason {
		t.Errorf("expected Reason=%q, got %q", Fixtures["empty_response"].ExpectedReason, result.Reason)
	}
}

func TestVerifyDaemonStatusRejectsLabProcessCounters(t *testing.T) {
	// This test verifies that the verifier does NOT accept lab-process counters.
	// The only authoritative source is daemon-sourced telemetry.
	
	// Simulate a status where lab-process counters would show success
	// but daemon has no attempts (should fail)
	statusWithZeroDaemonAttempts := `{"started_at":"2024-01-01T00:00:00Z","icmp_os_ping":{"enabled":true,"attempts":0,"successes":0,"failures":0,"max_concurrent":1}}`
	
	result := VerifyDaemonStatus(statusWithZeroDaemonAttempts)
	
	// The lab must fail when daemon attempts == 0, regardless of any other evidence
	if result.OK {
		t.Error("expected verifier to reject zero daemon attempts, but it passed")
	}
	if result.ICMPExercised {
		t.Error("expected ICMPExercised=false when daemon attempts == 0")
	}
	if result.EvidenceSource != "daemon-status" {
		t.Errorf("expected EvidenceSource='daemon-status', got %q", result.EvidenceSource)
	}
}

func TestVerifyDaemonStatusAcceptsPositiveDaemonAttempts(t *testing.T) {
	// This test verifies that positive daemon attempts are accepted.
	// Successes are not required - only attempts prove the path was exercised.
	
	statusWithPositiveAttempts := `{"started_at":"2024-01-01T00:00:00Z","icmp_os_ping":{"enabled":true,"attempts":3,"successes":0,"failures":3,"max_concurrent":1}}`
	
	result := VerifyDaemonStatus(statusWithPositiveAttempts)
	
	// All attempts failed, but the path was still exercised
	if !result.OK {
		t.Error("expected verifier to accept positive daemon attempts even when all failed")
	}
	if !result.ICMPExercised {
		t.Error("expected ICMPExercised=true when daemon attempts > 0")
	}
	if result.DaemonAttempts != 3 {
		t.Errorf("expected DaemonAttempts=3, got %d", result.DaemonAttempts)
	}
}

func TestVerifyDaemonStatusEvidenceSource(t *testing.T) {
	// Verify that evidence_source is always "daemon-status" when ICMP telemetry is present
	fixtures := []string{"attempts_zero", "attempts_positive"}
	
	for _, fixtureName := range fixtures {
		fixture := Fixtures[fixtureName]
		result := VerifyDaemonStatus(fixture.RawStatus)
		
		if result.EvidenceSource != "daemon-status" {
			t.Errorf("[%s] expected EvidenceSource='daemon-status', got %q", fixtureName, result.EvidenceSource)
		}
	}
}
