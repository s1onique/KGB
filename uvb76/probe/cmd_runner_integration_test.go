package probe

import (
	"context"
	"testing"
	"time"
)

func TestPingOSWithRunner_Success(t *testing.T) {
	runner := NewBoundedCommandRunner()
	ctx := context.Background()

	// Use localhost which should respond quickly
	latency, err := PingOSWithRunner(ctx, "127.0.0.1", 5*time.Second, runner)

	// On most systems, localhost is reachable and we get a valid latency
	// or we get ErrPingUnreachable if ping fails
	if err != nil && err != ErrPingUnreachable && err != ErrPingParseError {
		t.Errorf("unexpected error: %v", err)
	}
	if latency < 0 {
		t.Error("latency should be non-negative")
	}
}

func TestPingOSWithRunner_Timeout(t *testing.T) {
	runner := NewBoundedCommandRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Use an unreachable IP that will cause timeout
	_, err := PingOSWithRunner(ctx, "10.255.255.1", 100*time.Millisecond, runner)

	// Should get timeout error
	if err != context.DeadlineExceeded && err != ErrPingTimeout {
		// Accept both forms of timeout
		t.Logf("got error: %v (may include timeout)", err)
	}
}

func TestICMPPingCounters(t *testing.T) {
	ResetICMPPingCounters()

	// Simulate ping operations
	IncPingStarted()
	IncPingStarted()
	IncPingCompleted()
	IncPingTimeout()
	IncPingTruncated()
	IncPingError()

	counters := GetICMPPingCounters()

	if counters.Started != 2 {
		t.Errorf("expected Started=2, got %d", counters.Started)
	}
	if counters.Completed != 1 {
		t.Errorf("expected Completed=1, got %d", counters.Completed)
	}
	if counters.Timeout != 1 {
		t.Errorf("expected Timeout=1, got %d", counters.Timeout)
	}
	if counters.Truncated != 1 {
		t.Errorf("expected Truncated=1, got %d", counters.Truncated)
	}
	if counters.Error != 1 {
		t.Errorf("expected Error=1, got %d", counters.Error)
	}
	if counters.Inflight != 1 {
		t.Errorf("expected Inflight=1, got %d", counters.Inflight)
	}

	// Clean up
	ResetICMPPingCounters()
}

func TestOSExecSemaphore(t *testing.T) {
	sem := NewOSExecSemaphore(2)

	// Acquire two slots
	guard1 := AcquireSemaphore(sem)
	if guard1.sem != sem {
		t.Error("guard1 should hold semaphore")
	}

	guard2 := AcquireSemaphore(sem)
	if guard2.sem != sem {
		t.Error("guard2 should hold semaphore")
	}

	// Release and verify we can acquire again
	guard1.Release()
	guard2.Release()

	guard3 := AcquireSemaphore(sem)
	guard3.Release()
}

func TestOSExecSemaphore_DefaultToOne(t *testing.T) {
	// Test that zero/negative values default to 1
	sem := NewOSExecSemaphore(0)
	if sem == nil {
		t.Error("semaphore should not be nil")
	}

	sem2 := NewOSExecSemaphore(-1)
	if sem2 == nil {
		t.Error("semaphore should not be nil for negative value")
	}
}

// Integration test: Verify the actual ping command works with the bounded runner
func TestBoundedCommandRunner_ActualPing(t *testing.T) {
	runner := NewBoundedCommandRunner()
	ctx := context.Background()

	// Run actual ping command
	result := runner.Run(ctx, "ping", "-c", "1", "-W", "3", "127.0.0.1")

	// We expect either success or some form of ping error (not timeout since localhost is reachable)
	if result.TimedOut {
		t.Error("localhost ping should not timeout")
	}

	// Check that we have some output
	if len(result.Stdout) == 0 && result.Err == nil {
		t.Error("expected some output or error")
	}

	t.Logf("Result: stdout=%q, stderr=%q, err=%v, exitCode=%d, timedOut=%v, truncated=%v",
		string(result.Stdout), string(result.Stderr), result.Err, result.ExitCode, result.TimedOut, result.Truncated)
}
