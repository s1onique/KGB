// Package diag implements diagnostic capture for UVB-76.
// This file provides TCP quality evidence collection using ss command.
package diag

import (
	"context"
	"errors"
	"io"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/s1onique/KGB/uvb76/state"
)

// =============================================================================
// TcpQualityCollector — Collects TCP path quality evidence using ss
// =============================================================================

// Default limits for ss command output capture.
const (
	DefaultSSMaxStdoutBytes = 8192
	DefaultSSMaxStderrBytes = 512
	DefaultSSTimeout        = 2 * time.Second
)

// CommandRunner executes system commands. Used for testability.
type CommandRunner interface {
	RunCommand(ctx context.Context, name string, args ...string) ssCommandResult
}

// defaultCommandRunner implements CommandRunner using exec.CommandContext.
type defaultCommandRunner struct{}

func (r *defaultCommandRunner) RunCommand(ctx context.Context, name string, args ...string) ssCommandResult {
	cmd := exec.CommandContext(ctx, name, args...)
	return runSsCommandWithCmd(cmd, DefaultSSMaxStdoutBytes, DefaultSSMaxStderrBytes)
}

// TcpQualityCollector collects TCP quality evidence for diagnostic packets.
type TcpQualityCollector struct {
	timeout        time.Duration
	maxStdoutBytes int
	maxStderrBytes int
	runner         CommandRunner
}

// NewTcpQualityCollector creates a new TCP quality collector with default settings.
func NewTcpQualityCollector() *TcpQualityCollector {
	return &TcpQualityCollector{
		timeout:        DefaultSSTimeout,
		maxStdoutBytes: DefaultSSMaxStdoutBytes,
		maxStderrBytes: DefaultSSMaxStderrBytes,
	}
}

