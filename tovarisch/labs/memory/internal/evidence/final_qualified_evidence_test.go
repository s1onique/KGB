package evidence

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
)

func validFinalOutcome() *dockerlab.QualifiedLifecycleOutcome {
	obs := buildValidObservations()
	obs.Provenance = dockerlab.ProvenanceBinding{}
	return &dockerlab.QualifiedLifecycleOutcome{
		ContainerID: obs.Container.ID, ImageID: obs.Image.InspectedBeforeCreate,
		NetworkID: obs.Network.InspectResponseID, Started: true, Terminal: true,
		ContainerRemoved: true, NetworkRemoved: true, StartedByRuntime: true,
		Observations: dockerlab.CloneQualifiedExecutionObservations(obs),
		Phases: []dockerlab.QualifiedLifecyclePhase{
			dockerlab.PhasePrepared, dockerlab.PhaseStarted, dockerlab.PhaseWorkloadEntered,
			dockerlab.PhaseWorkloadObserved, dockerlab.PhaseWorkloadReturned,
			dockerlab.PhaseTerminalObserved, dockerlab.PhaseContainerRemoved,
			dockerlab.PhaseNetworkRemoved, dockerlab.PhaseLifecycleReturned,
		},
	}
}

func validFinalProvenance() ControllerProvenance {
	return ControllerProvenance{
		VCSRevision: strings.Repeat("a", 40), VCSTree: strings.Repeat("b", 40), GitObjectFormat: "sha1",
		ProducerVersion: "correction46-test/1.0.0", DockerServerVersion: "27.0.0",
		ExecutableSHA256: strings.Repeat("c", 64),
	}
}

