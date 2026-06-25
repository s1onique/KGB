// Package diag implements diagnostic capture for UVB-76.
// This file provides conversion from native TCP_INFO to the existing TcpQuality format.
//go:build !tiny
// +build !tiny

package diag

import (
	"time"

	"github.com/s1onique/KGB/uvb76/state"
)

// Source constants for TcpQuality.Source field
const (
	TcpQualitySourceNativeTCPInfo  = "native_tcp_info"
	TcpQualitySourceSyntheticTCPInfo = "synthetic_tcp_info"
	TcpQualitySourceCLIFallback   = "ss-tcp-info"
	TcpQualitySourceUnavailable  = "unavailable"
)

// TcpInfoToTcpQuality converts a native TcpInfoResult to the existing TcpQuality format.
// This preserves the existing evidence shape while using native TCP_INFO where available.
// When IsSynthetic is true, the source is marked as synthetic_tcp_info.
func TcpInfoToTcpQuality(result *TcpInfoResult, probeKind string, lookupTarget string) *state.TcpQuality {
	now := time.Now().UTC()
	collectedAt := now.Format(time.RFC3339)

	if result == nil || !result.Available {
		return &state.TcpQuality{
			Kind:           probeKind,
			LookupTarget:    lookupTarget,
			MatchedSocket:   false,
			Source:         TcpQualitySourceUnavailable,
			ErrorKind:      state.TcpQualityErrorUnavailable,
			Error:          getErrorMessage(result),
			CollectedAt:    collectedAt,
		}
	}

	// Determine source based on whether measurement is synthetic
	source := TcpQualitySourceNativeTCPInfo
	if result.IsSynthetic {
		source = TcpQualitySourceSyntheticTCPInfo
	}

	// Build successful TcpQuality block from native TCP_INFO
	// MatchedSocket=false for synthetic (diagnostic-only dial), true for actual probe socket
	tq := &state.TcpQuality{
		Kind:               probeKind,
		LookupTarget:       lookupTarget,
		MatchedSocket:      !result.IsSynthetic,
		Source:             source,
		State:              result.State,
		Local:              redactAddress(result.LocalAddr),
		Remote:             redactAddress(result.RemoteAddr),
		RTTUs:              result.RTTUs,
		RTTVarUs:           result.RTTVarUs,
		RetransmitsCurrent: result.RetransmitsCurrent,
		RetransmitsTotal:   result.RetransmitsTotal,
		Unacked:            result.Unacked,
		Lost:               result.Lost,
		Sacked:             result.Sacked,
		Reordering:          result.Reordering,
		SndCwnd:            result.SndCwnd,
		Ssthresh:           result.Ssthresh,
		CollectedAt:        collectedAt,
	}

	return tq
}

// TcpInfoErrorToTcpQuality creates an error TcpQuality from a native TCP_INFO error.
func TcpInfoErrorToTcpQuality(result *TcpInfoResult, probeKind string, lookupTarget string) *state.TcpQuality {
	now := time.Now().UTC()
	collectedAt := now.Format(time.RFC3339)

	source := TcpQualitySourceNativeTCPInfo
	if result != nil && result.IsSynthetic {
		source = TcpQualitySourceSyntheticTCPInfo
	}

	return &state.TcpQuality{
		Kind:           probeKind,
		LookupTarget:   lookupTarget,
		MatchedSocket:   false,
		Source:         source,
		ErrorKind:      mapErrorKind(result),
		Error:          getErrorMessage(result),
		CollectedAt:    collectedAt,
	}
}

func getErrorMessage(result *TcpInfoResult) string {
	if result == nil {
		return "tcp info result is nil"
	}
	if result.Error == nil {
		return "tcp info not available"
	}
	return result.Error.Message
}

func mapErrorKind(result *TcpInfoResult) state.TcpQualityErrorKind {
	if result == nil || result.Error == nil {
		return state.TcpQualityErrorUnavailable
	}

	switch result.Error.Kind {
	case "dial_failed":
		return state.TcpQualityErrorTargetUnresolved
	case "not_supported", "unsupported":
		return state.TcpQualityErrorUnavailable
	case "getsockopt_failed", "syscall_conn_failed":
		return state.TcpQualityErrorNoData
	default:
		return state.TcpQualityErrorUnavailable
	}
}
