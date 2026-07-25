package dockerlab

import (
	"encoding/binary"
	"errors"
	"io"
)

// Strict Docker multiplex framing errors. Canonical errors are stable and
// discoverable with errors.Is. They are never collapsed into plain io.EOF
// and are always paired with io.ErrUnexpectedEOF for structured truncation.
var (
	// ErrIncompleteControlFrameHeader is returned when a frame header
	// is partially read (1-7 bytes) before EOF or truncation.
	ErrIncompleteControlFrameHeader = errors.New("dockerlab: incomplete control frame header")

	// ErrIncompleteControlFramePayload is returned when a declared
	// payload is not fully delivered before EOF or truncation.
	ErrIncompleteControlFramePayload = errors.New("dockerlab: incomplete control frame payload")

	// ErrNoncanonicalControlFrameHeader is returned when a frame header
	// has nonzero reserved bytes (1, 2, or 3).
	ErrNoncanonicalControlFrameHeader = errors.New("dockerlab: noncanonical control frame header")

	// ErrUnexpectedControlStream is returned when the frame stream
	// identifier is one the daemon-to-controller control output does
	// not accept (stdin / unknown).
	ErrUnexpectedControlStream = errors.New("dockerlab: unexpected control stream")
)

var (
	ErrControlFrameTooLarge = errors.New("dockerlab: control frame too large")
	ErrInvalidControlFrame  = errors.New("dockerlab: invalid control frame")
)

const dockerFrameHeaderSize = 8

// controlFrameGuard validates each Docker multiplex frame header before
// stdcopy sees it. It retains only one 8-byte header and never allocates a
// payload-sized buffer.
//
// Strict framing rules (CORRECTION43):
//
//  1. Reserved bytes 1, 2, 3 must be zero. Otherwise
//     ErrNoncanonicalControlFrameHeader is returned.
//  2. Stream identifier must be 1 (stdout), 2 (stderr) or 3 (system-error).
//     Stream 0 (stdin) is rejected with ErrUnexpectedControlStream.
//     Values 4-255 are rejected with ErrInvalidControlFrame.
//  3. Declared payload size must be within the per-stream limit.
//     Otherwise ErrControlFrameTooLarge is returned.
//  4. A graceful EOF is allowed only when the current header has been
//     fully read and the declared payload has been fully delivered.
//  5. Any other EOF (after 1-7 header bytes, or with payloadRemaining > 0)
//     is reported as ErrIncompleteControlFrameHeader or
//     ErrIncompleteControlFramePayload joined with io.ErrUnexpectedEOF.
//
// State machine (per outer-loop iteration):
//
//	A: headerRead < 8, payloadRemaining == 0, headerOut == 0
//	   Read header bytes until full or EOF. On full: go to B.
//	   On partial header: ErrIncompleteControlFrameHeader.
//	B: headerRead == 8, headerValid == false
//	   Validate header. On invalid: terminal error. On valid: go to C.
//	C: headerValid == true, headerOut < 8
//	   Drain header bytes to caller. When fully drained: go to D.
//	D: headerOut == 8, payloadRemaining == 0
//	   Read payload bytes. When fully consumed: go to E.
//	E: headerOut == 8, payloadRemaining == 0
//	   Reset header cursor and re-enter A.
type controlFrameGuard struct {
	source           io.Reader
	header           [dockerFrameHeaderSize]byte
	headerRead       int
	headerOut        int
	payloadRemaining uint32
	headerValid      bool
	terminalErr      error
}

func newControlFrameGuard(source io.Reader) io.Reader {
	return &controlFrameGuard{source: source}
}

