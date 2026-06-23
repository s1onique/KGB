package server

import (
	"sort"
	"time"

	"github.com/s1onique/KGB/uvb76/state"
)

// anchorDependency contains information about a suppressed spike's anchor dependency.
type anchorDependency struct {
	suppressedEventID string
	anchorCaptureID   string
	anchorEventID     string
	anchorTargetID    string
	anchorProbeKind   string
}

// findAnchorSpike searches for the anchor spike event in the all-spikes store.
// Returns the spike event and its captures if found.
func findAnchorSpike(
	anchorCaptureID string,
	anchorTargetID string,
	anchorProbeKind string,
	allSpikesGetter func(targetID, kind string, limit int) []state.SpikeEvent,
	captureStore *state.CaptureStore,
) *state.SpikeEventWithCaptures {
	// Search in the anchor's target/probe kind
	spikes := allSpikesGetter(anchorTargetID, anchorProbeKind, 0)
	for _, spike := range spikes {
		if spike.EventID == anchorCaptureID || spike.EventID == anchorEventID(anchorCaptureID) {
			captures := captureStore.GetCaptures(spike.EventID)
			return &state.SpikeEventWithCaptures{
				SpikeEvent: spike,
				Captures:   captures,
			}
		}
	}
	return nil
}

// sortPinnedAnchorsByTimestamp sorts the pinned anchors slice by SampleTs descending.
// This ensures stable ordering for tests and consistent API response.
func sortPinnedAnchorsByTimestamp(anchors []state.SpikeEventWithCaptures) {
	sort.Slice(anchors, func(i, j int) bool {
		return anchors[i].SampleTs.After(anchors[j].SampleTs)
	})
}

// anchorEventID extracts the event ID from a capture ID or returns the capture ID as-is.
// In this system, capture IDs are typically the same as event IDs.
func anchorEventID(captureID string) string {
	return captureID
}

// buildAnchorEventSummary creates an anchor event summary from the anchor spike and capture.
// This is used when the anchor spike cannot be pinned but we want to embed evidence.
func buildAnchorEventSummary(spike *state.SpikeEvent, capture *state.DiagCapture) *state.AnchorEventSummary {
	if spike == nil {
		return nil
	}
	summary := &state.AnchorEventSummary{
		EventID:    spike.EventID,
		ProbeKind: spike.Kind,
		Severity:  spike.Severity,
		LatencyMs: spike.LatencyMs,
		SampleTs:  spike.SampleTs,
	}
	if capture != nil {
		summary.CaptureID = spike.EventID
		summary.CaptureStatus = capture.CaptureStatus
		summary.Source = capture.Source
		summary.CapturedAt = capture.CaptureStartedAt
	}
	return summary
}

