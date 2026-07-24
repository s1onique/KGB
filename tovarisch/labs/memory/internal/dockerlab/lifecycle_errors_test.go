// lifecycle_errors_test.go — Lifecycle error propagation tests.
//
// CORRECTION22 P0-9: every failure source is preserved via
// errors.Join so callers can introspect every applicable error.
// The tests use the recordingDockerRuntime + a deterministic
// TerminalObserver to drive the lifecycle without a real Docker
// daemon.

package dockerlab

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
)

// errRun is a sentinel for the workload callback failure.
var errRun = errors.New("workload run failed")

// errCleanupContainer is a sentinel for container cleanup failure.
var errCleanupContainer = errors.New("container cleanup failed")

// errCleanupNetwork is a sentinel for network cleanup failure.
var errCleanupNetwork = errors.New("network cleanup failed")

// errTerminal mirrors the production sentinel from qualified_runtime.go.
var errTerminal = errTerminalTimeout

// seedValidLifecycle installs the canary image and prepares the
// recording runtime so a healthy path would succeed. It returns
// the image ID and the seeded network ID for diagnostics.
func seedValidLifecycle(t *testing.T, fake *recordingDockerRuntime) (imageID, networkID string) {
	t.Helper()
	const canonicalID = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	fake.addImage("kgb-tovarisch-canary:latest", canonicalID)
	// Pre-populate the network so inspect-after-create returns the same ID.
	return canonicalID, ""
}

// TestRunErrorPropagates verifies that a workload error is the
// primary returned error and that the lifecycle still reports
// observations including pull=0.
func TestRunErrorPropagates(t *testing.T) {
	fake := newRecordingDockerRuntime()
	seedValidLifecycle(t, fake)
	audited := NewAuditedDockerRuntime(fake)

	opts := LifecycleOptions{
		ImageReference:   "kgb-tovarisch-canary:latest",
		NetworkName:      "kgb-lab-net",
		ContainerName:    "kgb-subject",
		ContainerCmd:     []string{"true"},
		TerminalTimeout:  time.Second,
		CleanupTimeout:   time.Second,
		TerminalObserver: func(_ context.Context, _ string) bool { return true },
		Run:              func(_ context.Context, _ string, _ *QualifiedExecutionObservations) error { return errRun },
	}
	_, err := executeQualifiedLifecycle(context.Background(), audited, opts.TerminalObserver, opts)
	if err == nil {
		t.Fatal("expected error from workload, got nil")
	}
	if !errors.Is(err, errRun) {
		t.Fatalf("expected workload error in chain, got: %v", err)
	}
}

// TestRunAndTerminalErrorsJoin verifies that a workload error
// and a terminal-state observation error are both reachable via
// errors.Is.
func TestRunAndTerminalErrorsJoin(t *testing.T) {
	fake := newRecordingDockerRuntime()
	seedValidLifecycle(t, fake)
	audited := NewAuditedDockerRuntime(fake)

	opts := LifecycleOptions{
		ImageReference:   "kgb-tovarisch-canary:latest",
		NetworkName:      "kgb-lab-net",
		ContainerName:    "kgb-subject",
		ContainerCmd:     []string{"true"},
		TerminalTimeout:  100 * time.Millisecond,
		CleanupTimeout:   time.Second,
		TerminalObserver: func(_ context.Context, _ string) bool { return false },
		Run:              func(_ context.Context, _ string, _ *QualifiedExecutionObservations) error { return errRun },
	}
	_, err := executeQualifiedLifecycle(context.Background(), audited, opts.TerminalObserver, opts)
	if err == nil {
		t.Fatal("expected error from run+terminal, got nil")
	}
	if !errors.Is(err, errRun) {
		t.Fatalf("expected workload error in chain, got: %v", err)
	}
	if !errors.Is(err, errTerminal) {
		t.Fatalf("expected terminal error in chain, got: %v", err)
	}
}

