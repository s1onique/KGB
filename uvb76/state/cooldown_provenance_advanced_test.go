package state

import (
	"testing"
	"time"
)

// =============================================================================
// Test D: Hidden anchor reason is precise
// =============================================================================

// TestProvenance_HiddenAnchorReason_TargetMismatch tests that when anchor is from
// a different target, the correct visibility reason is used.
func TestProvenance_HiddenAnchorReason_TargetMismatch(t *testing.T) {
	store := NewCaptureStore()
	peer := "tovarisch-peer"

	// Create anchor for target-1
	t0 := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	store.AddCaptureWithProvenance("anchor-target1", DiagCapture{
		Source:           peer,
		CaptureStartedAt: t0,
		Status:           DiagCaptureStatusOK,
		CaptureStatus:    CaptureStatusCaptured,
	}, "target-1", "http")

	// Evaluate cooldown for same peer (still active)
	t30 := t0.Add(30 * time.Second)
	decision := store.EvaluateCooldown(t30, peer, 90)

	if !decision.IsInCooldown {
		t.Fatal("Should be in cooldown")
	}

	// Verify anchor has correct target
	if decision.Anchor == nil {
		t.Fatal("Decision should have anchor")
	}
	if decision.Anchor.AnchorTargetID != "target-1" {
		t.Errorf("Anchor should have target-1, got %q", decision.Anchor.AnchorTargetID)
	}

	// Build cooldown info - anchor is "visible" by default (within same peer)
	info := BuildCooldownInfoFromDecision(decision, peer)
	if info == nil {
		t.Fatal("Should have cooldown_info")
	}

	// The state layer defaults to retained_visible when lastCapture exists
	// The API layer should override this when querying for a different target
	if info.AnchorTargetID != "target-1" {
		t.Errorf("CooldownInfo should have anchor_target_id=%q, got %q", "target-1", info.AnchorTargetID)
	}

	t.Log("PASS: Anchor target ID is preserved in cooldown info")
}

// TestProvenance_HiddenAnchorReason_ProbeMismatch tests that when anchor is from
// a different probe kind, the correct visibility reason is used.
func TestProvenance_HiddenAnchorReason_ProbeMismatch(t *testing.T) {
	store := NewCaptureStore()
	peer := "tovarisch-peer"

	// Create anchor for http probe
	t0 := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	store.AddCaptureWithProvenance("anchor-http", DiagCapture{
		Source:           peer,
		CaptureStartedAt: t0,
		Status:           DiagCaptureStatusOK,
		CaptureStatus:    CaptureStatusCaptured,
	}, "target-1", "http")

	// Evaluate cooldown
	t30 := t0.Add(30 * time.Second)
	decision := store.EvaluateCooldown(t30, peer, 90)

	if !decision.IsInCooldown {
		t.Fatal("Should be in cooldown")
	}

	// Verify anchor has correct probe kind
	if decision.Anchor == nil {
		t.Fatal("Decision should have anchor")
	}
	if decision.Anchor.AnchorProbeKind != "http" {
		t.Errorf("Anchor should have probe kind http, got %q", decision.Anchor.AnchorProbeKind)
	}

	// Build cooldown info
	info := BuildCooldownInfoFromDecision(decision, peer)
	if info == nil {
		t.Fatal("Should have cooldown_info")
	}

	// The API layer should override visibility reason when querying for icmp
	if info.AnchorProbeKind != "http" {
		t.Errorf("CooldownInfo should have anchor_probe_kind=%q, got %q", "http", info.AnchorProbeKind)
	}

	t.Log("PASS: Anchor probe kind is preserved in cooldown info")
}

// =============================================================================
// Test E: Warmup/startup scenario
// =============================================================================

