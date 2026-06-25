// Package diag implements diagnostic capture for UVB-76.
//go:build linux
// +build linux

package diag

import (
	"syscall"
	"testing"
	"unsafe"
)

const (
	nlmFRequest = 0x01
	nlmFDump    = 0x300
)

// Linux UAPI RTA_* constants: DST=1, SRC=2, IIF=3, OIF=4, GATEWAY=5, PRIORITY=6, PREFSRC=7, TABLE=15

func TestBuildRouteRequest_IPv4(t *testing.T) {
	data := buildRouteRequest(AF_INET, "192.0.2.1")
	if len(data) < 20 {
		t.Fatalf("message too short: %d bytes", len(data))
	}
	nlmsg := (*nlmsghdr)(unsafe.Pointer(&data[0]))
	if nlmsg.Type != RTM_GETROUTE {
		t.Errorf("expected type %d, got %d", RTM_GETROUTE, nlmsg.Type)
	}
	if nlmsg.Flags != nlmFRequest {
		t.Errorf("expected NLM_F_REQUEST (0x01), got %#x", nlmsg.Flags)
	}
	if nlmsg.Flags&nlmFDump != 0 {
		t.Errorf("must not set dump flags, got %#x", nlmsg.Flags)
	}
}

func TestBuildRouteRequest_IPv6(t *testing.T) {
	data := buildRouteRequest(AF_INET6, "2001:db8::1")
	nlmsg := (*nlmsghdr)(unsafe.Pointer(&data[0]))
	if nlmsg.Flags != nlmFRequest {
		t.Errorf("expected NLM_F_REQUEST for IPv6, got %#x", nlmsg.Flags)
	}
}

func TestBuildRouteRequest_ContainsDstAttribute(t *testing.T) {
	data := buildRouteRequest(AF_INET, "192.0.2.1")
	found := false
	for i := 28; i < len(data)-4; i += 4 {
		if data[i+2] == 1 && data[i+3] == 0 { // RTA_DST = 1
			found = true
			break
		}
	}
	if !found {
		t.Error("expected RTA_DST attribute (type=1)")
	}
}

func TestAddrLen(t *testing.T) {
	if addrLen(AF_INET) != 32 {
		t.Errorf("expected 32 for IPv4, got %d", addrLen(AF_INET))
	}
	if addrLen(AF_INET6) != 128 {
		t.Errorf("expected 128 for IPv6, got %d", addrLen(AF_INET6))
	}
}

func TestNlmsgAlign(t *testing.T) {
	for _, tt := range []struct{ input, expected int }{{0, 0}, {1, 4}, {4, 4}, {5, 8}, {8, 8}, {9, 12}, {16, 16}} {
		if result := nlmsgAlign(tt.input); result != tt.expected {
			t.Errorf("nlmsgAlign(%d): expected %d, got %d", tt.input, tt.expected, result)
		}
	}
}

func TestParseRtMsg_Empty(t *testing.T) {
	if result := parseRtMsg([]byte{}); result != nil {
		t.Error("expected nil for empty data")
	}
}

func TestParseRtMsg_TooShort(t *testing.T) {
	if result := parseRtMsg([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}); result != nil {
		t.Error("expected nil for data shorter than 12 bytes")
	}
}

func TestParseRtMsg_RouteTypes(t *testing.T) {
	for _, tt := range []struct {
		typeByte byte
		expected string
	}{
		{RTN_UNICAST, "unicast"},
		{RTN_LOCAL, "local"},
		{RTN_UNREACHABLE, "unreachable"},
		{RTN_BLACKHOLE, "blackhole"},
		{RTN_PROHIBIT, "prohibit"},
		{99, "unicast"}, // Unknown defaults to unicast
	} {
		data := make([]byte, 12)
		data[7] = tt.typeByte
		result := parseRtMsg(data)
		if result == nil || result.RouteType != tt.expected {
			t.Errorf("type %d: expected %q, got %q", tt.typeByte, tt.expected, result.RouteType)
		}
	}
}

func TestParseRtMsg_WithGateway_Type5(t *testing.T) {
	data := make([]byte, 12+8+4)
	data[7] = RTN_UNICAST
	data[12], data[14] = 8, 5 // RTA_GATEWAY=5
	data[15], data[16], data[17], data[18] = 192, 0, 2, 1

	result := parseRtMsg(data)
	if result == nil || result.Gateway == nil || result.Gateway.String() != "192.0.2.1" {
		t.Errorf("expected gateway 192.0.2.1, got %v", result.Gateway)
	}
}

func TestParseRtMsg_Type1IsDstNotGateway(t *testing.T) {
	data := make([]byte, 12+8+4)
	data[7] = RTN_UNICAST
	data[12], data[14] = 8, 1 // type=1 is RTA_DST, NOT gateway
	data[15], data[16], data[17], data[18] = 192, 0, 2, 1

	result := parseRtMsg(data)
	if result != nil && result.Gateway != nil {
		t.Errorf("type 1 should NOT set gateway (that's RTA_DST), got %v", result.Gateway)
	}
}

