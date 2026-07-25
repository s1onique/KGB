// control_retry.go — Docker controller readiness retry loop.
//
// CORRECTION39: Wraps the shared canarycontrol.IsRetryable authority in a
// bounded retry loop with deterministic test injection (fake clock and
// sleeper).
//
// The loop:
//   - retries ONLY on transient classifications (connection_failed,
//     request_timeout, health_not_ready) per canarycontrol.IsRetryable;
//   - stops immediately on protocol or argument defects;
//   - respects the parent context's deadline and cancellation;
//   - reports the final typed error to the caller.

package dockerlab

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/canarycontrol"
)

// Sleeper is the injection seam for retry-loop timing.
//
// The real implementation uses time.After; tests inject a counting fake.
type Sleeper interface {
	Sleep(ctx context.Context, d time.Duration) error
}

// realSleeper is the production sleeper.
type realSleeper struct{}

func (realSleeper) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// FakeSleeper records the sleep calls without actually sleeping.
// Tests use this to avoid real-time waits.
type FakeSleeper struct {
	mu       sync.Mutex
	Calls    []time.Duration
	BlockCtx context.Context // if non-nil, this Sleep blocks until ctx is done
}

func (f *FakeSleeper) Sleep(ctx context.Context, d time.Duration) error {
	f.mu.Lock()
	f.Calls = append(f.Calls, d)
	block := f.BlockCtx
	f.mu.Unlock()
	if block != nil {
		<-block.Done()
		return block.Err()
	}
	return nil
}

// RetryPolicy controls the readiness retry loop.
type RetryPolicy struct {
	// MaxAttempts is the maximum number of attempts (including the first).
	// 0 means "no retries" (one attempt total).
	MaxAttempts int
	// InitialBackoff is the wait before the first retry.
	InitialBackoff time.Duration
	// BackoffMultiplier grows the backoff exponentially (1.0 = constant).
	BackoffMultiplier float64
	// Sleeper is the injection seam.
	Sleeper Sleeper
}

// DefaultRetryPolicy is the production retry policy.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts:       5,
		InitialBackoff:    50 * time.Millisecond,
		BackoffMultiplier: 2.0,
		Sleeper:           realSleeper{},
	}
}

// ReadinessLoop runs the readiness probe repeatedly until success,
// permanent failure, deadline, cancellation, or MaxAttempts exceeded.
//
// The probe function is responsible for invoking the recording Engine
// seam (ControlRunner.ControlProbe) and returning a typed error.
//
// Returns the envelope on success or the final typed error.
func ReadinessLoop(
	ctx context.Context,
	policy RetryPolicy,
	probe func(ctx context.Context) error,
) error {
	if policy.Sleeper == nil {
		policy.Sleeper = realSleeper{}
	}
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 1
	}
	backoff := policy.InitialBackoff

	var lastErr error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if ctx.Err() != nil {
			if lastErr != nil {
				return lastErr
			}
			return ctx.Err()
		}
		err := probe(ctx)
		lastErr = err
		if err == nil {
			return nil
		}
		if !canarycontrol.IsRetryable(err) {
			return err
		}
		if attempt == policy.MaxAttempts {
			return err
		}
		// Wait before the next attempt.
		if err := policy.Sleeper.Sleep(ctx, backoff); err != nil {
			if lastErr != nil {
				return lastErr
			}
			return err
		}
		if policy.BackoffMultiplier > 1.0 {
			backoff = time.Duration(float64(backoff) * policy.BackoffMultiplier)
		}
	}
	return lastErr
}

// IsContextCanceled reports whether err is a context cancellation/deadline.
func IsContextCanceled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
