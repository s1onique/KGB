package state

import (
	"testing"
	"time"
)

// FakeClock provides a controllable clock for testing.
type FakeClock struct {
	now time.Time
}

func NewFakeClock(t time.Time) *FakeClock {
	return &FakeClock{now: t}
}

func (f *FakeClock) Now() time.Time {
	return f.now
}

func (f *FakeClock) Add(d time.Duration) {
	f.now = f.now.Add(d)
}

// =============================================================================
// Test A: Successful capture establishes cooldown
// =============================================================================

// TestCaptureCooldownDecision_EstablishesCooldown tests that a successful capture
// sets up the cooldown window correctly.
func TestCaptureCooldownDecision_EstablishesCooldown(t *testing.T) {
	const cooldownSeconds = 5
	peerName := "peer-1"

	store := NewCaptureStore()
	
	// Simulate a successful capture at t=0
	t0 := time.Date(2026, 6, 20, 4, 20, 0, 0, time.UTC)
	
	// Manually set the last capture time (simulating successful capture)
	store.mu.Lock()
	store.lastCapture[peerName] = t0
	store.mu.Unlock()
	
	// Evaluate cooldown at t=0 (exactly when capture happened)
	decision := store.EvaluateCooldown(t0, peerName, cooldownSeconds)
	
	// At exactly t=0, the cooldown IS active (0 < 5 seconds elapsed).
	// This prevents immediate re-capture right after a successful capture.
	// This is correct behavior: you must wait for the cooldown to expire.
	if !decision.IsInCooldown {
		t.Errorf("At t=0 (exactly when capture happened), cooldown should be active (just captured)")
	}
	
	// Evaluate cooldown at t=3s (inside cooldown)
	t3 := t0.Add(3 * time.Second)
	decision = store.EvaluateCooldown(t3, peerName, cooldownSeconds)
	
	if !decision.IsInCooldown {
		t.Errorf("At t=3s (inside 5s cooldown), should be in cooldown")
	}
	if decision.RemainingCooldownMs != 2000 {
		t.Errorf("Expected remaining 2000ms, got %d", decision.RemainingCooldownMs)
	}
	
	// Evaluate cooldown at t=5s (exactly at boundary)
	t5 := t0.Add(5 * time.Second)
	decision = store.EvaluateCooldown(t5, peerName, cooldownSeconds)
	
	if decision.IsInCooldown {
		t.Errorf("At t=5s (exactly at boundary), should NOT be in cooldown")
	}
	
	// Evaluate cooldown at t=6s (after cooldown expires)
	t6 := t0.Add(6 * time.Second)
	decision = store.EvaluateCooldown(t6, peerName, cooldownSeconds)
	
	if decision.IsInCooldown {
		t.Errorf("At t=6s (after 5s cooldown), should NOT be in cooldown")
	}
	
	// Verify metadata fields
	t3Decision := store.EvaluateCooldown(t3, peerName, cooldownSeconds)
	if t3Decision.CooldownKey != peerName {
		t.Errorf("Expected cooldown key %q, got %q", peerName, t3Decision.CooldownKey)
	}
	if !t3Decision.LastSuccessfulCaptureAt.Equal(t0) {
		t.Errorf("Expected lastSuccessfulCaptureAt=%v, got %v", t0, t3Decision.LastSuccessfulCaptureAt)
	}
	expectedEligible := t0.Add(5 * time.Second)
	if !t3Decision.NextCaptureEligibleAt.Equal(expectedEligible) {
		t.Errorf("Expected nextCaptureEligibleAt=%v, got %v", expectedEligible, t3Decision.NextCaptureEligibleAt)
	}
}

// =============================================================================
// Test B: Skipped event inside cooldown does NOT extend cooldown (preferred semantics)
// =============================================================================

