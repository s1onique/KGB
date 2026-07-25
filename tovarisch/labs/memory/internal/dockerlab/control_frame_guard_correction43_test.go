// control_frame_guard_correction43_test.go — Tests for the strict Docker
// multiplex framing rules introduced by CORRECTION43.
//
// These tests cover:
//   - explicit partial header / partial payload errors
//   - reserved byte validation
//   - narrowed stream identifier acceptance (stdout=1, stderr=2, system-error=3)
//   - frame-size and cumulative-bound regressions
//   - reader-contract fixtures (n>0+EOF, custom errors, fragmented reads)
//   - end-to-end fail-closed behavior through stdcopy + bounded writers
//   - property/coverage suite with bounded seed corpus
package dockerlab

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/canarycontrol"
)

// ========== P0-1: explicit framing errors are stable and discoverable ==========

func TestFrameGuard_StableErrorsDiscoverable(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"ErrIncompleteControlFrameHeader", ErrIncompleteControlFrameHeader},
		{"ErrIncompleteControlFramePayload", ErrIncompleteControlFramePayload},
		{"ErrNoncanonicalControlFrameHeader", ErrNoncanonicalControlFrameHeader},
		{"ErrUnexpectedControlStream", ErrUnexpectedControlStream},
	}
	for _, c := range cases {
		if c.err == nil {
			t.Fatalf("%s must not be nil", c.name)
		}
		if c.err.Error() == "" {
			t.Fatalf("%s must have a non-empty message", c.name)
		}
	}
}

// TestFrameGuard_PartialHeaderNotCollapsedToEOF ensures that a partial
// header is NEVER returned as plain io.EOF.
func TestFrameGuard_PartialHeaderNotCollapsedToEOF(t *testing.T) {
	for n := 1; n < dockerFrameHeaderSize; n++ {
		partial := make([]byte, n)
		_, err := readGuard(t, bytes.NewReader(partial))
		if err == nil {
			t.Fatalf("partial header of %d bytes must produce an error", n)
		}
		if errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("partial header %d returned plain io.EOF: %v", n, err)
		}
	}
}

// ========== P0-2: reject partial headers ==========

func TestFrameGuard_EmptyStreamGracefulEOF(t *testing.T) {
	// io.ReadAll converts io.EOF → nil. Verify the empty stream is read
	// with no error and no bytes.
	got, err := readGuard(t, bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got=%v want empty", got)
	}
}

func TestFrameGuard_PartialFirstHeaderEachLength(t *testing.T) {
	for n := 1; n < dockerFrameHeaderSize; n++ {
		partial := make([]byte, n)
		_, err := readGuard(t, bytes.NewReader(partial))
		if !errors.Is(err, ErrIncompleteControlFrameHeader) {
			t.Fatalf("len=%d err=%v want ErrIncompleteControlFrameHeader", n, err)
		}
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("len=%d err=%v want io.ErrUnexpectedEOF", n, err)
		}
	}
}

func TestFrameGuard_ValidFrameThenPartialHeaderEachLength(t *testing.T) {
	health := []byte(`{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}}`)
	complete := frame(1, uint32(len(health)), health)
	for n := 1; n < dockerFrameHeaderSize; n++ {
		partial := make([]byte, n)
		input := append(append([]byte{}, complete...), partial...)
		got, err := readGuard(t, bytes.NewReader(input))
		// The complete frame must be returned before the error.
		if !bytes.Equal(got, complete) {
			t.Fatalf("len=%d got=%v want=%v", n, got, complete)
		}
		if !errors.Is(err, ErrIncompleteControlFrameHeader) {
			t.Fatalf("len=%d err=%v want ErrIncompleteControlFrameHeader", n, err)
		}
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("len=%d err=%v want io.ErrUnexpectedEOF", n, err)
		}
	}
}

// bytesAndEOFReader returns (n, io.EOF) on the first call and (0, io.EOF) thereafter.
type bytesAndEOFReader struct {
	data []byte
	done bool
}

