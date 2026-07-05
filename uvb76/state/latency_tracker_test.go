package state

import (
	"testing"
)

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
	lt.Record(75.0, true)  // in fourth bucket (<= 100)
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
