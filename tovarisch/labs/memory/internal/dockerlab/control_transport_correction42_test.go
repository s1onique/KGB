package dockerlab

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/canarycontrol"
)

func TestControlRunner_NilRuntimeRejected(t *testing.T) {
	_, _, err := NewControlRunner(nil).ControlProbe(context.Background(), "container", canarycontrol.Operation("invalid"), 0, 0, 0)
	if !errors.Is(err, ErrControlRuntimeRequired) {
		t.Fatalf("err=%v", err)
	}
}

func TestControlRunner_NilRuntimeZeroEngineCalls(t *testing.T) {
	runner := NewControlRunner(nil)
	_, _, err := runner.ControlProbe(context.Background(), "container", canarycontrol.OpHealth, 8080, 0, time.Second)
	if !errors.Is(err, ErrControlRuntimeRequired) {
		t.Fatalf("err=%v", err)
	}
}

type closeRecordingConn struct {
	mu       sync.Mutex
	closed   int
	closeErr error
}

func (c *closeRecordingConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *closeRecordingConn) Write(p []byte) (int, error)      { return len(p), nil }
func (c *closeRecordingConn) LocalAddr() net.Addr              { return dummyAddr("local") }
func (c *closeRecordingConn) RemoteAddr() net.Addr             { return dummyAddr("remote") }
func (c *closeRecordingConn) SetDeadline(time.Time) error      { return nil }
func (c *closeRecordingConn) SetReadDeadline(time.Time) error  { return nil }
func (c *closeRecordingConn) SetWriteDeadline(time.Time) error { return nil }
func (c *closeRecordingConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed++
	return c.closeErr
}
func (c *closeRecordingConn) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

type dummyAddr string

func (a dummyAddr) Network() string { return string(a) }
func (a dummyAddr) String() string  { return string(a) }

func TestControlAdapter_NilHijackedConnRejected(t *testing.T) {
	api := &recordingDockerExecAPI{attachResponse: types.HijackedResponse{Reader: bufio.NewReader(bytes.NewReader(nil))}}
	_, err := (&ProductionControlExecRuntime{Client: api}).ExecAttach(context.Background(), "container", "exec")
	if !errors.Is(err, ErrHijackedConnectionRequired) {
		t.Fatalf("err=%v", err)
	}
}

func TestControlAdapter_NilHijackedReaderRejected(t *testing.T) {
	conn := &closeRecordingConn{}
	api := &recordingDockerExecAPI{attachResponse: types.HijackedResponse{Conn: conn}}
	_, err := (&ProductionControlExecRuntime{Client: api}).ExecAttach(context.Background(), "container", "exec")
	if !errors.Is(err, ErrHijackedReaderRequired) || conn.count() != 1 {
		t.Fatalf("err=%v close_count=%d", err, conn.count())
	}
}

func TestControlAdapter_InvalidResponseCloseErrorJoined(t *testing.T) {
	closeErr := errors.New("validation close failed")
	conn := &closeRecordingConn{closeErr: closeErr}
	api := &recordingDockerExecAPI{attachResponse: types.HijackedResponse{Conn: conn}}
	_, err := (&ProductionControlExecRuntime{Client: api}).ExecAttach(context.Background(), "container", "exec")
	if !errors.Is(err, ErrHijackedReaderRequired) || !errors.Is(err, closeErr) || conn.count() != 1 {
		t.Fatalf("err=%v close_count=%d", err, conn.count())
	}
}

