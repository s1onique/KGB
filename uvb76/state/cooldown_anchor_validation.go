package state

import (
	"fmt"
	"time"
)

// AnchorRetainedValidator validates anchor spike exists and is captured in timeline.
// Returns validation result with details about why anchor is/isn't valid.
type AnchorValidationResult struct {
	IsValid   bool   // true if anchor can justify suppression
	Reason   string // Human-readable reason
	IsRetained bool // Spike is in timeline
	IsCaptured bool // Spike has successful capture status
}

// AnchorRetainedValidator validates anchor spike exists in timeline.
// Returns true if anchor is retained, false if evicted/missing/not visible.
type AnchorRetainedValidator func(anchor CaptureCooldownAnchor) AnchorValidationResult

// EvaluateCooldownWithAnchorValidation computes cooldown decision with anchor retention validation.
// FIXED: Validator is called OUTSIDE the mutex to avoid deadlock risk.
//
// Key invariants enforced:
// 1. Cooldown anchor state must only suppress if the anchor spike is visible/retained
// 2. If the anchor spike is evicted or missing, the cooldown state is cleared
// 3. Cold start: empty runtime cooldown state must not suppress the first spike
// 4. lastCapture without lastCaptureAnchor: clears cooldown, never suppresses
func (cs *CaptureStore) EvaluateCooldownWithAnchorValidation(
	now time.Time,
	peerName string,
	cooldownSeconds int,
	anchorValidator AnchorRetainedValidator,
) CaptureCooldownDecision {
	// Step 1: Snapshot state under lock
	cs.mu.Lock()
	lastTime, hasLastCapture := cs.lastCapture[peerName]
	anchor, hasAnchor := cs.lastCaptureAnchor[peerName]
	cs.mu.Unlock()

	decision := CaptureCooldownDecision{
		CooldownKey:               peerName,
		DecisionNowAt:             now,
		CooldownSeconds:           cooldownSeconds,
		SkippedAttemptUpdatesCooldown: false,
	}

	// No prior successful capture - not in cooldown
	if !hasLastCapture || lastTime.IsZero() {
		decision.IsInCooldown = false
		return decision
	}

	// FIXED: lastCapture without lastCaptureAnchor = clear and allow capture
	// This prevents suppression when anchor metadata is incomplete
	if !hasAnchor {
		cs.clearCooldownIfSame(peerName, lastTime, "")
		decision.IsInCooldown = false
		return decision
	}

	// Step 2: Validate anchor OUTSIDE mutex (avoids deadlock)
	// When validator is nil, fall back to basic cooldown behavior (backward compatible)
	var validationResult *AnchorValidationResult
	if anchorValidator != nil {
		result := anchorValidator(anchor)
		validationResult = &result
		
		// FIXED: Anchor not valid = clear and allow capture
		if !result.IsValid {
			cs.clearCooldownIfSame(peerName, lastTime, anchor.AnchorCaptureID)
			decision.IsInCooldown = false
			return decision
		}
	}

	// Step 3: Proceed with cooldown evaluation
	decision.LastSuccessfulCaptureAt = lastTime
	eligibleAt := lastTime.Add(time.Duration(cooldownSeconds) * time.Second)
	decision.NextCaptureEligibleAt = eligibleAt

	elapsed := now.Sub(lastTime)
	cooldownDuration := time.Duration(cooldownSeconds) * time.Second

	if elapsed >= cooldownDuration {
		decision.IsInCooldown = false
		decision.RemainingCooldownMs = 0
	} else {
		decision.IsInCooldown = true
		decision.RemainingCooldownMs = (cooldownDuration - elapsed).Milliseconds()
		// Include anchor metadata
		anchorCopy := anchor
		// Mark retained only if validator ran and confirmed retention
		if validationResult != nil {
			anchorCopy.AnchorRetained = validationResult.IsRetained
		}
		decision.Anchor = &anchorCopy
	}

	return decision
}

// clearCooldownIfSame clears cooldown for a peer if the state matches.
// Uses compare-and-delete semantics to avoid racing newer anchors.
func (cs *CaptureStore) clearCooldownIfSame(peerName string, expectedTime time.Time, expectedAnchorID string) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Verify state hasn't changed (compare-and-delete)
	currentTime, hasTime := cs.lastCapture[peerName]
	if !hasTime || !currentTime.Equal(expectedTime) {
		return false // State changed, don't clear
	}

	if expectedAnchorID != "" {
		currentAnchor, hasAnchor := cs.lastCaptureAnchor[peerName]
		if !hasAnchor || currentAnchor.AnchorCaptureID != expectedAnchorID {
			return false // Anchor changed, don't clear
		}
	}

	delete(cs.lastCapture, peerName)
	delete(cs.lastCaptureAnchor, peerName)
	return true
}

