package domain

import (
	"sort"
)

// SampleWindow is an immutable analysis snapshot of latency samples.
// It must not share mutable LatencyTracker ring-buffer backing storage.
type SampleWindow struct {
	samples []Sample
}

// NewSampleWindow creates a new SampleWindow from a slice of samples.
// Only successful samples (OK=true) are included in the window.
// Failed samples are excluded from median/percentile calculations.
func NewSampleWindow(samples []Sample) SampleWindow {
	cp := make([]Sample, 0, len(samples))
	for _, s := range samples {
		if s.OK {
			cp = append(cp, s)
		}
	}
	return SampleWindow{samples: cp}
}

// Len returns the number of successful samples in the window.
func (w SampleWindow) Len() int {
	return len(w.samples)
}

// Samples returns a defensive copy of the samples slice.
// Mutations to the returned slice do not affect the window.
func (w SampleWindow) Samples() []Sample {
	if len(w.samples) == 0 {
		return nil
	}
	cp := make([]Sample, len(w.samples))
	copy(cp, w.samples)
	return cp
}

// Median returns the median latency of successful samples.
// Returns ok=false if the window is empty.
func (w SampleWindow) Median() (LatencyMillis, bool) {
	if len(w.samples) == 0 {
		return LatencyMillis{}, false
	}

	values := make([]float64, 0, len(w.samples))
	for _, s := range w.samples {
		values = append(values, s.Latency.Float64())
	}
	sort.Float64s(values)

	mid := len(values) / 2
	if len(values)%2 == 1 {
		return NewLatencyMillis(values[mid])
	}

	// Even count: average of two middle values
	v, ok := NewLatencyMillis((values[mid-1] + values[mid]) / 2)
	return v, ok
}

// P50 returns the 50th percentile (median) latency.
// This is an alias for Median for clarity.
func (w SampleWindow) P50() (LatencyMillis, bool) {
	return w.Median()
}

// P90 returns the 90th percentile latency.
// Returns ok=false if the window has fewer than 10 samples.
func (w SampleWindow) P90() (LatencyMillis, bool) {
	return w.percentile(90)
}

// P95 returns the 95th percentile latency.
// Returns ok=false if the window has fewer than 20 samples.
func (w SampleWindow) P95() (LatencyMillis, bool) {
	return w.percentile(95)
}

// P99 returns the 99th percentile latency.
// Returns ok=false if the window has fewer than 100 samples.
func (w SampleWindow) P99() (LatencyMillis, bool) {
	return w.percentile(99)
}

// percentile returns the specified percentile latency using linear interpolation.
func (w SampleWindow) percentile(p float64) (LatencyMillis, bool) {
	n := len(w.samples)
	if n == 0 {
		return LatencyMillis{}, false
	}

	values := make([]float64, 0, n)
	for _, s := range w.samples {
		values = append(values, s.Latency.Float64())
	}
	sort.Float64s(values)

	// Linear interpolation method (NIST recommended)
	rank := p/100.0*float64(n-1) + 1.0
	k := int(rank)
	d := rank - float64(k)

	var value float64
	if k <= 0 {
		value = values[0]
	} else if k >= n {
		value = values[n-1]
	} else {
		value = values[k-1] + d*(values[k]-values[k-1])
	}

	return NewLatencyMillis(value)
}
