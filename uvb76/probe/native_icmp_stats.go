// Package probe implements independent latency probing for UVB-76.
package probe

import (
	"os"
	"strings"
	"sync/atomic"
)

// NativeICMPStats records native ICMP socket telemetry.
type NativeICMPStats struct {
	Sent             uint64 `json:"sent"`
	Received         uint64 `json:"received"`
	Timeouts         uint64 `json:"timeouts"`
	SocketOpenErrors uint64 `json:"socket_open_errors"`
	PermissionErrors uint64 `json:"permission_errors"`
	ParseErrors      uint64 `json:"parse_errors"`
	UnmatchedReplies uint64 `json:"unmatched_replies"`
	LastRTTMillis    int64  `json:"last_rtt_ms"`
	LastErrorClass   string `json:"last_error_class"`
}

// NativeICMPStatsSnapshot is a read-only snapshot of native ICMP stats.
type NativeICMPStatsSnapshot struct {
	Backend          string `json:"backend"`
	Sent             uint64 `json:"sent"`
	Received         uint64 `json:"received"`
	Timeouts         uint64 `json:"timeouts"`
	SocketOpenErrs   uint64 `json:"socket_open_errors"`
	PermissionErrs   uint64 `json:"permission_errors"`
	ParseErrors      uint64 `json:"parse_errors"`
	UnmatchedReplies uint64 `json:"unmatched_replies"`
	LastRTTMillis    int64  `json:"last_rtt_ms"`
	LastErrorClass   string `json:"last_error_class"`
	LastError        string `json:"last_error,omitempty"`        // last runtime error string
	LastOperation    string `json:"last_operation,omitempty"`    // last operation attempted (e.g., "write_to", "read_from")
	// Socket mode diagnostics - shows which socket type is active
	SocketMode       NativeICMPSocketMode `json:"socket_mode"`
	DgramError       string              `json:"dgram_error,omitempty"`    // EACCES if datagram failed
	RawError         string              `json:"raw_error,omitempty"`     // error if raw socket failed
	PingGroupRange   string              `json:"ping_group_range,omitempty"` // from /proc/sys/net/ipv4/ping_group_range
}

// NativeICMPStatsRecorder provides methods to record native ICMP statistics.
type NativeICMPStatsRecorder interface {
	Snapshot() NativeICMPStatsSnapshot
}

// nativeICMPStatsInternal is the internal stats storage with atomic counters.
type nativeICMPStatsInternal struct {
	sent             atomic.Uint64
	received         atomic.Uint64
	timeouts         atomic.Uint64
	socketOpenErrors atomic.Uint64
	permissionErrors atomic.Uint64
	parseErrors      atomic.Uint64
	unmatchedReplies atomic.Uint64
	lastRTTMillis    atomic.Int64
	lastErrorClass   atomic.Value // string
	lastError        atomic.Value // string
	lastOperation    atomic.Value // string
}

// pingGroupRange reads the kernel's ping_group_range setting.
// This controls which GIDs can use unprivileged ICMP (SOCK_DGRAM).
// Returns empty string if reading fails (e.g., not on Linux).
func pingGroupRange() string {
	// Linux-specific: /proc/sys/net/ipv4/ping_group_range
	data, err := os.ReadFile("/proc/sys/net/ipv4/ping_group_range")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// Snapshot returns a read-only copy of the stats.
func (s *nativeICMPStatsInternal) Snapshot() NativeICMPStatsSnapshot {
	// Get socket open result for diagnostics
	socketResult := GetLastSocketOpenResult()

	snap := NativeICMPStatsSnapshot{
		Backend:          "native",
		Sent:             s.sent.Load(),
		Received:         s.received.Load(),
		Timeouts:         s.timeouts.Load(),
		SocketOpenErrs:   s.socketOpenErrors.Load(),
		PermissionErrs:   s.permissionErrors.Load(),
		ParseErrors:      s.parseErrors.Load(),
		UnmatchedReplies: s.unmatchedReplies.Load(),
		LastRTTMillis:    s.lastRTTMillis.Load(),
		LastErrorClass:   s.getLastErrorClass(),
		LastError:        s.getLastError(),
		LastOperation:    s.getLastOperation(),
	}

	// Add socket mode diagnostics if available
	if socketResult != nil {
		snap.SocketMode = socketResult.SocketMode
		snap.DgramError = socketResult.DgramError
		snap.RawError = socketResult.RawError
	}

	// Add ping_group_range for Linux diagnostics
	snap.PingGroupRange = pingGroupRange()

	return snap
}

func (s *nativeICMPStatsInternal) getLastErrorClass() string {
	if v := s.lastErrorClass.Load(); v != nil {
		return v.(string)
	}
	return ""
}

func (s *nativeICMPStatsInternal) getLastError() string {
	if v := s.lastError.Load(); v != nil {
		return v.(string)
	}
	return ""
}

func (s *nativeICMPStatsInternal) getLastOperation() string {
	if v := s.lastOperation.Load(); v != nil {
		return v.(string)
	}
	return ""
}

// NativeICMPErrorClass represents error classification for user-facing messages.
type NativeICMPErrorClass string

const (
	// ErrClassTimeout indicates the probe timed out.
	ErrClassTimeout NativeICMPErrorClass = "timeout"
	// ErrClassPermission indicates permission denied opening ICMP socket.
	ErrClassPermission NativeICMPErrorClass = "permission_denied"
	// ErrClassSocket indicates socket creation failed.
	ErrClassSocket NativeICMPErrorClass = "socket_error"
	// ErrClassParse indicates ICMP reply parsing failed.
	ErrClassParse NativeICMPErrorClass = "parse_error"
	// ErrClassUnreachable indicates host is unreachable.
	ErrClassUnreachable NativeICMPErrorClass = "unreachable"
	// ErrClassCanceled indicates context was canceled.
	ErrClassCanceled NativeICMPErrorClass = "canceled"
	// ErrClassOther indicates other errors.
	ErrClassOther NativeICMPErrorClass = "other"
)

// NativeICMPError wraps an error with its classification for actionable user messages.
type NativeICMPError struct {
	Err         error
	ErrorClass  NativeICMPErrorClass
	UserMessage string
}

func (e *NativeICMPError) Error() string {
	return e.Err.Error()
}

func (e *NativeICMPError) Unwrap() error {
	return e.Err
}

// NewNativeICMPError creates a classified error with user-facing message.
func NewNativeICMPError(class NativeICMPErrorClass, err error, userMsg string) *NativeICMPError {
	return &NativeICMPError{
		Err:         err,
		ErrorClass:  class,
		UserMessage: userMsg,
	}
}

// NativeICMPTelemetry wraps the native backend stats with a global accessor.
type NativeICMPTelemetry struct {
	stats *nativeICMPStatsInternal
}

// Snapshot returns a read-only copy of the telemetry.
func (t *NativeICMPTelemetry) Snapshot() NativeICMPStatsSnapshot {
	return t.stats.Snapshot()
}
