// Package server is an external consumer of state.
package server

import "github.com/s1onique/KGB/uvb76/state"

// BadRecentSamples demonstrates disallowed GetRecentSamples call outside state package.
func BadRecentSamples(t *state.LatencyTracker) int {
	samples := t.GetRecentSamples(100) // want "use LatencyTracker.GetSampleWindow or Manager.Get{HTTP,ICMP}SampleWindow for analysis; raw GetRecentSamples is owned by uvb76/state"
	return len(samples)
}

// BadExposeSamples demonstrates disallowed return of raw state samples.
func BadExposeSamples(t *state.LatencyTracker) []state.LatencySample { // want "do not expose \\[\\]state.LatencySample outside uvb76/state; expose domain.SampleWindow or API DTOs instead"
	return nil
}
