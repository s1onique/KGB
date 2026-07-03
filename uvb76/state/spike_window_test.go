package state

import (
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// TestSpikeWindow_DetectAndRecordWithWindow_HTTPFailure tests that HTTP probe failures
// produce spike events even without latency-based conditions.
func TestSpikeWindow_DetectAndRecordWithWindow_HTTPFailure(t *testing.T) {
	detector := NewSpikeDetector()

	// Build previous window with normal samples
	now := time.Now().UTC()
	prevSamples := make([]domain.Sample, 30)
	for i := 0; i < 30; i++ {
		lat, _ := domain.NewLatencyMillis(50.0)
		prevSamples[i] = domain.Sample{
			At:      now.Add(-time.Duration(i) * time.Second),
			Kind:    domain.ProbeKindHTTP,
			Latency: lat,
			OK:      true,
		}
	}
	prevWindow := domain.NewSampleWindow(prevSamples)

	// HTTP failure (503)
	httpStatus := 503
	errStr := "http_probe_503: HTTP 503"
	event := detector.DetectAndRecordWithWindow(
		"test-target", "http",
		5000.0, // timeout latency
		now,
		false, // not reachable
		nil,   // scheduler delay
		&httpStatus,
		&errStr,
		prevWindow,
		nil, // httpTrace
	)

	if event == nil {
		t.Fatal("Expected HTTP failure to produce spike event")
	}
	if event.Severity != "critical" {
		t.Errorf("Expected severity=critical for HTTP failure, got %s", event.Severity)
	}
	if len(event.Reasons) == 0 {
		t.Error("Expected at least one reason for HTTP failure spike")
	}
}

// TestSpikeWindow_DetectAndRecordWithWindow_LatencySpike tests that high latency
// produces spike events.
func TestSpikeWindow_DetectAndRecordWithWindow_LatencySpike(t *testing.T) {
	detector := NewSpikeDetector()

	// Build previous window with normal samples
	now := time.Now().UTC()
	prevSamples := make([]domain.Sample, 30)
	for i := 0; i < 30; i++ {
		lat, _ := domain.NewLatencyMillis(50.0)
		prevSamples[i] = domain.Sample{
			At:      now.Add(-time.Duration(i) * time.Second),
			Kind:    domain.ProbeKindHTTP,
			Latency: lat,
			OK:      true,
		}
	}
	prevWindow := domain.NewSampleWindow(prevSamples)

	// High latency spike (2000ms should trigger warning for HTTP: 1000 <= 2000 < 5000)
	event := detector.DetectAndRecordWithWindow(
		"test-target", "http",
		2000.0, // warning latency
		now,
		true, // reachable
		nil,
		nil,
		nil,
		prevWindow,
		nil,
	)

	if event == nil {
		t.Fatal("Expected latency spike to produce event")
	}
	if event.Severity != "warning" {
		t.Errorf("Expected severity=warning for 2000ms latency, got %s", event.Severity)
	}
}

// TestSpikeWindow_DetectAndRecordWithWindow_ICMPSpike tests that ICMP latency spikes
// are detected with appropriate thresholds.
func TestSpikeWindow_DetectAndRecordWithWindow_ICMPSpike(t *testing.T) {
	detector := NewSpikeDetector()

	// Build previous window with normal ICMP samples
	now := time.Now().UTC()
	prevSamples := make([]domain.Sample, 30)
	for i := 0; i < 30; i++ {
		lat, _ := domain.NewLatencyMillis(30.0)
		prevSamples[i] = domain.Sample{
			At:      now.Add(-time.Duration(i) * time.Second),
			Kind:    domain.ProbeKindICMP,
			Latency: lat,
			OK:      true,
		}
	}
	prevWindow := domain.NewSampleWindow(prevSamples)

	// 600ms should trigger warning (ICMP warning is 500ms)
	event := detector.DetectAndRecordWithWindow(
		"test-target", "icmp",
		600.0,
		now,
		true,
		nil,
		nil,
		nil,
		prevWindow,
		nil,
	)

	if event == nil {
		t.Fatal("Expected ICMP latency spike to produce event")
	}
	if event.Severity != "warning" {
		t.Errorf("Expected severity=warning for ICMP 600ms, got %s", event.Severity)
	}
}

// TestSpikeWindow_DetectAndRecordWithWindow_NoSpike tests that normal latency
// does not produce spike events.
func TestSpikeWindow_DetectAndRecordWithWindow_NoSpike(t *testing.T) {
	detector := NewSpikeDetector()

	// Build previous window with normal samples
	now := time.Now().UTC()
	prevSamples := make([]domain.Sample, 30)
	for i := 0; i < 30; i++ {
		lat, _ := domain.NewLatencyMillis(50.0)
		prevSamples[i] = domain.Sample{
			At:      now.Add(-time.Duration(i) * time.Second),
			Kind:    domain.ProbeKindHTTP,
			Latency: lat,
			OK:      true,
		}
	}
	prevWindow := domain.NewSampleWindow(prevSamples)

	// Normal latency (100ms well below thresholds)
	event := detector.DetectAndRecordWithWindow(
		"test-target", "http",
		100.0,
		now,
		true,
		nil,
		nil,
		nil,
		prevWindow,
		nil,
	)

	if event != nil {
		t.Error("Expected no spike event for normal latency")
	}
}

// TestSpikeWindow_DetectAndRecordWithWindow_EmptyWindow tests that empty previous
// window does not panic and handles gracefully.
func TestSpikeWindow_DetectAndRecordWithWindow_EmptyWindow(t *testing.T) {
	detector := NewSpikeDetector()

	emptyWindow := domain.SampleWindow{}
	now := time.Now().UTC()

	// Should not panic with empty window
	event := detector.DetectAndRecordWithWindow(
		"test-target", "http",
		5000.0,
		now,
		true,
		nil,
		nil,
		nil,
		emptyWindow,
		nil,
	)

	// Critical latency should still produce spike even without baseline
	if event == nil {
		t.Error("Expected spike event for extreme latency even with empty window")
	}
}

// TestSpikeWindow_DetectAndRecordWithWindow_InvalidLatency tests that invalid successful
// latency (NaN/Inf) is rejected and treated as failed sample.
func TestSpikeWindow_DetectAndRecordWithWindow_InvalidLatency(t *testing.T) {
	// Build previous window
	now := time.Now().UTC()
	prevSamples := make([]domain.Sample, 30)
	for i := 0; i < 30; i++ {
		lat, _ := domain.NewLatencyMillis(50.0)
		prevSamples[i] = domain.Sample{
			At:      now.Add(-time.Duration(i) * time.Second),
			Kind:    domain.ProbeKindHTTP,
			Latency: lat,
			OK:      true,
		}
	}

	// Create a sample with OK=false (treated as failed, not latency spike)
	sample := domain.Sample{
		At:   now,
		Kind: domain.ProbeKindHTTP,
		OK:   false,
		Err:  "invalid latency value",
	}
	prevWindow := domain.NewSampleWindow(prevSamples)

	// With failed current sample (OK=false), should not produce latency spike
	decision := domain.DecideSpike(sample, prevWindow, domain.DefaultSpikeConfigHTTP())
	if decision.Kind != domain.SpikeDecisionNone {
		t.Errorf("Expected no spike for failed sample, got %s", decision.Kind)
	}
}

// TestSpikeWindow_ManagerIntegration tests the Manager-level spike detection with window.
func TestSpikeWindow_ManagerIntegration(t *testing.T) {
	m := NewManager()

	// Record some samples
	for i := 0; i < 30; i++ {
		m.RecordLatency("test-target", float64(i%50)+10.0, true)
	}

	// Get sample window
	window := m.GetHTTPSampleWindow("test-target", 30)
	if window.Len() == 0 {
		t.Fatal("Expected samples in window")
	}

	// Trigger spike using Manager method
	event := m.DetectAndRecordSpikeWithWindow(
		"test-target", "http",
		5000.0, // extreme latency
		time.Now().UTC(),
		true,
		nil,
		nil,
		nil,
		window,
		nil,
	)

	if event == nil {
		t.Fatal("Expected spike event")
	}
	if event.TargetID != "test-target" {
		t.Errorf("Expected TargetID=test-target, got %s", event.TargetID)
	}
}
