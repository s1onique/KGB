// Package diag implements diagnostic capture for UVB-76.
// This file provides stub implementations for non-Linux platforms.
//go:build !linux
// +build !linux

package diag

import (
	"context"
)

// NetlinkError represents a netlink operation error.
type NetlinkError struct {
	Kind    string
	Message string
}

func (e *NetlinkError) Error() string { return e.Message }

func (e *NetlinkError) KindIs(kind string) bool { return e.Kind == kind }

// NetlinkRouteResult contains the result of a native route lookup.
type NetlinkRouteResult struct {
	Error *NetlinkError
}

// RouteLookupNative performs a native route lookup using NETLINK_ROUTE.
// On non-Linux platforms, this always returns an error.
func RouteLookupNative(ctx context.Context, target string) *NetlinkRouteResult {
	return &NetlinkRouteResult{
		Error: &NetlinkError{
			Kind:    "unsupported",
			Message: "NETLINK_ROUTE not available on this platform",
		},
	}
}

// RouteLookupNetlinkResultToProbeRoute converts a native result to ProbeRoute-compatible fields.
func RouteLookupNetlinkResultToProbeRoute(nlResult *NetlinkRouteResult, redact bool) *RouteLookupFields {
	return nil
}

// RouteLookupFields contains the parsed route lookup fields.
type RouteLookupFields struct {
	RouteType string
	Interface string
	SourceIP  string
	Gateway   string
	Table     string
	MTU       *int
	UID       *int
}
