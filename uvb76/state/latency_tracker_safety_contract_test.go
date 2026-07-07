package state

import (
	"math"
	"testing"
	"time"
)

// Contract tests for LatencyTracker safety invariants (part 2).
// These tests verify the runtime state invariants established by ACT-UVB76-HULK01.

func TestLatencyTrackerContract_NoDuplicateStaleSlots(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 5)

	// Write 10 samples into 5-slot buffer
	for i := 0; i < 10; i++ {
		lt.RecordAt(float64(i), true, time.Now().UTC().Add(time.Duration(i)*time.Second))
	}

	samples := lt.GetRecentSamples(5)

	// Check for duplicate timestamps
	seen := make(map[time.Time]bool)
	for _, s := range samples {
		if seen[s.Timestamp] {
			t.Errorf("duplicate timestamp found: %v", s.Timestamp)
		}
		seen[s.Timestamp] = true
	}
}

func TestLatencyTrackerContract_ReturnedSliceMutationSafety(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 5)

	lt.RecordAt(42.0, true, time.Now().UTC())

	samples1 := lt.GetRecentSamples(5)
	samples2 := lt.GetRecentSamples(5)

	// Verify the slice is a defensive copy - modifying one doesn't affect the other
	if len(samples1) > 0 {
		original := samples1[0].LatencyMs
		samples1[0].LatencyMs = 999.0
		if samples2[0].LatencyMs != original {
			t.Error("defensive copy not working - slice mutation affected original")
		}
	}
}

func TestLatencyTrackerContract_LenReturnedBound(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 100)

	// Write 150 samples into 100-slot buffer
	for i := 0; i < 150; i++ {
		lt.RecordAt(float64(i), true, time.Now().UTC().Add(time.Duration(i)*time.Second))
	}

	// Get with limit
	samples := lt.GetRecentSamples(50)
	if len(samples) != 50 {
		t.Errorf("expected 50 samples with limit=50, got %d", len(samples))
	}

	// Get without limit
	samples = lt.GetRecentSamples(200)
	if len(samples) > 100 {
		t.Errorf("expected max 100 samples (capacity), got %d", len(samples))
	}
}

func TestLatencyTrackerContract_NegativeLimit(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 10)

	lt.RecordAt(10.0, true, time.Now().UTC())

	// Negative limit should not panic
	samples := lt.GetRecentSamples(-5)
	if samples != nil {
		t.Errorf("expected nil for negative limit, got %d samples", len(samples))
	}
}

func TestLatencyTrackerContract_GetRecentSamplesWithErrorSamples(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 10)

	baseTime := time.Now().UTC()
	lt.RecordAt(100.0, true, baseTime)
	lt.RecordAt(0.0, false, baseTime.Add(time.Second))   // error sample
	lt.RecordAt(200.0, true, baseTime.Add(2*time.Second))
	lt.RecordAt(0.0, false, baseTime.Add(3*time.Second)) // error sample
	lt.RecordAt(300.0, true, baseTime.Add(4*time.Second))

	samples := lt.GetRecentSamples(10)
	if len(samples) != 5 {
		t.Errorf("expected 5 samples, got %d", len(samples))
	}

	summary := lt.GetSummary("test")
	if summary.SampleCount != 5 {
		t.Errorf("expected sample count 5, got %d", summary.SampleCount)
	}
	if summary.ErrorCount != 2 {
		t.Errorf("expected error count 2, got %d", summary.ErrorCount)
	}
}

func TestLatencyTrackerContract_SummaryWithAllFailedSamples(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 10)

	// All failed samples
	for i := 0; i < 5; i++ {
		lt.RecordAt(0.0, false, time.Now().UTC().Add(time.Duration(i)*time.Second))
	}

	summary := lt.GetSummary("test")
	if summary.SampleCount != 5 {
		t.Errorf("expected sample count 5, got %d", summary.SampleCount)
	}
	if summary.ErrorCount != 5 {
		t.Errorf("expected error count 5, got %d", summary.ErrorCount)
	}
}

func TestLatencyTrackerContract_GetSampleTimestamps_Empty(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 10)

	oldest, newest := lt.GetSampleTimestamps()
	if oldest != nil || newest != nil {
		t.Errorf("expected nil timestamps for empty tracker")
	}
}

func TestLatencyTrackerContract_GetSampleTimestamps_SingleSample(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 10)

	ts := time.Now().UTC()
	lt.RecordAt(10.0, true, ts)

	oldest, newest := lt.GetSampleTimestamps()
	if oldest == nil || newest == nil {
		t.Fatal("expected non-nil timestamps")
	}
	if !oldest.Equal(ts) || !newest.Equal(ts) {
		t.Errorf("expected both timestamps equal to %v", ts)
	}
}

func TestLatencyTrackerContract_PercentileOrdering(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 100)

	// Record samples with known latencies
	baseTime := time.Now().UTC()
	for i := 1; i <= 100; i++ {
		lt.RecordAt(float64(i), true, baseTime.Add(time.Duration(i)*time.Second))
	}

	summary := lt.GetSummary("test")

	// p50 <= p90 <= p95 <= p99
	if summary.P50LatencyMs != nil && summary.P90LatencyMs != nil {
		if *summary.P50LatencyMs > *summary.P90LatencyMs {
			t.Errorf("p50 (%f) > p90 (%f)", *summary.P50LatencyMs, *summary.P90LatencyMs)
		}
	}
	if summary.P90LatencyMs != nil && summary.P95LatencyMs != nil {
		if *summary.P90LatencyMs > *summary.P95LatencyMs {
			t.Errorf("p90 (%f) > p95 (%f)", *summary.P90LatencyMs, *summary.P95LatencyMs)
		}
	}
	if summary.P95LatencyMs != nil && summary.P99LatencyMs != nil {
		if *summary.P95LatencyMs > *summary.P99LatencyMs {
			t.Errorf("p95 (%f) > p99 (%f)", *summary.P95LatencyMs, *summary.P99LatencyMs)
		}
	}
}

func TestLatencyTrackerContract_PercentileSafety_NaNInput(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 10)

	// Record NaN latencies - just verify no panic
	lt.RecordAt(math.NaN(), true, time.Now().UTC())
	lt.RecordAt(math.NaN(), true, time.Now().UTC().Add(time.Second))

	// Should not panic
	summary := lt.GetSummary("test")
	// Verify summary is returned (may have nil percentiles)
	if summary.SampleCount < 0 {
		t.Errorf("sample count should not be negative: %d", summary.SampleCount)
	}
}

func TestLatencyTrackerContract_PercentileSafety_InfInput(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 10)

	// Record Inf latencies - just verify no panic
	lt.RecordAt(math.Inf(1), true, time.Now().UTC())
	lt.RecordAt(math.Inf(1), true, time.Now().UTC().Add(time.Second))

	// Should not panic
	summary := lt.GetSummary("test")
	// Verify summary is returned (may have nil percentiles)
	if summary.SampleCount < 0 {
		t.Errorf("sample count should not be negative: %d", summary.SampleCount)
	}
}
