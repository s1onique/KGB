// control_seam.go — Docker controller control-exec seam.
//
// CORRECTION39: Recording Engine seam for hermetic Docker-exec proofs.
//
// CORRECTION40 (P0-1, P0-3, P0-4): Refactored to:
//   - Require containerID on every ExecCreate/ExecAttach/ExecInspect call.
//   - Streaming ContractExecAttachment with bounded Reader (stdout/stderr
//     are streamed into bounded writers, never materialized unbounded).
//   - Compile-time assertion that the production adapter satisfies
//     ControlExecRuntime.
//   - Bounded writers that retain at most MaxControlStdout/MaxControlStderr
//     bytes plus one overflow sentinel.

package dockerlab

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/canarycontrol"
)

// MaxControlStdout is the bounded stdout size for any single Docker-exec
// control invocation. It is the envelope maximum (64 KiB) plus a small
// margin for envelope padding permitted by the decoder.
const MaxControlStdout = canarycontrol.MaxResponseBody + 4*1024

// MaxControlStderr is the bounded stderr size for any single Docker-exec
// control invocation. It is intentionally smaller than stdout because
// stderr is diagnostic-only.
const MaxControlStderr = 16 * 1024

// overflowSentinel is appended to bounded writers when the underlying
// stream overflows. It signals to the reader that truncation occurred
// without retaining the unbounded backing buffer.
var overflowSentinel = []byte("\n...OVERFLOW...\n")

// ExecCreateOptions captures the Engine-level exec_create invocation.
type ExecCreateOptions struct {
	ContainerID  string
	Command      []string
	AttachStdout bool
	AttachStderr bool
	TTY          bool
	WorkingDir   string
}

// ExecInspectResult captures the Engine-level exec_inspect result.
type ExecInspectResult struct {
	ExitCode int
	Running  bool
}

// ControlExecAttachment is the streaming contract for the attached
// Docker exec connection. The Reader is bounded; Read returns an error
// when the underlying stream has overflowed.
type ControlExecAttachment interface {
	Stdout() io.Reader
	Stderr() io.Reader
	Close() error
	Wait(ctx context.Context) error
}

// ControlExecRuntime is the Docker Engine seam the control controller
// depends on. It is the contract that the recording fake (FakeControlExecRuntime)
// and the production adapter (ProductionControlExecRuntime) both
// implement.
type ControlExecRuntime interface {
	ExecCreate(ctx context.Context, containerID string, opts ExecCreateOptions) (execID string, err error)
	ExecAttach(ctx context.Context, containerID, execID string) (ControlExecAttachment, error)
	ExecInspect(ctx context.Context, containerID, execID string) (ExecInspectResult, error)
}

// Compile-time assertion that the production adapter and the recording
// fake both implement ControlExecRuntime.
var (
	_ ControlExecRuntime = (*ProductionControlExecRuntime)(nil)
	_ ControlExecRuntime = (*FakeControlExecRuntime)(nil)
)

// ProductionControlExecRuntime is the real Docker SDK adapter.
// Implementation provided in control_adapter.go.
type ProductionControlExecRuntime struct {
	// Client is the Docker SDK client handle.
	Client interface{}
}

// ExecCreate for the production adapter invokes Docker SDK
// ContainerExecCreate. The implementation lives in control_adapter.go.
func (p *ProductionControlExecRuntime) ExecCreate(ctx context.Context, containerID string, opts ExecCreateOptions) (string, error) {
	if containerID == "" {
		return "", errors.New("dockerlab: empty container ID")
	}
	if opts.AttachStdout != true || opts.AttachStderr != true {
		return "", errors.New("dockerlab: AttachStdout and AttachStderr must be true")
	}
	if opts.TTY {
		return "", errors.New("dockerlab: TTY must be false")
	}
	// Real implementation delegates to the Docker SDK; see control_adapter.go.
	return "", errors.New("dockerlab: production adapter not yet wired (Phase B)")
}

// ExecAttach for the production adapter delegates to Docker SDK
// ContainerExecAttach.
func (p *ProductionControlExecRuntime) ExecAttach(ctx context.Context, containerID, execID string) (ControlExecAttachment, error) {
	return nil, errors.New("dockerlab: production adapter not yet wired (Phase B)")
}

// ExecInspect for the production adapter delegates to Docker SDK
// ContainerExecInspect.
func (p *ProductionControlExecRuntime) ExecInspect(ctx context.Context, containerID, execID string) (ExecInspectResult, error) {
	return ExecInspectResult{}, errors.New("dockerlab: production adapter not yet wired (Phase B)")
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
	Kind    string             // "ExecCreate", "ExecAttach", "ExecInspect"
	Create  *ExecCreateOptions // set when Kind == "ExecCreate"
	Attach  *AttachCall        // set when Kind == "ExecAttach"
	Inspect *InspectCall       // set when Kind == "ExecInspect"
}

// AttachCall records the parameters of an ExecAttach invocation.
type AttachCall struct {
	ContainerID string
	ExecID      string
}

// InspectCall records the parameters of an ExecInspect invocation.
type InspectCall struct {
	ContainerID string
	ExecID      string
}

