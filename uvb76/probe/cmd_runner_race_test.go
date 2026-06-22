package probe

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestBoundedCommandRunner_ConcurrentStdoutStderrCaptureRegression is a deterministic
// regression test for the crash in BoundedCommandRunner.copyBoundedWithBuf that occurred
// when stdout and stderr drains shared the same scratch buffer.
//
// The original bug had this shape:
//   scratch := make([]byte, 256)
//   go func() { copyBoundedWithBuf(stdout, ..., scratch) }()
//   copyBoundedWithBuf(stderr, ..., scratch)
//
// This test verifies both outputs are captured correctly and independently without
// shared-buffer corruption, even under concurrent read pressure from two background
// producers writing to stdout and stderr simultaneously.
func TestBoundedCommandRunner_ConcurrentStdoutStderrCaptureRegression(t *testing.T) {
	runner := NewBoundedCommandRunner()
	// Use tight limits to exercise truncation paths
	runner.maxStdoutBytes = 100
	runner.maxStderrBytes = 50

	ctx := context.Background()

	// Use two background shell producers for true concurrent stdout/stderr writes:
	// - Subshell 1 writes 1000 lines to stdout
	// - Subshell 2 writes 1000 lines to stderr
	// - wait ensures both complete before the command exits
	result := runner.Run(ctx, "sh", "-c",
		`( i=0; while [ $i -lt 1000 ]; do echo "stdout-line-$i"; i=$((i+1)); done ) &
		 ( i=0; while [ $i -lt 1000 ]; do echo "stderr-line-$i" >&2; i=$((i+1)); done ) &
		 wait`)

	// Verify no error
	if result.Err != nil {
		t.Errorf("unexpected error: %v", result.Err)
	}

	// Verify truncation occurred (we have far more than 100 bytes of stdout)
	if !result.Truncated {
		t.Error("expected truncation with 1000 lines of output")
	}

	// Verify stdout contains expected prefix (not corrupted by stderr buffer)
	stdoutStr := string(result.Stdout)
	if !strings.Contains(stdoutStr, "stdout-line-0") {
		t.Errorf("stdout corrupted: missing expected prefix, got %q", stdoutStr)
	}

	// Verify stderr contains expected prefix (not corrupted by stdout buffer)
	stderrStr := string(result.Stderr)
	if !strings.Contains(stderrStr, "stderr-line-0") {
		t.Errorf("stderr corrupted: missing expected prefix, got %q", stderrStr)
	}

	// Verify truncation bounds are respected
	if len(result.Stdout) > runner.maxStdoutBytes {
		t.Errorf("stdout exceeds limit: got %d bytes, max %d", len(result.Stdout), runner.maxStdoutBytes)
	}
	if len(result.Stderr) > runner.maxStderrBytes {
		t.Errorf("stderr exceeds limit: got %d bytes, max %d", len(result.Stderr), runner.maxStderrBytes)
	}

	// Verify no cross-contamination: stdout should not contain "stderr" marker
	if strings.Contains(stdoutStr, "stderr-line-") {
		t.Error("stdout contaminated with stderr content")
	}
	// Verify no cross-contamination: stderr should not contain "stdout" marker
	if strings.Contains(stderrStr, "stdout-line-") {
		t.Error("stderr contaminated with stdout content")
	}

	t.Logf("Result: stdout=%d bytes, stderr=%d bytes, truncated=%v", len(result.Stdout), len(result.Stderr), result.Truncated)
}

// TestBoundedCommandRunner_ConcurrentStdoutStderrStress is a stress test that runs
// many iterations of concurrent stdout/stderr capture to catch race conditions
// that may not manifest in single runs.
func TestBoundedCommandRunner_ConcurrentStdoutStderrStress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	runner := NewBoundedCommandRunner()
	ctx := context.Background()

	// Stress test: run many iterations to trigger race conditions
	const iterations = 100
	for i := 0; i < iterations; i++ {
		// Build the command with the actual iteration number
		cmd := fmt.Sprintf("echo stdout-stress-%d && echo stderr-stress-%d >&2", i, i)
		result := runner.Run(ctx, "sh", "-c", cmd)

		// Verify no corruption markers
		if strings.Contains(string(result.Stdout), "stderr-") {
			t.Errorf("iteration %d: stdout corrupted with stderr content", i)
		}
		if strings.Contains(string(result.Stderr), "stdout-") {
			t.Errorf("iteration %d: stderr corrupted with stdout content", i)
		}

		// Verify correct prefix
		expectedStdout := "stdout-stress-" + strconv.Itoa(i)
		expectedStderr := "stderr-stress-" + strconv.Itoa(i)
		if !strings.Contains(string(result.Stdout), expectedStdout) {
			t.Errorf("iteration %d: missing expected stdout prefix, got %q", i, result.Stdout)
		}
		if !strings.Contains(string(result.Stderr), expectedStderr) {
			t.Errorf("iteration %d: missing expected stderr prefix, got %q", i, result.Stderr)
		}
	}
}

