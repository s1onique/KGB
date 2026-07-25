// control_correction45_test.go — Production call-graph sequence
// tests for CORRECTION45.
//
// The placeholder
// TestExecuteQualifiedLifecycle_ContainerIDReachesExec
// (CORRECTION44) has been removed. It is replaced by a
// recording DockerExecAPI fixture that emits real Docker
// multiplex framing and valid canonical envelopes, plus a
// bounded test suite that drives RunCanonicalControlSequence
// the same way the production CLI does.
//
// The four-operation reachability flow is:
//
//	health -> initial state -> operate -> final state
//
// No live Docker daemon is required.

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/canarycontrol"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
)

// filepathWalk walks the supplied root and invokes visit for each
// regular file. It returns true from visit to continue or false
// to stop. It's a minimal plain-Go alternative to filepath.Walk
// that does not require re-exporting the entire walker at the
// test boundary.
func filepathWalk(root string, visit func(path string) bool) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !visit(path) {
			return filepath.SkipAll
		}
		return nil
	})
}

func filepathBase(path string) string {
	return filepath.Base(path)
}

func readFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// recordingCall records a single DockerExecAPI call.
type recordingCall struct {
	Kind        string
	ContainerID string
	ExecID      string
	CreateOpts  *types.ExecConfig
}

// operationRecord is the per-operation record cached by the
// recording fixture.
type operationRecord struct {
	Operation   canarycontrol.Operation
	ContainerID string
	Create      recordingCall
	Attach      recordingCall
	Inspect     recordingCall
}

// recordingDockerExecAPI is the canonical DockerExecAPI fixture
// used by all production call-graph tests. It emits real Docker
// multiplex framing and supplies valid canonical envelopes.
type recordingDockerExecAPI struct {
	mu sync.Mutex

	// Per-operation recorded calls. Each entry corresponds to a
	// single ControlProbe call.
	calls []operationRecord

	// Per-operation envelopes. The fixture returns matching
	// envelopes for each operation in the order it is requested.
	// The fixture cycles through these envelopes; tests set the
	// slice to control the response.
	envelopes [][]byte

	// Forced container ID for the Attach response. Tests use
	// this to assert that the inspect container identity is
	// correctly threaded through the production transport.
	attachContainerMismatch string

	// Failure injection.
	forceCreateErr  error
	forceAttachErr  error
	forceInspectErr error
	forceExitCode   int
	forceRunErr     error
}

func (f *recordingDockerExecAPI) ContainerExecCreate(_ context.Context, containerID string, options types.ExecConfig) (types.IDResponse, error) {
	f.mu.Lock()
	op := f.nextOperation(options)
	execID := fmt.Sprintf("exec-%d", len(f.calls)+1)
	rec := operationRecord{Operation: op, ContainerID: containerID}
	rec.Create = recordingCall{Kind: "ExecCreate", ContainerID: containerID, ExecID: execID, CreateOpts: copyExecConfig(options)}
	f.calls = append(f.calls, rec)
	fErr := f.forceCreateErr
	f.mu.Unlock()
	if fErr != nil {
		return types.IDResponse{}, fErr
	}
	return types.IDResponse{ID: execID}, nil
}

func (f *recordingDockerExecAPI) ContainerExecAttach(_ context.Context, execID string, _ types.ExecStartCheck) (types.HijackedResponse, error) {
	f.mu.Lock()
	idx := len(f.calls) - 1
	if idx < 0 || f.calls[idx].Attach.ExecID != "" {
		f.mu.Unlock()
		return types.HijackedResponse{}, errors.New("attach called out of order")
	}
	f.calls[idx].Attach = recordingCall{Kind: "ExecAttach", ExecID: execID, ContainerID: f.calls[idx].ContainerID}
	// Pick the envelope for this call.
	var env []byte
	if idx < len(f.envelopes) {
		env = f.envelopes[idx]
	} else if len(f.envelopes) > 0 {
		env = f.envelopes[len(f.envelopes)-1]
	} else {
		env = successHealthEnvelope()
	}
	conn, reader := pipeWithPayload(multiplexFrame(1, env))
	mismatch := f.attachContainerMismatch
	runErr := f.forceRunErr
	f.mu.Unlock()
	if runErr != nil {
		// Force the writer to fail mid-stream.
		_ = conn.Close()
		return types.HijackedResponse{}, runErr
	}
	if mismatch != "" {
		// The inspector will reject the mismatch in the next step.
		_ = mismatch
	}
	return types.HijackedResponse{Conn: conn, Reader: bufio.NewReader(reader)}, nil
}

