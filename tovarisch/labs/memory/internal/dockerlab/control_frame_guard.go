package dockerlab

import (
	"encoding/binary"
	"errors"
	"io"
)

var (
	ErrControlFrameTooLarge = errors.New("dockerlab: control frame too large")
	ErrInvalidControlFrame  = errors.New("dockerlab: invalid control frame")
)

const dockerFrameHeaderSize = 8

// controlFrameGuard validates each Docker multiplex frame header before
// stdcopy sees it. It retains only one 8-byte header and never allocates a
// payload-sized buffer.
type controlFrameGuard struct {
	source           io.Reader
	header           [dockerFrameHeaderSize]byte
	headerRead       int
	headerOut        int
	payloadRemaining uint32
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
		if g.headerOut < g.headerRead {
			n := copy(p[written:], g.header[g.headerOut:g.headerRead])
			g.headerOut += n
			written += n
			if written == len(p) {
				return written, nil
			}
			continue
		}

		if g.payloadRemaining > 0 {
			want := len(p) - written
			if uint32(want) > g.payloadRemaining {
				want = int(g.payloadRemaining)
			}
			n, err := g.source.Read(p[written : written+want])
			written += n
			g.payloadRemaining -= uint32(n)
			if err != nil {
				if written > 0 {
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
			continue
		}

		g.headerRead, g.headerOut = 0, 0
		for g.headerRead < dockerFrameHeaderSize {
			n, err := g.source.Read(g.header[g.headerRead:])
			g.headerRead += n
			if err != nil {
				if errors.Is(err, io.EOF) && g.headerRead == 0 {
					if written > 0 {
						g.terminalErr = io.EOF
						return written, nil
					}
					return 0, io.EOF
				}
				if written > 0 {
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

		declared := binary.BigEndian.Uint32(g.header[4:])
		if !validDockerStream(g.header[0]) {
			g.terminalErr = ErrInvalidControlFrame
			return written, ErrInvalidControlFrame
		}
		if declared > frameLimit(g.header[0]) {
			g.terminalErr = ErrControlFrameTooLarge
			return written, ErrControlFrameTooLarge
		}
		g.payloadRemaining = declared
	}
	return written, nil
}

func validDockerStream(stream byte) bool {
	return stream <= 3
}

func frameLimit(stream byte) uint32 {
	switch stream {
	case 1, 0:
		return uint32(MaxControlStdout)
	default:
		return uint32(MaxControlStderr)
	}
}