// processSuppressedCaptureAnchorVisibility determines anchor visibility for a suppressed capture.
// It handles three cases:
// 1. Anchor is visible in timeline -> retained_visible
// 2. Anchor was pinned into response -> pinned_anchor + embedded_summary
// 3. Anchor is not visible AND not pinned -> embedded_summary OR degraded
//
// IMPORTANT: For cross-probe suppression, anchor_visible=true only if the anchor row is
// in visibleCrossProbeSpikes or in anchorsToPin. This ensures the invariant is based on
// FINAL RESPONSE VISIBILITY, not "exists somewhere in retained state."
func processSuppressedCaptureAnchorVisibility(
	cooldownInfo *state.CaptureCooldownInfo,
	queryProbeKind string,
	targetID string,
	visibleAnchorTimestamps map[time.Time]bool,
	visibleCapturedEventIDs map[string]bool,
	anchorsToPin map[string]*state.SpikeEventWithCaptures,
	allSpikesGetter func(targetID, kind string, limit int) []state.SpikeEvent,
	captureStore *state.CaptureStore,
	// visibleCrossProbeSpikes: visible spikes from the anchor's probe kind (for cross-probe visibility check)
	visibleCrossProbeSpikes []state.SpikeEvent,
) *state.CaptureCooldownInfo {
	if cooldownInfo == nil || cooldownInfo.LastSuccessfulCaptureAt == nil {
		return cooldownInfo
	}

	anchorTime := *cooldownInfo.LastSuccessfulCaptureAt
	anchorCaptureID := cooldownInfo.AnchorCaptureID
	anchorProbeKind := cooldownInfo.AnchorProbeKind
	anchorTargetID := cooldownInfo.AnchorTargetID

	var anchorTimelineVisible bool
	var visibilityReason string

	if cooldownInfo.IsCrossProbeSuppression {
		// Cross-probe suppression: anchor_visible=true ONLY if anchor is in visibleCrossProbeSpikes
		// or in anchorsToPin. This ensures invariant is based on FINAL RESPONSE VISIBILITY,
		// not "exists somewhere in retained state."
		if anchorProbeKind == "" {
			anchorTimelineVisible = false
			visibilityReason = "anchor_probe_kind_missing"
		} else if anchorVisibleInVisibleSpikes(anchorTime, visibleCrossProbeSpikes, captureStore) {
			// Anchor IS in the visible cross-probe window
			anchorTimelineVisible = true
			visibilityReason = state.AnchorVisibilityReasonRetained
		} else if _, pinned := anchorsToPin[anchorCaptureID]; pinned {
			// Anchor was pinned into the response
			anchorTimelineVisible = true
			visibilityReason = state.AnchorVisibilityReasonPinned
		} else {
			// Anchor is not visible in response and not pinned
			anchorTimelineVisible = false
			visibilityReason = state.AnchorVisibilityReasonFilterWindow
		}
	} else {
		// Same-probe suppression: check if anchor is visible in this probe kind's timeline
		if visibleAnchorTimestamps[anchorTime] || visibleCapturedEventIDs[anchorCaptureID] {
			anchorTimelineVisible = true
			visibilityReason = state.AnchorVisibilityReasonRetained
		} else if _, pinned := anchorsToPin[anchorCaptureID]; pinned {
			// Anchor was pinned into the response
			anchorTimelineVisible = true
			visibilityReason = state.AnchorVisibilityReasonPinned
		} else {
			// Anchor is not visible and not pinned - this is response-time anchor expiry
			anchorTimelineVisible = false
			visibilityReason = state.AnchorVisibilityReasonFilterWindow
		}
	}

	// Update cooldown info with visibility
	cooldownInfo.AnchorTimelineVisible = anchorTimelineVisible
	cooldownInfo.AnchorVisibilityReason = visibilityReason
	cooldownInfo.AnchorArtifactVisible = true
	cooldownInfo.AnchorVisible = anchorTimelineVisible

	// Handle anchor summary embedding for non-visible anchors
	if !anchorTimelineVisible {
		// Try to embed anchor event summary
		if anchorProbeKind == "" {
			anchorProbeKind = queryProbeKind
		}
		if anchorTargetID == "" {
			anchorTargetID = targetID
		}

		anchorSpike := findAnchorSpike(
			anchorCaptureID,
			anchorTargetID,
			anchorProbeKind,
			allSpikesGetter,
			captureStore,
		)

		if anchorSpike != nil {
			// Find the captured capture record
			var anchorCapture *state.DiagCapture
			for _, c := range anchorSpike.Captures {
				if c.CaptureStatus == state.CaptureStatusCaptured {
					anchorCapture = &c
					break
				}
			}
			cooldownInfo.AnchorEventSummary = buildAnchorEventSummary(&anchorSpike.SpikeEvent, anchorCapture)
			cooldownInfo.AnchorVisibilityReason = state.AnchorVisibilityReasonEmbedded
		} else {
			// Cannot embed summary - mark as degraded
			cooldownInfo.SuppressionDegraded = true
			cooldownInfo.SuppressionDegradedReason = "anchor_not_visible_at_response_time"
			cooldownInfo.AnchorVisibilityReason = state.AnchorVisibilityReasonDegraded
		}
	}

	return cooldownInfo
}

// anchorVisibleInProbeKindStatic checks if an anchor is visible in a specific probe kind's spike set.
// Falls back to using the query's targetID if anchorTargetID is empty.
// NOTE: This checks ALL retained spikes. Use anchorVisibleInVisibleSpikes for response-time checks.
func anchorVisibleInProbeKindStatic(
	probeKind string,
	anchorTargetID string,
	anchorTime time.Time,
	spikesGetter func(targetID, kind string, limit int) []state.SpikeEvent,
	captureStore *state.CaptureStore,
) bool {
	// Fall back to using the query's target if anchorTargetID is empty
	// This handles legacy/test scenarios where AnchorTargetID wasn't set
	targetID := anchorTargetID
	spikes := spikesGetter(targetID, probeKind, 0)
	for _, spike := range spikes {
		captures := captureStore.GetCaptures(spike.EventID)
		for _, capture := range captures {
			if capture.CaptureStatus == state.CaptureStatusCaptured && capture.CaptureStartedAt.Equal(anchorTime) {
				return true
			}
		}
	}
	return false
}

// anchorVisibleInVisibleSpikes checks if an anchor is in the VISIBLE spike window.
// This is used for cross-probe suppression to ensure anchor_visible=true only means
// the anchor is in the FINAL RESPONSE VISIBILITY, not just "exists somewhere in retained state."
func anchorVisibleInVisibleSpikes(
	anchorTime time.Time,
	visibleSpikes []state.SpikeEvent,
	captureStore *state.CaptureStore,
) bool {
	for _, spike := range visibleSpikes {
		captures := captureStore.GetCaptures(spike.EventID)
		for _, capture := range captures {
			if capture.CaptureStatus == state.CaptureStatusCaptured && capture.CaptureStartedAt.Equal(anchorTime) {
				return true
			}
		}
	}
	return false
}
