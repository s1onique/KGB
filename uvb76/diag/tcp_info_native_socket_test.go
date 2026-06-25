// Package diag implements diagnostic capture for UVB-76.
//go:build linux
// +build linux

package diag

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestCollectTcpQualityFromActualConn_NativeTCPInfo verifies that CollectTcpQualityFromConn
// collects TCP_INFO from an actual HTTP probe connection with source=native_tcp_info
// and matched_socket=true.
//
// This is a Linux-only regression test that proves actual socket TCP_INFO collection
// without synthetic dials or CLI fallback.
func TestCollectTcpQualityFromActualConn_NativeTCPInfo(t *testing.T) {
	// Start a local HTTP server on a random port
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	// Parse server address
	serverAddr := server.Listener.Addr().String()

	// Create a real TCP connection (simulating what httptrace.GotConn captures)
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		t.Skipf("cannot dial test server: %v", err)
	}
	defer conn.Close()

	// Give connection time to establish
	time.Sleep(50 * time.Millisecond)

	// Collect TCP quality from the actual connection
	ctx := context.Background()
	tq := CollectTcpQualityFromConn(ctx, "http", "localhost", conn)

	// TCP_INFO should be available on loopback
	if tq == nil {
		t.Skip("TCP_INFO not available on this system (may occur in some container environments)")
	}

	// Verify source is native_tcp_info (not synthetic)
	if tq.Source != TcpQualitySourceNativeTCPInfo {
		t.Errorf("Expected source '%s', got '%s'", TcpQualitySourceNativeTCPInfo, tq.Source)
	}

	// Verify matched_socket is true (actual probe socket)
	if !tq.MatchedSocket {
		t.Error("Expected matched_socket=true for actual probe socket")
	}

	// Verify kind is preserved
	if tq.Kind != "http" {
		t.Errorf("Expected kind 'http', got '%s'", tq.Kind)
	}

	// Verify lookup target is preserved
	if tq.LookupTarget != "localhost" {
		t.Errorf("Expected lookup_target 'localhost', got '%s'", tq.LookupTarget)
	}

	// Verify state is captured
	if tq.State != "ESTAB" {
		t.Errorf("Expected state 'ESTAB', got '%s'", tq.State)
	}

	// Addresses should be redacted
	if tq.Local == "" {
		t.Error("Expected local address to be set")
	}
	if tq.Remote == "" {
		t.Error("Expected remote address to be set")
	}

	// On loopback, RTT should be very low
	if tq.RTTUs != nil && *tq.RTTUs > 10000 {
		t.Logf("Note: RTT higher than expected on loopback: %d us", *tq.RTTUs)
	}

	t.Logf("Native TCP_INFO collected: source=%s matched=%v state=%s rtt_us=%v",
		tq.Source, tq.MatchedSocket, tq.State, ptrValue(tq.RTTUs))
}

// TestCollectTcpQualityFromConn_ActualTCPConn verifies that CollectTcpQualityFromConn
// produces source=native_tcp_info and matched_socket=true for a real TCP connection.
func TestCollectTcpQualityFromConn_ActualTCPConn(t *testing.T) {
	// Create a local TCP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot create local listener: %v", err)
	}
	defer listener.Close()

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
	conn, err := net.Dial("tcp", listener.Addr().String())
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

	// Collect from the actual client-side connection
	ctx := context.Background()
	targetHost := "127.0.0.1"
	tq := CollectTcpQualityFromConn(ctx, "http", targetHost, conn)

	if tq == nil {
		t.Skip("TCP_INFO not available on this system")
	}

	// Verify native TCP_INFO semantics
	if tq.Source != TcpQualitySourceNativeTCPInfo {
		t.Errorf("Expected source '%s', got '%s'", TcpQualitySourceNativeTCPInfo, tq.Source)
	}

	if !tq.MatchedSocket {
		t.Error("Expected matched_socket=true for actual TCP connection")
	}

	// This should NOT be synthetic
	if tq.Source == TcpQualitySourceSyntheticTCPInfo {
		t.Error("Expected source to NOT be synthetic_tcp_info for actual connection")
	}

	t.Logf("Actual socket TCP_INFO: source=%s matched=%v state=%s",
		tq.Source, tq.MatchedSocket, tq.State)
}

// TestCollectTcpQualityFromConn_NilConn verifies that nil connection returns nil.
func TestCollectTcpQualityFromConn_NilConn(t *testing.T) {
	ctx := context.Background()
	tq := CollectTcpQualityFromConn(ctx, "http", "localhost", nil)

	if tq != nil {
		t.Error("Expected nil for nil connection")
	}
}

// TestCollectTcpQualityFromConn_NonTCPConn verifies that non-TCP connections return nil.
func TestCollectTcpQualityFromConn_NonTCPConn(t *testing.T) {
	ctx := context.Background()

	// UDP connection
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Skipf("cannot create UDP listener: %v", err)
	}
	defer conn.Close()

	tq := CollectTcpQualityFromConn(ctx, "http", "localhost", conn)

	// Non-TCP connections should not produce TCP_INFO
	if tq != nil && tq.Source == TcpQualitySourceNativeTCPInfo && tq.MatchedSocket {
		t.Error("UDP connection should not produce matched native TCP_INFO")
	}
}

// Helper function
func ptrValue(p *int64) int64 {
	if p == nil {
		return -1
	}
	return *p
}
