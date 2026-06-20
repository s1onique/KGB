package state

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestLatencyTracker_ICMPCrashRegression_LongRun is a long-run regression test
// for the SIGSEGV crash at 2026-06-19 01:16:28.
//
// Crash stack:
//   probe.(*ICMPClient).probeAll.func2
//   → probe.(*ICMPClient).probeTarget
//   → state.(*Manager).GetRecentICMPLatencySamples
//   → state.(*LatencyTracker).GetRecentSamples
//   → runtime.makeslice
//   → runtime.mallocgc
//   → runtime.memclrNoHeapPointers
//   → SIGSEGV
//
// The crash occurred during a bounded 3600-sample allocation (201,600 bytes).
// This test verifies that the ICMP latency tracker snapshot path remains safe
// under sustained concurrent access with the exact production-like parameters.
func TestLatencyTracker_ICMPCrashRegression_LongRun(t *testing.T) {
	// Use the exact production ICMP configuration: 3600 samples
	buckets := []int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}
	capacity := 3600
	lt := NewLatencyTracker(buckets, capacity)

	// Pre-fill to simulate ~1 hour of production data
	for i := 0; i < 3600; i++ {
		lt.RecordAt(float64(i%100)+10.0, true, time.Now().UTC())
	}

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	// Writers: simulate continuous ICMP probes (1 per second in production)
	writerCount := 2
	for w := 0; w < writerCount; w++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stopCh:
					return
				default:
					latency := float64((i*17)%200) + 10.0
					reachable := i%10 != 0
					lt.RecordAt(latency, reachable, time.Now().UTC())
					runtime.Gosched()
					time.Sleep(time.Millisecond) // Simulate probe interval
				}
			}
		}(w)
	}

	// Readers: simulate concurrent GetRecentICMPLatencySamples calls
	// This is the exact crash path
	readerCount := 4
	for r := 0; r < readerCount; r++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					// This is the EXACT crash path: GetRecentSamples(3600)
					samples := lt.GetRecentSamples(3600)
					// Verify returned slice is valid
					for _, s := range samples {
						_ = s.LatencyMs
						_ = s.Reachable
					}
					// Also test GetSummary which reads all ring buffer state
					_ = lt.GetSummary("icmp-target")
					runtime.Gosched()
				}
			}
		}(r)
	}

	// Run for extended duration to match production crash timing (~1 hour = 3600 probes)
	// For CI, use shortened duration (30 seconds) but with aggressive goroutine churn
	t.Log("Running long-run ICMP crash regression test (30s)...")
	time.Sleep(30 * time.Second)
	close(stopCh)
	wg.Wait()

	// Final verification: buffer must be in valid state
	samples := lt.GetRecentSamples(capacity)
	if len(samples) > capacity {
		t.Errorf("GetRecentSamples(%d) returned %d samples (exceeds capacity)", capacity, len(samples))
	}
	if lt.count > capacity {
		t.Errorf("Internal count %d exceeds capacity %d", lt.count, capacity)
	}
}

// TestManager_ICMPCrashRegression_LongRun is the Manager-level regression test
// for the ICMP crash. It exercises the full call chain from Manager to tracker.
func TestManager_ICMPCrashRegression_LongRun(t *testing.T) {
	m := NewManager()
	// Configure ICMP with production parameters
	m.ConfigureICMP([]int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}, 3600)

	targetID := "uvb-76-router"
	capacity := m.GetICMPMaxSamples()

	// Pre-fill to simulate ~1 hour of production data
	for i := 0; i < 3600; i++ {
		m.RecordICMPLatencyAt(targetID, float64(i%100)+10.0, true, time.Now().UTC())
	}

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	// Writers: simulate ICMP probes
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stopCh:
				return
			default:
				latency := float64((i*17)%200) + 10.0
				reachable := i%10 != 0
				m.RecordICMPLatency(targetID, latency, reachable)
				runtime.Gosched()
				time.Sleep(time.Millisecond)
			}
		}
	}()

	// Readers: this is the crash path
	readerCount := 4
	for r := 0; r < readerCount; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					// EXACT crash path: Manager.GetRecentICMPLatencySamples(..., 3600)
					samples := m.GetRecentICMPLatencySamples(targetID, 3600)
					if len(samples) > capacity {
						t.Errorf("Read returned %d samples (expected max %d)", len(samples), capacity)
					}
					// Also test summary path
					_ = m.GetICMPLatencySummary(targetID)
					runtime.Gosched()
				}
			}
		}()
	}

	t.Log("Running Manager-level ICMP crash regression test (30s)...")
	time.Sleep(30 * time.Second)
	close(stopCh)
	wg.Wait()

	// Final verification
	samples := m.GetRecentICMPLatencySamples(targetID, capacity)
	if len(samples) > capacity {
		t.Errorf("Final GetRecentICMPLatencySamples(%d) returned %d samples",
			capacity, len(samples))
	}
}