func (r *bytesAndEOFReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	n := copy(p, r.data)
	return n, io.EOF
}

func TestFrameGuard_HeaderReaderReturnsBytesAndEOF(t *testing.T) {
	// All returned bytes must be processed before the error.
	// Structured truncation must be reported as ErrIncompleteControlFrameHeader.
	for n := 1; n < dockerFrameHeaderSize; n++ {
		reader := &bytesAndEOFReader{data: make([]byte, n)}
		_, err := readGuard(t, reader)
		if !errors.Is(err, ErrIncompleteControlFrameHeader) {
			t.Fatalf("len=%d err=%v", n, err)
		}
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("len=%d err=%v", n, err)
		}
	}
}

// fragmentedHeaderReader returns one byte at a time, then EOF.
type fragmentedHeaderReader struct {
	pending []byte
}

func (r *fragmentedHeaderReader) Read(p []byte) (int, error) {
	if len(r.pending) == 0 {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = r.pending[0]
	r.pending = r.pending[1:]
	return 1, nil
}

func TestFrameGuard_FragmentedHeaderThenUnexpectedEOF(t *testing.T) {
	for n := 1; n < dockerFrameHeaderSize; n++ {
		pending := make([]byte, n)
		_, err := readGuard(t, &fragmentedHeaderReader{pending: pending})
		if !errors.Is(err, ErrIncompleteControlFrameHeader) {
			t.Fatalf("n=%d err=%v", n, err)
		}
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("n=%d err=%v", n, err)
		}
	}
}

// ========== P0-3: reject partial payloads ==========

func TestFrameGuard_PartialFirstPayload(t *testing.T) {
	// Stream=1, declared size=10, but only 4 bytes of payload delivered.
	header := frame(1, 10, nil)[:dockerFrameHeaderSize]
	truncated := append(header, []byte("abcd")...)
	_, err := readGuard(t, bytes.NewReader(truncated))
	if !errors.Is(err, ErrIncompleteControlFramePayload) {
		t.Fatalf("err=%v", err)
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err=%v want io.ErrUnexpectedEOF", err)
	}
}

func TestFrameGuard_ValidFrameThenPartialPayload(t *testing.T) {
	// Complete stdout frame, then a partial header + partial payload.
	// The complete frame must be returned first; the partial payload is
	// detected with ErrIncompleteControlFramePayload.
	health := []byte(`{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}}`)
	complete := frame(1, uint32(len(health)), health)
	header2 := frame(1, 10, nil)[:dockerFrameHeaderSize]
	truncated := append(append([]byte{}, complete...), header2...)
	truncated = append(truncated, []byte("abcd")...)
	got, err := readGuard(t, bytes.NewReader(truncated))
	if !bytes.HasPrefix(got, complete) {
		t.Fatalf("complete frame must be a prefix of returned bytes: got=%v", got)
	}
	if !errors.Is(err, ErrIncompleteControlFramePayload) {
		t.Fatalf("err=%v want ErrIncompleteControlFramePayload", err)
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err=%v want io.ErrUnexpectedEOF", err)
	}
}

// payloadEOFReader returns N bytes of a payload plus EOF in a single call.
type payloadEOFReader struct {
	bytesLeft int
}

func (r *payloadEOFReader) Read(p []byte) (int, error) {
	if r.bytesLeft == 0 {
		return 0, io.EOF
	}
	want := len(p)
	if want > r.bytesLeft {
		want = r.bytesLeft
	}
	for i := 0; i < want; i++ {
		p[i] = 'x'
	}
	r.bytesLeft -= want
	return want, nil
}

func TestFrameGuard_PayloadReaderReturnsBytesAndEOF(t *testing.T) {
	// Stream=1, declared size=10, only 4 bytes of payload delivered before EOF.
	header := frame(1, 10, nil)[:dockerFrameHeaderSize]
	source := &payloadAndHeaderReader{header: header, payload: []byte("abcd")}
	_, err := readGuard(t, source)
	if !errors.Is(err, ErrIncompleteControlFramePayload) {
		t.Fatalf("err=%v", err)
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err=%v want io.ErrUnexpectedEOF", err)
	}
}

