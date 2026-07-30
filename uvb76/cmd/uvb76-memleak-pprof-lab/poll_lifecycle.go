// Package main provides the UVB-76 pprof memory leak lab.
//
// # Lossless Poll Lifecycle - Single Production Authority
//
// This file implements:
// P0-1: Remove generic PollFn authority, use PollInput
// P0-2: Observable poll goroutine termination
// P0-7: Lossless concurrent target polling
// P0-6: WaitGroup ownership - only collectors in WaitGroup, not polling
// P0-9: Terminal poll causes classified with errors.Join
package main

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Sentinel errors for poll lifecycle.
var (
	// ErrPollChannelNil is returned when poll result channel is nil.
	ErrPollChannelNil = errors.New("poll result channel is nil")

	// ErrPollChannelClosed is returned when channel is closed.
	ErrPollChannelClosed = errors.New("poll result channel closed")

	// ErrPollDrainTimeoutInvalid is returned when drain timeout is not positive.
	ErrPollDrainTimeoutInvalid = errors.New("poll drain timeout must be positive")

	// ErrPollCancelled is returned when poll context is cancelled.
	ErrPollCancelled = errors.New("poll context cancelled")

	// ErrPollResultMissing is returned when result is not received before deadline.
	ErrPollResultMissing = errors.New("poll result missing")

	// ErrPollReceiveDeadline is returned when receive deadline is exceeded.
	ErrPollReceiveDeadline = errors.New("poll result receive deadline")

	// ErrPollGoroutineNotTerminated is returned when poll goroutine does not terminate.
	ErrPollGoroutineNotTerminated = errors.New("poll goroutine did not terminate")

	// ErrCollectorInputNil is returned when collector input is nil.
	ErrCollectorInputNil = errors.New("collector input is nil")

	// ErrProfileCaptureInterrupted is returned when profile capture is interrupted.
	ErrProfileCaptureInterrupted = errors.New("profile capture interrupted")

	// ErrPollLifecycleInput is returned for invalid lifecycle input.
	ErrPollLifecycleInput = errors.New("poll lifecycle input error")
)

// CollectionLifecycleInput contains all inputs for the collection phase.
type CollectionLifecycleInput struct {
	// ObservationCtx is the context for observation phase (collection + polling).
	// P0-7: Must derive from labCtx with observationEnd deadline.
	// P0-7: Must not be nil; validated before goroutine launch.
	ObservationCtx context.Context

	// ProfileCtx is the context for profile capture phase.
	// P0-7: Must derive from labCtx with finalProfileDeadline.
	// P0-7: Must not be nil; validated before goroutine launch.
	// P0-7: ProfileCtx outlives ObservationCtx to avoid deadline collision.
	ProfileCtx context.Context

	// ObservationCancel is the cancellation function for observation phase.
	// P0-7: Must not be nil; validated before goroutine launch.
	ObservationCancel context.CancelFunc

	// WaitGroup for collector goroutines (not for polling).
	// P0-6: CollectAndSnapshot is the sole WaitGroup owner.
	// P0-7: Must not be nil; validated before goroutine launch.
	WaitGroup *sync.WaitGroup

	// CollectorInput contains the collector state for CollectAndSnapshot.
	// P0-6: Must be populated with real sample slices, errors, and mutex.
	// P0-7: Must not be nil; validated before goroutine launch.
	CollectorInput *CollectorInput

	// PollInput contains the inputs for PollTargetAuthority.
	// P0-1: Production poll authority - exact PollTargetAuthority implementation.
	// P0-1: Tests inject behavior through httptest.Server/http.Client.
	PollInput TargetPollInput

	// PollDrainTimeout is the maximum time to wait for poll result after cancellation.
	// P0-7: Must be positive; validated before goroutine launch.
	PollDrainTimeout time.Duration

	// CaptureProfilesFn captures profiles and returns any error.
	// P0-7: Called synchronously with ProfileCtx; caller waits for completion.
	// P0-7: Must not be nil; validated before goroutine launch.
	CaptureProfilesFn func(context.Context) error
}