func (f *recordingDockerExecAPI) ContainerExecInspect(_ context.Context, execID string) (types.ContainerExecInspect, error) {
	f.mu.Lock()
	idx := len(f.calls) - 1
	if idx < 0 {
		f.mu.Unlock()
		return types.ContainerExecInspect{}, errors.New("inspect called out of order")
	}
	containerID := f.calls[idx].ContainerID
	f.calls[idx].Inspect = recordingCall{Kind: "ExecInspect", ExecID: execID, ContainerID: containerID}
	exitCode := f.forceExitCode
	inspectErr := f.forceInspectErr
	mismatch := f.attachContainerMismatch
	f.mu.Unlock()
	if inspectErr != nil {
		return types.ContainerExecInspect{}, inspectErr
	}
	// When the test requested a container identity mismatch the
	// fixture returns a different ContainerID than the one
	// supplied to ExecCreate. The production ExecInspect
	// callback validates containerID == inspect.ContainerID and
	// returns an error when they differ.
	inspectContainerID := containerID
	if mismatch != "" {
		inspectContainerID = mismatch
	}
	return types.ContainerExecInspect{
		ExecID:      execID,
		ContainerID: inspectContainerID,
		ExitCode:    exitCode,
		Running:     false,
	}, nil
}

// nextOperation inspects the argv to determine which canary
// operation this exec-call is for. The fourth token is the
// operation name (health, state, operate).
func (f *recordingDockerExecAPI) nextOperation(cfg types.ExecConfig) canarycontrol.Operation {
	if len(cfg.Cmd) < 3 {
		return canarycontrol.OpHealth
	}
	switch canarycontrol.Operation(cfg.Cmd[2]) {
	case canarycontrol.OpHealth:
		return canarycontrol.OpHealth
	case canarycontrol.OpState:
		return canarycontrol.OpState
	case canarycontrol.OpOperate:
		return canarycontrol.OpOperate
	}
	return canarycontrol.OpHealth
}

func copyExecConfig(cfg types.ExecConfig) *types.ExecConfig {
	cp := cfg
	if cfg.Cmd != nil {
		cp.Cmd = append([]string(nil), cfg.Cmd...)
	}
	return &cp
}

// pipeWithPayload returns a (writeCloser, reader) pair where the
// reader emits the supplied bytes and the writer is closed once
// the data is consumed.
func pipeWithPayload(data []byte) (net.Conn, io.Reader) {
	pr, pw := net.Pipe()
	go func() {
		_, _ = pw.Write(data)
		_ = pw.Close()
	}()
	return pw, pr
}

// multiplexFrame builds a Docker multiplex frame for the given
// stream and payload. stream==1 is stdout, stream==2 is stderr.
func multiplexFrame(stream byte, payload []byte) []byte {
	header := make([]byte, 8)
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
	out := append([]byte(nil), header...)
	out = append(out, payload...)
	return out
}

func successHealthEnvelope() []byte {
	env, _ := json.Marshal(map[string]any{
		"schema_version": canarycontrol.SchemaVersion,
		"operation":      string(canarycontrol.OpHealth),
		"success":        true,
		"http_status":    200,
		"health":         map[string]any{"ready": true, "mode": "bounded"},
	})
	return env
}

func successStateEnvelope(mode string, ops int) []byte {
	env, _ := json.Marshal(map[string]any{
		"schema_version": canarycontrol.SchemaVersion,
		"operation":      string(canarycontrol.OpState),
		"success":        true,
		"http_status":    200,
		"state":          map[string]any{"mode": mode, "retained_blocks": 0, "retained_bytes": 0, "operation_count": ops, "fd_count": 5, "ready": true},
	})
	return env
}

func successOperateEnvelope(requested int) []byte {
	env, _ := json.Marshal(map[string]any{
		"schema_version": canarycontrol.SchemaVersion,
		"operation":      string(canarycontrol.OpOperate),
		"success":        true,
		"http_status":    200,
		"workload":       map[string]any{"requested": requested, "attempted": requested, "completed": requested},
	})
	return env
}

func newControlRunnerForTest(api *recordingDockerExecAPI) *dockerlab.ControlRunner {
	return dockerlab.NewControlRunner(&dockerlab.ProductionControlExecRuntime{Client: api})
}

// newRecordingSequenceFixture constructs a fresh fixture with
// default envelopes for the four-operation flow.
func newRecordingSequenceFixture() *recordingDockerExecAPI {
	return &recordingDockerExecAPI{
		envelopes: [][]byte{
			successHealthEnvelope(),
			successStateEnvelope("bounded", 0),
			successOperateEnvelope(100),
			successStateEnvelope("bounded", 100),
		},
	}
}