// payloadAndHeaderReader concatenates a fixed header followed by a payload
// length-limited source.
type payloadAndHeaderReader struct {
	header  []byte
	headerN int
	payload []byte
}

func (r *payloadAndHeaderReader) Read(p []byte) (int, error) {
	if r.headerN < len(r.header) {
		n := copy(p, r.header[r.headerN:])
		r.headerN += n
		return n, nil
	}
	if len(r.payload) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.payload)
	r.payload = r.payload[n:]
	return n, nil
}

func TestFrameGuard_PayloadOneByteShort(t *testing.T) {
	// Stream=1, declared size=10, payload has 9 bytes.
	header := frame(1, 10, nil)[:dockerFrameHeaderSize]
	input := append(header, bytes.Repeat([]byte("x"), 9)...)
	_, err := readGuard(t, bytes.NewReader(input))
	if !errors.Is(err, ErrIncompleteControlFramePayload) {
		t.Fatalf("err=%v", err)
	}
}

func TestFrameGuard_ZeroLengthPayloadAccepted(t *testing.T) {
	// Stream=1, declared size=0. Payload empty is accepted.
	for _, stream := range []byte{1, 2, 3} {
		header := frame(stream, 0, nil)[:dockerFrameHeaderSize]
		got, err := readGuard(t, bytes.NewReader(header))
		if err != nil {
			t.Fatalf("stream=%d err=%v", stream, err)
		}
		if !bytes.Equal(got, header) {
			t.Fatalf("stream=%d got=%v", stream, got)
		}
	}
}

// TestFrameGuard_PartialPayloadForEachStream ensures structured truncation
// is reported for stdout, stderr, and system-error frames.
func TestFrameGuard_PartialPayloadForEachStream(t *testing.T) {
	for _, stream := range []byte{1, 2, 3} {
		header := frame(stream, 10, nil)[:dockerFrameHeaderSize]
		truncated := append(header, bytes.Repeat([]byte("x"), 5)...)
		_, err := readGuard(t, bytes.NewReader(truncated))
		if !errors.Is(err, ErrIncompleteControlFramePayload) {
			t.Fatalf("stream=%d err=%v", stream, err)
		}
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("stream=%d err=%v", stream, err)
		}
	}
}

// ========== P0-5: reserved header bytes ==========

func TestFrameGuard_ReservedByte1Nonzero(t *testing.T) {
	header := []byte{1, 1, 0, 0, 0, 0, 0, 0}
	_, err := readGuard(t, bytes.NewReader(header))
	if !errors.Is(err, ErrNoncanonicalControlFrameHeader) {
		t.Fatalf("err=%v", err)
	}
}

func TestFrameGuard_ReservedByte2Nonzero(t *testing.T) {
	header := []byte{1, 0, 1, 0, 0, 0, 0, 0}
	_, err := readGuard(t, bytes.NewReader(header))
	if !errors.Is(err, ErrNoncanonicalControlFrameHeader) {
		t.Fatalf("err=%v", err)
	}
}

func TestFrameGuard_ReservedByte3Nonzero(t *testing.T) {
	header := []byte{1, 0, 0, 1, 0, 0, 0, 0}
	_, err := readGuard(t, bytes.NewReader(header))
	if !errors.Is(err, ErrNoncanonicalControlFrameHeader) {
		t.Fatalf("err=%v", err)
	}
}

func TestFrameGuard_AllReservedBytesZero(t *testing.T) {
	// Stream=1, reserved=0, declared=0. Must be accepted.
	header := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	_, err := readGuard(t, bytes.NewReader(header))
	if err != nil {
		t.Fatalf("err=%v", err)
	}
}

// ========== P0-6: stream identifier policy ==========

func TestFrameGuard_StdoutAccepted(t *testing.T) {
	header := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	_, err := readGuard(t, bytes.NewReader(header))
	if err != nil {
		t.Fatalf("err=%v", err)
	}
}