// CollectionLifecycleResult contains results from the collection phase.
type CollectionLifecycleResult struct {
	// PollResult is the final poll result.
	PollResult TargetPollResult

	// PollTerminalError is any terminal error from polling.
	PollTerminalError error

	// PollResultReceived is true if poll result was received before drain timeout.
	PollResultReceived bool

	// PollGoroutineTerminated is true if poll goroutine terminated.
	PollGoroutineTerminated bool

	// ProfileErr is any error from profile capture.
	ProfileErr error

	// Snapshot is the collector snapshot.
	Snapshot CollectorSnapshot

	// SnapshotErr is any error from CollectAndSnapshot.
	SnapshotErr error
}

// ErrNilObservationCtx is returned when ObservationCtx is nil.
var ErrNilObservationCtx = errors.New("ObservationCtx is required")

// ErrNilProfileCtx is returned when ProfileCtx is nil.
var ErrNilProfileCtx = errors.New("ProfileCtx is required")

// ErrNilObservationCancel is returned when ObservationCancel is nil.
var ErrNilObservationCancel = errors.New("ObservationCancel is required")

// ErrNilWaitGroup is returned when WaitGroup is nil.
var ErrNilWaitGroup = errors.New("WaitGroup is required")

// ErrNilCaptureProfilesFn is returned when CaptureProfilesFn is nil.
var ErrNilCaptureProfilesFn = errors.New("CaptureProfilesFn is required")