// ClearStaleCooldownForPeer clears cooldown state for a peer if the anchor is no longer retained.
// This should be called during cold start or when a peer reconnects after a long gap.
// FIXED: Validator is called OUTSIDE the mutex to avoid deadlock risk.
func (cs *CaptureStore) ClearStaleCooldownForPeer(peerName string, anchorValidator AnchorRetainedValidator) bool {
	// Step 1: Snapshot anchor under lock
	cs.mu.Lock()
	anchor, exists := cs.lastCaptureAnchor[peerName]
	lastTime, hasTime := cs.lastCapture[peerName]
	cs.mu.Unlock()

	if !exists || !hasTime {
		return false // No cooldown state
	}

	// Step 2: Validate OUTSIDE mutex (avoids deadlock)
	if anchorValidator != nil {
		result := anchorValidator(anchor)
		if !result.IsValid {
			// Step 3: Use compare-and-delete to clear only if still the same
			cs.clearCooldownIfSame(peerName, lastTime, anchor.AnchorCaptureID)
			return true
		}
	}

	return false
}

// ClearAllStaleCooldowns clears all cooldown entries where anchors are no longer retained.
// Returns the count of cleared cooldowns.
// FIXED: Validator is called OUTSIDE the mutex to avoid deadlock risk.
func (cs *CaptureStore) ClearAllStaleCooldowns(anchorValidator AnchorRetainedValidator) int {
	// Step 1: Snapshot all anchors under lock
	cs.mu.Lock()
	type cooldownEntry struct {
		peerName  string
		anchor    CaptureCooldownAnchor
		lastTime  time.Time
	}
	var entries []cooldownEntry
	for peerName, anchor := range cs.lastCaptureAnchor {
		lastTime, hasTime := cs.lastCapture[peerName]
		if hasTime {
			entries = append(entries, cooldownEntry{peerName: peerName, anchor: anchor, lastTime: lastTime})
		}
	}
	cs.mu.Unlock()

	if anchorValidator == nil || len(entries) == 0 {
		return 0
	}

	// Step 2: Validate each anchor OUTSIDE mutex (avoids deadlock)
	type staleEntry struct {
		peerName       string
		lastTime       time.Time
		anchorCaptureID string
	}
	var staleEntries []staleEntry
	for _, entry := range entries {
		result := anchorValidator(entry.anchor)
		if !result.IsValid {
			staleEntries = append(staleEntries, staleEntry{
				peerName:        entry.peerName,
				lastTime:        entry.lastTime,
				anchorCaptureID: entry.anchor.AnchorCaptureID,
			})
		}
	}

	// Step 3: Clear stale entries using compare-and-delete
	cleared := 0
	for _, stale := range staleEntries {
		if cs.clearCooldownIfSame(stale.peerName, stale.lastTime, stale.anchorCaptureID) {
			cleared++
		}
	}

	return cleared
}

// =============================================================================
// Anchor Proof for Timeline API
// =============================================================================

// AnchorProof provides proof that the anchor spike was validated as retained.
type AnchorProof struct {
	// SpikeEventID is the event ID of the anchor spike.
	SpikeEventID string `json:"spike_event_id"`
	// IsRetained indicates whether the spike is retained in the timeline.
	IsRetained bool `json:"is_retained"`
	// RetentionReason explains why the spike is or is not retained.
	RetentionReason string `json:"retention_reason,omitempty"`
	// CapturedAt is the capture timestamp of the anchor.
	CapturedAt time.Time `json:"captured_at,omitempty"`
}

// BuildCooldownInfoWithAnchorProof creates cooldown info with explicit anchor proof.
// This variant should be used when the caller has validated anchor retention
// and wants to include the validation result in the response.
func BuildCooldownInfoWithAnchorProof(decision CaptureCooldownDecision, source string, anchorProof *AnchorProof) *CaptureCooldownInfo {
	info := BuildCooldownInfoFromDecision(decision, source)
	if info != nil && anchorProof != nil {
		info.CooldownSourceSpikeID = anchorProof.SpikeEventID
		info.CooldownSourceRetained = anchorProof.IsRetained
	}
	return info
}

