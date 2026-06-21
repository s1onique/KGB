package state

import (
	"time"
)

// LatencySnapshot represents an immutable snapshot of LatencyTracker state.
// This is the preferred read primitive for external consumers (handlers, tests).
// All data is caller-owned; no shared backing storage with the tracker.
type LatencySnapshot struct {
	TargetID        string
	Samples        []LatencySample // caller-owned copy
	Count          int
	Capacity       int
	Sum            float64
	ErrorCount     int
	OldestSampleTs *time.Time
	NewestSampleTs *time.Time
	Buckets        []int64 // copy of bucket boundaries
}

// Snapshot returns a complete immutable snapshot of tracker state.
// This is the preferred single primitive for latency series handlers.
//
// All returned slices and data are caller-owned. No shared backing storage
// with the tracker's ring buffer. Suitable for concurrent UI/API reads
// while probes continue recording samples.
//
// Thread-safe: holds read lock for entire snapshot construction.
func (lt *LatencyTracker) Snapshot(targetID string, limit int) *LatencySnapshot {
	if lt == nil {
		return nil
	}

	lt.mu.RLock()
	defer lt.mu.RUnlock()

	snap := &LatencySnapshot{
		TargetID:   targetID,
		Capacity:   lt.maxSamples,
		Sum:        lt.sum,
		ErrorCount: lt.errorCount,
		Buckets:    make([]int64, len(lt.buckets)),
	}

	copy(snap.Buckets, lt.buckets)

	// Validate read-path invariants
	if lt.count == 0 || lt.maxSamples <= 0 || len(lt.recentSamples) != lt.maxSamples {
		return snap // Return empty snapshot
	}

	// Validate count and head bounds
	if lt.count < 0 || lt.count > lt.maxSamples {
		return snap // Return empty snapshot (fail-closed)
	}
	if lt.head < 0 || lt.head >= lt.maxSamples {
		return snap // Return empty snapshot (fail-closed)
	}

	// Clamp limit under lock
	clampedLimit := limit
	if clampedLimit <= 0 {
		clampedLimit = lt.count
	}
	if clampedLimit > lt.count {
		clampedLimit = lt.count
	}
	if clampedLimit > lt.maxSamples {
		clampedLimit = lt.maxSamples
	}

	snap.Count = clampedLimit

	// Copy timestamps while holding lock
	if lt.count > 0 {
		oldestIdx := (lt.head - lt.count + lt.maxSamples) % lt.maxSamples
		oldestCopy := lt.recentSamples[oldestIdx].Timestamp
		snap.OldestSampleTs = &oldestCopy

		newestIdx := (lt.head - 1 + lt.maxSamples) % lt.maxSamples
		newestCopy := lt.recentSamples[newestIdx].Timestamp
		snap.NewestSampleTs = &newestCopy
	}

	// Copy samples while holding lock
	if clampedLimit > 0 {
		snap.Samples = make([]LatencySample, clampedLimit)
		start := (lt.head - clampedLimit + lt.maxSamples) % lt.maxSamples
		for i := 0; i < clampedLimit; i++ {
			idx := (start + i) % lt.maxSamples
			snap.Samples[i] = lt.recentSamples[idx]
		}
	}

	return snap
}
