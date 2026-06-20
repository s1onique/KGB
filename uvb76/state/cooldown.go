package state

import (
	"time"
)

// =============================================================================
// CaptureCooldownDecision — Authoritative Cooldown Decision
// =============================================================================

// CaptureCooldownDecision represents the authoritative cooldown decision at a specific moment.
// This is the single source of truth used for BOTH:
//   - The capture/skip decision in TriggerCapture
//   - The cooldown_info metadata in skipped captures
//
// This ensures the exported metadata exactly matches the decision logic.
type CaptureCooldownDecision struct {
	// IsInCooldown indicates whether the capture should be skipped due to cooldown.
	IsInCooldown bool `json:"is_in_cooldown"`
	// CooldownKey is the key used for cooldown state (typically peer/source name).
	CooldownKey string `json:"cooldown_key"`
	// DecisionNowAt is the timestamp when this decision was made.
	DecisionNowAt time.Time `json:"decision_now_at"`
	// LastSuccessfulCaptureAt is the timestamp of the last successful capture for this key.
	// Zero time if no prior capture exists.
	LastSuccessfulCaptureAt time.Time `json:"last_successful_capture_at"`
	// NextCaptureEligibleAt is when the next capture will be eligible.
	// Zero time if not in cooldown.
	NextCaptureEligibleAt time.Time `json:"next_capture_eligible_at"`
	// RemainingCooldownMs is the remaining cooldown in milliseconds.
	// 0 if not in cooldown or cooldown has expired.
	RemainingCooldownMs int64 `json:"remaining_cooldown_ms"`
	// CooldownSeconds is the configured cooldown duration in seconds.
	CooldownSeconds int `json:"cooldown_seconds"`
	// SkippedAttemptUpdatesCooldown documents the cooldown semantics:
	// - true: skipped attempts extend the cooldown window
	// - false: only successful captures update cooldown
	// This is the authoritative semantics documented in tests.
	SkippedAttemptUpdatesCooldown bool `json:"skipped_attempt_updates_cooldown"`
}

// EvaluateCooldown computes the authoritative cooldown decision at the given time.
// This is the single shared function used by both:
//   - TriggerCapture to decide whether to skip a capture
//   - recordSuppressedCapture to generate cooldown_info metadata
//
// The returned decision is used for both the skip/capture decision AND metadata,
// ensuring they are always consistent.
//
// Semantics: Only successful captures update lastSuccessfulCaptureAt.
// Skipped cooldown attempts do NOT extend the cooldown window.
func (cs *CaptureStore) EvaluateCooldown(now time.Time, peerName string, cooldownSeconds int) CaptureCooldownDecision {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	decision := CaptureCooldownDecision{
		CooldownKey:               peerName,
		DecisionNowAt:             now,
		CooldownSeconds:           cooldownSeconds,
		SkippedAttemptUpdatesCooldown: false, // Preferred semantics: skipped attempts do NOT extend cooldown
	}

	lastTime, exists := cs.lastCapture[peerName]
	if !exists || lastTime.IsZero() {
		// No prior successful capture - not in cooldown
		decision.IsInCooldown = false
		return decision
	}

	decision.LastSuccessfulCaptureAt = lastTime
	eligibleAt := lastTime.Add(time.Duration(cooldownSeconds) * time.Second)
	decision.NextCaptureEligibleAt = eligibleAt

	elapsed := now.Sub(lastTime)
	cooldownDuration := time.Duration(cooldownSeconds) * time.Second

	if elapsed >= cooldownDuration {
		// Cooldown expired
		decision.IsInCooldown = false
		decision.RemainingCooldownMs = 0
	} else {
		// Still in cooldown
		decision.IsInCooldown = true
		decision.RemainingCooldownMs = (cooldownDuration - elapsed).Milliseconds()
	}

	return decision
}

// BuildCooldownInfoFromDecision creates a CaptureCooldownInfo from a decision.
// This ensures cooldown_info exactly matches the decision used for skip/capture.
//
// Anchor visibility defaults: When lastSuccessfulCaptureAt exists, the anchor capture
// is assumed visible (AnchorVisible=true, reason="retained_visible"). The API layer
// can override these defaults when the anchor spike is outside the response scope.
func BuildCooldownInfoFromDecision(decision CaptureCooldownDecision, source string) *CaptureCooldownInfo {
	if !decision.IsInCooldown {
		return nil
	}

	info := &CaptureCooldownInfo{
		Scope:                         "per_diagnostic_peer",
		LastSuccessfulCaptureAt:        &decision.LastSuccessfulCaptureAt,
		NextCaptureEligibleAt:          &decision.NextCaptureEligibleAt,
		CooldownSeconds:               decision.CooldownSeconds,
		RemainingCooldownMs:           &decision.RemainingCooldownMs,
		SkippedAttemptUpdatesCooldown: decision.SkippedAttemptUpdatesCooldown,
		// CaptureKey is the peer.Name used for cooldown decision
		CaptureKey:     decision.CooldownKey,
		DecisionNowAt:  &decision.DecisionNowAt,
		// Anchor visibility defaults: assume anchor is visible since lastCapture exists.
		// API layer should override if anchor spike is outside response scope.
		AnchorVisible:           !decision.LastSuccessfulCaptureAt.IsZero(),
		AnchorVisibilityReason: "retained_visible",
	}

	return info
}