// =============================================================================
// Anchor Retention Validator
// =============================================================================

// ValidateAnchorWithCaptureStatus validates that an anchor's spike is retained AND
// the anchor capture has successful status (DiagCaptureStatusOK).
//
// This is the production validator that enforces:
// 1. Anchor spike exists in timeline (wasn't evicted)
// 2. Anchor capture has successful status (DiagCaptureStatusOK)
//
// Suppression requires BOTH conditions to be true.
func ValidateAnchorWithCaptureStatus(
	spikeStoreGetter func(targetID, probeKind string) []SpikeEvent,
	captureStoreGetter func(eventID string) []DiagCapture,
) AnchorRetainedValidator {
	return func(anchor CaptureCooldownAnchor) AnchorValidationResult {
		if anchor.AnchorCaptureID == "" {
			return AnchorValidationResult{IsValid: false, Reason: "missing_anchor_capture_id"}
		}
		if anchor.AnchorTargetID == "" {
			return AnchorValidationResult{IsValid: false, Reason: "missing_anchor_target_id"}
		}

		// Step 1: Check if anchor spike is retained in timeline
		spikes := spikeStoreGetter(anchor.AnchorTargetID, anchor.AnchorProbeKind)
		var spikeFound bool
		for _, spike := range spikes {
			if spike.EventID == anchor.AnchorCaptureID || spike.EventID == anchor.AnchorEventID {
				spikeFound = true
				break
			}
		}
		if !spikeFound {
			return AnchorValidationResult{IsValid: false, Reason: "anchor_spike_evicted", IsRetained: false}
		}

		// Step 2: Check if anchor capture has successful status
		captures := captureStoreGetter(anchor.AnchorCaptureID)
		var captureOK bool
		for _, capture := range captures {
			// Valid capture: Status is OK AND NOT suppressed by cooldown
			// (suppressed captures were intentionally skipped, not actually performed)
			if capture.Status == DiagCaptureStatusOK && !capture.SuppressedByCooldown {
				captureOK = true
				break
			}
		}
		// FIXED: Capture not OK = invalid, allow capture (don't suppress)
		if !captureOK {
			return AnchorValidationResult{
				IsValid: false, Reason: "anchor_spike_retained_capture_not_ok",
				IsRetained: true, IsCaptured: false,
			}
		}

		return AnchorValidationResult{
			IsValid: true, Reason: "anchor_spike_retained_capture_ok",
			IsRetained: true, IsCaptured: true,
		}
	}
}

// ValidateAnchorAgainstTimeline validates that an anchor's spike is still in the timeline.
// Returns AnchorValidationResult proving the anchor is a valid captured spike.
//
// NOTE: This is a legacy validator that only checks spike retention.
// Use ValidateAnchorWithCaptureStatus for production code to also verify capture status.
func ValidateAnchorAgainstTimeline(
	spikeStoreGetter func(targetID, probeKind string) []SpikeEvent,
) AnchorRetainedValidator {
	return func(anchor CaptureCooldownAnchor) AnchorValidationResult {
		if anchor.AnchorCaptureID == "" {
			return AnchorValidationResult{IsValid: false, Reason: "missing_anchor_capture_id"}
		}
		if anchor.AnchorTargetID == "" {
			return AnchorValidationResult{IsValid: false, Reason: "missing_anchor_target_id"}
		}
		spikes := spikeStoreGetter(anchor.AnchorTargetID, anchor.AnchorProbeKind)
		for _, spike := range spikes {
			if spike.EventID == anchor.AnchorCaptureID || spike.EventID == anchor.AnchorEventID {
				return AnchorValidationResult{
					IsValid: true, Reason: "anchor_spike_retained",
					IsRetained: true, IsCaptured: true,
				}
			}
		}
		return AnchorValidationResult{IsValid: false, Reason: "anchor_spike_evicted"}
	}
}

// String implements fmt.Stringer for CaptureCooldownAnchor.
func (a CaptureCooldownAnchor) String() string {
	return fmt.Sprintf("CaptureCooldownAnchor{EventID:%s CaptureID:%s TargetID:%s ProbeKind:%s Source:%s CreatedAt:%v Retained:%v}",
		a.AnchorEventID, a.AnchorCaptureID, a.AnchorTargetID, a.AnchorProbeKind, a.AnchorSource, a.AnchorCreatedAt, a.AnchorRetained)
}