func TestFrameGuard_StderrAccepted(t *testing.T) {
	header := []byte{2, 0, 0, 0, 0, 0, 0, 0}
	_, err := readGuard(t, bytes.NewReader(header))
	if err != nil {
		t.Fatalf("err=%v", err)
	}
}

func TestFrameGuard_SystemErrorAccepted(t *testing.T) {
	header := []byte{3, 0, 0, 0, 0, 0, 0, 0}
	_, err := readGuard(t, bytes.NewReader(header))
	if err != nil {
		t.Fatalf("err=%v", err)
	}
}

func TestFrameGuard_StdinRejected(t *testing.T) {
	// Stream=0 (stdin) is never produced by daemon-to-controller control output.
	header := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	_, err := readGuard(t, bytes.NewReader(header))
	if !errors.Is(err, ErrUnexpectedControlStream) {
		t.Fatalf("err=%v want ErrUnexpectedControlStream", err)
	}
}

func TestFrameGuard_UnknownStreamsRejected(t *testing.T) {
	for _, stream := range []byte{4, 5, 9, 100, 255} {
		header := []byte{stream, 0, 0, 0, 0, 0, 0, 0}
		_, err := readGuard(t, bytes.NewReader(header))
		if !errors.Is(err, ErrInvalidControlFrame) {
			t.Fatalf("stream=%d err=%v want ErrInvalidControlFrame", stream, err)
		}
	}
}

// ========== P0-7: frame-size and cumulative bounds regressions ==========

func TestFrameGuard_StdoutAtLimitRegression(t *testing.T) {
	assertFrameGuardSize(t, 1, MaxControlStdout, nil)
}
func TestFrameGuard_StdoutOneOverRegression(t *testing.T) {
	assertFrameGuardSize(t, 1, MaxControlStdout+1, ErrControlFrameTooLarge)
}
func TestFrameGuard_StderrAtLimitRegression(t *testing.T) {
	assertFrameGuardSize(t, 2, MaxControlStderr, nil)
}
func TestFrameGuard_StderrOneOverRegression(t *testing.T) {
	assertFrameGuardSize(t, 2, MaxControlStderr+1, ErrControlFrameTooLarge)
}
func TestFrameGuard_DeclaredUint32MaxRegression(t *testing.T) {
	_, err := readGuard(t, bytes.NewReader(frame(1, ^uint32(0), nil)))
	if !errors.Is(err, ErrControlFrameTooLarge) {
		t.Fatalf("err=%v", err)
	}
}
func TestFrameGuard_ManyStdoutCumulativeOverflowRegression(t *testing.T) {
	_, _, err := demuxFrames([][]byte{
		bytes.Repeat([]byte("a"), MaxControlStdout/2+1),
		bytes.Repeat([]byte("b"), MaxControlStdout/2+1),
	}, nil)
	if !errors.Is(err, ErrStdoutOverflow) {
		t.Fatalf("err=%v", err)
	}
}
func TestFrameGuard_ManyStderrCumulativeOverflowRegression(t *testing.T) {
	_, _, err := demuxFrames(nil, [][]byte{
		bytes.Repeat([]byte("a"), MaxControlStderr/2+1),
		bytes.Repeat([]byte("b"), MaxControlStderr/2+1),
	})
	if !errors.Is(err, ErrStderrOverflow) {
		t.Fatalf("err=%v", err)
	}
}
func TestFrameGuard_BothCumulativeLimitsRegression(t *testing.T) {
	stdout, stderr, err := demuxFrames(
		[][]byte{
			bytes.Repeat([]byte("a"), MaxControlStdout/2),
			bytes.Repeat([]byte("b"), MaxControlStdout-MaxControlStdout/2),
		},
		[][]byte{
			bytes.Repeat([]byte("c"), MaxControlStderr/2),
			bytes.Repeat([]byte("d"), MaxControlStderr-MaxControlStderr/2),
		},
	)
	if err != nil || len(stdout) != MaxControlStdout || len(stderr) != MaxControlStderr {
		t.Fatalf("stdout=%d stderr=%d err=%v", len(stdout), len(stderr), err)
	}
}

