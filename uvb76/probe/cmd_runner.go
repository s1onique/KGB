// Package probe implements independent latency probing for UVB-76.
package probe

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// CommandRunner provides bounded command execution for ICMP ping.
type CommandRunner interface {
	// Run executes a command with the given timeout and returns captured output.
	// Output is truncated if it exceeds MaxStdoutBytes or MaxStderrBytes.
	Run(ctx context.Context, name string, args ...string) CommandResult
}

// CommandResult contains the result of a bounded command execution.
type CommandResult struct {
	Stdout    []byte
	Stderr    []byte
	ExitCode  int
	TimedOut  bool
	Truncated bool
	Err       error
}

// Default limits for command output capture.
// 4 KiB is sufficient for ping output which is typically < 1 KiB.
// This prevents unbounded memory growth on constrained routers.
const (
	DefaultMaxStdoutBytes = 4096
	DefaultMaxStderrBytes = 1024
)

// BoundedCommandRunner executes commands with bounded output capture.
// Uses StdoutPipe/StderrPipe with explicit bounded reads instead of
// setting cmd.Stdout/cmd.Stderr to custom writers, which would trigger
// os/exec writerDescriptor goroutines.
//
// BoundedCommandRunner is safe for concurrent use. All mutable state is
// confined to each Run() call via local variables. The runner itself
// holds only immutable configuration (maxStdoutBytes, maxStderrBytes).
type BoundedCommandRunner struct {
	maxStdoutBytes int
	maxStderrBytes int
}

// NewBoundedCommandRunner creates a new bounded command runner with defaults.
func NewBoundedCommandRunner() *BoundedCommandRunner {
	return &BoundedCommandRunner{
		maxStdoutBytes: DefaultMaxStdoutBytes,
		maxStderrBytes: DefaultMaxStderrBytes,
	}
}

// Run executes a command with bounded output capture.
// Uses StdoutPipe/StderrPipe with explicit bounded copy loops.
// This avoids os/exec writerDescriptor goroutines that could cause
// SIGSEGV during io.copyBuffer on constrained routers.
//
// Thread-safe: all mutable state is local to this call.
// The runner's maxStdoutBytes/maxStderrBytes fields are read-only.
//
// Pipe ordering: reads complete before Wait. The child process closes
// its stdout/stderr when it exits, which causes EOF on our read end.
// After both reads return, we call Wait to reap the process.
func (r *BoundedCommandRunner) Run(ctx context.Context, name string, args ...string) CommandResult {
	cmd := exec.CommandContext(ctx, name, args...)

	// Use pipes to avoid writerDescriptor goroutines
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return CommandResult{Err: err}
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		stdoutPipe.Close()
		return CommandResult{Err: err}
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		stdoutPipe.Close()
		stderrPipe.Close()
		return CommandResult{Err: err}
	}

	// Channel for stdout goroutine result - safe for cross-goroutine communication.
	stdoutCh := make(chan struct {
		data      []byte
		truncated bool
	}, 1)

	// Separate read buffers for each pipe to avoid races.
	// Each buffer is confined to its own goroutine.
	stderrReadBuf := make([]byte, 256)

	// Read stdout with bounded copy in separate goroutine.
	// Uses a channel to safely pass result back.
	go func() {
		stdoutBuf, truncated := r.copyBoundedWithBuf(stdoutPipe, r.maxStdoutBytes, make([]byte, 256))
		stdoutCh <- struct {
			data      []byte
			truncated bool
		}{stdoutBuf, truncated}
	}()

	// Read stderr with bounded copy - runs in this goroutine.
	stderrResult, stderrTruncated := r.copyBoundedWithBuf(stderrPipe, r.maxStderrBytes, stderrReadBuf)

	// Wait for stdout goroutine to finish and collect result.
	// The channel send happens when the goroutine's copyBoundedWithBuf returns.
	// That occurs when: (a) we've read maxStdoutBytes, or (b) EOF is observed.
	// EOF is observed when the child process exits and closes its stdout,
	// which happens independently of our cmd.Wait() call.
	stdoutRes := <-stdoutCh

	// Close pipes after reads complete.
	// Most callers need not close these explicitly, but we do so defensively.
	stdoutPipe.Close()
	stderrPipe.Close()

	// Reap the process and release resources.
	waitErr := cmd.Wait()

	result := CommandResult{
		Stdout:    stdoutRes.data,
		Stderr:    stderrResult,
		Truncated: stdoutRes.truncated || stderrTruncated,
		Err:       waitErr,
	}

	// Classify errors
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	}

	if ctx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.Err = ErrPingTimeout
	}

	return result
}

