// Package probe implements independent latency probing for UVB-76.
package probe

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// NativeICMPPacketConn is the interface for ICMP packet connections.
// This allows for both real (*icmp.PacketConn) and fake connections in tests.
type NativeICMPPacketConn interface {
	ReadFrom(b []byte) (n int, addr net.Addr, err error)
	WriteTo(b []byte, addr net.Addr) (n int, err error)
	SetReadDeadline(t time.Time) error
	Close() error
}

// NativeICMPSocketOpener is the interface for opening ICMP sockets.
type NativeICMPSocketOpener interface {
	OpenSocket() (NativeICMPPacketConn, error)
}

// NativeICMPBackend implements ICMPProbeBackend using native Go ICMP sockets.
// This avoids per-second os/exec ping execution on constrained routers,
// which has caused SIGSEGV on ASUS RT-AX88U / linux arm64.
type NativeICMPBackend struct {
	opener    NativeICMPSocketOpener
	conn      NativeICMPPacketConn
	mu        sync.Mutex
	stats     *nativeICMPStatsInternal
	backendID uint16 // unique ID for this backend instance to match Echo Replies
}

// NewNativeICMPBackend creates a new native ICMP backend.
// Returns an error if the ICMP socket cannot be opened (e.g., permission denied).
func NewNativeICMPBackend() (*NativeICMPBackend, error) {
	return NewNativeICMPBackendWithOpener(&realICMPSocketOpener{})
}

// NewNativeICMPBackendWithOpener creates a native ICMP backend with a custom socket opener.
// This exists for testing with fake socket openers.
func NewNativeICMPBackendWithOpener(opener NativeICMPSocketOpener) (*NativeICMPBackend, error) {
	conn, err := opener.OpenSocket()
	if err != nil {
		return nil, err
	}

	backendID := generateBackendID()

	return &NativeICMPBackend{
		opener:    opener,
		conn:      conn,
		stats:     &nativeICMPStatsInternal{},
		backendID: backendID,
	}, nil
}

// Close closes the ICMP socket.
func (b *NativeICMPBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn != nil {
		return b.conn.Close()
	}
	return nil
}

// Stats returns a snapshot of the backend statistics.
func (b *NativeICMPBackend) Stats() NativeICMPStatsRecorder {
	return b.stats
}

