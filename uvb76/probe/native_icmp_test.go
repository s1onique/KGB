package probe

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// fakeICMPSocketOpener is a mock socket opener for testing.
type fakeICMPSocketOpener struct{ openErr error }

func (f *fakeICMPSocketOpener) OpenSocket() (NativeICMPPacketConn, error) {
	if f.openErr != nil {
		return nil, f.openErr
	}
	return nil, errors.New("fake opener not connected to real socket")
}

// fakeICMPPacketConn is a mock ICMP packet connection for testing.
type fakeICMPPacketConn struct {
	multiRead    []readResult
	multiReadIdx int
	readErr      error
	writeErr     error
	closed       bool
}

type readResult struct{ data []byte; addr net.Addr; err error }

func (f *fakeICMPPacketConn) ReadFrom(b []byte) (int, net.Addr, error) {
	if f.multiRead != nil && f.multiReadIdx < len(f.multiRead) {
		r := f.multiRead[f.multiReadIdx]
		f.multiReadIdx++
		if r.err != nil {
			return 0, nil, r.err
		}
		n := copy(b, r.data)
		return n, r.addr, nil
	}
	if f.readErr != nil {
		return 0, nil, f.readErr
	}
	return 0, nil, errors.New("no more data")
}

func (f *fakeICMPPacketConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(b), nil
}
func (f *fakeICMPPacketConn) Close() error                       { f.closed = true; return nil }
func (f *fakeICMPPacketConn) SetReadDeadline(time.Time) error   { return nil }

// capturingFakeICMPPacketConn captures write data to build matching replies.
type capturingFakeICMPPacketConn struct {
	fakeICMPPacketConn
	backendID uint16
	sendTime  time.Time
}

func (f *capturingFakeICMPPacketConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	msg, err := icmp.ParseMessage(1, b)
	if err != nil {
		return 0, err
	}
	echo, ok := msg.Body.(*icmp.Echo)
	if !ok {
		return 0, errors.New("not an echo message")
	}
	replyData, replyAddr, _ := buildTestEchoReply(f.backendID, uint16(echo.Seq), "127.0.0.1", f.sendTime.UnixNano())
	f.fakeICMPPacketConn.multiRead = []readResult{{data: replyData, addr: replyAddr}}
	return len(b), nil
}

// buildTestEchoReply builds a test echo reply with UDPAddr peer.
func buildTestEchoReply(id, seq uint16, sourceIP string, timestamp int64) ([]byte, net.Addr, error) {
	return buildTestEchoReplyWithPeerType(id, seq, sourceIP, timestamp, "udp")
}

// buildTestEchoReplyWithPeerType builds a test echo reply with configurable peer type.
func buildTestEchoReplyWithPeerType(id, seq uint16, sourceIP string, timestamp int64, peerType string) ([]byte, net.Addr, error) {
	body := &icmp.Echo{ID: int(id), Seq: int(seq), Data: int64ToBytes(timestamp)}
	msg := icmp.Message{Type: ipv4.ICMPTypeEchoReply, Code: 0, Body: body}
	data, err := msg.Marshal(nil)
	if err != nil {
		return nil, nil, err
	}
	ip := net.ParseIP(sourceIP)
	switch peerType {
	case "ipaddr":
		return data, &net.IPAddr{IP: ip}, nil
	default:
		return data, &net.UDPAddr{IP: ip}, nil
	}
}

func TestNativeICMPBackendBuildsEchoRequest(t *testing.T) {
	backend := &NativeICMPBackend{backendID: 42}
	packet, err := backend.buildEchoRequest(12345)
	if err != nil {
		t.Fatalf("buildEchoRequest failed: %v", err)
	}
	msg, err := icmp.ParseMessage(1, packet)
	if err != nil {
		t.Fatalf("failed to parse ICMP message: %v", err)
	}
	if msg.Type != ipv4.ICMPTypeEcho {
		t.Errorf("expected ICMPTypeEcho, got %v", msg.Type)
	}
	echo := msg.Body.(*icmp.Echo)
	if uint16(echo.ID) != backend.backendID {
		t.Errorf("expected ID %d, got %d", backend.backendID, echo.ID)
	}
	if uint16(echo.Seq) != 12345 {
		t.Errorf("expected sequence 12345, got %d", echo.Seq)
	}
}

func TestNativeICMPBackendMatchesExpectedEchoReply(t *testing.T) {
	backend := &NativeICMPBackend{backendID: 42}
	replyData, addr, _ := buildTestEchoReply(42, 123, "192.168.1.1", time.Now().UnixNano())
	rtt, matched, err := backend.matchEchoReply(replyData, addr, net.ParseIP("192.168.1.1"), 123)
	if err != nil {
		t.Fatalf("matchEchoReply failed: %v", err)
	}
	if !matched {
		t.Error("expected reply to match but it did not")
	}
	if rtt < 0 || rtt > 100*time.Millisecond {
		t.Errorf("unexpected RTT: %v", rtt)
	}
}

