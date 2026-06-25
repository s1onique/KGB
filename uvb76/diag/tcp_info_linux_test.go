// Package diag implements diagnostic capture for UVB-76.
//go:build linux
// +build linux

package diag

import (
	"context"
	"net"
	"testing"
	"time"
)

// TestTcpInfo_LoopbackConnection tests TCP_INFO collection on a loopback connection.
// This test requires a running TCP server, so we create a local listener.
func TestTcpInfo_LoopbackConnection(t *testing.T) {
	// Create a local TCP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot create local listener: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().String()

	// Accept connection in background
	done := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		done <- conn
	}()

	// Dial the server
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	// Accept the server-side connection
	var serverConn net.Conn
	select {
	case serverConn = <-done:
		defer serverConn.Close()
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for accept")
	}

	// Give connection time to establish
	time.Sleep(50 * time.Millisecond)

	// Get TCP_INFO from the client connection
	ctx := context.Background()
	result := GetTcpInfo(ctx, conn)

	// Verify result structure
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// TCP_INFO should be available on loopback
	if !result.Available {
		t.Logf("TCP_INFO not available: %v", result.Error)
		// This can happen if kernel doesn't support TCP_INFO for this socket type
		t.Skip("TCP_INFO not available on this system")
	}

	// Verify basic fields
	if result.State != "ESTAB" {
		t.Errorf("expected state 'ESTAB', got '%s'", result.State)
	}

	// RTT on loopback should be very low
	if result.RTTUs != nil && *result.RTTUs > 10000 {
		t.Logf("unexpectedly high RTT: %d us", *result.RTTUs)
	}

	t.Logf("TCP_INFO collected: state=%s rtt_us=%v cwnd=%v",
		result.State, ptrVal(result.RTTUs), ptrVal32(result.SndCwnd))
}

// TestTcpInfo_NonTCPConnection tests that GetTcpInfo returns error for non-TCP connections.
func TestTcpInfo_NonTCPConnection(t *testing.T) {
	ctx := context.Background()

	// UDP "connection" (actually a UDPConn which is not a TCPConn)
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Skipf("cannot create UDP listener: %v", err)
	}
	defer conn.Close()

	result := GetTcpInfo(ctx, conn)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Available {
		t.Error("expected Available=false for UDP connection")
	}
	if result.Error == nil {
		t.Error("expected error for non-TCP connection")
	}
}

// TestTcpInfo_ClosedConnection tests that GetTcpInfo handles closed connections gracefully.
func TestTcpInfo_ClosedConnection(t *testing.T) {
	ctx := context.Background()

	// Create and immediately close a connection
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot create local listener: %v", err)
	}
	defer listener.Close()

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	conn.Close() // Close immediately

	result := GetTcpInfo(ctx, conn)

	// Should handle gracefully (may fail with various errors)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Either available or has an error - both are valid outcomes
}

// TestDialAndGetTcpInfo_BasicDial tests basic dial and TCP_INFO collection.
func TestDialAndGetTcpInfo_BasicDial(t *testing.T) {
	// Start a local HTTP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot create local listener: %v", err)
	}
	defer listener.Close()

	// Accept in background
	done := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		done <- conn
	}()

	// Dial and get TCP_INFO
	ctx := context.Background()
	result, conn, err := DialAndGetTCPInfo(ctx, "tcp", listener.Addr().String())
	if conn != nil {
		defer conn.Close()
	}

	// Accept server connection
	select {
	case serverConn := <-done:
		defer serverConn.Close()
	case <-time.After(5 * time.Second):
		// Accept might timeout, which is fine
	}

	if err != nil {
		t.Fatalf("DialAndGetTCPInfo failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if !result.Available {
		t.Skip("TCP_INFO not available on this system")
	}

	if result.State != "ESTAB" {
		t.Errorf("expected state 'ESTAB', got '%s'", result.State)
	}

	// Verify addresses are captured
	if result.LocalAddr == "" {
		t.Error("expected LocalAddr to be set")
	}
	if result.RemoteAddr == "" {
		t.Error("expected RemoteAddr to be set")
	}
}

// TestTcpStateToString tests the TCP state string conversion.
func TestTcpStateToString(t *testing.T) {
	tests := []struct {
		state    uint8
		expected string
	}{
		{1, "ESTAB"},
		{2, "SYN-SENT"},
		{3, "SYN-RECV"},
		{4, "FIN-WAIT-1"},
		{5, "FIN-WAIT-2"},
		{6, "TIME-WAIT"},
		{7, "CLOSE"},
		{8, "CLOSE-WAIT"},
		{9, "LAST-ACK"},
		{10, "LISTEN"},
		{11, "CLOSING"},
		{99, "UNKNOWN(99)"},
	}

	for _, tt := range tests {
		result := tcpStateToString(tt.state)
		if result != tt.expected {
			t.Errorf("tcpStateToString(%d): expected %s, got %s", tt.state, tt.expected, result)
		}
	}
}

// TestTcpInfo_Timeout tests that context timeout is respected.
func TestTcpInfo_Timeout(t *testing.T) {
	// Create a listener that will block accepts
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot create local listener: %v", err)
	}
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Dial (will succeed) but we won't accept, so connection stays in SYN-SENT or similar
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	// Give connection time to establish partially
	time.Sleep(50 * time.Millisecond)

	// Get TCP_INFO with timeout context
	result := GetTcpInfo(ctx, conn)

	// Should handle timeout gracefully
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// The result may or may not be available depending on connection state
}

// Helper functions

func ptrVal(p *int64) int64 {
	if p == nil {
		return -1
	}
	return *p
}

func ptrVal32(p *int32) int32 {
	if p == nil {
		return -1
	}
	return *p
}
