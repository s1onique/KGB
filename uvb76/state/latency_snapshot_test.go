package state

import (
	"testing"
	"time"
)

// TestLatencyTracker_Snapshot_Basic verifies that Snapshot() returns correct data.
func TestLatencyTracker_Snapshot_Basic(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 100)

	// Record samples
	for i := 0; i < 50; i++ {
		lt.RecordAt(float64(i+10), true, time.Now().UTC())
	}

	snap := lt.Snapshot("test-target", 30)
	if snap == nil {
		t.Fatal("Snapshot returned nil")
	}

	if snap.TargetID != "test-target" {
		t.Errorf("Expected TargetID 'test-target', got %q", snap.TargetID)
	}
	if snap.Count != 30 {
		t.Errorf("Expected Count 30, got %d", snap.Count)
	}
	if len(snap.Samples) != 30 {
		t.Errorf("Expected 30 samples, got %d", len(snap.Samples))
	}
	if snap.Capacity != 100 {
		t.Errorf("Expected Capacity 100, got %d", snap.Capacity)
	}
}

// TestLatencyTracker_Snapshot_EmptyTracker verifies Snapshot returns empty snapshot for empty tracker.
func TestLatencyTracker_Snapshot_EmptyTracker(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 100)

	snap := lt.Snapshot("empty-target", 50)
	if snap == nil {
		t.Fatal("Snapshot returned nil")
	}

	if snap.Count != 0 {
		t.Errorf("Expected Count 0, got %d", snap.Count)
	}
	if len(snap.Samples) != 0 {
		t.Errorf("Expected 0 samples, got %d", len(snap.Samples))
	}
	if snap.OldestSampleTs != nil {
		t.Errorf("Expected nil OldestSampleTs for empty tracker")
	}
	if snap.NewestSampleTs != nil {
		t.Errorf("Expected nil NewestSampleTs for empty tracker")
	}
}

// TestLatencyTracker_Snapshot_NilTracker verifies Snapshot handles nil receiver.
func TestLatencyTracker_Snapshot_NilTracker(t *testing.T) {
	var lt *LatencyTracker = nil
	snap := lt.Snapshot("nil-target", 50)
	if snap != nil {
		t.Errorf("Expected nil snapshot for nil tracker, got %v", snap)
	}
}

// TestLatencyTracker_Snapshot_LimitClamping verifies limit is clamped correctly.
func TestLatencyTracker_Snapshot_LimitClamping(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 100)

	// Record 30 samples
	for i := 0; i < 30; i++ {
		lt.RecordAt(float64(i+10), true, time.Now().UTC())
	}

	// Request more than available
	snap := lt.Snapshot("test", 50)
	if snap.Count != 30 {
		t.Errorf("Expected Count 30 (clamped from 50), got %d", snap.Count)
	}

	// Request negative
	snap = lt.Snapshot("test", -5)
	if snap.Count != 30 {
		t.Errorf("Expected Count 30 (clamped from -5), got %d", snap.Count)
	}

	// Request zero
	snap = lt.Snapshot("test", 0)
	if snap.Count != 30 {
		t.Errorf("Expected Count 30 (clamped from 0), got %d", snap.Count)
	}
}

// TestLatencyTracker_Snapshot_Timestamps verifies timestamps are copied correctly.
func TestLatencyTracker_Snapshot_Timestamps(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 100)

	now := time.Now().UTC()
	lt.RecordAt(10.0, true, now.Add(-10*time.Second))
	lt.RecordAt(20.0, true, now.Add(-5*time.Second))
	lt.RecordAt(30.0, true, now)

	snap := lt.Snapshot("test", 3)
	if snap == nil {
		t.Fatal("Snapshot returned nil")
	}

	if snap.OldestSampleTs == nil {
		t.Fatal("Expected non-nil OldestSampleTs")
	}
	if snap.NewestSampleTs == nil {
		t.Fatal("Expected non-nil NewestSampleTs")
	}

	// Verify ordering
	if !snap.OldestSampleTs.Before(*snap.NewestSampleTs) {
		t.Errorf("Oldest should be before Newest")
	}
}

// TestLatencyTracker_Snapshot_DefensiveCopy verifies returned slices are caller-owned.
func TestLatencyTracker_Snapshot_DefensiveCopy(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 100)

	// Record samples
	for i := 0; i < 50; i++ {
		lt.RecordAt(float64(i+10), true, time.Now().UTC())
	}

	snap := lt.Snapshot("test", 30)
	if snap == nil {
		t.Fatal("Snapshot returned nil")
	}

	// Verify capacity matches count (defensive copy)
	if cap(snap.Samples) != 30 {
		t.Errorf("Sample slice capacity %d != count %d (should be exact)", cap(snap.Samples), snap.Count)
	}

	// Mutate the snapshot
	originalValue := snap.Samples[0].LatencyMs
	snap.Samples[0].LatencyMs = 9999.0

	// Get another snapshot - should NOT see mutation
	snap2 := lt.Snapshot("test", 30)
	if snap2.Samples[0].LatencyMs != originalValue {
		t.Errorf("Mutation leaked: got %f, want %f", snap2.Samples[0].LatencyMs, originalValue)
	}

	// Mutate buckets
	originalBucket := snap.Buckets[0]
	snap.Buckets[0] = 9999

	snap3 := lt.Snapshot("test", 1)
	if snap3.Buckets[0] != originalBucket {
		t.Errorf("Bucket mutation leaked: got %d, want %d", snap3.Buckets[0], originalBucket)
	}
}

