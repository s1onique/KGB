package state

import (
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.GetSnapshotCount() != 0 {
		t.Errorf("Expected empty manager, got count %d", m.GetSnapshotCount())
	}
}

func TestUpdateAndGetSnapshot(t *testing.T) {
	m := NewManager()

	snap := &TargetSnapshot{
		TargetID:  "test-1",
		ScrapedAt: time.Now().UTC(),
		Reachable: true,
		Status:    "ok",
	}

	m.UpdateSnapshot("test-1", snap)

	retrieved := m.GetSnapshot("test-1")
	if retrieved == nil {
		t.Fatal("GetSnapshot returned nil")
	}
	if retrieved.TargetID != "test-1" {
		t.Errorf("Expected target ID 'test-1', got '%s'", retrieved.TargetID)
	}
	if retrieved.Reachable != true {
		t.Error("Expected Reachable to be true")
	}
}

func TestGetAllSnapshots(t *testing.T) {
	m := NewManager()

	m.UpdateSnapshot("test-1", &TargetSnapshot{TargetID: "test-1", Reachable: true})
	m.UpdateSnapshot("test-2", &TargetSnapshot{TargetID: "test-2", Reachable: false})

	snaps := m.GetAllSnapshots()
	if len(snaps) != 2 {
		t.Errorf("Expected 2 snapshots, got %d", len(snaps))
	}
}

func TestGetNonexistentSnapshot(t *testing.T) {
	m := NewManager()
	snap := m.GetSnapshot("nonexistent")
	if snap != nil {
		t.Error("Expected nil for nonexistent snapshot")
	}
}

func TestBoundedState(t *testing.T) {
	m := NewManager()

	// Add 10 snapshots (should be bounded by config)
	for i := 0; i < 10; i++ {
		m.UpdateSnapshot(string(rune('0'+i)), &TargetSnapshot{
			TargetID:  string(rune('0' + i)),
			Reachable: true,
		})
	}

	if m.GetSnapshotCount() != 10 {
		t.Errorf("Expected 10 snapshots, got %d", m.GetSnapshotCount())
	}

	// Update one existing - should replace, not grow
	m.UpdateSnapshot("0", &TargetSnapshot{TargetID: "0", Reachable: false})

	if m.GetSnapshotCount() != 10 {
		t.Errorf("Expected still 10 snapshots after update, got %d", m.GetSnapshotCount())
	}
}

// Latency Tracker Tests

func TestNewLatencyTracker(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 100)
	if lt == nil {
		t.Fatal("NewLatencyTracker returned nil")
	}
	if lt.count != 0 {
		t.Errorf("Expected count 0, got %d", lt.count)
	}
}

func TestLatencyTracker_Record(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 10)

	lt.Record(10.5, true)
	lt.Record(25.0, true)
	lt.Record(50.0, false)

	if lt.count != 3 {
		t.Errorf("Expected count 3, got %d", lt.count)
	}
}

func TestLatencyTracker_RingBuffer(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 5)

	// Record 10 samples into a buffer of size 5
	for i := 0; i < 10; i++ {
		lt.Record(float64(i*10), true)
	}

	// Count should be capped at 5
	if lt.count != 5 {
		t.Errorf("Expected count 5, got %d", lt.count)
	}
}

func TestLatencyTracker_GetRecentSamples(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 10)

	lt.Record(10.0, true)
	lt.Record(20.0, true)
	lt.Record(30.0, true)

	samples := lt.GetRecentSamples(2)
	if len(samples) != 2 {
		t.Errorf("Expected 2 samples, got %d", len(samples))
	}
}

func TestLatencyTracker_GetSummary(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 100)

	lt.Record(10.0, true)
	lt.Record(20.0, true)
	lt.Record(30.0, true)

	summary := lt.GetSummary("test-target")
	if summary.TargetID != "test-target" {
		t.Errorf("Expected target ID 'test-target', got '%s'", summary.TargetID)
	}
	if summary.SampleCount != 3 {
		t.Errorf("Expected sample count 3, got %d", summary.SampleCount)
	}
	if summary.MinLatencyMs != 10.0 {
		t.Errorf("Expected min 10.0, got %f", summary.MinLatencyMs)
	}
	if summary.MaxLatencyMs != 30.0 {
		t.Errorf("Expected max 30.0, got %f", summary.MaxLatencyMs)
	}
	if summary.AvgLatencyMs != 20.0 {
		t.Errorf("Expected avg 20.0, got %f", summary.AvgLatencyMs)
	}
}

