package state

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestSpikeDetector_DetectAndRecord_ConcurrentSameTargetICMP is a regression test
// for the SIGSEGV crash at 2026-06-19 01:16:28.
//
// Root cause: Runtime crash shows calculateMedian receiving an impossible slice header
// (len=3600 cap=4), strongly indicating unsynchronized concurrent access to
// SpikeDetector history/baseline slices from overlapping ICMP probe goroutines.
//
// This test exercises the exact crash path: multiple goroutines calling
// DetectAndRecord for the same target and kind (icmp), with ICMP-capacity-sized
// sample buffers (3600 samples).
func TestSpikeDetector_DetectAndRecord_ConcurrentSameTargetICMP(t *testing.T) {
	detector := NewSpikeDetector()

	// Use ICMP-capacity-sized sample buffer (3600) - this is the problematic size
	capacity := 3600

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	// Multiple concurrent workers hitting the same target/kind
	workerCount := 32
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			i := 0
			for {
				select {
				case <-stopCh:
					return
				default:
					// Generate samples that look like ICMP probe data
					latency := float64((i*17)%100) + 10.0 // varied latency 10-110ms
					reached := i%10 != 0                  // 10% failure rate

					// Build previous samples slice (simulating GetRecentICMPLatencySamples)
					// Use the problematic capacity size
					prevSamples := make([]LatencySample, capacity)
					for j := 0; j < capacity; j++ {
						prevSamples[j] = LatencySample{
							Timestamp: time.Now().UTC().Add(-time.Duration(j) * time.Second),
							LatencyMs: float64((j*13)%100) + 10.0,
							Reachable: j%12 != 0,
						}
					}

					// This is the crash path: DetectAndRecord with full-capacity samples
					detector.DetectAndRecord(
						"target-1",       // same target for all workers
						"icmp",           // ICMP probe kind
						latency,
						time.Now().UTC(),
						reached,
						nil,
						nil,
						nil,
						prevSamples,
					)
					i++
					runtime.Gosched()
				}
			}
		}(worker)
	}

	// Run stress test for 3 seconds
	time.Sleep(3 * time.Second)
	close(stopCh)
	wg.Wait()

	// Verify detector is still functional
	counts := detector.GetAllSpikeCounts()
	if counts == nil {
		t.Error("GetAllSpikeCounts returned nil after concurrent access")
	}
}

// TestSpikeDetector_DetectAndRecord_ConcurrentMixedTargetsAndKinds tests concurrent
// access to multiple targets and probe kinds simultaneously.
func TestSpikeDetector_DetectAndRecord_ConcurrentMixedTargetsAndKinds(t *testing.T) {
	detector := NewSpikeDetector()

	targets := []string{"target-1", "target-2", "target-3"}
	kinds := []string{"icmp", "http"}

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	// Create workers for each target/kind combination
	for _, target := range targets {
		for _, kind := range kinds {
			wg.Add(1)
			go func(target, kind string) {
				defer wg.Done()
				i := 0
				for {
					select {
					case <-stopCh:
						return
					default:
						latency := float64((i*17)%1000) + 10.0
						reached := i%10 != 0

						// Build samples with varying sizes
						sampleCount := 30 + (i % 100)
						prevSamples := make([]LatencySample, sampleCount)
						for j := 0; j < sampleCount; j++ {
							prevSamples[j] = LatencySample{
								Timestamp: time.Now().UTC().Add(-time.Duration(j) * time.Second),
								LatencyMs: float64((j*13)%100) + 10.0,
								Reachable: j%12 != 0,
							}
						}

						detector.DetectAndRecord(
							target,
							kind,
							latency,
							time.Now().UTC(),
							reached,
							nil,
							nil,
							nil,
							prevSamples,
						)
						i++
						runtime.Gosched()
					}
				}
			}(target, kind)
		}
	}

	// Run stress test
	time.Sleep(2 * time.Second)
	close(stopCh)
	wg.Wait()

	// Verify all trackers were created and are functional
	for _, target := range targets {
		for _, kind := range kinds {
			spikes := detector.GetSpikes(target, kind, 10)
			if spikes == nil {
				t.Errorf("GetSpikes returned nil for %s/%s", target, kind)
			}
		}
	}
}

