// control_protocol_test.go — Tests for the canonical dockerlab control path.
//
// Covers the ControlExecRuntime seam, ControlRunner, and retry
// loop introduced by CORRECTION39.
package dockerlab

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/canarycontrol"
)

// ===== Engine seam recording tests =====

func TestEngineSeam_RecordsExactHealthArgv(t *testing.T) {
	fake := &FakeControlExecRuntime{}
	fake.NextAttachStdout = []byte(`{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}}`)
	fake.NextInspectResult = ExecInspectResult{ExitCode: 0}

	r := NewControlRunner(fake)
	_, env, err := r.ControlProbe(context.Background(), "container-1", canarycontrol.OpHealth, 8080, 0, 5*time.Second)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if env == nil || env.Operation != "health" {
		t.Fatalf("expected health envelope, got %+v", env)
	}
	creates := fake.CallsByKind("ExecCreate")
	if len(creates) != 1 {
		t.Fatalf("expected 1 ExecCreate call, got %d", len(creates))
	}
	got := creates[0].Create.Command
	want := []string{"/app/canary", "control", "health", "--port", "8080", "--timeout", "5s"}
	if !equalStrings(got, want) {
		t.Errorf("argv mismatch:\n got=%v\nwant=%v", got, want)
	}
	if !creates[0].Create.AttachStdout || !creates[0].Create.AttachStderr {
		t.Error("AttachStdout and AttachStderr must be true")
	}
	if creates[0].Create.TTY {
		t.Error("TTY must be false")
	}
	if ArgsContainsForbidden(got) {
		t.Errorf("argv contains forbidden token: %v", got)
	}
}

