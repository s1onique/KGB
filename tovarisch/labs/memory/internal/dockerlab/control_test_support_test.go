package dockerlab

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"time"
)

type FakeSleeper struct {
	mu    sync.Mutex
	Calls []time.Duration
}

func (f *FakeSleeper) Sleep(ctx context.Context, d time.Duration) error {
	f.mu.Lock()
	f.Calls = append(f.Calls, d)
	f.mu.Unlock()
	return nil
}

type RecordedCall struct {
	Kind        string
	ContainerID string
	ExecID      string
	Create      *ExecCreateOptions
}

type FakeControlExecRuntime struct {
	mu sync.Mutex

	NextCreateExecID   string
	ReturnEmptyExecID  bool
	NextCreateErr      error
	NextAttachErr      error
	NextInspectResult  ExecInspectResult
	NextInspectErr     error
	NextAttachStdout   []byte
	NextAttachStderr   []byte
	Stream             io.Reader
	AttachmentOverride ControlExecAttachment
	CloseErr           error
	BlockCreate        bool
	BlockAttach        bool
	BlockInspect       bool

	Calls      []RecordedCall
	Attachment *fakeAttachment
}

var _ ControlExecRuntime = (*FakeControlExecRuntime)(nil)

func (f *FakeControlExecRuntime) ExecCreate(ctx context.Context, containerID string, opts ExecCreateOptions) (string, error) {
	f.mu.Lock()
	copyOpts := opts
	copyOpts.Command = append([]string(nil), opts.Command...)
	f.Calls = append(f.Calls, RecordedCall{Kind: "ExecCreate", ContainerID: containerID, Create: &copyOpts})
	block, id, err := f.BlockCreate, f.NextCreateExecID, f.NextCreateErr
	f.mu.Unlock()
	if block {
		<-ctx.Done()
		return "", context.Cause(ctx)
	}
	if err != nil {
		return "", err
	}
	if id == "" && !f.ReturnEmptyExecID {
		id = "exec-fake-1"
	}
	return id, nil
}

func (f *FakeControlExecRuntime) ExecAttach(ctx context.Context, containerID, execID string) (ControlExecAttachment, error) {
	f.mu.Lock()
	f.Calls = append(f.Calls, RecordedCall{Kind: "ExecAttach", ContainerID: containerID, ExecID: execID})
	block, err := f.BlockAttach, f.NextAttachErr
	override := f.AttachmentOverride
	stream := f.Stream
	if stream == nil {
		stream = multiplexedStream(f.NextAttachStdout, f.NextAttachStderr)
	}
	a := &fakeAttachment{reader: stream, closeErr: f.CloseErr}
	f.Attachment = a
	f.mu.Unlock()
	if block {
		<-ctx.Done()
		return nil, context.Cause(ctx)
	}
	if err != nil {
		return nil, err
	}
	if override != nil {
		return override, nil
	}
	return a, nil
}

func (f *FakeControlExecRuntime) ExecInspect(ctx context.Context, containerID, execID string) (ExecInspectResult, error) {
	f.mu.Lock()
	f.Calls = append(f.Calls, RecordedCall{Kind: "ExecInspect", ContainerID: containerID, ExecID: execID})
	block, result, err := f.BlockInspect, f.NextInspectResult, f.NextInspectErr
	f.mu.Unlock()
	if block {
		<-ctx.Done()
		return ExecInspectResult{}, context.Cause(ctx)
	}
	return result, err
}

func (f *FakeControlExecRuntime) CallsByKind(kind string) []RecordedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []RecordedCall
	for _, call := range f.Calls {
		if call.Kind == kind {
			out = append(out, call)
		}
	}
	return out
}

type fakeAttachment struct {
	reader    io.Reader
	closeErr  error
	closeHook func()
	closeOnce sync.Once
	closeDone chan struct{}
	mu        sync.Mutex
	closed    int
	result    error
}

func (a *fakeAttachment) Reader() io.Reader { return a.reader }
func (a *fakeAttachment) Close() error {
	a.closeOnce.Do(func() {
		a.mu.Lock()
		a.closed++
		a.result = a.closeErr
		a.mu.Unlock()
		if a.closeHook != nil {
			a.closeHook()
		}
		if a.closeDone != nil {
			close(a.closeDone)
		}
	})
	return a.result
}
func (a *fakeAttachment) closeCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closed
}

func multiplexedStream(stdout, stderr []byte) io.Reader {
	var out bytes.Buffer
	writeFrame := func(stream byte, data []byte) {
		if len(data) == 0 {
			return
		}
		header := make([]byte, 8)
		header[0] = stream
		binary.BigEndian.PutUint32(header[4:], uint32(len(data)))
		out.Write(header)
		out.Write(data)
	}
	writeFrame(1, stdout)
	writeFrame(2, stderr)
	return bytes.NewReader(out.Bytes())
}

type failAfterReader struct {
	reader io.Reader
	err    error
}

func (r *failAfterReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if errors.Is(err, io.EOF) {
		return n, r.err
	}
	return n, err
}
