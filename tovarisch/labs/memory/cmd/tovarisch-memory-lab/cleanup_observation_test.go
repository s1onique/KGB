// cleanup_observation_test.go — Cleanup Observation Tests
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03
//
// Tests for cleanup observation with injectable DockerRunner seam.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"
)

// mockDockerRunner records calls and returns configurable results.
type mockDockerRunner struct {
	calls     [][]string
	results   []DockerCommandResult
	errs      []error
	callIndex int
}

func newMockDockerRunner() *mockDockerRunner {
	return &mockDockerRunner{
		calls:     make([][]string, 0),
		results:   make([]DockerCommandResult, 0),
		errs:      make([]error, 0),
		callIndex: 0,
	}
}

func (m *mockDockerRunner) Run(ctx context.Context, limits DockerCommandLimits, args ...string) (DockerCommandResult, error) {
	m.calls = append(m.calls, args)
	if m.callIndex < len(m.results) {
		r := m.results[m.callIndex]
		e := error(nil)
		if m.callIndex < len(m.errs) {
			e = m.errs[m.callIndex]
		}
		m.callIndex++
		return r, e
	}
	return DockerCommandResult{ExitCode: 1, Stderr: []byte("mock: no result configured")}, nil
}

func (m *mockDockerRunner) AddResult(result DockerCommandResult, err error) {
	m.results = append(m.results, result)
	m.errs = append(m.errs, err)
}

// mockProcessReader returns configurable /proc data.
type mockProcessReader struct {
	data map[string][]byte
	errs map[string]error
}

func newMockProcessReader() *mockProcessReader {
	return &mockProcessReader{
		data: make(map[string][]byte),
		errs: make(map[string]error),
	}
}

func (m *mockProcessReader) ReadFile(path string) ([]byte, error) {
	if err, ok := m.errs[path]; ok {
		return nil, err
	}
	if data, ok := m.data[path]; ok {
		return data, nil
	}
	return nil, errors.New("mock: path not configured")
}

func (m *mockProcessReader) AddProcStat(pid int, startTime uint64) {
	statPath := "/proc/" + strconv.Itoa(pid) + "/stat"
	// Format: pid (comm) S ppid ... starttime
	m.data[statPath] = []byte("12345 (test) S 1 12345 12345 0 -1 4194304 1000 0 0 0 0 0 0 0 20 0 1 0 " +
		strconv.FormatUint(startTime, 10) + " 12345678 256 0 0 0 0 0 0 /root/test")
}

// =============================================================================
// CONTAINER OBSERVATION TESTS
// =============================================================================