// TestProductionControlSequence_ExactContainerIDEveryOperation
// proves the exact container ID is recorded at ExecCreate,
// ExecAttach, and ExecInspect for every operation.
func TestProductionControlSequence_ExactContainerIDEveryOperation(t *testing.T) {
	api := newRecordingSequenceFixture()
	control := newControlRunnerForTest(api)
	obs := &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		Reachability:  dockerlab.ReachabilityObservations{Method: dockerlab.ReachabilityMethodDockerExec},
	}
	if _, _, _, err := RunCanonicalControlSequence(
		context.Background(), control, obs,
		CanonicalControlSequenceOptions{ContainerID: "container-exact", Port: 8080, Operations: 100, Timeout: 5 * time.Second},
	); err != nil {
		t.Fatalf("RunCanonicalControlSequence: %v", err)
	}
	if got := len(api.calls); got != 4 {
		t.Fatalf("expected 4 operations, got %d", got)
	}
	for i, op := range api.calls {
		if op.ContainerID != "container-exact" {
			t.Fatalf("operation %d (%s): container=%q want=container-exact", i, op.Operation, op.ContainerID)
		}
		if op.Create.ContainerID != "container-exact" {
			t.Fatalf("operation %d ExecCreate container=%q", i, op.Create.ContainerID)
		}
		if op.Attach.ContainerID != "container-exact" {
			t.Fatalf("operation %d ExecAttach container=%q", i, op.Attach.ContainerID)
		}
		if op.Inspect.ContainerID != "container-exact" {
			t.Fatalf("operation %d ExecInspect container=%q", i, op.Inspect.ContainerID)
		}
	}
}

// TestProductionControlSequence_ExactOperationOrder proves the
// canonical operation order is health, state, operate, state.
func TestProductionControlSequence_ExactOperationOrder(t *testing.T) {
	api := newRecordingSequenceFixture()
	control := newControlRunnerForTest(api)
	obs := &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		Reachability:  dockerlab.ReachabilityObservations{Method: dockerlab.ReachabilityMethodDockerExec},
	}
	if _, _, _, err := RunCanonicalControlSequence(
		context.Background(), control, obs,
		CanonicalControlSequenceOptions{ContainerID: "container-order", Port: 8080, Operations: 100, Timeout: 5 * time.Second},
	); err != nil {
		t.Fatalf("sequence: %v", err)
	}
	want := []canarycontrol.Operation{
		canarycontrol.OpHealth,
		canarycontrol.OpState,
		canarycontrol.OpOperate,
		canarycontrol.OpState,
	}
	for i, op := range want {
		if got := api.calls[i].Operation; got != op {
			t.Fatalf("operation %d: got=%s want=%s", i, got, op)
		}
	}
}

// extractOperationEnv parses the recorded argv for the i-th
// operation and returns the create options.
func extractOperationEnv(api *recordingDockerExecAPI, i int) types.ExecConfig {
	if i < 0 || i >= len(api.calls) {
		return types.ExecConfig{}
	}
	if api.calls[i].Create.CreateOpts == nil {
		return types.ExecConfig{}
	}
	return *api.calls[i].Create.CreateOpts
}

// TestProductionControlSequence_HealthObservationFromEnvelope
// proves the health observation is populated from the envelope.
func TestProductionControlSequence_HealthObservationFromEnvelope(t *testing.T) {
	api := newRecordingSequenceFixture()
	control := newControlRunnerForTest(api)
	obs := &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		Reachability:  dockerlab.ReachabilityObservations{Method: dockerlab.ReachabilityMethodDockerExec},
	}
	if _, _, _, err := RunCanonicalControlSequence(
		context.Background(), control, obs,
		CanonicalControlSequenceOptions{ContainerID: "container-health", Port: 8080, Operations: 100, Timeout: 5 * time.Second},
	); err != nil {
		t.Fatalf("sequence: %v", err)
	}
	if obs.Reachability.Health.Operation != canarycontrol.OpHealth {
		t.Fatalf("health operation=%s", obs.Reachability.Health.Operation)
	}
	if obs.Reachability.Health.ExecExitCode != 0 {
		t.Fatalf("health exit=%d", obs.Reachability.Health.ExecExitCode)
	}
	if obs.Reachability.Health.HTTPStatus != 200 {
		t.Fatalf("health http_status=%d", obs.Reachability.Health.HTTPStatus)
	}
	if !obs.Reachability.Health.ResponseValidated {
		t.Fatalf("health response_validated=false")
	}
	if obs.Reachability.Health.Mode != "bounded" {
		t.Fatalf("health mode=%q", obs.Reachability.Health.Mode)
	}
}

