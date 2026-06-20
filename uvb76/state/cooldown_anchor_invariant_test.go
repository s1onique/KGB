package state

import (
	"testing"
	"time"
)

// =============================================================================
// Cooldown Anchor Invariant Tests
//
// These tests catch the "all-suppressed cooldown false green" scenario:
// The UI/API must never show only retained skipped_cooldown spikes without
// exposing a valid prior successful cooldown anchor.
// =============================================================================

// TestCooldownAnchor_EmptyStore_FirstSpikeMustNotBeSkipped tests that a fresh
// capture store with no prior captures must never suppress the first spike.
func TestCooldownAnchor_EmptyStore_FirstSpikeMustNotBeSkipped(t *testing.T) {
	const cooldownSeconds = 5
	peerName := "peer-1"

	store := NewCaptureStore()

	// Empty store - no prior captures
	now := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)

	decision := store.EvaluateCooldown(now, peerName, cooldownSeconds)

	// CRITICAL: Empty store must NOT be in cooldown
	if decision.IsInCooldown {
		t.Errorf("Empty store must NOT be in cooldown, got IsInCooldown=true")
	}

	// CRITICAL: No prior capture timestamp
	if !decision.LastSuccessfulCaptureAt.IsZero() {
		t.Errorf("Empty store must have zero LastSuccessfulCaptureAt, got %v", decision.LastSuccessfulCaptureAt)
	}

	// BuildCooldownInfoFromDecision must return nil for non-cooldown
	info := BuildCooldownInfoFromDecision(decision, peerName)
	if info != nil {
		t.Error("Empty store must have nil cooldown_info (not suppressed)")
	}

	t.Log("PASS: Empty store does not suppress first spike")
}

// TestCooldownAnchor_ValidPriorCapture_SkippedCooldownAllowed tests that
// skipped_cooldown is valid ONLY when there's a prior successful capture.
func TestCooldownAnchor_ValidPriorCapture_SkippedCooldownAllowed(t *testing.T) {
	const cooldownSeconds = 5
	peerName := "peer-1"

	store := NewCaptureStore()

	// Simulate successful capture at t=0
	t0 := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	store.mu.Lock()
	store.lastCapture[peerName] = t0
	store.mu.Unlock()

	// At t=3s (inside cooldown), should be skipped
	t3 := t0.Add(3 * time.Second)
	decision := store.EvaluateCooldown(t3, peerName, cooldownSeconds)

	if !decision.IsInCooldown {
		t.Error("Inside cooldown, should be in cooldown")
	}

	// CRITICAL: Must have valid lastSuccessfulCaptureAt
	if decision.LastSuccessfulCaptureAt.IsZero() {
		t.Error("skipped_cooldown requires non-zero LastSuccessfulCaptureAt")
	}

	// BuildCooldownInfoFromDecision must return valid info
	info := BuildCooldownInfoFromDecision(decision, peerName)
	if info == nil {
		t.Fatal("skipped_cooldown requires non-nil cooldown_info")
	}

	// CRITICAL: cooldown_info must have the anchor timestamp
	if info.LastSuccessfulCaptureAt == nil || info.LastSuccessfulCaptureAt.IsZero() {
		t.Error("cooldown_info must have LastSuccessfulCaptureAt for skipped_cooldown")
	}

	if !info.LastSuccessfulCaptureAt.Equal(t0) {
		t.Errorf("cooldown_info.LastSuccessfulCaptureAt should be %v, got %v", t0, *info.LastSuccessfulCaptureAt)
	}

	t.Log("PASS: Skipped cooldown with valid prior capture has anchor metadata")
}