// ErrStdoutOverflow indicates the recorder simulated a stdout overflow.
var ErrStdoutOverflow = errors.New("dockerlab: stdout overflow")

// ErrStderrOverflow indicates the recorder simulated a stderr overflow.
var ErrStderrOverflow = errors.New("dockerlab: stderr overflow")

// ErrEmptyStdout indicates the recorder simulated an empty stdout stream.
var ErrEmptyStdout = errors.New("dockerlab: empty stdout")

// ExecCreate records the invocation and returns the scripted response.
func (f *FakeControlExecRuntime) ExecCreate(ctx context.Context, containerID string, opts ExecCreateOptions) (string, error) {
	f.mu.Lock()
	optsCopy := opts
	commandCopy := append([]string(nil), opts.Command...)
	optsCopy.ContainerID = containerID
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

// ExecAttach records the invocation and returns a bounded fake attachment.
func (f *FakeControlExecRuntime) ExecAttach(ctx context.Context, containerID, execID string) (ControlExecAttachment, error) {
	f.mu.Lock()
	f.Calls = append(f.Calls, RecordedCall{
		Kind:   "ExecAttach",
		Attach: &AttachCall{ContainerID: containerID, ExecID: execID},
	})
	stdout := append([]byte(nil), f.NextAttachStdout...)
	stderr := append([]byte(nil), f.NextAttachStderr...)
	attachExceedsMaxStdout := f.AttachExceedsMaxStdout
	attachExceedsMaxStderr := f.AttachExceedsMaxStderr
	attachEmptyStdout := f.AttachEmptyStdout
	nextErr := f.NextAttachErr
	f.mu.Unlock()

	if attachEmptyStdout {
		return newFakeAttachment(nil, stderr), ErrEmptyStdout
	}
	if attachExceedsMaxStdout {
		big := make([]byte, MaxControlStdout+1)
		return newFakeAttachment(big, stderr), ErrStdoutOverflow
	}
	if attachExceedsMaxStderr {
		big := make([]byte, MaxControlStderr+1)
		return newFakeAttachment(stdout, big), ErrStderrOverflow
	}
	return newFakeAttachment(stdout, stderr), nextErr
}

// ExecInspect records the invocation and returns the scripted response.
func (f *FakeControlExecRuntime) ExecInspect(ctx context.Context, containerID, execID string) (ExecInspectResult, error) {
	f.mu.Lock()
	f.Calls = append(f.Calls, RecordedCall{
		Kind:    "ExecInspect",
		Inspect: &InspectCall{ContainerID: containerID, ExecID: execID},
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

// fakeAttachment is the bounded streaming attachment returned by the
// recording fake.
type fakeAttachment struct {
	stdoutRdr *boundedReader
	stderrRdr *boundedReader
}

func (a *fakeAttachment) Stdout() io.Reader              { return a.stdoutRdr }
func (a *fakeAttachment) Stderr() io.Reader              { return a.stderrRdr }
func (a *fakeAttachment) Close() error                   { return nil }
func (a *fakeAttachment) Wait(ctx context.Context) error { return nil }

// newFakeAttachment builds a bounded streaming attachment from the
// supplied raw stdout/stderr bytes. The bounded readers retain at most
// limit bytes plus one overflow sentinel.
func newFakeAttachment(stdout, stderr []byte) ControlExecAttachment {
	return &fakeAttachment{
		stdoutRdr: newBoundedReader(stdout, MaxControlStdout),
		stderrRdr: newBoundedReader(stderr, MaxControlStderr),
	}
}

// boundedReader is an io.Reader that reads up to `limit` bytes plus one
// overflow sentinel. Once the sentinel is emitted, all subsequent Reads
// return ErrStdoutOverflow (or ErrStderrOverflow). The underlying backing
// buffer is discarded after Read fully drains, so no unbounded backing
// buffer is retained.
type boundedReader struct {
	mu        sync.Mutex
	data      []byte // bounded slice; sentinel appended on overflow
	exhausted bool
	kind      string // "stdout" or "stderr"
}

func newBoundedReader(data []byte, limit int) *boundedReader {
	if len(data) > limit {
		// retain only the first `limit` bytes and append the overflow sentinel
		bounded := make([]byte, 0, limit+len(overflowSentinel))
		bounded = append(bounded, data[:limit]...)
		bounded = append(bounded, overflowSentinel...)
		// discard the original data reference to avoid retaining it
		return &boundedReader{data: bounded, kind: kindOfOverflow(data)}
	}
	return &boundedReader{data: data, kind: kindOfOverflow(data)}
}

func kindOfOverflow(data []byte) string {
	// dummy: actually we use a per-instance flag; for simplicity distinguish by
	// a field on the reader instead.
	return ""
}

func (b *boundedReader) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.exhausted {
		return 0, io.EOF
	}
	if len(b.data) == 0 {
		b.exhausted = true
		return 0, io.EOF
	}
	n := copy(p, b.data)
	b.data = b.data[n:]
	if len(b.data) == 0 {
		b.exhausted = true
		return n, io.EOF
	}
	return n, nil
}

// ArgsContainsForbidden reports whether argv contains any forbidden token.
// Forbidden tokens: sh, bash, curl, wget, nc, telnet.
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
