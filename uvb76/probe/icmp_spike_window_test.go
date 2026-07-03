package probe

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
	"github.com/s1onique/KGB/uvb76/state"
)

// mockICMPSampleRecorder implements ICMPSampleRecorder for testing.
// Records all calls and observed limits for verification.
// Note: This mock stores raw samples internally but only exposes domain.SampleWindow externally.
type mockICMPSampleRecorder struct {
	mu        sync.RWMutex
	samples   map[string][]state.LatencySample
	// Track calls to GetICMPSampleWindow
	lastWindowLimit int
	allWindowLimits []int
	windowCalls    int
}

func newMockICMPSampleRecorder() *mockICMPSampleRecorder {
	return &mockICMPSampleRecorder{
		samples:        make(map[string][]state.LatencySample),
		allWindowLimits: make([]int, 0),
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

// GetICMPSampleWindow implements ICMPSampleRecorder for testing.
// Inlines the slice limiting logic since GetRecentICMPLatencySamples is no longer part of the interface.
func (m *mockICMPSampleRecorder) GetICMPSampleWindow(targetID string, limit int) domain.SampleWindow {
	m.mu.Lock()
	m.lastWindowLimit = limit
	m.allWindowLimits = append(m.allWindowLimits, limit)
	m.windowCalls++
	m.mu.Unlock()

	m.mu.RLock()
	defer m.mu.RUnlock()
	samples := m.samples[targetID]
	if len(samples) == 0 {
		return domain.SampleWindow{}
	}

	// Inline slice limiting (previously done in GetRecentICMPLatencySamples)
	var samplesToConvert []state.LatencySample
	if len(samples) <= limit {
		samplesToConvert = samples
	} else {
		// Return most recent samples
		samplesToConvert = samples[len(samples)-limit:]
	}

	domainSamples := make([]domain.Sample, len(samplesToConvert))
	for i, s := range samplesToConvert {
		domainSamples[i] = state.LatencySampleToDomainSampleWithKind(s, domain.ProbeKindICMP)
	}

	return domain.NewSampleWindow(domainSamples)
}

// DetectAndRecordSpikeWithWindow implements ICMPSampleRecorder for testing.
func (m *mockICMPSampleRecorder) DetectAndRecordSpikeWithWindow(targetID, kind string, latencyMs float64, sampleTs time.Time, reachable bool, schedulerDelayMs *float64, httpStatus *int, probeError *string, previousWindow domain.SampleWindow, httpTrace *state.HTTPTrace) *state.SpikeEvent {
	// Minimal implementation for testing - just return nil
	return nil
}

// GetWindowCalls returns the number of calls to GetICMPSampleWindow
func (m *mockICMPSampleRecorder) GetWindowCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.windowCalls
}

// GetLastWindowLimit returns the last limit observed by GetICMPSampleWindow
func (m *mockICMPSampleRecorder) GetLastWindowLimit() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastWindowLimit
}

// GetAllWindowLimits returns all limits observed by GetICMPSampleWindow
func (m *mockICMPSampleRecorder) GetAllWindowLimits() []int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]int, len(m.allWindowLimits))
	copy(result, m.allWindowLimits)
	return result
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
	recorder.windowCalls = 0
	recorder.lastWindowLimit = 0
	recorder.mu.Unlock()
	
	// Call probeTarget - it should use MaxICMPSpikeDetectionSamples (120)
	// not RecentSamplesMax (3600)
	client.probeTarget(target.ID)
	
	// Verify GetICMPSampleWindow was called (the domain boundary method)
	if recorder.GetWindowCalls() == 0 {
		t.Fatal("GetICMPSampleWindow was not called")
	}
	
	// Verify the exact limit that was requested
	lastLimit := recorder.GetLastWindowLimit()
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
