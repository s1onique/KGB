package lab

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

// RealCommandRunner executes real system commands via os/exec.
type RealCommandRunner struct{}

// NewRealCommandRunner creates a new real command runner.
func NewRealCommandRunner() *RealCommandRunner {
	return &RealCommandRunner{}
}

// Run executes a command with the given context.
func (r *RealCommandRunner) Run(ctx context.Context, name string, args ...string) CommandResult {
	started := time.Now()
	cmd := exec.CommandContext(ctx, name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return CommandResult{
			Command: append([]string{name}, args...),
			Stdout:  stdout.String(),
			Stderr:  stderr.String(),
			Err:     err,
			Started: started,
			Ended:   time.Now(),
		}
	}

	err := cmd.Wait()
	ended := time.Now()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return CommandResult{
		Command:  append([]string{name}, args...),
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Err:      err,
		Started:  started,
		Ended:    ended,
	}
}

