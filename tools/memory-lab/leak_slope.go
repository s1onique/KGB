// leak_slope.go — Leak-slope analysis for repeated-request memory workloads
//
// Calculates memory growth slope (KiB/minute) from sampled memory snapshots.
// Slope detection identifies memory leaks under repeated request patterns.
//
// Reference: kgb://doctrine/embedded-memory-frugality

package main

import "time"

// calculateSlopeKiBPerMin calculates the memory growth slope in KiB/minute.
func calculateSlopeKiBPerMin(firstRSS, lastRSS, durationSeconds float64) float64 {
	if durationSeconds <= 0 {
		return 0.0
	}
	growth := lastRSS - firstRSS
	// Convert to KiB/minute: growth / duration_seconds * 60
	return growth / durationSeconds * 60.0
}

// calculateLeakSlopeMetrics computes leak-slope analysis from sampled memory snapshots.
func calculateLeakSlopeMetrics(samples []MemorySnapshot, first, last MemorySnapshot, maxRSS, maxPSS int64, workload HTTPWorkloadResult) *LeakSlopeMetrics {
	durationSeconds := float64(workload.DurationMs) / 1000.0

	// Calculate slopes in KiB/minute
	rssSlope := calculateSlopeKiBPerMin(float64(first.RSSKiB), float64(last.RSSKiB), durationSeconds)
	pssSlope := calculateSlopeKiBPerMin(float64(first.PSSKiB), float64(last.PSSKiB), durationSeconds)

	return &LeakSlopeMetrics{
		SampledPoints:     len(samples),
		DurationSeconds:   durationSeconds,
		RSSFirstKiB:       first.RSSKiB,
		RSSMaxKiB:         maxRSS,
		RSSLastKiB:        last.RSSKiB,
		PSSFirstKiB:       first.PSSKiB,
		PSSMaxKiB:         maxPSS,
		PSSLastKiB:        last.PSSKiB,
		RSSGrowthKiB:      last.RSSKiB - first.RSSKiB,
		PSSGrowthKiB:      last.PSSKiB - first.PSSKiB,
		RSSSlopeKiBPerMin: rssSlope,
		PSSSlopeKiBPerMin: pssSlope,
		RequestCount:      workload.Operations,
		RequestErrors:     workload.Errors,
	}
}

// isLeakSlopeWorkload returns true if the workload type is a leak-slope measurement.
func isLeakSlopeWorkload(wt WorkloadType) bool {
	return wt == WorkloadTovarischLeakSlope ||
		wt == WorkloadTovarischLeakSlopeNetDiag ||
		wt == WorkloadUVB76LeakSlope ||
		wt == WorkloadUVB76LeakSlopeNetDiag
}

// DrainDuration returns how long a leak-slope workload ran based on sample count and interval.
func DrainDuration(sampledPoints int, intervalMs int) time.Duration {
	if sampledPoints <= 1 || intervalMs <= 0 {
		return 0
	}
	return time.Duration(sampledPoints-1) * time.Duration(intervalMs) * time.Millisecond
}