func TestNativeICMPBackendRejectsUnmatchedReply(t *testing.T) {
	backend := &NativeICMPBackend{backendID: 42}
	sendTime := time.Now().UnixNano()

	tests := []struct {
		name string
		id   uint16
		seq  uint16
		ip   string
	}{
		{"wrong ID", 99, 123, "192.168.1.1"},
		{"wrong sequence", 42, 999, "192.168.1.1"},
		{"wrong source IP", 42, 123, "10.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, addr, _ := buildTestEchoReply(tt.id, tt.seq, tt.ip, sendTime)
			_, matched, _ := backend.matchEchoReply(data, addr, net.ParseIP("192.168.1.1"), 123)
			if matched {
				t.Errorf("expected reply to NOT match for %s", tt.name)
			}
		})
	}
}

func TestNativeICMPPingWithFakeConnection(t *testing.T) {
	backendID := uint16(42)
	fakeConn := &capturingFakeICMPPacketConn{backendID: backendID, sendTime: time.Now()}
	backend := &NativeICMPBackend{opener: &fakeICMPSocketOpener{}, conn: fakeConn, stats: &nativeICMPStatsInternal{}, backendID: backendID}

	latency, err := backend.Ping(context.Background(), "127.0.0.1", 3*time.Second)
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
	if latency < 0 {
		t.Errorf("unexpected negative latency: %v", latency)
	}
	stats := backend.stats.Snapshot()
	if stats.Sent != 1 || stats.Received != 1 || stats.Timeouts != 0 {
		t.Errorf("unexpected stats: sent=%d received=%d timeouts=%d", stats.Sent, stats.Received, stats.Timeouts)
	}
}

func TestNativeICMPPingTimeout(t *testing.T) {
	fakeConn := &fakeICMPPacketConn{readErr: &net.DNSError{Err: "timeout", IsTimeout: true}}
	backend := &NativeICMPBackend{opener: &fakeICMPSocketOpener{}, conn: fakeConn, stats: &nativeICMPStatsInternal{}, backendID: 42}

	_, err := backend.Ping(context.Background(), "127.0.0.1", 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error for timeout")
	}
	var icmpErr *NativeICMPError
	if !errors.As(err, &icmpErr) {
		t.Fatalf("expected NativeICMPError, got %T", err)
	}
	if icmpErr.ErrorClass != ErrClassTimeout {
		t.Errorf("expected error class timeout, got %v", icmpErr.ErrorClass)
	}
	if stats := backend.stats.Snapshot(); stats.Sent != 1 || stats.Timeouts != 1 {
		t.Errorf("unexpected stats: sent=%d timeouts=%d", stats.Sent, stats.Timeouts)
	}
}

func TestNativeICMPMatchEchoReplyUnmatchedContinuesWaiting(t *testing.T) {
	backend := &NativeICMPBackend{backendID: 42, stats: &nativeICMPStatsInternal{}}
	sendTime := time.Now().UnixNano()

	replyData, addr, _ := buildTestEchoReply(99, 123, "127.0.0.1", sendTime)
	_, matched, _ := backend.matchEchoReply(replyData, addr, net.ParseIP("127.0.0.1"), 123)
	if matched {
		t.Error("expected reply to NOT match (wrong ID)")
	}

	replyData2, addr2, _ := buildTestEchoReply(42, 999, "127.0.0.1", sendTime)
	_, matched2, _ := backend.matchEchoReply(replyData2, addr2, net.ParseIP("127.0.0.1"), 123)
	if matched2 {
		t.Error("expected reply to NOT match (wrong sequence)")
	}

	replyData3, addr3, _ := buildTestEchoReply(42, 123, "10.0.0.1", sendTime)
	_, matched3, _ := backend.matchEchoReply(replyData3, addr3, net.ParseIP("127.0.0.1"), 123)
	if matched3 {
		t.Error("expected reply to NOT match (wrong source IP)")
	}
}

func TestNativeICMPPingHostUnreachable(t *testing.T) {
	backend := &NativeICMPBackend{opener: &fakeICMPSocketOpener{}, conn: nil, stats: &nativeICMPStatsInternal{}, backendID: 42}
	_, err := backend.Ping(context.Background(), "nonexistent.invalid", 3*time.Second)
	if err == nil {
		t.Fatal("expected error for unresolved host")
	}
	var icmpErr *NativeICMPError
	if !errors.As(err, &icmpErr) {
		t.Fatalf("expected NativeICMPError, got %T", err)
	}
	if icmpErr.ErrorClass != ErrClassUnreachable {
		t.Errorf("expected error class unreachable, got %v", icmpErr.ErrorClass)
	}
}

