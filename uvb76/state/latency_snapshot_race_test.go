package state

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestLatencyTracker_Snapshot_ConcurrentRecordAndSnapshot_Race is a race test
// that verifies Snapshot() + RecordAt() are race-free.
func TestLatencyTracker_Snapshot_ConcurrentRecordAndSnapshot_Race(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}
	capacity := 3600
	lt := NewLatencyTracker(buckets, capacity)

	// Pre-fill
	for i := 0; i < capacity-10; i++ {
		lt.RecordAt(float64(i%100)+10.0, true, time.Now().UTC())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Writers
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-ctx.Done():
					return
				default:
					lt.RecordAt(float64((i*17)%200)+10.0, i%10 != 0, time.Now().UTC())
				}
			}
		}()
	}

	// Snapshot readers
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					snap := lt.Snapshot("race-target", 3600)
					if snap != nil {
						if snap.Count > snap.Capacity {
							t.Errorf("Count %d > Capacity %d", snap.Count, snap.Capacity)
						}
						if len(snap.Samples) != snap.Count {
							t.Errorf("Sample len %d != count %d", len(snap.Samples), snap.Count)
						}
					}
				}
			}
		}()
	}

	// Test with various limits
	limits := []int{100, 500, 1800, 3600}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			j := 0
			for {
				select {
				case <-ctx.Done():
					return
				default:
					limit := limits[j%len(limits)]
					snap := lt.Snapshot("race-target", limit)
					if snap != nil && len(snap.Samples) > limit {
						t.Errorf("Snapshot returned %d samples, limit was %d", len(snap.Samples), limit)
					}
					j++
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestLatencyTracker_Snapshot_ConcurrentSnapshotAndGetSummary_Race verifies
// Snapshot() + GetSummary() are race-free.
func TestLatencyTracker_Snapshot_ConcurrentSnapshotAndGetSummary_Race(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}
	capacity := 3600
	lt := NewLatencyTracker(buckets, capacity)

	// Pre-fill
	for i := 0; i < 2000; i++ {
		lt.RecordAt(float64(i%500)+10.0, i%15 != 0, time.Now().UTC())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				lt.RecordAt(float64((i*17)%500)+10.0, i%10 != 0, time.Now().UTC())
			}
		}
	}()

	// Snapshot readers
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					snap := lt.Snapshot("summary-snapshot-target", 3600)
					if snap != nil && snap.Count > capacity {
						t.Errorf("Snapshot count %d exceeds capacity %d", snap.Count, capacity)
					}
				}
			}
		}()
	}

	// GetSummary readers
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					summary := lt.GetSummary("summary-snapshot-target")
					if summary.SampleCount > capacity {
						t.Errorf("Summary count %d exceeds capacity %d", summary.SampleCount, capacity)
					}
				}
			}
		}()
	}

	wg.Wait()
}

// TestManager_GetICMPSnapshot_Concurrent_Race is a Manager-level race test.
func TestManager_GetICMPSnapshot_Concurrent_Race(t *testing.T) {
	m := NewManager()
	m.ConfigureICMP([]int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}, 3600)

	targetID := "race-target"

	// Pre-fill
	for i := 0; i < 3600; i++ {
		m.RecordICMPLatencyAt(targetID, float64(i%100)+10.0, true, time.Now().UTC())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				m.RecordICMPLatency(targetID, float64((i*17)%200)+10.0, i%10 != 0)
			}
		}
	}()

	// Snapshot readers (new primitive)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					snap := m.GetICMPSnapshot(targetID, 3600)
					if snap == nil {
						t.Errorf("Snapshot returned nil")
						continue
					}
					if snap.Count < 0 || snap.Count > snap.Capacity {
						t.Errorf("Invalid snapshot: count=%d, capacity=%d", snap.Count, snap.Capacity)
					}
				}
			}
		}()
	}

	// GetRecentSamples readers (existing primitive)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					samples := m.GetRecentICMPLatencySamples(targetID, 3600)
					if len(samples) > 3600 {
						t.Errorf("Got %d samples, expected max 3600", len(samples))
					}
				}
			}
		}()
	}

	wg.Wait()
}