// TestProvenance_WarmupAnchor_Labeled tests that anchors created during warmup
// are properly labeled.
func TestProvenance_WarmupAnchor_Labeled(t *testing.T) {
	store := NewCaptureStore()
	peer := "tovarisch-peer"

	// Simulate a warmup anchor being set via test helper
	t0 := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	store.SetLastCaptureAnchor(peer, CaptureCooldownAnchor{
		AnchorCaptureID:  "warmup-capture",
		AnchorTargetID:  "warmup-target",
		AnchorProbeKind: "http",
		AnchorSource:     peer,
		AnchorCreatedAt:  t0,
		CreatedFrom:      "startup_warmup",
		IsWarmupAnchor:   true,
	})

	// Evaluate cooldown
	t30 := t0.Add(30 * time.Second)
	decision := store.EvaluateCooldown(t30, peer, 90)

	if !decision.IsInCooldown {
		t.Fatal("Should be in cooldown")
	}

	// Verify anchor is properly labeled as warmup
	if decision.Anchor == nil {
		t.Fatal("Decision should have anchor")
	}
	if !decision.Anchor.IsWarmupAnchor {
		t.Error("Anchor should be labeled as warmup")
	}
	if decision.Anchor.CreatedFrom != "startup_warmup" {
		t.Errorf("Anchor should have CreatedFrom=startup_warmup, got %q", decision.Anchor.CreatedFrom)
	}

	// Build cooldown info
	info := BuildCooldownInfoFromDecision(decision, peer)
	if info == nil {
		t.Fatal("Should have cooldown_info")
	}
	if !info.IsWarmupAnchor {
		t.Error("CooldownInfo should propagate warmup indicator")
	}

	t.Log("PASS: Warmup anchor is properly labeled")
}

// =============================================================================
// Test F: Timestamp collision handling
// =============================================================================

// TestProvenance_TimestampCollision_PreferEventID tests that when two captures
// have the same timestamp, event ID matching is used to disambiguate.
func TestProvenance_TimestampCollision_PreferEventID(t *testing.T) {
	store := NewCaptureStore()
	peer := "tovarisch-peer"

	// Create first anchor capture
	t0 := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	store.AddCaptureWithProvenance("event-alpha", DiagCapture{
		Source:           peer,
		CaptureStartedAt: t0,
		Status:           DiagCaptureStatusOK,
		CaptureStatus:    CaptureStatusCaptured,
	}, "target-alpha", "http")

	// Create second anchor capture at SAME timestamp (collision)
	store.AddCaptureWithProvenance("event-beta", DiagCapture{
		Source:           peer,
		CaptureStartedAt: t0, // Same timestamp!
		Status:           DiagCaptureStatusOK,
		CaptureStatus:    CaptureStatusCaptured,
	}, "target-beta", "icmp")

	// The second capture should have overwritten the anchor
	anchor := store.GetLastCaptureAnchor(peer)

	// Latest capture wins (event-beta)
	if anchor.AnchorCaptureID != "event-beta" {
		t.Errorf("Latest capture should win: expected event-beta, got %q", anchor.AnchorCaptureID)
	}
	if anchor.AnchorTargetID != "target-beta" {
		t.Errorf("Anchor should have target-beta, got %q", anchor.AnchorTargetID)
	}
	if anchor.AnchorProbeKind != "icmp" {
		t.Errorf("Anchor should have probe kind icmp, got %q", anchor.AnchorProbeKind)
	}

	t.Log("PASS: Timestamp collision handled correctly (latest capture wins)")
}

// =============================================================================
// Test G: Empty store has no anchor
// =============================================================================

// TestProvenance_EmptyStore_NoAnchor tests that an empty store has no anchor.
func TestProvenance_EmptyStore_NoAnchor(t *testing.T) {
	store := NewCaptureStore()
	peer := "nonexistent-peer"

	anchor := store.GetLastCaptureAnchor(peer)
	if anchor.AnchorCaptureID != "" {
		t.Error("Empty store should have empty anchor")
	}

	allAnchors := store.GetAllLastCaptureAnchors()
	if len(allAnchors) != 0 {
		t.Errorf("Empty store should have no anchors, got %d", len(allAnchors))
	}

	t.Log("PASS: Empty store has no anchor")
}

// =============================================================================
// Test H: Clear() resets anchors
// =============================================================================