// copyBoundedWithBuf reads from reader until limit is reached or EOF.
// Uses a caller-provided read buffer to avoid per-read allocation overhead.
// Returns captured bytes and whether truncation occurred.
// After hitting the limit, continues draining to prevent blocking the child process.
func (r *BoundedCommandRunner) copyBoundedWithBuf(reader io.Reader, limit int, readBuf []byte) ([]byte, bool) {
	buf := make([]byte, 0, limit)
	truncated := false

	// Read into bounded buffer
	for len(buf) < limit {
		n, err := reader.Read(readBuf)
		if n > 0 {
			if len(buf)+n > limit {
				// Take only what fits
				n = limit - len(buf)
				truncated = true
			}
			buf = append(buf, readBuf[:n]...)
			if len(buf) >= limit {
				break
			}
		}
		if err != nil {
			if err == io.EOF {
				return buf, truncated
			}
			// Non-EOF error - stop reading but still drain
			break
		}
	}

	// If we hit the limit, drain remaining output to prevent blocking the child
	if truncated || len(buf) >= limit {
		truncated = true
		// Drain any remaining data without storing it
		for {
			n, err := reader.Read(readBuf)
			if n > 0 {
				// Discard
			}
			if err != nil {
				if err == io.EOF {
					break
				}
				break
			}
		}
	}

	return buf, truncated
}

// copyBounded reads from reader until limit is reached or context expires.
// Returns captured bytes and whether truncation occurred.
// After hitting the limit, continues draining to prevent blocking the child process.
// DEPRECATED: Use copyBoundedWithBuf for better allocation efficiency.
func (r *BoundedCommandRunner) copyBounded(reader io.Reader, limit int) ([]byte, bool) {
	return r.copyBoundedWithBuf(reader, limit, make([]byte, 256))
}

// PingOSWithRunner runs a ping using the provided CommandRunner.
// This replaces PingOS for bounded command execution.
// It records telemetry to the global ICMP ping telemetry for daemon HTTP API exposure.
func PingOSWithRunner(ctx context.Context, host string, timeout time.Duration, runner CommandRunner) (time.Duration, error) {
	IncPingStarted()
	defer IncPingCompleted()

	// Record attempt in daemon telemetry
	if tm := GetGlobalICMPTelemetry(); tm != nil {
		tm.RecordAttempt()
	}

	var latency time.Duration
	var err error

	timeoutSecs := int(timeout.Seconds())
	if timeoutSecs < 1 {
		timeoutSecs = 1
	}

	// Create a context with hard timeout for the ping command
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result := runner.Run(cmdCtx, "ping", "-c", "1", "-W", strconv.Itoa(timeoutSecs), host)

	if result.TimedOut {
		IncPingTimeout()
		if tm := GetGlobalICMPTelemetry(); tm != nil {
			tm.RecordFailure("timeout")
		}
		return 0, ErrPingTimeout
	}

	if result.Truncated {
		IncPingTruncated()
	}

	if result.Err != nil {
		// Check for common error messages
		outputStr := string(result.Stdout) + string(result.Stderr)
		if strings.Contains(outputStr, "Destination Host Unreachable") ||
			strings.Contains(outputStr, "Request timeout") ||
			strings.Contains(outputStr, "100% packet loss") {
			if tm := GetGlobalICMPTelemetry(); tm != nil {
				tm.RecordFailure("unreachable")
			}
			return 0, ErrPingUnreachable
		}
		IncPingError()
		errMsg := result.Err.Error()
		if tm := GetGlobalICMPTelemetry(); tm != nil {
			tm.RecordFailure(errMsg)
		}
		return 0, result.Err
	}

	// Parse the output to extract RTT
	latency, err = parsePingOutput(string(result.Stdout))
	if err != nil {
		IncPingError()
		if tm := GetGlobalICMPTelemetry(); tm != nil {
			tm.RecordFailure(err.Error())
		}
		return 0, err
	}

	// Record success in daemon telemetry
	if tm := GetGlobalICMPTelemetry(); tm != nil {
		tm.RecordSuccess()
	}

	return latency, nil
}