// =============================================================================
// CaptureCooldownInfo — Cooldown Metadata for UI
// =============================================================================

// CaptureCooldownInfo holds metadata about why a spike was suppressed by cooldown.
// This provides auditable context for UI display.
// IMPORTANT: This is computed from CaptureCooldownDecision to ensure consistency.
type CaptureCooldownInfo struct {
	// Scope indicates the cooldown scope: "global", "per_target", "per_probe", or "per_target_and_probe".
	Scope string `json:"cooldown_scope"`
	// LastSuccessfulCaptureAt is the timestamp of the successful capture that started the cooldown.
	LastSuccessfulCaptureAt *time.Time `json:"last_successful_capture_at,omitempty"`
	// NextCaptureEligibleAt is when the next capture will be eligible.
	NextCaptureEligibleAt *time.Time `json:"next_capture_eligible_at,omitempty"`
	// CooldownSourceSpikeID is the event ID of the spike that caused the cooldown (if retained).
	CooldownSourceSpikeID string `json:"cooldown_source_spike_id,omitempty"`
	// CooldownSourceRetained indicates if the source spike is still retained.
	CooldownSourceRetained bool `json:"cooldown_source_retained"`
	// CooldownSourceTargetID is the target ID of the source capture.
	CooldownSourceTargetID string `json:"cooldown_source_target_id,omitempty"`
	// CooldownSeconds is the configured cooldown duration.
	CooldownSeconds int `json:"cooldown_seconds"`
	// RemainingCooldownMs is the remaining cooldown in milliseconds.
	// Added for explicit observability - computed from NextCaptureEligibleAt - now.
	RemainingCooldownMs *int64 `json:"remaining_cooldown_ms,omitempty"`
	// SkippedAttemptUpdatesCooldown documents the cooldown semantics.
	// - true: skipped attempts extend the cooldown window
	// - false: only successful captures update cooldown (preferred semantics)
	SkippedAttemptUpdatesCooldown bool `json:"skipped_attempt_updates_cooldown"`
	// CaptureKey is the key used for cooldown state (typically peer/source name).
	// Useful for debugging which identity triggered the cooldown.
	CaptureKey string `json:"cooldown_key,omitempty"`
	// DecisionNowAt is the timestamp when the cooldown decision was made.
	// This proves the cooldown_info was computed at the same moment as the decision.
	DecisionNowAt *time.Time `json:"decision_now_at,omitempty"`
	// AnchorVisible indicates whether the successful capture anchor is visible in the current response scope.
	// - true: anchor spike is retained and visible in current API response
	// - false: anchor spike is not visible (outside filter window, evicted, or suppressed)
	AnchorVisible bool `json:"anchor_visible"`
	// AnchorVisibilityReason explains why anchor_visible is false.
	// Empty when anchor_visible is true.
	// Values: "retained_visible", "outside_filter_window", "evicted_from_retention", "suppressed_cooldown"
	AnchorVisibilityReason string `json:"anchor_visibility_reason,omitempty"`
}

// =============================================================================
// SpikeCaptureInfo — Capture-derived Protection Info
// =============================================================================

// SpikeCaptureInfo holds capture-derived protection info for a spike.
type SpikeCaptureInfo struct {
	CaptureStatus CaptureStatus        `json:"capture_status"`
	CaptureExists bool                 `json:"capture_exists"` // true if artifact exists
	IsProtected   bool                 `json:"is_protected"`   // true if spike must not be purged
	CooldownInfo  *CaptureCooldownInfo `json:"cooldown_info,omitempty"` // cooldown metadata if suppressed
}

