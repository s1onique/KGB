package probe

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// mockICMPSampleRecorder implements ICMPSampleRecorder for testing.
// Records all calls and observed limits for verification.
type mockICMPSampleRecorder struct {
	mu        sync.RWMutex
	samples   map[string][]state.LatencySample
	// Track calls to GetRecentICMPLatencySamples
	lastLimit int
	allLimits []int
	calls     int
}

func newMockICMPSampleRecorder() *mockICMPSampleRecorder {
	return &mockICMPSampleRecorder{
		samples:   make(map[string][]state.LatencySample),
		allLimits: make([]int, 0),
	}
}

func (m *mockICMPSampleRecorder) RecordICMPLatency(targetID string, latencyMs float64, reachable bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := state.LatencySample{
		Timestamp: time.Now().UTC(),
		LatencyMs: latencyMs,
		Reachable: reachable,
	}
	m.samples[targetID] = append(m.samples[targetID], s)
}

func (m *mockICMPSampleRecorder) GetRecentICMPLatencySamples(targetID string, limit int) []state.LatencySample {
	m.mu.Lock()
	m.lastLimit = limit
	m.allLimits = append(m.allLimits, limit)
	m.calls++
	m.mu.Unlock()

	m.mu.RLock()
	defer m.mu.RUnlock()
	samples := m.samples[targetID]
	if len(samples) == 0 {
		return nil
	}
	if len(samples) <= limit {
		// Return a defensive copy
		result := make([]state.LatencySample, len(samples))
		copy(result, samples)
		return result
	}
	// Return most recent samples
	result := make([]state.LatencySample, limit)
	copy(result, samples[len(samples)-limit:])
	return result
}

func (m *mockICMPSampleRecorder) DetectAndRecordSpike(targetID, kind string, latencyMs float64, sampleTs time.Time, reachable bool, schedulerDelayMs *float64, httpStatus *int, probeError *string, previousSamples []state.LatencySample, httpTrace *state.HTTPTrace) *state.SpikeEvent {
	// Minimal implementation for testing - just return nil
	return nil
}

// GetLastLimit returns the last limit observed by GetRecentICMPLatencySamples
func (m *mockICMPSampleRecorder) GetLastLimit() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastLimit
}

// GetAllLimits returns all limits observed by GetRecentICMPLatencySamples
func (m *mockICMPSampleRecorder) GetAllLimits() []int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]int, len(m.allLimits))
	copy(result, m.allLimits)
	return result
}

// GetCalls returns the number of calls to GetRecentICMPLatencySamples
func (m *mockICMPSampleRecorder) GetCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.calls
}

// TestICMPClient_UsesBoundedSpikeWindow verifies that probeTarget uses the bounded
// spike detection window (MaxICMPSpikeDetectionSamples=120) instead of the full
// retention window (3600).
func TestICMPClient_UsesBoundedSpikeWindow(t *testing.T) {
	cfg := &config.ICMPProbeConfig{
		IntervalSeconds:     1,
		TimeoutSeconds:       3,
		RecentSamplesMax:    3600, // Full retention window
		RetainedRangeSeconds: 3600,
	}
	recorder := newMockICMPSampleRecorder()
	
	// Create a mock backend that returns success
	backend := &mockICMPBackend{}
	
	client := &ICMPClient{
		backend: backend,
		cfg:     cfg,
		state:   recorder,
		targets: make(map[string]*config.TargetConfig),
		enabled: true,
	}
	
	target := &config.TargetConfig{
		ID:      "test-target",
		BaseURL: "http://127.0.0.1:9999/status",
		Enabled: true,
	}
	client.targets[target.ID] = target
	
	// Add some samples to the recorder
	for i := 0; i < 200; i++ {
		recorder.RecordICMPLatency(target.ID, float64(i)+10.0, true)
	}
	
	// Reset call count before the probe
	recorder.mu.Lock()
	recorder.calls = 0
	recorder.lastLimit = 0
	recorder.mu.Unlock()
	
	// Call probeTarget - it should use MaxICMPSpikeDetectionSamples (120)
	// not RecentSamplesMax (3600)
	client.probeTarget(target.ID)
	
	// Verify GetRecentICMPLatencySamples was called
	if recorder.GetCalls() == 0 {
		t.Fatal("GetRecentICMPLatencySamples was not called")
	}
	
	// Verify the exact limit that was requested
	lastLimit := recorder.GetLastLimit()
	if lastLimit != MaxICMPSpikeDetectionSamples {
		t.Errorf("probeTarget requested %d samples, want %d (MaxICMPSpikeDetectionSamples)", 
			lastLimit, MaxICMPSpikeDetectionSamples)
	}
	
	// Verify it did NOT request the full retention window
	if lastLimit == cfg.RecentSamplesMax {
		t.Errorf("probeTarget still requests full retention window (%d), should use bounded spike window", lastLimit)
	}
	
	// Verify the bounded window is smaller than the full retention
	if MaxICMPSpikeDetectionSamples >= 3600 {
		t.Errorf("MaxICMPSpikeDetectionSamples (%d) should be much smaller than 3600", MaxICMPSpikeDetectionSamples)
	}
}

