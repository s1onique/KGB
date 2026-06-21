package probe

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBoundedCommandRunner_Success(t *testing.T) {
	runner := NewBoundedCommandRunner()
	ctx := context.Background()

	result := runner.Run(ctx, "echo", "-n", "hello world")

	if result.Err != nil {
		t.Errorf("unexpected error: %v", result.Err)
	}
	if result.TimedOut {
		t.Error("should not timeout")
	}
	if result.Truncated {
		t.Error("should not be truncated")
	}
	if !strings.Contains(string(result.Stdout), "hello world") {
		t.Errorf("expected 'hello world', got %q", result.Stdout)
	}
}

func TestBoundedCommandRunner_Timeout(t *testing.T) {
	runner := NewBoundedCommandRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Use sleep command that will exceed the timeout
	result := runner.Run(ctx, "sleep", "1")

	if !result.TimedOut {
		t.Error("expected timeout")
	}
	// The runner may return DeadlineExceeded or ErrPingTimeout depending on context
	if result.Err != context.DeadlineExceeded && result.Err != ErrPingTimeout {
		t.Errorf("expected DeadlineExceeded or ErrPingTimeout, got %v", result.Err)
	}
}

func TestBoundedCommandRunner_ExitError(t *testing.T) {
	runner := NewBoundedCommandRunner()
	ctx := context.Background()

	// Run a command that exits with non-zero status
	result := runner.Run(ctx, "sh", "-c", "exit 42")

	if result.Err == nil {
		t.Error("expected error for non-zero exit")
	}
	var exitErr *exec.ExitError
	if !errors.As(result.Err, &exitErr) {
		t.Errorf("expected ExitError, got %T", result.Err)
	}
	if result.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", result.ExitCode)
	}
}

func TestBoundedCommandRunner_Stderr(t *testing.T) {
	runner := NewBoundedCommandRunner()
	ctx := context.Background()

	result := runner.Run(ctx, "sh", "-c", "echo error >&2")

	if !strings.Contains(string(result.Stderr), "error") {
		t.Errorf("expected 'error' in stderr, got %q", result.Stderr)
	}
}

func TestBoundedCommandRunner_Truncation(t *testing.T) {
	runner := &BoundedCommandRunner{
		maxStdoutBytes: 10,
		maxStderrBytes: 10,
	}
	ctx := context.Background()

	// Generate more than 10 bytes of output
	result := runner.Run(ctx, "echo", "-n", "this is more than ten bytes of output")

	if !result.Truncated {
		t.Error("expected truncation")
	}
	if len(result.Stdout) > 10 {
		t.Errorf("stdout should be truncated to 10 bytes, got %d", len(result.Stdout))
	}
}

// TestBoundedCommandRunner_StdoutAndStderr tests both stdout and stderr output capture.
func TestBoundedCommandRunner_StdoutAndStderr(t *testing.T) {
	runner := NewBoundedCommandRunner()
	ctx := context.Background()

	// Generate both stdout and stderr output concurrently
	result := runner.Run(ctx, "sh", "-c", "echo stdout-line && echo stderr-line >&2 && echo another-stdout")

	if result.Err != nil {
		t.Errorf("unexpected error: %v", result.Err)
	}
	if !strings.Contains(string(result.Stdout), "stdout-line") {
		t.Errorf("expected stdout-line in stdout, got %q", result.Stdout)
	}
	if !strings.Contains(string(result.Stderr), "stderr-line") {
		t.Errorf("expected stderr-line in stderr, got %q", result.Stderr)
	}
}

// TestBoundedCommandRunner_Concurrent runs multiple Run() calls in parallel.
// This test exercises concurrent use of the same BoundedCommandRunner instance,
// verifying that each Run() call maintains independent state and produces correct output.
func TestBoundedCommandRunner_Concurrent(t *testing.T) {
	runner := NewBoundedCommandRunner()
	ctx := context.Background()

	// Run multiple commands concurrently - use unique output per goroutine
	const numGoroutines = 10
	errs := make(chan error, numGoroutines)
	count := atomic.Int32{}
	count.Store(0)

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Use sh -c to create a unique string per goroutine
			result := runner.Run(ctx, "sh", "-c", "echo goroutine-"+strconv.Itoa(id))
			if result.Err != nil {
				errs <- result.Err
				return
			}
			if result.TimedOut {
				errs <- errors.New("unexpected timeout")
				return
			}
			// Verify output contains expected identifier
			expected := "goroutine-" + strconv.Itoa(id)
			if !strings.Contains(string(result.Stdout), expected) {
				errs <- fmt.Errorf("expected output containing %q, got %q", expected, result.Stdout)
				return
			}
			count.Add(1)
		}(i)
	}

	wg.Wait()
	close(errs)

	// Check no errors occurred
	errCount := 0
	for err := range errs {
		t.Errorf("concurrent run error: %v", err)
		errCount++
	}

	// Verify all goroutines completed successfully
	if count.Load() != numGoroutines {
		t.Errorf("expected %d successful runs, got %d", numGoroutines, count.Load())
	}
	if errCount > 0 {
		t.Errorf("had %d errors in concurrent execution", errCount)
	}
}