func TestObserveContainerCleanup_ContainerGone(t *testing.T) {
	mock := newMockDockerRunner()
	mock.AddResult(DockerCommandResult{
		ExitCode: 1,
		Stderr:   []byte("Error: No such container: abc123def456\n"),
	}, nil)

	observer, err := NewCleanupObserverWithRunner(mock.Run, DefaultDockerCommandLimits())
	if err != nil {
		t.Fatal(err)
	}
	obs, err := observer.ObserveContainerCleanup(context.Background(), "abc123def456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.Status != ObjectGone {
		t.Errorf("expected ObjectGone, got %v", obs.Status)
	}
	if obs.ID != "abc123def456" {
		t.Errorf("expected ID abc123def456, got %s", obs.ID)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
}

func TestObserveContainerCleanup_ContainerExists(t *testing.T) {
	mock := newMockDockerRunner()
	mock.AddResult(DockerCommandResult{
		ExitCode: 0,
		Stdout:   []byte("abc123def456\n"),
	}, nil)

	observer, err := NewCleanupObserverWithRunner(mock.Run, DefaultDockerCommandLimits())
	if err != nil {
		t.Fatal(err)
	}
	obs, err := observer.ObserveContainerCleanup(context.Background(), "abc123def456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.Status != ObjectExists {
		t.Errorf("expected ObjectExists, got %v", obs.Status)
	}
}

func TestObserveContainerCleanup_ContainerUnavailableOnError(t *testing.T) {
	mock := newMockDockerRunner()
	mock.AddResult(DockerCommandResult{
		ExitCode: 1,
		Stderr:   []byte("Error: Something went wrong\n"),
	}, nil)

	observer, err := NewCleanupObserverWithRunner(mock.Run, DefaultDockerCommandLimits())
	if err != nil {
		t.Fatal(err)
	}
	obs, err := observer.ObserveContainerCleanup(context.Background(), "abc123def456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.Status != ObjectUnavailable {
		t.Errorf("expected ObjectUnavailable, got %v", obs.Status)
	}
}

func TestObserveContainerCleanup_ContainerUnavailableOnTimeout(t *testing.T) {
	mock := newMockDockerRunner()
	mock.AddResult(DockerCommandResult{
		ExitCode: 1,
		TimedOut: true,
	}, nil)

	observer, err := NewCleanupObserverWithRunner(mock.Run, DefaultDockerCommandLimits())
	if err != nil {
		t.Fatal(err)
	}
	obs, err := observer.ObserveContainerCleanup(context.Background(), "abc123def456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.Status != ObjectUnavailable {
		t.Errorf("expected ObjectUnavailable, got %v", obs.Status)
	}
}

func TestObserveContainerCleanup_EmptyIDReturnsError(t *testing.T) {
	mock := newMockDockerRunner()
	observer, err := NewCleanupObserverWithRunner(mock.Run, DefaultDockerCommandLimits())
	if err != nil {
		t.Fatal(err)
	}
	_, err = observer.ObserveContainerCleanup(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty container ID")
	}
}

func TestObserveContainerCleanup_DifferentIDReturned(t *testing.T) {
	mock := newMockDockerRunner()
	mock.AddResult(DockerCommandResult{
		ExitCode: 0,
		Stdout:   []byte("differentid\n"),
	}, nil)

	observer, err := NewCleanupObserverWithRunner(mock.Run, DefaultDockerCommandLimits())
	if err != nil {
		t.Fatal(err)
	}
	obs, err := observer.ObserveContainerCleanup(context.Background(), "abc123def456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.Status != ObjectUnavailable {
		t.Errorf("expected ObjectUnavailable, got %v", obs.Status)
	}
}

// =============================================================================
// NETWORK OBSERVATION TESTS
// =============================================================================

func TestObserveNetworkCleanup_NetworkGone(t *testing.T) {
	mock := newMockDockerRunner()
	mock.AddResult(DockerCommandResult{
		ExitCode: 1,
		Stderr:   []byte("Error: No such network: net123\n"),
	}, nil)

	observer, err := NewCleanupObserverWithRunner(mock.Run, DefaultDockerCommandLimits())
	if err != nil {
		t.Fatal(err)
	}
	obs, err := observer.ObserveNetworkCleanup(context.Background(), "net123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.Status != ObjectGone {
		t.Errorf("expected ObjectGone, got %v", obs.Status)
	}
}

func TestObserveNetworkCleanup_NetworkExists(t *testing.T) {
	mock := newMockDockerRunner()
	mock.AddResult(DockerCommandResult{
		ExitCode: 0,
		Stdout:   []byte("net123\n"),
	}, nil)

	observer, err := NewCleanupObserverWithRunner(mock.Run, DefaultDockerCommandLimits())
	if err != nil {
		t.Fatal(err)
	}
	obs, err := observer.ObserveNetworkCleanup(context.Background(), "net123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.Status != ObjectExists {
		t.Errorf("expected ObjectExists, got %v", obs.Status)
	}
}

func TestObserveNetworkCleanup_EmptyIDReturnsError(t *testing.T) {
	mock := newMockDockerRunner()
	observer, err := NewCleanupObserverWithRunner(mock.Run, DefaultDockerCommandLimits())
	if err != nil {
		t.Fatal(err)
	}
	_, err = observer.ObserveNetworkCleanup(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty network ID")
	}
}

// =============================================================================
// PROCESS OBSERVATION TESTS
// =============================================================================

func TestObserveProcessCleanup_ProcessGone(t *testing.T) {
	mock := newMockDockerRunner()
	reader := newMockProcessReader()
	// /proc/<pid>/stat does not exist
	reader.errs["/proc/12345/stat"] = os.ErrNotExist

	observer, err := NewCleanupObserverWithRunner(mock.Run, DefaultDockerCommandLimits())
	if err != nil {
		t.Fatal(err)
	}
	obs, err := observer.ObserveProcessCleanup(context.Background(), 12345, 99999, reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.Status != ProcessGoneCode {
		t.Errorf("expected ProcessGoneCode, got %v", obs.Status)
	}
	// P0-4 FIX: Start time preserved
	if obs.StartTime != 99999 {
		t.Errorf("expected start time 99999, got %d", obs.StartTime)
	}
}

func TestObserveProcessCleanup_ProcessStillAlive(t *testing.T) {
	mock := newMockDockerRunner()
	reader := newMockProcessReader()
	reader.AddProcStat(12345, 99999)

	observer, err := NewCleanupObserverWithRunner(mock.Run, DefaultDockerCommandLimits())
	if err != nil {
		t.Fatal(err)
	}
	obs, err := observer.ObserveProcessCleanup(context.Background(), 12345, 99999, reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.Status != ProcessStillAliveCode {
		t.Errorf("expected ProcessStillAliveCode, got %v", obs.Status)
	}
}

func TestObserveProcessCleanup_PIDReused(t *testing.T) {
	mock := newMockDockerRunner()
	reader := newMockProcessReader()
	// Different start time = PID reused
	reader.AddProcStat(12345, 88888)

	observer, err := NewCleanupObserverWithRunner(mock.Run, DefaultDockerCommandLimits())
	if err != nil {
		t.Fatal(err)
	}
	obs, err := observer.ObserveProcessCleanup(context.Background(), 12345, 99999, reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.Status != ProcessPIDReusedCode {
		t.Errorf("expected ProcessPIDReusedCode, got %v", obs.Status)
	}
	// P0-4 FIX: Start time preserved (expected, not observed)
	if obs.StartTime != 99999 {
		t.Errorf("expected preserved start time 99999, got %d", obs.StartTime)
	}
}

func TestObserveProcessCleanup_InvalidPID(t *testing.T) {
	mock := newMockDockerRunner()
	observer, err := NewCleanupObserverWithRunner(mock.Run, DefaultDockerCommandLimits())
	if err != nil {
		t.Fatal(err)
	}
	obs, err := observer.ObserveProcessCleanup(context.Background(), 0, 99999, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.Status != ProcessUnavailableCode {
		t.Errorf("expected ProcessUnavailableCode, got %v", obs.Status)
	}
}

// =============================================================================
// OBSERVER CONSTRUCTION TESTS
// =============================================================================

func TestNewCleanupObserverWithRunner_RejectsNil(t *testing.T) {
	_, err := NewCleanupObserverWithRunner(nil, DefaultDockerCommandLimits())
	if err == nil {
		t.Error("expected error for nil runner")
	}
	if err.Error() != "docker runner is nil" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestNewCleanupObserverWithRunner_AcceptsValidRunner(t *testing.T) {
	mock := newMockDockerRunner()
	observer, err := NewCleanupObserverWithRunner(mock.Run, DefaultDockerCommandLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if observer == nil {
		t.Error("expected non-nil observer")
	}
}

// =============================================================================
// BOUNDED WRITER TESTS
// =============================================================================


// =============================================================================
// NETWORK IDENTITY TESTS
// =============================================================================

func TestWriteNetworkIdentity_RoundTrip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "netid-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	err = WriteNetworkIdentity(tmpDir, "net123", "test-network")
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	id, name, err := ReadNetworkIdentity(tmpDir)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if id != "net123" {
		t.Errorf("expected net123, got %s", id)
	}
	if name != "test-network" {
		t.Errorf("expected test-network, got %s", name)
	}
}

func TestReadNetworkIdentity_RejectsUnknownFields(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "netid-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write JSON with unknown field
	path := tmpDir + "/network-identity.json"
	err = os.WriteFile(path, []byte(`{"schema_version":"1.0.0","id":"net123","name":"test","unknown":true}`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = ReadNetworkIdentity(tmpDir)
	if err == nil {
		t.Error("expected error for unknown field")
	}
}

func TestReadNetworkIdentity_RejectsTrailingData(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "netid-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	path := tmpDir + "/network-identity.json"
	err = os.WriteFile(path, []byte(`{"schema_version":"1.0.0","id":"net123","name":"test"}{"id":"extra"}`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = ReadNetworkIdentity(tmpDir)
	if err == nil {
		t.Error("expected error for trailing JSON")
	}
}

func TestReadNetworkIdentity_RejectsWrongSchema(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "netid-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	path := tmpDir + "/network-identity.json"
	err = os.WriteFile(path, []byte(`{"schema_version":"2.0.0","id":"net123","name":"test"}`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = ReadNetworkIdentity(tmpDir)
	if err == nil {
		t.Error("expected error for wrong schema version")
	}
}

// =============================================================================
// INTEGRATION TESTS
// =============================================================================

func TestObserveDeclaredRunCleanup_RejectsNilRuns(t *testing.T) {
	observer := NewCleanupObserver()
	_, err := ObserveDeclaredRunCleanup(context.Background(), nil, observer)
	if err == nil {
		t.Error("expected error for nil runs")
	}
}

func TestObserveDeclaredRunCleanup_RejectsNilObserver(t *testing.T) {
	_, err := ObserveDeclaredRunCleanup(context.Background(), []*VerifiedRun{}, nil)
	if err == nil {
		t.Error("expected error for nil observer")
	}
}

func TestBuildObservedMatrixCleanupEvidence_RejectsEmptyMatrixID(t *testing.T) {
	obs := []RunCleanupObservation{
		{RunID: "run1", Scenario: "canary-growing", Container: ContainerIdentityObservation{ID: "c1", Status: ObjectGone}, Network: NetworkIdentityObservation{ID: "n1", Status: ObjectGone}, Process: ProcessIdentityObservation{PID: 1, StartTime: 100, Status: ProcessGoneCode}},
		{RunID: "run2", Scenario: "canary-bounded", Container: ContainerIdentityObservation{ID: "c2", Status: ObjectGone}, Network: NetworkIdentityObservation{ID: "n2", Status: ObjectGone}, Process: ProcessIdentityObservation{PID: 2, StartTime: 200, Status: ProcessGoneCode}},
		{RunID: "run3", Scenario: "canary-descriptor", Container: ContainerIdentityObservation{ID: "c3", Status: ObjectGone}, Network: NetworkIdentityObservation{ID: "n3", Status: ObjectGone}, Process: ProcessIdentityObservation{PID: 3, StartTime: 300, Status: ProcessGoneCode}},
	}
	_, err := BuildObservedMatrixCleanupEvidence("", "per_run", obs, time.Now())
	if err == nil {
		t.Error("expected error for empty matrix ID")
	}
}

func TestBuildObservedMatrixCleanupEvidence_RejectsMatrixSharedEmptyNetwork(t *testing.T) {
	obs := []RunCleanupObservation{
		{RunID: "run1", Scenario: "canary-growing", Container: ContainerIdentityObservation{ID: "c1", Status: ObjectGone}, Network: NetworkIdentityObservation{ID: "", Status: ObjectGone}, Process: ProcessIdentityObservation{PID: 1, StartTime: 100, Status: ProcessGoneCode}},
		{RunID: "run2", Scenario: "canary-bounded", Container: ContainerIdentityObservation{ID: "c2", Status: ObjectGone}, Network: NetworkIdentityObservation{ID: "", Status: ObjectGone}, Process: ProcessIdentityObservation{PID: 2, StartTime: 200, Status: ProcessGoneCode}},
		{RunID: "run3", Scenario: "canary-descriptor", Container: ContainerIdentityObservation{ID: "c3", Status: ObjectGone}, Network: NetworkIdentityObservation{ID: "", Status: ObjectGone}, Process: ProcessIdentityObservation{PID: 3, StartTime: 300, Status: ProcessGoneCode}},
	}
	_, err := BuildObservedMatrixCleanupEvidence("matrix1", "matrix_shared", obs, time.Now())
	if err == nil {
		t.Error("expected error for matrix_shared with empty network")
	}
}

func TestBuildObservedMatrixCleanupEvidence_ValidPerRun(t *testing.T) {
	obs := []RunCleanupObservation{
		{RunID: "run1", Scenario: "canary-growing", Container: ContainerIdentityObservation{ID: "c1", Status: ObjectGone}, Network: NetworkIdentityObservation{ID: "n1", Status: ObjectGone}, Process: ProcessIdentityObservation{PID: 1, StartTime: 100, Status: ProcessGoneCode}},
		{RunID: "run2", Scenario: "canary-bounded", Container: ContainerIdentityObservation{ID: "c2", Status: ObjectGone}, Network: NetworkIdentityObservation{ID: "n2", Status: ObjectGone}, Process: ProcessIdentityObservation{PID: 2, StartTime: 200, Status: ProcessGoneCode}},
		{RunID: "run3", Scenario: "canary-descriptor", Container: ContainerIdentityObservation{ID: "c3", Status: ObjectGone}, Network: NetworkIdentityObservation{ID: "n3", Status: ObjectGone}, Process: ProcessIdentityObservation{PID: 3, StartTime: 300, Status: ProcessGoneCode}},
	}
	evidence, err := BuildObservedMatrixCleanupEvidence("matrix1", "per_run", obs, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evidence.MatrixID != "matrix1" {
		t.Errorf("expected matrix1, got %s", evidence.MatrixID)
	}
	if len(evidence.Runs) != 3 {
		t.Errorf("expected 3 runs, got %d", len(evidence.Runs))
	}
}

func TestBuildObservedMatrixCleanupEvidence_ValidMatrixShared(t *testing.T) {
	obs := []RunCleanupObservation{
		{RunID: "run1", Scenario: "canary-growing", Container: ContainerIdentityObservation{ID: "c1", Status: ObjectGone}, Network: NetworkIdentityObservation{ID: "sharednet", Status: ObjectGone}, Process: ProcessIdentityObservation{PID: 1, StartTime: 100, Status: ProcessGoneCode}},
		{RunID: "run2", Scenario: "canary-bounded", Container: ContainerIdentityObservation{ID: "c2", Status: ObjectGone}, Network: NetworkIdentityObservation{ID: "sharednet", Status: ObjectGone}, Process: ProcessIdentityObservation{PID: 2, StartTime: 200, Status: ProcessGoneCode}},
		{RunID: "run3", Scenario: "canary-descriptor", Container: ContainerIdentityObservation{ID: "c3", Status: ObjectGone}, Network: NetworkIdentityObservation{ID: "sharednet", Status: ObjectGone}, Process: ProcessIdentityObservation{PID: 3, StartTime: 300, Status: ProcessGoneCode}},
	}
	evidence, err := BuildObservedMatrixCleanupEvidence("matrix1", "matrix_shared", obs, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evidence.NetworkOwnership != "matrix_shared" {
		t.Errorf("expected matrix_shared, got %s", evidence.NetworkOwnership)
	}
}

// TestProduceObservedCleanupEvidence_SeamExercise proves that the extracted
// produceObservedCleanupEvidence function calls the injected observer with
// the correct VerifiedRun inputs and writes the returned observations unchanged.
// P0 FIX: Extended assertions for DeclaredScenario, NetworkID, and all artifact fields.
func TestProduceObservedCleanupEvidence_SeamExercise(t *testing.T) {
	// Create a temporary matrix directory with child run artifacts
	tmpDir, err := os.MkdirTemp("", "matrix-producer-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create run directories with minimal artifacts
	runsDir := tmpDir + "/runs"
	os.MkdirAll(runsDir, 0755)

	// Canonical scenario order
	scenarios := []string{"canary-growing", "canary-bounded", "canary-descriptor"}
	runIDs := []string{"canary-growing-1", "canary-bounded-2", "canary-descriptor-3"}
	containerIDs := []string{"abc123def456", "789xyz123abc", "456uvw789xyz"}
	networkIDs := []string{"net-grow-001", "net-bound-002", "net-desc-003"}
	pids := []int{1234, 5678, 9012}
	startTimes := []uint64{100000, 200000, 300000}
	observedAt := time.Now()

	// Create VerifiedRun values with ALL identity fields
	runs := []*VerifiedRun{
		{
			DeclaredRunID:    runIDs[0],
			DeclaredScenario: scenarios[0],
			ContainerID:      containerIDs[0],
			NetworkID:        networkIDs[0],
			SubjectPID:       pids[0],
			SubjectStartTime: startTimes[0],
		},
		{
			DeclaredRunID:    runIDs[1],
			DeclaredScenario: scenarios[1],
			ContainerID:      containerIDs[1],
			NetworkID:        networkIDs[1],
			SubjectPID:       pids[1],
			SubjectStartTime: startTimes[1],
		},
		{
			DeclaredRunID:    runIDs[2],
			DeclaredScenario: scenarios[2],
			ContainerID:      containerIDs[2],
			NetworkID:        networkIDs[2],
			SubjectPID:       pids[2],
			SubjectStartTime: startTimes[2],
		},
	}

	// Track observer calls
	var observeCalls int
	var observedRuns []*VerifiedRun

	// Injected observer that records calls and asserts ALL inputs
	injectedObservations := []RunCleanupObservation{
		{RunID: runIDs[0], Scenario: scenarios[0], Container: ContainerIdentityObservation{ID: containerIDs[0], Status: ObjectGone}, Network: NetworkIdentityObservation{ID: networkIDs[0], Status: ObjectGone}, Process: ProcessIdentityObservation{PID: pids[0], StartTime: startTimes[0], Status: ProcessGoneCode}},
		{RunID: runIDs[1], Scenario: scenarios[1], Container: ContainerIdentityObservation{ID: containerIDs[1], Status: ObjectGone}, Network: NetworkIdentityObservation{ID: networkIDs[1], Status: ObjectGone}, Process: ProcessIdentityObservation{PID: pids[1], StartTime: startTimes[1], Status: ProcessGoneCode}},
		{RunID: runIDs[2], Scenario: scenarios[2], Container: ContainerIdentityObservation{ID: containerIDs[2], Status: ObjectGone}, Network: NetworkIdentityObservation{ID: networkIDs[2], Status: ObjectGone}, Process: ProcessIdentityObservation{PID: pids[2], StartTime: startTimes[2], Status: ProcessGoneCode}},
	}

	mockObserve := func(ctx context.Context, observed []*VerifiedRun) ([]RunCleanupObservation, error) {
		observeCalls++
		observedRuns = observed

		// P0 FIX: Assert ALL VerifiedRun fields reach the observer
		if len(observed) != 3 {
			t.Errorf("observer received %d runs, want 3", len(observed))
		}
		for i, run := range observed {
			if run.DeclaredRunID != runIDs[i] {
				t.Errorf("run[%d].DeclaredRunID = %s, want %s", i, run.DeclaredRunID, runIDs[i])
			}
			if run.DeclaredScenario != scenarios[i] {
				t.Errorf("run[%d].DeclaredScenario = %s, want %s", i, run.DeclaredScenario, scenarios[i])
			}
			if run.ContainerID != containerIDs[i] {
				t.Errorf("run[%d].ContainerID = %s, want %s", i, run.ContainerID, containerIDs[i])
			}
			// P0 FIX: Assert NetworkID is observed
			if run.NetworkID != networkIDs[i] {
				t.Errorf("run[%d].NetworkID = %s, want %s", i, run.NetworkID, networkIDs[i])
			}
			if run.SubjectPID != pids[i] {
				t.Errorf("run[%d].SubjectPID = %d, want %d", i, run.SubjectPID, pids[i])
			}
			if run.SubjectStartTime != startTimes[i] {
				t.Errorf("run[%d].SubjectStartTime = %d, want %d", i, run.SubjectStartTime, startTimes[i])
			}
		}

		return injectedObservations, nil
	}

	matrixManifest := &MatrixManifest{
		SchemaVersion: MatrixSchemaVersion,
		MatrixID:      "matrix-test-123",
		StartedAt:     observedAt,
		FinishedAt:    observedAt.Add(180 * time.Second),
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: scenarios[0], RunID: runIDs[0], ChecksumsSHA256: "sha1"},
			{Index: 2, Scenario: scenarios[1], RunID: runIDs[1], ChecksumsSHA256: "sha2"},
			{Index: 3, Scenario: scenarios[2], RunID: runIDs[2], ChecksumsSHA256: "sha3"},
		},
	}
	matrixJSON, jsonErr := json.MarshalIndent(matrixManifest, "", "  ")
	if jsonErr != nil {
		t.Fatalf("marshal manifest: %v", jsonErr)
	}
	if writeErr := os.WriteFile(tmpDir+"/matrix-manifest.json", matrixJSON, 0644); writeErr != nil {
		t.Fatalf("write manifest: %v", writeErr)
	}

	deps := MatrixCommandDeps{
		ObserveCleanup: mockObserve,
		Now:            func() time.Time { return observedAt.Add(185 * time.Second) },
	}

	// Call the extracted production function
	evidence, fnErr := produceObservedCleanupEvidence(
		context.Background(),
		tmpDir,
		"matrix-test-123",
		matrixManifest,
		runs,
		deps,
	)
	if fnErr != nil {
		t.Fatalf("produceObservedCleanupEvidence failed: %v", fnErr)
	}

	// Assert observer was called exactly once
	if observeCalls != 1 {
		t.Errorf("ObserveCleanup called %d times, want 1", observeCalls)
	}

	// Assert the exact VerifiedRun values reached the observer
	if len(observedRuns) != 3 {
		t.Fatalf("observedRuns count = %d, want 3", len(observedRuns))
	}

	// P0 FIX: Assert complete artifact structure
	if evidence.SchemaVersion != "1.0.0" {
		t.Errorf("evidence.SchemaVersion = %s, want 1.0.0", evidence.SchemaVersion)
	}
	if evidence.MatrixID != "matrix-test-123" {
		t.Errorf("evidence.MatrixID = %s, want matrix-test-123", evidence.MatrixID)
	}
	if evidence.NetworkOwnership != "per_run" {
		t.Errorf("evidence.NetworkOwnership = %s, want per_run", evidence.NetworkOwnership)
	}

	// P0-5 FIX: Verify evidence returned directly (write removed in P0-5)
	// Previously this read from disk; now use the returned evidence directly
	if evidence.SchemaVersion != "1.0.0" {
		t.Errorf("evidence.SchemaVersion = %s, want 1.0.0", evidence.SchemaVersion)
	}
	if evidence.MatrixID != "matrix-test-123" {
		t.Errorf("evidence.MatrixID = %s, want matrix-test-123", evidence.MatrixID)
	}
	if evidence.NetworkOwnership != "per_run" {
		t.Errorf("evidence.NetworkOwnership = %s, want per_run", evidence.NetworkOwnership)
	}

	if len(evidence.Runs) != 3 {
		t.Fatalf("evidence.Runs count = %d, want 3", len(evidence.Runs))
	}

	for i, record := range evidence.Runs {
		if record.RunID != runIDs[i] {
			t.Errorf("evidence.Runs[%d].RunID = %s, want %s", i, record.RunID, runIDs[i])
		}
		if record.Scenario != scenarios[i] {
			t.Errorf("evidence.Runs[%d].Scenario = %s, want %s", i, record.Scenario, scenarios[i])
		}
		if record.Container.ID != containerIDs[i] {
			t.Errorf("evidence.Runs[%d].Container.ID = %s, want %s", i, record.Container.ID, containerIDs[i])
		}
		if record.Container.Status != "gone" {
			t.Errorf("evidence.Runs[%d].Container.Status = %s, want gone", i, record.Container.Status)
		}
		// P0 FIX: Assert network identity propagation
		if record.Network.ID != networkIDs[i] {
			t.Errorf("evidence.Runs[%d].Network.ID = %s, want %s", i, record.Network.ID, networkIDs[i])
		}
		if record.Network.Status != "gone" {
			t.Errorf("evidence.Runs[%d].Network.Status = %s, want gone", i, record.Network.Status)
		}
		if record.Process.PID != pids[i] {
			t.Errorf("evidence.Runs[%d].Process.PID = %d, want %d", i, record.Process.PID, pids[i])
		}
		if record.Process.StartTime != startTimes[i] {
			t.Errorf("evidence.Runs[%d].Process.StartTime = %d, want %d", i, record.Process.StartTime, startTimes[i])
		}
		if record.Process.Status != "gone" {
			t.Errorf("evidence.Runs[%d].Process.Status = %s, want gone", i, record.Process.Status)
		}
	}

	t.Logf("PASS: observed run IDs: %v", runIDs)
	t.Logf("PASS: observed container IDs: %v", containerIDs)
	t.Logf("PASS: observed network IDs: %v", networkIDs)
	t.Logf("PASS: observed PIDs: %v", pids)
	t.Logf("PASS: observed scenarios: %v", scenarios)
}

// TestMatrixCommandWithDeps_RejectsNilObserveCleanup proves that the public
// entry point fails closed when ObserveCleanup is nil.
func TestMatrixCommandWithDeps_RejectsNilObserveCleanup(t *testing.T) {
	err := matrixCommandWithDeps(
		[]string{"memory-lab", "matrix", "--artifacts-dir", "/nonexistent"},
		MatrixCommandDeps{
			ObserveCleanup: nil,
			Now:           time.Now,
		},
	)
	if err == nil {
		t.Error("expected error for nil ObserveCleanup")
	}
}

// TestMatrixCommandWithDeps_RejectsNilNow proves that the public
// entry point fails closed when Now is nil.
func TestMatrixCommandWithDeps_RejectsNilNow(t *testing.T) {
	err := matrixCommandWithDeps(
		[]string{"memory-lab", "matrix", "--artifacts-dir", "/nonexistent"},
		MatrixCommandDeps{
			ObserveCleanup: func(ctx context.Context, runs []*VerifiedRun) ([]RunCleanupObservation, error) {
				return nil, nil
			},
			Now: nil,
		},
	)
	if err == nil {
		t.Error("expected error for nil Now")
	}
}

// TestProduceObservedCleanupEvidence_RejectsNilDeps proves that nil dependencies
// are rejected rather than silently falling back to real implementations.
func TestProduceObservedCleanupEvidence_RejectsNilDeps(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "matrix-producer-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	now := time.Now()
	manifest := &MatrixManifest{
		SchemaVersion: MatrixSchemaVersion,
		MatrixID:      "test",
		StartedAt:     now,
		FinishedAt:    now,
		Runs:          []MatrixRunDeclaration{},
	}

	// Test nil ObserveCleanup
	_, err = produceObservedCleanupEvidence(
		context.Background(),
		tmpDir,
		"test",
		manifest,
		[]*VerifiedRun{},
		MatrixCommandDeps{ObserveCleanup: nil, Now: time.Now},
	)
	if err == nil {
		t.Error("expected error for nil ObserveCleanup")
	}

	// Test nil Now
	_, err = produceObservedCleanupEvidence(
		context.Background(),
		tmpDir,
		"test",
		manifest,
		[]*VerifiedRun{},
		MatrixCommandDeps{ObserveCleanup: func(ctx context.Context, runs []*VerifiedRun) ([]RunCleanupObservation, error) { return nil, nil }, Now: nil},
	)
	if err == nil {
		t.Error("expected error for nil Now")
	}
}

// =============================================================================
// P0-3: WAITDELAY EXPIRED TESTS
// =============================================================================

func TestObserveContainerCleanup_WaitDelayExpiredReturnsUnavailable(t *testing.T) {
	// P0-3 FIX: WaitDelayExpired is ambiguous execution result, not authoritative evidence
	mock := newMockDockerRunner()
	mock.AddResult(DockerCommandResult{
		ExitCode:         0,
		Stdout:           []byte("abc123def456\n"),
		WaitDelayExpired: true,
	}, nil)

	observer, err := NewCleanupObserverWithRunner(mock.Run, DefaultDockerCommandLimits())
	if err != nil {
		t.Fatal(err)
	}

	obs, err := observer.ObserveContainerCleanup(context.Background(), "abc123def456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.Status != ObjectUnavailable {
		t.Errorf("expected ObjectUnavailable on WaitDelayExpired, got %v", obs.Status)
	}
}

func TestObserveNetworkCleanup_WaitDelayExpiredReturnsUnavailable(t *testing.T) {
	// P0-3 FIX: WaitDelayExpired is ambiguous execution result, not authoritative evidence
	mock := newMockDockerRunner()
	mock.AddResult(DockerCommandResult{
		ExitCode:         0,
		Stdout:           []byte("net123\n"),
		WaitDelayExpired: true,
	}, nil)

	observer, err := NewCleanupObserverWithRunner(mock.Run, DefaultDockerCommandLimits())
	if err != nil {
		t.Fatal(err)
	}

	obs, err := observer.ObserveNetworkCleanup(context.Background(), "net123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.Status != ObjectUnavailable {
		t.Errorf("expected ObjectUnavailable on WaitDelayExpired, got %v", obs.Status)
	}
}

func TestObserveContainerCleanup_StdoutOverflowReturnsUnavailable(t *testing.T) {
	// P0-3 FIX: StdoutOverflow is ambiguous execution result
	mock := newMockDockerRunner()
	mock.AddResult(DockerCommandResult{
		ExitCode:       0,
		Stdout:         []byte("abc123def456\n"),
		StdoutOverflow: true,
	}, nil)

	observer, err := NewCleanupObserverWithRunner(mock.Run, DefaultDockerCommandLimits())
	if err != nil {
		t.Fatal(err)
	}

	obs, err := observer.ObserveContainerCleanup(context.Background(), "abc123def456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.Status != ObjectUnavailable {
		t.Errorf("expected ObjectUnavailable on StdoutOverflow, got %v", obs.Status)
	}
}

func TestObserveNetworkCleanup_StderrOverflowReturnsUnavailable(t *testing.T) {
	// P0-3 FIX: StderrOverflow is ambiguous execution result
	mock := newMockDockerRunner()
	mock.AddResult(DockerCommandResult{
		ExitCode:       0,
		Stdout:         []byte("net123\n"),
		StderrOverflow: true,
	}, nil)

	observer, err := NewCleanupObserverWithRunner(mock.Run, DefaultDockerCommandLimits())
	if err != nil {
		t.Fatal(err)
	}

	obs, err := observer.ObserveNetworkCleanup(context.Background(), "net123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.Status != ObjectUnavailable {
		t.Errorf("expected ObjectUnavailable on StderrOverflow, got %v", obs.Status)
	}
}
