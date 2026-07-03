package domain

// CaptureStatus represents the derived capture status for UI display.
// This is a closed enum - only the defined constants are valid.
//
// Note: "none" is included because it exists in the existing state.CaptureStatus
// contract (state/diag_capture.go CaptureStatusNone). The enum reflects the full
// set of statuses that the frontend/API may emit. Renaming would require a
// separate migration ACT.
type CaptureStatus string

const (
	CaptureStatusCaptured         CaptureStatus = "captured"
	CaptureStatusSkippedCooldown CaptureStatus = "skipped_cooldown"
	CaptureStatusFailed          CaptureStatus = "failed"
	CaptureStatusDisabled        CaptureStatus = "disabled"
	CaptureStatusNotConfigured   CaptureStatus = "not_configured"
	CaptureStatusNotAttempted    CaptureStatus = "not_attempted"
	CaptureStatusInProgress      CaptureStatus = "in_progress"
	CaptureStatusNone            CaptureStatus = "none"
	CaptureStatusMissing         CaptureStatus = "missing"
)

// ParseCaptureStatus parses a raw string into a CaptureStatus.
// Returns false for unknown or empty status strings.
func ParseCaptureStatus(raw string) (CaptureStatus, bool) {
	switch CaptureStatus(raw) {
	case CaptureStatusCaptured,
		CaptureStatusSkippedCooldown,
		CaptureStatusFailed,
		CaptureStatusDisabled,
		CaptureStatusNotConfigured,
		CaptureStatusNotAttempted,
		CaptureStatusInProgress,
		CaptureStatusNone,
		CaptureStatusMissing:
		return CaptureStatus(raw), true
	default:
		return "", false
	}
}

// IsValid returns true if the status is a known canonical value.
func (s CaptureStatus) IsValid() bool {
	_, ok := ParseCaptureStatus(string(s))
	return ok
}