// Mixed corruption cases (P0-7 additions).

func TestFrameGuard_ValidNearLimitThenPartialFrame(t *testing.T) {
	// A near-limit valid frame followed by a partial frame must fail closed.
	big := bytes.Repeat([]byte("a"), MaxControlStdout-1)
	complete := frame(1, uint32(len(big)), big)
	partial := make([]byte, 3)
	input := append(append([]byte{}, complete...), partial...)
	got, err := readGuard(t, bytes.NewReader(input))
	if !bytes.Equal(got, complete) {
		t.Fatalf("near-limit frame not returned first: got=%v", got)
	}
	if !errors.Is(err, ErrIncompleteControlFrameHeader) {
		t.Fatalf("err=%v", err)
	}
}

func TestFrameGuard_ValidCumulativeThenMalformedHeader(t *testing.T) {
	// A valid frame then a malformed header (nonzero reserved byte).
	complete := frame(1, 0, nil)
	malformed := []byte{1, 1, 0, 0, 0, 0, 0, 0}
	input := append(complete, malformed...)
	got, err := readGuard(t, bytes.NewReader(input))
	if !bytes.Equal(got, complete) {
		t.Fatalf("valid frame not returned first: got=%v", got)
	}
	if !errors.Is(err, ErrNoncanonicalControlFrameHeader) {
		t.Fatalf("err=%v", err)
	}
}

func TestFrameGuard_OversizedFrameAfterValidEnvelope(t *testing.T) {
	// A valid envelope then an oversized frame must fail closed.
	health := []byte(`{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}}`)
	complete := frame(1, uint32(len(health)), health)
	oversized := frame(1, MaxControlStdout+1, nil)
	input := append(complete, oversized...)
	got, err := readGuard(t, bytes.NewReader(input))
	if !bytes.Equal(got, complete) {
		t.Fatalf("valid envelope not returned first: got=%v", got)
	}
	if !errors.Is(err, ErrControlFrameTooLarge) {
		t.Fatalf("err=%v", err)
	}
}

// ========== P0-8: reader-contract fixtures ==========

// "n > 0, err = io.EOF" — single-call partial + EOF.
func TestFrameGuard_ReaderContract_NPositiveEOF(t *testing.T) {
	// 3 bytes of header + EOF.
	for _, n := range []int{1, 2, 3, 5, 7} {
		header := make([]byte, n)
		_, err := readGuard(t, &bytesAndEOFReader{data: header})
		if !errors.Is(err, ErrIncompleteControlFrameHeader) {
			t.Fatalf("n=%d err=%v", n, err)
		}
	}
}

// "n > 0, err = custom error" — custom source errors remain discoverable.
type customErrorReader struct {
	done bool
}

var errCustomSource = errors.New("source: custom error")

func (r *customErrorReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	// 4 bytes of header + custom error.
	if len(p) < 4 {
		return 0, errCustomSource
	}
	copy(p[:4], []byte{1, 0, 0, 0})
	return 4, errCustomSource
}

func TestFrameGuard_ReaderContract_CustomErrorDiscoverable(t *testing.T) {
	_, err := readGuard(t, &customErrorReader{})
	if !errors.Is(err, errCustomSource) {
		t.Fatalf("err=%v want custom error", err)
	}
}

// "n = 0, err = nil once" — a single zero-byte read must not cause infinite loop.
type zeroByteNilErrReader struct {
	zeroReadDone bool
	bytesRead    int
	tail         []byte
}

func (r *zeroByteNilErrReader) Read(p []byte) (int, error) {
	if !r.zeroReadDone {
		r.zeroReadDone = true
		return 0, nil
	}
	if len(r.tail) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.tail)
	r.tail = r.tail[n:]
	r.bytesRead += n
	return n, nil
}

