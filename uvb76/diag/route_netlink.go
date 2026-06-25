// Package diag implements diagnostic capture for UVB-76.
// This file provides native NETLINK_ROUTE route lookup implementation.
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
)

// negativeErrno converts a syscall.Errno to its negative representation as used in netlink error codes.
func negativeErrno(errno syscall.Errno) int32 {
	return -int32(errno)
}

// RouteLookupNative performs a native route lookup using NETLINK_ROUTE.
// This is the preferred implementation that avoids CLI composition.
//
// Returns a structured result compatible with the existing ProbeRoute evidence shape.
func RouteLookupNative(ctx context.Context, target string) *NetlinkRouteResult {
	result := &NetlinkRouteResult{}

	// Parse target IP
	ip := net.ParseIP(target)
	if ip == nil {
		result.Error = &NetlinkError{Kind: "invalid_target", Message: "invalid IP address"}
		return result
	}
	result.Destination = ip

	// Determine address family
	family := AF_INET
	if ip4 := ip.To4(); ip4 != nil {
		result.Destination = ip4
		family = AF_INET
	} else {
		family = AF_INET6
	}

	// Open netlink socket
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW, NETLINK_ROUTE)
	if err != nil {
		result.Error = &NetlinkError{Kind: "open_failed", Message: fmt.Sprintf("socket: %v", err)}
		return result
	}
	defer syscall.Close(fd)

	// Set receive timeout using NsecToTimeval for platform-correct field types
	tv := syscall.NsecToTimeval((2 * time.Second).Nanoseconds())
	if deadline, ok := ctx.Deadline(); ok {
		timeout := deadline.Sub(time.Now())
		if timeout > 0 {
			tv = syscall.NsecToTimeval(timeout.Nanoseconds())
		}
	}
	if err := syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv); err != nil {
		result.Error = &NetlinkError{Kind: "open_failed", Message: fmt.Sprintf("setsockopt: %v", err)}
		return result
	}

	// Bind socket
	addr := &syscall.SockaddrNetlink{
		Family: syscall.AF_NETLINK,
		Pid:    uint32(syscall.Gettid()),
		Groups: 0,
	}
	if err := syscall.Bind(fd, addr); err != nil {
		result.Error = &NetlinkError{Kind: "bind_failed", Message: fmt.Sprintf("bind: %v", err)}
		return result
	}

	// Build and send route lookup request
	req := buildRouteRequest(family, target)
	if err := sendNetlinkRequest(fd, req); err != nil {
		result.Error = &NetlinkError{Kind: "send_failed", Message: fmt.Sprintf("send: %v", err)}
		return result
	}

	// Receive and parse response
	msgs, err := receiveNetlinkResponse(fd)
	if err != nil {
		if opErr, ok := err.(*NetlinkError); ok {
			result.Error = opErr
		} else {
			result.Error = &NetlinkError{Kind: "receive_failed", Message: err.Error()}
		}
		return result
	}

	// Parse route response
	return parseRouteResponse(msgs, result)
}

// buildRouteRequest constructs a rtnetlink RTM_GETROUTE message.
func buildRouteRequest(family int, target string) []byte {
	// Build destination bytes
	var dstBytes []byte
	if ip := net.ParseIP(target); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			dstBytes = ip4
		} else {
			dstBytes = ip.To16()
		}
	}

	// Calculate total message size
	hdrLen := 16                      // nlmsghdr
	rtmsgLen := 12                    // rtmsg
	attrHdrLen := 4                   // rtattr header
	totalLen := nlmsgAlign(hdrLen+rtmsgLen) + nlmsgAlign(attrHdrLen+len(dstBytes))
	data := make([]byte, totalLen)

	// Write nlmsghdr
	nlmsg := (*nlmsghdr)(unsafe.Pointer(&data[0]))
	nlmsg.Len = uint32(totalLen)
	nlmsg.Type = RTM_GETROUTE
	nlmsg.Flags = NLM_F_REQUEST // 0x01 - request only, no dump modifiers
	nlmsg.Seq = 1
	nlmsg.Pid = 0

	// Write rtmsg
	rtmsgOff := nlmsgAlign(16)
	rtmsg := &rtMsg{
		Family: byte(family),
		DstLen: byte(addrLen(family)),
		Table:  RT_TABLE_MAIN,
		Scope:  RT_SCOPE_UNIVERSE,
		Type:   RTN_UNICAST,
		Flags:  0, // metric in lower byte
	}
	copy(data[rtmsgOff:rtmsgOff+12], (*(*[12]byte)(unsafe.Pointer(rtmsg)))[:])

	// Write rtattr for destination (RTA_DST = 1)
	attrOff := nlmsgAlign(hdrLen + rtmsgLen)
	attr := (*rtattr)(unsafe.Pointer(&data[attrOff]))
	attr.Len = uint16(attrHdrLen + len(dstBytes))
	attr.Type = 1 // RTA_DST

	copy(data[attrOff+attrHdrLen:], dstBytes)

	return data
}