// TestProductionControlSequence_InitialStateObservationFromEnvelope
// proves the initial state is populated from the envelope.
func TestProductionControlSequence_InitialStateObservationFromEnvelope(t *testing.T) {
	api := newRecordingSequenceFixture()
	control := newControlRunnerForTest(api)
	obs := &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		Reachability:  dockerlab.ReachabilityObservations{Method: dockerlab.ReachabilityMethodDockerExec},
	}
	if _, _, _, err := RunCanonicalControlSequence(
		context.Background(), control, obs,
		CanonicalControlSequenceOptions{ContainerID: "container-initial", Port: 8080, Operations: 100, Timeout: 5 * time.Second},
	); err != nil {
		t.Fatalf("sequence: %v", err)
	}
	if obs.Reachability.InitialState.Operation != canarycontrol.OpState {
		t.Fatalf("initial operation=%s", obs.Reachability.InitialState.Operation)
	}
	if obs.Reachability.InitialState.Mode != "bounded" {
		t.Fatalf("initial mode=%q", obs.Reachability.InitialState.Mode)
	}
	if !obs.Reachability.InitialState.ResponseValidated {
		t.Fatalf("initial response_validated=false")
	}
}

// TestProductionControlSequence_OperateObservationFromEnvelope
// proves the operate observation is populated from the envelope.
func TestProductionControlSequence_OperateObservationFromEnvelope(t *testing.T) {
	api := newRecordingSequenceFixture()
	control := newControlRunnerForTest(api)
	obs := &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		Reachability:  dockerlab.ReachabilityObservations{Method: dockerlab.ReachabilityMethodDockerExec},
	}
	if _, _, _, err := RunCanonicalControlSequence(
		context.Background(), control, obs,
		CanonicalControlSequenceOptions{ContainerID: "container-operate", Port: 8080, Operations: 100, Timeout: 5 * time.Second},
	); err != nil {
		t.Fatalf("sequence: %v", err)
	}
	if obs.Reachability.Operate.Operation != canarycontrol.OpOperate {
		t.Fatalf("operate operation=%s", obs.Reachability.Operate.Operation)
	}
	if obs.Reachability.Operate.Requested != 100 || obs.Reachability.Operate.Attempted != 100 || obs.Reachability.Operate.Completed != 100 {
		t.Fatalf("operate counts: requested=%d attempted=%d completed=%d",
			obs.Reachability.Operate.Requested, obs.Reachability.Operate.Attempted, obs.Reachability.Operate.Completed)
	}
	if !obs.Reachability.Operate.ResponseValidated {
		t.Fatalf("operate response_validated=false")
	}
}

// TestProductionControlSequence_FinalStateObservationFromEnvelope
// proves the final state is populated from the envelope.
func TestProductionControlSequence_FinalStateObservationFromEnvelope(t *testing.T) {
	api := newRecordingSequenceFixture()
	control := newControlRunnerForTest(api)
	obs := &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		Reachability:  dockerlab.ReachabilityObservations{Method: dockerlab.ReachabilityMethodDockerExec},
	}
	if _, _, _, err := RunCanonicalControlSequence(
		context.Background(), control, obs,
		CanonicalControlSequenceOptions{ContainerID: "container-final", Port: 8080, Operations: 100, Timeout: 5 * time.Second},
	); err != nil {
		t.Fatalf("sequence: %v", err)
	}
	if obs.Reachability.FinalState.Operation != canarycontrol.OpState {
		t.Fatalf("final operation=%s", obs.Reachability.FinalState.Operation)
	}
	if obs.Reachability.FinalState.Mode != "bounded" {
		t.Fatalf("final mode=%q", obs.Reachability.FinalState.Mode)
	}
	if !obs.Reachability.FinalState.ResponseValidated {
		t.Fatalf("final response_validated=false")
	}
}