func TestFrameGuard_ReaderContract_ZeroByteNilOnce(t *testing.T) {
	// Source emits (0, nil) once, then the header bytes, then EOF.
	// The guard must not loop infinitely on a (0, nil) read.
	header := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	source := &zeroByteNilErrReader{tail: header}
	guard := newControlFrameGuard(source)
	buf := make([]byte, 64)
	totalRead := 0
	for i := 0; i < 50; i++ {
		n, err := guard.Read(buf)
		totalRead += n
		if err != nil {
			break
		}
	}
	if totalRead > len(header) {
		t.Fatalf("totalRead=%d exceeds header size=%d", totalRead, len(header))
	}
	// Bytes that were read must be the prefix of the header.
	if totalRead > 0 && !bytes.Equal(buf[:totalRead], header[:totalRead]) {
		t.Fatalf("read bytes %v != header prefix %v", buf[:totalRead], header[:totalRead])
	}
}

// "fragmented one-byte reads" — a one-byte-at-a-time reader.
type oneByteReader struct {
	data []byte
}

func (r *oneByteReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = r.data[0]
	r.data = r.data[1:]
	return 1, nil
}

func TestFrameGuard_ReaderContract_OneByteReads(t *testing.T) {
	// Fragmented header + payload in single-byte reads.
	health := []byte(`{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}}`)
	complete := frame(1, uint32(len(health)), health)
	got, err := readGuard(t, &oneByteReader{data: complete})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !bytes.Equal(got, complete) {
		t.Fatalf("got=%v", got)
	}
}

// "fragmented header and payload in the same Read" — partial header + partial payload
// delivered in a single Read call. The contract requires the partial header to be
// reported and the partial payload bytes to be processed.
func TestFrameGuard_ReaderContract_FragmentedHeaderAndPayloadSameRead(t *testing.T) {
	// 4 bytes of header + 2 bytes that would be payload + EOF, all in one Read.
	// Since the header is partial (4 bytes < 8), the error is partial header.
	truncated := []byte{1, 0, 0, 0, 'x', 'y'}
	_, err := readGuard(t, &bytesAndEOFReader{data: truncated})
	if !errors.Is(err, ErrIncompleteControlFrameHeader) {
		t.Fatalf("err=%v", err)
	}
}

// ========== P0-9: deterministic property suite (bounded seed corpus) ==========