// GetCaptureInfo returns derived capture protection info for a spike.
// A spike is protected if it has a capture artifact that exists or capture is in progress.
func (cs *CaptureStore) GetCaptureInfo(eventID string, isInFlight bool) SpikeCaptureInfo {
	// Check in-flight first - this takes precedence
	if isInFlight {
		return SpikeCaptureInfo{
			CaptureStatus: CaptureStatusInProgress,
			CaptureExists: false,
			IsProtected:   true, // Don't race cleanup against in-flight captures
		}
	}

	captures := cs.GetCaptures(eventID)
	if len(captures) == 0 {
		return SpikeCaptureInfo{
			CaptureStatus: CaptureStatusNone,
			CaptureExists: false,
			IsProtected:   false,
		}
	}

	// Take the most recent capture
	capture := captures[len(captures)-1]

	// Check suppressed by cooldown
	if capture.SuppressedByCooldown {
		return SpikeCaptureInfo{
			CaptureStatus: CaptureStatusSkippedCooldown,
			CaptureExists: false,
			IsProtected:   false, // Suppressed captures are purge-eligible
			CooldownInfo:  capture.CooldownInfo,
		}
	}

	// Check capture status
	switch capture.Status {
	case DiagCaptureStatusOK:
		// ok status with no artifact means partial/success without data
		// Still protected if status is ok (capture was attempted)
		return SpikeCaptureInfo{
			CaptureStatus: CaptureStatusCaptured,
			CaptureExists: capture.NetworkDiag != nil,
			IsProtected:   true,
		}
	case DiagCaptureStatusTimeout:
		// Timeout with artifact is protected, without is purge-eligible
		return SpikeCaptureInfo{
			CaptureStatus: CaptureStatusFailed,
			CaptureExists: capture.NetworkDiag != nil,
			IsProtected:   capture.NetworkDiag != nil,
		}
	case DiagCaptureStatusError:
		// Error with artifact is protected, without is purge-eligible
		return SpikeCaptureInfo{
			CaptureStatus: CaptureStatusFailed,
			CaptureExists: capture.NetworkDiag != nil,
			IsProtected:   capture.NetworkDiag != nil,
		}
	case DiagCaptureStatusDisabled:
		return SpikeCaptureInfo{
			CaptureStatus: CaptureStatusDisabled,
			CaptureExists: false,
			IsProtected:   false,
		}
	case DiagCaptureStatusNoPeerMapping:
		return SpikeCaptureInfo{
			CaptureStatus: CaptureStatusNotConfigured,
			CaptureExists: false,
			IsProtected:   false,
		}
	default:
		return SpikeCaptureInfo{
			CaptureStatus: CaptureStatusNotAttempted,
			CaptureExists: false,
			IsProtected:   false,
		}
	}
}

// =============================================================================
// GetProtectionInfo — Spike Protection Status
// =============================================================================

// GetProtectionInfo returns protection info for a spike event.
// This method checks in-flight captures internally.
func (cs *CaptureStore) GetProtectionInfo(eventID string) (isProtected bool, hasArtifact bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	// Check in-flight first - in-flight spikes are protected
	if cs.inFlight[eventID] {
		return true, false
	}

	captures := cs.captures[eventID]
	if len(captures) == 0 {
		return false, false
	}

	// Take the most recent capture
	capture := captures[len(captures)-1]

	// Check suppressed by cooldown - purge eligible
	// hasCapture=false because suppression means this capture didn't complete;
	// any artifact present was from a previous capture, not this suppressed attempt
	if capture.SuppressedByCooldown {
		return false, false
	}

	// Check capture status
	switch capture.Status {
	case DiagCaptureStatusOK:
		// ok status means capture was attempted - protected
		return true, capture.NetworkDiag != nil
	case DiagCaptureStatusTimeout:
		// Timeout with artifact is protected, without is purge-eligible
		return capture.NetworkDiag != nil, capture.NetworkDiag != nil
	case DiagCaptureStatusError:
		// Error with artifact is protected, without is purge-eligible
		return capture.NetworkDiag != nil, capture.NetworkDiag != nil
	case DiagCaptureStatusDisabled, DiagCaptureStatusNoPeerMapping:
		return false, capture.NetworkDiag != nil
	default:
		return false, capture.NetworkDiag != nil
	}
}

// =============================================================================
// Test Helpers
// =============================================================================

// SetLastCapture sets the lastCapture timestamp for a peer.
// This is intended for test use only - production code should use AddCapture.
func (cs *CaptureStore) SetLastCapture(peerName string, t time.Time) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.lastCapture[peerName] = t
}

// =============================================================================
// Legacy Compatibility
// =============================================================================

// GetCooldownInfoForSkippedCapture returns cooldown info for a skipped capture.
// This is a convenience wrapper around EvaluateCooldown + BuildCooldownInfoFromDecision.
// DEPRECATED: Use EvaluateCooldown and BuildCooldownInfoFromDecision directly
// to ensure consistency between decision and metadata.
func (cs *CaptureStore) GetCooldownInfoForSkippedCapture(now time.Time, peerName string, cooldownSeconds int) *CaptureCooldownInfo {
	decision := cs.EvaluateCooldown(now, peerName, cooldownSeconds)
	return BuildCooldownInfoFromDecision(decision, peerName)
}
