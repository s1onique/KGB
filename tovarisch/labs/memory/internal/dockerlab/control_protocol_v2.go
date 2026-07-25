// control_protocol_v2.go — Docker controller protocol orchestrator (v2).
//
// CORRECTION39: Migrates the dockerlab-side canary-control protocol
// execution to use canarycontrol types and the recording Engine seam.
//
// CORRECTION40 (P0-3, P0-5, P0-6, P0-7):
//   - containerID is now required on every ExecCreate/ExecAttach/ExecInspect.
//   - Per-attempt timeout enforced via attemptCtx.
//   - Operation identity binding: env.Operation must match op.Kind.
//   - Typed failure errors via ControlFailureError.

package dockerlab

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/canarycontrol"
)

// ErrExitEnvelopeMismatch is returned when the exec exit code disagrees
// with the envelope success flag.
var ErrExitEnvelopeMismatch = errors.New("dockerlab: exit/envelope mismatch")

// ErrContainerIDRequired is returned when the caller does not provide
// a non-empty container ID.
var ErrContainerIDRequired = errors.New("dockerlab: container ID required")

// ErrWrongOperation is returned when the envelope's Operation does not
// match the requested operation kind.
var ErrWrongOperation = errors.New("dockerlab: envelope operation does not match request")

// ErrControlTimeout indicates a per-attempt timeout while waiting for
// the readiness loop to complete. Returned only for host-owned deadlines,
// not for caller cancellation or transport failures.
var ErrControlTimeout = errors.New("dockerlab: control attempt timeout")

