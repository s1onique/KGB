// Package state provides bounded in-memory state management for UVB-76.
package state

import (
	"sync"
	"time"
)

// TargetSnapshot represents the latest status received from a tovarisch target.
type TargetSnapshot struct {
	TargetID    string            `json:"target_id"`
	ScrapedAt   time.Time         `json:"scraped_at"`
	Reachable   bool              `json:"reachable"`
	Status      string            `json:"status,omitempty"`
	Version     string            `json:"version,omitempty"`
	NodeID      string            `json:"node_id,omitempty"`
	Checks      []CheckResult     `json:"checks,omitempty"`
	Error       string            `json:"error,omitempty"`
	RawResponse string            `json:"raw_response,omitempty"`
}

// CheckResult represents a single health check result.
type CheckResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// LatencySample represents a single latency measurement.
type LatencySample struct {
	Timestamp time.Time `json:"timestamp"`
	LatencyMs float64  `json:"latency_ms"`
	Reachable bool     `json:"reachable"`
}

// Histogram holds bounded histogram data for latency measurements.
type Histogram struct {
	Buckets []int64 `json:"buckets"`
	Counts  []int64 `json:"counts"`
}

// LatencySummary provides statistical summary of latency data.
type LatencySummary struct {
	TargetID        string    `json:"target_id"`
	SampleCount     int       `json:"sample_count"`
	MinLatencyMs    float64   `json:"min_latency_ms"`
	MaxLatencyMs    float64   `json:"max_latency_ms"`
	AvgLatencyMs    float64   `json:"avg_latency_ms"`
	MedianLatencyMs float64   `json:"median_latency_ms"`
	Histogram       Histogram `json:"histogram"`
}

// LatencyTracker tracks latency data for a single target.
// Memory is bounded: only stores maxSamples samples in ring buffer.
// Stats are derived from the current ring buffer contents only.
type LatencyTracker struct {
	mu            sync.Mutex
	buckets       []int64         // sorted bucket boundaries in ms
	recentSamples []LatencySample // ring buffer
	maxSamples   int              // max capacity (must be > 0)
	head          int              // next write position
	count         int              // total samples currently stored
	sum           float64          // sum of current samples in buffer
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

	// If we're at capacity, subtract the value being overwritten from sum
	if lt.count == lt.maxSamples {
		oldSample := lt.recentSamples[lt.head]
		lt.sum -= oldSample.LatencyMs
	}

	lt.recentSamples[lt.head] = sample
	lt.head = (lt.head + 1) % lt.maxSamples
	if lt.count < lt.maxSamples {
		lt.count++
	}

	// Update sum
	lt.sum += latencyMs
}

// GetSummary returns a latency summary for graph display.
// Stats are derived from the current ring buffer contents only (bounded).
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

	// Extract current samples from ring buffer for sorting
	currentSamples := make([]float64, lt.count)
	for i := 0; i < lt.count; i++ {
		idx := (lt.head - lt.count + i + lt.maxSamples) % lt.maxSamples
		currentSamples[i] = lt.recentSamples[idx].LatencyMs
	}

	// Calculate stats from current samples
	summary.MinLatencyMs = currentSamples[0]
	summary.MaxLatencyMs = currentSamples[0]

	// Simple sort for min/max/median
	for i := 1; i < len(currentSamples); i++ {
		if currentSamples[i] < summary.MinLatencyMs {
			summary.MinLatencyMs = currentSamples[i]
		}
		if currentSamples[i] > summary.MaxLatencyMs {
			summary.MaxLatencyMs = currentSamples[i]
		}
		// Insertion sort for median
		j := i
		for j > 0 && currentSamples[j-1] > currentSamples[j] {
			currentSamples[j-1], currentSamples[j] = currentSamples[j], currentSamples[j-1]
			j--
		}
	}

	// Average
	summary.AvgLatencyMs = lt.sum / float64(lt.count)

	// Median
	mid := lt.count / 2
	if lt.count%2 == 0 {
		summary.MedianLatencyMs = (currentSamples[mid-1] + currentSamples[mid]) / 2
	} else {
		summary.MedianLatencyMs = currentSamples[mid]
	}

	// Histogram counts
	for _, val := range currentSamples {
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

// Manager manages the bounded state for all targets.
type Manager struct {
	mu              sync.RWMutex
	snapshots        map[string]*TargetSnapshot       // keyed by target ID
	latencyTrackers map[string]*LatencyTracker      // keyed by target ID
	buckets         []int64                          // histogram bucket boundaries
	maxSamples      int                             // max recent samples per target
}

// NewManager creates a new state manager with bounded capacity.
func NewManager() *Manager {
	return &Manager{
		snapshots:        make(map[string]*TargetSnapshot),
		latencyTrackers: make(map[string]*LatencyTracker),
		buckets:         []int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000},
		maxSamples:      100,
	}
}

