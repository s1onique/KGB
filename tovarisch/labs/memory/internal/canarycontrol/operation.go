// operation.go — Host-side typed control operation.
//
// The Docker controller (internal/dockerlab) builds the executable argv via
// ControlOperation.BuildArgv(). This is the canonical authority for what
// argv is passed to docker exec.
//
// CORRECTION34: typed operation ownership moved from cmd/canary to this
// shared package so the controller cannot drift from canary-side vocabulary.

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

// Validate enforces port range, timeout, count, and operation kind before
// any Docker exec is attempted.
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
	if op.Kind == OpOperate && op.Count <= 0 {
		return fmt.Errorf("invalid count for operate: %d", op.Count)
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
// The flag order is fixed and asserted by tests. No shell, no curl, no wget,
// no additional arguments are permitted.
func (op ControlOperation) BuildArgv() []string {
	timeoutStr := op.Timeout.String()
	switch op.Kind {
	case OpHealth:
		return []string{
			CanaryExecutable,
			"control", string(OpHealth),
			"--port", fmt.Sprintf("%d", op.Port),
			"--timeout", timeoutStr,
		}
	case OpState:
		return []string{
			CanaryExecutable,
			"control", string(OpState),
			"--port", fmt.Sprintf("%d", op.Port),
			"--timeout", timeoutStr,
		}
	case OpOperate:
		return []string{
			CanaryExecutable,
			"control", string(OpOperate),
			"--port", fmt.Sprintf("%d", op.Port),
			"--count", fmt.Sprintf("%d", op.Count),
			"--timeout", timeoutStr,
		}
	}
	return nil
}
