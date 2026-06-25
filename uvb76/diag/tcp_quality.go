// Package diag implements diagnostic capture for UVB-76.
// This file provides TCP quality evidence collection with native TCP_INFO primary
// and explicit ss -tin fallback seam.
package diag

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"sync"
	"time"

	"github.com/s1onique/KGB/uvb76/state"
)

const (
	DefaultSSMaxStdoutBytes = 8192
	DefaultSSMaxStderrBytes = 512
	DefaultSSTimeout = 2 * time.Second
	DefaultNativeTimeout = 5 * time.Second
)

type CommandRunner interface {
	RunCommand(ctx context.Context, name string, args ...string) ssCommandResult
}

type defaultCommandRunner struct{}

func (r *defaultCommandRunner) RunCommand(ctx context.Context, name string, args ...string) ssCommandResult {
	cmd := exec.CommandContext(ctx, name, args...)
	return runSsCommandWithCmd(cmd, DefaultSSMaxStdoutBytes, DefaultSSMaxStderrBytes)
}

type TcpQualityCollector struct {
	timeout        time.Duration
	maxStdoutBytes int
	maxStderrBytes int
	runner        CommandRunner
	nativeTimeout  time.Duration
	useCLIFallback bool
}

func NewTcpQualityCollector() *TcpQualityCollector {
	return &TcpQualityCollector{
		timeout:        DefaultSSTimeout,
		maxStdoutBytes: DefaultSSMaxStdoutBytes,
		maxStderrBytes: DefaultSSMaxStderrBytes,
		nativeTimeout:  DefaultNativeTimeout,
	}
}

// CollectTcpQuality collects TCP quality evidence for the given probe kind and target.
// Uses synthetic TCP_INFO dial as the primary native backend, with ss -tin behind an explicit fallback seam.
// The synthetic dial is explicitly labeled as such in the source field.
func (c *TcpQualityCollector) CollectTcpQuality(ctx context.Context, probeKind string, target string) *state.TcpQuality {
	now := time.Now().UTC()
	collectedAt := now.Format(time.RFC3339)

	if probeKind != "http" {
		return unavailableResult(probeKind, target, TcpQualitySourceUnavailable,
			state.TcpQualityErrorUnavailable, "tcp quality is http/probe-only", collectedAt)
	}
	if target == "" {
		return unavailableResult(probeKind, "", TcpQualitySourceUnavailable,
			state.TcpQualityErrorTargetUnresolved, "target is empty", collectedAt)
	}

	resolvedIP := c.resolveTarget(target)
	if resolvedIP == "" {
		return unavailableResult(probeKind, target, TcpQualitySourceUnavailable,
			state.TcpQualityErrorTargetUnresolved, "cannot resolve target", collectedAt)
	}

	if !c.useCLIFallback {
		if nativeResult := c.collectNativeTcpInfo(ctx, probeKind, resolvedIP); nativeResult != nil {
			return nativeResult
		}
	}

	return c.collectViaCLI(ctx, probeKind, resolvedIP, collectedAt)
}

// collectNativeTcpInfo uses synthetic TCP_INFO dial as the native backend.
// This is explicitly labeled as synthetic since it measures a diagnostic-only connection.
func (c *TcpQualityCollector) collectNativeTcpInfo(ctx context.Context, probeKind string, targetIP string) *state.TcpQuality {
	nativeCtx, cancel := context.WithTimeout(ctx, c.nativeTimeout)
	defer cancel()

	address := fmt.Sprintf("%s:80", targetIP)
	result, conn, err := GetTcpInfoFromSyntheticDial(nativeCtx, address)
	if conn != nil {
		conn.Close()
	}

	if err != nil {
		return nil
	}

	if !result.Available {
		if result.Error != nil {
			switch result.Error.Kind {
			case "not_supported", "unsupported":
				return nil
			case "not_tcp":
				return TcpInfoErrorToTcpQuality(result, probeKind, targetIP)
			default:
				return nil
			}
		}
		return nil
	}

	return TcpInfoToTcpQuality(result, probeKind, targetIP)
}

func (c *TcpQualityCollector) resolveTarget(target string) string {
	if net.ParseIP(target) != nil {
		return target
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupIP(ctx, "ip", target)
	if err != nil || len(addrs) == 0 {
		return ""
	}
	return addrs[0].String()
}

type ssCommandResult struct {
	Stdout          []byte
	Stderr          []byte
	ExitCode        int
	CommandNotFound bool
	Truncated       bool
	Err             error
}

func (c *TcpQualityCollector) runSsCommand(ctx context.Context, targetIP string) ssCommandResult {
	if c.runner != nil {
		return c.runner.RunCommand(ctx, "ss", "-tin")
	}
	cmd := exec.CommandContext(ctx, "ss", "-tin")
	return runSsCommandWithCmd(cmd, c.maxStdoutBytes, c.maxStderrBytes)
}

func runSsCommandWithCmd(cmd *exec.Cmd, maxStdoutBytes, maxStderrBytes int) ssCommandResult {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return ssCommandResult{Err: err}
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		stdoutPipe.Close()
		return ssCommandResult{Err: err}
	}

	if err := cmd.Start(); err != nil {
		stdoutPipe.Close()
		stderrPipe.Close()
		if errors.Is(err, exec.ErrNotFound) {
			return ssCommandResult{CommandNotFound: true}
		}
		return ssCommandResult{Err: err}
	}

	var wg sync.WaitGroup
	var stdout []byte
	var stderr []byte
	var stdoutTruncated bool

	wg.Add(2)
	go func() {
		defer wg.Done()
		stdout, stdoutTruncated = copyBounded(stdoutPipe, maxStdoutBytes)
	}()
	go func() {
		defer wg.Done()
		stderr, _ = copyBounded(stderrPipe, maxStderrBytes)
	}()
	wg.Wait()

	stdoutPipe.Close()
	stderrPipe.Close()

	waitErr := cmd.Wait()
	var exitErr *exec.ExitError
	exitCode := 0
	if errors.As(waitErr, &exitErr) {
		exitCode = exitErr.ExitCode()
	}

	return ssCommandResult{
		Stdout:    stdout,
		Stderr:    stderr,
		ExitCode:  exitCode,
		Truncated: stdoutTruncated,
	}
}

func copyBounded(reader io.Reader, limit int) ([]byte, bool) {
	buf := make([]byte, 0, limit)
	truncated := false
	readBuf := make([]byte, 256)

	for len(buf) < limit {
		n, err := reader.Read(readBuf)
		if n > 0 {
			if len(buf)+n > limit {
				n = limit - len(buf)
				truncated = true
			}
			buf = append(buf, readBuf[:n]...)
			if len(buf) >= limit {
				break
			}
		}
		if err != nil {
			if err == io.EOF {
				return buf, truncated
			}
			break
		}
	}

	if truncated || len(buf) >= limit {
		truncated = true
		for {
			n, err := reader.Read(readBuf)
			if n > 0 {
			}
			if err != nil {
				if err == io.EOF {
					break
				}
				break
			}
		}
	}

	return buf, truncated
}
