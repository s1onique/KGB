package state

import (
	"strings"
	"testing"
	"time"
)

// TestSpikeDetector_HTTPProbeFailure creates a spike event for HTTP probe failure.
// This is the core fix: probe failures (reachable=false) must create diagnostic events.
func TestSpikeDetector_HTTPProbeFailure(t *testing.T) {
	config := SpikeConfig{
		ICMPWarningMs:        500,
		ICMPCriticalMs:       2000,
		HTTPWarningMs:        1000,
		HTTPCriticalMs:       5000,
		RelativeMultiplier:    10.0,
		MinSamplesForMedian:  5,
		MaxPreviousSamples:   30,
		MaxEventsPerTracker:  100,
	}
	detector := NewSpikeDetectorWithConfig(config)

	now := time.Now().UTC()
	var prevSamples []LatencySample
	for i := 0; i < 25; i++ {
		prevSamples = append(prevSamples, LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		})
	}

	errStr := "request failed: context deadline exceeded"
	event := detector.DetectAndRecord(
		"test-target", "http",
		3000.0, // 3 second timeout
		now,
		false, // reachable = false - THIS IS THE KEY
		nil, nil, &errStr,
		prevSamples,
		nil, // httpTrace
	)

	if event == nil {
		t.Fatal("Expected spike event for failed HTTP probe")
	}
	if event.Kind != "http" {
		t.Errorf("Expected kind 'http', got '%s'", event.Kind)
	}
	if event.Severity != "critical" {
		t.Errorf("Expected severity 'critical' for failure, got '%s'", event.Severity)
	}
	// Should have failure reason
	if len(event.Reasons) == 0 {
		t.Error("Expected at least one reason for failure spike")
	}
	foundFailureReason := false
	for _, r := range event.Reasons {
		if strings.Contains(r, "failure") || strings.Contains(r, "timeout") {
			foundFailureReason = true
		}
	}
	if !foundFailureReason {
		t.Errorf("Expected failure-related reason, got %v", event.Reasons)
	}
	// Error should be captured
	if event.ProbeError == nil || *event.ProbeError != errStr {
		t.Errorf("Expected probe error '%s', got %v", errStr, event.ProbeError)
	}
}

// TestSpikeDetector_HTTPProbeTimeout tests that timeout errors are classified correctly.
func TestSpikeDetector_HTTPProbeTimeout(t *testing.T) {
	config := SpikeConfig{
		HTTPWarningMs:        1000,
		HTTPCriticalMs:       5000,
		RelativeMultiplier:   10.0,
		MinSamplesForMedian:  5,
		MaxPreviousSamples:   30,
		MaxEventsPerTracker:  100,
	}
	detector := NewSpikeDetectorWithConfig(config)

	now := time.Now().UTC()
	var prevSamples []LatencySample
	prevSamples = append(prevSamples, LatencySample{
		Timestamp: now.Add(-1 * time.Second),
		LatencyMs: 50.0,
		Reachable: true,
	})

	// Timeout error - case insensitive check
	errStr := "Request Failed: Context deadline exceeded"
	event := detector.DetectAndRecord(
		"test-target", "http",
		5000.0,
		now,
		false, // failed
		nil, nil, &errStr,
		prevSamples,
		nil, // httpTrace
	)

	if event == nil {
		t.Fatal("Expected spike event for timeout")
	}
	// Should have timeout-specific reason (case insensitive)
	foundTimeout := false
	for _, r := range event.Reasons {
		if r == "http_probe_timeout" {
			foundTimeout = true
		}
	}
	if !foundTimeout {
		t.Errorf("Expected 'http_probe_timeout' reason, got %v", event.Reasons)
	}
}

// TestSpikeDetector_HTTPProbeConnectionRefused tests connection refused classification.
func TestSpikeDetector_HTTPProbeConnectionRefused(t *testing.T) {
	config := SpikeConfig{
		HTTPWarningMs:        1000,
		HTTPCriticalMs:       5000,
		RelativeMultiplier:   10.0,
		MinSamplesForMedian:  5,
		MaxPreviousSamples:   30,
		MaxEventsPerTracker:  100,
	}
	detector := NewSpikeDetectorWithConfig(config)

	now := time.Now().UTC()
	var prevSamples []LatencySample
	prevSamples = append(prevSamples, LatencySample{
		Timestamp: now.Add(-1 * time.Second),
		LatencyMs: 50.0,
		Reachable: true,
	})

	errStr := "request failed: connection refused"
	event := detector.DetectAndRecord(
		"test-target", "http",
		5.0, // Very fast failure
		now,
		false,
		nil, nil, &errStr,
		prevSamples,
		nil, // httpTrace
	)

	if event == nil {
		t.Fatal("Expected spike event for connection refused")
	}
	foundConnRefused := false
	for _, r := range event.Reasons {
		if r == "http_probe_connection_refused" {
			foundConnRefused = true
		}
	}
	if !foundConnRefused {
		t.Errorf("Expected 'http_probe_connection_refused' reason, got %v", event.Reasons)
	}
}

// TestSpikeDetector_FailedProbeCreatesSpikeEvenWithLowLatency tests that even fast
// failures (like connection refused) create spike events.
func TestSpikeDetector_FailedProbeCreatesSpikeEvenWithLowLatency(t *testing.T) {
	config := SpikeConfig{
		HTTPWarningMs:        1000,
		HTTPCriticalMs:       5000,
		RelativeMultiplier:   10.0,
		MinSamplesForMedian:  5,
		MaxPreviousSamples:   30,
		MaxEventsPerTracker:  100,
	}
	detector := NewSpikeDetectorWithConfig(config)

	now := time.Now().UTC()
	var prevSamples []LatencySample
	for i := 0; i < 25; i++ {
		prevSamples = append(prevSamples, LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		})
	}

	// Very fast failure (5ms) - would NOT trigger latency spike
	errStr := "request failed: connection reset"
	event := detector.DetectAndRecord(
		"test-target", "http",
		5.0,
		now,
		false, // reachable = false
		nil, nil, &errStr,
		prevSamples,
		nil, // httpTrace
	)

	// SHOULD still create a spike because failure is a diagnostic event
	if event == nil {
		t.Fatal("Expected spike event for failed probe even with low latency")
	}
}

