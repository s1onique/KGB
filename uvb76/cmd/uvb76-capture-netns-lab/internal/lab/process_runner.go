package lab

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

// ProcessHandle represents a running process.
type ProcessHandle struct {
	PID         int
	StopFn      func(context.Context) error
	StdoutPath  string
	StderrPath  string
}

// ProcessRunner manages long-running processes.
type ProcessRunner interface {
	Start(ctx context.Context, name string, args ...string) (*ProcessHandle, CommandResult)
	StartWithLogs(ctx context.Context, name string, args []string, stdoutPath, stderrPath string) (*ProcessHandle, CommandResult)
}

// RealProcessRunner executes real processes via os/exec.
type RealProcessRunner struct {
	mu      sync.Mutex
	process map[int]*exec.Cmd
}

// NewRealProcessRunner creates a new real process runner.
func NewRealProcessRunner() *RealProcessRunner {
	return &RealProcessRunner{
		process: make(map[int]*exec.Cmd),
	}
}

// Start begins a process and returns a handle.
func (r *RealProcessRunner) Start(ctx context.Context, name string, args ...string) (*ProcessHandle, CommandResult) {
	cmd := exec.CommandContext(ctx, name, args...)

	if err := cmd.Start(); err != nil {
		return nil, CommandResult{
			Command: append([]string{name}, args...),
			Err:     err,
			Started: time.Now(),
			Ended:   time.Now(),
		}
	}

	r.mu.Lock()
	r.process[cmd.Process.Pid] = cmd
	r.mu.Unlock()

	handle := &ProcessHandle{
		PID: cmd.Process.Pid,
		StopFn: func(ctx context.Context) error {
			r.mu.Lock()
			delete(r.process, cmd.Process.Pid)
			r.mu.Unlock()

			if cmd.Process == nil {
				return nil
			}
			return cmd.Process.Kill()
		},
	}

	return handle, CommandResult{
		Command:  append([]string{name}, args...),
		ExitCode: 0,
		Err:      nil,
		Started:  time.Now(),
		Ended:    time.Now(),
	}
}

// Stop terminates a process.
func (r *RealProcessRunner) Stop(ctx context.Context, handle *ProcessHandle) error {
	if handle == nil || handle.StopFn == nil {
		return nil
	}
	return handle.StopFn(ctx)
}

// StartWithLogs begins a process with stdout/stderr redirected to files.
func (r *RealProcessRunner) StartWithLogs(ctx context.Context, name string, args []string, stdoutPath, stderrPath string) (*ProcessHandle, CommandResult) {
	cmd := exec.CommandContext(ctx, name, args...)

	// Open log files
	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return nil, CommandResult{
			Command: append([]string{name}, args...),
			Err:     fmt.Errorf("create stdout log: %w", err),
			Started: time.Now(),
			Ended:   time.Now(),
		}
	}
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		stdoutFile.Close()
		return nil, CommandResult{
			Command: append([]string{name}, args...),
			Err:     fmt.Errorf("create stderr log: %w", err),
			Started: time.Now(),
			Ended:   time.Now(),
		}
	}

	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile

	if err := cmd.Start(); err != nil {
		stdoutFile.Close()
		stderrFile.Close()
		return nil, CommandResult{
			Command: append([]string{name}, args...),
			Err:     err,
			Started: time.Now(),
			Ended:   time.Now(),
		}
	}

	r.mu.Lock()
	r.process[cmd.Process.Pid] = cmd
	r.mu.Unlock()

	handle := &ProcessHandle{
		PID:        cmd.Process.Pid,
		StopFn:     func(ctx context.Context) error {
			r.mu.Lock()
			delete(r.process, cmd.Process.Pid)
			r.mu.Unlock()
			if cmd.Process == nil {
				return nil
			}
			return cmd.Process.Kill()
		},
		StdoutPath: stdoutPath,
		StderrPath: stderrPath,
	}

	return handle, CommandResult{
		Command:  append([]string{name}, args...),
		ExitCode: 0,
		Err:      nil,
		Started:  time.Now(),
		Ended:    time.Now(),
	}
}

