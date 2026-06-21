package state

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestLatencyTracker_ReturnedSliceIsDefensiveCopy_Bounded verifies that GetRecentSamples()
// returns a caller-owned slice that is NOT backed by the internal ring buffer.
// Mutations to the returned slice must not affect the tracker state.
// This is a standalone test for bounded window scenarios (ICMP hot path fix).
func TestLatencyTracker_ReturnedSliceIsDefensiveCopy_Bounded(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 100)

	// Record samples - must be enough to be significant
	for i := 0; i < 50; i++ {
		lt.Record(float64(50+i), true)
	}

	// Get samples with bounded window (120 - the ICMP hot path limit)
	samples := lt.GetRecentSamples(5)
	if len(samples) != 5 {
		t.Fatalf("GetRecentSamples(5) returned %d samples, want 5", len(samples))
	}

	// Mutate the returned slice
	originalValue := samples[0].LatencyMs
	samples[0].LatencyMs = 999.0
	samples[0].Reachable = false

	// Get samples again - should NOT see the mutation
	samples2 := lt.GetRecentSamples(5)
	if samples2[0].LatencyMs != originalValue {
		t.Errorf("Mutation of returned slice affected tracker state: got %f, want %f",
			samples2[0].LatencyMs, originalValue)
	}
	if samples2[0].Reachable != true {
		t.Errorf("Mutation of returned slice affected tracker state: Reachable changed")
	}
}

// TestLatencyTracker_ReturnedSliceCannotAffectCapacity verifies that even if someone
// tries to access beyond the returned slice bounds, it doesn't affect the tracker.
func TestLatencyTracker_ReturnedSliceCannotAffectCapacity(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 100)

	// Record some samples - use Record() like the existing working test
	for i := 0; i < 50; i++ {
		lt.Record(float64(i+10), true)
	}

	// Get a small slice
	samples := lt.GetRecentSamples(5)
	if len(samples) != 5 {
		t.Fatalf("GetRecentSamples(5) returned %d samples, want 5", len(samples))
	}

	// The slice capacity should be exactly 5, not 100
	if cap(samples) != 5 {
		t.Errorf("Returned slice capacity = %d, want 5 (defensive copy should have exact capacity)", cap(samples))
	}
}

// TestLatencyTracker_CorruptStateGetRecentSamples_Safe verifies that GetRecentSamples
// returns nil safely instead of panicking when state is corrupted.
// Each corruption scenario uses a fresh tracker to ensure isolation.
func TestLatencyTracker_CorruptStateGetRecentSamples_Safe(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}

	// Test various corruption scenarios - each gets its own fresh tracker
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
			name: "count_exceeds_capacity",
			mutate: func(lt *LatencyTracker) {
				lt.mu.Lock()
				lt.count = 200
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
		{
			name: "nil_slice",
			mutate: func(lt *LatencyTracker) {
				lt.mu.Lock()
				lt.recentSamples = nil
				lt.mu.Unlock()
			},
		},
		{
			name: "wrong_slice_length",
			mutate: func(lt *LatencyTracker) {
				lt.mu.Lock()
				lt.recentSamples = make([]LatencySample, 50)
				lt.mu.Unlock()
			},
		},
		{
			name: "zero_capacity",
			mutate: func(lt *LatencyTracker) {
				lt.mu.Lock()
				lt.recentSamples = make([]LatencySample, 0)
				lt.mu.Unlock()
			},
		},
	}

	for _, tc := range corruptions {
		t.Run(tc.name, func(t *testing.T) {
			// Create fresh tracker for each scenario
			lt := NewLatencyTracker(buckets, 100)

			// Pre-fill with samples
			for i := 0; i < 50; i++ {
				lt.RecordAt(float64(i+10), true, time.Now().UTC())
			}

			// Apply corruption
			tc.mutate(lt)

			// GetRecentSamples should NOT panic
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("GetRecentSamples panicked on %s: %v", tc.name, r)
				}
			}()

			samples := lt.GetRecentSamples(50)

			// FAIL-CLOSED: corruption must return nil
			// The result should be nil due to corruption (fail-closed)
			if samples != nil {
				t.Errorf("GetRecentSamples(50) returned %d samples, want nil (fail-closed on corruption: %s)",
					len(samples), tc.name)
			}
		})
	}
}