// Ping implements ICMPProbeBackend by sending an ICMP Echo Request and waiting for Reply.
func (b *NativeICMPBackend) Ping(ctx context.Context, host string, timeout time.Duration) (time.Duration, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ip, err := net.ResolveIPAddr("ip4", host)
	if err != nil {
		b.setErrorClass(ErrClassUnreachable)
		return 0, NewNativeICMPError(
			ErrClassUnreachable,
			fmt.Errorf("resolve host %q: %w", host, err),
			fmt.Sprintf("failed to resolve %s; check DNS or use IP address", host),
		)
	}

	targetIP := ip.IP.To4()
	if targetIP == nil {
		b.setErrorClass(ErrClassOther)
		return 0, NewNativeICMPError(
			ErrClassOther,
			fmt.Errorf("not an IPv4 address: %s", ip.IP),
			"native ICMP only supports IPv4 targets",
		)
	}

	sendTime := time.Now()
	sequence := uint16(sendTime.UnixNano() % 0xFFFF)
	msg, err := b.buildEchoRequest(sequence)
	if err != nil {
		b.stats.parseErrors.Add(1)
		b.setErrorClass(ErrClassParse)
		return 0, NewNativeICMPError(
			ErrClassParse,
			fmt.Errorf("build echo request: %w", err),
			"failed to build ICMP packet; internal error",
		)
	}

	_, err = b.conn.WriteTo(msg, &net.UDPAddr{IP: targetIP, Zone: ""})
	if err != nil {
		b.classifyAndRecordSocketError(err)
		return 0, NewNativeICMPError(
			b.getLastErrorClass(),
			fmt.Errorf("send echo: %w", err),
			b.getUserMessageForError(err),
		)
	}

	b.stats.sent.Add(1)
	replyBuf := make([]byte, 512)
	deadline := sendTime.Add(timeout)

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			b.stats.timeouts.Add(1)
			b.setErrorClass(ErrClassTimeout)
			return 0, NewNativeICMPError(
				ErrClassTimeout,
				fmt.Errorf("timeout waiting for echo reply from %s", targetIP),
				"native ICMP timeout; target may be unreachable or latency exceeds timeout",
			)
		}

		if err := b.conn.SetReadDeadline(deadline); err != nil {
			b.classifyAndRecordSocketError(err)
			return 0, NewNativeICMPError(
				b.getLastErrorClass(),
				fmt.Errorf("set read deadline: %w", err),
				"failed to set socket read deadline",
			)
		}

		n, peer, err := b.conn.ReadFrom(replyBuf)
		if err != nil {
			select {
			case <-ctx.Done():
				b.setErrorClass(ErrClassCanceled)
				return 0, NewNativeICMPError(
					ErrClassCanceled,
					ctx.Err(),
					"context canceled; probe interrupted",
				)
			default:
			}

			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				b.stats.timeouts.Add(1)
				b.setErrorClass(ErrClassTimeout)
				return 0, NewNativeICMPError(
					ErrClassTimeout,
					fmt.Errorf("timeout waiting for echo reply from %s", targetIP),
					"native ICMP timeout; target may be unreachable or latency exceeds timeout",
				)
			}

			b.classifyAndRecordSocketError(err)
			return 0, NewNativeICMPError(
				b.getLastErrorClass(),
				fmt.Errorf("read reply: %w", err),
				b.getUserMessageForError(err),
			)
		}

		rtt, matched, matchErr := b.matchEchoReply(replyBuf[:n], peer, targetIP, sequence)
		if matchErr != nil {
			b.stats.parseErrors.Add(1)
			b.setErrorClass(ErrClassParse)
			return 0, NewNativeICMPError(
				ErrClassParse,
				fmt.Errorf("parse reply: %w", matchErr),
				"failed to parse ICMP reply; possible network issue",
			)
		}

		if matched {
			b.stats.received.Add(1)
			b.stats.lastRTTMillis.Store(int64(rtt.Milliseconds()))
			return rtt, nil
		}

		b.stats.unmatchedReplies.Add(1)
	}
}

func (b *NativeICMPBackend) buildEchoRequest(sequence uint16) ([]byte, error) {
	body := struct {
		Identifier uint16
		SeqNum     uint16
		Timestamp  int64
	}{
		Identifier: b.backendID,
		SeqNum:     sequence,
		Timestamp:  time.Now().UnixNano(),
	}

	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   int(body.Identifier),
			Seq:  int(body.SeqNum),
			Data: int64ToBytes(body.Timestamp),
		},
	}

	msgBytes, err := msg.Marshal(nil)
	if err != nil {
		return nil, fmt.Errorf("marshal message: %w", err)
	}

	return msgBytes, nil
}

// icmpPeerIP extracts the IP address from an ICMP peer address.
// Handles both SOCK_DGRAM (*net.UDPAddr) and SOCK_RAW (*net.IPAddr) peers.
func icmpPeerIP(peer net.Addr) net.IP {
	switch addr := peer.(type) {
	case *net.UDPAddr:
		return addr.IP
	case *net.IPAddr:
		return addr.IP
	default:
		return nil
	}
}

