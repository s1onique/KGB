package state

import (
	"testing"
	"time"
)

// Ghost suppression: cooldown suppresses even when anchor spike is evicted.
// Fix: suppression conditional on anchor spike being in retained timeline.

func TestFirstSpikeForPeerIsNeverSuppressed(t *testing.T) {
	const cooldownSeconds = 90
	peerName := "peer-ghost-1"
	store := NewCaptureStore()
	alwaysFalse := func(anchor CaptureCooldownAnchor) AnchorValidationResult {
		return AnchorValidationResult{IsValid: false, Reason: "always_false"}
	}
	now := time.Date(2026, 6, 22, 14, 54, 0, 0, time.UTC)
	decision := store.EvaluateCooldownWithAnchorValidation(now, peerName, cooldownSeconds, alwaysFalse)
	if decision.IsInCooldown {
		t.Error("First spike must NOT be suppressed - cold start must start cold")
	}
}

func TestCooldownSuccessWithoutRetainedAnchorDoesNotSuppress(t *testing.T) {
	const cooldownSeconds = 90
	peerName := "peer-ghost-2"
	store := NewCaptureStore()
	t0 := time.Date(2026, 6, 22, 14, 54, 0, 0, time.UTC)
	store.SetLastCaptureAnchor(peerName, CaptureCooldownAnchor{
		AnchorCaptureID: "capture-evicted-001", AnchorTargetID: "target-1",
		AnchorProbeKind: "http", AnchorSource: peerName, AnchorCreatedAt: t0,
	})
	anchorNotRetained := func(anchor CaptureCooldownAnchor) AnchorValidationResult {
		return AnchorValidationResult{IsValid: false, Reason: "anchor_not_retained"}
	}
	t5 := t0.Add(5 * time.Second)
	decision := store.EvaluateCooldownWithAnchorValidation(t5, peerName, cooldownSeconds, anchorNotRetained)
	if decision.IsInCooldown {
		t.Error("Cooldown must NOT suppress when anchor spike is not retained")
	}
}

func TestStaleAnchorIDClearsCooldownAndAllowsCapture(t *testing.T) {
	const cooldownSeconds = 90
	peerName := "peer-ghost-3"
	store := NewCaptureStore()
	t0 := time.Date(2026, 6, 22, 14, 54, 0, 0, time.UTC)
	store.SetLastCaptureAnchor(peerName, CaptureCooldownAnchor{
		AnchorCaptureID: "capture-evicted-stale", AnchorTargetID: "target-1",
		AnchorProbeKind: "http", AnchorSource: peerName, AnchorCreatedAt: t0,
	})
	emptySpikeStore := func(targetID, probeKind string) []SpikeEvent { return []SpikeEvent{} }
	validator := ValidateAnchorAgainstTimeline(emptySpikeStore)
	t5 := t0.Add(5 * time.Second)
	decision := store.EvaluateCooldownWithAnchorValidation(t5, peerName, cooldownSeconds, validator)
	if decision.IsInCooldown {
		t.Error("Stale anchor ID must clear cooldown")
	}
}

func TestCrossProbeSuppressionRequiresVisibleCapturedAnchor(t *testing.T) {
	const cooldownSeconds = 90
	peerName := "peer-cross-probe"
	store := NewCaptureStore()
	t0 := time.Date(2026, 6, 22, 14, 54, 0, 0, time.UTC)
	store.SetLastCaptureAnchor(peerName, CaptureCooldownAnchor{
		AnchorCaptureID: "icmp-capture-001", AnchorTargetID: "target-1",
		AnchorProbeKind: "icmp", AnchorSource: peerName, AnchorCreatedAt: t0,
	})
	missingICMPSpikeStore := func(targetID, probeKind string) []SpikeEvent {
		return []SpikeEvent{{EventID: "http-spike-001"}}
	}
	validator := ValidateAnchorAgainstTimeline(missingICMPSpikeStore)
	t5 := t0.Add(5 * time.Second)
	decision := store.EvaluateCooldownWithAnchorValidation(t5, peerName, cooldownSeconds, validator)
	if decision.IsInCooldown {
		t.Error("Cross-probe suppression must NOT suppress when anchor is not visible")
	}
}

