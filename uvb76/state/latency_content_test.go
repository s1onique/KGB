package state

import (
	"testing"
	"time"
)

// TestLatencyTracker_GetRecentSamples_ReturnsNewestLimitChronological verifies that
// GetRecentSamples returns the most recent `limit` samples in chronological order,
// not the oldest `limit` samples. This was a regression in the initial fix attempt.
func TestLatencyTracker_GetRecentSamples_ReturnsNewestLimitChronological(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 10)

	base := time.Date(2026, 6, 19, 1, 16, 28, 0, time.UTC)
	// Record 15 samples (exceeds capacity of 10)
	for i := 0; i < 15; i++ {
		lt.RecordAt(float64(i), true, base.Add(time.Duration(i)*time.Second))
	}

	// Request the 5 most recent samples
	samples := lt.GetRecentSamples(5)
	if len(samples) != 5 {
		t.Fatalf("expected 5 samples, got %d", len(samples))
	}

	// The most recent 5 samples should be: 10, 11, 12, 13, 14
	// (since capacity is 10, samples 0-4 were evicted)
	want := []float64{10, 11, 12, 13, 14}
	for i, sample := range samples {
		if sample.LatencyMs != want[i] {
			t.Errorf("sample %d latency = %v, want %v", i, sample.LatencyMs, want[i])
		}
		// Verify timestamps are in chronological order
		if i > 0 {
			if !samples[i].Timestamp.After(samples[i-1].Timestamp) {
				t.Errorf("sample %d timestamp not after sample %d", i, i-1)
			}
		}
	}
}

// TestLatencyTracker_GetRecentSamples_ChronologicalOrderPartialFill verifies
// chronological order when buffer is not full.
func TestLatencyTracker_GetRecentSamples_ChronologicalOrderPartialFill(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 10)

	base := time.Date(2026, 6, 19, 1, 16, 28, 0, time.UTC)
	// Record only 5 samples (less than capacity)
	for i := 0; i < 5; i++ {
		lt.RecordAt(float64(i*10), true, base.Add(time.Duration(i)*time.Second))
	}

	// Request all 5 samples
	samples := lt.GetRecentSamples(5)
	if len(samples) != 5 {
		t.Fatalf("expected 5 samples, got %d", len(samples))
	}

	// Should be: 0, 10, 20, 30, 40
	want := []float64{0, 10, 20, 30, 40}
	for i, sample := range samples {
		if sample.LatencyMs != want[i] {
			t.Errorf("sample %d latency = %v, want %v", i, sample.LatencyMs, want[i])
		}
	}
}

// TestLatencyTracker_GetRecentSamples_RequestAllWhenFull verifies requesting
// all samples when buffer is at capacity.
func TestLatencyTracker_GetRecentSamples_RequestAllWhenFull(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 5)

	base := time.Date(2026, 6, 19, 1, 16, 28, 0, time.UTC)
	// Fill exactly to capacity
	for i := 0; i < 5; i++ {
		lt.RecordAt(float64(i), true, base.Add(time.Duration(i)*time.Second))
	}

	// Request all 5 samples
	samples := lt.GetRecentSamples(5)
	if len(samples) != 5 {
		t.Fatalf("expected 5 samples, got %d", len(samples))
	}

	// Should be: 0, 1, 2, 3, 4
	want := []float64{0, 1, 2, 3, 4}
	for i, sample := range samples {
		if sample.LatencyMs != want[i] {
			t.Errorf("sample %d latency = %v, want %v", i, sample.LatencyMs, want[i])
		}
	}
}

// TestLatencyTracker_GetRecentSamples_OverCapacity verifies wrapping behavior
// when buffer wraps around the ring buffer.
func TestLatencyTracker_GetRecentSamples_OverCapacity(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 5)

	base := time.Date(2026, 6, 19, 1, 16, 28, 0, time.UTC)
	// Write 12 samples (wraps around twice: 5 + 5 + 2)
	for i := 0; i < 12; i++ {
		lt.RecordAt(float64(i), true, base.Add(time.Duration(i)*time.Second))
	}

	// Should have exactly 5 samples (capacity)
	if lt.count != 5 {
		t.Fatalf("expected count 5, got %d", lt.count)
	}

	// Request all 5 samples - should be the most recent 5: 7, 8, 9, 10, 11
	samples := lt.GetRecentSamples(5)
	if len(samples) != 5 {
		t.Fatalf("expected 5 samples, got %d", len(samples))
	}

	want := []float64{7, 8, 9, 10, 11}
	for i, sample := range samples {
		if sample.LatencyMs != want[i] {
			t.Errorf("sample %d latency = %v, want %v", i, sample.LatencyMs, want[i])
		}
	}
}

// TestLatencyTracker_GetRecentSamples_ClampValues verifies that GetRecentSamples
// properly clamps limit values before allocation.
func TestLatencyTracker_GetRecentSamples_ClampValues(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	capacity := 100
	lt := NewLatencyTracker(buckets, capacity)

	// Fill buffer partially
	for i := 0; i < 50; i++ {
		lt.Record(float64(i), true)
	}

	tests := []struct {
		name        string
		limit       int
		expectedMax int
	}{
		{"zero limit returns all", 0, 50},
		{"negative limit returns all", -1, 50},
		{"limit exceeds count returns all", 100, 50},
		{"limit equals count", 50, 50},
		{"limit less than count", 25, 25},
		{"capacity size exceeds count", capacity, 50},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			samples := lt.GetRecentSamples(tc.limit)
			if len(samples) != tc.expectedMax {
				t.Errorf("GetRecentSamples(%d) returned %d samples, expected %d",
					tc.limit, len(samples), tc.expectedMax)
			}
		})
	}
}

// TestLatencyTracker_GetRecentSamples_EmptyBuffer verifies behavior with empty buffer.
func TestLatencyTracker_GetRecentSamples_EmptyBuffer(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 100)

	samples := lt.GetRecentSamples(10)
	if len(samples) != 0 {
		t.Errorf("Expected 0 samples from empty buffer, got %d", len(samples))
	}

	samples = lt.GetRecentSamples(0)
	if len(samples) != 0 {
		t.Errorf("Expected 0 samples from empty buffer with limit 0, got %d", len(samples))
	}
}
