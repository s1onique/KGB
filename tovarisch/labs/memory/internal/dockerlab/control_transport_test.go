package dockerlab

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/canarycontrol"
)

const healthEnvelope = `{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}}`

type recordingDockerExecAPI struct {
	createContainer string
	createOptions   types.ExecConfig
	attachExecID    string
	attachOptions   types.ExecStartCheck
	inspectExecID   string
	createResponse  types.IDResponse
	attachResponse  types.HijackedResponse
	inspectResponse types.ContainerExecInspect
	createErr       error
	attachErr       error
	inspectErr      error
}

func (f *recordingDockerExecAPI) ContainerExecCreate(_ context.Context, containerID string, options types.ExecConfig) (types.IDResponse, error) {
	f.createContainer, f.createOptions = containerID, options
	return f.createResponse, f.createErr
}
func (f *recordingDockerExecAPI) ContainerExecAttach(_ context.Context, execID string, options types.ExecStartCheck) (types.HijackedResponse, error) {
	f.attachExecID, f.attachOptions = execID, options
	return f.attachResponse, f.attachErr
}
func (f *recordingDockerExecAPI) ContainerExecInspect(_ context.Context, execID string) (types.ContainerExecInspect, error) {
	f.inspectExecID = execID
	return f.inspectResponse, f.inspectErr
}

func TestControlAdapter_ExactCreateOptions(t *testing.T) {
	api := &recordingDockerExecAPI{createResponse: types.IDResponse{ID: "exec-exact"}}
	runtime := &ProductionControlExecRuntime{Client: api}
	argv := []string{"/app/canary", "control", "health"}
	id, err := runtime.ExecCreate(context.Background(), "container-exact", ExecCreateOptions{Command: argv, AttachStdout: true, AttachStderr: true})
	if err != nil || id != "exec-exact" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if api.createContainer != "container-exact" {
		t.Fatalf("container=%q", api.createContainer)
	}
	if !reflect.DeepEqual(api.createOptions.Cmd, argv) || !api.createOptions.AttachStdout || !api.createOptions.AttachStderr || api.createOptions.Tty {
		t.Fatalf("options=%+v", api.createOptions)
	}
}

