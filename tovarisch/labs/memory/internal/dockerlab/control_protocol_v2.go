// control_protocol_v2.go — Docker controller protocol orchestrator (v2).
package dockerlab

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/canarycontrol"
)

var (
	ErrExitEnvelopeMismatch = errors.New("dockerlab: exit/envelope mismatch")
	ErrContainerIDRequired  = errors.New("dockerlab: container ID required")
	ErrExecIDRequired       = errors.New("dockerlab: exec ID required")
	ErrWrongOperation       = errors.New("dockerlab: envelope operation does not match request")
	ErrControlTimeout       = errors.New("dockerlab: control attempt timeout")
)

type ControlRunner struct {
	Runtime ControlExecRuntime
}

func NewControlRunner(runtime ControlExecRuntime) *ControlRunner {
	return &ControlRunner{Runtime: runtime}
}

func (r *ControlRunner) ControlProbe(ctx context.Context, containerID string, kind canarycontrol.Operation, port, expectedRequest int, timeout time.Duration) (int, *canarycontrol.ControlEnvelope, error) {
	if strings.TrimSpace(containerID) == "" {
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
	attemptCtx, cancel := context.WithTimeoutCause(ctx, timeout, ErrControlTimeout)
	defer cancel()
	return r.runExec(attemptCtx, op, argv, expectedRequest, containerID)
}

func contextFailure(ctx context.Context) error {
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return nil
}

func (r *ControlRunner) runExec(ctx context.Context, op canarycontrol.ControlOperation, argv []string, expectedRequest int, containerID string) (exitCode int, env *canarycontrol.ControlEnvelope, retErr error) {
	execID, err := r.Runtime.ExecCreate(ctx, containerID, ExecCreateOptions{Command: argv, AttachStdout: true, AttachStderr: true, TTY: false})
	if err != nil {
		if ctxErr := contextFailure(ctx); ctxErr != nil {
			return -1, nil, ctxErr
		}
		return -1, nil, err
	}
	if strings.TrimSpace(execID) == "" {
		return -1, nil, ErrExecIDRequired
	}

	attachment, err := r.Runtime.ExecAttach(ctx, containerID, execID)
	if err != nil {
		if ctxErr := contextFailure(ctx); ctxErr != nil {
			return -1, nil, ctxErr
		}
		return -1, nil, err
	}
	if attachment == nil {
		return -1, nil, errors.New("dockerlab: nil exec attachment")
	}
	defer func() {
		if closeErr := attachment.Close(); closeErr != nil {
			retErr = errors.Join(retErr, closeErr)
		}
	}()

	stdout := newBoundedWriter(MaxControlStdout, ErrStdoutOverflow)
	stderr := newBoundedWriter(MaxControlStderr, ErrStderrOverflow)
	if _, err := stdcopy.StdCopy(stdout, stderr, attachment.Reader()); err != nil {
		if ctxErr := contextFailure(ctx); ctxErr != nil {
			return -1, nil, errors.Join(ctxErr, err)
		}
		return -1, nil, err
	}
	if ctxErr := contextFailure(ctx); ctxErr != nil {
		return -1, nil, ctxErr
	}
	if len(stdout.Bytes()) == 0 {
		return -1, nil, ErrEmptyStdout
	}

	inspect, err := r.Runtime.ExecInspect(ctx, containerID, execID)
	if err != nil {
		if ctxErr := contextFailure(ctx); ctxErr != nil {
			return -1, nil, errors.Join(ctxErr, err)
		}
		return -1, nil, err
	}

	env, err = canarycontrol.DecodeEnvelopeExactlyOne(stdout.Bytes())
	if err != nil {
		var protocol *canarycontrol.ProtocolError
		if errors.As(err, &protocol) {
			return inspect.ExitCode, nil, &ControlFailureError{Operation: op.Kind, ExitCode: inspect.ExitCode, Stderr: string(stderr.Bytes()), Protocol: protocol}
		}
		return inspect.ExitCode, nil, &ControlFailureError{Operation: op.Kind, ExitCode: inspect.ExitCode, Stderr: string(stderr.Bytes()), Cause: err}
	}
	failure := func(cause error, protocol *canarycontrol.ProtocolError) (int, *canarycontrol.ControlEnvelope, error) {
		return inspect.ExitCode, env, &ControlFailureError{Operation: op.Kind, ExitCode: inspect.ExitCode, HTTPStatus: env.HTTPStatus, Stderr: string(stderr.Bytes()), Cause: cause, Protocol: protocol}
	}
	if inspect.ExitCode == 0 && !env.Success || inspect.ExitCode != 0 && env.Success {
		return failure(ErrExitEnvelopeMismatch, &canarycontrol.ProtocolError{ErrClass: canarycontrol.ErrUnexpectedHTTPStatus, Message: "exit/envelope mismatch"})
	}
	if env.Operation != string(op.Kind) {
		return failure(fmt.Errorf("%w: requested=%s got=%s", ErrWrongOperation, op.Kind, env.Operation), nil)
	}
	if op.Kind == canarycontrol.OpOperate && env.Workload != nil && expectedRequest > 0 && env.Workload.Requested != expectedRequest {
		return failure(nil, &canarycontrol.ProtocolError{ErrClass: canarycontrol.ErrWorkloadCountMismatch, Message: "requested count mismatch"})
	}
	if !env.Success {
		return failure(nil, &canarycontrol.ProtocolError{ErrClass: env.ErrorClass, Message: string(env.ErrorClass)})
	}
	return inspect.ExitCode, env, nil
}

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

func (e *ControlFailureError) Unwrap() []error {
	causes := make([]error, 0, 2)
	if e.Protocol != nil {
		causes = append(causes, e.Protocol)
	}
	if e.Cause != nil && e.Cause != e.Protocol {
		causes = append(causes, e.Cause)
	}
	return causes
}

func IsProtocolRetryable(err error) bool { return canarycontrol.IsRetryable(err) }