// FakeProcessRunner records process starts without executing.
type FakeProcessRunner struct {
	mu      sync.Mutex
	process map[int]bool
	PIDs    []int
}

// NewFakeProcessRunner creates a new fake process runner.
func NewFakeProcessRunner() *FakeProcessRunner {
	return &FakeProcessRunner{
		process: make(map[int]bool),
		PIDs:   []int{},
	}
}

// Start records a process start.
func (r *FakeProcessRunner) Start(ctx context.Context, name string, args ...string) (*ProcessHandle, CommandResult) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.PIDs = append(r.PIDs, 1000+len(r.PIDs))
	pid := r.PIDs[len(r.PIDs)-1]
	r.process[pid] = true

	handle := &ProcessHandle{
		PID: pid,
		StopFn: func(ctx context.Context) error {
			r.mu.Lock()
			delete(r.process, pid)
			r.mu.Unlock()
			return nil
		},
	}

	return handle, CommandResult{
		Command:  append([]string{name}, args...),
		ExitCode: 0,
		Err:      nil,
		Started:  time.Now(),
		Ended:    time.Now(),
	}
}

// Stop removes a process from tracking.
func (r *FakeProcessRunner) Stop(ctx context.Context, handle *ProcessHandle) error {
	if handle == nil || handle.StopFn == nil {
		return nil
	}
	return handle.StopFn(ctx)
}

// GetPIDs returns all started PIDs.
func (r *FakeProcessRunner) GetPIDs() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	pids := make([]int, len(r.PIDs))
	copy(pids, r.PIDs)
	return pids
}

// StartWithLogs records a process start with log paths.
func (r *FakeProcessRunner) StartWithLogs(ctx context.Context, name string, args []string, stdoutPath, stderrPath string) (*ProcessHandle, CommandResult) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.PIDs = append(r.PIDs, 1000+len(r.PIDs))
	pid := r.PIDs[len(r.PIDs)-1]
	r.process[pid] = true

	handle := &ProcessHandle{
		PID:        pid,
		StopFn:     func(ctx context.Context) error { return nil },
		StdoutPath: stdoutPath,
		StderrPath: stderrPath,
	}

	return handle, CommandResult{
		Command:  append([]string{name}, args...),
		ExitCode: 0,
		Err:      nil,
		Started:  time.Now(),
		Ended:    time.Now(),
	}
}

// LoggingRunner wraps a CommandRunner to log all commands.
type LoggingRunner struct {
	Inner CommandRunner
	Logs  *[]CommandLog
}

// NewLoggingRunner creates a logging wrapper around a runner.
func NewLoggingRunner(inner CommandRunner, logs *[]CommandLog) *LoggingRunner {
	return &LoggingRunner{
		Inner: inner,
		Logs:  logs,
	}
}

// Run executes a command and logs it.
func (r *LoggingRunner) Run(ctx context.Context, name string, args ...string) CommandResult {
	res := r.Inner.Run(ctx, name, args...)
	*r.Logs = append(*r.Logs, LogCommand(res))
	return res
}

// NsRunner wraps a runner to execute commands in a namespace.
type NsRunner struct {
	Runner CommandRunner
	NS     string
}

// NewNsRunner creates a namespace-wrapped runner.
func NewNsRunner(runner CommandRunner, ns string) *NsRunner {
	return &NsRunner{
		Runner: runner,
		NS:     ns,
	}
}

// Run executes a command inside the namespace.
func (r *NsRunner) Run(ctx context.Context, name string, args ...string) CommandResult {
	fullArgs := append([]string{"netns", "exec", r.NS, name}, args...)
	return r.Runner.Run(ctx, "ip", fullArgs...)
}
