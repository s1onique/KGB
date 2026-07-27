// record_outcome_test.go — Tests for RecordOutcome fixture builder.
//
// Reference: ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION51-CORRECTION03
package evidence

import (
	"testing"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
)

// TestRecordOutcome_ExactCompletePhaseSequence verifies the full phase sequence.
func TestRecordOutcome_ExactCompletePhaseSequence(t *testing.T) {
	outcome := RecordOutcome(true, true, true)

	want := []dockerlab.QualifiedLifecyclePhase{
		dockerlab.PhasePrepared,
		dockerlab.PhaseStarted,
		dockerlab.PhaseWorkloadEntered,
		dockerlab.PhaseWorkloadObserved,
		dockerlab.PhaseWorkloadReturned,
		dockerlab.PhaseTerminalObserved,
		dockerlab.PhaseContainerRemoved,
		dockerlab.PhaseNetworkRemoved,
		dockerlab.PhaseLifecycleReturned,
	}

	if len(outcome.Phases) != len(want) {
		t.Fatalf("phase count: got %d, want %d", len(outcome.Phases), len(want))
	}

	for i := range want {
		if outcome.Phases[i] != want[i] {
			t.Errorf("phase[%d]: got %v, want %v", i, outcome.Phases[i], want[i])
		}
	}
}

// TestRecordOutcome_NoDuplicatePhases verifies no phase is duplicated.
func TestRecordOutcome_NoDuplicatePhases(t *testing.T) {
	outcome := RecordOutcome(true, true, true)

	seen := make(map[dockerlab.QualifiedLifecyclePhase]bool)
	for _, phase := range outcome.Phases {
		if seen[phase] {
			t.Errorf("duplicate phase: %v", phase)
		}
		seen[phase] = true
	}
}

// TestRecordOutcome_NonTerminalOmitsTerminalPhase verifies terminal phase is omitted.
func TestRecordOutcome_NonTerminalOmitsTerminalPhase(t *testing.T) {
	outcome := RecordOutcome(false, true, true)

	for _, phase := range outcome.Phases {
		if phase == dockerlab.PhaseTerminalObserved {
			t.Error("PhaseTerminalObserved should not be present when terminal=false")
		}
	}

	if outcome.Terminal {
		t.Error("Terminal should be false")
	}
}

// TestRecordOutcome_ContainerPresentOmitsContainerRemovedPhase verifies container_removed phase.
func TestRecordOutcome_ContainerPresentOmitsContainerRemovedPhase(t *testing.T) {
	outcome := RecordOutcome(true, false, true)

	for _, phase := range outcome.Phases {
		if phase == dockerlab.PhaseContainerRemoved {
			t.Error("PhaseContainerRemoved should not be present when containerRemoved=false")
		}
	}

	if outcome.ContainerRemoved {
		t.Error("ContainerRemoved should be false")
	}
}

// TestRecordOutcome_NetworkPresentOmitsNetworkRemovedPhase verifies network_removed phase.
func TestRecordOutcome_NetworkPresentOmitsNetworkRemovedPhase(t *testing.T) {
	outcome := RecordOutcome(true, true, false)

	for _, phase := range outcome.Phases {
		if phase == dockerlab.PhaseNetworkRemoved {
			t.Error("PhaseNetworkRemoved should not be present when networkRemoved=false")
		}
	}

	if outcome.NetworkRemoved {
		t.Error("NetworkRemoved should be false")
	}
}

// TestRecordOutcome_AllFieldsPopulated verifies required fields are populated.
func TestRecordOutcome_AllFieldsPopulated(t *testing.T) {
	outcome := RecordOutcome(true, true, true)

	if outcome.Observations == nil {
		t.Fatal("Observations is nil")
	}
	if outcome.Observations.SchemaVersion != dockerlab.QualifiedExecutionObservationsSchemaVersion {
		t.Error("SchemaVersion mismatch")
	}
	if outcome.Observations.Image.RequestedReference == "" {
		t.Error("Image.RequestedReference is empty")
	}
	if outcome.Observations.Container.ID == "" {
		t.Error("Container.ID is empty")
	}
	if outcome.Observations.Network.RequestedName == "" {
		t.Error("Network.RequestedName is empty")
	}
}
