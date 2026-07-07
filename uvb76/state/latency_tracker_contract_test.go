package state

import (
	"testing"
	"time"
)

// Contract tests for LatencyTracker ring-buffer invariants (part 1).
// These tests verify the runtime state invariants established by ACT-UVB76-HULK01.

func TestLatencyTrackerContract_Empty(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 10)

	samples := lt.GetRecentSamples(5)
	if samples != nil {
		t.Errorf("expected nil for empty tracker, got %d samples", len(samples))
	}

	oldest, newest := lt.GetSampleTimestamps()
	if oldest != nil {
		t.Errorf("expected nil oldest for empty tracker")
	}
	if newest != nil {
		t.Errorf("expected nil newest for empty tracker")
	}

	summary := lt.GetSummary("empty-target")
	if summary.SampleCount != 0 {
		t.Errorf("expected sample count 0, got %d", summary.SampleCount)
	}
}

func TestLatencyTrackerContract_CapacityOne(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 1)

	ts1 := time.Now().UTC()
	lt.RecordAt(10.0, true, ts1)

	samples := lt.GetRecentSamples(5)
	if len(samples) != 1 {
		t.Errorf("expected 1 sample, got %d", len(samples))
	}

	ts2 := ts1.Add(time.Second)
	lt.RecordAt(20.0, true, ts2)

	samples = lt.GetRecentSamples(5)
	if len(samples) != 1 {
		t.Errorf("expected 1 sample after overwrite, got %d", len(samples))
	}

	if samples[0].LatencyMs != 20.0 {
		t.Errorf("expected latency 20.0, got %f", samples[0].LatencyMs)
	}
}

func TestLatencyTrackerContract_CapacityN(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	capacity := 5
	lt := NewLatencyTracker(buckets, capacity)

	for i := 1; i <= capacity; i++ {
		lt.RecordAt(float64(i*10), true, time.Now().UTC().Add(time.Duration(i)*time.Second))
	}

	samples := lt.GetRecentSamples(capacity)
	if len(samples) != capacity {
		t.Errorf("expected %d samples, got %d", capacity, len(samples))
	}

	oldest, newest := lt.GetSampleTimestamps()
	if oldest == nil || newest == nil {
		t.Fatal("expected non-nil timestamps")
	}
	if oldest.After(*newest) {
		t.Errorf("oldest timestamp after newest: oldest=%v, newest=%v", *oldest, *newest)
	}
}

func TestLatencyTrackerContract_LimitZero(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 10)

	for i := 0; i < 5; i++ {
		lt.Record(float64(i*10), true)
	}

	samples := lt.GetRecentSamples(0)
	if samples != nil {
		t.Errorf("expected nil for limit=0, got %d samples", len(samples))
	}
}

func TestLatencyTrackerContract_LimitLessThanCapacity(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 10)

	for i := 0; i < 8; i++ {
		lt.RecordAt(float64(i*10), true, time.Now().UTC().Add(time.Duration(i)*time.Second))
	}

	limit := 3
	samples := lt.GetRecentSamples(limit)
	if len(samples) != limit {
		t.Errorf("expected %d samples, got %d", limit, len(samples))
	}
}

func TestLatencyTrackerContract_LimitEqualCapacity(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	capacity := 10
	lt := NewLatencyTracker(buckets, capacity)

	for i := 0; i < capacity; i++ {
		lt.RecordAt(float64(i*10), true, time.Now().UTC().Add(time.Duration(i)*time.Second))
	}

	samples := lt.GetRecentSamples(capacity)
	if len(samples) != capacity {
		t.Errorf("expected %d samples, got %d", capacity, len(samples))
	}
}

func TestLatencyTrackerContract_LimitGreaterThanCapacity(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	capacity := 5
	lt := NewLatencyTracker(buckets, capacity)

	for i := 0; i < capacity; i++ {
		lt.RecordAt(float64(i*10), true, time.Now().UTC().Add(time.Duration(i)*time.Second))
	}

	limit := 100
	samples := lt.GetRecentSamples(limit)
	if len(samples) != capacity {
		t.Errorf("expected %d samples (capacity), got %d", capacity, len(samples))
	}
}

func TestLatencyTrackerContract_WritesBeyondCapacity(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	capacity := 3
	lt := NewLatencyTracker(buckets, capacity)

	for i := 0; i < 10; i++ {
		lt.RecordAt(float64(i*10), true, time.Now().UTC().Add(time.Duration(i)*time.Second))
	}

	samples := lt.GetRecentSamples(capacity)
	if len(samples) != capacity {
		t.Errorf("expected %d samples, got %d", capacity, len(samples))
	}

	if samples[0].LatencyMs != 70.0 {
		t.Errorf("expected first sample latency 70.0, got %f", samples[0].LatencyMs)
	}
	if samples[1].LatencyMs != 80.0 {
		t.Errorf("expected second sample latency 80.0, got %f", samples[1].LatencyMs)
	}
	if samples[2].LatencyMs != 90.0 {
		t.Errorf("expected third sample latency 90.0, got %f", samples[2].LatencyMs)
	}
}

func TestLatencyTrackerContract_NewestOldestOrdering(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 10)

	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		lt.RecordAt(float64((i+1)*10), true, baseTime.Add(time.Duration(i)*time.Minute))
	}

	samples := lt.GetRecentSamples(5)
	if len(samples) != 5 {
		t.Fatalf("expected 5 samples, got %d", len(samples))
	}

	for i := 1; i < len(samples); i++ {
		if samples[i].Timestamp.Before(samples[i-1].Timestamp) {
			t.Errorf("samples not in chronological order: samples[%d]=%v, samples[%d]=%v",
				i-1, samples[i-1].Timestamp, i, samples[i].Timestamp)
		}
	}

	oldest, newest := lt.GetSampleTimestamps()
	if oldest == nil || newest == nil {
		t.Fatal("expected non-nil timestamps")
	}
	if !oldest.Equal(samples[0].Timestamp) {
		t.Errorf("oldest timestamp mismatch")
	}
	if !newest.Equal(samples[len(samples)-1].Timestamp) {
		t.Errorf("newest timestamp mismatch")
	}
}

func TestLatencyTrackerContract_TimestampOrderingInvariant(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 100)

	baseTime := time.Now().UTC()
	for i := 0; i < 50; i++ {
		lt.RecordAt(float64(i), true, baseTime.Add(time.Duration(i)*time.Second))
	}

	oldest, newest := lt.GetSampleTimestamps()
	if oldest == nil || newest == nil {
		t.Fatal("expected non-nil timestamps")
	}
	if oldest.After(*newest) {
		t.Errorf("INVARIANT VIOLATION: oldest (%v) is after newest (%v)", *oldest, *newest)
	}
}

func TestLatencyTrackerContract_NoZeroUninitializedSamples(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 10)

	lt.RecordAt(42.0, true, time.Now().UTC())

	samples := lt.GetRecentSamples(10)

	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}

	if samples[0].Timestamp.IsZero() {
		t.Error("returned sample has zero timestamp - possible uninitialized slot")
	}

	if samples[0].LatencyMs != 42.0 {
		t.Errorf("expected latency 42.0, got %f", samples[0].LatencyMs)
	}
}
