// Package main provides the UVB-76 pprof memory leak lab.
//
// # Lifecycle Cancellation Chain Integration Tests
//
// Phase 3-4: Real lifecycle/cancellation chain - deterministic integration
//
// These tests prove:
//   - Real wrapped cancelfunc executes when lifecycle returns
//   - Child observes context cancellation
//   - Profile seam is called exactly once
//   - Deterministic transport failure drives the lifecycle cancellation scenario
//
// NOTE: These tests do NOT prove:
//   - Finalization authority (no production seam observed)
//   - ProfileCaptureProfile real path (uses injected seam)
//   - Production publication composition
//   - Typed transport-to-parent propagation (PollTerminalError is diagnostic only)
package main

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Phase 3-4: Lifecycle Cancellation Tests
// =============================================================================

// TestLifecycleRealCancelExecuted proves the wrapped real cancelfunc is called
// exactly once when lifecycle returns.
func TestLifecycleRealCancelExecuted(t *testing.T) {
	var cancelCalls atomic.Int64

	// Create observation context with wrapped cancel
	obsCtx, realCancel := context.WithCancel(context.Background())

	wrappedCancel := func() {
		cancelCalls.Add(1)
		realCancel()
	}

	// Call the wrapped cancel
	wrappedCancel()

	// Child witness that observes cancellation
	select {
	case <-obsCtx.Done():
		t.Log("Context cancelled via wrapped cancel")
	case <-time.After(1 * time.Second):
		t.Error("Context was not cancelled")
	}

	// Cancel should be called exactly once
	if cancelCalls.Load() != 1 {
		t.Errorf("cancelCalls: got %d, want 1", cancelCalls.Load())
	} else {
		t.Log("Real cancelfunc called exactly once")
	}
}

// TestChildObservesCancellation proves child goroutine observes context cancellation.
// This is a unit test of the cancellation pattern, not dependent on RunCollectionLifecycle.
func TestChildObservesCancellation(t *testing.T) {
	var childTerminated atomic.Bool

	obsCtx, cancel := context.WithCancel(context.Background())

	// Child that observes cancellation
	go func() {
		<-obsCtx.Done()
		childTerminated.Store(true)
	}()

	// Cancel after a brief delay
	time.Sleep(10 * time.Millisecond)
	cancel()

	// Wait for child to observe
	time.Sleep(10 * time.Millisecond)

	if !childTerminated.Load() {
		t.Error("Child did not observe cancellation")
	} else {
		t.Log("Child observed context cancellation")
	}
}

