package main

import (
	"errors"
	"fmt"
	"slices"
	"sync"
)

// ErrNilCollectorDependency is returned when CollectAndSnapshot receives a nil dependency.
var ErrNilCollectorDependency = errors.New("nil collector dependency")

// CollectorSnapshot holds a storage-isolated copy of collector outputs.
// The snapshot's backing storage is independent from the collector slices,
// so later mutations to the collector slices do not affect the snapshot.
type CollectorSnapshot struct {
	TovarischSamples []ProcessSample
	UVB76Samples     []ProcessSample
	CollectorErrors  []string
}

// CollectorInput holds the mutable inputs to CollectAndSnapshot.
// Collectors write to these slices concurrently.
type CollectorInput struct {
	TovarischSamples *[]ProcessSample
	UVB76Samples     *[]ProcessSample
	CollectorErrors  *[]string
	SamplesMu        *sync.Mutex
}

// CollectAndSnapshot orchestrates collector lifecycle:
// 1. Validates all dependencies before proceeding (fail-closed)
// 2. Signals cancellation to all collectors
// 3. Waits for both collectors to call Done
// 4. Takes a storage-isolated snapshot under mutex
//
// Returns a snapshot with independent backing storage. The original
// collector slices can still be mutated by callers after return;
// the snapshot only guarantees its storage is isolated.
func CollectAndSnapshot(
	cancelFn func(), // Called to request cancellation
	wg *sync.WaitGroup, // WaitGroup both collectors must register with
	input *CollectorInput, // Mutable slices collectors write to
) (CollectorSnapshot, error) {
	// Fail-closed: validate all dependencies before proceeding
	if cancelFn == nil {
		return CollectorSnapshot{}, fmt.Errorf("%w: cancelFn", ErrNilCollectorDependency)
	}
	if wg == nil {
		return CollectorSnapshot{}, fmt.Errorf("%w: WaitGroup", ErrNilCollectorDependency)
	}
	if input == nil {
		return CollectorSnapshot{}, fmt.Errorf("%w: CollectorInput", ErrNilCollectorDependency)
	}
	if input.SamplesMu == nil {
		return CollectorSnapshot{}, fmt.Errorf("%w: SamplesMu", ErrNilCollectorDependency)
	}
	if input.TovarischSamples == nil {
		return CollectorSnapshot{}, fmt.Errorf("%w: TovarischSamples", ErrNilCollectorDependency)
	}
	if input.UVB76Samples == nil {
		return CollectorSnapshot{}, fmt.Errorf("%w: UVB76Samples", ErrNilCollectorDependency)
	}
	if input.CollectorErrors == nil {
		return CollectorSnapshot{}, fmt.Errorf("%w: CollectorErrors", ErrNilCollectorDependency)
	}

	// Step 1: Signal cancellation to all collectors
	cancelFn()

	// Step 2: Wait for both collectors to exit
	wg.Wait()

	// Step 3: Take snapshot under mutex with isolated storage
	input.SamplesMu.Lock()
	snapshot := CollectorSnapshot{
		TovarischSamples: slices.Clone(*input.TovarischSamples),
		UVB76Samples:     slices.Clone(*input.UVB76Samples),
		CollectorErrors:  slices.Clone(*input.CollectorErrors),
	}
	input.SamplesMu.Unlock()

	// Step 4: Return snapshot
	return snapshot, nil
}
