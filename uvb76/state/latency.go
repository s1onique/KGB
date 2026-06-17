package state

import (
	"sort"
	"sync"
	"time"
)

// LatencyTracker tracks latency data for a single target.
// Memory is bounded: only stores maxSamples samples in ring buffer.
type LatencyTracker struct {
	mu            sync.Mutex
	buckets       []int64         // sorted bucket boundaries in ms
	recentSamples []LatencySample // ring buffer
	maxSamples   int              // max capacity
	head          int              // next write position
	count         int              // total samples currently stored
	sum           float64          // sum of current samples in buffer
	errorCount    int              // count of failed (unreachable) samples
}

// NewLatencyTracker creates a new latency tracker with given buckets and max samples.
// Panics if buckets are empty or maxSamples is <= 0.
func NewLatencyTracker(buckets []int64, maxSamples int) *LatencyTracker {
	if maxSamples <= 0 {
		panic("maxSamples must be > 0")
	}
	if len(buckets) == 0 {
		panic("buckets cannot be empty")
	}
	// Ensure buckets are sorted
	sortedBuckets := make([]int64, len(buckets))
	copy(sortedBuckets, buckets)
	for i := 1; i < len(sortedBuckets); i++ {
		for j := 0; j < len(sortedBuckets)-i; j++ {
			if sortedBuckets[j] > sortedBuckets[j+1] {
				sortedBuckets[j], sortedBuckets[j+1] = sortedBuckets[j+1], sortedBuckets[j]
			}
		}
	}

	return &LatencyTracker{
		buckets:       sortedBuckets,
		recentSamples: make([]LatencySample, maxSamples),
		maxSamples:   maxSamples,
	}
}

// Record adds a latency sample to the tracker.
func (lt *LatencyTracker) Record(latencyMs float64, reachable bool) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	// Store in ring buffer
	sample := LatencySample{
		Timestamp: time.Now().UTC(),
		LatencyMs: latencyMs,
		Reachable: reachable,
	}

	// Track error count
	if !reachable {
		lt.errorCount++
	}

	// If we're at capacity, only subtract from sum if the old sample was successful
	// (failed samples were never added to sum, so subtracting would corrupt average)
	if lt.count == lt.maxSamples {
		oldSample := lt.recentSamples[lt.head]
		if oldSample.Reachable {
			lt.sum -= oldSample.LatencyMs
		} else {
			lt.errorCount--
		}
	}

	lt.recentSamples[lt.head] = sample
	lt.head = (lt.head + 1) % lt.maxSamples
	if lt.count < lt.maxSamples {
		lt.count++
	}

	// Update sum (only for successful probes)
	if reachable {
		lt.sum += latencyMs
	}
}

// GetSummary returns a latency summary for graph display.
// Stats are derived from the current ring buffer contents only (bounded).
// Failed probes are counted separately but excluded from percentile calculations.
func (lt *LatencyTracker) GetSummary(targetID string) LatencySummary {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	summary := LatencySummary{
		TargetID:  targetID,
		Histogram: Histogram{Buckets: lt.buckets, Counts: make([]int64, len(lt.buckets))},
	}

	if lt.count == 0 {
		return summary
	}

	summary.SampleCount = lt.count
	summary.ErrorCount = lt.errorCount

	// Extract successful samples from ring buffer for percentile calculations
	successfulSamples := make([]float64, 0, lt.count-lt.errorCount)
	for i := 0; i < lt.count; i++ {
		idx := (lt.head - lt.count + i + lt.maxSamples) % lt.maxSamples
		sample := lt.recentSamples[idx]
		if sample.Reachable {
			successfulSamples = append(successfulSamples, sample.LatencyMs)
		}
	}

	// Extract all samples (for histogram, min, max)
	allSamples := make([]float64, lt.count)
	successCount := 0
	for i := 0; i < lt.count; i++ {
		idx := (lt.head - lt.count + i + lt.maxSamples) % lt.maxSamples
		sample := lt.recentSamples[idx]
		allSamples[i] = sample.LatencyMs
		if sample.Reachable {
			successCount++
		}
	}

	if successCount == 0 {
		// All samples failed - set defaults
		summary.MinLatencyMs = 0
		summary.MaxLatencyMs = 0
		summary.AvgLatencyMs = 0
		summary.MedianLatencyMs = 0
		// Histogram still counts all samples
		for _, val := range allSamples {
			placed := false
			for i, bucket := range lt.buckets {
				if val <= float64(bucket) {
					summary.Histogram.Counts[i]++
					placed = true
					break
				}
			}
			if !placed {
				summary.Histogram.Counts[len(lt.buckets)-1]++
			}
		}
		return summary
	}

	// Calculate stats from all samples
	summary.MinLatencyMs = allSamples[0]
	summary.MaxLatencyMs = allSamples[0]

	// Simple sort for min/max/median (include all samples)
	for i := 1; i < len(allSamples); i++ {
		if allSamples[i] < summary.MinLatencyMs {
			summary.MinLatencyMs = allSamples[i]
		}
		if allSamples[i] > summary.MaxLatencyMs {
			summary.MaxLatencyMs = allSamples[i]
		}
		// Insertion sort for median
		j := i
		for j > 0 && allSamples[j-1] > allSamples[j] {
			allSamples[j-1], allSamples[j] = allSamples[j], allSamples[j-1]
			j--
		}
	}

	// Average (successful samples only)
	summary.AvgLatencyMs = lt.sum / float64(successCount)

	// Median (successful samples only - sort them)
	sort.Float64s(successfulSamples)
	mid := len(successfulSamples) / 2
	if len(successfulSamples)%2 == 0 {
		summary.MedianLatencyMs = (successfulSamples[mid-1] + successfulSamples[mid]) / 2
	} else {
		summary.MedianLatencyMs = successfulSamples[mid]
	}

	// Calculate percentiles (p50, p90, p95, p99)
	percentiles := CalculatePercentiles(successfulSamples, []float64{50, 90, 95, 99})
	if p50, ok := percentiles[50]; ok {
		summary.P50LatencyMs = p50
	}
	if p90, ok := percentiles[90]; ok {
		summary.P90LatencyMs = p90
	}
	if p95, ok := percentiles[95]; ok {
		summary.P95LatencyMs = p95
	}
	if p99, ok := percentiles[99]; ok {
		summary.P99LatencyMs = p99
	}

	// Histogram counts (all samples)
	for _, val := range allSamples {
		placed := false
		for i, bucket := range lt.buckets {
			if val <= float64(bucket) {
				summary.Histogram.Counts[i]++
				placed = true
				break
			}
		}
		// If value exceeds last bucket, put in last bucket
		if !placed {
			summary.Histogram.Counts[len(lt.buckets)-1]++
		}
	}

	return summary
}

// GetRecentSamples returns the recent latency samples in chronological order.
func (lt *LatencyTracker) GetRecentSamples(limit int) []LatencySample {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	if limit <= 0 || limit > lt.count {
		limit = lt.count
	}

	samples := make([]LatencySample, limit)
	start := lt.head - limit
	if start < 0 {
		start += lt.maxSamples
	}

	for i := 0; i < limit; i++ {
		idx := (start + i) % lt.maxSamples
		samples[i] = lt.recentSamples[idx]
	}

	return samples
}
