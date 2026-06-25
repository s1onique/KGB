// Package diag implements diagnostic capture for UVB-76.
// This file provides native TCP_INFO via getsockopt for Linux platforms.
//go:build linux
// +build linux

package diag

import (
	"context"
	"fmt"
	"net"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/s1onique/KGB/uvb76/state"
)

// Linux TCP_INFO constants
const (
	IPPROTO_TCP = 6
	TCP_INFO    = 11
)

// Linux TCP states
const (
	tcpStateEstablished = 1
	tcpStateSynSent    = 2
	tcpStateSynRecv    = 3
	tcpStateFinWait1   = 4
	tcpStateFinWait2   = 5
	tcpStateTimeWait   = 6
	tcpStateClose      = 7
	tcpStateCloseWait  = 8
	tcpStateLastAck    = 9
	tcpStateListen     = 10
	tcpStateClosing    = 11
)

// tcpInfoKernel is the stable prefix of Linux struct tcp_info.
// Per linux/tcp.h, the first fields are stable across kernels.
// We only use the stable prefix to avoid reading garbage from newer kernels.
type tcpInfoKernel struct {
	TcpiState        uint8
	TcpiCaState      uint8
	TcpiRetransmits  uint8
	TcpiProbes       uint8
	TcpiBackoff      uint8
	TcpiOptions      uint8
	TcpiSndWscale    uint8
	TcpiRcvWscale    uint8
	TcpiRto          uint32
	TcpiAto          uint32
	TcpiSndMss       uint32
	TcpiRcvMss       uint32
	TcpiUnacked       uint32
	TcpiSacked       uint32
	TcpiLost         uint32
	TcpiRetrans      uint32
	TcpiFackets      uint32
	TcpiLastDataSent uint32
	TcpiLastAckSent  uint32
	TcpiLastDataRecv uint32
	TcpiLastAckRecv  uint32
	TcpiPmtu         uint32
	TcpiRcvSsthresh  uint32
	TcpiRtt          uint32
	TcpiRttvar       uint32
	TcpiSndSsthresh  uint32
	TcpiSndCwnd      uint32
	TcpiAdvmss       uint32
	TcpiReordering    uint32
	TcpiRcvRtt       uint32
	TcpiRcvSpace     uint32
	TcpiTotalRetrans uint32
}

// TcpInfoResult contains the result of native TCP_INFO collection.
type TcpInfoResult struct {
	Available bool

	// IsSynthetic indicates this was collected from a synthetic connection
	// (e.g., a dial created just for diagnostics) vs the actual probe socket.
	IsSynthetic bool

	// State is the TCP connection state
	State string

	RTTUs            *int64
	RTTVarUs         *int64
	RetransmitsCurrent *int64
	RetransmitsTotal   *int64
	SndCwnd          *int32
	Ssthresh         *int32
	Unacked          *int64
	Lost             *int64
	Sacked           *int64
	Reordering       *int64

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

// GetTcpInfo retrieves TCP_INFO for a connected socket using SyscallConn.
// This requires an already-established TCP connection.
func GetTcpInfo(ctx context.Context, conn net.Conn) *TcpInfoResult {
	result := &TcpInfoResult{Available: false}

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		result.Error = &TcpInfoError{Kind: "not_tcp", Message: "not a TCP connection"}
		return result
	}

	// Use SyscallConn for raw fd access
	sysConn, err := tcpConn.SyscallConn()
	if err != nil {
		result.Error = &TcpInfoError{Kind: "syscall_conn_failed", Message: fmt.Sprintf("SyscallConn failed: %v", err)}
		return result
	}

	var info tcpInfoKernel
	infoSize := unsafe.Sizeof(info)

	err = sysConn.Read(func(fd uintptr) bool {
		err = unix.Getsockopt(int(fd), IPPROTO_TCP, TCP_INFO, (*byte)(unsafe.Pointer(&info)), &infoSize)
		return err == nil
	})

	if err != nil {
		if err == syscall.EOPNOTSUPP {
			result.Error = &TcpInfoError{Kind: "not_supported", Message: "TCP_INFO not supported"}
		} else {
			result.Error = &TcpInfoError{Kind: "getsockopt_failed", Message: fmt.Sprintf("getsockopt failed: %v", err)}
		}
		return result
	}

	result.Available = true
	result.State = tcpStateToString(info.TcpiState)

	// RTT is in microseconds on modern Linux kernels (2.6+)
	if info.TcpiRtt > 0 {
		rtt := int64(info.TcpiRtt)
		result.RTTUs = &rtt
	}
	if info.TcpiRttvar > 0 {
		rttvar := int64(info.TcpiRttvar)
		result.RTTVarUs = &rttvar
	}
	if info.TcpiRetrans > 0 {
		retrans := int64(info.TcpiRetrans)
		result.RetransmitsCurrent = &retrans
	}
	if info.TcpiTotalRetrans > 0 {
		total := int64(info.TcpiTotalRetrans)
		result.RetransmitsTotal = &total
	}
	if info.TcpiSndCwnd > 0 {
		cwnd := int32(info.TcpiSndCwnd)
		result.SndCwnd = &cwnd
	}
	if info.TcpiSndSsthresh > 0 {
		ssthresh := int32(info.TcpiSndSsthresh)
		result.Ssthresh = &ssthresh
	}
	if info.TcpiUnacked > 0 {
		unacked := int64(info.TcpiUnacked)
		result.Unacked = &unacked
	}
	if info.TcpiLost > 0 {
		lost := int64(info.TcpiLost)
		result.Lost = &lost
	}
	if info.TcpiSacked > 0 {
		sacked := int64(info.TcpiSacked)
		result.Sacked = &sacked
	}
	if info.TcpiReordering > 0 {
		reorder := int64(info.TcpiReordering)
		result.Reordering = &reorder
	}

	if conn.LocalAddr() != nil {
		result.LocalAddr = conn.LocalAddr().String()
	}
	if conn.RemoteAddr() != nil {
		result.RemoteAddr = conn.RemoteAddr().String()
	}

	return result
}