func TestAnchorRetainedInTimelineAllowsSuppression(t *testing.T) {
	const cooldownSeconds = 90
	peerName := "peer-valid-anchor"
	store := NewCaptureStore()
	t0 := time.Date(2026, 6, 22, 14, 54, 0, 0, time.UTC)
	store.SetLastCaptureAnchor(peerName, CaptureCooldownAnchor{
		AnchorCaptureID: "capture-retained-001", AnchorTargetID: "target-1",
		AnchorProbeKind: "http", AnchorSource: peerName, AnchorCreatedAt: t0,
	})
	retainedSpikeStore := func(targetID, probeKind string) []SpikeEvent {
		return []SpikeEvent{{EventID: "capture-retained-001"}}
	}
	validator := ValidateAnchorAgainstTimeline(retainedSpikeStore)
	t5 := t0.Add(5 * time.Second)
	decision := store.EvaluateCooldownWithAnchorValidation(t5, peerName, cooldownSeconds, validator)
	if !decision.IsInCooldown {
		t.Error("Valid anchor SHOULD suppress")
	}
	if decision.Anchor == nil || !decision.Anchor.AnchorRetained {
		t.Error("Anchor should be marked as retained")
	}
}

func TestSuppressedAttemptDoesNotUpdateAnchorOrExtendCooldown(t *testing.T) {
	const cooldownSeconds = 5
	peerName := "peer-suppress-test"
	store := NewCaptureStore()
	t0 := time.Date(2026, 6, 22, 14, 54, 0, 0, time.UTC)
	store.SetLastCaptureAnchor(peerName, CaptureCooldownAnchor{
		AnchorCaptureID: "capture-001", AnchorTargetID: "target-1",
		AnchorProbeKind: "http", AnchorSource: peerName, AnchorCreatedAt: t0,
	})
	t3 := t0.Add(3 * time.Second)
	suppressedCapture := DiagCapture{
		Source: peerName, CaptureStartedAt: t3, Status: DiagCaptureStatusOK,
		CaptureStatus: CaptureStatusSkippedCooldown, SuppressedByCooldown: true,
	}
	store.AddCapture("spike-suppressed", suppressedCapture)
	lastCapture := store.GetLastCaptureTime(peerName)
	if !lastCapture.Equal(t0) {
		t.Errorf("Suppressed capture must NOT update lastCapture")
	}
	t6 := t0.Add(6 * time.Second)
	decision := store.EvaluateCooldown(t6, peerName, cooldownSeconds)
	if decision.IsInCooldown {
		t.Error("After cooldown expiry, suppression must not extend cooldown")
	}
}

func TestColdStartWithValidatorClearsStaleAnchors(t *testing.T) {
	const cooldownSeconds = 90
	peerName := "peer-cold-start"
	store := NewCaptureStore()
	t0 := time.Date(2026, 6, 22, 14, 54, 0, 0, time.UTC)
	store.SetLastCaptureAnchor(peerName, CaptureCooldownAnchor{
		AnchorCaptureID: "capture-stale-coldstart", AnchorTargetID: "target-1",
		AnchorProbeKind: "http", AnchorSource: peerName, AnchorCreatedAt: t0,
	})
	staleValidator := func(anchor CaptureCooldownAnchor) AnchorValidationResult {
		return AnchorValidationResult{IsValid: false, Reason: "always_invalid"}
	}
	cleared := store.ClearStaleCooldownForPeer(peerName, staleValidator)
	if !cleared {
		t.Error("Stale cooldown should have been cleared")
	}
	lastCapture := store.GetLastCaptureTime(peerName)
	if !lastCapture.IsZero() {
		t.Error("After cold start clear, lastCapture should be zero")
	}
}

func TestClearAllStaleCooldowns(t *testing.T) {
	store := NewCaptureStore()
	peers := []string{"peer-a", "peer-b", "peer-c"}
	for i, peer := range peers {
		t := time.Date(2026, 6, 22, 14, 54, i, 0, time.UTC)
		store.SetLastCaptureAnchor(peer, CaptureCooldownAnchor{
			AnchorCaptureID: "capture-" + peer, AnchorTargetID: "target-1",
			AnchorProbeKind: "http", AnchorSource: peer, AnchorCreatedAt: t,
		})
	}
	allStale := func(anchor CaptureCooldownAnchor) AnchorValidationResult {
		return AnchorValidationResult{IsValid: false, Reason: "always_invalid"}
	}
	cleared := store.ClearAllStaleCooldowns(allStale)
	if cleared != 3 {
		t.Errorf("Expected 3 cooldowns cleared, got %d", cleared)
	}
	for _, peer := range peers {
		lastCapture := store.GetLastCaptureTime(peer)
		if !lastCapture.IsZero() {
			t.Errorf("lastCapture for %s should be zero after clear", peer)
		}
	}
}