func TestControlAdapter_ExactAttachOptions(t *testing.T) {
	conn := &closeRecordingConn{}
	api := &recordingDockerExecAPI{attachResponse: types.HijackedResponse{Conn: conn, Reader: bufio.NewReader(bytes.NewReader(nil))}}
	attachment, err := (&ProductionControlExecRuntime{Client: api}).ExecAttach(context.Background(), "container", "exec")
	if err != nil {
		t.Fatal(err)
	}
	if api.attachOptions.Detach || api.attachOptions.Tty {
		t.Fatalf("attach options=%+v", api.attachOptions)
	}
	if err := attachment.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDockerExecAttachment_CloseIdempotent(t *testing.T) {
	conn := &closeRecordingConn{closeErr: errors.New("first close")}
	attachment := newDockerExecAttachment(types.HijackedResponse{Conn: conn, Reader: bufio.NewReader(bytes.NewReader(nil))})
	first := attachment.Close()
	second := attachment.Close()
	if !errors.Is(first, conn.closeErr) || !errors.Is(second, conn.closeErr) || conn.count() != 1 {
		t.Fatalf("first=%v second=%v close_count=%d", first, second, conn.count())
	}
}

func TestControlAdapter_CreateAndAttachTTYCannotDiverge(t *testing.T) {
	conn := &closeRecordingConn{}
	api := &recordingDockerExecAPI{createResponse: types.IDResponse{ID: "exec"}, attachResponse: types.HijackedResponse{Conn: conn, Reader: bufio.NewReader(bytes.NewReader(nil))}}
	runtime := &ProductionControlExecRuntime{Client: api}
	id, err := runtime.ExecCreate(context.Background(), "container", ExecCreateOptions{TTY: false})
	if err != nil {
		t.Fatal(err)
	}
	attachment, err := runtime.ExecAttach(context.Background(), "container", id)
	if err != nil {
		t.Fatal(err)
	}
	defer attachment.Close()
	if api.createOptions.Tty || api.attachOptions.Tty {
		t.Fatalf("create=%v attach=%v", api.createOptions.Tty, api.attachOptions.Tty)
	}
}

func frame(stream byte, declared uint32, payload []byte) []byte {
	result := make([]byte, dockerFrameHeaderSize+len(payload))
	result[0] = stream
	binary.BigEndian.PutUint32(result[4:8], declared)
	copy(result[8:], payload)
	return result
}

type chunkReader struct {
	data   []byte
	chunks []int
	index  int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	limit := len(p)
	if r.index < len(r.chunks) && r.chunks[r.index] < limit {
		limit = r.chunks[r.index]
	}
	r.index++
	if limit > len(r.data) {
		limit = len(r.data)
	}
	n := copy(p[:limit], r.data[:limit])
	r.data = r.data[n:]
	return n, nil
}

func readGuard(t *testing.T, reader io.Reader) ([]byte, error) {
	t.Helper()
	return io.ReadAll(newControlFrameGuard(reader))
}

func TestFrameGuard_ValidFragmentedHeader(t *testing.T) {
	input := frame(1, 3, []byte("abc"))
	got, err := readGuard(t, &chunkReader{data: input, chunks: []int{1, 2, 1, 4, 3}})
	if err != nil || !bytes.Equal(got, input) {
		t.Fatalf("got=%v err=%v", got, err)
	}
}
func TestFrameGuard_ValidFragmentedPayload(t *testing.T) {
	input := frame(1, 5, []byte("abcde"))
	got, err := readGuard(t, &chunkReader{data: input, chunks: []int{8, 1, 1, 1, 1, 1}})
	if err != nil || !bytes.Equal(got, input) {
		t.Fatalf("got=%v err=%v", got, err)
	}
}
func TestFrameGuard_MultipleFrames(t *testing.T) {
	input := append(frame(1, 1, []byte("a")), frame(2, 1, []byte("b"))...)
	got, err := readGuard(t, bytes.NewReader(input))
	if err != nil || !bytes.Equal(got, input) {
		t.Fatalf("got=%v err=%v", got, err)
	}
}
func TestFrameGuard_StdoutFrameAtLimit(t *testing.T) {
	assertFrameGuardSize(t, 1, MaxControlStdout, nil)
}
func TestFrameGuard_StdoutFrameOneOver(t *testing.T) {
	assertFrameGuardSize(t, 1, MaxControlStdout+1, ErrControlFrameTooLarge)
}
func TestFrameGuard_StderrFrameAtLimit(t *testing.T) {
	assertFrameGuardSize(t, 2, MaxControlStderr, nil)
}
func TestFrameGuard_StderrFrameOneOver(t *testing.T) {
	assertFrameGuardSize(t, 2, MaxControlStderr+1, ErrControlFrameTooLarge)
}
func assertFrameGuardSize(t *testing.T, stream byte, size int, want error) {
	t.Helper()
	payload := bytes.Repeat([]byte("x"), min(size, max(MaxControlStdout, MaxControlStderr)))
	_, err := readGuard(t, bytes.NewReader(frame(stream, uint32(size), payload)))
	if !errors.Is(err, want) {
		t.Fatalf("err=%v want=%v", err, want)
	}
}
func TestFrameGuard_InvalidStreamRejected(t *testing.T) {
	_, err := readGuard(t, bytes.NewReader(frame(9, 0, nil)))
	if !errors.Is(err, ErrInvalidControlFrame) {
		t.Fatalf("err=%v", err)
	}
}
func TestFrameGuard_DeclaredUint32MaxRejectedWithoutPayload(t *testing.T) {
	_, err := readGuard(t, bytes.NewReader(frame(1, ^uint32(0), nil)))
	if !errors.Is(err, ErrControlFrameTooLarge) {
		t.Fatalf("err=%v", err)
	}
}
func TestFrameGuard_NoLargeAllocationOnOversizedHeader(t *testing.T) {
	header := frame(1, ^uint32(0), nil)
	allocs := testing.AllocsPerRun(100, func() {
		var one [1]byte
		_, _ = newControlFrameGuard(bytes.NewReader(header)).Read(one[:])
	})
	if allocs > 4 {
		t.Fatalf("allocs=%f", allocs)
	}
}

func demuxFrames(stdoutFrames, stderrFrames [][]byte) ([]byte, []byte, error) {
	var stream bytes.Buffer
	for _, payload := range stdoutFrames {
		stream.Write(frame(1, uint32(len(payload)), payload))
	}
	for _, payload := range stderrFrames {
		stream.Write(frame(2, uint32(len(payload)), payload))
	}
	stdout := newBoundedWriter(MaxControlStdout, ErrStdoutOverflow)
	stderr := newBoundedWriter(MaxControlStderr, ErrStderrOverflow)
	_, err := stdcopy.StdCopy(stdout, stderr, newControlFrameGuard(&stream))
	return stdout.Bytes(), stderr.Bytes(), err
}
func TestControlStream_ManyValidStdoutFramesCumulativeOverflow(t *testing.T) {
	_, _, err := demuxFrames([][]byte{bytes.Repeat([]byte("a"), MaxControlStdout/2+1), bytes.Repeat([]byte("b"), MaxControlStdout/2+1)}, nil)
	if !errors.Is(err, ErrStdoutOverflow) {
		t.Fatalf("err=%v", err)
	}
}
func TestControlStream_ManyValidStderrFramesCumulativeOverflow(t *testing.T) {
	_, _, err := demuxFrames(nil, [][]byte{bytes.Repeat([]byte("a"), MaxControlStderr/2+1), bytes.Repeat([]byte("b"), MaxControlStderr/2+1)})
	if !errors.Is(err, ErrStderrOverflow) {
		t.Fatalf("err=%v", err)
	}
}
func TestControlStream_AtBothCumulativeLimits(t *testing.T) {
	stdout, stderr, err := demuxFrames([][]byte{bytes.Repeat([]byte("a"), MaxControlStdout/2), bytes.Repeat([]byte("b"), MaxControlStdout-MaxControlStdout/2)}, [][]byte{bytes.Repeat([]byte("c"), MaxControlStderr/2), bytes.Repeat([]byte("d"), MaxControlStderr-MaxControlStderr/2)})
	if err != nil || len(stdout) != MaxControlStdout || len(stderr) != MaxControlStderr {
		t.Fatalf("stdout=%d stderr=%d err=%v", len(stdout), len(stderr), err)
	}
}

type blockingReadAttachment struct {
	reader    *blockingReader
	mu        sync.Mutex
	closeOnce sync.Once
	closeErr  error
	closed    int
}
type blockingReader struct {
	started chan struct{}
	unblock chan struct{}
	once    sync.Once
}

func newBlockingAttachment(closeErr error) *blockingReadAttachment {
	return &blockingReadAttachment{reader: &blockingReader{started: make(chan struct{}), unblock: make(chan struct{})}, closeErr: closeErr}
}
func (r *blockingReader) Read([]byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	<-r.unblock
	return 0, net.ErrClosed
}
func (a *blockingReadAttachment) Reader() io.Reader { return a.reader }
func (a *blockingReadAttachment) Close() error {
	a.closeOnce.Do(func() { a.mu.Lock(); a.closed++; a.mu.Unlock(); close(a.reader.unblock) })
	return a.closeErr
}
func (a *blockingReadAttachment) count() int { a.mu.Lock(); defer a.mu.Unlock(); return a.closed }

func runBlockedProbe(t *testing.T, parent context.Context, timeout time.Duration, trigger func(*blockingReadAttachment)) error {
	t.Helper()
	attachment := newBlockingAttachment(nil)
	fake := successfulFake()
	fake.AttachmentOverride = attachment
	done := make(chan error, 1)
	go func() { done <- probe(fake, parent, timeout) }()
	select {
	case <-attachment.reader.started:
	case <-time.After(time.Second):
		t.Fatal("read did not start")
	}
	trigger(attachment)
	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		t.Fatal("probe did not return")
		return nil
	}
}
func TestControlStream_ParentCancellationDuringRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	err := runBlockedProbe(t, ctx, time.Second, func(*blockingReadAttachment) { cancel() })
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrControlTimeout) {
		t.Fatalf("err=%v", err)
	}
}
func TestControlStream_ParentDeadlineDuringRead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := runBlockedProbe(t, ctx, time.Second, func(*blockingReadAttachment) {})
	if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrControlTimeout) {
		t.Fatalf("err=%v", err)
	}
}
func TestControlStream_AttemptTimeoutDuringRead(t *testing.T) {
	err := runBlockedProbe(t, context.Background(), 20*time.Millisecond, func(*blockingReadAttachment) {})
	if !errors.Is(err, ErrControlTimeout) {
		t.Fatalf("err=%v", err)
	}
}
func TestControlStream_CancellationClosesConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var attachment *blockingReadAttachment
	err := runBlockedProbe(t, ctx, time.Second, func(a *blockingReadAttachment) { attachment = a; cancel() })
	if err == nil || attachment.count() != 1 {
		t.Fatalf("err=%v close_count=%d", err, attachment.count())
	}
}
func TestControlStream_CancellationNoWatcherLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	_ = runBlockedProbe(t, ctx, time.Second, func(*blockingReadAttachment) { cancel() })
	runtime.Gosched()
	if after := runtime.NumGoroutine(); after > before+2 {
		t.Fatalf("goroutines before=%d after=%d", before, after)
	}
}
func TestControlStream_CopyErrorPlusContext(t *testing.T) {
	attachment := newBlockingAttachment(nil)
	fake := successfulFake()
	fake.AttachmentOverride = attachment
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- probe(fake, ctx, time.Second) }()
	<-attachment.reader.started
	cancel()
	err := <-done
	if !errors.Is(err, context.Canceled) || !errors.Is(err, net.ErrClosed) {
		t.Fatalf("err=%v", err)
	}
}
func TestControlStream_CloseErrorPlusContext(t *testing.T) {
	closeErr := errors.New("close failed")
	attachment := newBlockingAttachment(closeErr)
	fake := successfulFake()
	fake.AttachmentOverride = attachment
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- probe(fake, ctx, time.Second) }()
	<-attachment.reader.started
	cancel()
	err := <-done
	if !errors.Is(err, context.Canceled) || !errors.Is(err, closeErr) {
		t.Fatalf("err=%v", err)
	}
}