func TestFrameGuard_PropertySuite(t *testing.T) {
	health := []byte(`{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}}`)
	seeds := []struct {
		name        string
		data        []byte
		wantAccept  bool
		wantErr     error
		wantUnknown []error
	}{
		{
			name:       "valid_complete_frame_health_envelope",
			data:       frame(1, uint32(len(health)), health),
			wantAccept: true,
		},
		{
			name:       "valid_complete_stdout_then_stderr",
			data:       append(frame(1, 3, []byte("abc")), frame(2, 3, []byte("def"))...),
			wantAccept: true,
		},
		{
			name:       "partial_header_4bytes",
			data:       []byte{1, 0, 0, 0},
			wantAccept: false,
			wantErr:    ErrIncompleteControlFrameHeader,
		},
		{
			name:       "partial_header_7bytes",
			data:       []byte{1, 0, 0, 0, 0, 0, 0},
			wantAccept: false,
			wantErr:    ErrIncompleteControlFrameHeader,
		},
		{
			name:       "partial_payload_5of10",
			data:       append(frame(1, 10, nil)[:dockerFrameHeaderSize], []byte("abcde")...),
			wantAccept: false,
			wantErr:    ErrIncompleteControlFramePayload,
		},
		{
			name:       "declared_size_uint32_max",
			data:       frame(1, ^uint32(0), nil),
			wantAccept: false,
			wantErr:    ErrControlFrameTooLarge,
		},
		{
			name:       "noncanonical_reserved_byte_1",
			data:       []byte{1, 1, 0, 0, 0, 0, 0, 0},
			wantAccept: false,
			wantErr:    ErrNoncanonicalControlFrameHeader,
		},
		{
			name:       "noncanonical_reserved_byte_2",
			data:       []byte{1, 0, 1, 0, 0, 0, 0, 0},
			wantAccept: false,
			wantErr:    ErrNoncanonicalControlFrameHeader,
		},
		{
			name:       "noncanonical_reserved_byte_3",
			data:       []byte{1, 0, 0, 1, 0, 0, 0, 0},
			wantAccept: false,
			wantErr:    ErrNoncanonicalControlFrameHeader,
		},
		{
			name:       "stdin_stream_rejected",
			data:       []byte{0, 0, 0, 0, 0, 0, 0, 0},
			wantAccept: false,
			wantErr:    ErrUnexpectedControlStream,
		},
		{
			name:       "unknown_stream_rejected",
			data:       []byte{9, 0, 0, 0, 0, 0, 0, 0},
			wantAccept: false,
			wantErr:    ErrInvalidControlFrame,
		},
	}
	for _, seed := range seeds {
		seed := seed
		t.Run(seed.name, func(t *testing.T) {
			got, err := readGuard(t, bytes.NewReader(seed.data))
			if seed.wantAccept {
				if err != nil {
					t.Fatalf("expected accepted, got err=%v", err)
				}
				if !bytes.Equal(got, seed.data) {
					t.Fatalf("output bytes != input bytes")
				}
			} else {
				if err == nil {
					t.Fatalf("expected rejected, got no error")
				}
				if seed.wantErr != nil && !errors.Is(err, seed.wantErr) {
					t.Fatalf("err=%v want %v", err, seed.wantErr)
				}
				// All structured truncation errors must be paired with io.ErrUnexpectedEOF.
				if seed.wantErr == ErrIncompleteControlFrameHeader || seed.wantErr == ErrIncompleteControlFramePayload {
					if !errors.Is(err, io.ErrUnexpectedEOF) {
						t.Fatalf("err=%v want io.ErrUnexpectedEOF", err)
					}
				}
			}
		})
	}
}

// TestFrameGuard_PropertyRetainedStateBounded ensures the guard never
// retains payload-sized buffers, regardless of input.
func TestFrameGuard_PropertyRetainedStateBounded(t *testing.T) {
	// 100 KiB of valid frames must not cause the guard to retain large state.
	big := bytes.Repeat([]byte("a"), 100*1024)
	input := frame(1, uint32(len(big)), big)
	allocs := testing.AllocsPerRun(100, func() {
		var one [1]byte
		_, _ = newControlFrameGuard(bytes.NewReader(input)).Read(one[:])
	})
	if allocs > 4 {
		t.Fatalf("allocs=%f (guard must remain bounded)", allocs)
	}
}

// ========== P0-4: end-to-end fail-closed behavior ==========

// successProbe returns whether the probe succeeded through the entire
// controlFrameGuard -> stdcopy.StdCopy -> bounded writers -> DecodeEnvelopeExactlyOne
// pipeline. It is used to prove that no incomplete trailing data can be silently
// accepted as a valid envelope.
func successProbe(t *testing.T, fake *FakeControlExecRuntime) (int, *canarycontrol.ControlEnvelope, error) {
	t.Helper()
	return NewControlRunner(fake).ControlProbe(context.Background(), "container-identity", canarycontrol.OpHealth, 8080, 0, 5*time.Second)
}

func TestControlProbe_ValidEnvelopeThenPartialHeaderRejected(t *testing.T) {
	health := []byte(`{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}}`)
	complete := frame(1, uint32(len(health)), health)
	partial := make([]byte, 3)
	input := append(append([]byte{}, complete...), partial...)
	fake := successfulFake()
	fake.Stream = bytes.NewReader(input)
	_, _, err := successProbe(t, fake)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrIncompleteControlFrameHeader) {
		t.Fatalf("err=%v want ErrIncompleteControlFrameHeader", err)
	}
}

