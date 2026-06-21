package state

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestManager_ICMP_SpikeDetection_Integration exercises the real production call path:
// ICMP probe goroutine -> Manager -> LatencyTracker -> GetRecentSamples -> DetectAndRecord -> calculateMedian
//
// This test reproduces the production shape where samples come from shared mutable
// storage (the latency tracker ring buffer), not freshly allocated per-worker slices.
func TestManager_ICMP_SpikeDetection_Integration(t *testing.T) {
	// Set up manager with ICMP-capacity-sized buffer (the problematic size from production)
	capacity := 3600
	m := NewManager()
	m.ConfigureICMP([]int64{5, 10, 25, 50, 100}, capacity)

	targetID := "test-target"

	// Pre-populate the tracker to near capacity (simulates running ICMP probe for ~1 hour)
	for i := 0; i < capacity-10; i++ {
		m.RecordICMPLatencyAt(targetID, float64(50+(i%30)), true, time.Now().UTC().Add(-time.Duration(i)*time.Second))
	}

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	// Writer: simulates ongoing ICMP probes recording latency
	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-stopCh:
				return
			default:
				latency := float64(50 + (i % 50))
				m.RecordICMPLatency(targetID, latency, true)
				i++
				runtime.Gosched()
			}
		}
	}()

	// Readers: simulate ICMP probe goroutines calling DetectAndRecord
	// Each reader gets samples via Manager.GetRecentICMPLatencySamples, which calls
	// LatencyTracker.GetRecentSamples() that returns a defensive copy
	readerCount := 8
	for r := 0; r < readerCount; r++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					// This is the exact production call path:
					// 1. Get samples from manager (returns defensive copy from tracker)
					samples := m.GetRecentICMPLatencySamples(targetID, capacity)

					// 2. Record current latency
					m.RecordICMPLatency(targetID, float64(50+(readerID*7)%50), true)

					// 3. Call spike detection with the retrieved samples
					// This exercises the path: GetRecentICMPLatencySamples -> DetectAndRecord -> calculateMedian
					detector := NewSpikeDetector()
					detector.DetectAndRecord(
						targetID,
						"icmp",
						float64(100+readerID*10),
						time.Now().UTC(),
						true,
						nil,
						nil,
						nil,
						samples,
							nil, // httpTrace
					)

					runtime.Gosched()
				}
			}
		}(r)
	}

	// Run stress test for 3 seconds
	time.Sleep(3 * time.Second)
	close(stopCh)
	wg.Wait()

	// Verify manager is still functional
	samples := m.GetRecentICMPLatencySamples(targetID, capacity)
	if len(samples) > capacity {
		t.Errorf("GetRecentICMPLatencySamples(%d) returned %d samples", capacity, len(samples))
	}
	summary := m.GetICMPLatencySummary(targetID)
	if summary.SampleCount > capacity {
		t.Errorf("ICMP sample count %d exceeds capacity %d", summary.SampleCount, capacity)
	}
}

// TestLatencyTracker_ReturnedSliceIsDefensiveCopy verifies that GetRecentSamples
// returns a truly independent copy by mutating the returned slice and checking
// the tracker state is unaffected.
func TestLatencyTracker_ReturnedSliceIsDefensiveCopy(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 100)

	// Record some samples
	for i := 0; i < 50; i++ {
		lt.Record(float64(50+i), true)
	}

	// Get samples
	samples := lt.GetRecentSamples(50)
	if len(samples) != 50 {
		t.Fatalf("Expected 50 samples, got %d", len(samples))
	}

	// Mutate the returned slice
	originalValue := samples[0].LatencyMs
	samples[0].LatencyMs = 99999.0

	// Get samples again and verify original value is unchanged
	samples2 := lt.GetRecentSamples(50)
	if samples2[0].LatencyMs != originalValue {
		t.Errorf("Mutation of returned slice affected tracker state: expected %v, got %v",
			originalValue, samples2[0].LatencyMs)
	}
}