func TestControlInspect_RunningTrueRejected(t *testing.T) {
	fake := successfulFake()
	fake.NextInspectResult = ExecInspectResult{ExitCode: 0, Running: true}
	if err := probe(fake, context.Background(), time.Second); !errors.Is(err, ErrExecStillRunning) {
		t.Fatalf("err=%v", err)
	}
}
func TestControlInspect_RunningFalseAccepted(t *testing.T) {
	if err := probe(successfulFake(), context.Background(), time.Second); err != nil {
		t.Fatal(err)
	}
}
func TestControlInspect_RunningTruePreservesExitAndStderr(t *testing.T) {
	fake := successfulFake()
	fake.NextAttachStderr = []byte("diagnostic")
	fake.NextInspectResult = ExecInspectResult{ExitCode: 17, Running: true}
	_, _, err := NewControlRunner(fake).ControlProbe(context.Background(), "container-identity", canarycontrol.OpHealth, 8080, 0, time.Second)
	var failure *ControlFailureError
	if !errors.As(err, &failure) || failure.ExitCode != 17 || failure.Stderr != "diagnostic" {
		t.Fatalf("failure=%+v err=%v", failure, err)
	}
}

func TestControlStream_CancelDuringRead_Count100(t *testing.T) {
	for range 100 {
		ctx, cancel := context.WithCancel(context.Background())
		err := runBlockedProbe(t, ctx, time.Second, func(*blockingReadAttachment) { cancel() })
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
}
func TestControlStream_AttemptTimeoutDuringRead_Count100(t *testing.T) {
	for range 100 {
		if err := runBlockedProbe(t, context.Background(), time.Millisecond, func(*blockingReadAttachment) {}); !errors.Is(err, ErrControlTimeout) {
			t.Fatal(err)
		}
	}
}
func TestControlStream_NoGoroutineGrowth(t *testing.T) {
	before := runtime.NumGoroutine()
	TestControlStream_CancelDuringRead_Count100(t)
	runtime.GC()
	runtime.Gosched()
	if after := runtime.NumGoroutine(); after > before+3 {
		t.Fatalf("before=%d after=%d", before, after)
	}
}
func TestControlStream_NoConnectionLeak(t *testing.T) {
	for range 100 {
		ctx, cancel := context.WithCancel(context.Background())
		var a *blockingReadAttachment
		_ = runBlockedProbe(t, ctx, time.Second, func(x *blockingReadAttachment) { a = x; cancel() })
		if a.count() != 1 {
			t.Fatalf("close_count=%d", a.count())
		}
	}
}
