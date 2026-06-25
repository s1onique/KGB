// Package diag implements diagnostic capture for UVB-76.
// This file provides native NETLINK_ROUTE type definitions.
//go:build linux
// +build linux

package diag

import (
	"net"
)

// NETLINK_ROUTE socket family and rtnetlink groups
const (
	NETLINK_ROUTE = 0

	// Netlink flags (from Linux UAPI)
	NLM_F_REQUEST = 0x01
	NLM_F_ROOT    = 0x100
	NLM_F_MATCH   = 0x200
	NLM_F_DUMP    = NLM_F_ROOT | NLM_F_MATCH // 0x300

	// Route message types
	RTM_GETROUTE = 26
	RTM_NEWROUTE  = 24

	RT_SCOPE_UNIVERSE = 0
	RT_TABLE_MAIN     = 254

	RTA_UNSPEC   = 0
	RTA_DST      = 1
	RTA_SRC      = 2
	RTA_IIF      = 3
	RTA_OIF      = 4
	RTA_GATEWAY  = 5
	RTA_PRIORITY = 6
	RTA_PREFSRC  = 7
	RTA_TABLE    = 15

	AF_UNSPEC = 0
	AF_INET   = 2
	AF_INET6  = 10

	RTN_UNICAST     = 1
	RTN_LOCAL       = 2
	RTN_UNREACHABLE = 3
	RTN_BLACKHOLE   = 5
	RTN_PROHIBIT    = 6
)

// Netlink route lookup errors
var (
	ErrNetlinkOpen       = &NetlinkError{Kind: "open_failed", Message: "failed to open netlink socket"}
	ErrNetlinkBind       = &NetlinkError{Kind: "bind_failed", Message: "failed to bind netlink socket"}
	ErrNetlinkSend       = &NetlinkError{Kind: "send_failed", Message: "failed to send netlink message"}
	ErrNetlinkReceive    = &NetlinkError{Kind: "receive_failed", Message: "failed to receive netlink response"}
	ErrNetlinkTimeout    = &NetlinkError{Kind: "timeout", Message: "netlink request timed out"}
	ErrNetlinkPermission = &NetlinkError{Kind: "permission", Message: "netlink permission denied"}
	ErrNetlinkNoRoute    = &NetlinkError{Kind: "no_route", Message: "no route to host"}
	ErrNetlinkMalformed  = &NetlinkError{Kind: "malformed", Message: "malformed netlink response"}
)

// NetlinkError represents a netlink operation error.
type NetlinkError struct {
	Kind    string
	Message string
}

func (e *NetlinkError) Error() string { return e.Message }

// KindIs checks if the error kind matches.
func (e *NetlinkError) KindIs(kind string) bool { return e.Kind == kind }

// NetlinkRouteResult contains the result of a native route lookup.
type NetlinkRouteResult struct {
	Destination   net.IP
	RouteType     string
	Interface     int
	InterfaceName string
	Gateway       net.IP
	SourceIP      net.IP
	Table         int
	Protocol      int
	Scope         int
	Metric        int
	Error         *NetlinkError
}

// nlmsghdr is the netlink message header
type nlmsghdr struct {
	Len   uint32
	Type  uint16
	Flags uint16
	Seq   uint32
	Pid   uint32
}

// rtattr is the route attribute header
type rtattr struct {
	Len  uint16
	Type uint16
}

// rtMsg is the route message
type rtMsg struct {
	Family  byte
	DstLen  byte
	SrcLen  byte
	Tos     byte
	Table   byte
	Proto   byte
	Scope   byte
	Type    byte
	Flags   uint32
}

// parsedRoute holds intermediate parsed route data.
type parsedRoute struct {
	RouteType  string
	Interface  int
	Gateway    net.IP
	SourceIP   net.IP
	Table      int
	Protocol   int
	Scope      int
	Metric     int
	Preference int // lower is better
}

// RouteLookupFields contains the parsed route lookup fields.
// This is compatible with the existing RouteLookupParser output.
type RouteLookupFields struct {
	RouteType string
	Interface string
	SourceIP  string
	Gateway   string
	Table     string
	MTU       *int
	UID       *int
}
