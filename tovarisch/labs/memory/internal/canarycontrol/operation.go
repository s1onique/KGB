// operation.go — Host-side typed control operation.
//
// The Docker controller (internal/dockerlab) builds the executable argv via
// ControlOperation.BuildArgv(). This is the canonical authority for what
// argv is passed to docker exec.
//
// CORRECTION35: BuildArgv now fails closed. An unvalidated ControlOperation
// cannot produce executable argv. Use NewControlOperation to construct or
// BuildArgv to consume; either path goes through Validate.

package canarycontrol

import (
	"errors"
	"fmt"
	"time"
)

// ControlOperation is the typed host-side request to run a canary control
// operation in the container.
type ControlOperation struct {
	Kind    Operation
	Port    int
	Count   int           // For operate; ignored otherwise
	Timeout time.Duration // Per-attempt timeout
}

// CanaryExecutable is the in-container binary path the Docker controller
// must exec. It is intentionally non-configurable: argv must always start
// with this binary path.
const CanaryExecutable = "/app/canary"

// NewControlOperation constructs a validated ControlOperation. Returns an
// error if any argument is invalid.
func NewControlOperation(kind Operation, port, count int, timeout time.Duration) (ControlOperation, error) {
	op := ControlOperation{
		Kind:    kind,
		Port:    port,
		Count:   count,
		Timeout: timeout,
	}
	if err := op.Validate(); err != nil {
		return ControlOperation{}, err
	}
	return op, nil
}

// Validate enforces port range, timeout, count, and operation kind before
// any Docker exec is attempted.
//
// Per-operation count rules:
//   - OpHealth  : Count MUST be 0
//   - OpState   : Count MUST be 0
//   - OpOperate : Count MUST be > 0
func (op ControlOperation) Validate() error {
	if op.Kind == "" {
		return errors.New("empty operation kind")
	}
	if !IsValidOperation(op.Kind) {
		return fmt.Errorf("unknown operation kind: %q", op.Kind)
	}
	if op.Port <= 0 || op.Port > 65535 {
		return fmt.Errorf("invalid port: %d", op.Port)
	}
	if op.Timeout <= 0 {
		return fmt.Errorf("invalid timeout: %v", op.Timeout)
	}
	switch op.Kind {
	case OpHealth:
		if op.Count != 0 {
			return fmt.Errorf("health operation must have count=0, got %d", op.Count)
		}
	case OpState:
		if op.Count != 0 {
			return fmt.Errorf("state operation must have count=0, got %d", op.Count)
		}
	case OpOperate:
		if op.Count <= 0 {
			return fmt.Errorf("invalid count for operate: %d", op.Count)
		}
	}
	return nil
}

// BuildArgv constructs the exact executable + argument vector that the
// Docker controller must pass to docker exec.
//
//   - /app/canary control health  --port N --timeout D
//   - /app/canary control state   --port N --timeout D
//   - /app/canary control operate --port N --count C --timeout D
//
// BuildArgv fails closed: it calls Validate internally and returns an error
// for any unvalidated operation. The flag order is fixed.
//
// No shell, no curl, no wget, no additional arguments are permitted.
func (op ControlOperation) BuildArgv() ([]string, error) {
	if err := op.Validate(); err != nil {
		return nil, fmt.Errorf("BuildArgv failed validation: %w", err)
	}
	timeoutStr := op.Timeout.String()
	switch op.Kind {
	case OpHealth:
		return []string{
			CanaryExecutable,
			"control", string(OpHealth),
			"--port", fmt.Sprintf("%d", op.Port),
			"--timeout", timeoutStr,
		}, nil
	case OpState:
		return []string{
			CanaryExecutable,
			"control", string(OpState),
			"--port", fmt.Sprintf("%d", op.Port),
			"--timeout", timeoutStr,
		}, nil
	case OpOperate:
		return []string{
			CanaryExecutable,
			"control", string(OpOperate),
			"--port", fmt.Sprintf("%d", op.Port),
			"--count", fmt.Sprintf("%d", op.Count),
			"--timeout", timeoutStr,
		}, nil
	}
	return nil, fmt.Errorf("unreachable: validated op has unknown kind %q", op.Kind)
}
