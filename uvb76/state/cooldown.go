package state

import (
	"time"
)

// CaptureCooldownAnchor records provenance of the successful capture that started cooldown.
// INVARIANT: suppressed event requires anchor spike to be retained/visible.
type CaptureCooldownAnchor struct {
	AnchorEventID         string     `json:"anchor_event_id,omitempty"`
	AnchorCaptureID       string     `json:"anchor_capture_id,omitempty"` // May differ from AnchorEventID
	AnchorTargetID        string     `json:"anchor_target_id,omitempty"`
	AnchorProbeKind       string     `json:"anchor_probe_kind,omitempty"` // http/icmp
	AnchorSource          string     `json:"anchor_source,omitempty"`
	AnchorUpdatedByStatus string     `json:"anchor_updated_by_status,omitempty"` // captured, diag_capture_success
	AnchorCreatedAt       time.Time  `json:"anchor_created_at,omitempty"`
	AnchorCompletedAt     *time.Time `json:"anchor_completed_at,omitempty"`
	CreatedFrom           string     `json:"created_from,omitempty"` // diag_capture_success, startup_warmup, etc
	IsWarmupAnchor        bool       `json:"is_warmup_anchor,omitempty"`
	AnchorRetained        bool       `json:"anchor_retained,omitempty"` // Validated at decision time
}

// CaptureCooldownDecision is authoritative cooldown decision for skip/capture and metadata.
type CaptureCooldownDecision struct {
	IsInCooldown               bool                  `json:"is_in_cooldown"`
	CooldownKey                string                `json:"cooldown_key"`
	DecisionNowAt              time.Time             `json:"decision_now_at"`
	LastSuccessfulCaptureAt     time.Time             `json:"last_successful_capture_at"`
	NextCaptureEligibleAt      time.Time             `json:"next_capture_eligible_at"`
	RemainingCooldownMs        int64                 `json:"remaining_cooldown_ms"`
	CooldownSeconds            int                   `json:"cooldown_seconds"`
	SkippedAttemptUpdatesCooldown bool               `json:"skipped_attempt_updates_cooldown"`
	Anchor                    *CaptureCooldownAnchor `json:"anchor,omitempty"`
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
		// Still in cooldown - include anchor provenance
		decision.IsInCooldown = true
		decision.RemainingCooldownMs = (cooldownDuration - elapsed).Milliseconds()
		
		// Include anchor provenance from the parallel map
		if anchor, ok := cs.lastCaptureAnchor[peerName]; ok {
			anchorCopy := anchor
			decision.Anchor = &anchorCopy
		}
	}

	return decision
}