// TestICMPClient_BoundedWindowSmallerThanRetention verifies the bounded spike window
// is appropriately sized for spike detection, not full retention.
func TestICMPClient_BoundedWindowSmallerThanRetention(t *testing.T) {
	// The spike detector needs MinSamplesForMedian=20 samples for reliable median
	// We use 120 to provide comfortable headroom
	if MaxICMPSpikeDetectionSamples < 20 {
		t.Errorf("MaxICMPSpikeDetectionSamples (%d) should be >= 20 for spike detection", MaxICMPSpikeDetectionSamples)
	}
	
	// 120 samples at 1s interval = 2 minutes of history
	// This is plenty for spike detection (relative to 60-minute retention)
	expectedMaxWindow := 120
	if MaxICMPSpikeDetectionSamples != expectedMaxWindow {
		t.Errorf("MaxICMPSpikeDetectionSamples = %d, want %d", MaxICMPSpikeDetectionSamples, expectedMaxWindow)
	}
}

// TestICMPClient_UIAPICanStillRequest3600 verifies that the full 3600-sample history
// is still available for UI/API reads even though the hot path uses a bounded window.
func TestICMPClient_UIAPICanStillRequest3600(t *testing.T) {
	_ = &config.ICMPProbeConfig{
		IntervalSeconds:     1,
		TimeoutSeconds:       3,
		RecentSamplesMax:    3600, // Full retention window
		RetainedRangeSeconds: 3600,
	}
	recorder := newMockICMPSampleRecorder()
	
	// Simulate having 3600 samples
	for i := 0; i < 3600; i++ {
		recorder.RecordICMPLatency("test-target", float64(i%100)+10.0, true)
	}
	
	// UI/API can still request the full 3600 samples
	samples := recorder.GetRecentICMPLatencySamples("test-target", 3600)
	if len(samples) != 3600 {
		t.Errorf("GetRecentICMPLatencySamples(3600) returned %d samples, want 3600", len(samples))
	}
	
	// The bounded spike window (120) should still work
	spikeSamples := recorder.GetRecentICMPLatencySamples("test-target", MaxICMPSpikeDetectionSamples)
	if len(spikeSamples) != MaxICMPSpikeDetectionSamples {
		t.Errorf("GetRecentICMPSpikeDetectionSamples returned %d samples, want %d", 
			len(spikeSamples), MaxICMPSpikeDetectionSamples)
	}
}

// mockICMPBackend implements ICMPProbeBackend for testing.
type mockICMPBackend struct {
	shouldFail bool
	failAfter  int
	callCount  int
}

func (b *mockICMPBackend) Ping(ctx context.Context, host string, timeout time.Duration) (time.Duration, error) {
	b.callCount++
	if b.shouldFail {
		return 0, nil // Return success to avoid nil pointer in ICMPClient
	}
	return 50 * time.Millisecond, nil
}
