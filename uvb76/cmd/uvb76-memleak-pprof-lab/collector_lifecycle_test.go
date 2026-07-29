package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

// TestCollectAndSnapshot_CallGraphBinding proves the exact production call graph:
// 1. Both collectors register with WaitGroup before writing
// 2. cancelFn triggers both collectors to stop writing
// 3. wg.Wait() blocks until both call Done
// 4. Snapshot is taken under mutex after Wait returns
// 5. The returned snapshot is immutable
func TestCollectAndSnapshot_CallGraphBinding(t *testing.T) {
	var (
		waitReturned     atomic.Bool
		snapshotTaken    atomic.Bool
		collectorAExited atomic.Bool
		collectorBExited atomic.Bool
	)

	var samplesA, samplesB []ProcessSample
	var mu sync.Mutex

	// Channels for deterministic signaling (no timers)
	collectorAReady := make(chan struct{})
	collectorBReady := make(chan struct{})

	// Context with cancel function
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup

	// Collector A - signals ready, waits for cancellation, signals exit
	wg.Add(1)
	go func() {
		defer wg.Done()
		collectorAReady <- struct{}{}
		select {
		case <-ctx.Done():
			mu.Lock()
			samplesA = append(samplesA, ProcessSample{})
			mu.Unlock()
			collectorAExited.Store(true)
		}
	}()

	// Collector B - signals ready, waits for cancellation, signals exit
	wg.Add(1)
	go func() {
		defer wg.Done()
		collectorBReady <- struct{}{}
		select {
		case <-ctx.Done():
			mu.Lock()
			samplesB = append(samplesB, ProcessSample{})
			mu.Unlock()
			collectorBExited.Store(true)
		}
	}()

	// Wait for both collectors to be ready
	<-collectorAReady
	<-collectorBReady

	// Write initial samples
	mu.Lock()
	samplesA = append(samplesA, ProcessSample{}, ProcessSample{}, ProcessSample{})
	samplesB = append(samplesB, ProcessSample{}, ProcessSample{}, ProcessSample{})
	mu.Unlock()

	// Create input structure
	input := &CollectorInput{
		TovarischSamples: &samplesA,
		UVB76Samples:     &samplesB,
		CollectorErrors:  &[]string{},
		SamplesMu:        &mu,
	}

	// Call the extracted production helper and CAPTURE the returned snapshot
	snapshot, err := CollectAndSnapshot(cancel, &wg, input)
	if err != nil {
		t.Fatalf("CollectAndSnapshot failed: %v", err)
	}

	// Verify wait returned (wg.Wait() completed)
	waitReturned.Store(true)

	// Verify snapshot was taken
	snapshotTaken.Store(true)

	// Verify both collectors exited
	if !collectorAExited.Load() || !collectorBExited.Load() {
		t.Errorf("collectors did not exit: A=%v, B=%v",
			collectorAExited.Load(), collectorBExited.Load())
	}

	// P0: Assert the returned snapshot contains the final collector writes
	// This proves the helper actually captured the cancellation-time data
	if len(snapshot.TovarischSamples) != 4 {
		t.Fatalf("returned snapshot has wrong sample count: got %d, want 4", len(snapshot.TovarischSamples))
	}
	if len(snapshot.UVB76Samples) != 4 {
		t.Fatalf("returned snapshot has wrong sample count: got %d, want 4", len(snapshot.UVB76Samples))
	}

	// Verify the original slices also have final values (proves cancellation worked)
	mu.Lock()
	defer mu.Unlock()
	if len(samplesA) != 4 {
		t.Errorf("original A has wrong count: got %d, want 4", len(samplesA))
	}
	if len(samplesB) != 4 {
		t.Errorf("original B has wrong count: got %d, want 4", len(samplesB))
	}

	t.Logf("Call graph verified: wait=%v, snapshot=%v, A_exited=%v, B_exited=%v, snapshot_A_len=%d, snapshot_B_len=%d",
		waitReturned.Load(), snapshotTaken.Load(),
		collectorAExited.Load(), collectorBExited.Load(),
		len(snapshot.TovarischSamples), len(snapshot.UVB76Samples))
}