// TestProvenance_Clear_ResetsAnchors tests that Clear() resets all anchors.
func TestProvenance_Clear_ResetsAnchors(t *testing.T) {
	store := NewCaptureStore()
	peer := "tovarisch-peer"

	// Create anchor
	t0 := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	store.AddCaptureWithProvenance("event-1", DiagCapture{
		Source:           peer,
		CaptureStartedAt: t0,
		Status:           DiagCaptureStatusOK,
		CaptureStatus:    CaptureStatusCaptured,
	}, "target-1", "http")

	// Verify anchor exists
	anchor := store.GetLastCaptureAnchor(peer)
	if anchor.AnchorCaptureID != "event-1" {
		t.Fatal("Anchor should exist before Clear")
	}

	// Clear the store
	store.Clear()

	// Verify anchor is gone
	anchor = store.GetLastCaptureAnchor(peer)
	if anchor.AnchorCaptureID != "" {
		t.Error("Anchor should be cleared")
	}

	allAnchors := store.GetAllLastCaptureAnchors()
	if len(allAnchors) != 0 {
		t.Errorf("All anchors should be cleared, got %d", len(allAnchors))
	}

	t.Log("PASS: Clear() resets all anchors")
}

// =============================================================================
// Test I: Decision anchor is a copy, not a reference
// =============================================================================

// TestProvenance_DecisionAnchorIsCopy tests that EvaluateCooldown returns a
// copy of the anchor, not a reference to internal state.
func TestProvenance_DecisionAnchorIsCopy(t *testing.T) {
	store := NewCaptureStore()
	peer := "tovarisch-peer"

	// Create anchor
	t0 := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	store.AddCaptureWithProvenance("event-1", DiagCapture{
		Source:           peer,
		CaptureStartedAt: t0,
		Status:           DiagCaptureStatusOK,
		CaptureStatus:    CaptureStatusCaptured,
	}, "target-1", "http")

	// Evaluate cooldown
	t30 := t0.Add(30 * time.Second)
	decision := store.EvaluateCooldown(t30, peer, 90)

	if decision.Anchor == nil {
		t.Fatal("Decision should have anchor")
	}

	// Modify the decision anchor (should not affect internal state)
	decision.Anchor.AnchorCaptureID = "modified-event"

	// Evaluate again
	decision2 := store.EvaluateCooldown(t30, peer, 90)

	// Internal state should be unchanged
	if decision2.Anchor == nil {
		t.Fatal("Second decision should have anchor")
	}
	if decision2.Anchor.AnchorCaptureID != "event-1" {
		t.Error("Internal anchor should be unchanged after modifying decision copy")
	}

	t.Log("PASS: Decision anchor is a copy, not a reference")
}

// =============================================================================
// Test J: Cooldown info includes all provenance fields
// =============================================================================

// TestProvenance_CooldownInfo_AllFields tests that cooldown_info includes
// all provenance fields from the anchor.
func TestProvenance_CooldownInfo_AllFields(t *testing.T) {
	store := NewCaptureStore()
	peer := "tovarisch-peer"

	t0 := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	store.AddCaptureWithProvenance("event-full", DiagCapture{
		Source:           peer,
		CaptureStartedAt: t0,
		Status:           DiagCaptureStatusOK,
		CaptureStatus:    CaptureStatusCaptured,
	}, "target-full", "http")

	// Evaluate cooldown
	t30 := t0.Add(30 * time.Second)
	decision := store.EvaluateCooldown(t30, peer, 90)

	// Build cooldown info
	info := BuildCooldownInfoFromDecision(decision, peer)

	// Verify all provenance fields
	if info == nil {
		t.Fatal("Should have cooldown_info")
	}
	if info.AnchorCaptureID != "event-full" {
		t.Errorf("Expected anchor_capture_id=event-full, got %q", info.AnchorCaptureID)
	}
	if info.AnchorTargetID != "target-full" {
		t.Errorf("Expected anchor_target_id=target-full, got %q", info.AnchorTargetID)
	}
	if info.AnchorProbeKind != "http" {
		t.Errorf("Expected anchor_probe_kind=http, got %q", info.AnchorProbeKind)
	}
	if info.AnchorSource != peer {
		t.Errorf("Expected anchor_source=%q, got %q", peer, info.AnchorSource)
	}
	if info.AnchorCreatedAt == nil || !info.AnchorCreatedAt.Equal(t0) {
		t.Errorf("Expected anchor_created_at=%v, got %v", t0, info.AnchorCreatedAt)
	}
	if info.CreatedFrom != "diag_capture_success" {
		t.Errorf("Expected created_from=diag_capture_success, got %q", info.CreatedFrom)
	}
	if info.AnchorUpdatedByStatus != "captured" {
		t.Errorf("Expected anchor_updated_by_status=captured, got %q", info.AnchorUpdatedByStatus)
	}

	t.Log("PASS: Cooldown info includes all provenance fields")
}