// GetTcpInfoFromSyntheticDial creates a synthetic TCP connection and collects TCP_INFO.
// This is explicitly labeled as synthetic since it measures a diagnostic-only connection,
// not the actual HTTP probe socket.
func GetTcpInfoFromSyntheticDial(ctx context.Context, address string) (*TcpInfoResult, net.Conn, error) {
	dialer := &net.Dialer{Timeout: 5 * time.Second}

	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return &TcpInfoResult{
			Available:   false,
			IsSynthetic: true,
			Error:      &TcpInfoError{Kind: "dial_failed", Message: fmt.Sprintf("dial failed: %v", err)},
		}, nil, err
	}

	time.Sleep(10 * time.Millisecond)

	result := GetTcpInfo(ctx, conn)
	if result != nil {
		result.IsSynthetic = true
	}
	if result != nil && !result.Available {
		conn.Close()
		return result, nil, fmt.Errorf(result.Error.Message)
	}

	return result, conn, nil
}

func tcpStateToString(state uint8) string {
	switch state {
	case tcpStateEstablished:
		return "ESTAB"
	case tcpStateSynSent:
		return "SYN-SENT"
	case tcpStateSynRecv:
		return "SYN-RECV"
	case tcpStateFinWait1:
		return "FIN-WAIT-1"
	case tcpStateFinWait2:
		return "FIN-WAIT-2"
	case tcpStateTimeWait:
		return "TIME-WAIT"
	case tcpStateClose:
		return "CLOSE"
	case tcpStateCloseWait:
		return "CLOSE-WAIT"
	case tcpStateLastAck:
		return "LAST-ACK"
	case tcpStateListen:
		return "LISTEN"
	case tcpStateClosing:
		return "CLOSING"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", state)
	}
}

// CollectTcpQualityFromConn collects TCP_INFO from an actual HTTP probe connection.
// This provides native_tcp_info evidence from the real probe socket.
//
// This function is intended for use with the connection obtained via httptrace.GotConn.
// The caller must NOT read, write, or close the connection - it is owned by the transport.
//
// Parameters:
//   - ctx: context for timeout/cancellation
//   - probeKind: the probe kind (e.g., "http")
//   - lookupTarget: the target host/IP being probed
//   - conn: the actual net.Conn from the HTTP probe
//
// Returns:
//   - *state.TcpQuality with source=native_tcp_info and matched_socket=true on success
//   - nil if conn is nil, non-TCP, closed, or TCP_INFO is unavailable
func CollectTcpQualityFromConn(ctx context.Context, probeKind string, lookupTarget string, conn net.Conn) *state.TcpQuality {
	if conn == nil {
		return nil
	}

	// GetTcpInfo sets IsSynthetic=false by default for actual connections
	result := GetTcpInfo(ctx, conn)
	if result == nil || !result.Available {
		return nil
	}

	// Ensure IsSynthetic is false for actual probe socket
	result.IsSynthetic = false

	return TcpInfoToTcpQuality(result, probeKind, lookupTarget)
}
