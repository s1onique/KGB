// Package domain provides immutable analysis types.
package domain

import (
	"time"
)

// SampleWindow is an immutable analysis snapshot of latency samples.
type SampleWindow struct {
	samples []Sample
}

// Sample represents a single latency sample in the domain.
type Sample struct {
	At      time.Time
	Latency LatencyMillis
	OK      bool
	Err     string
}

// LatencyMillis wraps a float64 latency value.
type LatencyMillis struct {
	value float64
}

// Len returns the number of samples in the window.
func (w SampleWindow) Len() int {
	return len(w.samples)
}

// Samples returns a defensive copy of the samples slice.
func (w SampleWindow) Samples() []Sample {
	if len(w.samples) == 0 {
		return nil
	}
	cp := make([]Sample, len(w.samples))
	copy(cp, w.samples)
	return cp
}