func TestValidateAnchorAgainstTimeline_MissingCaptureID(t *testing.T) {
	spikeStore := func(targetID, probeKind string) []SpikeEvent {
		return []SpikeEvent{{EventID: "some-event"}}
	}
	validator := ValidateAnchorAgainstTimeline(spikeStore)
	anchor := CaptureCooldownAnchor{
		AnchorCaptureID: "", AnchorTargetID: "target-1", AnchorProbeKind: "http",
	}
	result := validator(anchor)
	if result.IsValid {
		t.Error("Validator should return invalid for anchor with no capture ID")
	}
}

func TestValidateAnchorAgainstTimeline_MissingTargetID(t *testing.T) {
	spikeStore := func(targetID, probeKind string) []SpikeEvent {
		return []SpikeEvent{{EventID: "some-event"}}
	}
	validator := ValidateAnchorAgainstTimeline(spikeStore)
	anchor := CaptureCooldownAnchor{
		AnchorCaptureID: "capture-001", AnchorTargetID: "", AnchorProbeKind: "http",
	}
	result := validator(anchor)
	if result.IsValid {
		t.Error("Validator should return invalid for anchor with no target ID")
	}
}

func TestEvaluateCooldownWithAnchorValidation_NilValidatorFallback(t *testing.T) {
	const cooldownSeconds = 5
	peerName := "peer-fallback"
	store := NewCaptureStore()
	t0 := time.Date(2026, 6, 22, 14, 54, 0, 0, time.UTC)
	// Set both lastCapture and lastCaptureAnchor to test nil validator fallback.
	// Without anchor, the code clears cooldown regardless of validator (safety default).
	store.SetLastCaptureAnchor(peerName, CaptureCooldownAnchor{
		AnchorCaptureID: "capture-001", AnchorTargetID: "target-1",
		AnchorProbeKind: "http", AnchorSource: peerName, AnchorCreatedAt: t0,
	})
	t3 := t0.Add(3 * time.Second)
	decision := store.EvaluateCooldownWithAnchorValidation(t3, peerName, cooldownSeconds, nil)
	if !decision.IsInCooldown {
		t.Error("Nil validator should fall back to basic cooldown behavior")
	}
}

func TestEvaluateCooldownWithAnchorValidation_AnchorEventIDDifferentFromCaptureID(t *testing.T) {
	const cooldownSeconds = 90
	peerName := "peer-different-ids"
	store := NewCaptureStore()
	t0 := time.Date(2026, 6, 22, 14, 54, 0, 0, time.UTC)
	store.SetLastCaptureAnchor(peerName, CaptureCooldownAnchor{
		AnchorEventID: "spike-event-001", AnchorCaptureID: "capture-record-001",
		AnchorTargetID: "target-1", AnchorProbeKind: "http",
		AnchorSource: peerName, AnchorCreatedAt: t0,
	})
	eventIDSPIkeStore := func(targetID, probeKind string) []SpikeEvent {
		return []SpikeEvent{{EventID: "spike-event-001"}}
	}
	validator := ValidateAnchorAgainstTimeline(eventIDSPIkeStore)
	t5 := t0.Add(5 * time.Second)
	decision := store.EvaluateCooldownWithAnchorValidation(t5, peerName, cooldownSeconds, validator)
	if !decision.IsInCooldown {
		t.Error("Should suppress when anchor found by event ID")
	}
	if decision.Anchor == nil || !decision.Anchor.AnchorRetained {
		t.Error("Anchor should be marked as retained")
	}
}