// NewManagerWithConfig creates a new state manager with custom latency config.
func NewManagerWithConfig(buckets []int64, maxSamples int) *Manager {
	if maxSamples <= 0 {
		maxSamples = 100
	}
	if len(buckets) == 0 {
		buckets = []int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}
	}
	m := NewManager()
	m.buckets = buckets
	m.maxSamples = maxSamples
	return m
}

// UpdateSnapshot stores the latest snapshot for a target.
func (m *Manager) UpdateSnapshot(targetID string, snap *TargetSnapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snapshots[targetID] = snap
}

// GetSnapshot returns the latest snapshot for a target.
func (m *Manager) GetSnapshot(targetID string) *TargetSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshots[targetID]
}

// GetAllSnapshots returns all stored snapshots.
func (m *Manager) GetAllSnapshots() map[string]*TargetSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]*TargetSnapshot, len(m.snapshots))
	for k, v := range m.snapshots {
		result[k] = v
	}
	return result
}

// GetSnapshotCount returns the number of stored snapshots.
func (m *Manager) GetSnapshotCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.snapshots)
}

// RecordLatency records a latency measurement for a target.
func (m *Manager) RecordLatency(targetID string, latencyMs float64, reachable bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tracker, exists := m.latencyTrackers[targetID]
	if !exists {
		tracker = NewLatencyTracker(m.buckets, m.maxSamples)
		m.latencyTrackers[targetID] = tracker
	}
	tracker.Record(latencyMs, reachable)
}

// GetLatencySummary returns the latency summary for a target.
func (m *Manager) GetLatencySummary(targetID string) LatencySummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tracker := m.latencyTrackers[targetID]
	if tracker == nil {
		return LatencySummary{TargetID: targetID, Histogram: Histogram{Buckets: m.buckets}}
	}
	return tracker.GetSummary(targetID)
}

// GetRecentLatencySamples returns the recent latency samples for a target.
func (m *Manager) GetRecentLatencySamples(targetID string, limit int) []LatencySample {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tracker := m.latencyTrackers[targetID]
	if tracker == nil {
		return []LatencySample{}
	}
	// Clamp limit to valid range
	if limit <= 0 {
		limit = m.maxSamples
	} else if limit > m.maxSamples {
		limit = m.maxSamples
	}
	return tracker.GetRecentSamples(limit)
}

// GetAllLatencySummaries returns latency summaries for all tracked targets.
func (m *Manager) GetAllLatencySummaries() map[string]LatencySummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]LatencySummary)
	for targetID, tracker := range m.latencyTrackers {
		result[targetID] = tracker.GetSummary(targetID)
	}
	return result
}

// GetAllTargetSummaries returns latency summaries for all configured targets.
// Includes targets with zero samples (stable API shape on fresh boot).
func (m *Manager) GetAllTargetSummaries(targetIDs []string) map[string]LatencySummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]LatencySummary, len(targetIDs))
	buckets := make([]int64, len(m.buckets))
	copy(buckets, m.buckets)

	for _, targetID := range targetIDs {
		tracker := m.latencyTrackers[targetID]
		if tracker != nil {
			result[targetID] = tracker.GetSummary(targetID)
		} else {
			// Return empty summary with proper bucket structure
			result[targetID] = LatencySummary{
				TargetID:    targetID,
				SampleCount: 0,
				Histogram: Histogram{
					Buckets: buckets,
					Counts: make([]int64, len(buckets)),
				},
			}
		}
	}
	return result
}

// GetLatencyBuckets returns the configured histogram buckets.
func (m *Manager) GetLatencyBuckets() []int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	buckets := make([]int64, len(m.buckets))
	copy(buckets, m.buckets)
	return buckets
}
