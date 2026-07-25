package dockerlab

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func runCorrection46Lifecycle(t *testing.T, run QualifiedRunFunc) (*QualifiedLifecycleOutcome, error) {
	t.Helper()
	fake := newRecordingDockerRuntime()
	seedValidLifecycle(t, fake)
	audited := NewAuditedDockerRuntime(fake)
	opts := LifecycleOptions{
		ImageReference: "kgb-tovarisch-canary:latest", NetworkName: "kgb-lab-net",
		ContainerName: "kgb-subject", ContainerCmd: []string{"true"},
		TerminalTimeout: time.Second, CleanupTimeout: time.Second,
		Run: run,
	}
	return executeQualifiedLifecycle(context.Background(), audited, func(context.Context, string) bool { return true }, opts)
}

func TestQualifiedLifecycle_WorkloadCannotMutateCanonicalObservation(t *testing.T) {
	var callbackInput QualifiedWorkloadInput
	outcome, err := runCorrection46Lifecycle(t, func(_ context.Context, input QualifiedWorkloadInput) (*QualifiedWorkloadResult, error) {
		callbackInput = input
		return validQualifiedWorkloadResult(input), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if callbackInput.ContainerID == "" || callbackInput.NetworkID == "" || outcome.Observations == nil {
		t.Fatal("immutable input/final observation missing")
	}
}

func TestQualifiedLifecycle_MergesWorkloadObservationsExactlyOnce(t *testing.T) {
	calls := 0
	outcome, err := runCorrection46Lifecycle(t, func(_ context.Context, input QualifiedWorkloadInput) (*QualifiedWorkloadResult, error) {
		calls++
		return validQualifiedWorkloadResult(input), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("merge source called %d times", calls)
	}
	if !outcome.Observations.Reachability.Success {
		t.Fatal("reachability did not survive lifecycle return")
	}
	want := []QualifiedLifecyclePhase{PhasePrepared, PhaseStarted, PhaseWorkloadEntered, PhaseWorkloadObserved, PhaseWorkloadReturned, PhaseTerminalObserved, PhaseContainerRemoved, PhaseNetworkRemoved, PhaseLifecycleReturned}
	if !reflect.DeepEqual(outcome.Phases, want) {
		t.Fatalf("phases=%v want=%v", outcome.Phases, want)
	}
}

func TestQualifiedLifecycle_NilWorkloadResultOnSuccessRejected(t *testing.T) {
	_, err := runCorrection46Lifecycle(t, func(context.Context, QualifiedWorkloadInput) (*QualifiedWorkloadResult, error) { return nil, nil })
	if !errors.Is(err, ErrMissingQualifiedWorkloadResult) {
		t.Fatalf("err=%v", err)
	}
}

func TestQualifiedLifecycle_InvalidWorkloadObservationsRejectedBeforeMerge(t *testing.T) {
	outcome, err := runCorrection46Lifecycle(t, func(_ context.Context, input QualifiedWorkloadInput) (*QualifiedWorkloadResult, error) {
		result := validQualifiedWorkloadResult(input)
		result.Observations.Reachability.NetworkID = "invalid"
		return result, nil
	})
	if !errors.Is(err, ErrInvalidQualifiedWorkloadObservations) {
		t.Fatalf("err=%v", err)
	}
	if outcome.Observations.Reachability.NetworkID != "" {
		t.Fatal("invalid workload observation was merged")
	}
}

func TestQualifiedLifecycleOutcome_DeepCopyMutableMembers(t *testing.T) {
	outcome, err := runCorrection46Lifecycle(t, successfulQualifiedWorkload)
	if err != nil {
		t.Fatal(err)
	}
	outcome.Observations.Image.InspectedRepoDigests = []string{"one"}
	clone := CloneQualifiedExecutionObservations(outcome.Observations)
	clone.Image.InspectedRepoDigests[0] = "two"
	clone.Reachability.Health.Mode = "mutated"
	if outcome.Observations.Image.InspectedRepoDigests[0] != "one" {
		t.Fatal("repo digest slice aliases clone")
	}
	if outcome.Observations.Reachability.Health.Mode == "mutated" {
		t.Fatal("nested state mutation leaked")
	}
}

func TestLifecycleFailure_WorkloadAndTerminalCausesPreserved(t *testing.T) {
	assertLifecycleCauses(t, true, false, false)
}
func TestLifecycleFailure_WorkloadAndCleanupCausesPreserved(t *testing.T) {
	assertLifecycleCauses(t, true, true, false)
}
func TestLifecycleFailure_TerminalAndCleanupCausesPreserved(t *testing.T) {
	assertLifecycleCauses(t, false, true, true)
}
func TestLifecycleFailure_AllCausesPreserved(t *testing.T) {
	assertLifecycleCauses(t, true, true, true)
}

func assertLifecycleCauses(t *testing.T, workloadFailure, cleanupFailure, terminalFailure bool) {
	t.Helper()
	fake := newRecordingDockerRuntime()
	seedValidLifecycle(t, fake)
	if cleanupFailure {
		fake.containerRemoveErr = errCleanupContainer
	}
	audited := NewAuditedDockerRuntime(fake)
	run := successfulQualifiedWorkload
	if workloadFailure {
		run = func(context.Context, QualifiedWorkloadInput) (*QualifiedWorkloadResult, error) { return nil, errRun }
	}
	opts := LifecycleOptions{ImageReference: "kgb-tovarisch-canary:latest", NetworkName: "kgb-lab-net", Run: run, TerminalTimeout: time.Millisecond, CleanupTimeout: time.Second}
	_, err := executeQualifiedLifecycle(context.Background(), audited, func(context.Context, string) bool { return !terminalFailure }, opts)
	if workloadFailure && !errors.Is(err, errRun) {
		t.Fatalf("workload cause missing: %v", err)
	}
	if cleanupFailure && !errors.Is(err, errCleanupContainer) {
		t.Fatalf("cleanup cause missing: %v", err)
	}
	if terminalFailure && !errors.Is(err, errTerminalTimeout) {
		t.Fatalf("terminal cause missing: %v", err)
	}
}