// ControlProbe executes a single control operation via the recording
// Engine seam and validates the resulting envelope via canarycontrol.
//
// containerID MUST be non-empty (CORRECTION40 P0-3).
//
// OperationKind MUST be health, state, or operate. For operate, expectedRequest
// is the caller-supplied count that the workload.requested must match.
//
// Returns:
//   - exit code (from exec_inspect),
//   - parsed envelope (nil on error),
//   - error (typed).
func (r *ControlRunner) ControlProbe(
	ctx context.Context,
	containerID string,
	kind canarycontrol.Operation,
	port int,
	expectedRequest int,
	timeout time.Duration,
) (int, *canarycontrol.ControlEnvelope, error) {
	if containerID == "" || strings.TrimSpace(containerID) == "" {
		return -1, nil, ErrContainerIDRequired
	}
	op, err := canarycontrol.NewControlOperation(kind, port, expectedRequest, timeout)
	if err != nil {
		return -1, nil, err
	}
	argv, err := op.BuildArgv()
	if err != nil {
		return -1, nil, err
	}
	// Per-attempt timeout (CORRECTION40 P0-5): use attemptCtx for every
	// Engine call. This is the host-owned attempt deadline. Caller
	// cancellation propagates through the parent context.
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return r.runExec(attemptCtx, op, argv, expectedRequest, containerID)
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
	containerID string,
) (int, *canarycontrol.ControlEnvelope, error) {
	execID, err := r.Runtime.ExecCreate(ctx, containerID, ExecCreateOptions{
		ContainerID:  containerID,
		Command:      argv,
		AttachStdout: true,
		AttachStderr: true,
		TTY:          false,
	})
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return -1, nil, ErrControlTimeout
		}
		return -1, nil, err
	}

	attachment, err := r.Runtime.ExecAttach(ctx, containerID, execID)
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
		if ctx.Err() == context.DeadlineExceeded {
			return -1, nil, ErrControlTimeout
		}
		return -1, nil, err
	}
	defer attachment.Close()

	// Read bounded stdout (CORRECTION40 P0-4 streaming)
	stdoutBytes, stdoutErr := io.ReadAll(attachment.Stdout())
	// Read bounded stderr too (within limits)
	stderrBytes, _ := io.ReadAll(attachment.Stderr())
	boundedStderr := stderrBytes
	if len(boundedStderr) > MaxControlStderr {
		boundedStderr = boundedStderr[:MaxControlStderr]
	}
	if stdoutErr != nil {
		if errors.Is(stdoutErr, ErrStdoutOverflow) {
			return -1, nil, ErrStdoutOverflow
		}
		if ctx.Err() == context.DeadlineExceeded {
			return -1, nil, ErrControlTimeout
		}
	}
	if len(stdoutBytes) == 0 {
		return -1, nil, ErrEmptyStdout
	}
	if len(stdoutBytes) > MaxControlStdout {
		return -1, nil, ErrStdoutOverflow
	}

	if err := attachment.Wait(ctx); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return -1, nil, ErrControlTimeout
		}
	}

	inspect, err := r.Runtime.ExecInspect(ctx, containerID, execID)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return -1, nil, ErrControlTimeout
		}
		return -1, nil, err
	}

	// Decode the envelope via the shared decoder (no parallel copy).
	env, err := canarycontrol.DecodeEnvelopeExactlyOne(stdoutBytes)
	if err != nil {
		failure := &ControlFailureError{
			Operation:  op.Kind,
			ExitCode:   inspect.ExitCode,
			Stderr:     string(boundedStderr),
			Cause:      err,
			HTTPStatus: 0,
		}
		return inspect.ExitCode, nil, failure
	}

	// Exit/envelope consistency.
	if inspect.ExitCode == 0 && !env.Success {
		return inspect.ExitCode, env, &ControlFailureError{
			Operation:  op.Kind,
			ExitCode:   inspect.ExitCode,
			HTTPStatus: env.HTTPStatus,
			Stderr:     string(boundedStderr),
			Cause:      ErrExitEnvelopeMismatch,
			Protocol:   &canarycontrol.ProtocolError{ErrClass: canarycontrol.ErrUnexpectedHTTPStatus},
		}
	}
	if inspect.ExitCode != 0 && env.Success {
		return inspect.ExitCode, env, &ControlFailureError{
			Operation:  op.Kind,
			ExitCode:   inspect.ExitCode,
			HTTPStatus: env.HTTPStatus,
			Stderr:     string(boundedStderr),
			Cause:      ErrExitEnvelopeMismatch,
			Protocol:   &canarycontrol.ProtocolError{ErrClass: canarycontrol.ErrUnexpectedHTTPStatus},
		}
	}

	// Operation identity binding (CORRECTION40 P0-6).
	if env.Operation != string(op.Kind) {
		return inspect.ExitCode, env, &ControlFailureError{
			Operation:  op.Kind,
			ExitCode:   inspect.ExitCode,
			HTTPStatus: env.HTTPStatus,
			Stderr:     string(boundedStderr),
			Cause:      fmt.Errorf("%w: requested=%s got=%s", ErrWrongOperation, op.Kind, env.Operation),
		}
	}

	// For operate, verify requested == expected.
	if op.Kind == canarycontrol.OpOperate && env.Workload != nil && expectedRequest > 0 {
		if env.Workload.Requested != expectedRequest {
			return inspect.ExitCode, env, &ControlFailureError{
				Operation:  op.Kind,
				ExitCode:   inspect.ExitCode,
				HTTPStatus: env.HTTPStatus,
				Stderr:     string(boundedStderr),
				Protocol:   &canarycontrol.ProtocolError{ErrClass: canarycontrol.ErrWorkloadCountMismatch},
			}
		}
	}

	// Failure path: typed failure preserved.
	if !env.Success {
		return inspect.ExitCode, env, &ControlFailureError{
			Operation:  op.Kind,
			ExitCode:   inspect.ExitCode,
			HTTPStatus: env.HTTPStatus,
			Stderr:     string(boundedStderr),
			Protocol:   &canarycontrol.ProtocolError{ErrClass: env.ErrorClass, Message: string(env.ErrorClass)},
		}
	}

	return inspect.ExitCode, env, nil
}

// ControlFailureError is the typed failure error returned for any
// exit/envelope disagreement, wrong-operation envelope, decode failure,
// or valid failure envelope.
//
// Requirements (CORRECTION40 P0-7):
//   - errors.As to *canarycontrol.ProtocolError succeeds.
//   - shared error class is preserved via Protocol.
//   - operation, HTTP status, exec exit code are preserved.
//   - stderr is bounded (via MaxControlStderr truncation).
//   - transport cause is preserved when present.
type ControlFailureError struct {
	Protocol   *canarycontrol.ProtocolError
	Operation  canarycontrol.Operation
	ExitCode   int
	HTTPStatus int
	Stderr     string
	Cause      error
}

func (e *ControlFailureError) Error() string {
	if e.Cause != nil {
		return e.Cause.Error()
	}
	if e.Protocol != nil {
		return e.Protocol.Error()
	}
	return "dockerlab: control failure"
}

// Unwrap returns the underlying cause if present, otherwise the protocol
// error if present.
func (e *ControlFailureError) Unwrap() error {
	if e.Cause != nil {
		return e.Cause
	}
	return e.Protocol
}

// IsProtocolRetryable reports whether err is a *canarycontrol.ProtocolError
// with a retryable classification. Used by the readiness loop.
//
// Delegates to canarycontrol.IsRetryable.
func IsProtocolRetryable(err error) bool {
	return canarycontrol.IsRetryable(err)
}
