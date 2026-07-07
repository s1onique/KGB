package state

import (
	"context"
	"sync"
	"testing"
	"time"
)

// Contract tests for concurrent runtime state access (part 1).
// These tests verify the runtime state invariants established by ACT-UVB76-HULK01.
// Run with: go test -race ./state -run TestConcurrencyContract

func TestConcurrencyContract_BasicRace(t *testing.T) {
	// Basic concurrent read/write test
	mgr := NewManager()

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Writer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				mgr.RecordLatency("target1", float64(i%100), true)
			}
		}
	}()

	// Reader goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				_ = mgr.GetLatencySummary("target1")
				_ = mgr.GetRecentLatencySamples("target1", 10)
			}
		}
	}()

	wg.Wait()
}

func TestConcurrencyContract_ICMPHTTPConcurrentWrites(t *testing.T) {
	mgr := NewManager()

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// ICMP writer (high cadence)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				mgr.RecordICMPLatency("target1", float64(i%50), true)
			}
		}
	}()

	// HTTP writer (lower cadence)
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
				mgr.RecordLatency("target1", float64(i%100), true)
			}
		}
	}()

	wg.Wait()
}

func TestConcurrencyContract_ReaderWhileWriting(t *testing.T) {
	mgr := NewManager()

	// Pre-populate some data using RecordLatency (no timestamp variant for HTTP)
	for i := 0; i < 50; i++ {
		mgr.RecordLatency("target1", float64(i), true)
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Continuous writer
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

	// Continuous reader - this was a crash site historically
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				_ = mgr.GetRecentLatencySamples("target1", 20)
				_ = mgr.GetLatencySummary("target1")
				oldest, newest := mgr.GetLatencySampleTimestamps("target1")
				_ = oldest
				_ = newest
			}
		}
	}()

	wg.Wait()
}

func TestConcurrencyContract_LatencySeriesAPICongestion(t *testing.T) {
	mgr := NewManager()

	// Pre-populate
	for i := 0; i < 100; i++ {
		mgr.RecordLatency("target1", float64(i), true)
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
				mgr.RecordLatency("target1", float64(i%200), true)
			}
		}
	}()

	// Latency series reader (like API handler)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				snap := mgr.GetHTTPSnapshot("target1", 100)
				_ = snap
			}
		}
	}()

	wg.Wait()
}

func TestConcurrencyContract_SpikeClassification(t *testing.T) {
	mgr := NewManager()

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Writer
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
				latency := float64(i % 200)
				mgr.RecordLatency("target1", latency, latency < 150)
			}
		}
	}()

	// Spike event reader
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
