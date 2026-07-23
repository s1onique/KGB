// bounded_command.go — Generic Bounded Command Executor
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03-TERMINAL-QUALIFICATION01
//
// Provides a generic bounded command executor with strict lifecycle management.
// The Docker runner is a thin adapter around this executor.
//
// Key properties:
// - No shell invocation by default
// - Explicit timeout and WaitDelay control
// - Bounded stdout/stderr with overflow detection
// - Typed result semantics for all exit conditions

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// CommandLimits bounds command execution resources.
type CommandLimits struct {
	Timeout        time.Duration
	WaitDelay      time.Duration
	MaxStdoutBytes int64
	MaxStderrBytes int64
}

// DefaultCommandLimits returns sensible defaults for bounded commands.
func DefaultCommandLimits() CommandLimits {
	return CommandLimits{
		Timeout:        30 * time.Second,
		WaitDelay:      5 * time.Second,
		MaxStdoutBytes: 64 * 1024,
		MaxStderrBytes: 64 * 1024,
	}
}

// BoundedCommandResult represents the result of a bounded command execution.
type BoundedCommandResult struct {
	Stdout           []byte
	Stderr           []byte
	ExitCode         int
	TimedOut         bool
	WaitDelayExpired bool
	StdoutOverflow   bool
	StderrOverflow   bool
}

// RunBoundedCommand executes a command with strict resource bounds.
// Returns a typed result for all exit conditions including infrastructure failures.
//
// Input validation:
// - executable must be non-empty
// - timeout must be > 0
// - waitDelay must be >= 0
// - maxStdoutBytes must be > 0
// - maxStderrBytes must be > 0
func RunBoundedCommand(
	ctx context.Context,
	executable string,
	limits CommandLimits,
	args ...string,
) (BoundedCommandResult, error) {
	// Input validation
	if executable == "" {
		return BoundedCommandResult{}, errors.New("executable is empty")
	}
	if ctx == nil {
		return BoundedCommandResult{}, errors.New("context is nil")
	}
	if limits.Timeout <= 0 {
		return BoundedCommandResult{}, errors.New("timeout must be greater than zero")
	}
	if limits.WaitDelay < 0 {
		return BoundedCommandResult{}, errors.New("waitDelay must be non-negative")
	}
	if limits.MaxStdoutBytes <= 0 {
		return BoundedCommandResult{}, errors.New("maxStdoutBytes must be greater than zero")
	}
	if limits.MaxStderrBytes <= 0 {
		return BoundedCommandResult{}, errors.New("maxStderrBytes must be greater than zero")
	}

	result := BoundedCommandResult{}

	// Create deadline-bearing context from timeout
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(limits.Timeout))
	defer cancel()

	// Execute without shell
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.WaitDelay = limits.WaitDelay

	// Use bounded writers for stdout and stderr
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer

	stdoutWriter := &overflowWriter{
		w:     &stdoutBuf,
		limit: limits.MaxStdoutBytes,
	}
	stderrWriter := &overflowWriter{
		w:     &stderrBuf,
		limit: limits.MaxStderrBytes,
	}

	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

	// Start command
	startErr := cmd.Start()
	if startErr != nil {
		// Start failure is a typed error
		return result, fmt.Errorf("start %s: %w", executable, startErr)
	}

	// Wait for command with timeout
	waitErr := cmd.Wait()

	// Capture overflow state from retained writer pointers
	result.StdoutOverflow = stdoutWriter.HasOverflow()
	result.StderrOverflow = stderrWriter.HasOverflow()

	// Check for timeout via context deadline
	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
		}
	default:
	}

	// Capture output after command completes
	result.Stdout = captureBoundedOutput(&stdoutBuf, limits.MaxStdoutBytes)
	result.Stderr = captureBoundedOutput(&stderrBuf, limits.MaxStderrBytes)

	// Get exit code from wait error
	if waitErr != nil {
		// Check if this is a WaitDelay expiration
		if errors.Is(waitErr, exec.ErrWaitDelay) {
			result.WaitDelayExpired = true
			// ErrWaitDelay means process exited but output pipes exceeded WaitDelay
			// Still capture exit code if available
			var exitErr *exec.ExitError
			if errors.As(waitErr, &exitErr) {
				result.ExitCode = exitErr.ExitCode()
			}
			// Return result without error - this is a valid outcome
			return result, nil
		}

		// Check for timeout (wrapped in ExitError)
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			// If we already timed out, that's the primary condition
			if result.TimedOut {
				return result, nil
			}
			// Non-zero exit is not an error, return structured result
			return result, nil
		}

		// Other unexpected wait failure
		if result.TimedOut {
			return result, nil
		}
		return result, fmt.Errorf("wait %s: %w", executable, waitErr)
	}

	// Process exited successfully (exit code 0)
	return result, nil
}

