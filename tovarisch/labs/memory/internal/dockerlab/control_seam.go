// control_seam.go — Docker controller control-exec seam.
package dockerlab

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/canarycontrol"
)

const MaxControlStdout = canarycontrol.MaxResponseBody + 4*1024
const MaxControlStderr = 16 * 1024

type ExecCreateOptions struct {
	Command      []string
	AttachStdout bool
	AttachStderr bool
	TTY          bool
	WorkingDir   string
}

type ExecInspectResult struct {
	ExitCode int
	Running  bool
}

type ControlExecAttachment interface {
	Reader() io.Reader
	Close() error
}

type ControlExecRuntime interface {
	ExecCreate(context.Context, string, ExecCreateOptions) (string, error)
	ExecAttach(context.Context, string, string) (ControlExecAttachment, error)
	ExecInspect(context.Context, string, string) (ExecInspectResult, error)
}

type DockerExecAPI interface {
	ContainerExecCreate(context.Context, string, types.ExecConfig) (types.IDResponse, error)
	ContainerExecAttach(context.Context, string, types.ExecStartCheck) (types.HijackedResponse, error)
	ContainerExecInspect(context.Context, string) (types.ContainerExecInspect, error)
}

var (
	_ ControlExecRuntime = (*ProductionControlExecRuntime)(nil)
	_ DockerExecAPI      = (*client.Client)(nil)
)

type ProductionControlExecRuntime struct {
	Client DockerExecAPI
}

func (p *ProductionControlExecRuntime) ExecCreate(ctx context.Context, containerID string, opts ExecCreateOptions) (string, error) {
	if strings.TrimSpace(containerID) == "" {
		return "", ErrContainerIDRequired
	}
	if p == nil || p.Client == nil {
		return "", errors.New("dockerlab: Docker exec client required")
	}
	response, err := p.Client.ContainerExecCreate(ctx, containerID, types.ExecConfig{
		Cmd:          append([]string(nil), opts.Command...),
		AttachStdout: opts.AttachStdout,
		AttachStderr: opts.AttachStderr,
		Tty:          opts.TTY,
		WorkingDir:   opts.WorkingDir,
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(response.ID) == "" {
		return "", ErrExecIDRequired
	}
	return response.ID, nil
}

func (p *ProductionControlExecRuntime) ExecAttach(ctx context.Context, containerID, execID string) (ControlExecAttachment, error) {
	if strings.TrimSpace(containerID) == "" {
		return nil, ErrContainerIDRequired
	}
	if strings.TrimSpace(execID) == "" {
		return nil, ErrExecIDRequired
	}
	if p == nil || p.Client == nil {
		return nil, errors.New("dockerlab: Docker exec client required")
	}
	response, err := p.Client.ContainerExecAttach(ctx, execID, types.ExecStartCheck{Detach: false, Tty: false})
	if err != nil {
		return nil, err
	}
	if response.Conn == nil {
		return nil, ErrHijackedConnectionRequired
	}
	if response.Reader == nil {
		return nil, errors.Join(ErrHijackedReaderRequired, response.Conn.Close())
	}
	return newDockerExecAttachment(response), nil
}

func (p *ProductionControlExecRuntime) ExecInspect(ctx context.Context, containerID, execID string) (ExecInspectResult, error) {
	if strings.TrimSpace(containerID) == "" {
		return ExecInspectResult{}, ErrContainerIDRequired
	}
	if strings.TrimSpace(execID) == "" {
		return ExecInspectResult{}, ErrExecIDRequired
	}
	if p == nil || p.Client == nil {
		return ExecInspectResult{}, errors.New("dockerlab: Docker exec client required")
	}
	inspect, err := p.Client.ContainerExecInspect(ctx, execID)
	if err != nil {
		return ExecInspectResult{}, err
	}
	if inspect.ExecID != "" && inspect.ExecID != execID {
		return ExecInspectResult{}, errors.New("dockerlab: inspect exec identity mismatch")
	}
	if inspect.ContainerID != "" && inspect.ContainerID != containerID {
		return ExecInspectResult{}, errors.New("dockerlab: inspect container identity mismatch")
	}
	return ExecInspectResult{ExitCode: inspect.ExitCode, Running: inspect.Running}, nil
}

type dockerExecAttachment struct {
	reader    io.Reader
	conn      io.Closer
	closeOnce sync.Once
	closeDone chan struct{}
	closeErr  error
}

func newDockerExecAttachment(response types.HijackedResponse) *dockerExecAttachment {
	return &dockerExecAttachment{reader: response.Reader, conn: response.Conn, closeDone: make(chan struct{})}
}

func (a *dockerExecAttachment) Reader() io.Reader { return a.reader }
func (a *dockerExecAttachment) Close() error {
	a.closeOnce.Do(func() {
		a.closeErr = a.conn.Close()
		close(a.closeDone)
	})
	<-a.closeDone
	return a.closeErr
}

var (
	ErrStdoutOverflow             = errors.New("dockerlab: stdout overflow")
	ErrStderrOverflow             = errors.New("dockerlab: stderr overflow")
	ErrEmptyStdout                = errors.New("dockerlab: empty stdout")
	ErrHijackedConnectionRequired = errors.New("dockerlab: hijacked connection required")
	ErrHijackedReaderRequired     = errors.New("dockerlab: hijacked reader required")
)

type boundedWriter struct {
	buf      []byte
	limit    int
	overflow error
}

func newBoundedWriter(limit int, overflow error) *boundedWriter {
	return &boundedWriter{buf: make([]byte, 0, limit), limit: limit, overflow: overflow}
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	remaining := w.limit - len(w.buf)
	if len(p) <= remaining {
		w.buf = append(w.buf, p...)
		return len(p), nil
	}
	if remaining > 0 {
		w.buf = append(w.buf, p[:remaining]...)
	}
	return remaining, w.overflow
}

func (w *boundedWriter) Bytes() []byte { return w.buf }

func ArgsContainsForbidden(argv []string) bool {
	forbidden := map[string]struct{}{
		"sh": {}, "/bin/sh": {}, "bash": {}, "/bin/bash": {},
		"curl": {}, "wget": {}, "nc": {}, "telnet": {},
	}
	for _, arg := range argv {
		if _, ok := forbidden[arg]; ok {
			return true
		}
	}
	return false
}
