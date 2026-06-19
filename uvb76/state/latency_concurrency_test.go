package state

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestLatencyTracker_ConcurrentRecordAndGetStress tests concurrent RecordAt and
// GetRecentSamples operations with ICMP-capacity-sized buffers (3600 samples).
// This is a regression test for the SIGSEGV crash at 2026-06-19 01:16:28.
func TestLatencyTracker_ConcurrentRecordAndGetStress(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	capacity := 3600
	lt := NewLatencyTracker(buckets, capacity)

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	// Writer goroutines: simulate ICMP probe writes
	writerCount := 4
	for w := 0; w < writerCount; w++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			i := 0
			for {
				select {
				case <-stopCh:
					return
				default:
					latency := float64((i*17)%200) + 10.0 // varied latency 10-210ms
					reachable := i%10 != 0               // 10% failure rate
					lt.RecordAt(latency, reachable, time.Now().UTC())
					i++
					// Yield to increase chance of interleaving
					runtime.Gosched()
				}
			}
		}(w)
	}

	// Reader goroutines: simulate GetRecentSamples calls
	readerCount := 4
	for r := 0; r < readerCount; r++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			i := 0
			for {
				select {
				case <-stopCh:
					return
				default:
					// Test with various limits including the problematic 3600
					limits := []int{100, 500, 1000, 1800, 3600}
					limit := limits[i%len(limits)]
					samples := lt.GetRecentSamples(limit)
					// Verify returned slice is valid (no out-of-bounds crash)
					for _, s := range samples {
						_ = s.LatencyMs
						_ = s.Reachable
					}
					i++
					runtime.Gosched()
				}
			}
		}(r)
	}

	// Run for a fixed duration
	time.Sleep(2 * time.Second)
	close(stopCh)
	wg.Wait()

	// Final verification: buffer should be in valid state
	samples := lt.GetRecentSamples(capacity)
	if len(samples) > capacity {
		t.Errorf("GetRecentSamples(%d) returned %d samples (exceeds capacity)", capacity, len(samples))
	}
	if lt.count > capacity {
		t.Errorf("Internal count %d exceeds capacity %d", lt.count, capacity)
	}
}

// TestLatencyTracker_ConcurrentGetRecentSamples_ICMPCapacity is a targeted regression test
// for the crash path: GetRecentSamples(3600) during concurrent ICMP probe writes.
func TestLatencyTracker_ConcurrentGetRecentSamples_ICMPCapacity(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	capacity := 3600
	lt := NewLatencyTracker(buckets, capacity)

	// Pre-fill buffer to near capacity to maximize race conditions
	for i := 0; i < capacity-10; i++ {
		lt.RecordAt(float64(i%100)+10.0, true, time.Now().UTC())
	}

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	// Single writer to simulate ongoing ICMP probes
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopCh:
				return
			default:
				lt.RecordAt(50.0, true, time.Now().UTC())
				runtime.Gosched()
			}
		}
	}()

	// Multiple concurrent readers requesting the full ICMP capacity
	readerCount := 8
	for r := 0; r < readerCount; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					// This is the exact crash path: GetRecentSamples(3600)
					samples := lt.GetRecentSamples(capacity)
					// Verify no corruption
					if len(samples) > capacity {
						t.Errorf("Read returned %d samples (expected max %d)", len(samples), capacity)
					}
					runtime.Gosched()
				}
			}
		}()
	}

	// Run stress test
	time.Sleep(2 * time.Second)
	close(stopCh)
	wg.Wait()

	// Final verification
	samples := lt.GetRecentSamples(capacity)
	if len(samples) > capacity {
		t.Errorf("Final GetRecentSamples(%d) returned %d samples", capacity, len(samples))
	}
}

// TestLatencyTracker_ConcurrentRecordAt_GetSampleTimestamps tests concurrent
// RecordAt and GetSampleTimestamps operations.
func TestLatencyTracker_ConcurrentRecordAt_GetSampleTimestamps(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	capacity := 3600
	lt := NewLatencyTracker(buckets, capacity)

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	// Writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stopCh:
				return
			default:
				lt.RecordAt(float64(i%100)+10.0, true, time.Now().UTC())
				runtime.Gosched()
			}
		}
	}()

	// Reader
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopCh:
				return
			default:
				oldest, newest := lt.GetSampleTimestamps()
				if oldest != nil && newest != nil {
					// Verify ordering if both present
					if oldest.After(*newest) {
						t.Errorf("Oldest timestamp is after newest: oldest=%v, newest=%v",
							oldest, newest)
					}
				}
				runtime.Gosched()
			}
		}
	}()

	time.Sleep(2 * time.Second)
	close(stopCh)
	wg.Wait()
}

// TestLatencyTracker_ConcurrentGetSummary tests concurrent GetSummary operations.
func TestLatencyTracker_ConcurrentGetSummary(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	capacity := 3600
	lt := NewLatencyTracker(buckets, capacity)

	// Pre-fill
	for i := 0; i < 1000; i++ {
		lt.Record(float64(i%100)+10.0, i%10 != 0)
	}

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	// Writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stopCh:
				return
			default:
				lt.Record(float64(i%100)+10.0, true)
				runtime.Gosched()
			}
		}
	}()

	// Multiple readers
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					summary := lt.GetSummary("test-target")
					// Verify internal consistency
					if summary.SampleCount < 0 {
						t.Errorf("Negative sample count: %d", summary.SampleCount)
					}
					if summary.SampleCount > capacity {
						t.Errorf("Sample count %d exceeds capacity %d", summary.SampleCount, capacity)
					}
					runtime.Gosched()
				}
			}
		}()
	}

	time.Sleep(2 * time.Second)
	close(stopCh)
	wg.Wait()
}

// TestManager_ConcurrentRecordAndGet_ICMPRegression is a regression test at the
// Manager level for the crash path: Manager.GetRecentICMPLatencySamples(3600).
func TestManager_ConcurrentRecordAndGet_ICMPRegression(t *testing.T) {
	m := NewManager()
	m.ConfigureICMP([]int64{5, 10, 25, 50, 100}, 3600)

	targetID := "test-target"
	capacity := m.GetICMPMaxSamples()

	// Pre-fill near capacity
	for i := 0; i < capacity-10; i++ {
		m.RecordICMPLatencyAt(targetID, float64(i%100)+10.0, true, time.Now().UTC())
	}

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	// Writer simulating ICMP probe
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stopCh:
				return
			default:
				m.RecordICMPLatency(targetID, 50.0, true)
				runtime.Gosched()
			}
		}
	}()

	// Multiple concurrent readers
	readerCount := 8
	for r := 0; r < readerCount; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					// This is the exact crash path
					samples := m.GetRecentICMPLatencySamples(targetID, 3600)
					if len(samples) > capacity {
						t.Errorf("Read returned %d samples (expected max %d)", len(samples), capacity)
					}
					runtime.Gosched()
				}
			}
		}()
	}

	time.Sleep(2 * time.Second)
	close(stopCh)
	wg.Wait()

	// Final verification
	samples := m.GetRecentICMPLatencySamples(targetID, capacity)
	if len(samples) > capacity {
		t.Errorf("Final GetRecentICMPLatencySamples(%d) returned %d samples",
			capacity, len(samples))
	}
}