func TestICMPBackendDoesNotSilentlyFallback(t *testing.T) {
	permErr := errors.New("operation not permitted")
	fakeOpener := &fakeICMPSocketOpener{openErr: permErr}
	backend, err := NewNativeICMPBackendWithOpener(fakeOpener)
	if err == nil {
		if backend != nil {
			backend.Close()
		}
		t.Fatal("expected error creating backend with failing opener")
	}
	if !errors.Is(err, permErr) {
		t.Errorf("expected permission error, got %v", err)
	}
}

func TestNativeICMPBackendClose(t *testing.T) {
	fakeConn := &fakeICMPPacketConn{}
	backend := &NativeICMPBackend{stats: &nativeICMPStatsInternal{}, backendID: 42, conn: fakeConn}
	if err := backend.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
	if !fakeConn.closed {
		t.Error("expected conn.Close() to be called")
	}
}

func TestNativeICMPMatchEchoReplyMatchesRawIPAddrPeer(t *testing.T) {
	// Raw SOCK_RAW sockets return *net.IPAddr peers, not *net.UDPAddr.
	// This test ensures matchEchoReply handles *net.IPAddr correctly.
	backend := &NativeICMPBackend{backendID: 42}
	replyData, addr, _ := buildTestEchoReplyWithPeerType(42, 123, "192.168.1.1", time.Now().UnixNano(), "ipaddr")

	// Verify the peer is actually an IPAddr
	if _, ok := addr.(*net.IPAddr); !ok {
		t.Fatalf("expected *net.IPAddr peer, got %T", addr)
	}

	rtt, matched, err := backend.matchEchoReply(replyData, addr, net.ParseIP("192.168.1.1"), 123)
	if err != nil {
		t.Fatalf("matchEchoReply failed: %v", err)
	}
	if !matched {
		t.Error("expected reply to match with *net.IPAddr peer but it did not")
	}
	if rtt < 0 || rtt > 100*time.Millisecond {
		t.Errorf("unexpected RTT: %v", rtt)
	}
}

func TestNativeICMPMatchEchoReplyRejectsWrongRawIPAddrPeer(t *testing.T) {
	// Ensure wrong IP in *net.IPAddr peer is rejected.
	backend := &NativeICMPBackend{backendID: 42}
	replyData, addr, _ := buildTestEchoReplyWithPeerType(42, 123, "10.0.0.1", time.Now().UnixNano(), "ipaddr")

	if _, ok := addr.(*net.IPAddr); !ok {
		t.Fatalf("expected *net.IPAddr peer, got %T", addr)
	}

	_, matched, _ := backend.matchEchoReply(replyData, addr, net.ParseIP("192.168.1.1"), 123)
	if matched {
		t.Error("expected reply to NOT match with wrong IP in *net.IPAddr peer")
	}
}

func TestIcmpPeerIPHelper(t *testing.T) {
	// Test icmpPeerIP helper handles both UDPAddr and IPAddr.
	tests := []struct {
		name     string
		peer     net.Addr
		wantNil  bool
		wantIP   string
	}{
		{
			name:    "UDPAddr peer",
			peer:    &net.UDPAddr{IP: net.ParseIP("192.168.1.1")},
			wantNil: false,
			wantIP:  "192.168.1.1",
		},
		{
			name:    "IPAddr peer",
			peer:    &net.IPAddr{IP: net.ParseIP("10.0.0.5")},
			wantNil: false,
			wantIP:  "10.0.0.5",
		},
		{
			name:    "nil peer",
			peer:    nil,
			wantNil: true,
		},
		{
			name:    "unexpected type",
			peer:    &net.TCPAddr{IP: net.ParseIP("1.2.3.4")},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := icmpPeerIP(tt.peer)
			if tt.wantNil && got != nil {
				t.Errorf("icmpPeerIP() = %v, want nil", got)
			}
			if !tt.wantNil && got == nil {
				t.Errorf("icmpPeerIP() = nil, want non-nil")
			}
			if !tt.wantNil && !got.Equal(net.ParseIP(tt.wantIP)) {
				t.Errorf("icmpPeerIP() = %v, want %v", got, net.ParseIP(tt.wantIP))
			}
		})
	}
}

func TestPingGroupRangeHelper(t *testing.T) {
	// Test pingGroupRange reads the kernel setting.
	// On non-Linux this returns empty string.
	result := pingGroupRange()
	t.Logf("pingGroupRange() = %q", result)
	// Just verify it doesn't panic and returns a string.
	// On macOS/Windows this will be empty; on Linux it should have a value like "1 0" or "0 4294967295".
}
