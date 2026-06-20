package state

import (
	"testing"
	"time"
)

// =============================================================================
// Test A: Successful capture creates provenance
// =============================================================================

// TestProvenance_SuccessfulCaptureCreatesAnchor tests that a successful capture
// creates a provenance-bearing anchor in the lastCaptureAnchor map.
func TestProvenance_SuccessfulCaptureCreatesAnchor(t *testing.T) {
	store := NewCaptureStore()
	peer := "tovarisch-peer"
	eventID := "spike-event-1"
	targetID := "test-target"
	probeKind := "http"

	now := time.Now().UTC()
	finishedAt := now.Add(100 * time.Millisecond)

	// Add a successful capture with provenance
	store.AddCaptureWithProvenance(eventID, DiagCapture{
		Source:              peer,
		CaptureStartedAt:    now,
		CaptureFinishedAt:   &finishedAt,
		Status:             DiagCaptureStatusOK,
		CaptureStatus:       CaptureStatusCaptured,
	}, targetID, probeKind)

	// Verify anchor was created
	anchor := store.GetLastCaptureAnchor(peer)
	if anchor.AnchorCaptureID == "" {
		t.Error("Successful capture should set AnchorCaptureID in provenance")
	}
	if anchor.AnchorCaptureID != eventID {
		t.Errorf("Expected AnchorCaptureID=%q, got %q", eventID, anchor.AnchorCaptureID)
	}
	if anchor.AnchorTargetID != targetID {
		t.Errorf("Expected AnchorTargetID=%q, got %q", targetID, anchor.AnchorTargetID)
	}
	if anchor.AnchorProbeKind != probeKind {
		t.Errorf("Expected AnchorProbeKind=%q, got %q", probeKind, anchor.AnchorProbeKind)
	}
	if anchor.AnchorSource != peer {
		t.Errorf("Expected AnchorSource=%q, got %q", peer, anchor.AnchorSource)
	}
	if !anchor.AnchorCreatedAt.Equal(now) {
		t.Errorf("Expected AnchorCreatedAt=%v, got %v", now, anchor.AnchorCreatedAt)
	}
	if anchor.CreatedFrom != "diag_capture_success" {
		t.Errorf("Expected CreatedFrom=diag_capture_success, got %q", anchor.CreatedFrom)
	}

	// Verify lastCapture timestamp was also updated
	lastCapture := store.GetLastCaptureTime(peer)
	if lastCapture.IsZero() {
		t.Error("Successful capture should update lastCapture timestamp")
	}

	t.Log("PASS: Successful capture creates provenance anchor")
}

// =============================================================================
// Test B: Skipped cooldown references provenance but doesn't advance it
// =============================================================================

