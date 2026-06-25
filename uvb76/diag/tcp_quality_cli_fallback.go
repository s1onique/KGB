// Package diag implements diagnostic capture for UVB-76.
// This file provides the CLI fallback implementation for TCP quality collection.
//go:build !tiny
// +build !tiny

package diag

import (
	"context"
	"strings"

	"github.com/s1onique/KGB/uvb76/state"
)

// collectViaCLI collects TCP quality using ss -tin CLI.
// This is the explicit fallback seam when native TCP_INFO is unavailable.
func (c *TcpQualityCollector) collectViaCLI(ctx context.Context, probeKind string, targetIP string, collectedAt string) *state.TcpQuality {
	cmdCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	result := c.runSsCommand(cmdCtx, targetIP)

	if cmdCtx.Err() == context.DeadlineExceeded {
		return unavailableResult(probeKind, targetIP, TcpQualitySourceCLIFallback,
			state.TcpQualityErrorTimeout, "ss command timed out", collectedAt)
	}
	if cmdCtx.Err() == context.Canceled {
		return unavailableResult(probeKind, targetIP, TcpQualitySourceCLIFallback,
			state.TcpQualityErrorTimeout, "ss command canceled", collectedAt)
	}
	if result.CommandNotFound {
		return unavailableResult(probeKind, targetIP, TcpQualitySourceCLIFallback,
			state.TcpQualityErrorCommandMissing, "ss command not found", collectedAt)
	}
	if result.ExitCode != 0 {
		return unavailableResult(probeKind, targetIP, TcpQualitySourceCLIFallback,
			state.TcpQualityErrorNonZeroExit, "ss command failed", collectedAt)
	}

	rawOutput := strings.TrimSpace(string(result.Stdout))
	if rawOutput == "" {
		return unavailableResult(probeKind, targetIP, TcpQualitySourceCLIFallback,
			state.TcpQualityErrorNoData, "empty ss output", collectedAt)
	}

	sockets, err := parseSsOutput(rawOutput)
	if err != nil {
		return unavailableResult(probeKind, targetIP, TcpQualitySourceCLIFallback,
			state.TcpQualityErrorParseFailed, "failed to parse ss output", collectedAt)
	}

	matched, matchCount := findMatchingSocket(sockets, targetIP)
	if matched == nil {
		return unavailableResultWithMatchCount(probeKind, targetIP, TcpQualitySourceCLIFallback,
			state.TcpQualityErrorNoMatchingSocket, "no matching TCP socket found for probe destination",
			&matchCount, collectedAt)
	}

	return cliResultToTcpQuality(matched, matchCount, probeKind, targetIP, collectedAt)
}

// unavailableResult creates an error TcpQuality block.
func unavailableResult(probeKind, target, source string, errKind state.TcpQualityErrorKind, errMsg, collectedAt string) *state.TcpQuality {
	return &state.TcpQuality{
		Kind:           probeKind,
		LookupTarget:  target,
		MatchedSocket: false,
		Source:        source,
		ErrorKind:     errKind,
		Error:         errMsg,
		CollectedAt:   collectedAt,
	}
}

// unavailableResultWithMatchCount creates an error TcpQuality block with match count.
func unavailableResultWithMatchCount(probeKind, target, source string, errKind state.TcpQualityErrorKind, errMsg string, matchCount *int, collectedAt string) *state.TcpQuality {
	return &state.TcpQuality{
		Kind:          probeKind,
		LookupTarget:  target,
		MatchedSocket: false,
		Source:       source,
		ErrorKind:    errKind,
		Error:        errMsg,
		MatchCount:   matchCount,
		CollectedAt:  collectedAt,
	}
}

// cliResultToTcpQuality converts CLI socket data to TcpQuality.
func cliResultToTcpQuality(sock *ssSocket, matchCount int, probeKind, targetIP, collectedAt string) *state.TcpQuality {
	return &state.TcpQuality{
		Kind:               probeKind,
		LookupTarget:       targetIP,
		MatchedSocket:      true,
		Source:             TcpQualitySourceCLIFallback,
		MatchCount:         &matchCount,
		State:              sock.State,
		Local:              redactAddress(sock.Local),
		Remote:             redactAddress(sock.Remote),
		SendQueueBytes:     sock.SendQueueBytes,
		RecvQueueBytes:     sock.RecvQueueBytes,
		RTTUs:              sock.RTTUs,
		RTTVarUs:           sock.RTTVarUs,
		RetransmitsCurrent: sock.RetransmitsCurrent,
		RetransmitsTotal:   sock.RetransmitsTotal,
		Unacked:            sock.Unacked,
		Lost:               sock.Lost,
		Sacked:             sock.Sacked,
		Reordering:          sock.Reordering,
		SndCwnd:            sock.SndCwnd,
		Ssthresh:           sock.Ssthresh,
		DeliveryRateBps:    sock.DeliveryRateBps,
		CollectedAt:        collectedAt,
	}
}