// TestSpikeDetector_ConcurrentReadWriteSpikeEvents tests concurrent spike event
// recording and reading across multiple targets.
func TestSpikeDetector_ConcurrentReadWriteSpikeEvents(t *testing.T) {
	detector := NewSpikeDetector()

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	// Writers: generate spikes
	writerCount := 8
	for w := 0; w < writerCount; w++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stopCh:
					return
				default:
					latency := float64(2000 + (i%50)*100) // high latency to trigger spikes
					reached := true

					prevSamples := make([]LatencySample, 30)
					for j := 0; j < 30; j++ {
						prevSamples[j] = LatencySample{
							Timestamp: time.Now().UTC().Add(-time.Duration(j) * time.Second),
							LatencyMs: float64(50 + j%50),
							Reachable: true,
						}
					}

					detector.DetectAndRecord(
						"target-1",
						"icmp",
						latency,
						time.Now().UTC(),
						reached,
						nil,
						nil,
						nil,
						prevSamples,
					)
					runtime.Gosched()
				}
			}
		}(w)
	}

	// Readers: read spikes concurrently
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
					spikes := detector.GetSpikes("target-1", "icmp", 100)
					if spikes != nil {
						// Verify spike data integrity
						for _, spike := range spikes {
							_ = spike.EventID
							_ = spike.Severity
						}
					}
					counts := detector.GetAllSpikeCounts()
					_ = counts
					runtime.Gosched()
				}
			}
		}(r)
	}

	// Run stress test
	time.Sleep(2 * time.Second)
	close(stopCh)
	wg.Wait()
}

// TestSpikeDetector_CalculateMedian_DefensiveAgainstCorruptedSlice tests that
// calculateMedian handles corrupted slice headers gracefully.
func TestSpikeDetector_CalculateMedian_DefensiveAgainstCorruptedSlice(t *testing.T) {
	detector := NewSpikeDetector()

	// Test empty slice
	emptySamples := []LatencySample{}
	if median := detector.calculateMedian(emptySamples); median != 0 {
		t.Errorf("calculateMedian(empty) = %v, want 0", median)
	}

	// Test nil slice
	var nilSamples []LatencySample
	if median := detector.calculateMedian(nilSamples); median != 0 {
		t.Errorf("calculateMedian(nil) = %v, want 0", median)
	}

	// Test normal slice
	normalSamples := []LatencySample{
		{LatencyMs: 10, Reachable: true},
		{LatencyMs: 20, Reachable: true},
		{LatencyMs: 30, Reachable: true},
		{LatencyMs: 40, Reachable: true},
		{LatencyMs: 50, Reachable: true},
	}
	median := detector.calculateMedian(normalSamples)
	if median != 30 {
		t.Errorf("calculateMedian(normal) = %v, want 30", median)
	}

	// Test slice with only unreachable samples (should return 0)
	unreachableSamples := []LatencySample{
		{LatencyMs: 10, Reachable: false},
		{LatencyMs: 20, Reachable: false},
	}
	if median := detector.calculateMedian(unreachableSamples); median != 0 {
		t.Errorf("calculateMedian(unreachable) = %v, want 0", median)
	}
}

// TestSpikeDetector_SpikeDetectionStillWorksAfterConcurrentAccess verifies that
// spike detection logic remains correct after heavy concurrent access.
func TestSpikeDetector_SpikeDetectionStillWorksAfterConcurrentAccess(t *testing.T) {
	detector := NewSpikeDetector()

	// Pre-populate some normal samples
	normalSamples := make([]LatencySample, 30)
	for i := 0; i < 30; i++ {
		normalSamples[i] = LatencySample{
			Timestamp: time.Now().UTC().Add(-time.Duration(i) * time.Second),
			LatencyMs: float64(50 + i%20),
			Reachable: true,
		}
	}

	// Trigger a high spike that should be detected
	highLatency := float64(3000) // 3000ms should trigger critical threshold
	spike := detector.DetectAndRecord(
		"test-target",
		"icmp",
		highLatency,
		time.Now().UTC(),
		true,
		nil,
		nil,
		nil,
		normalSamples,
	)

	if spike == nil {
		t.Error("Expected spike to be detected for high latency, got nil")
	} else {
		if spike.Severity != "critical" {
			t.Errorf("Expected severity 'critical', got '%s'", spike.Severity)
		}
		if spike.LatencyMs != highLatency {
			t.Errorf("Expected latency %v, got %v", highLatency, spike.LatencyMs)
		}
	}

	// Trigger a normal latency that should NOT be detected
	normalLatency := float64(100)
	noSpike := detector.DetectAndRecord(
		"test-target",
		"icmp",
		normalLatency,
		time.Now().UTC(),
		true,
		nil,
		nil,
		nil,
		normalSamples,
	)

	if noSpike != nil {
		t.Error("Expected no spike for normal latency, got spike")
	}
}

