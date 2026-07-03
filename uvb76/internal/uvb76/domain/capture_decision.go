package domain

import (
	"time"
)

// CaptureDecisionKind classifies the outcome of a capture decision.
type CaptureDecisionKind string

const (
	CaptureDecisionRun          CaptureDecisionKind = "run"
	CaptureDecisionSkipCooldown CaptureDecisionKind = "skip_cooldown"
	CaptureDecisionDisabled      CaptureDecisionKind = "disabled"
	CaptureDecisionNotConfigured CaptureDecisionKind = "not_configured"
)

// CaptureDecision represents a pure capture decision with reason.
type CaptureDecision struct {
	Kind   CaptureDecisionKind
	Reason string
}

// DiagnosticCaptureConfig holds the configuration for diagnostic capture decisions.
type DiagnosticCaptureConfig struct {
	Enabled    bool
	Configured bool
	Cooldown   time.Duration
}

// DecideCapture is a pure function that determines whether a diagnostic capture should run.
// It does not perform any I/O, logging, or state mutation.
//
// Cooldown applies only after a prior successful capture. A zero lastSuccessfulCapture
// means capture has not succeeded yet and must not suppress the first diagnostic capture.
func DecideCapture(
	now time.Time,
	lastSuccessfulCapture time.Time,
	cfg DiagnosticCaptureConfig,
) CaptureDecision {
	if !cfg.Enabled {
		return CaptureDecision{
			Kind:   CaptureDecisionDisabled,
			Reason: "diagnostic_capture_disabled",
		}
	}

	if !cfg.Configured {
		return CaptureDecision{
			Kind:   CaptureDecisionNotConfigured,
			Reason: "diagnostic_capture_not_configured",
		}
	}

	// Cooldown suppresses capture ONLY after a prior successful capture.
	// Zero lastSuccessfulCapture means no prior success - allow capture.
	if !lastSuccessfulCapture.IsZero() && cfg.Cooldown > 0 && now.Sub(lastSuccessfulCapture) < cfg.Cooldown {
		return CaptureDecision{
			Kind:   CaptureDecisionSkipCooldown,
			Reason: "capture_cooldown_active",
		}
	}

	return CaptureDecision{Kind: CaptureDecisionRun}
}