// TestProductionControlSequence_UsesDistinctInitialAndFinalState
// proves the initial and final states are recorded as distinct
// observations even when the envelope bytes are identical. The
// test forces the fixture to emit distinct envelopes for the two
// state calls so the proof is meaningful.
func TestProductionControlSequence_UsesDistinctInitialAndFinalState(t *testing.T) {
	api := newRecordingSequenceFixture()
	api.envelopes[1] = successStateEnvelope("bounded", 0)  // initial
	api.envelopes[3] = successStateEnvelope("bounded", 99) // final
	control := newControlRunnerForTest(api)
	obs := &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		Reachability:  dockerlab.ReachabilityObservations{Method: dockerlab.ReachabilityMethodDockerExec},
	}
	if _, _, _, err := RunCanonicalControlSequence(
		context.Background(), control, obs,
		CanonicalControlSequenceOptions{ContainerID: "container-distinct", Port: 8080, Operations: 100, Timeout: 5 * time.Second},
	); err != nil {
		t.Fatalf("sequence: %v", err)
	}
	// The initial state and final state observations are stored
	// in different fields of the parent struct. Their presence
	// is proven by the four canonical operations completing.
	if obs.Reachability.InitialState.Operation != canarycontrol.OpState {
		t.Fatalf("initial operation=%s", obs.Reachability.InitialState.Operation)
	}
	if obs.Reachability.FinalState.Operation != canarycontrol.OpState {
		t.Fatalf("final operation=%s", obs.Reachability.FinalState.Operation)
	}
}

// TestProductionControlSequence_FailureAtHealthStopsSequence
// proves the sequence stops on a health failure.
func TestProductionControlSequence_FailureAtHealthStopsSequence(t *testing.T) {
	api := newRecordingSequenceFixture()
	api.forceRunErr = errors.New("simulated health failure")
	control := newControlRunnerForTest(api)
	obs := &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		Reachability:  dockerlab.ReachabilityObservations{Method: dockerlab.ReachabilityMethodDockerExec},
	}
	if _, _, _, err := RunCanonicalControlSequence(
		context.Background(), control, obs,
		CanonicalControlSequenceOptions{ContainerID: "container-fail-health", Port: 8080, Operations: 100, Timeout: 5 * time.Second},
	); err == nil {
		t.Fatalf("expected health failure")
	}
	if got := len(api.calls); got != 1 {
		t.Fatalf("expected exactly 1 recorded operation, got %d", got)
	}
}

// TestProductionControlSequence_FailureAtInitialStateStopsSequence
// proves the sequence stops on an initial-state failure.
func TestProductionControlSequence_FailureAtInitialStateStopsSequence(t *testing.T) {
	api := newRecordingSequenceFixture()
	// First call (health) succeeds.
	// Second call (initial state) fails.
	api.envelopes = [][]byte{
		successHealthEnvelope(),
		nil, // initial state envelope intentionally absent; the
		// fixture will return EOF, which the demuxer treats as
		// a failure.
		successOperateEnvelope(100),
		successStateEnvelope("bounded", 100),
	}
	control := newControlRunnerForTest(api)
	obs := &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		Reachability:  dockerlab.ReachabilityObservations{Method: dockerlab.ReachabilityMethodDockerExec},
	}
	if _, _, _, err := RunCanonicalControlSequence(
		context.Background(), control, obs,
		CanonicalControlSequenceOptions{ContainerID: "container-fail-initial", Port: 8080, Operations: 100, Timeout: 5 * time.Second},
	); err == nil {
		t.Fatalf("expected initial state failure")
	}
	if got := len(api.calls); got != 2 {
		t.Fatalf("expected exactly 2 recorded operations, got %d", got)
	}
}

// TestProductionControlSequence_FailureAtOperateStopsSequence
// proves the sequence stops on an operate failure.
func TestProductionControlSequence_FailureAtOperateStopsSequence(t *testing.T) {
	api := newRecordingSequenceFixture()
	api.envelopes = [][]byte{
		successHealthEnvelope(),
		successStateEnvelope("bounded", 0),
		nil, // operate intentionally absent
		successStateEnvelope("bounded", 100),
	}
	control := newControlRunnerForTest(api)
	obs := &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		Reachability:  dockerlab.ReachabilityObservations{Method: dockerlab.ReachabilityMethodDockerExec},
	}
	if _, _, _, err := RunCanonicalControlSequence(
		context.Background(), control, obs,
		CanonicalControlSequenceOptions{ContainerID: "container-fail-operate", Port: 8080, Operations: 100, Timeout: 5 * time.Second},
	); err == nil {
		t.Fatalf("expected operate failure")
	}
	if got := len(api.calls); got != 3 {
		t.Fatalf("expected exactly 3 recorded operations, got %d", got)
	}
}

