package state

import (
	"testing"
	"time"
)

// TestAnchorTimelineVisible_ArtifactExistsButTimelineNotVisible tests the core bug:
// suppression should NOT be valid when anchor_artifact_visible=true but anchor_timeline_visible=false.
// This is the "row-visibility ghost suppression" bug scenario.
func TestAnchorTimelineVisible_ArtifactExistsButTimelineNotVisible(t *testing.T) {
	const cooldownSeconds = 90
	peerName := "peer-ghost-artifact"
	store := NewCaptureStore()
	t0 := time.Date(2026, 6, 23, 14, 0, 0, 0, time.UTC)

	// Setup anchor with capture record that exists (artifact visible)
	store.SetLastCaptureAnchor(peerName, CaptureCooldownAnchor{
		AnchorCaptureID:       "icmp-capture-001",
		AnchorTargetID:        "target-1",
		AnchorProbeKind:       "icmp",
		AnchorSource:          peerName,
		AnchorCreatedAt:       t0,
		AnchorUpdatedByStatus: "captured",
	})

	// Simulate the bug scenario: spike store returns empty (timeline NOT visible)
	// but capture store has the capture record (artifact IS visible)
	emptySpikeStore := func(targetID, probeKind string) []SpikeEvent {
		return []SpikeEvent{} // Anchor spike NOT in timeline
	}
	captureStoreWithRecord := func(eventID string) []DiagCapture {
		return []DiagCapture{{
			Status: DiagCaptureStatusOK,
			CaptureStatus: CaptureStatusCaptured,
		}}
	}
	validator := ValidateAnchorWithCaptureStatus(emptySpikeStore, captureStoreWithRecord)

	// At t0+5s, cooldown is still active
	t5 := t0.Add(5 * time.Second)
	decision := store.EvaluateCooldownWithAnchorValidation(t5, peerName, cooldownSeconds, validator)

	// CRITICAL: Even though artifact exists, suppression must be rejected
	// because anchor is NOT timeline-visible (the spike row is missing from the UI)
	if decision.IsInCooldown {
		t.Error("BUG REPRODUCED: suppression allowed with artifact_visible=true but timeline_visible=false. " +
			"This is ghost suppression - anchor row is absent from mixed HTTP/ICMP timeline.")
	}
}

// TestAnchorTimelineVisible_BothArtifactAndTimelineVisible tests that suppression
// IS valid when BOTH artifact and timeline are visible.
func TestAnchorTimelineVisible_BothArtifactAndTimelineVisible(t *testing.T) {
	const cooldownSeconds = 90
	peerName := "peer-valid"
	store := NewCaptureStore()
	t0 := time.Date(2026, 6, 23, 14, 0, 0, 0, time.UTC)

	// Setup anchor
	store.SetLastCaptureAnchor(peerName, CaptureCooldownAnchor{
		AnchorCaptureID:       "icmp-capture-001",
		AnchorTargetID:        "target-1",
		AnchorProbeKind:       "icmp",
		AnchorSource:          peerName,
		AnchorCreatedAt:       t0,
		AnchorUpdatedByStatus: "captured",
	})

	// Spike store has the anchor (timeline visible)
	spikeStoreWithAnchor := func(targetID, probeKind string) []SpikeEvent {
		return []SpikeEvent{{EventID: "icmp-capture-001"}}
	}
	captureStoreWithRecord := func(eventID string) []DiagCapture {
		return []DiagCapture{{Status: DiagCaptureStatusOK}}
	}
	validator := ValidateAnchorWithCaptureStatus(spikeStoreWithAnchor, captureStoreWithRecord)

	t5 := t0.Add(5 * time.Second)
	decision := store.EvaluateCooldownWithAnchorValidation(t5, peerName, cooldownSeconds, validator)

	// Suppression should be allowed when both artifact and timeline are visible
	if !decision.IsInCooldown {
		t.Error("Suppression should be allowed when both artifact and timeline are visible")
	}
}

