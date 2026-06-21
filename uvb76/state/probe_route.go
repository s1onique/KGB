package state

// =============================================================================
// ProbeRoute — Route Lookup Evidence for Diagnostic Packets
// =============================================================================

// ProbeRouteKind represents the type of probe that triggered the route lookup.
type ProbeRouteKind string

const (
	ProbeRouteKindHTTP ProbeRouteKind = "http"
	ProbeRouteKindICMP ProbeRouteKind = "icmp"
)

// RouteLookupErrorKind classifies route lookup failures.
type RouteLookupErrorKind string

const (
	RouteLookupErrorCommandMissing RouteLookupErrorKind = "command_missing"
	RouteLookupErrorNonZeroExit   RouteLookupErrorKind = "non_zero_exit"
	RouteLookupErrorTimeout       RouteLookupErrorKind = "timeout"
	RouteLookupErrorParseFailed   RouteLookupErrorKind = "parse_failed"
	RouteLookupErrorNoData       RouteLookupErrorKind = "no_data"
	RouteLookupErrorUnavailable   RouteLookupErrorKind = "unavailable"
)

// ProbeRoute represents the result of a route lookup for probe attribution.
// This provides evidence of which kernel route was selected for the exact
// probe destination at capture time.
//
// Privacy: Source IPs and gateways are redacted to "redacted" to prevent
// exposure of internal network topology. Interface names are included as they
// are not considered sensitive infrastructure metadata.
type ProbeRoute struct {
	// Kind is the type of probe that triggered this route lookup.
	Kind ProbeRouteKind `json:"kind"`

	// ProbeHost is the host/IP that was probed (may be redacted).
	// For HTTP: the host from the probe URL
	// For ICMP: the target host/IP
	ProbeHost string `json:"probe_host"`

	// ResolvedIP is the resolved IP if DNS was involved (may be redacted).
	ResolvedIP string `json:"resolved_ip,omitempty"`

	// LookupTarget is the IP address that was looked up (may be redacted).
	LookupTarget string `json:"lookup_target"`

	// Ok indicates whether the route lookup succeeded.
	Ok bool `json:"ok"`

	// RouteType is the route type (e.g., "unicast", "local", "unreachable").
	RouteType string `json:"route_type,omitempty"`

	// Interface is the selected output interface/device.
	Interface string `json:"interface,omitempty"`

	// SourceIP is the source address that would be used (redacted).
	SourceIP string `json:"source_ip,omitempty"`

	// Gateway is the next hop gateway (redacted, if present).
	Gateway string `json:"gateway,omitempty"`

	// Table is the routing table that was used (e.g., "main", "local", or numeric).
	Table string `json:"table,omitempty"`

	// MTU is the path MTU if available.
	MTU *int `json:"mtu,omitempty"`

	// UID is the socket UID if present (typically null for route lookups).
	UID *int `json:"uid,omitempty"`

	// Error is a human-readable error message if the lookup failed.
	Error string `json:"error,omitempty"`

	// ErrorKind classifies the error for machine processing.
	ErrorKind RouteLookupErrorKind `json:"error_kind,omitempty"`

	// CollectedAt is when the route lookup was performed.
	CollectedAt string `json:"collected_at"`
}
