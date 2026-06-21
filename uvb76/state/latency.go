package state

import (
	"sort"
	"sync"
	"time"
)

// LatencyTracker tracks latency data for a single target.
// Memory is bounded: only stores maxSamples samples in ring buffer.
//
// Thread-safety: All exported methods are safe for concurrent use.
// Writers (RecordAt) hold an exclusive lock.
// Readers (GetSummary, GetRecentSamples, GetSampleTimestamps) hold shared read locks.
type LatencyTracker struct {
	mu            sync.RWMutex
	buckets       []int64         // sorted bucket boundaries in ms (read-only after init)
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

// Record adds a latency sample to the tracker with the current timestamp.
func (lt *LatencyTracker) Record(latencyMs float64, reachable bool) {
	lt.RecordAt(latencyMs, reachable, time.Now().UTC())
}

// validateAndRepairLocked validates and repairs LatencyTracker invariants under lock.
// Returns true if the tracker is in a valid state.
// Returns false ONLY if the tracker is unrecoverable.
//
// INVARIANTS CHECKED AND REPAIRED:
// - maxSamples > 0
// - count >= 0 && count <= maxSamples
// - head >= 0 && head < maxSamples
// - len(recentSamples) == maxSamples
//
// Callers MUST hold lt.mu.
func (lt *LatencyTracker) validateAndRepairLocked() bool {
	// FAIL-CLOSED: Validate invariant: maxSamples > 0
	if lt.maxSamples <= 0 {
		// Cannot operate with invalid buffer size - state is unrecoverable
		return false
	}

	// FAIL-CLOSED: Validate invariant: count >= 0 and count <= maxSamples
	if lt.count < 0 || lt.count > lt.maxSamples {
		lt.count = 0 // Recover by resetting count
	}

	// FAIL-CLOSED: Validate invariant: head >= 0 and head < maxSamples
	if lt.head < 0 || lt.head >= lt.maxSamples {
		lt.head = 0 // Recover by resetting head
	}

	// FAIL-CLOSED: Validate invariant: recentSamples length equals maxSamples
	if len(lt.recentSamples) != lt.maxSamples {
		// Buffer corrupted: resize to match maxSamples
		lt.recentSamples = make([]LatencySample, lt.maxSamples)
		lt.head = 0
		lt.count = 0
		lt.sum = 0
		lt.errorCount = 0
	}

	// Tracker is valid after any repairs
	return true
}

// snapshotLimitLocked clamps the requested limit to valid bounds.
// Ensures no allocation can exceed actual buffer capacity.
//
// Callers MUST hold lt.mu.
func (lt *LatencyTracker) snapshotLimitLocked(requestedLimit int) int {
	if requestedLimit <= 0 {
		return 0
	}

	// Clamp to buffer capacity
	capacity := len(lt.recentSamples)
	if capacity == 0 {
		return 0
	}

	// Clamp to actual count
	count := lt.count
	if count <= 0 {
		return 0
	}

	// Return minimum of requested, count, and capacity
	limit := requestedLimit
	if limit > count {
		limit = count
	}
	if limit > capacity {
		limit = capacity
	}

	return limit
}

// RecordAt adds a latency sample with a specific timestamp.
// This is intended for deterministic testing; prefer Record in production.
//
// FAIL-CLOSED INVARIANT GUARDS:
// - Validates all ring buffer state before mutation
// - Returns early without mutating state if invariants are violated
// - This prevents heap corruption from propagating to makeslice/memclr paths
func (lt *LatencyTracker) RecordAt(latencyMs float64, reachable bool, timestamp time.Time) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	// Validate and repair invariants before any mutation
	if !lt.validateAndRepairLocked() {
		// Invariants unrecoverable - state is corrupted
		return
	}

	// Store in ring buffer
	sample := LatencySample{
		Timestamp: timestamp,
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

	// Final bounds check before mutation
	if lt.head >= 0 && lt.head < len(lt.recentSamples) {
		lt.recentSamples[lt.head] = sample
	} else {
		return // Cannot write: head index out of bounds
	}

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
// Thread-safe: holds read lock, allowing concurrent readers while writers are excluded.
func (lt *LatencyTracker) GetSummary(targetID string) LatencySummary {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

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

// GetRecentSamples returns the most recent `limit` latency samples in chronological order.
// Thread-safe: acquires read lock, clamps limit, and returns a defensive copy.
// Never returns internal backing storage.
//
// INVARIANT: All ring-buffer state (head, count, maxSamples, samples) is read-only
// while the lock is held. This prevents SIGSEGV from concurrent state mutation during
// the makeslice/loop that would corrupt the slice header or array pointer.
//
// FAIL-CLOSED: If tracker invariants are broken (corrupted head/count/capacity),
// this returns nil instead of allocating from corrupt values. This prevents
// SIGSEGV from heap/memory corruption on constrained routers.
//
// NOTE: This is a READ-ONLY operation. It does NOT repair tracker state.
func (lt *LatencyTracker) GetRecentSamples(limit int) []LatencySample {
	if lt == nil || limit <= 0 {
		return nil
	}

	lt.mu.RLock()
	defer lt.mu.RUnlock()

	// Validate read-path invariants (NO state modification)
	capacity := len(lt.recentSamples)
	if capacity == 0 || lt.maxSamples <= 0 {
		return nil
	}

	// FAIL-CLOSED: Verify buffer length matches declared maxSamples
	// This catches corruption from external writes, bad type assertions, etc.
	if capacity != lt.maxSamples {
		return nil
	}

	if lt.count <= 0 {
		return nil
	}

	// FAIL-CLOSED: Verify count is in valid range
	if lt.count < 0 || lt.count > lt.maxSamples {
		return nil
	}

	// Validate head is in valid range
	head := lt.head
	if head < 0 || head >= capacity {
		// Crash containment: invariant violation indicates corruption
		// Return nil to avoid SIGSEGV during makeslice
		return nil
	}

	// Use shared helper for limit clamping
	count := lt.snapshotLimitLocked(limit)
	if count <= 0 {
		return nil
	}

	// Allocate defensive copy (caller-owned, no shared backing array)
	out := make([]LatencySample, count)
	start := (head - count + capacity) % capacity
	for i := 0; i < count; i++ {
		idx := (start + i) % capacity
		out[i] = lt.recentSamples[idx]
	}

	return out
}

// GetSampleTimestamps returns the oldest and newest sample timestamps.
// Returns copies to avoid returning pointers into mutable ring buffer storage.
// Thread-safe: holds read lock, allowing concurrent readers while writers are excluded.
func (lt *LatencyTracker) GetSampleTimestamps() (oldest, newest *time.Time) {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	if lt.count == 0 {
		return nil, nil
	}

	// Oldest sample is at position (head - count) mod maxSamples
	oldestIdx := (lt.head - lt.count + lt.maxSamples) % lt.maxSamples
	oldestValue := lt.recentSamples[oldestIdx].Timestamp
	oldest = &oldestValue

	// Newest sample is at position (head - 1) mod maxSamples
	newestIdx := (lt.head - 1 + lt.maxSamples) % lt.maxSamples
	newestValue := lt.recentSamples[newestIdx].Timestamp
	newest = &newestValue

	return oldest, newest
}