// TestCaptureCooldownDecision_SkippedDoesNotExtendCooldown tests the preferred semantics:
// skipped cooldown attempts do NOT extend the cooldown window.
func TestCaptureCooldownDecision_SkippedDoesNotExtendCooldown(t *testing.T) {
	const cooldownSeconds = 5
	peerName := "peer-1"

	store := NewCaptureStore()
	
	// Simulate a successful capture at t=0
	t0 := time.Date(2026, 6, 20, 4, 20, 0, 0, time.UTC)
	
	store.mu.Lock()
	store.lastCapture[peerName] = t0
	store.mu.Unlock()
	
	// At t=4s, event arrives and is skipped (still in cooldown)
	t4 := t0.Add(4 * time.Second)
	decision := store.EvaluateCooldown(t4, peerName, cooldownSeconds)
	
	if !decision.IsInCooldown {
		t.Errorf("At t=4s (inside cooldown), should be in cooldown")
	}
	if decision.RemainingCooldownMs != 1000 {
		t.Errorf("Expected remaining 1000ms, got %d", decision.RemainingCooldownMs)
	}
	
	// At t=5s, cooldown expires exactly
	t5 := t0.Add(5 * time.Second)
	decision = store.EvaluateCooldown(t5, peerName, cooldownSeconds)
	
	if decision.IsInCooldown {
		t.Errorf("At t=5s (cooldown expired), should NOT be in cooldown")
	}
	
	// At t=6s or t=7s, next event MUST capture (cooldown not extended)
	for _, tt := range []time.Time{t0.Add(6 * time.Second), t0.Add(7 * time.Second)} {
		decision = store.EvaluateCooldown(tt, peerName, cooldownSeconds)
		if decision.IsInCooldown {
			t.Errorf("At t=%v, cooldown should be expired (skipped attempts do NOT extend)", tt)
		}
	}
	
	// Verify exported cooldown info at t=4s still points to t=5s
	t4Decision := store.EvaluateCooldown(t4, peerName, cooldownSeconds)
	info := BuildCooldownInfoFromDecision(t4Decision, peerName)
	
	if info == nil {
		t.Fatal("Expected cooldown_info to be non-nil when in cooldown")
	}
	
	expectedEligible := t0.Add(5 * time.Second)
	if !info.NextCaptureEligibleAt.Equal(expectedEligible) {
		t.Errorf("next_capture_eligible_at should be %v, got %v", expectedEligible, *info.NextCaptureEligibleAt)
	}
	
	if info.SkippedAttemptUpdatesCooldown {
		t.Error("Semantics violation: skipped_attempt_updates_cooldown should be false (preferred semantics)")
	}
}

// =============================================================================
// Test D: Exported metadata matches decision
// =============================================================================

// TestCaptureCooldownDecision_MetadataMatchesDecision tests that cooldown_info
// exactly matches the decision used for skip/capture.
func TestCaptureCooldownDecision_MetadataMatchesDecision(t *testing.T) {
	const cooldownSeconds = 5
	peerName := "peer-1"

	store := NewCaptureStore()
	
	// Successful capture at t=0
	t0 := time.Date(2026, 6, 20, 4, 20, 0, 0, time.UTC)
	
	store.mu.Lock()
	store.lastCapture[peerName] = t0
	store.mu.Unlock()
	
	// At t=3s, event arrives and is skipped
	t3 := t0.Add(3 * time.Second)
	decision := store.EvaluateCooldown(t3, peerName, cooldownSeconds)
	
	// Verify decision fields
	if !decision.IsInCooldown {
		t.Fatal("Expected to be in cooldown at t=3s")
	}
	if decision.CooldownKey != peerName {
		t.Errorf("Expected cooldown_key=%q, got %q", peerName, decision.CooldownKey)
	}
	if !decision.DecisionNowAt.Equal(t3) {
		t.Errorf("Expected decision_now_at=%v, got %v", t3, decision.DecisionNowAt)
	}
	if !decision.LastSuccessfulCaptureAt.Equal(t0) {
		t.Errorf("Expected last_successful_capture_at=%v, got %v", t0, decision.LastSuccessfulCaptureAt)
	}
	expectedRemaining := int64(2000)
	if decision.RemainingCooldownMs != expectedRemaining {
		t.Errorf("Expected remaining_cooldown_ms=%d, got %d", expectedRemaining, decision.RemainingCooldownMs)
	}
	
	// Build cooldown_info from decision
	info := BuildCooldownInfoFromDecision(decision, peerName)
	
	if info == nil {
		t.Fatal("Expected cooldown_info to be non-nil")
	}
	if info.CaptureKey != decision.CooldownKey {
		t.Errorf("cooldown_info.cooldown_key should match decision.cooldown_key")
	}
	if info.RemainingCooldownMs == nil || *info.RemainingCooldownMs != expectedRemaining {
		t.Errorf("cooldown_info.remaining_cooldown_ms should be %d, got %v", expectedRemaining, info.RemainingCooldownMs)
	}
	
	// CRITICAL: Verify DecisionNowAt matches exactly (Blocker 1 fix)
	if info.DecisionNowAt == nil {
		t.Fatal("cooldown_info.decision_now_at must not be nil")
	}
	if !info.DecisionNowAt.Equal(decision.DecisionNowAt) {
		t.Fatalf("cooldown_info.decision_now_at must match decision.decision_now_at: got %v, want %v", info.DecisionNowAt, decision.DecisionNowAt)
	}
	
	// Verify Scope matches the actual decision basis
	if info.Scope != "per_diagnostic_peer" {
		t.Errorf("cooldown_info.scope should be per_diagnostic_peer (honest naming), got %q", info.Scope)
	}
}

