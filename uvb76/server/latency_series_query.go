package server

import (
	"errors"
	"net/url"
	"strconv"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// Query parameter constants for bounds enforcement.
// These prevent pathological request parameters from causing excessive allocations.
const (
	// DefaultRangeSeconds is the default time range for latency series queries.
	DefaultRangeSeconds = 3600

	// DefaultStepSeconds is the default step between data points.
	DefaultStepSeconds = 60

	// DefaultWindowSeconds is the default aggregation window size.
	DefaultWindowSeconds = 300
)

// ErrTargetIDRequired is returned when target_id is missing from query.
var ErrTargetIDRequired = errors.New("target_id is required")

// ErrInvalidProbeKind is returned when probe_kind is invalid.
var ErrInvalidProbeKind = errors.New("probe_kind must be 'http' or 'icmp'")

// ErrRangeExceedsMaximum is returned when range_seconds exceeds MaxRangeSeconds.
var ErrRangeExceedsMaximum = errors.New("range_seconds exceeds maximum")

// ErrStepExceedsMaximum is returned when step_seconds exceeds MaxStepSeconds.
var ErrStepExceedsMaximum = errors.New("step_seconds exceeds maximum")

// LatencySeriesQuery represents parsed query parameters for the latency series endpoint.
type LatencySeriesQuery struct {
	TargetID      string
	ProbeKind    domain.ProbeKind
	ProbeKindStr string
	RangeSeconds  int
	StepSeconds   int
	WindowSeconds int
}

// ParseLatencySeriesQuery parses query parameters and applies defaults and clamps.
// This is a pure function with no state access - it only performs parse/default/clamp/reject.
func ParseLatencySeriesQuery(values url.Values) (LatencySeriesQuery, error) {
	q := LatencySeriesQuery{
		RangeSeconds:  DefaultRangeSeconds,
		StepSeconds:   DefaultStepSeconds,
		WindowSeconds: DefaultWindowSeconds,
	}

	// Parse target_id (required)
	targetID := values.Get("target_id")
	if targetID == "" {
		return q, ErrTargetIDRequired
	}
	q.TargetID = targetID

	// Parse probe_kind (optional, defaults to http)
	probeKindStr := values.Get("probe_kind")
	if probeKindStr == "" {
		probeKindStr = "http"
	}
	q.ProbeKindStr = probeKindStr

	// Validate probe_kind
	switch probeKindStr {
	case "http":
		q.ProbeKind = domain.ProbeKindHTTP
	case "icmp":
		q.ProbeKind = domain.ProbeKindICMP
	default:
		return q, ErrInvalidProbeKind
	}

	// Parse range_seconds
	if rs := values.Get("range_seconds"); rs != "" {
		if parsed, err := strconv.Atoi(rs); err == nil && parsed > 0 {
			q.RangeSeconds = parsed
		}
	}
	if q.RangeSeconds > MaxRangeSeconds {
		return q, ErrRangeExceedsMaximum
	}

	// Parse step_seconds
	if ss := values.Get("step_seconds"); ss != "" {
		if parsed, err := strconv.Atoi(ss); err == nil && parsed > 0 {
			q.StepSeconds = parsed
		}
	}
	if q.StepSeconds < MinStepSeconds {
		q.StepSeconds = MinStepSeconds
	}
	if q.StepSeconds > MaxStepSeconds {
		return q, ErrStepExceedsMaximum
	}

	// Parse window_seconds
	if ws := values.Get("window_seconds"); ws != "" {
		if parsed, err := strconv.Atoi(ws); err == nil && parsed > 0 {
			q.WindowSeconds = parsed
		}
	}
	if q.WindowSeconds > MaxWindowSeconds {
		q.WindowSeconds = MaxWindowSeconds
	}
	if q.WindowSeconds <= 0 {
		q.WindowSeconds = DefaultWindowSeconds
	}

	return q, nil
}
