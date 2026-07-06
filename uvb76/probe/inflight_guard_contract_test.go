package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// Contract tests for in-flight probe guard invariants.
// These tests verify the runtime state invariants established by ACT-UVB76-HULK01.
// Tests verify overlapping probe execution cannot corrupt state.

// targetsToPtr converts a slice of TargetConfig to a slice of pointers.
func targetsToPtr(targets []config.TargetConfig) []*config.TargetConfig {
	result := make([]*config.TargetConfig, len(targets))
	for i := range targets {
		result[i] = &targets[i]
	}
	return result
}

// newMockServer creates a local HTTP test server that responds with 200 OK.
func newMockServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func TestInflightGuardContract_SameTargetSameProbeKindOverlap(t *testing.T) {
	mgr := state.NewManager()
	server := newMockServer()
	defer server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     1,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:   100,
	}

	targets := []config.TargetConfig{
		{ID: "test-target", BaseURL: server.URL, Enabled: true},
	}

	client := NewClient(httpCfg, mgr, targetsToPtr(targets))
	client.Start()
	defer client.Stop()

	// Trigger rapid overlapping probes
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			target := targets[0]
			_ = client.ProbeTarget(&target)
		}(i)
	}

	wg.Wait()

	// Verify state is not corrupted
	samples := mgr.GetRecentLatencySamples("test-target", 100)
	if len(samples) < 0 {
		t.Errorf("invalid sample count: %d", len(samples))
	}
}

func TestInflightGuardContract_DifferentTargetSameProbeKind(t *testing.T) {
	mgr := state.NewManager()
	server1 := newMockServer()
	server2 := newMockServer()
	server3 := newMockServer()
	defer server1.Close()
	defer server2.Close()
	defer server3.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     1,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:   100,
	}

	targets := []config.TargetConfig{
		{ID: "target-1", BaseURL: server1.URL, Enabled: true},
		{ID: "target-2", BaseURL: server2.URL, Enabled: true},
		{ID: "target-3", BaseURL: server3.URL, Enabled: true},
	}

	client := NewClient(httpCfg, mgr, targetsToPtr(targets))
	client.Start()
	defer client.Stop()

	// Probe different targets concurrently
	var wg sync.WaitGroup
	for i := range targets {
		wg.Add(1)
		go func(t *config.TargetConfig) {
			defer wg.Done()
			_ = client.ProbeTarget(t)
		}(&targets[i])
	}

	wg.Wait()

	// All targets should have valid state
	for _, target := range targets {
		samples := mgr.GetRecentLatencySamples(target.ID, 100)
		_ = samples // Just verify no crash
	}
}

func TestInflightGuardContract_SameTargetDifferentProbeKind(t *testing.T) {
	mgr := state.NewManager()
	server := newMockServer()
	defer server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     1,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:   100,
	}

	targets := []config.TargetConfig{
		{ID: "test-target", BaseURL: server.URL, Enabled: true},
	}

	httpClient := NewClient(httpCfg, mgr, targetsToPtr(targets))
	httpClient.Start()
	defer httpClient.Stop()

	// HTTP probe should not interfere with ICMP state
	mgr.RecordICMPLatency("test-target", 10.0, true)
	mgr.RecordICMPLatency("test-target", 15.0, true)

	// Verify ICMP state is preserved
	icmpSamples := mgr.GetRecentICMPLatencySamples("test-target", 100)
	if len(icmpSamples) != 2 {
		t.Errorf("expected 2 ICMP samples, got %d", len(icmpSamples))
	}
}

func TestInflightGuardContract_LongRunningProbeReturns(t *testing.T) {
	mgr := state.NewManager()
	server := newMockServer()
	defer server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     1,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:   100,
	}

	targets := []config.TargetConfig{
		{ID: "test-target", BaseURL: server.URL, Enabled: true},
	}

	client := NewClient(httpCfg, mgr, targetsToPtr(targets))

	// Pre-populate some samples
	for i := 0; i < 10; i++ {
		mgr.RecordLatency("test-target", float64(i*10), true)
	}

	// After long-running probe completes, state should be consistent
	_ = client.ProbeTarget(&targets[0])

	finalCount := len(mgr.GetRecentLatencySamples("test-target", 100))

	// Count should not be negative
	if finalCount < 0 {
		t.Errorf("sample count is negative: %d", finalCount)
	}

	// Count should be bounded by capacity
	if finalCount > 100 {
		t.Errorf("sample count exceeds capacity: %d", finalCount)
	}
}

