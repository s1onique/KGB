// Package domain provides refined domain types for latency, spike detection, and capture decisions.
// This package enforces invariants at construction time and provides pure functions for analysis.
package domain

import (
	"math"
	"time"
)

// ProbeKind classifies the type of probe used to measure latency.
type ProbeKind string

const (
	ProbeKindHTTP ProbeKind = "http"
	ProbeKindICMP ProbeKind = "icmp"
)

// LatencyMillis represents a validated successful latency measurement in milliseconds.
// NaN, Inf, and negative values cannot be represented by this type.
type LatencyMillis struct {
	v float64
}

// NewLatencyMillis constructs a LatencyMillis from a raw float64 value.
// Returns false for NaN, Inf, or negative values.
// Zero latency is accepted as a valid measurement.
func NewLatencyMillis(v float64) (LatencyMillis, bool) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return LatencyMillis{}, false
	}
	return LatencyMillis{v: v}, true
}

// Float64 returns the raw float64 value.
func (l LatencyMillis) Float64() float64 {
	return l.v
}

// Sample represents a single latency measurement with provenance.
type Sample struct {
	At      time.Time
	Kind    ProbeKind
	Latency LatencyMillis
	OK      bool
	Err     string
}

// SampleFromState converts a state.LatencySample to a domain.Sample.
// Returns false if the sample is successful but has invalid latency.
func SampleFromState(ts time.Time, latencyMs float64, reachable bool, probeKind ProbeKind) (Sample, bool) {
	s := Sample{
		At:   ts,
		Kind: probeKind,
		OK:   reachable,
	}
	if reachable {
		lat, ok := NewLatencyMillis(latencyMs)
		if !ok {
			return Sample{}, false
		}
		s.Latency = lat
	}
	return s, true
}