// TestAnchorTimelineVisible_CrossProbeMissingFromAnchorSet tests cross-probe suppression
// where the ICMP anchor is missing from the ICMP spike set (cross-probe timeline visibility).
func TestAnchorTimelineVisible_CrossProbeMissingFromAnchorSet(t *testing.T) {
	const cooldownSeconds = 90
	peerName := "peer-cross-probe"
	store := NewCaptureStore()
	t0 := time.Date(2026, 6, 23, 14, 0, 0, 0, time.UTC)

	// HTTP spike suppressed by ICMP anchor
	store.SetLastCaptureAnchor(peerName, CaptureCooldownAnchor{
		AnchorCaptureID:       "icmp-capture-001",
		AnchorTargetID:        "target-1",
		AnchorProbeKind:       "icmp", // ICMP anchor
		AnchorSource:          peerName,
		AnchorCreatedAt:       t0,
		AnchorUpdatedByStatus: "captured",
	})

	// HTTP spikes exist, but ICMP anchor is NOT in ICMP spike set
	httpSpikeStore := func(targetID, probeKind string) []SpikeEvent {
		if probeKind == "http" {
			return []SpikeEvent{{EventID: "http-spike-001"}}
		}
		return []SpikeEvent{} // ICMP anchor missing from ICMP set!
	}
	captureStoreWithRecord := func(eventID string) []DiagCapture {
		return []DiagCapture{{Status: DiagCaptureStatusOK}}
	}
	validator := ValidateAnchorWithCaptureStatus(httpSpikeStore, captureStoreWithRecord)

	t5 := t0.Add(5 * time.Second)
	decision := store.EvaluateCooldownWithAnchorValidation(t5, peerName, cooldownSeconds, validator)

	// CRITICAL: Cross-probe suppression must be rejected when anchor is missing from anchor probe set
	if decision.IsInCooldown {
		t.Error("Cross-probe suppression must reject when ICMP anchor is missing from ICMP spike set")
	}
}

// TestAnchorTimelineVisible_ValidatesCaptureStatus tests that the validator
// properly checks capture status (DiagCaptureStatusOK, not suppressed).
func TestAnchorTimelineVisible_ValidatesCaptureStatus(t *testing.T) {
	peerName := "peer-capture-status"
	store := NewCaptureStore()
	t0 := time.Date(2026, 6, 23, 14, 0, 0, 0, time.UTC)

	store.SetLastCaptureAnchor(peerName, CaptureCooldownAnchor{
		AnchorCaptureID:       "capture-001",
		AnchorTargetID:        "target-1",
		AnchorProbeKind:       "http",
		AnchorSource:          peerName,
		AnchorCreatedAt:       t0,
	})

	spikeStore := func(targetID, probeKind string) []SpikeEvent {
		return []SpikeEvent{{EventID: "capture-001"}}
	}

	// Capture exists but is SKIPPED (suppressed by cooldown itself - invalid)
	captureStoreSkipped := func(eventID string) []DiagCapture {
		return []DiagCapture{{Status: DiagCaptureStatusOK, SuppressedByCooldown: true}}
	}
	validator := ValidateAnchorWithCaptureStatus(spikeStore, captureStoreSkipped)

	result := validator(CaptureCooldownAnchor{
		AnchorCaptureID: "capture-001",
		AnchorTargetID:  "target-1",
		AnchorProbeKind: "http",
	})

	if result.IsValid {
		t.Error("Anchor with skipped-cooldown capture should be invalid")
	}
	if result.Reason != "anchor_spike_retained_capture_not_ok" {
		t.Errorf("Expected reason 'anchor_spike_retained_capture_not_ok', got '%s'", result.Reason)
	}
}

// TestBuildCooldownInfo_SetsAnchorArtifactVisible documents the expected behavior
// of BuildCooldownInfoFromDecision regarding the new visibility fields.
func TestBuildCooldownInfo_SetsAnchorArtifactVisible(t *testing.T) {
	peerName := "peer-test"
	store := NewCaptureStore()
	t0 := time.Date(2026, 6, 23, 14, 0, 0, 0, time.UTC)

	store.SetLastCaptureAnchor(peerName, CaptureCooldownAnchor{
		AnchorCaptureID:       "capture-001",
		AnchorTargetID:        "target-1",
		AnchorProbeKind:       "http",
		AnchorSource:          peerName,
		AnchorCreatedAt:       t0,
		AnchorUpdatedByStatus: "captured",
	})

	t5 := t0.Add(5 * time.Second)
	decision := store.EvaluateCooldown(t5, peerName, 90)
	info := BuildCooldownInfoFromDecision(decision, peerName)

	if info == nil {
		t.Fatal("expected cooldown info")
	}

	// BuildCooldownInfoFromDecision sets default AnchorVisible=true when lastCapture exists
	// The API layer must override based on timeline visibility
	if !info.AnchorVisible {
		t.Error("BuildCooldownInfoFromDecision should default AnchorVisible=true when anchor exists")
	}

	// AnchorArtifactVisible should reflect that capture record exists
	// (This is set by API layer, not BuildCooldownInfoFromDecision)
	// Note: The default behavior sets AnchorVisible=true, API layer must override
	// to set anchor_timeline_visible based on actual spike visibility
}
