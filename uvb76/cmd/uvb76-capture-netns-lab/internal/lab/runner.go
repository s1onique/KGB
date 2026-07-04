// Package lab provides the netns-based capture lab orchestrator.
package lab

import (
	"context"
	"time"
)

// CommandRunner executes external commands with context/timeout.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) CommandResult
}

// CommandResult captures the outcome of a command execution.
type CommandResult struct {
	Command  []string
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
	Started  time.Time
	Ended    time.Time
}

// Duration returns the execution duration.
func (r CommandResult) Duration() time.Duration {
	return r.Ended.Sub(r.Started)
}

// OK returns true if the command succeeded.
func (r CommandResult) OK() bool {
	return r.Err == nil && r.ExitCode == 0
}