func TestParseRtMsg_WithOIF_Type4(t *testing.T) {
	data := make([]byte, 12+8+4)
	data[7] = RTN_UNICAST
	data[12], data[14] = 8, 4 // RTA_OIF=4
	data[15] = 2

	result := parseRtMsg(data)
	if result == nil || result.Interface != 2 {
		t.Errorf("expected interface 2, got %d", result.Interface)
	}
}

func TestParseRtMsg_WithPriority_Type6(t *testing.T) {
	data := make([]byte, 12+8+4)
	data[7] = RTN_UNICAST
	data[12], data[14] = 8, 6 // RTA_PRIORITY=6
	data[15] = 100

	result := parseRtMsg(data)
	if result == nil || result.Metric != 100 {
		t.Errorf("expected metric 100, got %d", result.Metric)
	}
}

func TestParseRtMsg_WithPrefsrc_Type7(t *testing.T) {
	data := make([]byte, 12+8+4)
	data[7] = RTN_UNICAST
	data[12], data[14] = 8, 7 // RTA_PREFSRC=7
	data[15], data[18] = 10, 1

	result := parseRtMsg(data)
	if result == nil || result.SourceIP == nil || result.SourceIP.String() != "10.0.0.1" {
		t.Errorf("expected source 10.0.0.1, got %v", result.SourceIP)
	}
}

func TestParseRtMsg_WithTable_Type15(t *testing.T) {
	data := make([]byte, 12+8+4)
	data[7] = RTN_UNICAST
	data[12], data[14] = 8, 15 // RTA_TABLE=15
	data[15] = 254

	result := parseRtMsg(data)
	if result == nil || result.Table != 254 {
		t.Errorf("expected table 254, got %d", result.Table)
	}
}

func TestParseRtMsg_CompleteRoute(t *testing.T) {
	data := make([]byte, 12+8+4+8+4+8+4+8+4)
	data[7] = RTN_UNICAST
	off := 12
	// RTA_GATEWAY (5)
	data[off], data[off+2], data[off+4], data[off+5], data[off+6], data[off+7] = 8, 5, 192, 0, 2, 1
	off += 8
	// RTA_OIF (4)
	data[off], data[off+2], data[off+4] = 8, 4, 1
	off += 8
	// RTA_PREFSRC (7)
	data[off], data[off+2], data[off+4], data[off+7] = 8, 7, 10, 1
	off += 8
	// RTA_PRIORITY (6)
	data[off], data[off+2], data[off+4] = 8, 6, 100

	result := parseRtMsg(data)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Gateway == nil || result.Gateway.String() != "192.0.2.1" {
		t.Errorf("gateway: expected 192.0.2.1, got %v", result.Gateway)
	}
	if result.Interface != 1 {
		t.Errorf("interface: expected 1, got %d", result.Interface)
	}
	if result.Metric != 100 {
		t.Errorf("metric: expected 100, got %d", result.Metric)
	}
}

func TestRouteLookupNetlinkResultToProbeRoute_Nil(t *testing.T) {
	if result := RouteLookupNetlinkResultToProbeRoute(nil, true); result != nil {
		t.Error("expected nil for nil input")
	}
}

func TestRouteLookupNetlinkResultToProbeRoute_Basic(t *testing.T) {
	nlResult := &NetlinkRouteResult{RouteType: "unicast", InterfaceName: "eth0"}
	result := RouteLookupNetlinkResultToProbeRoute(nlResult, true)
	if result == nil || result.RouteType != "unicast" || result.Interface != "eth0" {
		t.Errorf("unexpected: %+v", result)
	}
}

func TestRouteLookupNetlinkResultToProbeRoute_Redacted(t *testing.T) {
	nlResult := &NetlinkRouteResult{
		RouteType: "unicast", InterfaceName: "eth0",
		SourceIP: []byte{10, 0, 0, 1}, Gateway: []byte{192, 0, 2, 1},
	}
	result := RouteLookupNetlinkResultToProbeRoute(nlResult, true)
	if result == nil || result.SourceIP != "redacted" || result.Gateway != "redacted" {
		t.Errorf("expected redacted, got %s/%s", result.SourceIP, result.Gateway)
	}
}

func TestRouteLookupNetlinkResultToProbeRoute_NotRedacted(t *testing.T) {
	nlResult := &NetlinkRouteResult{
		RouteType: "unicast", InterfaceName: "eth0",
		SourceIP: []byte{10, 0, 0, 1}, Gateway: []byte{192, 0, 2, 1},
	}
	result := RouteLookupNetlinkResultToProbeRoute(nlResult, false)
	if result == nil || result.SourceIP != "10.0.0.1" || result.Gateway != "192.0.2.1" {
		t.Errorf("expected IPs, got %s/%s", result.SourceIP, result.Gateway)
	}
}