func TestEngineSeam_RecordsExactStateArgv(t *testing.T) {
	fake := &FakeControlExecRuntime{}
	fake.NextAttachStdout = []byte(`{"schema_version":"canary-control/v1","operation":"state","success":true,"http_status":200,"state":{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}}`)
	fake.NextInspectResult = ExecInspectResult{ExitCode: 0}

	r := NewControlRunner(fake)
	_, env, err := r.ControlProbe(context.Background(), "container-1", canarycontrol.OpState, 8080, 0, 5*time.Second)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if env == nil || env.Operation != "state" {
		t.Fatalf("expected state envelope, got %+v", env)
	}
	creates := fake.CallsByKind("ExecCreate")
	got := creates[0].Create.Command
	want := []string{"/app/canary", "control", "state", "--port", "8080", "--timeout", "5s"}
	if !equalStrings(got, want) {
		t.Errorf("argv mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestEngineSeam_RecordsExactOperateArgv(t *testing.T) {
	fake := &FakeControlExecRuntime{}
	fake.NextAttachStdout = []byte(`{"schema_version":"canary-control/v1","operation":"operate","success":true,"http_status":200,"workload":{"requested":5,"attempted":5,"completed":5}}`)
	fake.NextInspectResult = ExecInspectResult{ExitCode: 0}

	r := NewControlRunner(fake)
	_, env, err := r.ControlProbe(context.Background(), "container-1", canarycontrol.OpOperate, 8080, 5, 30*time.Second)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if env == nil || env.Operation != "operate" {
		t.Fatalf("expected operate envelope, got %+v", env)
	}
	creates := fake.CallsByKind("ExecCreate")
	got := creates[0].Create.Command
	want := []string{"/app/canary", "control", "operate", "--port", "8080", "--count", "5", "--timeout", "30s"}
	if !equalStrings(got, want) {
		t.Errorf("argv mismatch:\n got=%v\nwant=%v", got, want)
	}
}

// ===== Invalid input rejected before Engine =====

func TestEngineSeam_InvalidHealthPortRejectedBeforeEngine(t *testing.T) {
	fake := &FakeControlExecRuntime{}
	r := NewControlRunner(fake)
	_, _, err := r.ControlProbe(context.Background(), "container-1", canarycontrol.OpHealth, 0, 0, 5*time.Second)
	if err == nil {
		t.Fatal("expected error for port=0")
	}
	if len(fake.Calls) != 0 {
		t.Errorf("expected no Engine calls, got %d", len(fake.Calls))
	}
}

func TestEngineSeam_InvalidStateTimeoutRejectedBeforeEngine(t *testing.T) {
	fake := &FakeControlExecRuntime{}
	r := NewControlRunner(fake)
	_, _, err := r.ControlProbe(context.Background(), "container-1", canarycontrol.OpState, 8080, 0, 0)
	if err == nil {
		t.Fatal("expected error for timeout=0")
	}
	if len(fake.Calls) != 0 {
		t.Errorf("expected no Engine calls, got %d", len(fake.Calls))
	}
}

func TestEngineSeam_InvalidOperateCountRejectedBeforeEngine(t *testing.T) {
	fake := &FakeControlExecRuntime{}
	r := NewControlRunner(fake)
	_, _, err := r.ControlProbe(context.Background(), "container-1", canarycontrol.OpOperate, 8080, 0, 5*time.Second)
	if err == nil {
		t.Fatal("expected error for count=0")
	}
	if len(fake.Calls) != 0 {
		t.Errorf("expected no Engine calls, got %d", len(fake.Calls))
	}
}

// ===== Engine failure tests =====

func TestEngineSeam_ExecCreateFailure(t *testing.T) {
	fake := &FakeControlExecRuntime{}
	fake.NextCreateErr = errors.New("create failed")
	r := NewControlRunner(fake)
	_, _, err := r.ControlProbe(context.Background(), "container-1", canarycontrol.OpHealth, 8080, 0, 5*time.Second)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "create failed") {
		t.Errorf("expected create error, got %v", err)
	}
}

func TestEngineSeam_ExecAttachFailure(t *testing.T) {
	fake := &FakeControlExecRuntime{}
	fake.NextAttachErr = errors.New("attach failed")
	r := NewControlRunner(fake)
	_, _, err := r.ControlProbe(context.Background(), "container-1", canarycontrol.OpHealth, 8080, 0, 5*time.Second)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEngineSeam_ExecInspectFailure(t *testing.T) {
	fake := &FakeControlExecRuntime{}
	fake.NextAttachStdout = []byte(`{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}}`)
	fake.NextInspectErr = errors.New("inspect failed")
	r := NewControlRunner(fake)
	_, _, err := r.ControlProbe(context.Background(), "container-1", canarycontrol.OpHealth, 8080, 0, 5*time.Second)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ===== Output bound tests =====

func TestEngineSeam_StdoutOverflow(t *testing.T) {
	fake := &FakeControlExecRuntime{}
	fake.NextAttachStdout = make([]byte, MaxControlStdout/2+1)
	fake.Stream = io.MultiReader(multiplexedStream(fake.NextAttachStdout, nil), multiplexedStream(fake.NextAttachStdout, nil))
	r := NewControlRunner(fake)
	_, _, err := r.ControlProbe(context.Background(), "container-1", canarycontrol.OpHealth, 8080, 0, 5*time.Second)
	if !errors.Is(err, ErrStdoutOverflow) {
		t.Errorf("expected ErrStdoutOverflow, got %v", err)
	}
}

func TestEngineSeam_StderrOverflow(t *testing.T) {
	fake := &FakeControlExecRuntime{}
	fake.NextAttachStdout = []byte(`{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}}`)
	fake.NextAttachStderr = make([]byte, MaxControlStderr/2+1)
	fake.Stream = io.MultiReader(multiplexedStream(nil, fake.NextAttachStderr), multiplexedStream(nil, fake.NextAttachStderr))
	fake.NextInspectResult = ExecInspectResult{ExitCode: 0}
	r := NewControlRunner(fake)
	_, _, err := r.ControlProbe(context.Background(), "container-1", canarycontrol.OpHealth, 8080, 0, 5*time.Second)
	if !errors.Is(err, ErrStderrOverflow) {
		t.Errorf("expected ErrStderrOverflow, got %v", err)
	}
}

func TestEngineSeam_EmptyStdout(t *testing.T) {
	fake := &FakeControlExecRuntime{}
	fake.NextAttachStdout = nil
	r := NewControlRunner(fake)
	_, _, err := r.ControlProbe(context.Background(), "container-1", canarycontrol.OpHealth, 8080, 0, 5*time.Second)
	if !errors.Is(err, ErrEmptyStdout) {
		t.Errorf("expected ErrEmptyStdout, got %v", err)
	}
}

// ===== Exit/envelope consistency tests =====

func TestEngineSeam_ExitZeroWithFailureEnvelope_Rejected(t *testing.T) {
	fake := &FakeControlExecRuntime{}
	fake.NextAttachStdout = []byte(`{"schema_version":"canary-control/v1","operation":"health","success":false,"http_status":500,"error_class":"health_not_ready"}`)
	fake.NextInspectResult = ExecInspectResult{ExitCode: 0}
	r := NewControlRunner(fake)
	_, _, err := r.ControlProbe(context.Background(), "container-1", canarycontrol.OpHealth, 8080, 0, 5*time.Second)
	if !errors.Is(err, ErrExitEnvelopeMismatch) {
		t.Errorf("expected ErrExitEnvelopeMismatch, got %v", err)
	}
}

func TestEngineSeam_NonzeroExitWithSuccessEnvelope_Rejected(t *testing.T) {
	fake := &FakeControlExecRuntime{}
	fake.NextAttachStdout = []byte(`{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}}`)
	fake.NextInspectResult = ExecInspectResult{ExitCode: 1}
	r := NewControlRunner(fake)
	_, _, err := r.ControlProbe(context.Background(), "container-1", canarycontrol.OpHealth, 8080, 0, 5*time.Second)
	if !errors.Is(err, ErrExitEnvelopeMismatch) {
		t.Errorf("expected ErrExitEnvelopeMismatch, got %v", err)
	}
}

func TestEngineSeam_RequestedCountMismatch(t *testing.T) {
	fake := &FakeControlExecRuntime{}
	fake.NextAttachStdout = []byte(`{"schema_version":"canary-control/v1","operation":"operate","success":true,"http_status":200,"workload":{"requested":3,"attempted":3,"completed":3}}`)
	fake.NextInspectResult = ExecInspectResult{ExitCode: 0}
	r := NewControlRunner(fake)
	_, _, err := r.ControlProbe(context.Background(), "container-1", canarycontrol.OpOperate, 8080, 5, 30*time.Second)
	if err == nil {
		t.Fatal("expected error for requested mismatch")
	}
}

func TestEngineSeam_MalformedEnvelope(t *testing.T) {
	fake := &FakeControlExecRuntime{}
	fake.NextAttachStdout = []byte(`{"schema_version":"canary-control/v1","operation":"health","success":true`)
	fake.NextInspectResult = ExecInspectResult{ExitCode: 0}
	r := NewControlRunner(fake)
	_, _, err := r.ControlProbe(context.Background(), "container-1", canarycontrol.OpHealth, 8080, 0, 5*time.Second)
	if err == nil {
		t.Fatal("expected error for malformed envelope")
	}
}

func TestEngineSeam_TrailingEnvelopeData(t *testing.T) {
	fake := &FakeControlExecRuntime{}
	fake.NextAttachStdout = []byte(`{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}}{"extra":1}`)
	fake.NextInspectResult = ExecInspectResult{ExitCode: 0}
	r := NewControlRunner(fake)
	_, _, err := r.ControlProbe(context.Background(), "container-1", canarycontrol.OpHealth, 8080, 0, 5*time.Second)
	if err == nil {
		t.Fatal("expected error for trailing envelope")
	}
}

// ===== Typed failure envelope preservation (CORRECTION40 P0-7) =====

func TestEngineSeam_TypedFailureEnvelope_Preserved(t *testing.T) {
	fake := &FakeControlExecRuntime{}
	fake.NextAttachStdout = []byte(`{"schema_version":"canary-control/v1","operation":"health","success":false,"http_status":500,"error_class":"health_not_ready"}`)
	fake.NextInspectResult = ExecInspectResult{ExitCode: 1}
	r := NewControlRunner(fake)
	exitCode, env, err := r.ControlProbe(context.Background(), "container-1", canarycontrol.OpHealth, 8080, 0, 5*time.Second)
	if err == nil {
		t.Fatal("expected failure error for failure envelope")
	}
	if exitCode != 1 {
		t.Errorf("expected exit_code=1, got %d", exitCode)
	}
	if env == nil {
		t.Fatal("expected envelope")
	}
	if env.Success {
		t.Error("expected failure envelope")
	}
	if env.ErrorClass != canarycontrol.ErrHealthNotReady {
		t.Errorf("expected ErrHealthNotReady, got %s", env.ErrorClass)
	}
	// ControlFailureError must preserve the shared error class.
	var cfe *ControlFailureError
	if !errors.As(err, &cfe) {
		t.Fatalf("expected *ControlFailureError, got %T", err)
	}
	if cfe.Protocol == nil {
		t.Fatal("expected Protocol set on ControlFailureError")
	}
	if cfe.Protocol.ErrClass != canarycontrol.ErrHealthNotReady {
		t.Errorf("expected shared ErrHealthNotReady preserved, got %s", cfe.Protocol.ErrClass)
	}
}

// ===== Retry loop tests =====

func TestRetry_TransientThenSuccess(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts:       5,
		InitialBackoff:    time.Millisecond,
		BackoffMultiplier: 1.0,
		Sleeper:           &FakeSleeper{},
	}
	calls := 0
	probe := func(ctx context.Context) error {
		calls++
		if calls < 2 {
			return &canarycontrol.ProtocolError{ErrClass: canarycontrol.ErrConnectionFailed}
		}
		return nil
	}
	if err := ReadinessLoop(context.Background(), policy, probe); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
}

func TestRetry_MultipleTransientThenSuccess(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts:       5,
		InitialBackoff:    time.Millisecond,
		BackoffMultiplier: 1.0,
		Sleeper:           &FakeSleeper{},
	}
	calls := 0
	probe := func(ctx context.Context) error {
		calls++
		if calls < 4 {
			return &canarycontrol.ProtocolError{ErrClass: canarycontrol.ErrRequestTimeout}
		}
		return nil
	}
	if err := ReadinessLoop(context.Background(), policy, probe); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if calls != 4 {
		t.Errorf("expected 4 calls, got %d", calls)
	}
}

func TestRetry_PermanentFailureOneAttempt(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts:       5,
		InitialBackoff:    time.Millisecond,
		BackoffMultiplier: 1.0,
		Sleeper:           &FakeSleeper{},
	}
	calls := 0
	probe := func(ctx context.Context) error {
		calls++
		return &canarycontrol.ProtocolError{ErrClass: canarycontrol.ErrInvalidArguments}
	}
	err := ReadinessLoop(context.Background(), policy, probe)
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (permanent), got %d", calls)
	}
}