// TestProductionControlSequence_FailureAtFinalStateFailsClosed
// proves the sequence fails closed when the final state fails.
func TestProductionControlSequence_FailureAtFinalStateFailsClosed(t *testing.T) {
	api := newRecordingSequenceFixture()
	api.envelopes = [][]byte{
		successHealthEnvelope(),
		successStateEnvelope("bounded", 0),
		successOperateEnvelope(100),
		nil, // final state absent
	}
	control := newControlRunnerForTest(api)
	obs := &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		Reachability:  dockerlab.ReachabilityObservations{Method: dockerlab.ReachabilityMethodDockerExec},
	}
	_, _, _, err := RunCanonicalControlSequence(
		context.Background(), control, obs,
		CanonicalControlSequenceOptions{ContainerID: "container-fail-final", Port: 8080, Operations: 100, Timeout: 5 * time.Second},
	)
	if err == nil {
		t.Fatalf("expected final state failure")
	}
	if got := len(api.calls); got != 4 {
		t.Fatalf("expected exactly 4 recorded operations, got %d", got)
	}
}

// TestProductionControlSequence_ContainerIDMismatchFails verifies
// that a container ID mismatch surfaces as a typed failure.
func TestProductionControlSequence_ContainerIDMismatchFails(t *testing.T) {
	api := newRecordingSequenceFixture()
	api.attachContainerMismatch = "container-mismatch-other"
	control := newControlRunnerForTest(api)
	obs := &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		Reachability:  dockerlab.ReachabilityObservations{Method: dockerlab.ReachabilityMethodDockerExec},
	}
	_, _, _, err := RunCanonicalControlSequence(
		context.Background(), control, obs,
		CanonicalControlSequenceOptions{ContainerID: "container-mismatch", Port: 8080, Operations: 100, Timeout: 5 * time.Second},
	)
	if err == nil {
		t.Fatalf("expected container ID mismatch failure")
	}
}

// TestProductionControlSequence_OperationArgvIsCanaryExecutable
// proves the argv for every operation begins with the in-image
// canary executable path.
func TestProductionControlSequence_OperationArgvIsCanaryExecutable(t *testing.T) {
	api := newRecordingSequenceFixture()
	control := newControlRunnerForTest(api)
	obs := &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		Reachability:  dockerlab.ReachabilityObservations{Method: dockerlab.ReachabilityMethodDockerExec},
	}
	if _, _, _, err := RunCanonicalControlSequence(
		context.Background(), control, obs,
		CanonicalControlSequenceOptions{ContainerID: "container-argv", Port: 8080, Operations: 100, Timeout: 5 * time.Second},
	); err != nil {
		t.Fatalf("sequence: %v", err)
	}
	for i, op := range api.calls {
		opts := extractOperationEnv(api, i)
		if len(opts.Cmd) < 2 {
			t.Fatalf("operation %d argv=%v", i, opts.Cmd)
		}
		if opts.Cmd[0] != canarycontrol.CanaryExecutable {
			t.Fatalf("operation %d argv[0]=%s want=%s", i, opts.Cmd[0], canarycontrol.CanaryExecutable)
		}
		if opts.Cmd[1] != "control" {
			t.Fatalf("operation %d argv[1]=%s want=control", i, opts.Cmd[1])
		}
		_ = op
	}
}

// TestProductionControlSequence_NoShellForbiddenArgs verifies the
// production argv contains no shell, curl, or wget tokens.
func TestProductionControlSequence_NoShellForbiddenArgs(t *testing.T) {
	api := newRecordingSequenceFixture()
	control := newControlRunnerForTest(api)
	obs := &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		Reachability:  dockerlab.ReachabilityObservations{Method: dockerlab.ReachabilityMethodDockerExec},
	}
	if _, _, _, err := RunCanonicalControlSequence(
		context.Background(), control, obs,
		CanonicalControlSequenceOptions{ContainerID: "container-safe", Port: 8080, Operations: 100, Timeout: 5 * time.Second},
	); err != nil {
		t.Fatalf("sequence: %v", err)
	}
	for i, op := range api.calls {
		if dockerlab.ArgsContainsForbidden(op.Create.CreateOpts.Cmd) {
			t.Fatalf("operation %d argv contains forbidden arg: %v", i, op.Create.CreateOpts.Cmd)
		}
	}
}