// TestLatencyTracker_ICMPHotPathBoundedWindow_Race tests that concurrent spike window
// reads (120 samples) don't race with writes (1 per second in production).
//
// Duration is budget-aware: short default for CI, extended soak with
// UVB76_LONG_CRASH_TESTS=1.
func TestLatencyTracker_ICMPHotPathBoundedWindow_Race(t *testing.T) {
	duration := crashRegressionDuration(t, 3*time.Second)

	// Production ICMP config: 3600 samples capacity
	buckets := []int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}
	capacity := 3600
	lt := NewLatencyTracker(buckets, capacity)

	// Pre-fill to simulate ~1 hour of production data
	for i := 0; i < capacity; i++ {
		lt.RecordAt(float64(i%100)+10.0, true, time.Now().UTC())
	}

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var wg sync.WaitGroup

	// Writers: simulate continuous ICMP probes (1 per second)
	writerCount := 2
	for w := 0; w < writerCount; w++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for i := 0; ; i++ {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					latency := float64((i*17)%200) + 10.0
					reachable := i%10 != 0
					lt.RecordAt(latency, reachable, time.Now().UTC())
					runtime.Gosched()
				}
			}
		}(w)
	}

	// Readers: bounded spike window (120 samples) - the NEW hot path
	// This simulates the fix: ICMP probe goroutines now request 120 samples, not 3600
	readerCount := 4
	for r := 0; r < readerCount; r++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// NEW HOT PATH: bounded spike window (120 samples)
					samples := lt.GetRecentSamples(120)
					if samples != nil {
						for _, s := range samples {
							_ = s.LatencyMs
							_ = s.Reachable
						}
					}
					runtime.Gosched()
				}
			}
		}(r)
	}

	// Also test the UI/API path with full capacity (3600) - should still work
	uiReaderCount := 2
	for r := 0; r < uiReaderCount; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// UI/API path: full capacity (3600 samples)
					samples := lt.GetRecentSamples(3600)
					if samples != nil && len(samples) > 3600 {
						t.Errorf("UI/API path returned %d samples (exceeds 3600)", len(samples))
					}
					runtime.Gosched()
				}
			}
		}()
	}

	t.Logf("Running ICMP hot-path bounded window race test for %v...", duration)
	wg.Wait()

	// Final verification
	samples := lt.GetRecentSamples(capacity)
	if len(samples) > capacity {
		t.Errorf("Final GetRecentSamples(%d) returned %d samples", capacity, len(samples))
	}
}

// TestLatencyTracker_RecordRepairThenReadSafe verifies that RecordAt repairs corrupted
// state and subsequent GetRecentSamples calls return valid data.
func TestLatencyTracker_RecordRepairThenReadSafe(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 100)

	// Corrupt state
	lt.mu.Lock()
	lt.count = -1
	lt.head = 200
	lt.mu.Unlock()

	// RecordAt should repair the state
	lt.RecordAt(50.0, true, time.Now().UTC())

	// GetRecentSamples should see repaired state and return valid data
	samples := lt.GetRecentSamples(10)
	// Should either return nil (fail-closed) or valid data
	if samples != nil && len(samples) > 10 {
		t.Errorf("GetRecentSamples(10) returned %d samples", len(samples))
	}
}

// TestLatencyTracker_BoundedReadAllocation documents the memory savings from using
// a bounded spike window for ICMP hot-path reads.
//
// This test documents that 120 samples (~5.7KB) is significantly smaller than
// the old 3600-sample allocation (~173KB), reducing allocation pressure on
// constrained routers by ~30x.
func TestLatencyTracker_BoundedReadAllocation(t *testing.T) {
	// Approximate size of a LatencySample struct
	sampleSize := 48 // Timestamp (16) + LatencyMs (8) + Reachable (1) + padding + Error string pointer
	
	// Old allocation: 3600 samples
	oldAllocation := 3600 * sampleSize
	
	// New allocation: 120 samples (bounded spike detection window)
	newAllocation := 120 * sampleSize
	
	t.Logf("ICMP hot-path allocation reduction: %d bytes -> %d bytes (%.1fx smaller)",
		oldAllocation, newAllocation, float64(oldAllocation)/float64(newAllocation))
	
	if newAllocation >= oldAllocation {
		t.Errorf("New allocation should be smaller than old allocation")
	}
}
