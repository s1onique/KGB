package state

import (
	"context"
	"sync"
	"testing"
	"time"
)

// Contract tests for concurrent runtime state access (part 2 - invariant assertions).
// These tests verify the runtime state invariants established by ACT-UVB76-HULK01.
// Run with: go test -race ./state -run TestConcurrencyContract

func TestConcurrencyContract_InvariantAssertions(t *testing.T) {
	mgr := NewManager()

	// Pre-populate state
	for i := 0; i < 100; i++ {
		mgr.RecordLatency("target1", float64(i), true)
		mgr.RecordICMPLatency("target1", float64(i%50), true)
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mgr.RecordLatency("target1", float64(i%200), i%5 != 0)
			}
		}
	}()

	// Invariant checker - verifies counts never go negative
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				summary := mgr.GetLatencySummary("target1")
				// INVARIANT: sample count never negative
				if summary.SampleCount < 0 {
					t.Errorf("sample_count is negative: %d", summary.SampleCount)
				}
				// INVARIANT: error count never negative
				if summary.ErrorCount < 0 {
					t.Errorf("error_count is negative: %d", summary.ErrorCount)
				}
				// INVARIANT: error count <= total samples
				if summary.ErrorCount > summary.SampleCount {
					t.Errorf("error_count (%d) > sample_count (%d)", summary.ErrorCount, summary.SampleCount)
				}
				samples := mgr.GetRecentLatencySamples("target1", 100)
				if len(samples) < 0 {
					t.Errorf("recent samples length is negative: %d", len(samples))
				}
				if len(samples) > 100 {
					t.Errorf("recent samples exceed capacity: %d", len(samples))
				}
			}
		}
	}()

	wg.Wait()
}

func TestConcurrencyContract_MultipleTargets(t *testing.T) {
	mgr := NewManager()
	targets := []string{"target1", "target2", "target3"}

	// Pre-populate all targets
	for _, target := range targets {
		for i := 0; i < 50; i++ {
			mgr.RecordLatency(target, float64(i), true)
		}
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Concurrent writers for all targets
	for _, target := range targets {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for i := 0; ; i++ {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					mgr.RecordLatency(t, float64(i%100), i%4 != 0)
				}
			}
		}(target)
	}

	// Concurrent readers for all targets
	for _, target := range targets {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					_ = mgr.GetLatencySummary(t)
					_ = mgr.GetRecentLatencySamples(t, 50)
				}
			}
		}(target)
	}

	wg.Wait()
}

func TestConcurrencyContract_ICMPHTTPParallel(t *testing.T) {
	mgr := NewManager()

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// ICMP writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mgr.RecordICMPLatency("target1", float64(i%100), i%10 != 0)
			}
		}
	}()

	// HTTP writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mgr.RecordLatency("target1", float64(i%200), i%5 != 0)
			}
		}
	}()

	// ICMP reader
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				samples := mgr.GetRecentICMPLatencySamples("target1", 100)
				_ = len(samples) // Just verify no crash
			}
		}
	}()

	// HTTP reader
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				_ = mgr.GetLatencySummary("target1")
			}
		}
	}()

	wg.Wait()
}

func TestConcurrencyContract_SnapshotConsistency(t *testing.T) {
	mgr := NewManager()

	// Pre-populate
	for i := 0; i < 200; i++ {
		mgr.RecordLatency("target1", float64(i%100), i%7 != 0)
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mgr.RecordLatency("target1", float64(i%150), i%3 != 0)
			}
		}
	}()

	// Snapshot consistency checker
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				snap := mgr.GetHTTPSnapshot("target1", 100)
				if snap == nil {
					t.Errorf("snapshot is nil")
					continue
				}
				// INVARIANT: retained sample count never negative
				if snap.Count < 0 {
					t.Errorf("Count is negative: %d", snap.Count)
				}
				// INVARIANT: retained sample count never exceeds capacity
				if snap.Count > 100 {
					t.Errorf("Count (%d) exceeds capacity (100)", snap.Count)
				}
			}
		}
	}()

	wg.Wait()
}

func TestConcurrencyContract_RetainedSampleCountBound(t *testing.T) {
	mgr := NewManager()
	capacity := 100

	// Pre-populate to capacity
	for i := 0; i < capacity; i++ {
		mgr.RecordLatency("target1", float64(i), true)
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Continuous writer (exceeds capacity)
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(1 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mgr.RecordLatency("target1", float64(i), true)
			}
		}
	}()

	// Count verifier
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				samples := mgr.GetRecentLatencySamples("target1", capacity)
				// INVARIANT: retained sample count never exceeds capacity
				if len(samples) > capacity {
					t.Errorf("retained count (%d) exceeds capacity (%d)", len(samples), capacity)
				}
				// INVARIANT: retained sample count never negative
				if len(samples) < 0 {
					t.Errorf("retained count is negative: %d", len(samples))
				}
			}
		}
	}()

	wg.Wait()
}