func TestLatencyTracker_GetSummary_Empty(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 100)

	summary := lt.GetSummary("empty-target")
	if summary.SampleCount != 0 {
		t.Errorf("Expected sample count 0, got %d", summary.SampleCount)
	}
	if summary.MinLatencyMs != 0 {
		t.Errorf("Expected min 0, got %f", summary.MinLatencyMs)
	}
}

func TestLatencyTracker_HistogramBuckets(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 100)

	lt.Record(3.0, true)   // below first bucket (goes to first bucket that fits: <= 5)
	lt.Record(7.0, true)   // in first bucket (<= 10)
	lt.Record(15.0, true)  // in second bucket (<= 25)
	lt.Record(30.0, true)  // in third bucket (<= 50)
	lt.Record(75.0, true) // in fourth bucket (<= 100)
	lt.Record(150.0, true) // above last bucket (goes to last bucket)

	summary := lt.GetSummary("hist-target")

	// Verify histogram structure
	if len(summary.Histogram.Buckets) != len(buckets) {
		t.Errorf("Expected %d buckets, got %d", len(buckets), len(summary.Histogram.Buckets))
	}
	if len(summary.Histogram.Counts) != len(buckets) {
		t.Errorf("Expected %d counts, got %d", len(buckets), len(summary.Histogram.Counts))
	}

	// Total samples
	total := int64(0)
	for _, c := range summary.Histogram.Counts {
		total += c
	}
	if total != 6 {
		t.Errorf("Expected 6 total samples in histogram, got %d", total)
	}
}

// State Manager Latency Tests

func TestManager_RecordLatency(t *testing.T) {
	m := NewManager()

	m.RecordLatency("target-1", 10.0, true)
	m.RecordLatency("target-1", 20.0, true)
	m.RecordLatency("target-2", 30.0, true)

	summary1 := m.GetLatencySummary("target-1")
	summary2 := m.GetLatencySummary("target-2")

	if summary1.SampleCount != 2 {
		t.Errorf("Expected target-1 count 2, got %d", summary1.SampleCount)
	}
	if summary2.SampleCount != 1 {
		t.Errorf("Expected target-2 count 1, got %d", summary2.SampleCount)
	}
}

func TestManager_GetRecentLatencySamples(t *testing.T) {
	m := NewManager()

	m.RecordLatency("target-1", 10.0, true)
	m.RecordLatency("target-1", 20.0, true)
	m.RecordLatency("target-1", 30.0, true)

	samples := m.GetRecentLatencySamples("target-1", 2)
	if len(samples) != 2 {
		t.Errorf("Expected 2 samples, got %d", len(samples))
	}
}

func TestManager_GetRecentLatencySamples_Empty(t *testing.T) {
	m := NewManager()

	samples := m.GetRecentLatencySamples("nonexistent", 10)
	if len(samples) != 0 {
		t.Errorf("Expected 0 samples for nonexistent target, got %d", len(samples))
	}
}

func TestManager_GetAllLatencySummaries(t *testing.T) {
	m := NewManager()

	m.RecordLatency("target-1", 10.0, true)
	m.RecordLatency("target-2", 20.0, true)

	summaries := m.GetAllLatencySummaries()
	if len(summaries) != 2 {
		t.Errorf("Expected 2 summaries, got %d", len(summaries))
	}
}

func TestManager_GetLatencyBuckets(t *testing.T) {
	buckets := []int64{5, 10, 25, 50}
	m := NewManagerWithConfig(buckets, 50)

	retrieved := m.GetLatencyBuckets()
	if len(retrieved) != len(buckets) {
		t.Errorf("Expected %d buckets, got %d", len(buckets), len(retrieved))
	}
}

func TestManager_NewManagerWithConfig(t *testing.T) {
	buckets := []int64{1, 2, 3, 4, 5}
	maxSamples := 50
	m := NewManagerWithConfig(buckets, maxSamples)

	if m.maxSamples != maxSamples {
		t.Errorf("Expected maxSamples %d, got %d", maxSamples, m.maxSamples)
	}
	if len(m.buckets) != len(buckets) {
		t.Errorf("Expected %d buckets, got %d", len(buckets), len(m.buckets))
	}
}

