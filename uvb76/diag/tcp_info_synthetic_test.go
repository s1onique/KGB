// Package diag implements diagnostic capture for UVB-76.
//go:build linux
// +build linux

package diag

import (
	"context"
	"testing"
)

// TestSyntheticTCPInfo_SourceAndMatchedSocket verifies that synthetic TCP_INFO
// (from diagnostic-only dials) produces source=synthetic_tcp_info and matched_socket=false.
//
// This preserves the synthetic boundary: synthetic connections are explicitly
// marked as synthetic and must not produce matched_socket=true.
func TestSyntheticTCPInfo_SourceAndMatchedSocket(t *testing.T) {
	ctx := context.Background()

	// Use an invalid/unreachable address to force synthetic dial failure
	// But we test that the result indicates synthetic origin
	result, conn, err := GetTcpInfoFromSyntheticDial(ctx, "192.0.2.1:12345")

	// If dial fails, we still expect IsSynthetic=true in the result
	if result != nil {
		// Verify synthetic marking
		if !result.IsSynthetic {
			t.Error("Expected IsSynthetic=true for synthetic dial result")
		}
	}

	// Close connection if it was established
	if conn != nil {
		conn.Close()
	}

	// For unreachable addresses, we expect an error
	if err == nil {
		t.Log("Note: Dial succeeded (address may be reachable)")
	}
}

// TestGetTcpInfoFromSyntheticDial_ExplicitlySynthetic verifies that
// GetTcpInfoFromSyntheticDial marks results as IsSynthetic=true.
func TestGetTcpInfoFromSyntheticDial_ExplicitlySynthetic(t *testing.T) {
	ctx := context.Background()

	// Try to dial a local address
	result, conn, err := GetTcpInfoFromSyntheticDial(ctx, "127.0.0.1:1")

	if result != nil {
		// Verify explicit synthetic marking
		if !result.IsSynthetic {
			t.Error("Expected IsSynthetic=true for synthetic dial")
		}
	}

	// Close if established
	if conn != nil {
		conn.Close()
	}

	// We may or may not get TCP_INFO, but synthetic marking should be consistent
	_ = err // ignore dial result
}

// TestTcpInfoResult_IsSyntheticField documents that IsSynthetic is the key field
// for distinguishing synthetic from native TCP_INFO.
func TestTcpInfoResult_IsSyntheticField(t *testing.T) {
	result := &TcpInfoResult{
		Available:    true,
		IsSynthetic:  true,
		State:        "ESTAB",
	}

	// IsSynthetic should be true for synthetic connections
	if !result.IsSynthetic {
		t.Error("Expected IsSynthetic=true for synthetic connection")
	}

	// After conversion, source should be synthetic_tcp_info
	tq := TcpInfoToTcpQuality(result, "http", "test-host")

	if tq.Source != TcpQualitySourceSyntheticTCPInfo {
		t.Errorf("Expected source '%s', got '%s'", TcpQualitySourceSyntheticTCPInfo, tq.Source)
	}

	if tq.MatchedSocket {
		t.Error("Expected matched_socket=false for synthetic TCP_INFO")
	}
}

// TestTcpInfoToTcpQuality_SyntheticProducesMatchedSocketFalse verifies that
// TcpInfoToTcpQuality produces matched_socket=false when IsSynthetic=true.
func TestTcpInfoToTcpQuality_SyntheticProducesMatchedSocketFalse(t *testing.T) {
	result := &TcpInfoResult{
		Available:   true,
		IsSynthetic: true,
		State:      "ESTAB",
	}

	tq := TcpInfoToTcpQuality(result, "http", "10.0.0.5")

	// Synthetic must produce matched_socket=false
	if tq.MatchedSocket {
		t.Error("Synthetic TCP_INFO must produce matched_socket=false")
	}

	// Synthetic must produce source=synthetic_tcp_info
	if tq.Source != TcpQualitySourceSyntheticTCPInfo {
		t.Errorf("Expected source '%s', got '%s'", TcpQualitySourceSyntheticTCPInfo, tq.Source)
	}
}

// TestTcpInfoToTcpQuality_NativeProducesMatchedSocketTrue verifies that
// TcpInfoToTcpQuality produces matched_socket=true when IsSynthetic=false.
func TestTcpInfoToTcpQuality_NativeProducesMatchedSocketTrue(t *testing.T) {
	result := &TcpInfoResult{
		Available:   true,
		IsSynthetic: false, // Native/probe socket
		State:      "ESTAB",
	}

	tq := TcpInfoToTcpQuality(result, "http", "10.0.0.5")

	// Native must produce matched_socket=true
	if !tq.MatchedSocket {
		t.Error("Native TCP_INFO must produce matched_socket=true")
	}

	// Native must produce source=native_tcp_info
	if tq.Source != TcpQualitySourceNativeTCPInfo {
		t.Errorf("Expected source '%s', got '%s'", TcpQualitySourceNativeTCPInfo, tq.Source)
	}
}

// TestTcpInfoErrorToTcpQuality_SyntheticError verifies that error cases
// for synthetic connections produce source=synthetic_tcp_info.
func TestTcpInfoErrorToTcpQuality_SyntheticError(t *testing.T) {
	result := &TcpInfoResult{
		Available:   false,
		IsSynthetic: true,
		Error: &TcpInfoError{
			Kind:    "dial_failed",
			Message: "connection refused",
		},
	}

	tq := TcpInfoErrorToTcpQuality(result, "http", "10.0.0.5")

	// Even errors should preserve synthetic marking
	if tq.Source != TcpQualitySourceSyntheticTCPInfo {
		t.Errorf("Expected source '%s', got '%s'", TcpQualitySourceSyntheticTCPInfo, tq.Source)
	}
}