// captureBoundedOutput safely captures buffer content up to the limit.
func captureBoundedOutput(buf *bytes.Buffer, limit int64) []byte {
	data := buf.Bytes()
	if int64(len(data)) > limit {
		return data[:limit]
	}
	return data
}

// overflowWriter wraps an io.Writer and limits bytes written.
// Tracks overflow state for detection after completion.
type overflowWriter struct {
	w        io.Writer
	limit    int64
	written  int64
	overflow bool
}

func (ow *overflowWriter) Write(p []byte) (int, error) {
	originalLen := len(p)

	remaining := ow.limit - ow.written
	if remaining <= 0 {
		ow.overflow = true
		// Report original length to avoid short-write errors in cmd.Wait
		return originalLen, nil
	}

	toWrite := p
	if int64(len(toWrite)) > remaining {
		toWrite = toWrite[:remaining]
		ow.overflow = true
	}

	// Handle nil writer gracefully (for unit tests)
	if ow.w != nil {
		n, err := ow.w.Write(toWrite)
		ow.written += int64(n)
		if err != nil {
			return 0, err
		}
	} else {
		ow.written += int64(len(toWrite))
	}

	// Report original length
	return originalLen, nil
}

func (ow *overflowWriter) HasOverflow() bool {
	return ow.overflow
}

// =============================================================================
// DOCKER ADAPTER
// Note: DockerCommandLimits, DockerCommandResult, and DefaultDockerCommandLimits
// are defined in cleanup_observation.go to maintain backward compatibility.
// This file provides RunDockerCommand which uses the types from cleanup_observation.go.
// =============================================================================

// RunDockerCommand is a thin adapter that invokes the generic executor for Docker.
// Uses types from cleanup_observation.go for compatibility.
// Applies Docker-specific defaults before delegating to the strict executor.
func RunDockerCommand(
	ctx context.Context,
	limits DockerCommandLimits,
	args ...string,
) (DockerCommandResult, error) {
	// Apply defaults if not specified
	if limits.Timeout == 0 {
		limits.Timeout = DockerObservationTimeout
	}
	if limits.WaitDelay == 0 {
		limits.WaitDelay = DockerObservationWaitDelay
	}
	if limits.MaxStdoutBytes == 0 {
		limits.MaxStdoutBytes = DockerObservationMaxStdout
	}
	if limits.MaxStderrBytes == 0 {
		limits.MaxStderrBytes = DockerObservationMaxStderr
	}

	// Convert to generic command limits
	cmdLimits := CommandLimits{
		Timeout:        limits.Timeout,
		WaitDelay:      limits.WaitDelay,
		MaxStdoutBytes: limits.MaxStdoutBytes,
		MaxStderrBytes: limits.MaxStderrBytes,
	}

	result, err := RunBoundedCommand(ctx, "docker", cmdLimits, args...)
	return DockerCommandResult{
		Stdout:           result.Stdout,
		Stderr:           result.Stderr,
		ExitCode:         result.ExitCode,
		TimedOut:         result.TimedOut,
		WaitDelayExpired: result.WaitDelayExpired, // P0-3: Project WaitDelayExpired to Docker result
		StdoutOverflow:   result.StdoutOverflow,
		StderrOverflow:   result.StderrOverflow,
	}, err
}