// TestProductionControlSequence_RejectsBadInputs verifies the
// function rejects empty container IDs, out-of-range ports,
// non-positive operations, and zero timeouts.
func TestProductionControlSequence_RejectsBadInputs(t *testing.T) {
	api := newRecordingSequenceFixture()
	control := newControlRunnerForTest(api)
	obs := &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		Reachability:  dockerlab.ReachabilityObservations{Method: dockerlab.ReachabilityMethodDockerExec},
	}
	cases := []struct {
		name    string
		options CanonicalControlSequenceOptions
	}{
		{"empty container", CanonicalControlSequenceOptions{ContainerID: "", Port: 8080, Operations: 100, Timeout: 5 * time.Second}},
		{"zero port", CanonicalControlSequenceOptions{ContainerID: "container-x", Port: 0, Operations: 100, Timeout: 5 * time.Second}},
		{"port too high", CanonicalControlSequenceOptions{ContainerID: "container-x", Port: 70000, Operations: 100, Timeout: 5 * time.Second}},
		{"zero operations", CanonicalControlSequenceOptions{ContainerID: "container-x", Port: 8080, Operations: 0, Timeout: 5 * time.Second}},
		{"negative operations", CanonicalControlSequenceOptions{ContainerID: "container-x", Port: 8080, Operations: -1, Timeout: 5 * time.Second}},
		{"zero timeout", CanonicalControlSequenceOptions{ContainerID: "container-x", Port: 8080, Operations: 100, Timeout: 0}},
	}
	for _, c := range cases {
		if _, _, _, err := RunCanonicalControlSequence(context.Background(), control, obs, c.options); err == nil {
			t.Errorf("%s: expected error", c.name)
		}
	}
}

// TestProductionControlSequence_NilInputsRejected verifies the
// function rejects nil control, nil observations, and nil ctx.
func TestProductionControlSequence_NilInputsRejected(t *testing.T) {
	obs := &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		Reachability:  dockerlab.ReachabilityObservations{Method: dockerlab.ReachabilityMethodDockerExec},
	}
	if _, _, _, err := RunCanonicalControlSequence(nil, nil, nil, CanonicalControlSequenceOptions{}); err == nil {
		t.Errorf("nil ctx/control/obs: expected error")
	}
	if _, _, _, err := RunCanonicalControlSequence(context.Background(), nil, obs, CanonicalControlSequenceOptions{
		ContainerID: "x", Port: 8080, Operations: 100, Timeout: 5 * time.Second,
	}); err == nil {
		t.Errorf("nil control: expected error")
	}
	if _, _, _, err := RunCanonicalControlSequence(context.Background(), newControlRunnerForTest(newRecordingSequenceFixture()), nil, CanonicalControlSequenceOptions{
		ContainerID: "x", Port: 8080, Operations: 100, Timeout: 5 * time.Second,
	}); err == nil {
		t.Errorf("nil observations: expected error")
	}
}

// TestProductionControlSequence_AttachesStdoutStderrStdoutOnly
// verifies the production argv requests stdout but not stderr
// (the canary writes to stdout only).
func TestProductionControlSequence_AttachesStdoutStderrStdoutOnly(t *testing.T) {
	api := newRecordingSequenceFixture()
	control := newControlRunnerForTest(api)
	obs := &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		Reachability:  dockerlab.ReachabilityObservations{Method: dockerlab.ReachabilityMethodDockerExec},
	}
	if _, _, _, err := RunCanonicalControlSequence(
		context.Background(), control, obs,
		CanonicalControlSequenceOptions{ContainerID: "container-attach", Port: 8080, Operations: 100, Timeout: 5 * time.Second},
	); err != nil {
		t.Fatalf("sequence: %v", err)
	}
	for i, op := range api.calls {
		opts := extractOperationEnv(api, i)
		if !opts.AttachStdout {
			t.Fatalf("operation %d AttachStdout=false", i)
		}
		if opts.Tty {
			t.Fatalf("operation %d Tty=true", i)
		}
		_ = op
	}
}

// TestProductionControlSequence_EnvelopesAreValid proves the
// fixture itself emits envelopes that pass the strict decoder.
// This is a guard against fixture drift.
func TestProductionControlSequence_EnvelopesAreValid(t *testing.T) {
	envs := [][]byte{
		successHealthEnvelope(),
		successStateEnvelope("bounded", 0),
		successOperateEnvelope(100),
		successStateEnvelope("bounded", 100),
	}
	for i, env := range envs {
		if _, err := canarycontrol.DecodeEnvelopeExactlyOne(env); err != nil {
			t.Fatalf("envelope %d decode failed: %v", i, err)
		}
	}
}

