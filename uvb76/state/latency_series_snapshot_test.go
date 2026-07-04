package state

import (
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

func TestLatencySeriesSnapshot_EmptyTracker(t *testing.T) {
	// Create a fresh Manager with no targets
	m := NewManager()

	// Get snapshot for nonexistent target
	snap := m.GetHTTPSeriesSnapshot("nonexistent", 100)

	if snap.Window.Len() != 0 {
		t.Errorf("Expected empty window for nonexistent target, got len=%d", snap.Window.Len())
	}
	if snap.RetainedSampleCount != 0 {
		t.Errorf("Expected RetainedSampleCount=0, got %d", snap.RetainedSampleCount)
	}
	if snap.ErrorCount != 0 {
		t.Errorf("Expected ErrorCount=0, got %d", snap.ErrorCount)
	}
	if snap.ProbeKind != domain.ProbeKindHTTP {
		t.Errorf("Expected ProbeKind=HTTP, got %s", snap.ProbeKind)
	}
}

func TestLatencySeriesSnapshot_AllSuccessful(t *testing.T) {
	m := NewManager()

	targetID := "test-http-all-success"
	// Record successful samples
	for i := 0; i < 10; i++ {
		m.RecordLatency(targetID, 50.0+float64(i), true)
	}

	snap := m.GetHTTPSeriesSnapshot(targetID, 100)

	// Window should have all 10 successful samples
	if snap.Window.Len() != 10 {
		t.Errorf("Expected Window.Len()=10, got %d", snap.Window.Len())
	}
	if snap.RetainedSampleCount != 10 {
		t.Errorf("Expected RetainedSampleCount=10, got %d", snap.RetainedSampleCount)
	}
	if snap.ErrorCount != 0 {
		t.Errorf("Expected ErrorCount=0 for all successful samples, got %d", snap.ErrorCount)
	}
	if snap.OldestSampleTs.IsZero() {
		t.Error("Expected non-zero OldestSampleTs")
	}
	if snap.NewestSampleTs.IsZero() {
		t.Error("Expected non-zero NewestSampleTs")
	}
	if snap.OldestSampleTs.After(snap.NewestSampleTs) {
		t.Error("OldestSampleTs should be before NewestSampleTs")
	}
}

func TestLatencySeriesSnapshot_MixedSuccessAndFailed(t *testing.T) {
	m := NewManager()

	targetID := "test-http-mixed"
	// Record mixed samples: 5 successful, 3 failed
	for i := 0; i < 8; i++ {
		if i%3 == 0 {
			m.RecordLatency(targetID, 0, false)
		} else {
			m.RecordLatency(targetID, 50.0+float64(i)*10, true)
		}
	}

	snap := m.GetHTTPSeriesSnapshot(targetID, 100)

	// Window should have only successful samples (5)
	if snap.Window.Len() != 5 {
		t.Errorf("Expected Window.Len()=5 successful samples, got %d", snap.Window.Len())
	}
	// ErrorCount should be 3
	if snap.ErrorCount != 3 {
		t.Errorf("Expected ErrorCount=3, got %d", snap.ErrorCount)
	}
	if snap.RetainedSampleCount != 8 {
		t.Errorf("Expected RetainedSampleCount=8, got %d", snap.RetainedSampleCount)
	}
}

func TestLatencySeriesSnapshot_ICMPSnapshot(t *testing.T) {
	m := NewManager()

	targetID := "test-icmp"
	// Record ICMP samples
	for i := 0; i < 5; i++ {
		m.RecordICMPLatency(targetID, 10.0+float64(i), true)
	}

	snap := m.GetICMPSeriesSnapshot(targetID, 100)

	if snap.Window.Len() != 5 {
		t.Errorf("Expected Window.Len()=5, got %d", snap.Window.Len())
	}
	if snap.ProbeKind != domain.ProbeKindICMP {
		t.Errorf("Expected ProbeKind=ICMP, got %s", snap.ProbeKind)
	}
	if snap.ErrorCount != 0 {
		t.Errorf("Expected ErrorCount=0, got %d", snap.ErrorCount)
	}
}

func TestLatencySeriesSnapshot_TimestampsPreserved(t *testing.T) {
	m := NewManager()

	targetID := "test-timestamps"
	before := time.Now().UTC()

	// Record samples with small delay
	for i := 0; i < 3; i++ {
		time.Sleep(1 * time.Millisecond)
		m.RecordLatency(targetID, 50.0, true)
	}
	after := time.Now().UTC()

	snap := m.GetHTTPSeriesSnapshot(targetID, 100)

	if snap.OldestSampleTs.Before(before) {
		t.Error("OldestSampleTs should be after or equal to 'before' time")
	}
	if snap.NewestSampleTs.After(after) {
		t.Error("NewestSampleTs should be before or equal to 'after' time")
	}
}

func TestLatencySeriesSnapshot_LimitLargerThanCapacity(t *testing.T) {
	m := NewManager()

	targetID := "test-limit"
	// Record 5 samples
	for i := 0; i < 5; i++ {
		m.RecordLatency(targetID, float64(i+1)*10, true)
	}

	// Request more samples than available
	snap := m.GetHTTPSeriesSnapshot(targetID, 10000)

	// Should return all available samples
	if snap.RetainedSampleCount != 5 {
		t.Errorf("Expected RetainedSampleCount=5, got %d", snap.RetainedSampleCount)
	}
	if snap.Window.Len() != 5 {
		t.Errorf("Expected Window.Len()=5, got %d", snap.Window.Len())
	}
}

func TestLatencySeriesSnapshot_DispatchByKind(t *testing.T) {
	m := NewManager()

	httpTarget := "test-http"
	icmpTarget := "test-icmp"

	// Record HTTP samples
	for i := 0; i < 3; i++ {
		m.RecordLatency(httpTarget, 50.0, true)
	}

	// Record ICMP samples
	for i := 0; i < 4; i++ {
		m.RecordICMPLatency(icmpTarget, 10.0, true)
	}

	// Test HTTP dispatch
	httpSnap := m.GetSeriesSnapshot(httpTarget, domain.ProbeKindHTTP, 100)
	if httpSnap.Window.Len() != 3 {
		t.Errorf("Expected HTTP Window.Len()=3, got %d", httpSnap.Window.Len())
	}
	if httpSnap.ProbeKind != domain.ProbeKindHTTP {
		t.Errorf("Expected ProbeKind=HTTP, got %s", httpSnap.ProbeKind)
	}

	// Test ICMP dispatch
	icmpSnap := m.GetSeriesSnapshot(icmpTarget, domain.ProbeKindICMP, 100)
	if icmpSnap.Window.Len() != 4 {
		t.Errorf("Expected ICMP Window.Len()=4, got %d", icmpSnap.Window.Len())
	}
	if icmpSnap.ProbeKind != domain.ProbeKindICMP {
		t.Errorf("Expected ProbeKind=ICMP, got %s", icmpSnap.ProbeKind)
	}

	// Test unknown probe kind returns empty
	unknownSnap := m.GetSeriesSnapshot(httpTarget, domain.ProbeKind("unknown"), 100)
	if unknownSnap.Window.Len() != 0 {
		t.Errorf("Expected unknown probe kind Window.Len()=0, got %d", unknownSnap.Window.Len())
	}
}

func TestLatencySeriesSnapshot_RetainedSampleCapacity(t *testing.T) {
	m := NewManager()

	targetID := "test-capacity"
	// Record fewer samples than capacity
	for i := 0; i < 50; i++ {
		m.RecordLatency(targetID, 50.0, true)
	}

	snap := m.GetHTTPSeriesSnapshot(targetID, 100)

	// Count should be what we recorded
	if snap.RetainedSampleCount != 50 {
		t.Errorf("Expected RetainedSampleCount=50, got %d", snap.RetainedSampleCount)
	}
}

func TestLatencySeriesSnapshot_PercentileFromWindow(t *testing.T) {
	m := NewManager()

	targetID := "test-percentile"
	// Record samples with known values: [10, 20, 30, 40, 50]
	for i := 0; i < 5; i++ {
		m.RecordLatency(targetID, float64(i+1)*10, true)
	}

	snap := m.GetHTTPSeriesSnapshot(targetID, 100)

	// Test median/p50
	p50, ok := snap.Window.P50()
	if !ok {
		t.Error("Expected P50 to succeed")
	}
	expectedP50 := 30.0 // median of [10,20,30,40,50]
	if p50.Float64() != expectedP50 {
		t.Errorf("Expected P50=%f, got %f", expectedP50, p50.Float64())
	}

	// Test P90
	p90, ok := snap.Window.P90()
	if !ok {
		t.Error("Expected P90 to succeed")
	}
	// P90 of 5 samples should be around 46 (interpolated)
	if p90.Float64() < 40 || p90.Float64() > 50 {
		t.Errorf("Expected P90 between 40 and 50, got %f", p90.Float64())
	}
}

func TestLatencySeriesSnapshot_SampleMetasEnableExactErrorCount(t *testing.T) {
	m := NewManager()

	targetID := "test-exact-error-count"

	// Record samples with known failure pattern
	// Pattern: 9 successes, 6 failures = 15 total samples
	// This tests that the metadata enables exact per-window error counting

	// Window 1: 3 successes, 2 failures
	m.RecordLatency(targetID, 50.0, true)
	m.RecordLatency(targetID, 0, false)
	m.RecordLatency(targetID, 60.0, true)
	m.RecordLatency(targetID, 0, false)
	m.RecordLatency(targetID, 70.0, true)

	// Window 2: 5 successes, 0 failures
	m.RecordLatency(targetID, 40.0, true)
	m.RecordLatency(targetID, 45.0, true)
	m.RecordLatency(targetID, 55.0, true)
	m.RecordLatency(targetID, 65.0, true)
	m.RecordLatency(targetID, 75.0, true)

	// Window 3: 1 success, 4 failures
	m.RecordLatency(targetID, 0, false)
	m.RecordLatency(targetID, 0, false)
	m.RecordLatency(targetID, 80.0, true)
	m.RecordLatency(targetID, 0, false)
	m.RecordLatency(targetID, 0, false)

	snap := m.GetHTTPSeriesSnapshot(targetID, 100)

	// Verify sample metadata is populated
	if len(snap.Samples) == 0 {
		t.Fatal("Expected non-empty Samples metadata")
	}

	// Count errors from metadata
	errorCountFromMeta := 0
	for _, meta := range snap.Samples {
		if !meta.OK {
			errorCountFromMeta++
		}
	}

	// Verify total error count matches (6 failures total)
	if errorCountFromMeta != 6 {
		t.Errorf("Expected 6 total errors from metadata, got %d", errorCountFromMeta)
	}

	// Verify snapshot ErrorCount matches
	if snap.ErrorCount != 6 {
		t.Errorf("Expected ErrorCount=6, got %d", snap.ErrorCount)
	}

	// Verify metadata timestamps are non-zero
	for _, meta := range snap.Samples {
		if meta.At.IsZero() {
			t.Error("Expected non-zero timestamp in sample metadata")
		}
	}

	// Verify successful sample count in window (9 successes)
	successCount := 0
	for _, meta := range snap.Samples {
		if meta.OK {
			successCount++
		}
	}
	if successCount != 9 {
		t.Errorf("Expected 9 successful samples, got %d", successCount)
	}
}
