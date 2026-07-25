// qualified_lifecycle_contract.go — Finalized lifecycle ownership contract.
package dockerlab

import (
	"context"
	"errors"
	"fmt"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/canarycontrol"
)

// QualifiedLifecyclePhase is a stable diagnostic phase emitted by the real
// lifecycle and the post-lifecycle evidence producer. Pointer identities are
// deliberately not part of this contract or persisted evidence.
type QualifiedLifecyclePhase string

const (
	PhasePrepared          QualifiedLifecyclePhase = "prepared"
	PhaseStarted           QualifiedLifecyclePhase = "started"
	PhaseWorkloadEntered   QualifiedLifecyclePhase = "workload_entered"
	PhaseWorkloadObserved  QualifiedLifecyclePhase = "workload_observed"
	PhaseWorkloadReturned  QualifiedLifecyclePhase = "workload_returned"
	PhaseTerminalObserved  QualifiedLifecyclePhase = "terminal_observed"
	PhaseContainerRemoved  QualifiedLifecyclePhase = "container_removed"
	PhaseNetworkRemoved    QualifiedLifecyclePhase = "network_removed"
	PhaseLifecycleReturned QualifiedLifecyclePhase = "lifecycle_returned"
	PhaseProvenanceStamped QualifiedLifecyclePhase = "provenance_stamped"
	PhaseEvidenceBuilt     QualifiedLifecyclePhase = "evidence_built"
	PhaseEvidencePersisted QualifiedLifecyclePhase = "evidence_persisted"
)

var (
	// ErrMissingQualifiedWorkloadResult is returned when a successful callback
	// does not return its immutable observation result.
	ErrMissingQualifiedWorkloadResult = errors.New("qualified workload returned nil result on success")
	// ErrInvalidQualifiedWorkloadObservations is stable for errors.Is callers.
	ErrInvalidQualifiedWorkloadObservations = errors.New("invalid qualified workload observations")
)

// QualifiedWorkloadInput is an immutable-by-value view of lifecycle identity.
// The workload never receives the lifecycle's canonical observation pointer.
type QualifiedWorkloadInput struct {
	ContainerID string
	ImageID     string
	NetworkID   string
}

// QualifiedWorkloadObservations contains only facts owned by the workload.
type QualifiedWorkloadObservations struct {
	Reachability ReachabilityObservations
}

// QualifiedWorkloadResult transfers workload-owned facts to the lifecycle.
type QualifiedWorkloadResult struct {
	Observations QualifiedWorkloadObservations
}

// QualifiedRunFunc is the only qualified workload callback contract.
type QualifiedRunFunc func(ctx context.Context, input QualifiedWorkloadInput) (*QualifiedWorkloadResult, error)

// CloneQualifiedExecutionObservations returns a deep copy suitable for an
// ownership transfer. All current mutable slices and maps are duplicated.
func CloneQualifiedExecutionObservations(src *QualifiedExecutionObservations) *QualifiedExecutionObservations {
	if src == nil {
		return nil
	}
	cloned := *src
	cloned.Image.InspectedRepoDigests = append([]string(nil), src.Image.InspectedRepoDigests...)
	return &cloned
}

// validateQualifiedWorkloadObservations fails before the lifecycle merge.
func validateQualifiedWorkloadObservations(obs QualifiedWorkloadObservations) error {
	r := obs.Reachability
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidQualifiedWorkloadObservations, fmt.Sprintf(format, args...))
	}
	if r.Method != ReachabilityMethodDockerExec {
		return invalid("reachability method=%q", r.Method)
	}
	if err := ValidateCanonicalNetworkIDLenient(r.NetworkID); err != nil {
		return invalid("network ID: %v", err)
	}
	if r.TargetHost != "127.0.0.1" || r.TargetPort < 1 || r.TargetPort > 65535 {
		return invalid("target=%s:%d", r.TargetHost, r.TargetPort)
	}
	check := func(name string, got ReachabilityOperationObservation, want canarycontrol.Operation) error {
		if got.Operation != want || got.ExecExitCode != 0 || got.HTTPStatus != 200 || !got.ResponseValidated || got.Mode == "" {
			return invalid("%s is incomplete", name)
		}
		return nil
	}
	if err := check("health", r.Health, canarycontrol.OpHealth); err != nil {
		return err
	}
	if err := check("initial_state", r.InitialState, canarycontrol.OpState); err != nil {
		return err
	}
	if r.Operate.Operation != canarycontrol.OpOperate || r.Operate.ExecExitCode != 0 || r.Operate.HTTPStatus != 200 ||
		r.Operate.Requested <= 0 || r.Operate.Attempted != r.Operate.Requested || r.Operate.Completed != r.Operate.Attempted || !r.Operate.ResponseValidated {
		return invalid("operate is incomplete")
	}
	if err := check("final_state", r.FinalState, canarycontrol.OpState); err != nil {
		return err
	}
	if !r.Success {
		return invalid("success=false")
	}
	return nil
}

// RecordPhase appends a diagnostic phase to the caller-owned outcome.
func (o *QualifiedLifecycleOutcome) RecordPhase(phase QualifiedLifecyclePhase) {
	if o != nil {
		o.Phases = append(o.Phases, phase)
	}
}