func (g *controlFrameGuard) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if g.terminalErr != nil {
		return 0, g.terminalErr
	}

	written := 0
	for written < len(p) {
		// Stage B: validate a fully buffered header before any of its
		// bytes are returned to the caller. This guarantees that a
		// noncanonical header is never silently flushed.
		if g.headerRead == dockerFrameHeaderSize && !g.headerValid {
			if err := g.validateHeader(); err != nil {
				g.terminalErr = err
				return written, err
			}
			// Validation passed; arm payload. The header bytes are
			// still buffered and will be drained in Stage C.
			declared := binary.BigEndian.Uint32(g.header[4:])
			g.payloadRemaining = declared
			g.headerValid = true
		}

		// Stage C: drain any buffered header bytes that have already
		// been validated. This is the only path that copies the
		// header bytes into the caller's buffer.
		if g.headerOut < g.headerRead {
			n := copy(p[written:], g.header[g.headerOut:g.headerRead])
			g.headerOut += n
			written += n
			if written == len(p) {
				return written, nil
			}
			continue
		}

		// Stage D: read payload bytes if a frame is in progress.
		if g.payloadRemaining > 0 {
			want := len(p) - written
			if uint32(want) > g.payloadRemaining {
				want = int(g.payloadRemaining)
			}
			n, err := g.source.Read(p[written : written+want])
			written += n
			if n > 0 {
				g.payloadRemaining -= uint32(n)
			}
			if err != nil {
				if n > 0 {
					// Bytes already produced are returned.
					// Structured truncation is preserved as the
					// terminal error for the next call.
					if errors.Is(err, io.EOF) && g.payloadRemaining > 0 {
						g.terminalErr = errors.Join(ErrIncompleteControlFramePayload, io.ErrUnexpectedEOF)
					} else {
						g.terminalErr = err
					}
					return written, nil
				}
				// n == 0 with EOF
				if errors.Is(err, io.EOF) {
					if g.payloadRemaining > 0 {
						// Partial payload: bytes already produced
						// in this call are returned first.
						if written > 0 {
							g.terminalErr = errors.Join(ErrIncompleteControlFramePayload, io.ErrUnexpectedEOF)
							return written, nil
						}
						return 0, errors.Join(ErrIncompleteControlFramePayload, io.ErrUnexpectedEOF)
					}
					// Graceful EOF: payload was fully consumed.
					if written > 0 {
						g.terminalErr = io.EOF
						return written, nil
					}
					return 0, io.EOF
				}
				return 0, err
			}
			if n == 0 {
				if written > 0 {
					return written, nil
				}
				return 0, io.ErrNoProgress
			}
			continue
		}

		// Stage A: read header bytes for a new frame.
		// Reset header cursor and validity flag before reading.
		g.headerRead = 0
		g.headerOut = 0
		g.headerValid = false
		for g.headerRead < dockerFrameHeaderSize {
			n, err := g.source.Read(g.header[g.headerRead:])
			g.headerRead += n
			if err != nil {
				if errors.Is(err, io.EOF) {
					if g.headerRead == 0 {
						// Graceful EOF: no header bytes consumed
						// in this call. Honor the io.Reader contract.
						if written > 0 {
							g.terminalErr = io.EOF
							return written, nil
						}
						return 0, io.EOF
					}
					if g.headerRead < dockerFrameHeaderSize {
						// Partial header: 1-7 bytes consumed.
						partialErr := errors.Join(ErrIncompleteControlFrameHeader, io.ErrUnexpectedEOF)
						if written > 0 {
							// Return any bytes already produced
							// in this call first.
							g.terminalErr = partialErr
							return written, nil
						}
						return 0, partialErr
					}
					// Header complete and EOF in the same call;
					// validate on the next iteration.
					break
				}
				if n > 0 {
					g.terminalErr = err
					return written, nil
				}
				return 0, err
			}
			if n == 0 {
				if written > 0 {
					return written, nil
				}
				return 0, io.ErrNoProgress
			}
		}
	}
	return written, nil
}

// validateHeader enforces the canonical Docker multiplex header rules.
// It is called only after all 8 bytes are buffered.
func (g *controlFrameGuard) validateHeader() error {
	// Reserved bytes 1, 2, 3 must be zero.
	if g.header[1] != 0 || g.header[2] != 0 || g.header[3] != 0 {
		return ErrNoncanonicalControlFrameHeader
	}
	// Stream identifier must be 1, 2, or 3.
	switch g.header[0] {
	case 1, 2, 3:
		// accepted
	case 0:
		return ErrUnexpectedControlStream
	default:
		return ErrInvalidControlFrame
	}
	// Declared payload size must be within the per-stream limit.
	declared := binary.BigEndian.Uint32(g.header[4:])
	if declared > frameLimit(g.header[0]) {
		return ErrControlFrameTooLarge
	}
	return nil
}

// validDockerStream reports whether stream is 1 (stdout), 2 (stderr), or
// 3 (system-error). Stream 0 (stdin) and unknown values 4-255 are rejected
// by the caller. This helper is retained for callers that need to test the
// canonical three streams without caring about the rejection category.
func validDockerStream(stream byte) bool {
	return stream >= 1 && stream <= 3
}

// frameLimit returns the per-stream declared-size limit.
func frameLimit(stream byte) uint32 {
	switch stream {
	case 1:
		return uint32(MaxControlStdout)
	default:
		return uint32(MaxControlStderr)
	}
}
