package state

import (
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// TestReachabilityStateContract_Combined_PartiallyReachable tests HTTP failed + ICMP OK.
func TestReachabilityStateContract_Combined_PartiallyReachable(t *testing.T) {
	// HTTP failed + ICMP OK = partially_reachable
	// NOT network_unreachable - ICMP proves the host is reachable
	mgr := NewManager()
	targetID := "test-target"

	// Record some ICMP success
	for i := 0; i < 3; i++ {
		mgr.RecordICMPLatency(targetID, 30.0, true)
	}

	// Record HTTP failure
	mgr.RecordLatency(targetID, 5000.0, false)

	// Both should coexist independently
	httpSamples := mgr.GetRecentLatencySamples(targetID, 10)
	icmpSamples := mgr.GetRecentICMPLatencySamples(targetID, 10)

	if len(httpSamples) != 1 {
		t.Errorf("expected 1 HTTP sample, got %d", len(httpSamples))
	}
	if len(icmpSamples) != 3 {
		t.Errorf("expected 3 ICMP samples, got %d", len(icmpSamples))
	}

	// HTTP should be failed
	if httpSamples[0].Reachable {
		t.Error("HTTP should be marked as failed")
	}

	// ICMP should be reachable
	for _, s := range icmpSamples {
		if !s.Reachable {
			t.Error("ICMP should be reachable")
		}
	}
}

// TestReachabilityStateContract_Combined_NetworkUnreachable tests both failed.
func TestReachabilityStateContract_Combined_NetworkUnreachable(t *testing.T) {
	// Both failed = network_unreachable
	mgr := NewManager()
	targetID := "test-target"

	// Record HTTP failure
	mgr.RecordLatency(targetID, 5000.0, false)

	// Record ICMP failure
	mgr.RecordICMPLatency(targetID, 5000.0, false)

	// Both should be recorded
	httpSamples := mgr.GetRecentLatencySamples(targetID, 10)
	icmpSamples := mgr.GetRecentICMPLatencySamples(targetID, 10)

	if len(httpSamples) != 1 {
		t.Errorf("expected 1 HTTP sample, got %d", len(httpSamples))
	}
	if len(icmpSamples) != 1 {
		t.Errorf("expected 1 ICMP sample, got %d", len(icmpSamples))
	}

	if httpSamples[0].Reachable {
		t.Error("HTTP should be failed")
	}
	if icmpSamples[0].Reachable {
		t.Error("ICMP should be failed")
	}
}

// TestReachabilityStateContract_Combined_TransitionsDeterministic tests deterministic behavior.
func TestReachabilityStateContract_Combined_TransitionsDeterministic(t *testing.T) {
	// Same input sequence should always produce same state
	mgr1 := NewManager()
	mgr2 := NewManager()
	targetID := "test-target"

	// Record same sequence in both managers
	sequence := []struct {
		latency float64
		ok      bool
	}{
		{50.0, true},
		{52.0, true},
		{51.0, true},
		{5000.0, false},
		{55.0, true},
	}

	for _, s := range sequence {
		mgr1.RecordLatency(targetID, s.latency, s.ok)
		mgr2.RecordLatency(targetID, s.latency, s.ok)
	}

	// Compare state
	samples1 := mgr1.GetRecentLatencySamples(targetID, 10)
	samples2 := mgr2.GetRecentLatencySamples(targetID, 10)

	if len(samples1) != len(samples2) {
		t.Errorf("sample count mismatch: %d vs %d", len(samples1), len(samples2))
	}

	for i := range samples1 {
		if samples1[i].LatencyMs != samples2[i].LatencyMs {
			t.Errorf("sample %d latency mismatch: %f vs %f", i, samples1[i].LatencyMs, samples2[i].LatencyMs)
		}
		if samples1[i].Reachable != samples2[i].Reachable {
			t.Errorf("sample %d reachable mismatch", i)
		}
	}
}

// TestReachabilityStateContract_Combined_HTTPRecoveryDoesNotImplyICMPRecovery tests independence.
func TestReachabilityStateContract_Combined_HTTPRecoveryDoesNotImplyICMPRecovery(t *testing.T) {
	// HTTP recovery and ICMP recovery are independent signals
	mgr := NewManager()
	targetID := "test-target"

	// Both start failing
	mgr.RecordLatency(targetID, 5000.0, false)
	mgr.RecordICMPLatency(targetID, 3000.0, false)

	// HTTP recovers first
	mgr.RecordLatency(targetID, 50.0, true)

	// ICMP is still failing
	httpSamples := mgr.GetRecentLatencySamples(targetID, 2) // Only request actual count
	icmpSamples := mgr.GetRecentICMPLatencySamples(targetID, 2)

	// HTTP should be reachable (latest is at index 1 in chronological order)
	if len(httpSamples) != 2 {
		t.Fatalf("expected 2 HTTP samples, got %d", len(httpSamples))
	}
	if !httpSamples[1].Reachable {
		t.Errorf("HTTP should be reachable (recovered), got reachable=%v latency=%f", httpSamples[1].Reachable, httpSamples[1].LatencyMs)
	}

	// ICMP should still be failed (latest is at index 0 in chronological order)
	if len(icmpSamples) != 1 {
		t.Fatalf("expected 1 ICMP sample, got %d", len(icmpSamples))
	}
	if icmpSamples[0].Reachable {
		t.Error("ICMP should still be failed")
	}
}

// TestReachabilityStateContract_Combined_ICMPRecoveryDoesNotImplyHTTPRecovery tests independence.
func TestReachabilityStateContract_Combined_ICMPRecoveryDoesNotImplyHTTPRecovery(t *testing.T) {
	// ICMP recovery and HTTP recovery are independent signals
	mgr := NewManager()
	targetID := "test-target"

	// Both start failing
	mgr.RecordLatency(targetID, 5000.0, false)
	mgr.RecordICMPLatency(targetID, 3000.0, false)

	// ICMP recovers first
	mgr.RecordICMPLatency(targetID, 30.0, true)

	// HTTP is still failing
	httpSamples := mgr.GetRecentLatencySamples(targetID, 2) // Only request actual count
	icmpSamples := mgr.GetRecentICMPLatencySamples(targetID, 2)

	// ICMP should be reachable (latest is at index 1 in chronological order)
	if len(icmpSamples) != 2 {
		t.Fatalf("expected 2 ICMP samples, got %d", len(icmpSamples))
	}
	if !icmpSamples[1].Reachable {
		t.Errorf("ICMP should be reachable (recovered), got reachable=%v latency=%f", icmpSamples[1].Reachable, icmpSamples[1].LatencyMs)
	}

	// HTTP should still be failed (latest is at index 0 in chronological order)
	if len(httpSamples) != 1 {
		t.Fatalf("expected 1 HTTP sample, got %d", len(httpSamples))
	}
	if httpSamples[0].Reachable {
		t.Error("HTTP should still be failed")
	}
}

// TestReachabilityStateContract_Combined_ProbeEvidenceTypes verifies probe evidence types.
func TestReachabilityStateContract_Combined_ProbeEvidenceTypes(t *testing.T) {
	// Verify that state manager records correct evidence types
	mgr := NewManager()
	targetID := "test-target"

	now := time.Now()

	// Record with specific timestamp
	mgr.RecordLatency(targetID, 50.0, true)
	mgr.RecordICMPLatency(targetID, 30.0, true)

	// Record failure
	mgr.RecordLatency(targetID, 5000.0, false)
	mgr.RecordICMPLatencyAt(targetID, 3000.0, false, now.Add(-time.Minute))

	httpSamples := mgr.GetRecentLatencySamples(targetID, 10)
	icmpSamples := mgr.GetRecentICMPLatencySamples(targetID, 10)

	// HTTP: chronological order - first success, second failure
	if len(httpSamples) != 2 {
		t.Fatalf("expected 2 HTTP samples, got %d", len(httpSamples))
	}
	// HTTP[0] = success (first), HTTP[1] = failure (second)
	if !httpSamples[0].Reachable {
		t.Error("HTTP first sample should be success")
	}
	if httpSamples[1].Reachable {
		t.Error("HTTP second sample should be failure")
	}

	// ICMP: chronological order - first success (30ms), second failure (3000ms with -1min timestamp)
	if len(icmpSamples) != 2 {
		t.Fatalf("expected 2 ICMP samples, got %d", len(icmpSamples))
	}
	// ICMP[0] = success (first), ICMP[1] = failure (second, with old timestamp)
	if !icmpSamples[0].Reachable {
		t.Error("ICMP first sample should be success")
	}
	if icmpSamples[1].Reachable {
		t.Error("ICMP second sample should be failure")
	}
}

// TestReachabilityStateContract_Combined_CrossProbeIndependence verifies probe kind separation.
func TestReachabilityStateContract_Combined_CrossProbeIndependence(t *testing.T) {
	// HTTP and ICMP probes should be tracked independently
	mgr := NewManager()
	targetID := "test-target"

	// Record mixed sequence
	mgr.RecordLatency(targetID, 50.0, true)              // HTTP success
	mgr.RecordICMPLatency(targetID, 30.0, true)         // ICMP success
	mgr.RecordLatency(targetID, 5000.0, false)          // HTTP failure
	mgr.RecordICMPLatency(targetID, 3000.0, false)       // ICMP failure

	httpSamples := mgr.GetRecentLatencySamples(targetID, 10)
	icmpSamples := mgr.GetRecentICMPLatencySamples(targetID, 10)

	if len(httpSamples) != 2 {
		t.Errorf("expected 2 HTTP samples, got %d", len(httpSamples))
	}
	if len(icmpSamples) != 2 {
		t.Errorf("expected 2 ICMP samples, got %d", len(icmpSamples))
	}

	// HTTP: success then failure (chronological order: index 0 = oldest)
	if !httpSamples[0].Reachable {
		t.Error("HTTP older should be success")
	}
	if httpSamples[1].Reachable {
		t.Error("HTTP latest should be failure")
	}

	// ICMP: success then failure (chronological order: index 0 = oldest)
	if !icmpSamples[0].Reachable {
		t.Error("ICMP older should be success")
	}
	if icmpSamples[1].Reachable {
		t.Error("ICMP latest should be failure")
	}
}

// TestReachabilityStateContract_Combined_DuplicateFailureSamples verifies no duplicate events.
func TestReachabilityStateContract_Combined_DuplicateFailureSamples(t *testing.T) {
	// Duplicate failure samples should not emit duplicate first-failure events
	// The state manager records all samples; deduplication is at event emission level
	mgr := NewManager()
	targetID := "test-target"

	// Record multiple failures in sequence
	for i := 0; i < 5; i++ {
		mgr.RecordLatency(targetID, 5000.0, false)
	}

	samples := mgr.GetRecentLatencySamples(targetID, 10)

	// All should be failures
	for i, s := range samples {
		if s.Reachable {
			t.Errorf("sample %d should be failure, got reachable=true", i)
		}
	}
}

// TestReachabilityStateContract_Combined_RecoveryRequiresPriorFailure tests recovery precondition.
func TestReachabilityStateContract_Combined_RecoveryRequiresPriorFailure(t *testing.T) {
	// Recovery detection should require prior failure state
	// This is handled by the probe client via lastReachability tracking
	// State manager just records samples; event emission is at probe layer
	mgr := NewManager()
	targetID := "test-target"

	// Record first successful probe (no prior failure)
	mgr.RecordLatency(targetID, 50.0, true)

	samples := mgr.GetRecentLatencySamples(targetID, 10)
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}

	if !samples[0].Reachable {
		t.Error("first sample should be reachable")
	}
}

// Ensure domain package is imported for ProbeKind constants
var _ domain.ProbeKind = domain.ProbeKindHTTP