// AnchorVisibilityReason constants define why an anchor is or is not visible.
const (
	AnchorVisibilityReasonRetained          = "retained_visible"
	AnchorVisibilityReasonPinned            = "pinned_anchor"          // Anchor pinned for suppressed row visibility
	AnchorVisibilityReasonEmbedded          = "embedded_summary"       // Anchor summary embedded (anchor evicted but row shows proof)
	AnchorVisibilityReasonDegraded         = "degraded"               // Suppression degraded: no anchor visible AND no summary
	AnchorVisibilityReasonFilterWindow      = "outside_filter_window"
	AnchorVisibilityReasonEvictedRetention   = "evicted_from_retention"
	AnchorVisibilityReasonTargetFilter       = "outside_target_filter"
	AnchorVisibilityReasonProbeFilter        = "outside_probe_filter"
	AnchorVisibilityReasonStartupWarmup      = "startup_warmup_anchor"
	AnchorVisibilityReasonUnknown           = "unknown_anchor_not_visible"
)

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
		AnchorVisibilityReason: AnchorVisibilityReasonRetained,
	}

	// Include anchor provenance if available
	if decision.Anchor != nil {
		info.AnchorCaptureID = decision.Anchor.AnchorCaptureID
		info.AnchorTargetID = decision.Anchor.AnchorTargetID
		info.AnchorProbeKind = decision.Anchor.AnchorProbeKind
		info.AnchorSource = decision.Anchor.AnchorSource
		if !decision.Anchor.AnchorCreatedAt.IsZero() {
			info.AnchorCreatedAt = &decision.Anchor.AnchorCreatedAt
		}
		info.AnchorUpdatedByStatus = decision.Anchor.AnchorUpdatedByStatus
		info.CreatedFrom = decision.Anchor.CreatedFrom
		
		// Copy warmup indicator
		if decision.Anchor.IsWarmupAnchor {
			info.IsWarmupAnchor = true
		}
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
	// AnchorVisible is a legacy alias for AnchorTimelineVisible.
	// It indicates whether the anchor spike row is visible in the user-visible timeline.
	// - true: anchor spike is in the timeline (visible in same-probe OR cross-probe set)
	// - false: anchor spike is not visible (evicted, outside window, filtered out)
	// For granular visibility, use AnchorArtifactVisible and AnchorTimelineVisible.
	AnchorVisible bool `json:"anchor_visible"`
	// AnchorArtifactVisible indicates whether the capture artifact exists/retrievable in the store.
	// This is true when a successful capture record exists for the anchor event.
	// NOTE: This field is NOT used for suppression validity - only AnchorTimelineVisible is.
	AnchorArtifactVisible bool `json:"anchor_artifact_visible"`
	// AnchorTimelineVisible indicates whether the anchor event row is present in the
	// user-visible mixed HTTP/ICMP timeline response.
	// - true: anchor spike is in the timeline (visible in same-probe OR cross-probe set)
	// - false: anchor spike is not in timeline (evicted, outside window, filtered out)
	// SUPPRESSION INVARIANT: skipped_cooldown requires anchor_timeline_visible=true
	// OR anchor_event_summary is embedded (for response-time anchor expiry scenarios).
	// When false without anchor_event_summary, the row must be degraded.
	AnchorTimelineVisible bool `json:"anchor_timeline_visible"`
	// AnchorVisibilityReason explains why anchor_visible is false.
	// Empty when anchor_visible is true.
	// Values: "retained_visible", "pinned_anchor", "embedded_summary", "degraded",
	//         "outside_filter_window", "evicted_from_retention",
	//         "outside_target_filter", "outside_probe_filter", "startup_warmup_anchor",
	//         "unknown_anchor_not_visible"
	AnchorVisibilityReason string `json:"anchor_visibility_reason,omitempty"`
	
	// === Provenance Fields ===
	// These fields provide root-cause evidence for the cooldown anchor.
	
	// AnchorCaptureID is the event ID where the anchor capture was recorded.
	AnchorCaptureID string `json:"anchor_capture_id,omitempty"`
	// AnchorTargetID is the target ID of the successful anchor capture.
	AnchorTargetID string `json:"anchor_target_id,omitempty"`
	// AnchorProbeKind is the probe kind (http/icmp) of the anchor capture.
	AnchorProbeKind string `json:"anchor_probe_kind,omitempty"`
	// AnchorSource is the diagnostic peer/source that performed the anchor capture.
	AnchorSource string `json:"anchor_source,omitempty"`

	// SuppressedProbeKind is the probe kind of the spike that was suppressed by cooldown.
	// This is used by the UI to detect and explain cross-probe suppression scenarios.
	SuppressedProbeKind string `json:"suppressed_probe_kind,omitempty"`

	// IsCrossProbeSuppression indicates the suppressed spike's probe kind differs from
	// the anchor capture's probe kind.
	IsCrossProbeSuppression bool `json:"is_cross_probe_suppression,omitempty"`
	// AnchorCreatedAt is when the anchor capture was started.
	AnchorCreatedAt *time.Time `json:"anchor_created_at,omitempty"`
	// AnchorUpdatedByStatus describes what status updated this anchor.
	// Values: "captured", "diag_capture_success"
	AnchorUpdatedByStatus string `json:"anchor_updated_by_status,omitempty"`
	// CreatedFrom describes the path that created this anchor.
	// Values: "diag_capture_success", "startup_warmup", "api_injection", "test_helper"
	CreatedFrom string `json:"created_from,omitempty"`
	// IsWarmupAnchor indicates this anchor was created during startup/warmup.
	IsWarmupAnchor bool `json:"is_warmup_anchor,omitempty"`

	// === Anchor Event Summary (for Response-Time Anchor Expiry) ===
	// When the anchor spike is not visible in the timeline (evicted, outside window, etc.)
	// but suppression decision was valid, we embed the anchor event summary so the UI
	// can display auditable suppression context without requiring the anchor row.
	// This prevents "ghost suppression" where suppressed rows appear without any visible anchor.
	AnchorEventSummary *AnchorEventSummary `json:"anchor_event_summary,omitempty"`
	// SuppressionDegraded indicates the suppression row should be rendered as degraded/warning
	// instead of normal suppressed status, because:
	// - anchor is not visible AND
	// - anchor_event_summary could not be embedded
	// The UI should show this row differently to indicate incomplete provenance.
	SuppressionDegraded bool `json:"suppression_degraded,omitempty"`
	// SuppressionDegradedReason explains why the suppression is degraded.
	// Only set when SuppressionDegraded is true.
	// Values: "anchor_not_visible_at_response_time", "anchor_summary_unavailable"
	SuppressionDegradedReason string `json:"suppression_degraded_reason,omitempty"`
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

// SetLastCaptureAnchor sets the full provenance anchor for a peer.
// This is intended for test use only - production code should use AddCapture.
func (cs *CaptureStore) SetLastCaptureAnchor(peerName string, anchor CaptureCooldownAnchor) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.lastCapture[peerName] = anchor.AnchorCreatedAt
	cs.lastCaptureAnchor[peerName] = anchor
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