func (b *NativeICMPBackend) matchEchoReply(data []byte, peer net.Addr, expectedIP net.IP, sequence uint16) (time.Duration, bool, error) {
	msg, err := icmp.ParseMessage(1, data)
	if err != nil {
		return 0, false, fmt.Errorf("parse icmp: %w", err)
	}

	if msg.Type != ipv4.ICMPTypeEchoReply {
		return 0, false, nil
	}

	echo, ok := msg.Body.(*icmp.Echo)
	if !ok {
		return 0, false, fmt.Errorf("unexpected body type")
	}

	if uint16(echo.ID) != b.backendID {
		return 0, false, nil
	}

	if uint16(echo.Seq) != sequence {
		return 0, false, nil
	}

	// Handle both SOCK_DGRAM (*net.UDPAddr) and SOCK_RAW (*net.IPAddr) peers
	peerIP := icmpPeerIP(peer)
	if peerIP == nil {
		return 0, false, nil
	}

	peerIP4 := peerIP.To4()
	if peerIP4 == nil || !peerIP4.Equal(expectedIP) {
		return 0, false, nil
	}

	if len(echo.Data) >= 8 {
		sendTimeNanos := int64(binary.LittleEndian.Uint64(echo.Data))
		sendTime := time.Unix(0, sendTimeNanos)
		return time.Since(sendTime), true, nil
	}

	return time.Millisecond, true, nil
}

func (b *NativeICMPBackend) classifyAndRecordSocketError(err error) {
	errStr := err.Error()

	if isPermissionError(err) {
		b.stats.permissionErrors.Add(1)
		b.setErrorClass(ErrClassPermission)
		return
	}

	b.stats.socketOpenErrors.Add(1)

	switch {
	case containsAny(errStr, "socket", "bind", "operation"):
		b.setErrorClass(ErrClassSocket)
	case containsAny(errStr, "timeout", "deadline"):
		b.setErrorClass(ErrClassTimeout)
	default:
		b.setErrorClass(ErrClassOther)
	}
}

func (b *NativeICMPBackend) getUserMessageForError(err error) string {
	class := b.getLastErrorClass()
	switch class {
	case ErrClassPermission:
		return "native ICMP backend unavailable: permission denied opening ICMP socket; run as root, grant CAP_NET_RAW, or configure ping_group_range; set icmp.backend=os_ping to use legacy fallback explicitly"
	case ErrClassSocket:
		return "native ICMP socket error; check network configuration; set icmp.backend=os_ping to use legacy fallback explicitly"
	default:
		return fmt.Sprintf("native ICMP socket error: %v; set icmp.backend=os_ping to use legacy fallback explicitly", err)
	}
}

func (b *NativeICMPBackend) setErrorClass(class NativeICMPErrorClass) {
	b.stats.lastErrorClass.Store(string(class))
}

func (b *NativeICMPBackend) getLastErrorClass() NativeICMPErrorClass {
	if v := b.stats.lastErrorClass.Load(); v != nil {
		return NativeICMPErrorClass(v.(string))
	}
	return ""
}

var backendIDCounter uint16
var backendIDMu sync.Mutex

func generateBackendID() uint16 {
	backendIDMu.Lock()
	defer backendIDMu.Unlock()
	pid := uint16(0)
	if runtime.GOOS == "linux" {
		// Use a simple incrementing counter as fallback
	}
	backendIDCounter++
	id := pid ^ backendIDCounter ^ uint16(time.Now().UnixNano()&0xFFFF)
	if id == 0 {
		id = 1
	}
	return id
}

func int64ToBytes(v int64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(v))
	return b
}

// MarshalICMPEchoBody marshals the ICMP echo request body fields.
func MarshalICMPEchoBody(id, seq uint16, timestamp int64) ([]byte, error) {
	buf := make([]byte, 12)
	binary.LittleEndian.PutUint16(buf[0:2], id)
	binary.LittleEndian.PutUint16(buf[2:4], seq)
	binary.LittleEndian.PutUint64(buf[4:12], uint64(timestamp))
	return buf, nil
}

func isPermissionError(err error) bool {
	errStr := err.Error()
	return containsAny(errStr, "permission denied", "operation not permitted", "EPERM", "EACCES")
}

func containsAny(s string, substrs ...string) bool {
	lower := strings.ToLower(s)
	for _, sub := range substrs {
		if strings.Contains(lower, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}