func TestControlAdapter_ExactContainerAndExecIdentity(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	api := &recordingDockerExecAPI{createResponse: types.IDResponse{ID: "exec-exact"}, attachResponse: types.HijackedResponse{Conn: client, Reader: bufio.NewReader(client)}, inspectResponse: types.ContainerExecInspect{ExecID: "exec-exact", ContainerID: "container-exact", ExitCode: 7, Running: true}}
	runtime := &ProductionControlExecRuntime{Client: api}
	id, err := runtime.ExecCreate(context.Background(), "container-exact", ExecCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	attachment, err := runtime.ExecAttach(context.Background(), "container-exact", id)
	if err != nil {
		t.Fatal(err)
	}
	if err := attachment.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := runtime.ExecInspect(context.Background(), "container-exact", id)
	if err != nil {
		t.Fatal(err)
	}
	if api.createContainer != "container-exact" || api.attachExecID != id || api.inspectExecID != id {
		t.Fatalf("identity create=%q attach=%q inspect=%q", api.createContainer, api.attachExecID, api.inspectExecID)
	}
	if got.ExitCode != 7 || !got.Running {
		t.Fatalf("inspect=%+v", got)
	}
}

func successfulFake() *FakeControlExecRuntime {
	return &FakeControlExecRuntime{NextCreateExecID: "exec-identity", NextAttachStdout: []byte(healthEnvelope), NextInspectResult: ExecInspectResult{ExitCode: 0}}
}
func probe(fake *FakeControlExecRuntime, ctx context.Context, timeout time.Duration) error {
	_, _, err := NewControlRunner(fake).ControlProbe(ctx, "container-identity", canarycontrol.OpHealth, 8080, 0, timeout)
	return err
}

func TestControlAdapter_ClosesAttachmentOnSuccess(t *testing.T) {
	fake := successfulFake()
	if err := probe(fake, context.Background(), time.Second); err != nil {
		t.Fatal(err)
	}
	if fake.Attachment.closeCount() != 1 {
		t.Fatalf("close count=%d", fake.Attachment.closeCount())
	}
}
func TestControlAdapter_ClosesAttachmentOnDemuxFailure(t *testing.T) {
	fake := successfulFake()
	demuxErr := errors.New("demux read failure")
	fake.Stream = &failAfterReader{reader: multiplexedStream([]byte(healthEnvelope), nil), err: demuxErr}
	if err := probe(fake, context.Background(), time.Second); !errors.Is(err, demuxErr) {
		t.Fatalf("err=%v", err)
	}
	if fake.Attachment.closeCount() != 1 {
		t.Fatalf("close count=%d", fake.Attachment.closeCount())
	}
}
func TestControlAdapter_ClosesAttachmentOnOverflow(t *testing.T) {
	fake := successfulFake()
	fake.NextAttachStdout = bytes.Repeat([]byte("x"), MaxControlStdout/2+1)
	fake.Stream = io.MultiReader(multiplexedStream(fake.NextAttachStdout, nil), multiplexedStream(fake.NextAttachStdout, nil))
	if err := probe(fake, context.Background(), time.Second); !errors.Is(err, ErrStdoutOverflow) {
		t.Fatalf("err=%v", err)
	}
	if fake.Attachment.closeCount() != 1 {
		t.Fatalf("close count=%d", fake.Attachment.closeCount())
	}
}
func TestControlAdapter_CloseErrorJoined(t *testing.T) {
	fake := successfulFake()
	primary, closeErr := errors.New("read failed"), errors.New("close failed")
	fake.Stream = &failAfterReader{reader: bytes.NewReader(nil), err: primary}
	fake.CloseErr = closeErr
	err := probe(fake, context.Background(), time.Second)
	if !errors.Is(err, primary) || !errors.Is(err, closeErr) {
		t.Fatalf("joined err=%v", err)
	}
}

func TestControlTransport_StdoutAtLimit(t *testing.T) {
	assertBoundedStream(t, MaxControlStdout, 0, nil)
}
func TestControlTransport_StdoutOneOver(t *testing.T) {
	assertBoundedStream(t, MaxControlStdout+1, 0, ErrStdoutOverflow)
}
func TestControlTransport_StderrAtLimit(t *testing.T) {
	assertBoundedStream(t, 1, MaxControlStderr, nil)
}
func TestControlTransport_StderrOneOver(t *testing.T) {
	assertBoundedStream(t, 1, MaxControlStderr+1, ErrStderrOverflow)
}
func TestControlTransport_BothStreamsNearLimit(t *testing.T) {
	assertBoundedStream(t, MaxControlStdout, MaxControlStderr, nil)
}
func assertBoundedStream(t *testing.T, stdoutLen, stderrLen int, want error) {
	t.Helper()
	stdout, stderr := newBoundedWriter(MaxControlStdout, ErrStdoutOverflow), newBoundedWriter(MaxControlStderr, ErrStderrOverflow)
	_, err := stdCopyForTest(stdout, stderr, multiplexedStream(bytes.Repeat([]byte("x"), stdoutLen), bytes.Repeat([]byte("y"), stderrLen)))
	if !errors.Is(err, want) {
		t.Fatalf("err=%v want=%v", err, want)
	}
	if len(stdout.Bytes()) > MaxControlStdout || len(stderr.Bytes()) > MaxControlStderr {
		t.Fatalf("retained stdout=%d stderr=%d", len(stdout.Bytes()), len(stderr.Bytes()))
	}
}
func stdCopyForTest(stdout, stderr io.Writer, reader io.Reader) (int64, error) {
	return stdcopy.StdCopy(stdout, stderr, reader)
}
func TestControlTransport_DemuxFailure(t *testing.T) {
	failure := errors.New("demux failure")
	fake := successfulFake()
	fake.Stream = &failAfterReader{reader: multiplexedStream([]byte(healthEnvelope), nil), err: failure}
	if err := probe(fake, context.Background(), time.Second); !errors.Is(err, failure) {
		t.Fatalf("err=%v", err)
	}
}

func TestControlTimeout_ParentCancellation(t *testing.T) {
	fake := successfulFake()
	fake.BlockCreate = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := probe(fake, ctx, time.Second); !errors.Is(err, context.Canceled) || errors.Is(err, ErrControlTimeout) {
		t.Fatalf("err=%v", err)
	}
}
func TestControlTimeout_ParentDeadline(t *testing.T) {
	fake := successfulFake()
	fake.BlockCreate = true
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Millisecond))
	defer cancel()
	if err := probe(fake, ctx, time.Second); !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrControlTimeout) {
		t.Fatalf("err=%v", err)
	}
}
func TestControlTimeout_AttemptDeadline(t *testing.T) {
	fake := successfulFake()
	fake.BlockCreate = true
	if err := probe(fake, context.Background(), time.Millisecond); !errors.Is(err, ErrControlTimeout) {
		t.Fatalf("err=%v", err)
	}
}

