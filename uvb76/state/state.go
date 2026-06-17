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
	LatencyMs float64   `json:"latency_ms"`
	Reachable bool      `json:"reachable"`
	Error     string    `json:"error,omitempty"`
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
	ErrorCount      int       `json:"error_count"`
	MinLatencyMs    float64   `json:"min_latency_ms"`
	MaxLatencyMs    float64   `json:"max_latency_ms"`
	AvgLatencyMs    float64   `json:"avg_latency_ms"`
	MedianLatencyMs float64   `json:"median_latency_ms"`
	P50LatencyMs    *float64  `json:"p50_latency_ms,omitempty"`
	P90LatencyMs    *float64  `json:"p90_latency_ms,omitempty"`
	P95LatencyMs    *float64  `json:"p95_latency_ms,omitempty"`
	P99LatencyMs    *float64  `json:"p99_latency_ms,omitempty"`
	Histogram       Histogram `json:"histogram"`
}

// PercentilePoint represents a single time-series data point with percentiles.
// Uses null values (not omitted) to indicate missing percentiles in empty windows.
type PercentilePoint struct {
	Timestamp   time.Time `json:"ts"`
	SampleCount int      `json:"sample_count"`
	ErrorCount  int      `json:"error_count"`
	P50Ms       *float64 `json:"p50_ms"`
	P90Ms       *float64 `json:"p90_ms"`
	P95Ms       *float64 `json:"p95_ms"`
	P99Ms       *float64 `json:"p99_ms"`
}

// LatencySeries represents a time-series of latency percentiles for a target.
type LatencySeries struct {
	TargetID             string            `json:"target_id"`
	ProbeKind            string            `json:"probe_kind"`
	ProbeURL             string            `json:"probe_url"`
	IntervalSeconds      int               `json:"interval_seconds"`
	RangeSeconds         int               `json:"range_seconds"`
	StepSeconds          int               `json:"step_seconds"`
	WindowSeconds        int               `json:"window_seconds"`
	RetainedRangeSeconds int               `json:"retained_range_seconds"`
	Points               []PercentilePoint `json:"points"`
}

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

// GetMaxSamples returns the configured max samples per target.
func (m *Manager) GetMaxSamples() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.maxSamples
}

// CalculatePercentiles computes percentiles from a sorted slice of samples.
// Returns nil for any percentile if no valid (successful) samples exist.
func CalculatePercentiles(sortedSamples []float64, percentiles []float64) map[float64]*float64 {
	result := make(map[float64]*float64)
	n := len(sortedSamples)
	if n == 0 {
		for _, p := range percentiles {
			result[p] = nil
		}
		return result
	}

	for _, percentile := range percentiles {
		// Linear interpolation method (same as NIST recommended)
		rank := percentile/100.0*float64(n-1) + 1.0
		k := int(rank)
		d := rank - float64(k)

		if k <= 0 {
			result[percentile] = &sortedSamples[0]
		} else if k >= n {
			result[percentile] = &sortedSamples[n-1]
		} else {
			value := sortedSamples[k-1] + d*(sortedSamples[k]-sortedSamples[k-1])
			result[percentile] = &value
		}
	}
	return result
}
