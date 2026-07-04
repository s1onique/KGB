package lab

import (
	"context"
	"sync"
	"time"
)

// FakeCommandRunner records commands without executing them.
type FakeCommandRunner struct {
	mu       sync.Mutex
	Commands []RecordedCommand
	Results  map[string]CommandResult
}

// RecordedCommand records an invocation for later inspection.
type RecordedCommand struct {
	Name string
	Args []string
	Time time.Time
}

// NewFakeCommandRunner creates a new fake runner.
func NewFakeCommandRunner() *FakeCommandRunner {
	return &FakeCommandRunner{
		Results: make(map[string]CommandResult),
	}
}

// RecordCommand adds a result to return for future matching commands.
func (f *FakeCommandRunner) RecordCommand(name string, args []string, result CommandResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Commands = append(f.Commands, RecordedCommand{
		Name: name,
		Args: args,
		Time: time.Now(),
	})
}

// Run implements CommandRunner by recording the invocation.
func (f *FakeCommandRunner) Run(ctx context.Context, name string, args ...string) CommandResult {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Commands = append(f.Commands, RecordedCommand{
		Name: name,
		Args: args,
		Time: time.Now(),
	})

	// Return a default success result
	return CommandResult{
		Command:  append([]string{name}, args...),
		ExitCode: 0,
		Err:      nil,
		Started:  time.Now(),
		Ended:    time.Now(),
	}
}

// Reset clears all recorded commands.
func (f *FakeCommandRunner) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Commands = nil
}

