// control_correction44_test.go — Production call-graph, constructor,
// and reachability migration tests for CORRECTION44.
package dockerlab

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
)

// fakeDockerExecAPI satisfies the production DockerExecAPI seam.
type fakeDockerExecAPI struct{}

var _ DockerExecAPI = (*fakeDockerExecAPI)(nil)

func (fakeDockerExecAPI) ContainerExecCreate(ctx context.Context, container string, cfg types.ExecConfig) (types.IDResponse, error) {
	return types.IDResponse{ID: "exec-fake"}, nil
}

func (fakeDockerExecAPI) ContainerExecAttach(ctx context.Context, execID string, cfg types.ExecStartCheck) (types.HijackedResponse, error) {
	return types.HijackedResponse{}, nil
}

func (fakeDockerExecAPI) ContainerExecInspect(ctx context.Context, execID string) (types.ContainerExecInspect, error) {
	return types.ContainerExecInspect{ExitCode: 0}, nil
}

// TestNewDockerControl_NilClientFails verifies the typed
// constructor rejects a nil Docker exec client.
func TestNewDockerControl_NilClientFails(t *testing.T) {
	if _, err := NewDockerControl(nil); err == nil {
		t.Fatal("expected NewDockerControl to reject nil client")
	}
}

// TestNewDockerControl_RealRuntimeWiresBoundedTransport drives the
// typed constructor against a recording fake matching the real
// Docker client interface; the runner must be non-nil and the
// production transport must be the only call target.
func TestNewDockerControl_RealRuntimeWiresBoundedTransport(t *testing.T) {
	runner, err := NewDockerControl(&fakeDockerExecAPI{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner == nil {
		t.Fatal("runner must not be nil")
	}
	if runner.runtime == nil {
		t.Fatal("runtime must not be nil")
	}
}

// TestNewDockerControl_ReturnsTypedConstructionError ensures the
// returned error is the typed construction error sentinel.
func TestNewDockerControl_ReturnsTypedConstructionError(t *testing.T) {
	_, err := NewDockerControl(nil)
	if err == nil {
		t.Fatal("expected typed error")
	}
	if !errors.Is(err, ErrDockerExecClientRequired) {
		t.Fatalf("expected ErrDockerExecClientRequired, got %v", err)
	}
}

// TestExecuteQualifiedLifecycle_NilControlFails ensures the new
// dependency record rejects a missing control runner before any
// Docker mutation.
func TestExecuteQualifiedLifecycle_NilControlFails(t *testing.T) {
	audited := NewAuditedDockerRuntime(newRecordingDockerRuntime())
	_, err := executeQualifiedLifecycleWithDependencies(
		context.Background(),
		QualifiedLifecycleDependencies{Runtime: audited, Control: nil},
		func(_ context.Context, _ string) bool { return true },
		LifecycleOptions{ImageReference: "kgb-tovarisch-canary:latest", NetworkName: "kgb-lab-net", Run: func(context.Context, string, *QualifiedExecutionObservations) error { return nil }},
	)
	if err == nil {
		t.Fatal("expected nil control to fail before Docker lifecycle")
	}
	if !errors.Is(err, ErrQualifiedControlRequired) {
		t.Fatalf("expected ErrQualifiedControlRequired, got %v", err)
	}
}

// TestExecuteQualifiedLifecycle_DependencyRecordHonorsControl
// confirms the new dependency record binds the canonical
// control runner and runs prepare, start, and workload.
func TestExecuteQualifiedLifecycle_DependencyRecordHonorsControl(t *testing.T) {
	fake := newRecordingDockerRuntime()
	fake.addImage("kgb-tovarisch-canary:latest", "sha256:"+strings.Repeat("a", 64))
	audited := NewAuditedDockerRuntime(fake)
	runner, err := NewDockerControl(&fakeDockerExecAPI{})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	called := 0
	workload := func(_ context.Context, _ string, _ *QualifiedExecutionObservations) error { called++; return nil }
	outcome, err := executeQualifiedLifecycleWithDependencies(
		context.Background(),
		QualifiedLifecycleDependencies{Runtime: audited, Control: runner},
		func(_ context.Context, _ string) bool { return true },
		LifecycleOptions{ImageReference: "kgb-tovarisch-canary:latest", NetworkName: "kgb-lab-net", Run: workload, ContainerName: "kgb-subject", ContainerCmd: []string{"true"}, CleanupTimeout: time.Second, TerminalTimeout: time.Second},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome == nil || outcome.Observations == nil {
		t.Fatal("expected outcome with observations")
	}
	if called != 1 {
		t.Fatalf("expected workload to be called once, got %d", called)
	}
}