func TestControlProbe_ValidEnvelopeThenPartialPayloadRejected(t *testing.T) {
	health := []byte(`{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}}`)
	complete := frame(1, uint32(len(health)), health)
	header2 := frame(1, 10, nil)[:dockerFrameHeaderSize]
	truncated := append(append([]byte{}, complete...), header2...)
	truncated = append(truncated, bytes.Repeat([]byte("x"), 5)...)
	fake := successfulFake()
	fake.Stream = bytes.NewReader(truncated)
	_, _, err := successProbe(t, fake)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrIncompleteControlFramePayload) {
		t.Fatalf("err=%v want ErrIncompleteControlFramePayload", err)
	}
}

func TestControlProbe_PartialOnlyHeaderRejected(t *testing.T) {
	partial := make([]byte, 4)
	fake := successfulFake()
	fake.Stream = bytes.NewReader(partial)
	_, _, err := successProbe(t, fake)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrIncompleteControlFrameHeader) {
		t.Fatalf("err=%v want ErrIncompleteControlFrameHeader", err)
	}
}

func TestControlProbe_PartialOnlyPayloadRejected(t *testing.T) {
	header := frame(1, 10, nil)[:dockerFrameHeaderSize]
	truncated := append(header, bytes.Repeat([]byte("x"), 5)...)
	fake := successfulFake()
	fake.Stream = bytes.NewReader(truncated)
	_, _, err := successProbe(t, fake)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrIncompleteControlFramePayload) {
		t.Fatalf("err=%v want ErrIncompleteControlFramePayload", err)
	}
}

// TestControlProbe_IncompleteTrailingFrameNeverAcceptsFirstEnvelope
// proves that an incomplete trailing frame can never cause the first
// envelope to be (incorrectly) accepted.
func TestControlProbe_IncompleteTrailingFrameNeverAcceptsFirstEnvelope(t *testing.T) {
	health := []byte(`{"schema_version":"canary-control/v1","operation":"health","success":true,"http_status":200,"health":{"ready":true,"mode":"growing"}}`)
	complete := frame(1, uint32(len(health)), health)
	// Append a trailing invalid frame (1 byte header).
	trailing := []byte{1}
	input := append(append([]byte{}, complete...), trailing...)
	fake := successfulFake()
	fake.Stream = bytes.NewReader(input)
	exitCode, env, err := successProbe(t, fake)
	if err == nil {
		t.Fatalf("trailing incomplete frame must not be accepted")
	}
	if env != nil {
		t.Fatalf("env must not be populated on truncation: env=%+v", env)
	}
	if exitCode == 0 {
		t.Fatalf("exitCode must not be 0 on truncation")
	}
	if !errors.Is(err, ErrIncompleteControlFrameHeader) {
		t.Fatalf("err=%v", err)
	}
}

// TestControlProbe_NoncanonicalReservedBytesRejected proves that reserved
// bytes are rejected before envelope verification.
func TestControlProbe_NoncanonicalReservedBytesRejected(t *testing.T) {
	// header: stream=1, reserved byte 1 is nonzero, declared=0. The reserved
	// byte violation must be reported before envelope decoding.
	headerWithReserved := []byte{1, 1, 0, 0, 0, 0, 0, 0}
	fake := successfulFake()
	fake.Stream = bytes.NewReader(headerWithReserved)
	_, _, err := successProbe(t, fake)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrNoncanonicalControlFrameHeader) {
		t.Fatalf("err=%v", err)
	}
}

// ========== Helpers used by the new tests ==========

// multiplexedStreamWithExtra builds a buffer with a complete frame followed by
// extra bytes. Used by some end-to-end tests.
func multiplexedStreamWithExtra(stdout []byte, extra []byte) io.Reader {
	var out bytes.Buffer
	header := make([]byte, 8)
	header[0] = 1
	binary.BigEndian.PutUint32(header[4:], uint32(len(stdout)))
	out.Write(header)
	out.Write(stdout)
	out.Write(extra)
	return bytes.NewReader(out.Bytes())
}

// _ verifies stdcopy is reachable through the test helpers.
var _ = stdcopy.StdCopy
var _ = fmt.Sprintf