// TestLatencyTracker_Snapshot_CorruptedStateFailClosed verifies fail-closed behavior.
func TestLatencyTracker_Snapshot_CorruptedStateFailClosed(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}

	corruptions := []struct {
		name  string
		mutate func(*LatencyTracker)
	}{
		{
			name: "negative_count",
			mutate: func(lt *LatencyTracker) {
				lt.mu.Lock()
				lt.count = -1
				lt.mu.Unlock()
			},
		},
		{
			name: "negative_head",
			mutate: func(lt *LatencyTracker) {
				lt.mu.Lock()
				lt.head = -50
				lt.mu.Unlock()
			},
		},
		{
			name: "head_exceeds_capacity",
			mutate: func(lt *LatencyTracker) {
				lt.mu.Lock()
				lt.head = 200
				lt.mu.Unlock()
			},
		},
	}

	for _, tc := range corruptions {
		t.Run(tc.name, func(t *testing.T) {
			// Create fresh tracker for each scenario
			lt := NewLatencyTracker(buckets, 100)
			for i := 0; i < 50; i++ {
				lt.RecordAt(float64(i+10), true, time.Now().UTC())
			}

			// Apply corruption
			tc.mutate(lt)

			// Snapshot should return safe empty snapshot (fail-closed)
			snap := lt.Snapshot("test", 50)
			if snap == nil {
				t.Errorf("Snapshot should not return nil on corruption")
				return
			}
			// Count should be 0 or clamped safely
			if snap.Count > snap.Capacity {
				t.Errorf("Snapshot count %d exceeds capacity %d", snap.Count, snap.Capacity)
			}
			if len(snap.Samples) != snap.Count {
				t.Errorf("Sample slice length %d != count %d", len(snap.Samples), snap.Count)
			}
		})
	}
}

// TestManager_GetICMPSnapshot_Basic verifies Manager-level snapshot primitive.
func TestManager_GetICMPSnapshot_Basic(t *testing.T) {
	m := NewManager()
	m.ConfigureICMP([]int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}, 3600)

	targetID := "test-router"

	// Record samples
	for i := 0; i < 100; i++ {
		m.RecordICMPLatencyAt(targetID, float64(i+10), true, time.Now().UTC())
	}

	snap := m.GetICMPSnapshot(targetID, 50)
	if snap == nil {
		t.Fatal("Expected non-nil snapshot")
	}

	if snap.TargetID != targetID {
		t.Errorf("Expected TargetID %q, got %q", targetID, snap.TargetID)
	}
	if snap.Count != 50 {
		t.Errorf("Expected Count 50, got %d", snap.Count)
	}
	if len(snap.Samples) != 50 {
		t.Errorf("Expected 50 samples, got %d", len(snap.Samples))
	}
}

// TestManager_GetICMPSnapshot_EmptyTarget verifies Manager returns empty snapshot.
func TestManager_GetICMPSnapshot_EmptyTarget(t *testing.T) {
	m := NewManager()
	m.ConfigureICMP([]int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}, 3600)

	snap := m.GetICMPSnapshot("nonexistent", 50)
	if snap == nil {
		t.Fatal("Expected non-nil snapshot for non-existent target")
	}

	if snap.Count != 0 {
		t.Errorf("Expected Count 0, got %d", snap.Count)
	}
	if len(snap.Samples) != 0 {
		t.Errorf("Expected 0 samples, got %d", len(snap.Samples))
	}
	if len(snap.Buckets) == 0 {
		t.Errorf("Expected non-empty buckets")
	}
}

// TestManager_GetHTTPSnapshot_Basic verifies HTTP snapshot primitive.
func TestManager_GetHTTPSnapshot_Basic(t *testing.T) {
	m := NewManager()

	targetID := "http-target"

	// Record samples
	for i := 0; i < 50; i++ {
		m.RecordLatency(targetID, float64(i+10), true)
	}

	snap := m.GetHTTPSnapshot(targetID, 30)
	if snap == nil {
		t.Fatal("Expected non-nil snapshot")
	}

	if snap.Count != 30 {
		t.Errorf("Expected Count 30, got %d", snap.Count)
	}
	if len(snap.Samples) != 30 {
		t.Errorf("Expected 30 samples, got %d", len(snap.Samples))
	}
	if snap.Capacity != 100 {
		t.Errorf("Expected Capacity 100, got %d", snap.Capacity)
	}
}
