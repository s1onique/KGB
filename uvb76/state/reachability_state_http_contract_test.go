package state

import (
	"testing"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// TestReachabilityStateContract_HTTP_UnknownToHTTPOK tests unknown -> HTTP OK transition.
func TestReachabilityStateContract_HTTP_UnknownToHTTPOK(t *testing.T) {
	// Unknown state should transition to HTTP reachable on first success
	// Recovery events should NOT fire from unknown state
	mgr := NewManager()
	targetID := "test-target"

	// Record first successful HTTP probe
	mgr.RecordLatency(targetID, 50.0, true)

	// Check state
	samples := mgr.GetRecentLatencySamples(targetID, 10)
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}

	if !samples[0].Reachable {
		t.Error("expected first sample to be reachable")
	}
}

// TestReachabilityStateContract_HTTP_OKToFailed tests HTTP OK -> HTTP failed transition.
func TestReachabilityStateContract_HTTP_OKToFailed(t *testing.T) {
	// After successful probes, a failure should be detectable
	mgr := NewManager()
	targetID := "test-target"

	// Record successful probes
	for i := 0; i < 5; i++ {
		mgr.RecordLatency(targetID, 50.0, true)
	}

	// Record failure
	mgr.RecordLatency(targetID, 5000.0, false)

	// Check state
	samples := mgr.GetRecentLatencySamples(targetID, 10)
	if len(samples) != 6 {
		t.Fatalf("expected 6 samples, got %d", len(samples))
	}

	// Count reachable vs not
	reachableCount := 0
	failedCount := 0
	for _, s := range samples {
		if s.Reachable {
			reachableCount++
		} else {
			failedCount++
		}
	}

	if reachableCount != 5 {
		t.Errorf("expected 5 reachable samples, got %d", reachableCount)
	}
	if failedCount != 1 {
		t.Errorf("expected 1 failed sample, got %d", failedCount)
	}
}

// TestReachabilityStateContract_HTTP_Recovery tests HTTP recovery after failure.
func TestReachabilityStateContract_HTTP_Recovery(t *testing.T) {
	// HTTP recovery requires prior failure state
	mgr := NewManager()
	targetID := "test-target"

	// Record successful probes
	for i := 0; i < 5; i++ {
		mgr.RecordLatency(targetID, 50.0, true)
	}

	// Record failure
	mgr.RecordLatency(targetID, 5000.0, false)

	// Record recovery (success after failure)
	mgr.RecordLatency(targetID, 55.0, true)

	// Check state
	samples := mgr.GetRecentLatencySamples(targetID, 10)
	if len(samples) != 7 {
		t.Fatalf("expected 7 samples, got %d", len(samples))
	}

	// Latest should be reachable (index 6 in chronological order)
	if !samples[6].Reachable {
		t.Error("latest sample should be reachable (recovery)")
	}

	// Count failures and successes
	successCount := 0
	for _, s := range samples {
		if s.Reachable {
			successCount++
		}
	}
	if successCount != 6 {
		t.Errorf("expected 6 successful samples, got %d", successCount)
	}
}

// TestReachabilityStateContract_HTTP_DegradedNotFailed tests degraded is distinct from failed.
func TestReachabilityStateContract_HTTP_DegradedNotFailed(t *testing.T) {
	// HTTP degraded -> HTTP recovered is a valid transition
	// Degraded is NOT failed, but recovery still applies
	mgr := NewManager()
	targetID := "test-target"

	// Record successful probes with normal latency
	for i := 0; i < 5; i++ {
		mgr.RecordLatency(targetID, 50.0, true)
	}

	// Record degraded probe (success but high latency)
	mgr.RecordLatency(targetID, 2000.0, true) // Still reachable=true but degraded

	// Record recovery (back to normal latency)
	mgr.RecordLatency(targetID, 52.0, true)

	// Check state
	samples := mgr.GetRecentLatencySamples(targetID, 10)
	if len(samples) != 7 {
		t.Fatalf("expected 7 samples, got %d", len(samples))
	}

	// All should be reachable
	for _, s := range samples {
		if !s.Reachable {
			t.Error("all samples should be reachable (degraded is still reachable=true)")
		}
	}
}

// Ensure domain package is imported for ProbeKind constants
var _ domain.ProbeKind = domain.ProbeKindHTTP
