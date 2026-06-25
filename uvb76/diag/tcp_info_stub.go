// Package diag implements diagnostic capture for UVB-76.
// This file provides stub implementations for non-Linux platforms.
//go:build !linux
// +build !linux

package diag

import (
	"context"
	"fmt"
	"net"
)

// tcpInfoKernel is the stable prefix of Linux struct tcp_info.
// Stub definition for type consistency.
type tcpInfoKernel struct{}

// TcpInfoResult contains the result of native TCP_INFO collection.
// On non-Linux platforms, this always indicates TCP_INFO is unavailable.
type TcpInfoResult struct {
	// Available indicates whether TCP_INFO was retrievable
	Available bool

	// IsSynthetic indicates this was collected from a synthetic connection
	IsSynthetic bool

	// State is the TCP connection state (not available on non-Linux)
	State string

	RTTUs              *int64
	RTTVarUs           *int64
	RetransmitsCurrent *int64
	RetransmitsTotal   *int64
	SndCwnd           *int32
	Ssthresh          *int32
	Unacked           *int64
	Lost              *int64
	Sacked            *int64
	Reordering        *int64

	LocalAddr  string
	RemoteAddr string

	Error *TcpInfoError
}

// TcpInfoError represents a TCP_INFO collection error.
type TcpInfoError struct {
	Kind    string
	Message string
}

func (e *TcpInfoError) Error() string { return e.Message }

// GetTcpInfo retrieves TCP_INFO for a connected socket.
// On non-Linux platforms, this always returns an error.
func GetTcpInfo(ctx context.Context, conn net.Conn) *TcpInfoResult {
	result := &TcpInfoResult{
		Available:   false,
		IsSynthetic: false,
		Error: &TcpInfoError{
			Kind:    "unsupported",
			Message: "TCP_INFO is only available on Linux",
		},
	}

	if conn != nil {
		if conn.LocalAddr() != nil {
			result.LocalAddr = conn.LocalAddr().String()
		}
		if conn.RemoteAddr() != nil {
			result.RemoteAddr = conn.RemoteAddr().String()
		}
	}

	_ = ctx
	return result
}

// GetTcpInfoFromSyntheticDial is a stub that always fails on non-Linux.
func GetTcpInfoFromSyntheticDial(ctx context.Context, address string) (*TcpInfoResult, net.Conn, error) {
	result := &TcpInfoResult{
		Available:   false,
		IsSynthetic: true,
		Error: &TcpInfoError{
			Kind:    "unsupported",
			Message: "TCP_INFO is only available on Linux",
		},
	}

	return result, nil, fmt.Errorf("TCP_INFO is only available on Linux")
}
