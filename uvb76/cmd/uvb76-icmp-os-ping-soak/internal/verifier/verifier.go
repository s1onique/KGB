// Package verifier provides self-test fixtures for the ICMP OS ping soak lab.
package verifier

import (
	"encoding/json"
	"fmt"
)

// VerifierResult represents the result of verifying daemon-sourced ICMP telemetry.
type VerifierResult struct {
	OK             bool   `json:"ok"`
	ICMPExercised  bool   `json:"icmp_exercised"`
	EvidenceSource string `json:"evidence_source"`
	Reason         string `json:"reason"`
	DaemonAttempts uint64 `json:"daemon_attempts,omitempty"`
}

// DaemonStatus represents the expected structure of the daemon status API.
type DaemonStatus struct {
	StartedAt  string              `json:"started_at"`
	ICMPOSPing *ICMPOSPingTelemetry `json:"icmp_os_ping,omitempty"`
}

// ICMPOSPingTelemetry represents the ICMP OS ping telemetry from the daemon.
type ICMPOSPingTelemetry struct {
	Enabled       bool   `json:"enabled"`
	Attempts      uint64 `json:"attempts"`
	Successes     uint64 `json:"successes"`
	Failures      uint64 `json:"failures"`
	LastError     string `json:"last_error,omitempty"`
	MaxConcurrent int    `json:"max_concurrent"`
}

// VerifyDaemonStatus verifies daemon-sourced ICMP telemetry and returns a VerifierResult.
// This is the canonical verification function used by the soak lab.
func VerifyDaemonStatus(rawStatus string) VerifierResult {
	if rawStatus == "" {
		return VerifierResult{
			OK:             false,
			ICMPExercised:  false,
			EvidenceSource: "",
			Reason:         "empty daemon status response",
		}
	}

	var status DaemonStatus
	if err := json.Unmarshal([]byte(rawStatus), &status); err != nil {
		return VerifierResult{
			OK:             false,
			ICMPExercised:  false,
			EvidenceSource: "",
			Reason:         fmt.Sprintf("failed to parse daemon status: %v", err),
		}
	}

	// Check for ICMP telemetry presence
	if status.ICMPOSPing == nil {
		return VerifierResult{
			OK:             false,
			ICMPExercised:  false,
			EvidenceSource: "daemon-status",
			Reason:         "daemon status missing icmp_os_ping telemetry",
			DaemonAttempts: 0,
		}
	}

	// The canonical proof is daemon-sourced attempts > 0
	if status.ICMPOSPing.Attempts > 0 {
		return VerifierResult{
			OK:             true,
			ICMPExercised:  true,
			EvidenceSource: "daemon-status",
			Reason:         fmt.Sprintf("daemon reported %d ICMP OS ping attempts", status.ICMPOSPing.Attempts),
			DaemonAttempts: status.ICMPOSPing.Attempts,
		}
	}

	return VerifierResult{
		OK:             false,
		ICMPExercised:  false,
		EvidenceSource: "daemon-status",
		Reason:         "daemon reported zero ICMP OS ping attempts",
		DaemonAttempts: 0,
	}
}

// Fixtures provides test fixtures for the verifier.
var Fixtures = map[string]struct {
	RawStatus   string
	ExpectedOK  bool
	ExpectedExercised bool
	ExpectedAttempts uint64
	ExpectedReason string
}{
	"attempts_zero": {
		RawStatus: `{"started_at":"2024-01-01T00:00:00Z","icmp_os_ping":{"enabled":true,"attempts":0,"successes":0,"failures":0,"max_concurrent":1}}`,
		ExpectedOK: false,
		ExpectedExercised: false,
		ExpectedAttempts: 0,
		ExpectedReason: "daemon reported zero ICMP OS ping attempts",
	},
	"attempts_positive": {
		RawStatus: `{"started_at":"2024-01-01T00:00:00Z","icmp_os_ping":{"enabled":true,"attempts":5,"successes":4,"failures":1,"max_concurrent":1}}`,
		ExpectedOK: true,
		ExpectedExercised: true,
		ExpectedAttempts: 5,
		ExpectedReason: "daemon reported 5 ICMP OS ping attempts",
	},
	"missing_telemetry": {
		RawStatus: `{"started_at":"2024-01-01T00:00:00Z"}`,
		ExpectedOK: false,
		ExpectedExercised: false,
		ExpectedAttempts: 0,
		ExpectedReason: "daemon status missing icmp_os_ping telemetry",
	},
	"malformed_json": {
		RawStatus: `{"started_at":"2024-01-01T00:00:00Z","icmp_os_ping":{"enabled":true`,
		ExpectedOK: false,
		ExpectedExercised: false,
		ExpectedAttempts: 0,
		ExpectedReason: "failed to parse daemon status: unexpected end of JSON input",
	},
	"empty_response": {
		RawStatus: "",
		ExpectedOK: false,
		ExpectedExercised: false,
		ExpectedAttempts: 0,
		ExpectedReason: "empty daemon status response",
	},
}
