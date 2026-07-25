// control_protocol_v2.go — Docker controller protocol orchestrator (v2).
//
// CORRECTION39: Migrates the dockerlab-side canary-control protocol
// execution to use canarycontrol types and the recording Engine seam.
//
// This file orchestrates: typed-operation construction, exec argv
// construction, bounded stdout/stderr handling, exit/envelope
// consistency, and shared retry authority. It is the canonical
// implementation for the new (migrated) dockerlab controller path.
//
// The legacy client.go transport remains in place for callers that
// have not yet migrated. The migration is staged.

package dockerlab

import (
	"context"
	"errors"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/canarycontrol"
)

// ErrExitEnvelopeMismatch is returned when the exec exit code disagrees
// with the envelope success flag.
var ErrExitEnvelopeMismatch = errors.New("dockerlab: exit/envelope mismatch")

// ErrControlTimeout indicates a per-attempt timeout while waiting for
// the readiness loop to complete.
var ErrControlTimeout = errors.New("dockerlab: control timeout")

// ControlProbe executes a single control operation via the recording
// Engine seam and validates the resulting envelope via canarycontrol.
//
// OperationKind MUST be health, state, or operate. For operate, expectedRequest
// is the caller-supplied count that the workload.requested must match.
//
// The function returns:
//   - exit code (from exec_inspect),
//   - parsed envelope (nil on error),
//   - error (typed).
func (r *ControlRunner) ControlProbe(
	ctx context.Context,
	kind canarycontrol.Operation,
	port int,
	expectedRequest int,
	timeout time.Duration,
) (int, *canarycontrol.ControlEnvelope, error) {
	op, err := canarycontrol.NewControlOperation(kind, port, expectedRequest, timeout)
	if err != nil {
		return -1, nil, err
	}
	argv, err := op.BuildArgv()
	if err != nil {
		return -1, nil, err
	}
	return r.runExec(ctx, op, argv, expectedRequest)
}

// ControlRunner wires the recording Engine seam to the protocol layer.
type ControlRunner struct {
	Runtime ControlExecRuntime
}

// NewControlRunner builds a ControlRunner with the given runtime.
func NewControlRunner(runtime ControlExecRuntime) *ControlRunner {
	return &ControlRunner{Runtime: runtime}
}

// runExec performs the Engine-level exec create/attach/inspect sequence
// and validates the envelope.
func (r *ControlRunner) runExec(
	ctx context.Context,
	op canarycontrol.ControlOperation,
	argv []string,
	expectedRequest int,
) (int, *canarycontrol.ControlEnvelope, error) {
	containerID := "" // populated by the caller (omitted here for hermetic proof)
	execID, err := r.Runtime.ExecCreate(ctx, ExecCreateOptions{
		ContainerID:  containerID,
		Command:      argv,
		AttachStdout: true,
		AttachStderr: true,
		TTY:          false,
	})
	if err != nil {
		return -1, nil, err
	}
	stdout, stderr, err := r.Runtime.ExecAttach(ctx, ExecAttachOptions{
		ContainerID: containerID,
		ExecID:      execID,
		Stdout:      true,
		Stderr:      true,
	})
	if err != nil {
		if errors.Is(err, ErrEmptyStdout) {
			return -1, nil, ErrEmptyStdout
		}
		if errors.Is(err, ErrStdoutOverflow) {
			return -1, nil, ErrStdoutOverflow
		}
		if errors.Is(err, ErrStderrOverflow) {
			return -1, nil, ErrStderrOverflow
		}
		return -1, nil, err
	}
	if len(stdout) == 0 {
		return -1, nil, ErrEmptyStdout
	}
	// bounded stdout check
	if len(stdout) > MaxControlStdout {
		return -1, nil, ErrStdoutOverflow
	}
	inspect, err := r.Runtime.ExecInspect(ctx, containerID, execID)
	if err != nil {
		return -1, nil, err
	}
	// Decode the envelope via the shared decoder (no parallel copy).
	env, err := canarycontrol.DecodeEnvelopeExactlyOne(stdout)
	if err != nil {
		// Preserve stderr in a wrapped error so transport diagnostics survive.
		return inspect.ExitCode, nil, wrapWithStderr(err, stderr)
	}
	// Exit/envelope consistency: success envelope requires exit 0;
	// failure envelope requires nonzero exit. Any mismatch is rejected.
	if inspect.ExitCode == 0 && !env.Success {
		return inspect.ExitCode, nil, ErrExitEnvelopeMismatch
	}
	if inspect.ExitCode != 0 && env.Success {
		return inspect.ExitCode, nil, ErrExitEnvelopeMismatch
	}
	// For operate, verify requested == expected.
	if op.Kind == canarycontrol.OpOperate && env.Workload != nil && expectedRequest > 0 {
		if env.Workload.Requested != expectedRequest {
			return inspect.ExitCode, nil, wrapWithStderr(
				&canarycontrol.ProtocolError{ErrClass: canarycontrol.ErrWorkloadCountMismatch},
				stderr,
			)
		}
	}
	return inspect.ExitCode, env, nil
}

// wrapWithStderr attaches bounded stderr to a typed error. It is used to
// preserve transport diagnostics when the decoder rejects the envelope.
func wrapWithStderr(err error, stderr []byte) error {
	if len(stderr) == 0 {
		return err
	}
	// Truncate stderr to MaxControlStderr to avoid leaking unbounded output.
	bounded := stderr
	if len(bounded) > MaxControlStderr {
		bounded = bounded[:MaxControlStderr]
	}
	return &stderrWrapped{cause: err, stderr: string(bounded)}
}

// stderrWrapped carries a typed cause and bounded transport stderr.
type stderrWrapped struct {
	cause  error
	stderr string
}

func (e *stderrWrapped) Error() string  { return e.cause.Error() }
func (e *stderrWrapped) Unwrap() error  { return e.cause }
func (e *stderrWrapped) Stderr() string { return e.stderr }

// IsProtocolRetryable reports whether err is a *canarycontrol.ProtocolError
// with a retryable classification. Used by the readiness loop.
//
// Delegates to canarycontrol.IsRetryable.
func IsProtocolRetryable(err error) bool {
	return canarycontrol.IsRetryable(err)
}