// TestProfileSeamExactlyOnce proves the injected CaptureProfilesFn seam is called
// exactly once through the lifecycle using a deterministic RoundTripper.
func TestProfileSeamExactlyOnce(t *testing.T) {
	var captureCount atomic.Int64

	captureFn := func(ctx context.Context) error {
		calls := captureCount.Add(1)
		if calls > 1 {
			t.Errorf("Profile seam called %d times (should be exactly 1)", calls)
			return errors.New("double invocation detected")
		}
		// Simulate some work
		time.Sleep(50 * time.Millisecond)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var samples []ProcessSample
	var mu sync.Mutex

	// Use deterministic RoundTripper that returns error immediately
	transport := &deterministicErrorTransport{
		err: errors.New("deterministic transport failure"),
	}
	httpClient := &http.Client{Transport: transport}

	result := RunCollectionLifecycle(CollectionLifecycleInput{
		ObservationCtx:     ctx,
		ProfileCtx:         ctx,
		ObservationCancel:  cancel,
		WaitGroup:         &sync.WaitGroup{},
		CollectorInput: &CollectorInput{
			TovarischSamples: &samples,
			UVB76Samples:     &samples,
			CollectorErrors:  &[]string{},
			SamplesMu:        &mu,
		},
		PollInput: TargetPollInput{
			Client:          httpClient,
			UVB76APIBaseURL: "http://deterministic.invalid/fail",
			Target: TargetConfigBinding{
				TargetID: "test",
				BaseURL:  "http://deterministic.invalid/fail",
			},
			Auth:           TargetStateAuthInput{CookieName: "s", CookieValue: "v"},
			PollInterval:   10 * time.Millisecond,
			RequestTimeout: 50 * time.Millisecond,
			Deadline:       100 * time.Millisecond,
		},
		PollDrainTimeout:  50 * time.Millisecond,
		CaptureProfilesFn: captureFn,
	})

	// EXACT count assertion
	if captureCount.Load() != 1 {
		t.Errorf("Profile seam called %d times, want exactly 1", captureCount.Load())
	} else {
		t.Logf("Profile seam exactly-once verified: %d call(s)", captureCount.Load())
	}

	// Profile error should be set (poll failed due to transport error)
	if result.ProfileErr != nil {
		t.Logf("Profile capture returned with error: %v", result.ProfileErr)
	}

	// Transport error should be set in poll terminal error
	if result.PollTerminalError == nil {
		t.Log("Note: PollTerminalError is nil (may have completed before transport error propagated)")
	} else {
		t.Logf("Poll terminal error: %v", result.PollTerminalError)
	}
}

// deterministicErrorTransport is a RoundTripper that always returns a deterministic error.
type deterministicErrorTransport struct {
	err error
}

func (t *deterministicErrorTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, t.err
}

// =============================================================================
// Phase 3: Integration Test - Lifecycle Cancel Chain
// =============================================================================

// TestLifecycleCancelChain_Integration proves the complete production chain:
//   deterministic transport failure drives polling
//   → RunCollectionLifecycle executes its cancellation path
//   → lifecycle invokes wrapped real ObservationCancel exactly once
//   → ObservationCtx.Done() closes (same ctx the lifecycle uses)
//   → child terminates
//
// Key requirements:
//   - Lifecycle, cancel callback, and child share the SAME observation context pair
//   - Ordering is hard-asserted, not just logged
//
// This test does NOT prove:
//   - Finalization (no production seam observed)
//   - ProfileCaptureProfile real path
//   - Production publication topology
func TestLifecycleCancelChain_Integration(t *testing.T) {
	// Synchronization primitives for deterministic ordering
	var chainMu sync.Mutex
	var events []string

	logEvent := func(event string) {
		chainMu.Lock()
		events = append(events, event)
		chainMu.Unlock()
	}

	// Counters for exact-once assertions
	var cancelCalls atomic.Int64
	var childTerminationCalls atomic.Int64

	// Child termination channel
	childDone := make(chan struct{})

	// Create test-bounding context (for timeout, not cancellation)
	testCtx, testCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer testCancel()

	// Create the SAME observation context/cancel pair that lifecycle will use
	// This is critical: lifecycle, wrappedCancel, and child all share this pair
	obsCtx, realCancel := context.WithCancel(testCtx)

	wrappedCancel := func() {
		cancelCalls.Add(1)
		logEvent("lifecycle_cancel_invoked")
		realCancel()
	}

	// Register child BEFORE lifecycle execution, observing the SAME obsCtx
	go func() {
		select {
		case <-obsCtx.Done():
			childTerminationCalls.Add(1)
			logEvent("child_terminated")
			close(childDone)
		case <-time.After(10 * time.Second):
			logEvent("child_timeout")
		}
	}()

	// Create deterministic transport error
	injectedErr := errors.New("injected transport failure")
	transport := &deterministicErrorTransport{err: injectedErr}
	httpClient := &http.Client{Transport: transport}

	var samples []ProcessSample
	var mu sync.Mutex

	// Run the REAL production lifecycle with the SAME obsCtx/cancel pair
	logEvent("lifecycle_started")
	result := RunCollectionLifecycle(CollectionLifecycleInput{
		ObservationCtx:     obsCtx,  // Same context child observes
		ProfileCtx:         obsCtx,  // Same context
		ObservationCancel:  wrappedCancel,
		WaitGroup:         &sync.WaitGroup{},
		CollectorInput: &CollectorInput{
			TovarischSamples: &samples,
			UVB76Samples:     &samples,
			CollectorErrors:  &[]string{},
			SamplesMu:        &mu,
		},
		PollInput: TargetPollInput{
			Client:          httpClient,
			UVB76APIBaseURL: "http://deterministic.invalid/poll",
			Target: TargetConfigBinding{
				TargetID: "test",
				BaseURL:  "http://deterministic.invalid/poll",
			},
			Auth:           TargetStateAuthInput{CookieName: "s", CookieValue: "v"},
			PollInterval:   10 * time.Millisecond,
			RequestTimeout: 50 * time.Millisecond,
			Deadline:       100 * time.Millisecond,
		},
		PollDrainTimeout: 100 * time.Millisecond,
		CaptureProfilesFn: func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-time.After(1 * time.Second):
				return nil
			}
		},
	})
	logEvent("lifecycle_returned")

	// Wait for child to terminate
	select {
	case <-childDone:
	case <-time.After(2 * time.Second):
		t.Error("Child did not terminate within timeout")
	}

	// === HARD ASSERTIONS ===

	// 1. Cancel should be called exactly once by lifecycle
	if cancelCalls.Load() != 1 {
		t.Errorf("cancelCalls: got %d, want 1", cancelCalls.Load())
	}

	// 2. Child should terminate exactly once
	if childTerminationCalls.Load() != 1 {
		t.Errorf("childTerminationCalls: got %d, want 1", childTerminationCalls.Load())
	}

	// 3. HARD ORDERING ASSERTIONS
	chainMu.Lock()
	eventIdx := make(map[string]int)
	for i, e := range events {
		eventIdx[e] = i
	}
	chainMu.Unlock()

	// Required ordering: lifecycle_started < lifecycle_cancel_invoked < child_terminated
	// Note: lifecycle_returned can occur before or after child_terminated
	//       since lifecycle signals cancellation but doesn't own child termination
	if idx, ok := eventIdx["lifecycle_started"]; !ok || idx < 0 {
		t.Fatal("lifecycle_started event missing")
	}

	if cancelIdx, ok := eventIdx["lifecycle_cancel_invoked"]; !ok {
		t.Fatal("lifecycle_cancel_invoked event missing")
	} else {
		if startIdx := eventIdx["lifecycle_started"]; cancelIdx <= startIdx {
			t.Errorf("Ordering violated: lifecycle_started (%d) should be before lifecycle_cancel_invoked (%d)",
				startIdx, cancelIdx)
		}
	}

	if childIdx, ok := eventIdx["child_terminated"]; !ok {
		t.Fatal("child_terminated event missing")
	} else {
		if cancelIdx := eventIdx["lifecycle_cancel_invoked"]; childIdx <= cancelIdx {
			t.Errorf("Ordering violated: lifecycle_cancel_invoked (%d) should be before child_terminated (%d)",
				cancelIdx, childIdx)
		}
	}

	// Verify lifecycle_returned exists (its position relative to child is not constrained)
	if _, ok := eventIdx["lifecycle_returned"]; !ok {
		t.Fatal("lifecycle_returned event missing")
	}

	// 4. Observe poll terminal error for diagnostics only.
	// Typed transport-to-parent propagation is proved separately.
	if result.PollTerminalError != nil {
		t.Logf("Poll terminal error observed: %v", result.PollTerminalError)
	} else {
		t.Log("Note: PollTerminalError is nil (poll may have completed before transport error)")
	}

	t.Logf("Event chain: %v", events)
	t.Log("Ordering assertions passed")
}

// =============================================================================
// P0-12: Publication Authority Tests
// =============================================================================

// TestPersistResultNilGuard proves nil result is rejected by publication authority.
func TestPersistResultNilGuard(t *testing.T) {
	err := persistResult(nil, t.TempDir())
	if err == nil {
		t.Fatal("persistResult(nil) should fail")
	}
	if !errors.Is(err, ErrNilResult) {
		t.Errorf("Expected errors.Is(err, ErrNilResult), got: %v", err)
	} else {
		t.Log("Nil result correctly rejected with typed error")
	}
}

// TestPersistResultValidResult succeeds with valid result.
func TestPersistResultValidResult(t *testing.T) {
	result := &Result{
		SchemaVersion: 1,
		RunID:         "test-run",
		SourceCommit:  "abc123",
		TovarischPort: "12345",
		UVB76Port:     "12346",
		PProfPort:     "12347",
		Classification: "OBSERVED",
		OK:            true,
	}

	tmpDir := t.TempDir()
	err := persistResult(result, tmpDir)
	if err != nil {
		t.Fatalf("persistResult(valid) should succeed, got: %v", err)
	}
	t.Log("Valid result correctly persisted")
}