// TestBoundedCommandRunner_ConcurrentStderrStdoutStress runs concurrent Run() calls
// from multiple goroutines, each emitting both stdout and stderr, to catch
// cross-goroutine state corruption.
func TestBoundedCommandRunner_ConcurrentStderrStdoutStress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent stress test in short mode")
	}

	runner := NewBoundedCommandRunner()

	const numGoroutines = 20
	const iterations = 50

	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines*iterations)

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				// Use fmt.Sprintf to inject Go variables into shell command
				cmd := fmt.Sprintf(
					`echo "g%d-i%d-stdout" && echo "g%d-i%d-stderr" >&2`,
					gid, i, gid, i,
				)
				result := runner.Run(ctx, "sh", "-c", cmd)
				cancel()

				if result.Err != nil {
					errCh <- fmt.Errorf("goroutine %d iteration %d: %v", gid, i, result.Err)
					continue
				}

				// Verify expected unique stdout/stderr markers
				expectedStdout := fmt.Sprintf("g%d-i%d-stdout", gid, i)
				expectedStderr := fmt.Sprintf("g%d-i%d-stderr", gid, i)
				stdoutStr := string(result.Stdout)
				stderrStr := string(result.Stderr)

				// Each goroutine/iteration pair should have clean, unique output
				if !strings.Contains(stdoutStr, expectedStdout) {
					errCh <- fmt.Errorf("goroutine %d iteration %d: missing expected stdout %q, got %q",
						gid, i, expectedStdout, stdoutStr)
				}
				if !strings.Contains(stderrStr, expectedStderr) {
					errCh <- fmt.Errorf("goroutine %d iteration %d: missing expected stderr %q, got %q",
						gid, i, expectedStderr, stderrStr)
				}
				// Verify no cross-contamination
				if strings.Contains(stdoutStr, "stderr") {
					errCh <- fmt.Errorf("goroutine %d iteration %d: stdout corrupted with stderr content", gid, i)
				}
				if strings.Contains(stderrStr, "stdout") {
					errCh <- fmt.Errorf("goroutine %d iteration %d: stderr corrupted with stdout content", gid, i)
				}
			}
		}(g)
	}

	wg.Wait()
	close(errCh)

	// Collect and report all errors
	var errors []error
	for err := range errCh {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		t.Errorf("had %d errors in concurrent stress test:", len(errors))
		for _, err := range errors {
			t.Errorf("  - %v", err)
		}
	}
}

// TestBoundedCommandRunner_BothTruncated tests the case where both stdout and stderr
// exceed their limits simultaneously, exercising the drain paths in both goroutines.
func TestBoundedCommandRunner_BothTruncated(t *testing.T) {
	runner := &BoundedCommandRunner{
		maxStdoutBytes: 10,
		maxStderrBytes: 10,
	}
	// Use a timeout to prevent hanging if output generation exceeds expectations
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Generate output on both streams
	result := runner.Run(ctx, "sh", "-c",
		`echo "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" && echo "yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy" >&2`)

	if !result.Truncated {
		t.Error("expected truncation")
	}
	if len(result.Stdout) > runner.maxStdoutBytes {
		t.Errorf("stdout exceeds limit: got %d, max %d", len(result.Stdout), runner.maxStdoutBytes)
	}
	if len(result.Stderr) > runner.maxStderrBytes {
		t.Errorf("stderr exceeds limit: got %d, max %d", len(result.Stderr), runner.maxStderrBytes)
	}
}