// CollectTcpQuality collects TCP quality evidence for the given probe kind and target.
// For HTTP probes, it attempts to find the matching TCP socket and extract quality metrics.
// For ICMP probes, it returns an unavailable block (TCP quality is HTTP/TCP-only).
func (c *TcpQualityCollector) CollectTcpQuality(ctx context.Context, probeKind string, target string) *state.TcpQuality {
	now := time.Now().UTC()
	collectedAt := now.Format(time.RFC3339)

	// TCP quality is only applicable to HTTP probes
	if probeKind != "http" {
		return &state.TcpQuality{
			Kind:           probeKind,
			LookupTarget:  target,
			MatchedSocket: false,
			Source:        "ss-tcp-info",
			ErrorKind:     state.TcpQualityErrorUnavailable,
			Error:         "tcp quality is http/probe-only",
			CollectedAt:   collectedAt,
		}
	}

	if target == "" {
		return &state.TcpQuality{
			Kind:           probeKind,
			LookupTarget:  "",
			MatchedSocket: false,
			Source:        "ss-tcp-info",
			ErrorKind:     state.TcpQualityErrorTargetUnresolved,
			Error:         "target is empty",
			CollectedAt:   collectedAt,
		}
	}

	// Resolve target to IP if it's a hostname
	resolvedIP := c.resolveTarget(target)
	if resolvedIP == "" {
		return &state.TcpQuality{
			Kind:           probeKind,
			LookupTarget:  target,
			MatchedSocket: false,
			Source:        "ss-tcp-info",
			ErrorKind:     state.TcpQualityErrorTargetUnresolved,
			Error:         "cannot resolve target",
			CollectedAt:   collectedAt,
		}
	}

	// Run ss command with timeout
	// Always derive a child timeout from the parent context to ensure the collector
	// is bounded by both the collector timeout and the parent capture context.
	cmdCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	result := c.runSsCommand(cmdCtx, resolvedIP)

	// Handle timeout
	if cmdCtx.Err() == context.DeadlineExceeded {
		return &state.TcpQuality{
			Kind:           probeKind,
			LookupTarget:  resolvedIP,
			MatchedSocket: false,
			Source:        "ss-tcp-info",
			ErrorKind:     state.TcpQualityErrorTimeout,
			Error:         "ss command timed out",
			CollectedAt:   collectedAt,
		}
	}

	// Handle cancellation
	if cmdCtx.Err() == context.Canceled {
		return &state.TcpQuality{
			Kind:           probeKind,
			LookupTarget:  resolvedIP,
			MatchedSocket: false,
			Source:        "ss-tcp-info",
			ErrorKind:     state.TcpQualityErrorTimeout,
			Error:         "ss command canceled",
			CollectedAt:   collectedAt,
		}
	}

	// Handle command not found
	if result.CommandNotFound {
		return &state.TcpQuality{
			Kind:           probeKind,
			LookupTarget:  resolvedIP,
			MatchedSocket: false,
			Source:        "ss-tcp-info",
			ErrorKind:     state.TcpQualityErrorCommandMissing,
			Error:         "ss command not found",
			CollectedAt:   collectedAt,
		}
	}

	// Handle non-zero exit
	if result.ExitCode != 0 {
		return &state.TcpQuality{
			Kind:           probeKind,
			LookupTarget:  resolvedIP,
			MatchedSocket: false,
			Source:        "ss-tcp-info",
			ErrorKind:     state.TcpQualityErrorNonZeroExit,
			Error:         "ss command failed",
			CollectedAt:   collectedAt,
		}
	}

	// Parse ss output
	rawOutput := strings.TrimSpace(string(result.Stdout))
	if rawOutput == "" {
		return &state.TcpQuality{
			Kind:           probeKind,
			LookupTarget:  resolvedIP,
			MatchedSocket: false,
			Source:        "ss-tcp-info",
			ErrorKind:     state.TcpQualityErrorNoData,
			Error:         "empty ss output",
			CollectedAt:   collectedAt,
		}
	}

	sockets, err := parseSsOutput(rawOutput)
	if err != nil {
		return &state.TcpQuality{
			Kind:           probeKind,
			LookupTarget:  resolvedIP,
			MatchedSocket: false,
			Source:        "ss-tcp-info",
			ErrorKind:     state.TcpQualityErrorParseFailed,
			Error:         "failed to parse ss output",
			CollectedAt:   collectedAt,
		}
	}

	// Find best matching socket
	matched, matchCount := findMatchingSocket(sockets, resolvedIP)
	if matched == nil {
		return &state.TcpQuality{
			Kind:           probeKind,
			LookupTarget:  resolvedIP,
			MatchedSocket: false,
			Source:        "ss-tcp-info",
			ErrorKind:     state.TcpQualityErrorNoMatchingSocket,
			Error:         "no matching TCP socket found for probe destination",
			MatchCount:    &matchCount,
			CollectedAt:   collectedAt,
		}
	}

	// Build successful TcpQuality block
	// Queue fields are pointers to preserve observed zero values.
	tq := &state.TcpQuality{
		Kind:               probeKind,
		LookupTarget:       resolvedIP,
		MatchedSocket:      true,
		Source:             "ss-tcp-info",
		MatchCount:         &matchCount,
		State:              matched.State,
		Local:              redactAddress(matched.Local),
		Remote:             redactAddress(matched.Remote),
		SendQueueBytes:     matched.SendQueueBytes,
		RecvQueueBytes:     matched.RecvQueueBytes,
		RTTUs:              matched.RTTUs,
		RTTVarUs:           matched.RTTVarUs,
		RetransmitsCurrent: matched.RetransmitsCurrent,
		RetransmitsTotal:   matched.RetransmitsTotal,
		Unacked:            matched.Unacked,
		Lost:               matched.Lost,
		Sacked:             matched.Sacked,
		Reordering:         matched.Reordering,
		SndCwnd:            matched.SndCwnd,
		Ssthresh:           matched.Ssthresh,
		DeliveryRateBps:    matched.DeliveryRateBps,
		CollectedAt:        collectedAt,
	}

	return tq
}

// resolveTarget resolves a hostname to an IP address.
func (c *TcpQualityCollector) resolveTarget(target string) string {
	// If it looks like an IP address already, return as-is
	if net.ParseIP(target) != nil {
		return target
	}

	// Try to resolve
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupIP(ctx, "ip", target)
	if err != nil || len(addrs) == 0 {
		return ""
	}

	return addrs[0].String()
}

// ssCommandResult contains the result of an ss command execution.
type ssCommandResult struct {
	Stdout          []byte
	Stderr          []byte
	ExitCode        int
	CommandNotFound bool
	Truncated       bool
	Err             error
}

// runSsCommand executes the ss command and returns captured output.
// Uses concurrent goroutines to read stdout and stderr to prevent deadlocks.
func (c *TcpQualityCollector) runSsCommand(ctx context.Context, targetIP string) ssCommandResult {
	// Use injected runner for testing, otherwise use default
	if c.runner != nil {
		return c.runner.RunCommand(ctx, "ss", "-tin")
	}
	cmd := exec.CommandContext(ctx, "ss", "-tin")
	return runSsCommandWithCmd(cmd, c.maxStdoutBytes, c.maxStderrBytes)
}

// runSsCommandWithCmd executes a prepared command and returns captured output.
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

	// Read stdout and stderr concurrently to prevent deadlocks
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

// copyBounded reads from reader until limit is reached or EOF.
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

	// Drain remaining output to prevent blocking the child
	if truncated || len(buf) >= limit {
		truncated = true
		for {
			n, err := reader.Read(readBuf)
			if n > 0 {
				// Discard
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
