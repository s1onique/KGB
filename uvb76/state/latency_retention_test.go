package state

import (
	"testing"
)

// Latency Retention Tests

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