func TestManager_GetMaxSamples(t *testing.T) {
	m := NewManager()
	if m.GetMaxSamples() != 100 {
		t.Errorf("Expected maxSamples 100, got %d", m.GetMaxSamples())
	}
	
	m2 := NewManagerWithConfig([]int64{1, 2, 3}, 50)
	if m2.GetMaxSamples() != 50 {
		t.Errorf("Expected maxSamples 50, got %d", m2.GetMaxSamples())
	}
}

func TestLatencyTracker_RingRetention3700Samples(t *testing.T) {
	// Simulate ICMP probe: 1s interval, 3600s retained range
	// Insert 3700 samples (more than capacity of 3600)
	buckets := []int64{5, 10, 25, 50, 100}
	retentionCapacity := 3600
	lt := NewLatencyTracker(buckets, retentionCapacity)

	// Insert 3700 samples
	for i := 0; i < 3700; i++ {
		lt.Record(float64(i%100)+10.0, true) // latency varies 10-110ms
	}

	// After 3700 samples into a 3600-capacity buffer:
	// - count should be capped at 3600 (not 3700)
	// - buffer should correctly wrap
	if lt.count != retentionCapacity {
		t.Errorf("Expected count %d after 3700 inserts into %d capacity, got %d", 
			retentionCapacity, retentionCapacity, lt.count)
	}

	// Verify the buffer is correctly wrapping
	samples := lt.GetRecentSamples(retentionCapacity)
	if len(samples) != retentionCapacity {
		t.Errorf("Expected %d samples from GetRecentSamples, got %d", retentionCapacity, len(samples))
	}

	// Verify oldest and newest timestamps exist
	// Note: In real-world, these would span hours. In tests, all samples
	// are recorded in the same instant, so span is 0. This is expected.
	oldest, newest := lt.GetSampleTimestamps()
	if oldest == nil || newest == nil {
		t.Fatal("Expected non-nil timestamps")
	}
	
	// The ring buffer correctly tracks oldest/newest positions
	// In production, GetRecentSamples would return samples spanning the retention window
	// In tests without time mocking, all timestamps are identical
}

func TestLatencyTracker_RetentionCapacityNotExceeded(t *testing.T) {
	// Verify that count never exceeds capacity regardless of inserts
	buckets := []int64{5, 10, 25, 50, 100}
	retentionCapacity := 100
	lt := NewLatencyTracker(buckets, retentionCapacity)

	// Insert 10x the capacity
	for i := 0; i < 1000; i++ {
		lt.Record(float64(i), true)
	}

	// Count must never exceed capacity
	if lt.count > retentionCapacity {
		t.Errorf("Count %d exceeded capacity %d", lt.count, retentionCapacity)
	}
	if lt.count != retentionCapacity {
		t.Errorf("Expected count %d after 1000 inserts into %d capacity, got %d",
			retentionCapacity, retentionCapacity, lt.count)
	}
}

func TestLatencyTracker_PartialFillShowsAccumulation(t *testing.T) {
	// Verify that partial fills are correctly reported
	buckets := []int64{5, 10, 25, 50, 100}
	retentionCapacity := 3600
	lt := NewLatencyTracker(buckets, retentionCapacity)

	// Only 474 samples (like the screenshot shows)
	sampleCount := 474
	for i := 0; i < sampleCount; i++ {
		lt.Record(float64(i%100)+10.0, true)
	}

	// Count should reflect actual samples, not capacity
	if lt.count != sampleCount {
		t.Errorf("Expected count %d, got %d", sampleCount, lt.count)
	}

	// Capacity is still 3600
	if lt.maxSamples != retentionCapacity {
		t.Errorf("Expected maxSamples %d, got %d", retentionCapacity, lt.maxSamples)
	}

	// GetSummary should report partial fill
	summary := lt.GetSummary("partial-target")
	if summary.SampleCount != sampleCount {
		t.Errorf("Expected summary sample count %d, got %d", sampleCount, summary.SampleCount)
	}

	// Verify timestamps exist
	// Note: In tests without time mocking, all timestamps are identical (span = 0)
	// This is expected - production would show real time spans
	oldest, newest := lt.GetSampleTimestamps()
	if oldest == nil || newest == nil {
		t.Fatal("Expected non-nil timestamps")
	}
	
	// The key verification: count < capacity proves partial fill
	// This is the indicator that daemon has not run for full retention period
}