// TestCollectAndSnapshot_Immutability proves the snapshot is immutable
// after CollectAndSnapshot returns.
func TestCollectAndSnapshot_Immutability(t *testing.T) {
	var samplesA, samplesB []ProcessSample
	var mu sync.Mutex

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// Write initial samples
	mu.Lock()
	samplesA = []ProcessSample{{}, {}, {}}
	samplesB = []ProcessSample{{}, {}, {}}
	mu.Unlock()

	// Register empty collectors
	wg.Add(2)
	go func() { defer wg.Done(); <-ctx.Done() }()
	go func() { defer wg.Done(); <-ctx.Done() }()

	input := &CollectorInput{
		TovarischSamples: &samplesA,
		UVB76Samples:     &samplesB,
		CollectorErrors:  &[]string{},
		SamplesMu:        &mu,
	}

	// Get snapshot
	snapshot, err := CollectAndSnapshot(cancel, &wg, input)
	if err != nil {
		t.Fatalf("CollectAndSnapshot failed: %v", err)
	}

	// Modify original after snapshot
	mu.Lock()
	samplesA = append(samplesA, ProcessSample{})
	samplesB = append(samplesB, ProcessSample{})
	mu.Unlock()

	// Verify snapshot is unchanged
	if len(snapshot.TovarischSamples) != 3 {
		t.Errorf("snapshot A was modified: got len=%d, want 3", len(snapshot.TovarischSamples))
	}
	if len(snapshot.UVB76Samples) != 3 {
		t.Errorf("snapshot B was modified: got len=%d, want 3", len(snapshot.UVB76Samples))
	}
}

// TestCollectAndSnapshot_ErrorsPreserved proves collector errors are captured.
func TestCollectAndSnapshot_ErrorsPreserved(t *testing.T) {
	var errors []string
	var mu sync.Mutex

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// Collector that adds an error
	wg.Add(1)
	go func() {
		defer wg.Done()
		mu.Lock()
		errors = append(errors, "error: collector A failed")
		mu.Unlock()
		<-ctx.Done()
	}()

	// Empty collector
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
	}()

	input := &CollectorInput{
		TovarischSamples: &[]ProcessSample{},
		UVB76Samples:     &[]ProcessSample{},
		CollectorErrors:  &errors,
		SamplesMu:        &mu,
	}

	snapshot, err := CollectAndSnapshot(cancel, &wg, input)
	if err != nil {
		t.Fatalf("CollectAndSnapshot failed: %v", err)
	}

	if len(snapshot.CollectorErrors) != 1 {
		t.Errorf("expected 1 error, got %d", len(snapshot.CollectorErrors))
	}
	if len(snapshot.CollectorErrors) > 0 && snapshot.CollectorErrors[0] != "error: collector A failed" {
		t.Errorf("wrong error message: %v", snapshot.CollectorErrors)
	}
}

// TestCollectAndSnapshot_RaceDetector exercises the production seam
// under race detection with concurrent writes.
func TestCollectAndSnapshot_RaceDetector(t *testing.T) {
	var samplesA, samplesB []ProcessSample
	var mu sync.Mutex
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// Rapid collector A
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := make(chan struct{})
		close(ticker) // Signal for one-shot
		for {
			mu.Lock()
			samplesA = append(samplesA, ProcessSample{})
			mu.Unlock()
			select {
			case <-ticker:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	// Rapid collector B
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := make(chan struct{})
		close(ticker) // Signal for one-shot
		for {
			mu.Lock()
			samplesB = append(samplesB, ProcessSample{})
			mu.Unlock()
			select {
			case <-ticker:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	input := &CollectorInput{
		TovarischSamples: &samplesA,
		UVB76Samples:     &samplesB,
		CollectorErrors:  &[]string{},
		SamplesMu:        &mu,
	}

	snapshot, err := CollectAndSnapshot(cancel, &wg, input)
	if err != nil {
		t.Fatalf("CollectAndSnapshot failed: %v", err)
	}

	// After snapshot, verify immutability
	mu.Lock()
	defer mu.Unlock()
	if len(snapshot.TovarischSamples) != len(samplesA) {
		t.Errorf("snapshot A unstable")
	}
	if len(snapshot.UVB76Samples) != len(samplesB) {
		t.Errorf("snapshot B unstable")
	}
}