// TestBoundedCommandRunner_ConcurrentRace is specifically designed to detect data races
// when run with -race flag. It stresses the command runner with concurrent calls.
func TestBoundedCommandRunner_ConcurrentRace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping race stress test in short mode")
	}

	runner := NewBoundedCommandRunner()

	var wg sync.WaitGroup
	const numGoroutines = 20

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Mix different commands to stress different paths
			var cmd string
			switch id % 4 {
			case 0:
				cmd = "echo"
			case 1:
				cmd = "true"
			case 2:
				cmd = "false"
			case 3:
				cmd = "sh"
			}

			switch id % 4 {
			case 0:
				runner.Run(ctx, cmd, "-n", "test-output")
			case 1:
				runner.Run(ctx, cmd)
			case 2:
				runner.Run(ctx, cmd)
			case 3:
				runner.Run(ctx, cmd, "-c", "echo test >&2")
			}
		}(i)
	}

	wg.Wait()
}

// TestBoundedCommandRunner_ZeroOutput tests handling of commands with no output.
func TestBoundedCommandRunner_ZeroOutput(t *testing.T) {
	runner := NewBoundedCommandRunner()
	ctx := context.Background()

	result := runner.Run(ctx, "true")

	if result.Err != nil {
		t.Errorf("unexpected error: %v", result.Err)
	}
	if len(result.Stdout) != 0 {
		t.Errorf("expected empty stdout, got %d bytes", len(result.Stdout))
	}
	if len(result.Stderr) != 0 {
		t.Errorf("expected empty stderr, got %d bytes", len(result.Stderr))
	}
}

// TestBoundedCommandRunner_LargeOutput tests handling of commands with output larger than limit.
func TestBoundedCommandRunner_LargeOutput(t *testing.T) {
	runner := &BoundedCommandRunner{
		maxStdoutBytes: 100,
		maxStderrBytes: 50,
	}
	ctx := context.Background()

	// Generate large output (more than limits)
	result := runner.Run(ctx, "sh", "-c", "yes | head -100")

	if !result.Truncated {
		t.Error("expected truncation")
	}
	if len(result.Stdout) > runner.maxStdoutBytes {
		t.Errorf("stdout exceeded limit: got %d, max %d", len(result.Stdout), runner.maxStdoutBytes)
	}
	if len(result.Stderr) > runner.maxStderrBytes {
		t.Errorf("stderr exceeded limit: got %d, max %d", len(result.Stderr), runner.maxStderrBytes)
	}
}

// TestBoundedCommandRunner_StdoutOnly tests commands that only write to stdout.
func TestBoundedCommandRunner_StdoutOnly(t *testing.T) {
	runner := NewBoundedCommandRunner()
	ctx := context.Background()

	// Only write to stdout
	result := runner.Run(ctx, "sh", "-c", "echo all-good || true")

	if result.Err != nil {
		t.Errorf("unexpected error: %v", result.Err)
	}
	if !strings.Contains(string(result.Stdout), "all-good") {
		t.Errorf("expected all-good in stdout, got %q", result.Stdout)
	}
	if len(result.Stderr) > 0 {
		t.Errorf("expected empty stderr, got %q", result.Stderr)
	}
}

// TestBoundedCommandRunner_StderrOnly tests commands that only write to stderr.
func TestBoundedCommandRunner_StderrOnly(t *testing.T) {
	runner := NewBoundedCommandRunner()
	ctx := context.Background()

	result := runner.Run(ctx, "sh", "-c", "echo errors-here >&2 && true")

	if result.Err != nil {
		t.Errorf("unexpected error: %v", result.Err)
	}
	if len(result.Stdout) > 0 {
		t.Errorf("expected empty stdout, got %q", result.Stdout)
	}
	if !strings.Contains(string(result.Stderr), "errors-here") {
		t.Errorf("expected errors-here in stderr, got %q", result.Stderr)
	}
}