// TestValidateAnchorWithCaptureStatus_RetainedSpikeMissingCaptureIsInvalid tests that
// a retained spike with missing capture is INVALID for suppression.
func TestValidateAnchorWithCaptureStatus_RetainedSpikeMissingCaptureIsInvalid(t *testing.T) {
	spikeStore := func(targetID, probeKind string) []SpikeEvent {
		return []SpikeEvent{{EventID: "anchor-001"}}
	}
	captureStore := func(eventID string) []DiagCapture {
		return []DiagCapture{} // No capture
	}
	validator := ValidateAnchorWithCaptureStatus(spikeStore, captureStore)
	anchor := CaptureCooldownAnchor{
		AnchorCaptureID: "anchor-001", AnchorTargetID: "target-1", AnchorProbeKind: "http",
	}
	result := validator(anchor)
	if result.IsValid {
		t.Error("Missing capture should make validation invalid")
	}
	if result.Reason != "anchor_spike_retained_capture_not_ok" {
		t.Errorf("Expected reason 'anchor_spike_retained_capture_not_ok', got '%s'", result.Reason)
	}
}

// TestValidateAnchorWithCaptureStatus_RetainedSpikeFailedCaptureIsInvalid tests that
// a retained spike with failed capture is INVALID for suppression.
func TestValidateAnchorWithCaptureStatus_RetainedSpikeFailedCaptureIsInvalid(t *testing.T) {
	spikeStore := func(targetID, probeKind string) []SpikeEvent {
		return []SpikeEvent{{EventID: "anchor-001"}}
	}
	captureStore := func(eventID string) []DiagCapture {
		return []DiagCapture{{Status: DiagCaptureStatusError}}
	}
	validator := ValidateAnchorWithCaptureStatus(spikeStore, captureStore)
	anchor := CaptureCooldownAnchor{
		AnchorCaptureID: "anchor-001", AnchorTargetID: "target-1", AnchorProbeKind: "http",
	}
	result := validator(anchor)
	if result.IsValid {
		t.Error("Failed capture should make validation invalid")
	}
}

// TestValidateAnchorWithCaptureStatus_RetainedSpikeSkippedCooldownCaptureIsInvalid tests that
// a retained spike with skipped-cooldown capture is INVALID for suppression.
func TestValidateAnchorWithCaptureStatus_RetainedSpikeSkippedCooldownCaptureIsInvalid(t *testing.T) {
	spikeStore := func(targetID, probeKind string) []SpikeEvent {
		return []SpikeEvent{{EventID: "anchor-001"}}
	}
	captureStore := func(eventID string) []DiagCapture {
		return []DiagCapture{{Status: DiagCaptureStatusOK, SuppressedByCooldown: true}}
	}
	validator := ValidateAnchorWithCaptureStatus(spikeStore, captureStore)
	anchor := CaptureCooldownAnchor{
		AnchorCaptureID: "anchor-001", AnchorTargetID: "target-1", AnchorProbeKind: "http",
	}
	result := validator(anchor)
	if result.IsValid {
		t.Error("Skipped-cooldown capture should make validation invalid")
	}
}

// TestValidateCooldownWithAnchorValidation_RetainedSpikeButCaptureNotOKDoesNotSuppress tests that
// even with retained spike, if capture is not OK, suppression does NOT happen.
func TestEvaluateCooldownWithAnchorValidation_RetainedSpikeButCaptureNotOKDoesNotSuppress(t *testing.T) {
	const cooldownSeconds = 90
	peerName := "peer-capture-not-ok"
	store := NewCaptureStore()
	t0 := time.Date(2026, 6, 22, 14, 54, 0, 0, time.UTC)
	store.SetLastCaptureAnchor(peerName, CaptureCooldownAnchor{
		AnchorCaptureID: "anchor-001", AnchorTargetID: "target-1",
		AnchorProbeKind: "http", AnchorSource: peerName, AnchorCreatedAt: t0,
	})
	
	// Spike is retained but capture has error status
	spikeStore := func(targetID, probeKind string) []SpikeEvent {
		return []SpikeEvent{{EventID: "anchor-001"}}
	}
	captureStore := func(eventID string) []DiagCapture {
		return []DiagCapture{{Status: DiagCaptureStatusError}}
	}
	validator := ValidateAnchorWithCaptureStatus(spikeStore, captureStore)
	
	t5 := t0.Add(5 * time.Second)
	decision := store.EvaluateCooldownWithAnchorValidation(t5, peerName, cooldownSeconds, validator)
	
	// CRITICAL: Should NOT suppress because capture is not OK
	if decision.IsInCooldown {
		t.Error("Should NOT suppress when anchor spike retained but capture not OK")
	}
}