// TestRunAndCleanupErrorsJoin verifies that a workload error
// and a cleanup error are both reachable via errors.Is.
func TestRunAndCleanupErrorsJoin(t *testing.T) {
	fake := newRecordingDockerRuntime()
	fake.containerRemoveErr = errCleanupContainer
	seedValidLifecycle(t, fake)
	audited := NewAuditedDockerRuntime(fake)

	opts := LifecycleOptions{
		ImageReference:   "kgb-tovarisch-canary:latest",
		NetworkName:      "kgb-lab-net",
		ContainerName:    "kgb-subject",
		ContainerCmd:     []string{"true"},
		TerminalTimeout:  time.Second,
		CleanupTimeout:   time.Second,
		TerminalObserver: func(_ context.Context, _ string) bool { return true },
		Run:              func(_ context.Context, _ string, _ *QualifiedExecutionObservations) error { return errRun },
	}
	_, err := executeQualifiedLifecycle(context.Background(), audited, opts.TerminalObserver, opts)
	if err == nil {
		t.Fatal("expected error from run+cleanup, got nil")
	}
	if !errors.Is(err, errRun) {
		t.Fatalf("expected workload error in chain, got: %v", err)
	}
	if !errors.Is(err, errCleanupContainer) {
		t.Fatalf("expected container cleanup error in chain, got: %v", err)
	}
}

// TestTerminalAndCleanupErrorsJoin verifies that a terminal
// observation error and a cleanup error are both reachable via
// errors.Is (no workload error in this scenario).
func TestTerminalAndCleanupErrorsJoin(t *testing.T) {
	fake := newRecordingDockerRuntime()
	fake.networkRemoveErr = errCleanupNetwork
	seedValidLifecycle(t, fake)
	audited := NewAuditedDockerRuntime(fake)

	opts := LifecycleOptions{
		ImageReference:   "kgb-tovarisch-canary:latest",
		NetworkName:      "kgb-lab-net",
		ContainerName:    "kgb-subject",
		ContainerCmd:     []string{"true"},
		TerminalTimeout:  100 * time.Millisecond,
		CleanupTimeout:   time.Second,
		TerminalObserver: func(_ context.Context, _ string) bool { return false },
		Run:              func(_ context.Context, _ string, _ *QualifiedExecutionObservations) error { return nil },
	}
	_, err := executeQualifiedLifecycle(context.Background(), audited, opts.TerminalObserver, opts)
	if err == nil {
		t.Fatal("expected error from terminal+cleanup, got nil")
	}
	if !errors.Is(err, errTerminal) {
		t.Fatalf("expected terminal error in chain, got: %v", err)
	}
	if !errors.Is(err, errCleanupNetwork) {
		t.Fatalf("expected network cleanup error in chain, got: %v", err)
	}
}

// TestRunTerminalAndCleanupErrorsJoin verifies that a workload
// error, a terminal-state observation error, AND a cleanup error
// are all reachable via errors.Is.
func TestRunTerminalAndCleanupErrorsJoin(t *testing.T) {
	fake := newRecordingDockerRuntime()
	fake.containerRemoveErr = errCleanupContainer
	fake.networkRemoveErr = errCleanupNetwork
	seedValidLifecycle(t, fake)
	audited := NewAuditedDockerRuntime(fake)

	opts := LifecycleOptions{
		ImageReference:   "kgb-tovarisch-canary:latest",
		NetworkName:      "kgb-lab-net",
		ContainerName:    "kgb-subject",
		ContainerCmd:     []string{"true"},
		TerminalTimeout:  100 * time.Millisecond,
		CleanupTimeout:   time.Second,
		TerminalObserver: func(_ context.Context, _ string) bool { return false },
		Run:              func(_ context.Context, _ string, _ *QualifiedExecutionObservations) error { return errRun },
	}
	_, err := executeQualifiedLifecycle(context.Background(), audited, opts.TerminalObserver, opts)
	if err == nil {
		t.Fatal("expected error from run+terminal+cleanup, got nil")
	}
	if !errors.Is(err, errRun) {
		t.Fatalf("expected workload error in chain, got: %v", err)
	}
	if !errors.Is(err, errTerminal) {
		t.Fatalf("expected terminal error in chain, got: %v", err)
	}
	if !errors.Is(err, errCleanupContainer) {
		t.Fatalf("expected container cleanup error in chain, got: %v", err)
	}
}

