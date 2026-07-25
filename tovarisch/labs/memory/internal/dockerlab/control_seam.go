// control_seam.go — Docker controller control-exec seam.
//
// CORRECTION39: Recording Engine seam for hermetic Docker-exec proofs.
//
// This file introduces the new ControlExecRuntime interface (the recording
// seam) and a recording fake implementation. It does NOT replace the
// existing dockerlab/client.go transport. The migration is staged:
//
//   Phase A (this file): new seam + recording fake + tests.
//   Phase B (next ACT):   migrate CanaryHealthCheckViaExec etc.
//
// The seam captures every operation given to the Docker Engine, the
// configured attach flags, the captured stdout/stderr, and the exit code
// from exec_inspect.

package dockerlab

import (
	"context"
	"errors"
	"sync"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/canarycontrol"
)

// MaxControlStdout is the bounded stdout size for any single Docker-exec
// control invocation. It is the envelope maximum (64 KiB) plus a small
// margin for envelope padding permitted by the decoder.
//
// Per the CORRECTION39 P0-6 contract, stdout at exactly MaxResponseBody
// bytes of valid JSON must decode successfully. stdout beyond the cap
// must produce ErrResponseTooLarge via the shared decoder; stdout beyond
// MaxControlStdout is a transport-level overflow that fails closed
// without parsing.
const MaxControlStdout = canarycontrol.MaxResponseBody + 4*1024

// MaxControlStderr is the bounded stderr size for any single Docker-exec
// control invocation. It is intentionally smaller than stdout because
// stderr is diagnostic-only.
const MaxControlStderr = 16 * 1024

// ExecCreateOptions captures the Engine-level exec_create invocation.
type ExecCreateOptions struct {
	ContainerID  string
	Command      []string
	AttachStdout bool
	AttachStderr bool
	TTY          bool
	WorkingDir   string
}

// ExecAttachOptions captures the Engine-level exec_attach invocation.
type ExecAttachOptions struct {
	ContainerID string
	ExecID      string
	Stdout      bool
	Stderr      bool
}

// ExecInspectResult captures the Engine-level exec_inspect result.
type ExecInspectResult struct {
	ExitCode int
	Running  bool
}

// ControlExecRuntime is the narrow Docker Engine seam the control
// controller depends on. The recording fake (FakeControlExecRuntime)
// implements this interface and captures every call.
//
// A real implementation will wrap the Docker SDK's
// container_exec_create/attach/inspect calls.
type ControlExecRuntime interface {
	ExecCreate(ctx context.Context, opts ExecCreateOptions) (execID string, err error)
	ExecAttach(ctx context.Context, opts ExecAttachOptions) (stdout, stderr []byte, err error)
	ExecInspect(ctx context.Context, containerID, execID string) (ExecInspectResult, error)
}

// FakeControlExecRuntime is the recording fake.
//
// All methods are goroutine-safe via an embedded mutex. Tests may inspect
// the captured calls via the public fields and methods after the operation
// completes.
type FakeControlExecRuntime struct {
	mu sync.Mutex

	// ContainerExecCreate scripted responses
	NextCreateExecID string
	NextCreateErr    error

	// ContainerExecAttach scripted responses
	NextAttachStdout []byte
	NextAttachStderr []byte
	NextAttachErr    error

	// ContainerExecInspect scripted responses
	NextInspectResult ExecInspectResult
	NextInspectErr    error

	// Recorded calls
	Calls []RecordedCall

	// Configurable behavior flags
	AttachExceedsMaxStdout bool // when true, returns MaxControlStdout+1 bytes
	AttachExceedsMaxStderr bool // when true, returns MaxControlStderr+1 bytes
	AttachEmptyStdout      bool // when true, returns empty stdout
}

// RecordedCall is a captured Engine invocation.
type RecordedCall struct {
	Kind               string             // "ExecCreate", "ExecAttach", "ExecInspect"
	Create             *ExecCreateOptions // set when Kind == "ExecCreate"
	Attach             *ExecAttachOptions // set when Kind == "ExecAttach"
	Inspect            *string            // execID when Kind == "ExecInspect"
	InspectedContainer *string            // containerID when Kind == "ExecInspect"
}