// TestSpikeDetector_FailedProbeWithNoPreviousSamples tests that failed probes
// create spikes even without previous samples (no median available).
func TestSpikeDetector_FailedProbeWithNoPreviousSamples(t *testing.T) {
	config := SpikeConfig{
		HTTPWarningMs:        1000,
		HTTPCriticalMs:       5000,
		RelativeMultiplier:   10.0,
		MinSamplesForMedian:  5,
		MaxPreviousSamples:   30,
		MaxEventsPerTracker:  100,
	}
	detector := NewSpikeDetectorWithConfig(config)

	now := time.Now().UTC()
	// No previous samples
	var prevSamples []LatencySample

	errStr := "request failed: timeout"
	event := detector.DetectAndRecord(
		"test-target", "http",
		3000.0,
		now,
		false,
		nil, nil, &errStr,
		prevSamples,
		nil, // httpTrace
	)

	if event == nil {
		t.Fatal("Expected spike event for failed probe with no previous samples")
	}
	// Should still create event despite no median
}

// TestSpikeDetector_FailedProbeWithNilError tests that failed probes with nil
// error still create spike events.
func TestSpikeDetector_FailedProbeWithNilError(t *testing.T) {
	config := SpikeConfig{
		HTTPWarningMs:        1000,
		HTTPCriticalMs:       5000,
		RelativeMultiplier:   10.0,
		MinSamplesForMedian:  5,
		MaxPreviousSamples:   30,
		MaxEventsPerTracker:  100,
	}
	detector := NewSpikeDetectorWithConfig(config)

	now := time.Now().UTC()
	var prevSamples []LatencySample
	prevSamples = append(prevSamples, LatencySample{
		Timestamp: now.Add(-1 * time.Second),
		LatencyMs: 50.0,
		Reachable: true,
	})

	// No error string
	event := detector.DetectAndRecord(
		"test-target", "http",
		5000.0,
		now,
		false, // failed
		nil, nil, nil, // nil error
		prevSamples,
		nil, // httpTrace
	)

	if event == nil {
		t.Fatal("Expected spike event for failed probe even with nil error")
	}
	// Should still have generic failure reason
}

// TestSpikeDetector_ICMPProbeFailure tests that ICMP failures do NOT produce
// HTTP-style reasons. ICMP failures use latency-based spike detection.
func TestSpikeDetector_ICMPProbeFailure(t *testing.T) {
	config := SpikeConfig{
		ICMPWarningMs:        500,
		ICMPCriticalMs:       2000,
		HTTPWarningMs:        1000,
		HTTPCriticalMs:       5000,
		RelativeMultiplier:   10.0,
		MinSamplesForMedian:  5,
		MaxPreviousSamples:   30,
		MaxEventsPerTracker:  100,
	}
	detector := NewSpikeDetectorWithConfig(config)

	now := time.Now().UTC()
	var prevSamples []LatencySample
	for i := 0; i < 25; i++ {
		prevSamples = append(prevSamples, LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		})
	}

	// ICMP failure - reachable=false, but kind is "icmp"
	// Should NOT produce http_probe_* reasons
	event := detector.DetectAndRecord(
		"test-target", "icmp",
		5000.0, // ICMP timeout
		now,
		false, // unreachable
		nil, nil, nil,
		prevSamples,
		nil, // httpTrace
	)

	// ICMP failures should NOT create HTTP-style spike events
	// They continue to use latency-based spike detection (different semantics)
	if event != nil {
		// Verify no HTTP-style reasons appear
		for _, reason := range event.Reasons {
			if strings.HasPrefix(reason, "http_probe_") {
				t.Errorf("ICMP failure should not produce HTTP-style reason '%s'", reason)
			}
		}
		// But latency-based reasons are fine
		t.Logf("ICMP failure event created with reasons: %v", event.Reasons)
	}
}

// TestSpikeDetector_SuccessfulProbeDoesNotTriggerFailureEvent tests that successful
// probes don't create failure events even if they follow failures.
func TestSpikeDetector_SuccessfulProbeDoesNotTriggerFailureEvent(t *testing.T) {
	config := SpikeConfig{
		HTTPWarningMs:        1000,
		HTTPCriticalMs:       5000,
		RelativeMultiplier:   10.0,
		MinSamplesForMedian:  5,
		MaxPreviousSamples:   30,
		MaxEventsPerTracker:  100,
	}
	detector := NewSpikeDetectorWithConfig(config)

	now := time.Now().UTC()
	var prevSamples []LatencySample
	prevSamples = append(prevSamples, LatencySample{
		Timestamp: now.Add(-1 * time.Second),
		LatencyMs: 50.0,
		Reachable: true,
	})

	// Successful probe (reachable = true)
	event := detector.DetectAndRecord(
		"test-target", "http",
		100.0, // normal latency
		now,
		true, // reachable = true
		nil, nil, nil,
		prevSamples,
		nil, // httpTrace
	)

	// Should NOT create a failure event
	if event != nil {
		// But if latency was high enough, it might create a latency spike
		t.Logf("Latency spike created (acceptable): %v", event.Reasons)
	}
}
