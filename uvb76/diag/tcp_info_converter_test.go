package diag

import (
	"testing"

	"github.com/s1onique/KGB/uvb76/state"
)

func TestTcpInfoToTcpQuality_Success(t *testing.T) {
	// Create a mock TCP_INFO result
	result := &TcpInfoResult{
		Available: true,
		State:     "ESTAB",
		RTTUs:     intPtr(113000),
		RTTVarUs:  intPtr(12000),
		RetransmitsCurrent: intPtr(0),
		RetransmitsTotal:   intPtr(3),
		Unacked:   intPtr(5),
		Lost:      intPtr(0),
		Sacked:    intPtr(10),
		Reordering: intPtr(3),
		SndCwnd:   int32Ptr(10),
		Ssthresh:  int32Ptr(2147483647),
		LocalAddr:  "10.0.0.1:45678",
		RemoteAddr: "10.0.0.5:8080",
	}

	tq := TcpInfoToTcpQuality(result, "http", "10.0.0.5")

	if tq == nil {
		t.Fatal("expected non-nil TcpQuality")
	}
	if tq.Kind != "http" {
		t.Errorf("expected kind 'http', got '%s'", tq.Kind)
	}
	if tq.LookupTarget != "10.0.0.5" {
		t.Errorf("expected lookup_target '10.0.0.5', got '%s'", tq.LookupTarget)
	}
	if !tq.MatchedSocket {
		t.Error("expected matched_socket=true")
	}
	if tq.Source != TcpQualitySourceNativeTCPInfo {
		t.Errorf("expected source '%s', got '%s'", TcpQualitySourceNativeTCPInfo, tq.Source)
	}
	if tq.State != "ESTAB" {
		t.Errorf("expected state 'ESTAB', got '%s'", tq.State)
	}
	if tq.RTTUs == nil || *tq.RTTUs != 113000 {
		t.Errorf("expected rtt_us 113000, got %v", tq.RTTUs)
	}
	if tq.SndCwnd == nil || *tq.SndCwnd != 10 {
		t.Errorf("expected snd_cwnd 10, got %v", tq.SndCwnd)
	}
	if tq.Local != "redacted:45678" {
		t.Errorf("expected local 'redacted:45678', got '%s'", tq.Local)
	}
	if tq.Remote != "redacted:8080" {
		t.Errorf("expected remote 'redacted:8080', got '%s'", tq.Remote)
	}
}

func TestTcpInfoToTcpQuality_Unavailable(t *testing.T) {
	// Test nil result
	tq := TcpInfoToTcpQuality(nil, "http", "10.0.0.5")
	if tq == nil {
		t.Fatal("expected non-nil TcpQuality")
	}
	if tq.MatchedSocket {
		t.Error("expected matched_socket=false")
	}
	if tq.Source != TcpQualitySourceUnavailable {
		t.Errorf("expected source '%s', got '%s'", TcpQualitySourceUnavailable, tq.Source)
	}
}

func TestTcpInfoToTcpQuality_NotAvailable(t *testing.T) {
	result := &TcpInfoResult{
		Available: false,
		Error: &TcpInfoError{
			Kind:    "not_supported",
			Message: "TCP_INFO not supported",
		},
	}

	tq := TcpInfoToTcpQuality(result, "http", "10.0.0.5")
	if tq == nil {
		t.Fatal("expected non-nil TcpQuality")
	}
	if tq.MatchedSocket {
		t.Error("expected matched_socket=false")
	}
	if tq.Source != TcpQualitySourceUnavailable {
		t.Errorf("expected source '%s', got '%s'", TcpQualitySourceUnavailable, tq.Source)
	}
}

func TestTcpInfoErrorToTcpQuality(t *testing.T) {
	result := &TcpInfoResult{
		Available: false,
		Error: &TcpInfoError{
			Kind:    "dial_failed",
			Message: "connection refused",
		},
	}

	tq := TcpInfoErrorToTcpQuality(result, "http", "10.0.0.5")
	if tq == nil {
		t.Fatal("expected non-nil TcpQuality")
	}
	if tq.MatchedSocket {
		t.Error("expected matched_socket=false")
	}
	if tq.Source != TcpQualitySourceNativeTCPInfo {
		t.Errorf("expected source '%s', got '%s'", TcpQualitySourceNativeTCPInfo, tq.Source)
	}
	if tq.ErrorKind != state.TcpQualityErrorTargetUnresolved {
		t.Errorf("expected error_kind 'target_unresolved', got '%s'", tq.ErrorKind)
	}
}

func TestMapErrorKind(t *testing.T) {
	tests := []struct {
		result       *TcpInfoResult
		expectedKind state.TcpQualityErrorKind
	}{
		{
			result: &TcpInfoResult{
				Error: &TcpInfoError{Kind: "dial_failed", Message: "refused"},
			},
			expectedKind: state.TcpQualityErrorTargetUnresolved,
		},
		{
			result: &TcpInfoResult{
				Error: &TcpInfoError{Kind: "not_supported", Message: "unsupported"},
			},
			expectedKind: state.TcpQualityErrorUnavailable,
		},
		{
			result: &TcpInfoResult{
				Error: &TcpInfoError{Kind: "unsupported", Message: "unsupported"},
			},
			expectedKind: state.TcpQualityErrorUnavailable,
		},
		{
			result: &TcpInfoResult{
				Error: &TcpInfoError{Kind: "getsockopt_failed", Message: "failed"},
			},
			expectedKind: state.TcpQualityErrorNoData,
		},
		{
			result:       nil,
			expectedKind: state.TcpQualityErrorUnavailable,
		},
		{
			result: &TcpInfoResult{},
			expectedKind: state.TcpQualityErrorUnavailable,
		},
	}

	for _, tt := range tests {
		kind := mapErrorKind(tt.result)
		if kind != tt.expectedKind {
			t.Errorf("mapErrorKind(%+v): expected %s, got %s", tt.result, tt.expectedKind, kind)
		}
	}
}

func TestGetErrorMessage(t *testing.T) {
	tests := []struct {
		result    *TcpInfoResult
		expMsg    string
	}{
		{nil, "tcp info result is nil"},
		{&TcpInfoResult{}, "tcp info not available"},
		{&TcpInfoResult{Error: &TcpInfoError{Message: "custom error"}}, "custom error"},
	}

	for _, tt := range tests {
		msg := getErrorMessage(tt.result)
		if msg != tt.expMsg {
			t.Errorf("getErrorMessage(%+v): expected %q, got %q", tt.result, tt.expMsg, msg)
		}
	}
}

// Helper functions

func intPtr(v int64) *int64 {
	return &v
}

func int32Ptr(v int32) *int32 {
	return &v
}