// =============================================================================
// Test E: Eligibility invariant
// =============================================================================

// TestCaptureCooldownDecision_EligibilityInvariant tests that if remaining_cooldown_ms <= 0,
// then capture_status must NOT be skipped_cooldown.
func TestCaptureCooldownDecision_EligibilityInvariant(t *testing.T) {
	const cooldownSeconds = 5
	peerName := "peer-1"

	store := NewCaptureStore()
	
	// Successful capture at t=0
	t0 := time.Date(2026, 6, 20, 4, 20, 0, 0, time.UTC)
	
	store.mu.Lock()
	store.lastCapture[peerName] = t0
	store.mu.Unlock()
	
	// Test at various times
	testCases := []struct {
		elapsed       time.Duration
		expectInCooldown bool
		expectedRemaining int64
	}{
		{0 * time.Second, true, 5000},      // t=0: just captured, cooldown active (0 < 5s elapsed)
		{3 * time.Second, true, 2000},      // t=3s: in cooldown, 2s remaining
		{4 * time.Second, true, 1000},      // t=4s: in cooldown, 1s remaining
		{5 * time.Second, false, 0},        // t=5s: exactly expired
		{6 * time.Second, false, 0},        // t=6s: expired
		{10 * time.Second, false, 0},       // t=10s: expired
	}
	
	for _, tc := range testCases {
		t.Run(tc.elapsed.String(), func(t *testing.T) {
			now := t0.Add(tc.elapsed)
			decision := store.EvaluateCooldown(now, peerName, cooldownSeconds)
			
			if decision.IsInCooldown != tc.expectInCooldown {
				t.Errorf("At t=%v, expected in_cooldown=%v, got %v", tc.elapsed, tc.expectInCooldown, decision.IsInCooldown)
			}
			
			if decision.RemainingCooldownMs != tc.expectedRemaining {
				t.Errorf("At t=%v, expected remaining=%dms, got %dms", tc.elapsed, tc.expectedRemaining, decision.RemainingCooldownMs)
			}
			
			// Invariant: remaining <= 0 implies NOT in cooldown
			if decision.RemainingCooldownMs <= 0 && decision.IsInCooldown {
				t.Errorf("INVARIANT VIOLATION: remaining=%d but in_cooldown=true", decision.RemainingCooldownMs)
			}
		})
	}
}

// =============================================================================
// Test: No prior capture => not in cooldown
// =============================================================================

// TestCaptureCooldownDecision_NoPriorCapture tests that if there's no prior
// successful capture, the peer is not in cooldown.
func TestCaptureCooldownDecision_NoPriorCapture(t *testing.T) {
	const cooldownSeconds = 5
	peerName := "peer-1"

	store := NewCaptureStore()
	
	// No prior capture - fresh state
	now := time.Now().UTC()
	decision := store.EvaluateCooldown(now, peerName, cooldownSeconds)
	
	if decision.IsInCooldown {
		t.Error("With no prior capture, should NOT be in cooldown")
	}
	if !decision.LastSuccessfulCaptureAt.IsZero() {
		t.Error("With no prior capture, lastSuccessfulCaptureAt should be zero time")
	}
}

// =============================================================================
// Test: BuildCooldownInfoFromDecision nil when not in cooldown
// =============================================================================