// TestCooldownAnchor_MissingLastCapture_SkippedCooldownInvalid tests the invariant:
// If lastCapture is missing/zero but we're told to skip, that's a bug.
func TestCooldownAnchor_MissingLastCapture_SkippedCooldownInvalid(t *testing.T) {
	const cooldownSeconds = 5
	peerName := "peer-1"

	store := NewCaptureStore()

	// NOTE: Do NOT set store.lastCapture[peerName]
	// This simulates the scenario where cooldown state exists but anchor is gone

	now := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)

	decision := store.EvaluateCooldown(now, peerName, cooldownSeconds)

	// CRITICAL: Without lastCapture entry, must NOT be in cooldown
	if decision.IsInCooldown {
		t.Error("Missing lastCapture must NOT suppress - would cause all-suppressed-from-start")
	}

	// The decision should NOT have LastSuccessfulCaptureAt
	if !decision.LastSuccessfulCaptureAt.IsZero() {
		t.Error("Missing lastCapture should result in zero LastSuccessfulCaptureAt")
	}

	t.Log("PASS: Missing lastCapture does not cause spurious suppression")
}

// TestCooldownAnchor_CooldownExpired_NotInCooldown tests that when cooldown
// expires, the system is not in cooldown anymore.
func TestCooldownAnchor_CooldownExpired_NotInCooldown(t *testing.T) {
	const cooldownSeconds = 5
	peerName := "peer-1"

	store := NewCaptureStore()

	// Simulate successful capture at t=0
	t0 := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	store.mu.Lock()
	store.lastCapture[peerName] = t0
	store.mu.Unlock()

	// At t=6s (after 5s cooldown), cooldown must be expired
	t6 := t0.Add(6 * time.Second)
	decision := store.EvaluateCooldown(t6, peerName, cooldownSeconds)

	// After 6s with 5s cooldown, must NOT be in cooldown
	if decision.IsInCooldown {
		t.Error("After 6s with 5s cooldown, must NOT be in cooldown")
	}

	// Even though lastCapture exists, cooldown is expired
	info := BuildCooldownInfoFromDecision(decision, peerName)
	if info != nil {
		t.Error("Expired cooldown should have nil cooldown_info")
	}

	t.Log("PASS: Expired cooldown clears correctly")
}

// TestCooldownAnchor_CaptureEvictedButCooldownActive tests the scenario where
// the successful capture is no longer visible (evicted from retention), but cooldown
// is still active.
//
// The state layer provides anchor metadata defaults. The API layer is responsible
// for overriding AnchorVisible/AnchorVisibilityReason when the anchor spike
// is outside the visible response scope.
func TestCooldownAnchor_CaptureEvictedButCooldownActive(t *testing.T) {
	const cooldownSeconds = 90
	peerName := "tovarisch-peer"

	store := NewCaptureStore()

	// Successful capture at t=0 (visible in store.lastCapture)
	t0 := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	store.mu.Lock()
	store.lastCapture[peerName] = t0
	store.mu.Unlock()

	// At t=30s: capture was evicted from visible retention window,
	// but lastCapture still exists and cooldown is still active
	t30 := t0.Add(30 * time.Second)
	decision := store.EvaluateCooldown(t30, peerName, cooldownSeconds)

	// At t=30s with 90s cooldown, still in cooldown
	if !decision.IsInCooldown {
		t.Fatal("At t=30s with 90s cooldown, should still be in cooldown")
	}

	// CRITICAL: Must have anchor timestamp (even though capture is not visible)
	if decision.LastSuccessfulCaptureAt.IsZero() {
		t.Error("Evicted capture but active cooldown must have LastSuccessfulCaptureAt")
	}

	// Build cooldown_info
	info := BuildCooldownInfoFromDecision(decision, peerName)
	if info == nil {
		t.Fatal("Active cooldown must have cooldown_info")
	}

	// CRITICAL: Must have anchor timestamp for UI to explain
	if info.LastSuccessfulCaptureAt == nil || info.LastSuccessfulCaptureAt.IsZero() {
		t.Error("cooldown_info must have last_successful_capture_at (anchor timestamp)")
	}

	// State layer provides default anchor visibility (assumes anchor is visible).
	// API layer should override this when anchor is outside response scope.
	// The cooldown_info ALWAYS has anchor metadata - the reason field explains visibility.
	if info.AnchorVisibilityReason == "" {
		t.Error("cooldown_info must have anchor_visibility_reason (state layer default)")
	}

	t.Logf("PASS: Active cooldown has anchor metadata (reason: %s, anchor_visible: %v)", info.AnchorVisibilityReason, info.AnchorVisible)
}

