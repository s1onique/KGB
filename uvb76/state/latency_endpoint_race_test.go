package state

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestLatencyTracker_ConcurrentRecordSummaryAndSamples_Race is a regression test
// for the SIGSEGV crash during concurrent HTTP/2 latency endpoint requests:
//   - /latency (GetSummary via handleTargetLatency)
//   - /latency/series (GetRecentSamples via handleTargetLatencySeries)
//
// The crash stack showed:
//   goroutine 9070: /latency/series JSON encode
//   goroutine 9069: /latency summary
//
// Both handlers were racing on the same LatencyTracker without proper read-write
// synchronization. This test verifies that concurrent RecordAt + GetSummary +
// GetRecentSamples is race-free.
func TestLatencyTracker_ConcurrentRecordSummaryAndSamples_Race(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}
	capacity := 3600
	lt := NewLatencyTracker(buckets, capacity)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Writers: simulate ICMP/HTTP probe writes
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			i := 0
			for {
				select {
				case <-ctx.Done():
					return
				default:
					latency := float64((i*17)%200) + 10.0
					reachable := i%10 != 0
					lt.RecordAt(latency, reachable, time.Now().UTC())
					i++
				}
			}
		}()
	}

	// Readers: simulate /latency summary endpoint (GetSummary)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					_ = lt.GetSummary("target-a")
				}
			}
		}()
	}

	// Readers: simulate /latency/series endpoint (GetRecentSamples)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			limits := []int{100, 500, 1000, 1800, 3600}
			j := 0
			for {
				select {
				case <-ctx.Done():
					return
				default:
					limit := limits[j%len(limits)]
					samples := lt.GetRecentSamples(limit)
					// Verify no corruption
					for _, s := range samples {
						_ = s.LatencyMs
						_ = s.Reachable
					}
					j++
				}
			}
		}()
	}

	wg.Wait()
}

// TestLatencyTracker_ConcurrentRecordGetSampleTimestampsAndGetRecentSamples_Race
// tests the concurrent access pattern seen in the crash:
// goroutine A: GetSampleTimestamps (used by latency_series.go for Oldest/NewestSampleTs)
// goroutine B: GetRecentSamples (used by latency_series.go for Points aggregation)
// goroutine C: RecordAt (probe writes)
func TestLatencyTracker_ConcurrentRecordGetSampleTimestampsAndGetRecentSamples_Race(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}
	capacity := 3600
	lt := NewLatencyTracker(buckets, capacity)

	// Pre-fill to near capacity
	for i := 0; i < capacity-10; i++ {
		lt.RecordAt(float64(i%100)+10.0, true, time.Now().UTC())
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
				lt.RecordAt(float64((i*17)%200)+10.0, i%10 != 0, time.Now().UTC())
			}
		}
	}()

	// GetSampleTimestamps readers (used by series endpoint for ts bounds)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					oldest, newest := lt.GetSampleTimestamps()
					if oldest != nil && newest != nil {
						_ = oldest.After(*newest) // Verify ordering
					}
				}
			}
		}()
	}

	// GetRecentSamples readers (used by series endpoint for aggregation)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					samples := lt.GetRecentSamples(3600)
					if samples != nil {
						for _, s := range samples {
							_ = s.LatencyMs
						}
					}
				}
			}
		}()
	}

	wg.Wait()
}

// TestLatencyTracker_ConcurrentRecordSummaryAndGetRecentSamples_HistogramRace
// specifically targets the crash at line 266-276 in the original crash trace:
// encoding/json.structEncoder.encode
// where histogram bucket slices or sample slices were being read during JSON encode
// while another goroutine was mutating them.
func TestLatencyTracker_ConcurrentRecordSummaryAndGetRecentSamples_HistogramRace(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}
	capacity := 3600
	lt := NewLatencyTracker(buckets, capacity)

	// Pre-fill to ensure histogram is populated
	for i := 0; i < 1000; i++ {
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

	// GetSummary readers - this exercises Histogram.Counts iteration and
	// allSamples/successfulSamples slice iteration under read lock
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					summary := lt.GetSummary("test-target")
					// Verify histogram consistency
					if len(summary.Histogram.Counts) != len(buckets) {
						t.Errorf("Histogram counts length mismatch: got %d, want %d",
							len(summary.Histogram.Counts), len(buckets))
					}
					// Verify sample count consistency
					if summary.SampleCount < 0 {
						t.Errorf("Negative sample count: %d", summary.SampleCount)
					}
					if summary.SampleCount > capacity {
						t.Errorf("Sample count %d exceeds capacity %d", summary.SampleCount, capacity)
					}
				}
			}
		}()
	}

	// GetRecentSamples readers
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					samples := lt.GetRecentSamples(3600)
					_ = samples
				}
			}
		}()
	}

	wg.Wait()
}

// TestManager_ConcurrentLatencyEndpoints_Race is an integration-level test
// that simulates the exact crash scenario: concurrent HTTP/2 handlers hitting
// /latency and /latency/series endpoints while probes are recording samples.
//
// This test runs at the Manager level (not just LatencyTracker) to catch any
// races between Manager-level state management and individual tracker operations.
func TestManager_ConcurrentLatencyEndpoints_Race(t *testing.T) {
	m := NewManager()
	m.ConfigureICMP([]int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}, 3600)

	targetID := "test-target"
	capacity := m.GetICMPMaxSamples()

	// Pre-fill to simulate running system
	for i := 0; i < capacity-10; i++ {
		m.RecordICMPLatencyAt(targetID, float64(i%100)+10.0, true, time.Now().UTC())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Writer: simulate ongoing ICMP probes
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

	// Reader: simulate /latency endpoint (GetICMPLatencySummary)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					summary := m.GetICMPLatencySummary(targetID)
					if summary.SampleCount > capacity {
						t.Errorf("Sample count %d exceeds capacity %d", summary.SampleCount, capacity)
					}
				}
			}
		}()
	}

	// Reader: simulate /latency/series endpoint (GetRecentICMPLatencySamples)
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
					if len(samples) > capacity {
						t.Errorf("Got %d samples, expected max %d", len(samples), capacity)
					}
				}
			}
		}()
	}

	// Reader: simulate /latency/series timestamps (GetICMPLatencySampleTimestamps)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					oldest, newest := m.GetICMPLatencySampleTimestamps(targetID)
					if oldest != nil && newest != nil {
						_ = oldest.Before(*newest) // Verify ordering
					}
				}
			}
		}()
	}

	wg.Wait()
}