// TestBuildCooldownInfoFromDecision_NilWhenNotInCooldown tests that when not
// in cooldown, BuildCooldownInfoFromDecision returns nil.
func TestBuildCooldownInfoFromDecision_NilWhenNotInCooldown(t *testing.T) {
	const cooldownSeconds = 5
	peerName := "peer-1"

	store := NewCaptureStore()
	
	// Successful capture at t=0
	t0 := time.Date(2026, 6, 20, 4, 20, 0, 0, time.UTC)
	
	store.mu.Lock()
	store.lastCapture[peerName] = t0
	store.mu.Unlock()
	
	// At t=6s (after cooldown), should NOT be in cooldown
	t6 := t0.Add(6 * time.Second)
	decision := store.EvaluateCooldown(t6, peerName, cooldownSeconds)
	
	if decision.IsInCooldown {
		t.Fatal("At t=6s, should NOT be in cooldown")
	}
	
	// BuildCooldownInfoFromDecision should return nil when not in cooldown
	info := BuildCooldownInfoFromDecision(decision, peerName)
	if info != nil {
		t.Error("BuildCooldownInfoFromDecision should return nil when not in cooldown")
	}
}

// =============================================================================
// Test: Cooldown semantics documented
// =============================================================================

// TestCaptureCooldownDecision_SemanticsDocumented tests that the cooldown
// semantics are properly documented in the decision.
func TestCaptureCooldownDecision_SemanticsDocumented(t *testing.T) {
	const cooldownSeconds = 5
	peerName := "peer-1"

	store := NewCaptureStore()
	
	// Successful capture at t=0
	t0 := time.Date(2026, 6, 20, 4, 20, 0, 0, time.UTC)
	
	store.mu.Lock()
	store.lastCapture[peerName] = t0
	store.mu.Unlock()
	
	// At t=4s, skipped attempt
	t4 := t0.Add(4 * time.Second)
	decision := store.EvaluateCooldown(t4, peerName, cooldownSeconds)
	
	// The semantics field documents the behavior
	if decision.SkippedAttemptUpdatesCooldown {
		t.Error("Skipped attempts should NOT extend cooldown (preferred semantics)")
	}
	
	// At t=6s, the skipped attempt should NOT have extended cooldown
	t6 := t0.Add(6 * time.Second)
	decision = store.EvaluateCooldown(t6, peerName, cooldownSeconds)
	
	if decision.IsInCooldown {
		t.Error("After cooldown expires, should be eligible regardless of skipped attempts")
	}
}

// =============================================================================
// Test: AddCapture does not update lastCapture for skipped cooldown
// =============================================================================

// TestCaptureStore_AddCapture_SkippedCooldownDoesNotUpdateLastCapture verifies that
// skipped cooldown captures do NOT update the lastCapture map.
// This is a critical invariant: skipped cooldown records have Status=OK and
// SuppressedByCooldown=true, but should NOT refresh the cooldown window.
func TestCaptureStore_AddCapture_SkippedCooldownDoesNotUpdateLastCapture(t *testing.T) {
	store := NewCaptureStore()
	peer := "peer-1"

	// Set initial capture time at t=0
	t0 := time.Date(2026, 6, 20, 4, 20, 0, 0, time.UTC)
	store.mu.Lock()
	store.lastCapture[peer] = t0
	store.mu.Unlock()

	// Simulate a skipped cooldown capture at t=4s
	skippedAt := t0.Add(4 * time.Second)
	store.AddCapture("event-skip", DiagCapture{
		Source:               peer,
		CaptureStartedAt:     skippedAt,
		Status:               DiagCaptureStatusOK,
		CaptureStatus:        CaptureStatusSkippedCooldown,
		SuppressedByCooldown: true,
	})

	// CRITICAL: lastCapture must NOT have been updated by the skipped capture
	got := store.GetLastCaptureTime(peer)
	if !got.Equal(t0) {
		t.Fatalf("skipped cooldown capture must not update lastCapture: got %v, want %v", got, t0)
	}
	
	t.Logf("PASS: skipped cooldown capture did not update lastCapture (still %v)", t0)
	
	// Verify cooldown is still active at t=4s (should skip)
	decision := store.EvaluateCooldown(skippedAt, peer, 5)
	if !decision.IsInCooldown {
		t.Error("At t=4s (inside cooldown), should be in cooldown")
	}
	
	// Verify cooldown expires at t=5s
	t5 := t0.Add(5 * time.Second)
	decision = store.EvaluateCooldown(t5, peer, 5)
	if decision.IsInCooldown {
		t.Error("At t=5s (boundary), should NOT be in cooldown")
	}
}