func TestProductionEvidence_ExactPhaseOrder(t *testing.T) {
	outcome := validFinalOutcome()
	if _, err := BuildAndPersistFinalQualifiedEvidence(context.Background(), outcome, validFinalProvenance(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	want := []dockerlab.QualifiedLifecyclePhase{dockerlab.PhasePrepared, dockerlab.PhaseStarted, dockerlab.PhaseWorkloadEntered, dockerlab.PhaseWorkloadObserved, dockerlab.PhaseWorkloadReturned, dockerlab.PhaseTerminalObserved, dockerlab.PhaseContainerRemoved, dockerlab.PhaseNetworkRemoved, dockerlab.PhaseLifecycleReturned, dockerlab.PhaseProvenanceStamped, dockerlab.PhaseEvidenceBuilt, dockerlab.PhaseEvidencePersisted}
	if !reflect.DeepEqual(outcome.Phases, want) {
		t.Fatalf("phases=%v", outcome.Phases)
	}
}

func TestProductionEvidence_NotBuiltInsideWorkload(t *testing.T) {
	assertPhaseBeforeBuild(t, dockerlab.PhaseWorkloadReturned)
}
func TestProductionEvidence_NotBuiltBeforeTerminal(t *testing.T) {
	assertPhaseBeforeBuild(t, dockerlab.PhaseTerminalObserved)
}
func TestProductionEvidence_NotBuiltBeforeContainerRemoval(t *testing.T) {
	assertPhaseBeforeBuild(t, dockerlab.PhaseContainerRemoved)
}
func TestProductionEvidence_NotBuiltBeforeNetworkRemoval(t *testing.T) {
	assertPhaseBeforeBuild(t, dockerlab.PhaseNetworkRemoved)
}
func assertPhaseBeforeBuild(t *testing.T, phase dockerlab.QualifiedLifecyclePhase) {
	t.Helper()
	outcome := validFinalOutcome()
	if _, err := BuildAndPersistFinalQualifiedEvidence(context.Background(), outcome, validFinalProvenance(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	index := func(want dockerlab.QualifiedLifecyclePhase) int {
		for i, got := range outcome.Phases {
			if got == want {
				return i
			}
		}
		return -1
	}
	if index(phase) < 0 || index(phase) >= index(dockerlab.PhaseEvidenceBuilt) {
		t.Fatalf("phase order=%v", outcome.Phases)
	}
}

func TestProductionEvidence_UsesOutcomeObservation(t *testing.T) {
	outcome := validFinalOutcome()
	outcome.Observations.Reachability.TargetPort = 9090
	ev, err := BuildAndPersistFinalQualifiedEvidence(context.Background(), outcome, validFinalProvenance(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if ev.Reachability.TargetPort != 9090 {
		t.Fatal("producer did not consume outcome observation")
	}
}
func TestProductionEvidence_ReachabilitySurvivesLifecycleReturn(t *testing.T) {
	outcome := validFinalOutcome()
	ev, err := BuildAndPersistFinalQualifiedEvidence(context.Background(), outcome, validFinalProvenance(), t.TempDir())
	if err != nil || !ev.Reachability.Success {
		t.Fatalf("ev=%v err=%v", ev, err)
	}
}
func TestProductionEvidence_TerminalTruthSurvivesLifecycleReturn(t *testing.T) {
	outcome := validFinalOutcome()
	ev, err := BuildAndPersistFinalQualifiedEvidence(context.Background(), outcome, validFinalProvenance(), t.TempDir())
	if err != nil || !ev.Container.TerminalStateObserved {
		t.Fatalf("ev=%v err=%v", ev, err)
	}
}
func TestProductionEvidence_ProvenanceSurvivesPersistence(t *testing.T) {
	p := validFinalProvenance()
	ev, err := BuildAndPersistFinalQualifiedEvidence(context.Background(), validFinalOutcome(), p, t.TempDir())
	if err != nil || ev.Provenance.ExecutableSHA256 != p.ExecutableSHA256 {
		t.Fatalf("ev=%v err=%v", ev, err)
	}
}
func TestFinalEvidence_ProvenanceStampedAfterLifecycle(t *testing.T) {
	assertPhaseBeforeBuild(t, dockerlab.PhaseProvenanceStamped)
}
func TestFinalEvidence_EmptyProvenanceRejectedBeforeWrite(t *testing.T) {
	dir := t.TempDir()
	_, err := BuildAndPersistFinalQualifiedEvidence(context.Background(), validFinalOutcome(), ControllerProvenance{}, dir)
	if !errors.Is(err, ErrFinalQualifiedProvenanceIncomplete) {
		t.Fatalf("err=%v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "qualified-execution-evidence.json")); !os.IsNotExist(statErr) {
		t.Fatal("passing evidence was written")
	}
}
func TestFinalEvidence_UsesRunningBinaryProvenance(t *testing.T) {
	hash, err := executableSHA256()
	if err != nil {
		t.Fatal(err)
	}
	p := validFinalProvenance()
	p.ExecutableSHA256 = hash
	ev, err := BuildAndPersistFinalQualifiedEvidence(context.Background(), validFinalOutcome(), p, t.TempDir())
	if err != nil || ev.Provenance.ExecutableSHA256 != hash {
		t.Fatalf("err=%v", err)
	}
}
func TestFinalEvidence_ProvenanceNotTakenFromWorkloadResult(t *testing.T) {
	outcome := validFinalOutcome()
	outcome.Observations.Provenance.ExecutableSHA256 = strings.Repeat("d", 64)
	p := validFinalProvenance()
	ev, err := BuildAndPersistFinalQualifiedEvidence(context.Background(), outcome, p, t.TempDir())
	if err != nil || ev.Provenance.ExecutableSHA256 != p.ExecutableSHA256 {
		t.Fatalf("err=%v", err)
	}
}
func TestLifecycleFailure_NoPassingEvidenceWritten(t *testing.T) {
	outcome := validFinalOutcome()
	outcome.Terminal = false
	dir := t.TempDir()
	_, err := BuildAndPersistFinalQualifiedEvidence(context.Background(), outcome, validFinalProvenance(), dir)
	if err == nil {
		t.Fatal("expected rejection")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "qualified-execution-evidence.json")); !os.IsNotExist(statErr) {
		t.Fatal("passing artifact exists")
	}
}
func TestLifecycleFailure_RejectedDiagnosticContainsFinalPartialObservations(t *testing.T) {
	outcome := validFinalOutcome()
	outcome.Observations.Reachability = dockerlab.ReachabilityObservations{}
	ev := BuildEvidenceFromObservations(outcome.Observations)
	ev.SetDerivedFields()
	dir := t.TempDir()
	if err := PersistQualifiedExecutionEvidence(dir, ev); err == nil {
		t.Fatal("expected rejection")
	}
	data, err := os.ReadFile(filepath.Join(dir, "qualified-execution-evidence.rejected.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), outcome.ContainerID) || !strings.Contains(string(data), "\"pass\": false") {
		t.Fatal("diagnostic lacks partial observation/rejected marker")
	}
}

func TestProductionCLIRegression_C45IncompleteEvidenceSnapshot(t *testing.T) {
	outcome := validFinalOutcome()
	callbackSnapshot := &dockerlab.QualifiedExecutionObservations{}
	lifecycleInternal := outcome.Observations
	staleOutcome := &dockerlab.QualifiedLifecycleOutcome{Observations: callbackSnapshot}
	staleEvidence := BuildEvidenceFromObservations(staleOutcome.Observations)
	t.Logf("callback_observation_address=%p lifecycle_internal_observation_address=%p outcome_observation_address=%p evidence_input_observation_address=%p", callbackSnapshot, lifecycleInternal, staleOutcome.Observations, staleOutcome.Observations)
	if staleEvidence.Container.TerminalStateObserved || staleEvidence.Provenance.SourceCommit != "" || staleEvidence.Reachability.Method != "" {
		t.Fatal("C45 fixture no longer reproduces all three missing classes")
	}
	ev, err := BuildAndPersistFinalQualifiedEvidence(context.Background(), outcome, validFinalProvenance(), t.TempDir())
	if err != nil || !ev.Container.TerminalStateObserved || ev.Provenance.SourceCommit == "" || !ev.Reachability.Success {
		t.Fatalf("corrected final snapshot incomplete: ev=%v err=%v", ev, err)
	}
}
