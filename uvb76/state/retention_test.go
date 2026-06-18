package state

import (
	"testing"
	"time"
)

// TestLatencyTracker_RecordAt_DeterministicRetention tests ICMP retention with deterministic timestamps.
// This proves that with 3700 one-second samples over 3700 seconds (~1 hour),
// the ring buffer correctly retains 3600 and evicts oldest 100.
func TestLatencyTracker_RecordAt_DeterministicRetention(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	retentionCapacity := 3600
	lt := NewLatencyTracker(buckets, retentionCapacity)

	baseTime := time.Date(2026, 6, 18, 8, 0, 0, 0, time.UTC)

	// Insert 3700 samples, one second apart
	for i := 0; i < 3700; i++ {
		timestamp := baseTime.Add(time.Duration(i) * time.Second)
		lt.RecordAt(float64(i%100)+10.0, true, timestamp)
	}

	// After 3700 samples into a 3600-capacity buffer:
	// - count should be exactly 3600 (oldest 100 evicted)
	if lt.count != retentionCapacity {
		t.Errorf("Expected count %d after 3700 inserts, got %d", retentionCapacity, lt.count)
	}

	// Verify the oldest/newest timestamps span approximately 3599 seconds
	// (3700 samples total, newest is at t=3699s, oldest retained at t=100s)
	oldest, newest := lt.GetSampleTimestamps()
	if oldest == nil || newest == nil {
		t.Fatal("Expected non-nil timestamps")
	}

	span := newest.Sub(*oldest)
	expectedSpan := time.Duration(retentionCapacity-1) * time.Second // 3599 seconds
	if span != expectedSpan {
		t.Errorf("Expected timestamp span of %v, got %v", expectedSpan, span)
	}

	// Verify oldest sample is at baseTime + 100 seconds (first 100 evicted)
	expectedOldest := baseTime.Add(100 * time.Second)
	if !oldest.Equal(expectedOldest) {
		t.Errorf("Expected oldest sample at %v, got %v", expectedOldest, *oldest)
	}

	// Verify newest sample is at baseTime + 3699 seconds
	expectedNewest := baseTime.Add(3699 * time.Second)
	if !newest.Equal(expectedNewest) {
		t.Errorf("Expected newest sample at %v, got %v", expectedNewest, *newest)
	}
}

// TestLatencyTracker_RecordAt_MidnightWrap tests ring buffer wrapping at midnight boundary.
func TestLatencyTracker_RecordAt_MidnightWrap(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	retentionCapacity := 3600
	lt := NewLatencyTracker(buckets, retentionCapacity)

	// Start at 23:59:30 UTC and wrap through midnight
	baseTime := time.Date(2026, 6, 17, 23, 59, 30, 0, time.UTC)

	// Insert 3700 samples, one second apart (crosses midnight)
	for i := 0; i < 3700; i++ {
		timestamp := baseTime.Add(time.Duration(i) * time.Second)
		lt.RecordAt(float64(i%100)+10.0, true, timestamp)
	}

	// Count should still be correctly bounded
	if lt.count != retentionCapacity {
		t.Errorf("Expected count %d, got %d", retentionCapacity, lt.count)
	}

	// Verify span across midnight boundary is still correct
	oldest, newest := lt.GetSampleTimestamps()
	if oldest == nil || newest == nil {
		t.Fatal("Expected non-nil timestamps")
	}

	span := newest.Sub(*oldest)
	expectedSpan := time.Duration(retentionCapacity-1) * time.Second
	if span != expectedSpan {
		t.Errorf("Expected span of %v across midnight wrap, got %v", expectedSpan, span)
	}

	// Verify the oldest is 100 seconds after midnight (2026-06-18 00:01:10)
	// Sample 0 at 23:59:30, samples 0-99 (100 total) are evicted.
	// Sample 100 is at 23:59:30 + 100s = 00:01:10
	expectedOldest := time.Date(2026, 6, 18, 0, 1, 10, 0, time.UTC)
	if !oldest.Equal(expectedOldest) {
		t.Errorf("Expected oldest after midnight wrap at %v, got %v", expectedOldest, *oldest)
	}
}

// TestManager_RecordICMPLatencyAt tests that Manager correctly routes RecordAt to ICMP trackers.
func TestManager_RecordICMPLatencyAt(t *testing.T) {
	m := NewManager()
	m.ConfigureICMP([]int64{5, 10, 25, 50, 100}, 3600)

	baseTime := time.Date(2026, 6, 18, 8, 0, 0, 0, time.UTC)

	// Insert 100 samples via RecordAt
	for i := 0; i < 100; i++ {
		timestamp := baseTime.Add(time.Duration(i) * time.Second)
		m.RecordICMPLatencyAt("test-target", float64(i)+10.0, true, timestamp)
	}

	samples := m.GetRecentICMPLatencySamples("test-target", 3600)
	if len(samples) != 100 {
		t.Errorf("Expected 100 samples, got %d", len(samples))
	}

	// Verify first and last timestamps
	if samples[0].Timestamp.Unix() != baseTime.Unix() {
		t.Errorf("Expected first sample at %v, got %v", baseTime, samples[0].Timestamp)
	}
	lastIdx := len(samples) - 1
	expectedLast := baseTime.Add(99 * time.Second)
	if samples[lastIdx].Timestamp.Unix() != expectedLast.Unix() {
		t.Errorf("Expected last sample at %v, got %v", expectedLast, samples[lastIdx].Timestamp)
	}
}