func TestNetlinkError_KindIs(t *testing.T) {
	err := &NetlinkError{Kind: "timeout", Message: "timeout occurred"}
	if !err.KindIs("timeout") || err.KindIs("other") {
		t.Error("KindIs failed")
	}
}

func TestNetlinkError_Error(t *testing.T) {
	err := &NetlinkError{Kind: "timeout", Message: "timeout occurred"}
	if err.Error() != "timeout occurred" {
		t.Errorf("expected 'timeout occurred', got '%s'", err.Error())
	}
}

func TestParseRouteResponse_EmptyBuffer(t *testing.T) {
	result := &NetlinkRouteResult{}
	parsed := parseRouteResponse([]byte{}, result)
	if parsed.Error == nil || parsed.Error.Kind != "no_route" {
		t.Errorf("expected no_route error, got %v", parsed.Error)
	}
}

func TestParseRouteResponse_TruncatedHeader(t *testing.T) {
	result := &NetlinkRouteResult{}
	parsed := parseRouteResponse([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, result)
	if parsed.Error == nil {
		t.Error("expected error for truncated header")
	}
}

func TestParseRouteResponse_NLMSG_ERROR(t *testing.T) {
	data := make([]byte, 20)
	nlmsg := (*nlmsghdr)(unsafe.Pointer(&data[0]))
	nlmsg.Len = 20
	nlmsg.Type = uint16(syscall.NLMSG_ERROR)
	data[16], data[17], data[18], data[19] = 0xFE, 0xFF, 0xFF, 0xFF // -2 ENOENT

	result := &NetlinkRouteResult{}
	parsed := parseRouteResponse(data, result)
	if parsed.Error == nil || parsed.Error.Kind != "no_route" {
		t.Errorf("expected no_route error, got %v", parsed.Error)
	}
}

// TestNegativeErrno_Helper verifies the helper correctly converts syscall.Errno to negative int32.
func TestNegativeErrno_Helper(t *testing.T) {
	// ENOENT is errno 2 on Linux. In netlink error domain, it must be -2.
	enoentNeg := negativeErrno(syscall.ENOENT)
	if enoentNeg != -2 {
		t.Errorf("expected ENOENT as -2, got %d", enoentNeg)
	}
}

// TestNegativeErrno_OtherErrors verifies other errno values are correctly negated.
func TestNegativeErrno_OtherErrors(t *testing.T) {
	// EINVAL (22 on Linux) should become -22
	einvalNeg := negativeErrno(syscall.EINVAL)
	if einvalNeg != -22 {
		t.Errorf("expected EINVAL as -22, got %d", einvalNeg)
	}

	// ENOMEM (12 on Linux) should become -12
	enomemNeg := negativeErrno(syscall.ENOMEM)
	if enomemNeg != -12 {
		t.Errorf("expected ENOMEM as -12, got %d", enomemNeg)
	}
}

// TestParseRouteResponse_ENOENTViaHelper verifies ENOENT detection uses the negativeErrno helper.
func TestParseRouteResponse_ENOENTViaHelper(t *testing.T) {
	data := make([]byte, 20)
	nlmsg := (*nlmsghdr)(unsafe.Pointer(&data[0]))
	nlmsg.Len = 20
	nlmsg.Type = uint16(syscall.NLMSG_ERROR)
	// ENOENT = 2, negated via helper = -2 (0xFE 0xFF 0xFF 0xFF in little-endian)
	negErrno := negativeErrno(syscall.ENOENT)
	data[16] = byte(negErrno)
	data[17] = byte(negErrno >> 8)
	data[18] = byte(negErrno >> 16)
	data[19] = byte(negErrno >> 24)

	result := &NetlinkRouteResult{}
	parsed := parseRouteResponse(data, result)
	if parsed.Error == nil || parsed.Error.Kind != "no_route" {
		t.Errorf("expected no_route error via helper, got %v", parsed.Error)
	}
}

func TestParseRtMsg_MalformedAttributes(t *testing.T) {
	// Truncated attribute
	data := make([]byte, 12+3)
	data[7] = RTN_UNICAST
	data[12], data[13], data[14] = 8, 0, 5
	if result := parseRtMsg(data); result == nil || result.RouteType != "unicast" {
		t.Error("expected unicast for truncated attr")
	}

	// Invalid attr len
	data = make([]byte, 12+8)
	data[7] = RTN_UNICAST
	data[12], data[13], data[14], data[15] = 3, 0, 5, 0 // len=3 < 4
	if result := parseRtMsg(data); result == nil {
		t.Error("expected non-nil for invalid attr len")
	}

	// Attr claims more than buffer
	data = make([]byte, 12+8+4)
	data[7] = RTN_UNICAST
	data[12], data[13], data[14], data[15] = 20, 0, 5, 0 // claims 20 bytes
	if result := parseRtMsg(data); result == nil {
		t.Error("expected non-nil for attr len > buffer")
	}
}
