// Package probe implements independent latency probing for UVB-76.
package probe

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"golang.org/x/net/icmp"
)

// NativeICMPSocketMode represents the type of ICMP socket being used.
type NativeICMPSocketMode string

const (
	// SocketModeDgramICMP uses SOCK_DGRAM/IPPROTO_ICMP (unprivileged on Linux with ping_group_range).
	SocketModeDgramICMP NativeICMPSocketMode = "dgram_icmp"
	// SocketModeRawICMP uses SOCK_RAW/IPPROTO_ICMP (requires CAP_NET_RAW or root).
	SocketModeRawICMP NativeICMPSocketMode = "raw_icmp"
	// SocketModeUnknown indicates socket mode has not been determined.
	SocketModeUnknown NativeICMPSocketMode = "unknown"
)

// SocketOpenResult contains the result of socket open attempts.
type SocketOpenResult struct {
	Conn        NativeICMPPacketConn
	SocketMode NativeICMPSocketMode
	DgramError string // error from datagram socket attempt, if any
	RawError   string // error from raw socket attempt, if any
}

// realICMPSocketOpener opens real ICMP sockets using the system.
// It tries SOCK_DGRAM first (unprivileged), then falls back to SOCK_RAW (privileged).
type realICMPSocketOpener struct{}

// OpenSocket creates a native ICMP socket for IPv4.
// Tries SOCK_DGRAM first (unprivileged), falls back to SOCK_RAW (privileged).
// This handles routers like RT-AX88U where ping_group_range=1 0 disables
// unprivileged ICMP, but privileged raw sockets work.
func (r *realICMPSocketOpener) OpenSocket() (NativeICMPPacketConn, error) {
	result := r.tryOpenSocket()

	// Store the result for status reporting
	lastSocketOpenResult.Store(result)

	if result.Conn != nil {
		return result.Conn, nil
	}

	// Both socket types failed - return a descriptive error
	if result.DgramError != "" && result.RawError != "" {
		return nil, fmt.Errorf("native ICMP unavailable: dgram failed: %s; raw failed: %s", result.DgramError, result.RawError)
	}
	if result.DgramError != "" {
		return nil, fmt.Errorf("native ICMP unavailable: %s", result.DgramError)
	}
	if result.RawError != "" {
		return nil, fmt.Errorf("native ICMP unavailable: %s", result.RawError)
	}
	return nil, errors.New("native ICMP unavailable: unknown error")
}

// tryOpenSocket attempts to open both datagram and raw ICMP sockets.
// Returns the first successful socket, or the result with both errors if both fail.
func (r *realICMPSocketOpener) tryOpenSocket() *SocketOpenResult {
	result := &SocketOpenResult{}

	// Try datagram ICMP first (unprivileged on Linux with proper ping_group_range)
	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err == nil {
		result.Conn = conn
		result.SocketMode = SocketModeDgramICMP
		return result
	}

	// Record datagram error (EACCES/EPERM expected on RT-AX88U with ping_group_range=1 0)
	result.DgramError = extractSocketError(err)

	// Check if it's a permission error we might recover from with raw socket
	if !isPermissionError(err) {
		// Non-permission error - raw socket won't help
		return result
	}

	// Try raw ICMP (requires CAP_NET_RAW or root)
	conn, err = icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err == nil {
		result.Conn = conn
		result.SocketMode = SocketModeRawICMP
		return result
	}

	// Record raw error
	result.RawError = extractSocketError(err)

	return result
}

// extractSocketError extracts a user-friendly error message from socket errors.
func extractSocketError(err error) string {
	if err == nil {
		return ""
	}
	errStr := err.Error()
	// Extract syscall error code if present
	if os.IsPermission(err) {
		return "EACCES (permission denied)"
	}
	if strings.Contains(errStr, "operation not permitted") || strings.Contains(errStr, "EPERM") {
		return "EPERM (operation not permitted)"
	}
	if strings.Contains(errStr, "EACCES") {
		return "EACCES (permission denied)"
	}
	return errStr
}

// lastSocketOpenResult stores the result of the most recent socket open attempt.
// Used for status reporting.
var lastSocketOpenResult atomic.Value

func init() {
	lastSocketOpenResult.Store((*SocketOpenResult)(nil))
}

// GetLastSocketOpenResult returns the most recent socket open result for status reporting.
func GetLastSocketOpenResult() *SocketOpenResult {
	if v := lastSocketOpenResult.Load(); v != nil {
		return v.(*SocketOpenResult)
	}
	return nil
}
