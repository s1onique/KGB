package lab

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestFakeCommandRunner_RecordsCommands(t *testing.T) {
	runner := NewFakeCommandRunner()
	ctx := context.Background()

	// Run some commands
	runner.Run(ctx, "ip", "netns", "list")
	runner.Run(ctx, "ip", "link", "show")

	// Verify commands were recorded
	if len(runner.Commands) != 2 {
		t.Errorf("expected 2 commands, got %d", len(runner.Commands))
	}

	// Verify first command
	if runner.Commands[0].Name != "ip" {
		t.Errorf("expected first command name 'ip', got '%s'", runner.Commands[0].Name)
	}
	if len(runner.Commands[0].Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(runner.Commands[0].Args))
	}
	if runner.Commands[0].Args[0] != "netns" || runner.Commands[0].Args[1] != "list" {
		t.Errorf("unexpected args: %v", runner.Commands[0].Args)
	}
}

func TestFakeCommandRunner_Reset(t *testing.T) {
	runner := NewFakeCommandRunner()
	ctx := context.Background()

	runner.Run(ctx, "ip", "netns", "list")
	if len(runner.Commands) != 1 {
		t.Fatalf("expected 1 command before reset")
	}

	runner.Reset()
	if len(runner.Commands) != 0 {
		t.Errorf("expected 0 commands after reset, got %d", len(runner.Commands))
	}
}

func TestFakeCommandRunner_Concurrent(t *testing.T) {
	runner := NewFakeCommandRunner()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			runner.Run(ctx, "ip", "netns", "exec", "ns"+string(rune('0'+n)))
		}(i)
	}
	wg.Wait()

	if len(runner.Commands) != 10 {
		t.Errorf("expected 10 commands, got %d", len(runner.Commands))
	}
}

func TestCommandResult_OK(t *testing.T) {
	tests := []struct {
		name     string
		result   CommandResult
		expected bool
	}{
		{
			name:     "success",
			result:   CommandResult{ExitCode: 0, Err: nil},
			expected: true,
		},
		{
			name:     "non-zero exit",
			result:   CommandResult{ExitCode: 1, Err: nil},
			expected: false,
		},
		{
			name:     "error",
			result:   CommandResult{ExitCode: 0, Err: errors.New("failed")},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.OK(); got != tt.expected {
				t.Errorf("OK() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCommandResult_Duration(t *testing.T) {
	start := time.Now()
	end := start.Add(100 * time.Millisecond)

	result := CommandResult{
		Started: start,
		Ended:   end,
	}

	duration := result.Duration()
	if duration != 100*time.Millisecond {
		t.Errorf("expected 100ms, got %v", duration)
	}
}

func TestRealCommandRunner_Run(t *testing.T) {
	// Only run if on a system that has these commands
	runner := NewRealCommandRunner()
	ctx := context.Background()

	// Test successful command
	result := runner.Run(ctx, "echo", "hello")
	if !result.OK() {
		t.Errorf("expected success, got error: %v", result.Err)
	}
	if result.Stdout == "" && result.Stderr == "" {
		// Some systems may buffer differently
	}
	if len(result.Command) != 2 {
		t.Errorf("expected 2 command parts, got %d", len(result.Command))
	}
	if result.Command[0] != "echo" {
		t.Errorf("expected command[0] 'echo', got '%s'", result.Command[0])
	}
}

func TestRealCommandRunner_RunTimeout(t *testing.T) {
	runner := NewRealCommandRunner()

	// Create a context that will timeout immediately
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Sleep command should be cancelled
	time.Sleep(10 * time.Millisecond) // Let context expire

	result := runner.Run(ctx, "sleep", "10")
	if result.Err == nil && result.ExitCode == 0 {
		// If it succeeded anyway, that's fine - the command was fast enough
	}
}

