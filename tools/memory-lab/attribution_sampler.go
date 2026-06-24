// attribution_sampler.go — UVB-76 memory attribution RSS/PSS sampler
//
// Provides continuous RSS/PSS sampling during attribution lab runs.
// Samples from /proc/<pid>/smaps_rollup at configured intervals.
//
// Reference: kgb://doctrine/embedded-memory-frugality

package main

import (
	"context"
	"sync"
	"time"
)

// AttributionSampler samples RSS/PSS during the attribution run.
type AttributionSampler struct {
	pid       int
	samples   []RSSSample
	mu        sync.Mutex
	stopChan  chan struct{}
	wg        sync.WaitGroup
	sampleInt time.Duration
	startTime time.Time
}

// NewAttributionSampler creates a new RSS/PSS sampler.
func NewAttributionSampler(pid int, intervalMs int) *AttributionSampler {
	return &AttributionSampler{
		pid:       pid,
		samples:   make([]RSSSample, 0, 1000),
		stopChan:  make(chan struct{}),
		sampleInt: time.Duration(intervalMs) * time.Millisecond,
	}
}

// Start begins concurrent RSS/PSS sampling.
// Takes synchronous sample immediately before starting ticker.
// This ensures first sample has elapsed_ms near 0.
func (s *AttributionSampler) Start(ctx context.Context) {
	s.startTime = time.Now()

	// Take immediate first sample before ticker starts
	if snap, err := ReadMemorySnapshot(s.pid); err == nil {
		s.mu.Lock()
		s.samples = append(s.samples, RSSSample{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			ElapsedMs: time.Since(s.startTime).Milliseconds(),
			RSSKiB:    snap.RSSKiB,
			PSSKiB:    snap.PSSKiB,
		})
		s.mu.Unlock()
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.sampleInt)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stopChan:
				return
			case <-ticker.C:
				if snap, err := ReadMemorySnapshot(s.pid); err == nil {
					s.mu.Lock()
					s.samples = append(s.samples, RSSSample{
						Timestamp: time.Now().UTC().Format(time.RFC3339),
						ElapsedMs: time.Since(s.startTime).Milliseconds(),
						RSSKiB:    snap.RSSKiB,
						PSSKiB:    snap.PSSKiB,
					})
					s.mu.Unlock()
				}
			}
		}
	}()
}

// Stop halts sampling and waits for completion.
func (s *AttributionSampler) Stop() {
	close(s.stopChan)
	s.wg.Wait()
}

// Samples returns a copy of all collected samples.
func (s *AttributionSampler) Samples() []RSSSample {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]RSSSample, len(s.samples))
	copy(result, s.samples)
	return result
}