// TestCooldownAnchor_SkippedDoesNotExtendCooldown documents the key semantics:
// skipped cooldown attempts do NOT extend the cooldown window.
func TestCooldownAnchor_SkippedDoesNotExtendCooldown(t *testing.T) {
	const cooldownSeconds = 5
	peerName := "peer-1"

	store := NewCaptureStore()

	// Successful capture at t=0
	t0 := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	store.mu.Lock()
	store.lastCapture[peerName] = t0
	store.mu.Unlock()

	// At t=4s: skipped attempt (inside cooldown)
	t4 := t0.Add(4 * time.Second)
	decisionT4 := store.EvaluateCooldown(t4, peerName, cooldownSeconds)

	if !decisionT4.IsInCooldown {
		t.Error("At t=4s, should be in cooldown")
	}

	// Simulate skipped cooldown capture being recorded
	// (Note: skipped captures should NOT update lastCapture - tested elsewhere)
	skippedCapture := DiagCapture{
		Source:               peerName,
		CaptureStartedAt:     t4,
		Status:               DiagCaptureStatusOK,
		CaptureStatus:        CaptureStatusSkippedCooldown,
		SuppressedByCooldown: true,
	}
	store.AddCapture("event-skipped", skippedCapture)

	// At t=6s: second attempt (should NOT still be in cooldown)
	t6 := t0.Add(6 * time.Second)
	decisionT6 := store.EvaluateCooldown(t6, peerName, cooldownSeconds)

	if decisionT6.IsInCooldown {
		t.Error("At t=6s, cooldown should be expired (skipped attempts do NOT extend)")
	}

	// CRITICAL: lastCapture should still be the original t0, not updated by skipped
	gotLastCapture := store.GetLastCaptureTime(peerName)
	if !gotLastCapture.Equal(t0) {
		t.Errorf("Skipped capture must NOT update lastCapture: got %v, want %v", gotLastCapture, t0)
	}

	t.Log("PASS: Skipped cooldown attempts do not extend cooldown window")
}

// TestCooldownAnchor_SuppressedRecordsMustNotAdvanceAnchor tests that suppressed
// cooldown records (not successful captures) must not advance the anchor.
func TestCooldownAnchor_SuppressedRecordsMustNotAdvanceAnchor(t *testing.T) {
	store := NewCaptureStore()
	peer := "peer-1"

	// Successful capture at t=0
	t0 := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	store.mu.Lock()
	store.lastCapture[peer] = t0
	store.mu.Unlock()

	// Add multiple suppressed captures at various times
	suppressedTimes := []time.Time{
		t0.Add(1 * time.Second),
		t0.Add(2 * time.Second),
		t0.Add(3 * time.Second),
		t0.Add(4 * time.Second),
	}

	for i, t := range suppressedTimes {
		store.AddCapture("event-suppressed-"+string(rune('a'+i)), DiagCapture{
			Source:               peer,
			CaptureStartedAt:     t,
			Status:               DiagCaptureStatusOK,
			CaptureStatus:        CaptureStatusSkippedCooldown,
			SuppressedByCooldown: true,
		})
	}

	// CRITICAL: lastCapture must still be t0 (not advanced by suppressed)
	got := store.GetLastCaptureTime(peer)
	if !got.Equal(t0) {
		t.Fatalf("Suppressed captures must NOT advance lastCapture: got %v, want %v", got, t0)
	}

	// At t=5s, cooldown must be expired (not extended by suppressed records)
	t5 := t0.Add(5 * time.Second)
	decision := store.EvaluateCooldown(t5, peer, 5)
	if decision.IsInCooldown {
		t.Error("Cooldown must be expired at t=5s (suppressed records do not extend)")
	}

	t.Log("PASS: Suppressed cooldown records do not advance anchor")
}