// RunCollectionLifecycle orchestrates concurrent collection and polling.
// P0-1: Uses exact PollTargetAuthority implementation via PollInput.
// P0-2: Observable poll goroutine termination - waits for both result and goroutine done.
// P0-7: Single production authority for poll + collection lifecycle.
// P0-7: Uses blocking send to buffered channel (capacity=1) for lossless delivery.
// P0-6: WaitGroup ownership is with CollectAndSnapshot exclusively.
// P0-7: Profile capture is synchronous; lifecycle waits for completion.
func RunCollectionLifecycle(input CollectionLifecycleInput) CollectionLifecycleResult {
	result := CollectionLifecycleResult{}

	// P0-7: Validate all inputs before launching any goroutines
	// Fail fast with typed errors for malformed inputs
	var validationErrors []error

	if input.ObservationCtx == nil {
		validationErrors = append(validationErrors, ErrNilObservationCtx)
	}
	if input.ProfileCtx == nil {
		validationErrors = append(validationErrors, ErrNilProfileCtx)
	}
	if input.ObservationCancel == nil {
		validationErrors = append(validationErrors, ErrNilObservationCancel)
	}
	if input.WaitGroup == nil {
		validationErrors = append(validationErrors, ErrNilWaitGroup)
	}
	if input.CaptureProfilesFn == nil {
		validationErrors = append(validationErrors, ErrNilCaptureProfilesFn)
	}
	if input.CollectorInput == nil {
		validationErrors = append(validationErrors, ErrCollectorInputNil)
	}
	if input.PollDrainTimeout <= 0 {
		validationErrors = append(validationErrors, ErrPollDrainTimeoutInvalid)
	}

	if len(validationErrors) > 0 {
		result.SnapshotErr = errors.Join(
			ErrPollLifecycleInput,
			errors.Join(validationErrors...),
		)
		return result
	}

	drainTimeout := input.PollDrainTimeout

	// P0-1: Create poll result channel with capacity 1 for lossless delivery
	pollResultCh := make(chan TargetPollResult, 1)

	// P0-2: Create poll done channel to observe goroutine termination
	pollDone := make(chan struct{})

	// P0-1/P0-2: Start poll goroutine - NOT part of WaitGroup
	// P0-7: Poll observes ObservationCtx which derives from labCtx
	// P0-2: Close pollDone when goroutine exits
	pollCtx, pollCancel := context.WithCancel(input.ObservationCtx)
	go func() {
		defer close(pollDone)

		// P0-1: Use exact PollTargetAuthority implementation
		pollResult := PollTargetAuthority(pollCtx, input.PollInput)
		pollResultCh <- pollResult
	}()

	// P0-6: Collector goroutines are started by the RUNNER, not the helper.
	// P0-6: This helper does NOT start collectors - runner owns collector lifecycle.
	// P0-6: Only CollectAndSnapshot (called at the end) waits for collectors.

	// P0-7: Capture profiles SYNCHRONOUSLY with ProfileCtx.
	// P0-7: ProfileCtx derives from labCtx but has extended deadline.
	// P0-7: No profile goroutine - profile capture is blocking.
	// P0-7: Lifecycle cannot return until profile capture has returned.
	result.ProfileErr = input.CaptureProfilesFn(input.ProfileCtx)

	// Cancel polling
	pollCancel()

	// P0-2: Create separate bounded drain context after cancelling poll
	// P0-2: Must observe BOTH result received AND goroutine terminated
	drainCtx, drainCancel := context.WithTimeout(context.Background(), drainTimeout)
	defer drainCancel()

	// P0-2: Wait for both result and goroutine termination
	for !(result.PollResultReceived && result.PollGoroutineTerminated) {
		select {
		case pollResult, ok := <-pollResultCh:
			if !ok {
				// P0-2: Channel closed unexpectedly
				result.PollTerminalError = errors.Join(
					ErrPollResultMissing,
					ErrPollChannelClosed,
				)
				break
			}
			result.PollResult = pollResult
			result.PollResultReceived = true

		case <-pollDone:
			result.PollGoroutineTerminated = true

		case <-drainCtx.Done():
			// P0-2: Drain timeout - neither condition met
			var drainErrs []error
			drainErrs = append(drainErrs, ErrPollReceiveDeadline)

			if !result.PollResultReceived {
				drainErrs = append(drainErrs, ErrPollResultMissing)
			}
			if !result.PollGoroutineTerminated {
				drainErrs = append(drainErrs, ErrPollGoroutineNotTerminated)
			}
			drainErrs = append(drainErrs, drainCtx.Err())

			result.PollTerminalError = errors.Join(drainErrs...)
			goto collectSnapshot
		}
	}

collectSnapshot:
	// P0-6: CollectAndSnapshot is the SOLE WaitGroup owner
	// It cancels, waits, copies, and validates the collector state
	snapshot, snapshotErr := CollectAndSnapshot(
		input.ObservationCancel,
		input.WaitGroup,
		input.CollectorInput,
	)
	result.Snapshot = snapshot
	result.SnapshotErr = snapshotErr

	return result
}

// receiveTargetPollWithTimeout receives a poll result with bounded timeout.
// P0-7: Uses caller's context for deadline, not the already-cancelled collection context.
// P0-9: Classifies terminal cause with errors.Join.
// P0-7: Fails closed on channel close.
func receiveTargetPollWithTimeout(
	ctx context.Context,
	resultCh <-chan TargetPollResult,
) (TargetPollResult, error) {
	var zero TargetPollResult

	if resultCh == nil {
		return zero, ErrPollChannelNil
	}

	select {
	case result, ok := <-resultCh:
		if !ok {
			// P0-7: Channel closed - fail closed
			return zero, errors.Join(
				ErrPollResultMissing,
				ErrPollChannelClosed,
			)
		}
		return result, nil

	case <-ctx.Done():
		// P0-9: Classify terminal cause based on context error type
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return zero, errors.Join(
				ErrPollReceiveDeadline,
				context.DeadlineExceeded,
			)
		}

		return zero, errors.Join(
			ErrPollCancelled,
			context.Canceled,
		)
	}
}