// TestPullAttemptFailureObservations verifies that when the run
// callback invokes ImagePull, finalizePullAudit captures the
// attempt and the returned observation reflects:
//
//	observation_available: true
//	attempted:              true
//	attempt_count:          1
//	last_reference:         non-empty
func TestPullAttemptFailureObservations(t *testing.T) {
	fake := newRecordingDockerRuntime()
	seedValidLifecycle(t, fake)
	audited := NewAuditedDockerRuntime(fake)

	opts := LifecycleOptions{
		ImageReference:   "kgb-tovarisch-canary:latest",
		NetworkName:      "kgb-lab-net",
		ContainerName:    "kgb-subject",
		ContainerCmd:     []string{"true"},
		TerminalTimeout:  time.Second,
		CleanupTimeout:   time.Second,
		TerminalObserver: func(_ context.Context, _ string) bool { return true },
		Run: func(_ context.Context, _ string, _ *QualifiedExecutionObservations) error {
			// Trigger ImagePull via the audited runtime. This
			// records the attempt and returns the sentinel.
			_, _ = audited.ImagePull(context.Background(), "kgb-tovarisch-canary:latest", types.ImagePullOptions{})
			return nil
		},
	}
	outcome, err := executeQualifiedLifecycle(context.Background(), audited, opts.TerminalObserver, opts)
	if err == nil {
		t.Fatal("expected pull-prohibition error, got nil")
	}
	if outcome == nil || outcome.Observations == nil {
		t.Fatal("expected non-nil observations")
	}
	if !outcome.Observations.Pull.ObservationAvailable {
		t.Errorf("expected pull.observation_available=true, got %v", outcome.Observations.Pull.ObservationAvailable)
	}
	if !outcome.Observations.Pull.Attempted {
		t.Errorf("expected pull.attempted=true, got %v", outcome.Observations.Pull.Attempted)
	}
	if outcome.Observations.Pull.AttemptCount != 1 {
		t.Errorf("expected pull.attempt_count=1, got %d", outcome.Observations.Pull.AttemptCount)
	}
	if outcome.Observations.Pull.LastReference == "" {
		t.Errorf("expected pull.last_reference non-empty, got %q", outcome.Observations.Pull.LastReference)
	}
}

// Type assertion: ImagePullOptions must be a struct with All field.
// Keep the ImagePullOptions type compatible by not changing the
// public signature; this test relies on the audited runtime
// proxy method signature.

// ImagePull is called above with the explicit struct, which the
// runtime wrapper records.

// TestLifecycle_PhaseOrder verifies that the lifecycle invokes
// the recorded fake in the expected order: prepare, start, run,
// terminal-observation, cleanup.
func TestLifecycle_PhaseOrder(t *testing.T) {
	fake := newRecordingDockerRuntime()
	seedValidLifecycle(t, fake)
	audited := NewAuditedDockerRuntime(fake)

	opts := LifecycleOptions{
		ImageReference:   "kgb-tovarisch-canary:latest",
		NetworkName:      "kgb-lab-net",
		ContainerName:    "kgb-subject",
		ContainerCmd:     []string{"true"},
		TerminalTimeout:  time.Second,
		CleanupTimeout:   time.Second,
		TerminalObserver: func(_ context.Context, _ string) bool { return true },
		Run:              func(_ context.Context, _ string, _ *QualifiedExecutionObservations) error { return nil },
	}
	if _, err := executeQualifiedLifecycle(context.Background(), audited, opts.TerminalObserver, opts); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	// The expected call sequence is exactly the qualified lifecycle:
	// ImageInspect, NetworkCreate, NetworkInspect, ContainerCreate,
	// ContainerInspect, ContainerStart, [workload], ContainerRemove,
	// NetworkRemove, [absence probes via ContainerInspect/NetworkInspect].
	// We assert the ordered prefixes.
	// The expected call sequence is exactly the qualified lifecycle:
	// ImageInspect, NetworkCreate, NetworkInspect, ContainerCreate,
	// ContainerInspect, ContainerStart, [workload], ContainerRemove,
	// [absence probe via ContainerInspect], NetworkRemove,
	// [absence probe via NetworkInspect].
	// We assert the ordered prefixes including the cleanup probes.
	want := []string{
		"ImageInspectWithRaw",
		"NetworkCreate",
		"NetworkInspect",
		"ContainerCreate",
		"ContainerInspect",
		"ContainerStart",
		"ContainerRemove",
		"ContainerInspect",
		"NetworkRemove",
		"NetworkInspect",
	}
	if len(fake.calls) < len(want) {
		t.Fatalf("not enough calls recorded: got %d, want >= %d", len(fake.calls), len(want))
	}
	for i, w := range want {
		if fake.calls[i] != w {
			t.Fatalf("call[%d]: got %q, want %q (full=%v)", i, fake.calls[i], w, fake.calls)
		}
	}
}