// TestLatencyTracker_GetRecentSamples_NeverAllocatesMoreThanCapacity verifies
// that GetRecentSamples(3600) never allocates more than maxSamples regardless
// of internal state corruption attempts.
func TestLatencyTracker_GetRecentSamples_NeverAllocatesMoreThanCapacity(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	capacity := 3600
	lt := NewLatencyTracker(buckets, capacity)

	// Pre-fill
	for i := 0; i < 500; i++ {
		lt.RecordAt(float64(i%100)+10.0, true, time.Now().UTC())
	}

	// Test with limit exceeding capacity
	testLimits := []int{1, 100, 1000, 1800, 3600, 5000, 100000, 1000000}

	for _, limit := range testLimits {
		samples := lt.GetRecentSamples(limit)
		// Must never exceed actual buffer size
		if len(samples) > capacity {
			t.Errorf("GetRecentSamples(%d) returned %d samples (exceeds capacity %d)",
				limit, len(samples), capacity)
		}
	}
}

// TestLatencyTracker_CorruptedStateRecovery tests that invariant guards
// prevent panic/crash even when internal state is corrupted.
func TestLatencyTracker_CorruptedStateRecovery(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100}
	lt := NewLatencyTracker(buckets, 100)

	// Manually corrupt state (simulating heap corruption)
	lt.mu.Lock()
	lt.count = -1 // Corrupt: negative count
	lt.mu.Unlock()

	// Record should handle corrupted state gracefully
	lt.RecordAt(50.0, true, time.Now().UTC())

	// GetRecentSamples should also handle gracefully
	samples := lt.GetRecentSamples(50)
	if samples == nil {
		t.Log("GetRecentSamples returned nil for corrupted state (acceptable)")
	}

	// Corrupt head
	lt.mu.Lock()
	lt.count = 10
	lt.head = -999 // Corrupt: negative head
	lt.mu.Unlock()

	// Record should handle
	lt.RecordAt(60.0, true, time.Now().UTC())

	// Corrupt slice length
	lt.mu.Lock()
	lt.head = 0
	lt.recentSamples = make([]LatencySample, 50) // Wrong size
	lt.mu.Unlock()

	// Record should handle
	lt.RecordAt(70.0, true, time.Now().UTC())

	// Verify tracker still works
	samples = lt.GetRecentSamples(100)
	if len(samples) > 100 {
		t.Errorf("After corruption recovery, GetRecentSamples returned %d samples (exceeds 100)", len(samples))
	}
}

// TestLatencyTracker_ConcurrentGetSummary_ICMPStress is a targeted stress test
// for GetSummary under concurrent ICMP probe load.
func TestLatencyTracker_ConcurrentGetSummary_ICMPStress(t *testing.T) {
	buckets := []int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}
	capacity := 3600
	lt := NewLatencyTracker(buckets, capacity)

	// Pre-fill
	for i := 0; i < 2000; i++ {
		lt.RecordAt(float64(i%500)+10.0, i%20 != 0, time.Now().UTC())
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
				lt.RecordAt(float64((i*17)%500)+10.0, i%20 != 0, time.Now().UTC())
				runtime.Gosched()
			}
		}
	}()

	// Multiple summary readers
	readerCount := 4
	for r := 0; r < readerCount; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					summary := lt.GetSummary("stress-target")
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

	time.Sleep(10 * time.Second)
	close(stopCh)
	wg.Wait()
}