// =============================================================================
// CaptureStore.lastCapture Access Tests
// =============================================================================

// TestCaptureStore_GetLastCaptureTime_MissingPeer returns zero time
func TestCaptureStore_GetLastCaptureTime_MissingPeer(t *testing.T) {
	store := NewCaptureStore()

	got := store.GetLastCaptureTime("nonexistent-peer")
	if !got.IsZero() {
		t.Errorf("Missing peer should return zero time, got %v", got)
	}
}

// TestCaptureStore_AddCapture_SuccessfulCaptureUpdatesLastCapture
func TestCaptureStore_AddCapture_SuccessfulCaptureUpdatesLastCapture(t *testing.T) {
	store := NewCaptureStore()
	peer := "peer-1"

	// Successful capture at t=0
	t0 := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	store.AddCapture("event-1", DiagCapture{
		Source:           peer,
		CaptureStartedAt: t0,
		Status:           DiagCaptureStatusOK,
		CaptureStatus:    CaptureStatusCaptured,
	})

	got := store.GetLastCaptureTime(peer)
	if got.IsZero() {
		t.Error("Successful capture should update lastCapture")
	}
}

// =============================================================================
// CooldownInfo Field Completeness Tests
// =============================================================================

// TestBuildCooldownInfoFromDecision_AllRequiredFields tests that the built
// cooldown_info contains all required fields for anchor visibility.
func TestBuildCooldownInfoFromDecision_AllRequiredFields(t *testing.T) {
	const cooldownSeconds = 5
	peerName := "peer-1"

	store := NewCaptureStore()

	// Successful capture at t=0
	t0 := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	store.mu.Lock()
	store.lastCapture[peerName] = t0
	store.mu.Unlock()

	// At t=3s (inside cooldown)
	t3 := t0.Add(3 * time.Second)
	decision := store.EvaluateCooldown(t3, peerName, cooldownSeconds)

	info := BuildCooldownInfoFromDecision(decision, peerName)
	if info == nil {
		t.Fatal("cooldown_info should not be nil when in cooldown")
	}

	// CRITICAL fields for anchor visibility:
	// 1. Scope - identifies the cooldown scope
	if info.Scope == "" {
		t.Error("cooldown_info.scope must not be empty")
	}

	// 2. LastSuccessfulCaptureAt - the anchor timestamp
	if info.LastSuccessfulCaptureAt == nil {
		t.Error("cooldown_info.last_successful_capture_at must not be nil")
	}

	// 3. NextCaptureEligibleAt - when cooldown expires
	if info.NextCaptureEligibleAt == nil {
		t.Error("cooldown_info.next_capture_eligible_at must not be nil")
	}

	// 4. RemainingCooldownMs - explicit remaining time
	if info.RemainingCooldownMs == nil {
		t.Error("cooldown_info.remaining_cooldown_ms must not be nil")
	}

	// 5. SkippedAttemptUpdatesCooldown - semantic documentation
	if info.SkippedAttemptUpdatesCooldown {
		t.Error("skipped_attempt_updates_cooldown should be false (preferred semantics)")
	}

	// 6. CaptureKey - identifies which peer started the cooldown
	if info.CaptureKey == "" {
		t.Error("cooldown_info.capture_key must not be empty")
	}

	// 7. DecisionNowAt - proves when cooldown_info was computed
	if info.DecisionNowAt == nil {
		t.Error("cooldown_info.decision_now_at must not be nil")
	}

	t.Log("PASS: cooldown_info contains all required anchor visibility fields")
}