func TestControlFailureError_ProtocolOnly(t *testing.T) {
	protocol := &canarycontrol.ProtocolError{ErrClass: canarycontrol.ErrMalformedJSON, Message: "protocol"}
	err := &ControlFailureError{Protocol: protocol}
	var gotProtocol *canarycontrol.ProtocolError
	var gotFailure *ControlFailureError
	if !errors.As(err, &gotFailure) || !errors.As(err, &gotProtocol) || gotProtocol != protocol {
		t.Fatalf("chain=%v", err)
	}
}
func TestControlFailureError_CauseOnly(t *testing.T) {
	cause := errors.New("transport")
	err := &ControlFailureError{Cause: cause}
	var got *ControlFailureError
	if !errors.As(err, &got) || !errors.Is(err, cause) {
		t.Fatalf("chain=%v", err)
	}
}
func TestControlFailureError_ProtocolAndCause(t *testing.T) {
	protocol := &canarycontrol.ProtocolError{ErrClass: canarycontrol.ErrUnexpectedHTTPStatus, Message: "protocol"}
	cause := ErrExitEnvelopeMismatch
	err := &ControlFailureError{Protocol: protocol, Cause: cause}
	var gotProtocol *canarycontrol.ProtocolError
	var gotFailure *ControlFailureError
	if !errors.As(err, &gotFailure) || !errors.As(err, &gotProtocol) || !errors.Is(err, cause) {
		t.Fatalf("chain=%v", err)
	}
}

func TestControlIdentity_EmptyContainerRejected(t *testing.T) { assertContainerRejected(t, "") }
func TestControlIdentity_WhitespaceContainerRejected(t *testing.T) {
	assertContainerRejected(t, " \t\n")
}
func assertContainerRejected(t *testing.T, id string) {
	t.Helper()
	fake := successfulFake()
	_, _, err := NewControlRunner(fake).ControlProbe(context.Background(), id, canarycontrol.OpHealth, 8080, 0, time.Second)
	if !errors.Is(err, ErrContainerIDRequired) || len(fake.Calls) != 0 {
		t.Fatalf("err=%v calls=%d", err, len(fake.Calls))
	}
}
func TestControlIdentity_EmptyExecIDRejected(t *testing.T) {
	fake := successfulFake()
	fake.NextCreateExecID = ""
	fake.ReturnEmptyExecID = true
	if err := probe(fake, context.Background(), time.Second); !errors.Is(err, ErrExecIDRequired) {
		t.Fatalf("err=%v", err)
	}
	if len(fake.CallsByKind("ExecAttach")) != 0 {
		t.Fatal("attach called with empty exec ID")
	}
}
func TestControlOperation_WrongEnvelopeOperation(t *testing.T) {
	fake := successfulFake()
	fake.NextAttachStdout = []byte(`{"schema_version":"canary-control/v1","operation":"state","success":true,"http_status":200,"state":{"mode":"growing","retained_blocks":0,"retained_bytes":0,"operation_count":0,"fd_count":0,"ready":true}}`)
	if err := probe(fake, context.Background(), time.Second); !errors.Is(err, ErrWrongOperation) {
		t.Fatalf("err=%v", err)
	}
}

func TestControlIdentity_RecordsEveryEngineIdentity(t *testing.T) {
	fake := successfulFake()
	if err := probe(fake, context.Background(), time.Second); err != nil {
		t.Fatal(err)
	}
	for _, call := range fake.Calls {
		if call.ContainerID != "container-identity" {
			t.Fatalf("%s container=%q", call.Kind, call.ContainerID)
		}
		if call.Kind != "ExecCreate" && call.ExecID != "exec-identity" {
			t.Fatalf("%s exec=%q", call.Kind, call.ExecID)
		}
	}
}