// TestProvenance_SkippedCooldownReferencesAnchorButDoesNotAdvance tests that
// a skipped cooldown capture references the anchor but does NOT update it.
func TestProvenance_SkippedCooldownReferencesAnchorButDoesNotAdvance(t *testing.T) {
	store := NewCaptureStore()
	peer := "tovarisch-peer"
	originalEventID := "spike-anchor"

	// Step 1: Create successful anchor capture
	t0 := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	store.AddCaptureWithProvenance(originalEventID, DiagCapture{
		Source:           peer,
		CaptureStartedAt: t0,
		Status:           DiagCaptureStatusOK,
		CaptureStatus:    CaptureStatusCaptured,
	}, "target-1", "http")

	originalTimestamp := store.GetLastCaptureTime(peer)

	// Step 2: Evaluate cooldown at t=30s (inside 90s cooldown)
	t30 := t0.Add(30 * time.Second)
	decision := store.EvaluateCooldown(t30, peer, 90)

	if !decision.IsInCooldown {
		t.Fatal("At t=30s with 90s cooldown, should be in cooldown")
	}

	// Step 3: Verify decision contains anchor provenance
	if decision.Anchor == nil {
		t.Fatal("Decision should contain anchor provenance")
	}
	if decision.Anchor.AnchorCaptureID != originalEventID {
		t.Errorf("Decision anchor should reference original event, got %q", decision.Anchor.AnchorCaptureID)
	}

	// Step 4: Simulate skipped cooldown capture being recorded
	// NOTE: This should NOT update anchor provenance
	skippedEventID := "spike-skipped-1"
	skippedCapture := DiagCapture{
		Source:               peer,
		CaptureStartedAt:     t30,
		Status:               DiagCaptureStatusOK,
		CaptureStatus:        CaptureStatusSkippedCooldown,
		SuppressedByCooldown: true,
		CooldownInfo:         BuildCooldownInfoFromDecision(decision, peer),
	}
	store.AddCapture(skippedEventID, skippedCapture)

	// Step 5: CRITICAL - Anchor must NOT have been advanced by skipped capture
	currentAnchor := store.GetLastCaptureAnchor(peer)
	if currentAnchor.AnchorCaptureID != originalEventID {
		t.Errorf("Skipped capture must NOT update anchor: expected %q, got %q",
			originalEventID, currentAnchor.AnchorCaptureID)
	}
	if currentAnchor.AnchorTargetID != "target-1" {
		t.Errorf("Skipped capture must NOT update anchor target: expected target-1, got %q",
			currentAnchor.AnchorTargetID)
	}

	// Step 6: lastCapture timestamp must NOT have been advanced
	currentTimestamp := store.GetLastCaptureTime(peer)
	if !currentTimestamp.Equal(originalTimestamp) {
		t.Errorf("Skipped capture must NOT update lastCapture timestamp: expected %v, got %v",
			originalTimestamp, currentTimestamp)
	}

	t.Log("PASS: Skipped cooldown does not advance anchor provenance")
}

// =============================================================================
// Test C: Multiple skipped cooldown captures do not advance anchor
// =============================================================================

// TestProvenance_MultipleSkippedDoNotAdvanceAnchor tests that multiple skipped
// cooldown captures do not advance the anchor.
func TestProvenance_MultipleSkippedDoNotAdvanceAnchor(t *testing.T) {
	store := NewCaptureStore()
	peer := "tovarisch-peer"
	originalEventID := "spike-anchor"

	// Create successful anchor capture
	t0 := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	store.AddCaptureWithProvenance(originalEventID, DiagCapture{
		Source:           peer,
		CaptureStartedAt: t0,
		Status:           DiagCaptureStatusOK,
		CaptureStatus:    CaptureStatusCaptured,
	}, "target-original", "http")

	// Record multiple skipped cooldown captures
	skippedTimes := []time.Time{
		t0.Add(10 * time.Second),
		t0.Add(20 * time.Second),
		t0.Add(30 * time.Second),
		t0.Add(40 * time.Second),
		t0.Add(50 * time.Second),
	}

	for i, t := range skippedTimes {
		decision := store.EvaluateCooldown(t, peer, 90)
		store.AddCapture("skipped-"+string(rune('a'+i)), DiagCapture{
			Source:               peer,
			CaptureStartedAt:     t,
			Status:               DiagCaptureStatusOK,
			CaptureStatus:        CaptureStatusSkippedCooldown,
			SuppressedByCooldown: true,
			CooldownInfo:         BuildCooldownInfoFromDecision(decision, peer),
		})
	}

	// CRITICAL: Anchor must still reference original capture
	anchor := store.GetLastCaptureAnchor(peer)
	if anchor.AnchorCaptureID != originalEventID {
		t.Errorf("Multiple skipped captures must NOT update anchor: expected %q, got %q",
			originalEventID, anchor.AnchorCaptureID)
	}
	if anchor.AnchorTargetID != "target-original" {
		t.Errorf("Anchor target must remain original: expected target-original, got %q",
			anchor.AnchorTargetID)
	}

	// At t=100s, cooldown should be expired
	t100 := t0.Add(100 * time.Second)
	decision := store.EvaluateCooldown(t100, peer, 90)
	if decision.IsInCooldown {
		t.Error("After 100s with 90s cooldown, should NOT be in cooldown")
	}

	t.Log("PASS: Multiple skipped captures do not advance anchor")
}