func addrLen(family int) uint8 {
	if family == AF_INET {
		return 32
	}
	return 128
}

func nlmsgAlign(length int) int {
	return (length + 4 - 1) & ^(4 - 1)
}

func sendNetlinkRequest(fd int, data []byte) error {
	_, err := syscall.Write(fd, data)
	return err
}

func receiveNetlinkResponse(fd int) ([]byte, error) {
	// Read with buffer large enough for multiple messages
	buf := make([]byte, 8192)
	n, err := syscall.Read(fd, buf)
	if err != nil {
		if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
			return nil, ErrNetlinkTimeout
		}
		if err == syscall.EPERM {
			return nil, ErrNetlinkPermission
		}
		return nil, err
	}

	if n == 0 {
		return nil, ErrNetlinkNoRoute
	}

	return buf[:n], nil
}

// parseRouteResponse extracts route information from netlink messages.
func parseRouteResponse(buf []byte, result *NetlinkRouteResult) *NetlinkRouteResult {
	var bestRoute *parsedRoute
	off := 0

	for off < len(buf) {
		if off+16 > len(buf) {
			break
		}

		nlmsg := (*nlmsghdr)(unsafe.Pointer(&buf[off]))
		msgLen := int(nlmsg.Len)
		if msgLen < 16 || off+int(msgLen) > len(buf) {
			break
		}

		if nlmsg.Type == uint16(syscall.NLMSG_ERROR) {
			// Check for NLMSG_ERROR with error code
			if off+20 > len(buf) {
				break
			}
			err := (*int32)(unsafe.Pointer(&buf[off+16]))
			if *err != 0 {
				if *err == negativeErrno(syscall.ENOENT) {
					result.Error = ErrNetlinkNoRoute
				} else {
					result.Error = &NetlinkError{
						Kind:    "netlink_error",
						Message: fmt.Sprintf("netlink error: %d", *err),
					}
				}
				return result
			}
			// NLMSG_ERROR with error=0 is acknowledgment, continue
			off += nlmsgAlign(int(msgLen))
			continue
		}

		if nlmsg.Type != RTM_GETROUTE && nlmsg.Type != 24 { // RTM_NEWROUTE = 24
			off += nlmsgAlign(int(msgLen))
			continue
		}

		route := parseRtMsg(buf[off+16 : off+int(msgLen)])
		if route != nil {
			// For route lookup, prefer the most specific route
			if bestRoute == nil || route.Preference < bestRoute.Preference {
				bestRoute = route
			}
		}

		off += nlmsgAlign(int(msgLen))
	}

	if bestRoute == nil {
		result.Error = ErrNetlinkNoRoute
		return result
	}

	// Copy best route results
	result.RouteType = bestRoute.RouteType
	result.Interface = bestRoute.Interface
	result.Gateway = bestRoute.Gateway
	result.SourceIP = bestRoute.SourceIP
	result.Table = bestRoute.Table
	result.Protocol = bestRoute.Protocol
	result.Scope = bestRoute.Scope
	result.Metric = bestRoute.Metric

	// Look up interface name if we have interface index
	if result.Interface > 0 {
		result.InterfaceName = getInterfaceName(result.Interface)
	}

	return result
}

