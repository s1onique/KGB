package probe

import (
	"context"
	"net"
	"testing"
	"time"
)

// ============================================================================
// ICMP Address Type Tests
// ============================================================================
// These tests verify that WriteTo uses the correct address type based on socket mode:
// - Raw ICMP ("ip4:icmp") requires *net.IPAddr
// - Datagram ICMP ("udp4") requires *net.UDPAddr

func TestICMPWriteAddrUsesUDPAddrForDgram(t *testing.T) {
	// Datagram ICMP ("udp4") requires *net.UDPAddr for WriteTo.
	addr := icmpWriteAddr(SocketModeDgramICMP, net.ParseIP("192.168.1.1"))
	if _, ok := addr.(*net.UDPAddr); !ok {
		t.Fatalf("dgram mode WriteTo addr = %T, want *net.UDPAddr", addr)
	}
	udpAddr := addr.(*net.UDPAddr)
	if !udpAddr.IP.Equal(net.ParseIP("192.168.1.1")) {
		t.Errorf("dgram mode addr IP = %v, want 192.168.1.1", udpAddr.IP)
	}
}

func TestICMPWriteAddrUsesIPAddrForRaw(t *testing.T) {
	// Raw ICMP ("ip4:icmp") requires *net.IPAddr for WriteTo.
	addr := icmpWriteAddr(SocketModeRawICMP, net.ParseIP("10.0.0.1"))
	if _, ok := addr.(*net.IPAddr); !ok {
		t.Fatalf("raw mode WriteTo addr = %T, want *net.IPAddr", addr)
	}
	ipAddr := addr.(*net.IPAddr)
	if !ipAddr.IP.Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("raw mode addr IP = %v, want 10.0.0.1", ipAddr.IP)
	}
}

func TestICMPWriteAddrDefaultsToUDPAddr(t *testing.T) {
	// Unknown socket mode defaults to *net.UDPAddr (safe fallback).
	addr := icmpWriteAddr(SocketModeUnknown, net.ParseIP("1.2.3.4"))
	if _, ok := addr.(*net.UDPAddr); !ok {
		t.Fatalf("unknown mode WriteTo addr = %T, want *net.UDPAddr (default)", addr)
	}
}

// recordingFakeICMPPacketConn records the last address passed to WriteTo.
type recordingFakeICMPPacketConn struct {
	fakeICMPPacketConn
	lastWriteAddr net.Addr
}

func (f *recordingFakeICMPPacketConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	f.lastWriteAddr = addr
	return len(b), nil
}

func TestNativeICMPPingRawModeWritesToIPAddr(t *testing.T) {
	// When socket mode is RawICMP, WriteTo must receive *net.IPAddr.
	backendID := uint16(42)
	recorder := &recordingFakeICMPPacketConn{}
	backend := &NativeICMPBackend{
		opener:     &fakeICMPSocketOpener{},
		conn:       recorder,
		socketMode: SocketModeRawICMP, // Raw mode requires *net.IPAddr
		stats:      &nativeICMPStatsInternal{},
		backendID:  backendID,
	}

	// Execute Ping - it should call WriteTo with *net.IPAddr for raw mode
	_, _ = backend.Ping(context.Background(), "127.0.0.1", 3*time.Second)
	// We may get an error if no reply is received, but WriteTo should have been called
	// with the correct address type
	if recorder.lastWriteAddr == nil {
		t.Fatal("WriteTo was never called")
	}
	if _, ok := recorder.lastWriteAddr.(*net.IPAddr); !ok {
		t.Fatalf("raw mode WriteTo addr = %T, want *net.IPAddr", recorder.lastWriteAddr)
	}
}

func TestNativeICMPPingDgramModeWritesToUDPAddr(t *testing.T) {
	// When socket mode is DgramICMP, WriteTo must receive *net.UDPAddr.
	backendID := uint16(42)
	recorder := &recordingFakeICMPPacketConn{}
	backend := &NativeICMPBackend{
		opener:     &fakeICMPSocketOpener{},
		conn:       recorder,
		socketMode: SocketModeDgramICMP, // Datagram mode requires *net.UDPAddr
		stats:      &nativeICMPStatsInternal{},
		backendID:  backendID,
	}

	// Execute Ping - it should call WriteTo with *net.UDPAddr for dgram mode
	_, _ = backend.Ping(context.Background(), "127.0.0.1", 3*time.Second)
	// We may get an error if no reply is received, but WriteTo should have been called
	// with the correct address type
	if recorder.lastWriteAddr == nil {
		t.Fatal("WriteTo was never called")
	}
	if _, ok := recorder.lastWriteAddr.(*net.UDPAddr); !ok {
		t.Fatalf("dgram mode WriteTo addr = %T, want *net.UDPAddr", recorder.lastWriteAddr)
	}
}