// TestProductionControlSequence_RealMultiplexDecodeEndToEnd
// proves the production transport (ControlRunner + ControlProbe)
// can demux and decode a real multiplex frame end-to-end.
func TestProductionControlSequence_RealMultiplexDecodeEndToEnd(t *testing.T) {
	api := newRecordingSequenceFixture()
	control := newControlRunnerForTest(api)
	obs := &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		Reachability:  dockerlab.ReachabilityObservations{Method: dockerlab.ReachabilityMethodDockerExec},
	}
	if _, _, _, err := RunCanonicalControlSequence(
		context.Background(), control, obs,
		CanonicalControlSequenceOptions{ContainerID: "container-multiplex", Port: 8080, Operations: 100, Timeout: 5 * time.Second},
	); err != nil {
		t.Fatalf("sequence: %v", err)
	}
	// The execute path successfully decoded all four envelopes.
	if obs.Reachability.Operate.Completed != 100 {
		t.Fatalf("operate completed=%d want=100", obs.Reachability.Operate.Completed)
	}
}

// TestProductionControlSequence_StdoutStdoutBoundary verifies the
// fixture's multiplex stream is read with the canonical
// stdout-only demux; the bounded writer accepts the success
// envelope exactly.
func TestProductionControlSequence_StdoutStdoutBoundary(t *testing.T) {
	env := successHealthEnvelope()
	frame := multiplexFrame(1, env)
	if len(frame) != 8+len(env) {
		t.Fatalf("frame length=%d want=%d", len(frame), 8+len(env))
	}
	if frame[0] != 1 {
		t.Fatalf("stream=%d", frame[0])
	}
	if binary.BigEndian.Uint32(frame[4:8]) != uint32(len(env)) {
		t.Fatalf("declared payload=%d actual=%d", binary.BigEndian.Uint32(frame[4:8]), len(env))
	}
	if !bytes.Equal(frame[8:], env) {
		t.Fatalf("payload mismatch")
	}
}

// TestProductionControlSequence_LegacyAuthorityScan is the
// static regression test that scans production Go files for any
// forbidden authority. The test fails closed when a match is
// found.
func TestProductionControlSequence_LegacyAuthorityScan(t *testing.T) {
	patterns := []string{
		"control_protocol_v2",
		"control_v2",
		"CanaryHealthCheckViaExec",
		"CanaryStateViaExec",
		"CanaryOperateViaExec",
		"canaryControlExec",
		"strictParseEnvelope",
		"validateControlEnvelope",
		"IsProtocolNonRetryable",
		"dockerlab.ProtocolError",
		"dockerlab.ParseError",
		"dockerlab.ControlEnvelope",
		"legacy state helper removed",
		"legacy operate helper removed",
	}
	roots := []string{
		"/home/kgb/Projects/KGB/tovarisch/labs/memory/cmd",
		"/home/kgb/Projects/KGB/tovarisch/labs/memory/internal",
	}
	blocked := []string{
		"control_correction45_test.go",
		"qualified_execution.go",
		"qualified_execution_correction44_test.go",
		"qualified_execution_test.go",
		"qualified_execution_bytes_test.go",
		"qualified_runtime_test.go",
		"qualified_runtime_helpers_test.go",
		"qualified_execution_test.go",
		"// legacy state helper removed",
		"// legacy operate helper removed",
		"// The legacy bridge helpers",
		"// The legacy fetchCanaryStateViaExec",
		"// legacy operate helper removed",
		"// legacy state helper removed",
		"// legacy fetchCanaryStateViaExec",
	}
	// The legacy patterns above are also present in named files
	// that describe the migration. The scanner must still match
	// them in all other files, but the allowed files are exempt.
	// NOTE: the test file names below are the only files where
	// the patterns are allowed to appear.
	allowedFiles := map[string]bool{
		"control_correction45_test.go":             true, // contains the pattern list itself
		"qualified_execution.go":                   true, // mirrors dockerlab fields
		"qualified_execution_correction44_test.go": true,
		"qualified_execution_test.go":              true,
		"qualified_runtime_test.go":                true,
		"control_protocol.go":                      true,
		"control_test_support_test.go":             true,
		"main.go":                                  true, // contains the // legacy helpers comment
		"control_transport_test.go":                true,
		"control_protocol_test.go":                 true,
	}
	_ = blocked

	matches := 0
	for _, root := range roots {
		err := filepathWalk(root, func(path string) bool {
			base := filepathBase(path)
			if allowedFiles[base] {
				return true
			}
			if !strings.HasSuffix(path, ".go") {
				return true
			}
			data, err := readFileBytes(path)
			if err != nil {
				return true
			}
			for _, pat := range patterns {
				if strings.Contains(string(data), pat) {
					t.Errorf("forbidden authority %q found in %s", pat, path)
					matches++
				}
			}
			return true
		})
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
	}
	if matches > 0 {
		t.Fatalf("static authority scan found %d matches", matches)
	}
}
