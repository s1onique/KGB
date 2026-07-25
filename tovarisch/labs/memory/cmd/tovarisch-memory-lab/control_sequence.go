// control_sequence.go — Canonical control sequence used by the
// production CLI and the live qualification.
//
// CORRECTION45: this is the single canonical orchestrator. The
// real `runCommand` workload callback and the live qualification
// tests both call this function. There is no parallel test-only
// implementation. The sequence is exactly:
//
//	ControlProbe(containerID, health)
//	ControlProbe(containerID, state)
//	ControlProbe(containerID, operate)
//	ControlProbe(containerID, state)
//
// Each operation populates the corresponding reachability field
// on the supplied observations. The function fails closed when
// any operation fails — the canonical operation order is
// enforced and no further operations are attempted.

package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/canarycontrol"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
)

// CanonicalControlSequenceOptions binds the inputs the
// production sequence needs.
type CanonicalControlSequenceOptions struct {
	ContainerID string
	Port        int
	Operations  int
	Timeout     time.Duration
	// BeforeOperate runs after the initial state observation and before
	// operate. Production uses it to align the canonical control sequence
	// with the sampler stimulus phase without issuing a second sequence.
	BeforeOperate func(*CanaryState) error
}

// RunCanonicalControlSequence is the bounded production seam
// for the four-operation reachability flow.
//
// Calling order is fixed:
//
//  1. health
//  2. initial state
//  3. operate
//  4. final state
//
// All four operations share the same container ID. The function
// records each operation on the supplied observations and
// fails closed when any operation fails or when the inputs are
// invalid. The reachable canary state is returned as
// (*CanaryState, *WorkloadResult, *CanaryState); a nil state or
// nil workload result is returned on failure along with the
// non-nil error.
func RunCanonicalControlSequence(
	ctx context.Context,
	control *dockerlab.ControlRunner,
	observations *dockerlab.QualifiedExecutionObservations,
	options CanonicalControlSequenceOptions,
) (*CanaryState, *WorkloadResult, *CanaryState, error) {
	if ctx == nil {
		return nil, nil, nil, errors.New("canonical control sequence: context is nil")
	}
	if control == nil {
		return nil, nil, nil, errors.New("canonical control sequence: control is nil")
	}
	if observations == nil {
		return nil, nil, nil, errors.New("canonical control sequence: observations is nil")
	}
	if options.ContainerID == "" {
		return nil, nil, nil, errors.New("canonical control sequence: container ID is empty")
	}
	if options.Port <= 0 || options.Port > 65535 {
		return nil, nil, nil, fmt.Errorf("canonical control sequence: invalid port: %d", options.Port)
	}
	if options.Operations <= 0 {
		return nil, nil, nil, fmt.Errorf("canonical control sequence: invalid operations: %d", options.Operations)
	}
	if options.Timeout <= 0 {
		return nil, nil, nil, errors.New("canonical control sequence: timeout must be positive")
	}

	containerID := options.ContainerID

	// 1. Health check.
	healthExit, healthEnv, err := control.ControlProbe(ctx, containerID, canarycontrol.OpHealth, options.Port, 0, options.Timeout)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("canonical control sequence: health: %w", err)
	}
	if healthEnv == nil {
		return nil, nil, nil, errors.New("canonical control sequence: health envelope is nil")
	}
	if healthEnv.Health == nil {
		return nil, nil, nil, errors.New("canonical control sequence: health payload is nil")
	}
	observations.Reachability.Health = dockerlab.ReachabilityOperationObservation{
		Operation:         canarycontrol.OpHealth,
		ExecExitCode:      healthExit,
		HTTPStatus:        healthEnv.HTTPStatus,
		ResponseValidated: true,
		Mode:              healthEnv.Health.Mode,
	}

	// 2. Initial state.
	initialExit, initialEnv, err := control.ControlProbe(ctx, containerID, canarycontrol.OpState, options.Port, 0, options.Timeout)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("canonical control sequence: initial state: %w", err)
	}
	if initialEnv == nil {
		return nil, nil, nil, errors.New("canonical control sequence: initial state envelope is nil")
	}
	if initialEnv.State == nil {
		return nil, nil, nil, errors.New("canonical control sequence: initial state payload is nil")
	}
	initialState := canaryStateFromEnvelope(initialEnv)
	observations.Reachability.InitialState = dockerlab.ReachabilityOperationObservation{
		Operation:         canarycontrol.OpState,
		ExecExitCode:      initialExit,
		HTTPStatus:        initialEnv.HTTPStatus,
		ResponseValidated: true,
		Mode:              initialEnv.State.Mode,
	}
	if options.BeforeOperate != nil {
		if err := options.BeforeOperate(initialState); err != nil {
			return nil, nil, nil, fmt.Errorf("canonical control sequence: before operate: %w", err)
		}
	}

	// 3. Operate.
	operateExit, operateEnv, err := control.ControlProbe(ctx, containerID, canarycontrol.OpOperate, options.Port, options.Operations, options.Timeout)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("canonical control sequence: operate: %w", err)
	}
	if operateEnv == nil {
		return nil, nil, nil, errors.New("canonical control sequence: operate envelope is nil")
	}
	if operateEnv.Workload == nil {
		return nil, nil, nil, errors.New("canonical control sequence: operate payload is nil")
	}
	workloadResult := &WorkloadResult{
		Requested: operateEnv.Workload.Requested,
		Attempted: operateEnv.Workload.Attempted,
		Completed: operateEnv.Workload.Completed,
		Failed:    operateEnv.Workload.Requested - operateEnv.Workload.Completed,
		Returned:  operateEnv.Workload.Completed,
	}
	observations.Reachability.Operate = dockerlab.ReachabilityOperateObservation{
		Operation:         canarycontrol.OpOperate,
		ExecExitCode:      operateExit,
		HTTPStatus:        operateEnv.HTTPStatus,
		Requested:         operateEnv.Workload.Requested,
		Attempted:         operateEnv.Workload.Attempted,
		Completed:         operateEnv.Workload.Completed,
		ResponseValidated: true,
	}

	// 4. Final state.
	finalExit, finalEnv, err := control.ControlProbe(ctx, containerID, canarycontrol.OpState, options.Port, 0, options.Timeout)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("canonical control sequence: final state: %w", err)
	}
	if finalEnv == nil {
		return nil, nil, nil, errors.New("canonical control sequence: final state envelope is nil")
	}
	if finalEnv.State == nil {
		return nil, nil, nil, errors.New("canonical control sequence: final state payload is nil")
	}
	finalState := canaryStateFromEnvelope(finalEnv)
	observations.Reachability.FinalState = dockerlab.ReachabilityOperationObservation{
		Operation:         canarycontrol.OpState,
		ExecExitCode:      finalExit,
		HTTPStatus:        finalEnv.HTTPStatus,
		ResponseValidated: true,
		Mode:              finalEnv.State.Mode,
	}

	return initialState, workloadResult, finalState, nil
}
