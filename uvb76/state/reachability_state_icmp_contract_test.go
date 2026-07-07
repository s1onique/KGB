package state

import (
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// TestReachabilityStateContract_ICMP_UnknownToOK tests unknown -> ICMP OK transition.
func TestReachabilityStateContract_ICMP_UnknownToOK(t *testing.T) {
	// Unknown state should transition to ICMP reachable on first success
	mgr := NewManager()
	targetID := "test-target"

	// Record first successful ICMP probe
	mgr.RecordICMPLatency(targetID, 30.0, true)

	// Check state
	samples := mgr.GetRecentICMPLatencySamples(targetID, 10)
	if len(samples) != 1 {
		t.Fatalf("expected 1 ICMP sample, got %d", len(samples))
	}

	if !samples[0].Reachable {
		t.Error("expected first ICMP sample to be reachable")
	}
}

// TestReachabilityStateContract_ICMP_Recovery tests ICMP recovery after failure.
func TestReachabilityStateContract_ICMP_Recovery(t *testing.T) {
	// ICMP recovery requires prior failure state
	mgr := NewManager()
	targetID := "test-target"

	// Record successful probes
	for i := 0; i < 5; i++ {
		mgr.RecordICMPLatency(targetID, 30.0, true)
	}

	// Record failure
	mgr.RecordICMPLatency(targetID, 3000.0, false)

	// Record recovery
	mgr.RecordICMPLatency(targetID, 32.0, true)

	// Check state
	samples := mgr.GetRecentICMPLatencySamples(targetID, 10)
	if len(samples) != 7 {
		t.Fatalf("expected 7 samples, got %d", len(samples))
	}

	// Latest should be reachable (index 6 in chronological order)
	if !samples[6].Reachable {
		t.Error("latest ICMP sample should be reachable (recovery)")
	}
}

// TestReachabilityStateContract_ICMP_DeterministicTimestamps tests deterministic timestamps.
func TestReachabilityStateContract_ICMP_DeterministicTimestamps(t *testing.T) {
	// RecordICMPLatencyAt should produce deterministic results
	mgr := NewManager()
	targetID := "test-target"

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Record sequence with specific timestamps
	for i := 0; i < 5; i++ {
		ts := baseTime.Add(time.Duration(i) * time.Minute)
		mgr.RecordICMPLatencyAt(targetID, 30.0+float64(i), true, ts)
	}

	samples := mgr.GetRecentICMPLatencySamples(targetID, 10)

	// Should have 5 samples
	if len(samples) != 5 {
		t.Fatalf("expected 5 samples, got %d", len(samples))
	}

	// Verify order: chronological (oldest first)
	if !samples[0].Timestamp.Equal(baseTime) {
		t.Errorf("expected oldest sample at 12:00, got %v", samples[0].Timestamp)
	}
	if !samples[4].Timestamp.Equal(baseTime.Add(4 * time.Minute)) {
		t.Errorf("expected newest sample at 12:04, got %v", samples[4].Timestamp)
	}
}

// Ensure domain package is imported for ProbeKind constants
var _ domain.ProbeKind = domain.ProbeKindICMP
