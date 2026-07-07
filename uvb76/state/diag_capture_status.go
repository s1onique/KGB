package state

// =============================================================================
// DiagCaptureStatus — Capture Status Constants
// =============================================================================

type DiagCaptureStatus string

const (
	DiagCaptureStatusOK            DiagCaptureStatus = "ok"
	DiagCaptureStatusUnavailable   DiagCaptureStatus = "unavailable"
	DiagCaptureStatusTimeout       DiagCaptureStatus = "timeout"
	DiagCaptureStatusError         DiagCaptureStatus = "error"
	DiagCaptureStatusDisabled      DiagCaptureStatus = "disabled"
	DiagCaptureStatusNoPeerMapping DiagCaptureStatus = "no_peer_mapping"
)

// =============================================================================
// CaptureStatus — Derived Capture Protection Status
// =============================================================================

type CaptureStatus string

const (
	CaptureStatusCaptured         CaptureStatus = "captured"
	CaptureStatusSkippedCooldown  CaptureStatus = "skipped_cooldown"
	CaptureStatusFailed           CaptureStatus = "failed"
	CaptureStatusDisabled         CaptureStatus = "disabled"
	CaptureStatusNotConfigured    CaptureStatus = "not_configured"
	CaptureStatusNotAttempted     CaptureStatus = "not_attempted"
	CaptureStatusNone             CaptureStatus = "none"
	CaptureStatusInProgress       CaptureStatus = "in_progress"
	CaptureStatusMissing          CaptureStatus = "missing"
)

// CanonicalCaptureStatusFromDiagStatus maps a low-level DiagCaptureStatus
// to a canonical CaptureStatus for UI display and projection layers.
//
// This helper is used by:
// - CaptureService: to populate CaptureStatus on all service-created rows
// - API projection: for backward compatibility with legacy rows that lack CaptureStatus
//
// Mapping rules:
//   DiagCaptureStatusOK + hasNetworkDiag -> CaptureStatusCaptured
//   DiagCaptureStatusOK + no NetworkDiag  -> CaptureStatusFailed
//   DiagCaptureStatusError                -> CaptureStatusFailed
//   DiagCaptureStatusTimeout              -> CaptureStatusFailed
//   DiagCaptureStatusUnavailable         -> CaptureStatusNotAttempted
//   DiagCaptureStatusDisabled             -> CaptureStatusDisabled
//   DiagCaptureStatusNoPeerMapping       -> CaptureStatusNotConfigured
func CanonicalCaptureStatusFromDiagStatus(status DiagCaptureStatus, hasNetworkDiag bool) CaptureStatus {
	switch status {
	case DiagCaptureStatusOK:
		if hasNetworkDiag {
			return CaptureStatusCaptured
		}
		return CaptureStatusFailed
	case DiagCaptureStatusError:
		return CaptureStatusFailed
	case DiagCaptureStatusTimeout:
		return CaptureStatusFailed
	case DiagCaptureStatusUnavailable:
		return CaptureStatusNotAttempted
	case DiagCaptureStatusDisabled:
		return CaptureStatusDisabled
	case DiagCaptureStatusNoPeerMapping:
		return CaptureStatusNotConfigured
	default:
		return CaptureStatusFailed
	}
}