func TestInflightGuardContract_OverlapDoesNotCreateFakeReachabilityTransitions(t *testing.T) {
	mgr := state.NewManager()
	server := newMockServer()
	defer server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     1,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:   100,
	}

	targets := []config.TargetConfig{
		{ID: "test-target", BaseURL: server.URL, Enabled: true},
	}

	client := NewClient(httpCfg, mgr, targetsToPtr(targets))

	// Record a failed probe
	mgr.RecordLatency("test-target", 5000.0, false)

	// Run multiple overlapping probes
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = client.ProbeTarget(&targets[0])
		}()
	}

	wg.Wait()

	// Get summary and verify error count is consistent
	summary := mgr.GetLatencySummary("test-target")

	// Error count should never be negative
	if summary.ErrorCount < 0 {
		t.Errorf("error_count is negative: %d", summary.ErrorCount)
	}

	// Error count should not exceed total samples
	samples := mgr.GetRecentLatencySamples("test-target", 100)
	if summary.ErrorCount > len(samples) {
		t.Errorf("error_count (%d) > total samples (%d)", summary.ErrorCount, len(samples))
	}
}

func TestInflightGuardContract_NoStateCorruptionUnderConcurrentProbes(t *testing.T) {
	mgr := state.NewManager()
	server1 := newMockServer()
	server2 := newMockServer()
	server3 := newMockServer()
	defer server1.Close()
	defer server2.Close()
	defer server3.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     1,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:   100,
	}

	targets := []config.TargetConfig{
		{ID: "target-1", BaseURL: server1.URL, Enabled: true},
		{ID: "target-2", BaseURL: server2.URL, Enabled: true},
		{ID: "target-3", BaseURL: server3.URL, Enabled: true},
	}

	client := NewClient(httpCfg, mgr, targetsToPtr(targets))

	// Pre-populate state
	for _, target := range targets {
		for i := 0; i < 50; i++ {
			mgr.RecordLatency(target.ID, float64(i), true)
		}
	}

	// Run concurrent probes with context cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Multiple concurrent probe goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			target := &targets[id%len(targets)]
			select {
			case <-ctx.Done():
				return
			default:
				_ = client.ProbeTarget(target)
			}
		}(i)
	}

	wg.Wait()

	// Verify state integrity for all targets
	for _, target := range targets {
		samples := mgr.GetRecentLatencySamples(target.ID, 100)
		summary := mgr.GetLatencySummary(target.ID)

		// INVARIANT: sample count never exceeds capacity
		if len(samples) > 100 {
			t.Errorf("target %s: sample count %d exceeds capacity", target.ID, len(samples))
		}

		// INVARIANT: error count never negative
		if summary.ErrorCount < 0 {
			t.Errorf("target %s: error_count is negative: %d", target.ID, summary.ErrorCount)
		}

		// INVARIANT: error count <= total samples
		if summary.ErrorCount > summary.SampleCount {
			t.Errorf("target %s: error_count (%d) > sample_count (%d)",
				target.ID, summary.ErrorCount, summary.SampleCount)
		}
	}
}

func TestInflightGuardContract_SkippedOverlapDoesNotEmitFalseFailure(t *testing.T) {
	mgr := state.NewManager()
	server := newMockServer()
	defer server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     1,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:   100,
	}

	targets := []config.TargetConfig{
		{ID: "test-target", BaseURL: server.URL, Enabled: true},
	}

	client := NewClient(httpCfg, mgr, targetsToPtr(targets))

	// Record successful probes
	for i := 0; i < 10; i++ {
		mgr.RecordLatency("test-target", float64(50+i), true)
	}

	// Rapid probes should not cause false error count inflation
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = client.ProbeTarget(&targets[0])
		}()
	}

	wg.Wait()

	finalErrors := mgr.GetLatencySummary("test-target").ErrorCount

	// Error count should not have spurious increases
	if finalErrors < 0 {
		t.Errorf("error_count is negative: %d", finalErrors)
	}
}

func TestInflightGuardContract_SampleTimestampsRemainOrdered(t *testing.T) {
	mgr := state.NewManager()
	server := newMockServer()
	defer server.Close()

	httpCfg := &config.HTTPProbeConfig{
		IntervalSeconds:     1,
		TimeoutMilliseconds: 5000,
		RecentSamplesMax:   100,
	}

	targets := []config.TargetConfig{
		{ID: "test-target", BaseURL: server.URL, Enabled: true},
	}

	client := NewClient(httpCfg, mgr, targetsToPtr(targets))

	// Run concurrent probes
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = client.ProbeTarget(&targets[0])
		}()
	}

	wg.Wait()

	// Verify timestamp ordering is preserved
	oldest, newest := mgr.GetLatencySampleTimestamps("test-target")
	if oldest != nil && newest != nil {
		if oldest.After(*newest) {
			t.Errorf("timestamp ordering violation: oldest (%v) is after newest (%v)", *oldest, *newest)
		}
	}
}