func parseRtMsg(data []byte) *parsedRoute {
	if len(data) < 12 {
		return nil
	}

	route := &parsedRoute{}

	// Parse rtmsg structure with bounds checking
	rtmsg := (*rtMsg)(unsafe.Pointer(&data[0]))
	route.Protocol = int(rtmsg.Proto)
	route.Scope = int(rtmsg.Scope)
	// Note: rtmsg.Flags is NOT metric. Metric comes from RTA_PRIORITY attribute only.

	// Map route type
	switch rtmsg.Type {
	case RTN_UNICAST:
		route.RouteType = "unicast"
	case RTN_LOCAL:
		route.RouteType = "local"
	case RTN_UNREACHABLE:
		route.RouteType = "unreachable"
	case RTN_BLACKHOLE:
		route.RouteType = "blackhole"
	case RTN_PROHIBIT:
		route.RouteType = "prohibit"
	default:
		route.RouteType = "unicast"
	}

	// Parse attributes with defensive bounds checking
	attrOff := 12
	for attrOff+4 <= len(data) {
		attr := (*rtattr)(unsafe.Pointer(&data[attrOff]))
		// Validate attribute length is reasonable
		if attr.Len < 4 {
			break
		}
		attrTotalLen := int(attr.Len)
		if attrOff+attrTotalLen > len(data) {
			break
		}

		attrLen := attrTotalLen - 4
		attrData := data[attrOff+4 : attrOff+attrTotalLen]

		switch attr.Type {
		case RTA_GATEWAY: // type 5
			if attrLen >= 4 && len(attrData) >= 4 {
				route.Gateway = net.IP(attrData[:min(attrLen, len(attrData))])
			}
		case RTA_OIF: // type 4
			if len(attrData) >= 4 {
				route.Interface = int(*(*uint32)(unsafe.Pointer(&attrData[0])))
			}
		case RTA_PREFSRC: // type 7
			if attrLen >= 4 && len(attrData) >= 4 {
				route.SourceIP = net.IP(attrData[:min(attrLen, len(attrData))])
			}
		case RTA_TABLE: // type 15
			if len(attrData) >= 4 {
				route.Table = int(*(*uint32)(unsafe.Pointer(&attrData[0])))
			}
		case RTA_PRIORITY: // type 6
			// Route metric via RTA_PRIORITY (metric is stored as uint32)
			if len(attrData) >= 4 {
				metric := int(*(*uint32)(unsafe.Pointer(&attrData[0])))
				// Use RTA_PRIORITY metric if available, otherwise fall back to rtmsg.Flags
				if route.Metric == 0 {
					route.Metric = metric
				}
			}
		}

		attrOff += nlmsgAlign(attrTotalLen)
	}

	// Calculate preference (lower is better, prefer direct routes)
	route.Preference = route.Metric
	if route.Gateway != nil {
		route.Preference += 1000 // Gateway routes less preferred
	}

	return route
}

// getInterfaceName returns the interface name for a given index.
func getInterfaceName(ifindex int) string {
	iface, err := net.InterfaceByIndex(ifindex)
	if err != nil {
		return ""
	}
	return iface.Name
}

// RouteLookupNetlinkResultToProbeRoute converts a native result to ProbeRoute-compatible fields.
// This preserves the existing evidence shape while using native implementation.
func RouteLookupNetlinkResultToProbeRoute(nlResult *NetlinkRouteResult, redact bool) *RouteLookupFields {
	if nlResult == nil {
		return nil
	}

	fields := &RouteLookupFields{
		RouteType: nlResult.RouteType,
	}

	if nlResult.InterfaceName != "" {
		fields.Interface = nlResult.InterfaceName
	} else if nlResult.Interface > 0 {
		fields.Interface = fmt.Sprintf("if%d", nlResult.Interface)
	}

	if nlResult.SourceIP != nil {
		if redact {
			fields.SourceIP = "redacted"
		} else {
			fields.SourceIP = nlResult.SourceIP.String()
		}
	}

	if nlResult.Gateway != nil {
		if redact {
			fields.Gateway = "redacted"
		} else {
			fields.Gateway = nlResult.Gateway.String()
		}
	}

	if nlResult.Table > 0 {
		fields.Table = fmt.Sprintf("%d", nlResult.Table)
	}

	return fields
}