// ErrStdoutOverflow indicates the recorder simulated a stdout overflow.
var ErrStdoutOverflow = errors.New("dockerlab: stdout overflow")

// ErrStderrOverflow indicates the recorder simulated a stderr overflow.
var ErrStderrOverflow = errors.New("dockerlab: stderr overflow")

// ErrEmptyStdout indicates the recorder simulated an empty stdout stream.
var ErrEmptyStdout = errors.New("dockerlab: empty stdout")

// ExecCreate records the invocation and returns the scripted response.
func (f *FakeControlExecRuntime) ExecCreate(ctx context.Context, opts ExecCreateOptions) (string, error) {
	f.mu.Lock()
	optsCopy := opts
	commandCopy := append([]string(nil), opts.Command...)
	optsCopy.Command = commandCopy
	f.Calls = append(f.Calls, RecordedCall{Kind: "ExecCreate", Create: &optsCopy})
	nextID := f.NextCreateExecID
	nextErr := f.NextCreateErr
	f.mu.Unlock()
	if nextErr != nil {
		return "", nextErr
	}
	if nextID == "" {
		nextID = "exec-fake-1"
	}
	return nextID, nil
}

// ExecAttach records the invocation and returns the scripted response.
func (f *FakeControlExecRuntime) ExecAttach(ctx context.Context, opts ExecAttachOptions) ([]byte, []byte, error) {
	f.mu.Lock()
	optsCopy := opts
	f.Calls = append(f.Calls, RecordedCall{Kind: "ExecAttach", Attach: &optsCopy})
	stdout := append([]byte(nil), f.NextAttachStdout...)
	stderr := append([]byte(nil), f.NextAttachStderr...)
	attachExceedsMaxStdout := f.AttachExceedsMaxStdout
	attachExceedsMaxStderr := f.AttachExceedsMaxStderr
	attachEmptyStdout := f.AttachEmptyStdout
	nextErr := f.NextAttachErr
	f.mu.Unlock()

	if attachEmptyStdout {
		return nil, stderr, ErrEmptyStdout
	}
	if attachExceedsMaxStdout {
		big := make([]byte, MaxControlStdout+1)
		return big, stderr, ErrStdoutOverflow
	}
	if attachExceedsMaxStderr {
		big := make([]byte, MaxControlStderr+1)
		return stdout, big, ErrStderrOverflow
	}
	return stdout, stderr, nextErr
}

// ExecInspect records the invocation and returns the scripted response.
func (f *FakeControlExecRuntime) ExecInspect(ctx context.Context, containerID, execID string) (ExecInspectResult, error) {
	f.mu.Lock()
	f.Calls = append(f.Calls, RecordedCall{
		Kind:               "ExecInspect",
		InspectedContainer: &containerID,
		Inspect:            &execID,
	})
	result := f.NextInspectResult
	nextErr := f.NextInspectErr
	f.mu.Unlock()
	return result, nextErr
}

// CallsByKind returns a copy of all recorded calls of a specific kind.
func (f *FakeControlExecRuntime) CallsByKind(kind string) []RecordedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]RecordedCall, 0, len(f.Calls))
	for _, c := range f.Calls {
		if c.Kind == kind {
			out = append(out, c)
		}
	}
	return out
}

// Reset clears all recorded calls and scripted responses.
func (f *FakeControlExecRuntime) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = nil
	f.NextCreateExecID = ""
	f.NextCreateErr = nil
	f.NextAttachStdout = nil
	f.NextAttachStderr = nil
	f.NextAttachErr = nil
	f.NextInspectResult = ExecInspectResult{}
	f.NextInspectErr = nil
	f.AttachExceedsMaxStdout = false
	f.AttachExceedsMaxStderr = false
	f.AttachEmptyStdout = false
}

// ArgsContainsForbidden reports whether argv contains any forbidden token.
// Forbidden tokens: sh, bash, curl, wget, nc, telnet.
// Used by hermetic Engine-argv tests.
func ArgsContainsForbidden(argv []string) bool {
	forbidden := map[string]struct{}{
		"sh": {}, "/bin/sh": {}, "bash": {}, "/bin/bash": {},
		"curl": {}, "wget": {}, "nc": {}, "telnet": {},
	}
	for _, arg := range argv {
		if _, ok := forbidden[arg]; ok {
			return true
		}
	}
	return false
}