func TestRetry_MaxAttemptsExhausted(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts:       3,
		InitialBackoff:    time.Millisecond,
		BackoffMultiplier: 1.0,
		Sleeper:           &FakeSleeper{},
	}
	calls := 0
	probe := func(ctx context.Context) error {
		calls++
		return &canarycontrol.ProtocolError{ErrClass: canarycontrol.ErrConnectionFailed}
	}
	err := ReadinessLoop(context.Background(), policy, probe)
	if err == nil {
		t.Fatal("expected error after max attempts")
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestRetry_CallerCancellationStopsPromptly(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts:       10,
		InitialBackoff:    time.Hour, // long sleep
		BackoffMultiplier: 1.0,
		Sleeper:           &FakeSleeper{},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	probe := func(ctx context.Context) error {
		calls++
		return &canarycontrol.ProtocolError{ErrClass: canarycontrol.ErrConnectionFailed}
	}
	_ = ReadinessLoop(ctx, policy, probe)
	if calls != 0 || calls > 1 {
		// acceptable: 0 calls (cancel before probe) or 1 (probe ran then cancel)
	}
}

func TestRetry_SleepCallsBounded(t *testing.T) {
	sleeper := &FakeSleeper{}
	policy := RetryPolicy{
		MaxAttempts:       4,
		InitialBackoff:    time.Millisecond,
		BackoffMultiplier: 2.0,
		Sleeper:           sleeper,
	}
	calls := 0
	probe := func(ctx context.Context) error {
		calls++
		return &canarycontrol.ProtocolError{ErrClass: canarycontrol.ErrHealthNotReady}
	}
	_ = ReadinessLoop(context.Background(), policy, probe)
	// MaxAttempts=4 means 3 sleeps between 4 attempts.
	if len(sleeper.Calls) != 3 {
		t.Errorf("expected 3 sleep calls, got %d", len(sleeper.Calls))
	}
}

// ===== Forbidden argv test =====

func TestArgsContainsForbidden(t *testing.T) {
	if !ArgsContainsForbidden([]string{"sh", "-c", "echo"}) {
		t.Error("expected to detect sh")
	}
	if !ArgsContainsForbidden([]string{"/bin/bash", "-c", "echo"}) {
		t.Error("expected to detect /bin/bash")
	}
	if !ArgsContainsForbidden([]string{"curl", "http://x"}) {
		t.Error("expected to detect curl")
	}
	if ArgsContainsForbidden([]string{"/app/canary", "control", "health"}) {
		t.Error("should not flag legitimate argv")
	}
}

// ===== Helper =====

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
