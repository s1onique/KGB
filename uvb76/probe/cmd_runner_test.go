package probe

import (
	"context"
	"errors"
	"os/exec"
	"strings"
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
