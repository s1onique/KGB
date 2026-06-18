package state

import (
	"testing"
	"time"
)

// TestSpikeDetector_DefaultConfig tests that default configuration is sensible.
func TestSpikeDetector_DefaultConfig(t *testing.T) {
	config := DefaultSpikeConfig()

	if config.ICMPWarningMs != 500 {
		t.Errorf("Expected ICMP warning threshold 500ms, got %f", config.ICMPWarningMs)
	}
	if config.ICMPCriticalMs != 2000 {
		t.Errorf("Expected ICMP critical threshold 2000ms, got %f", config.ICMPCriticalMs)
	}
	if config.HTTPWarningMs != 1000 {
		t.Errorf("Expected HTTP warning threshold 1000ms, got %f", config.HTTPWarningMs)
	}
	if config.HTTPCriticalMs != 5000 {
		t.Errorf("Expected HTTP critical threshold 5000ms, got %f", config.HTTPCriticalMs)
	}
	if config.RelativeMultiplier != 10.0 {
		t.Errorf("Expected relative multiplier 10, got %f", config.RelativeMultiplier)
	}
	if config.MinSamplesForMedian != 20 {
		t.Errorf("Expected MinSamplesForMedian 20, got %d", config.MinSamplesForMedian)
	}
	if config.MaxPreviousSamples != 30 {
		t.Errorf("Expected MaxPreviousSamples 30, got %d", config.MaxPreviousSamples)
	}
	if config.MaxEventsPerTracker != 100 {
		t.Errorf("Expected MaxEventsPerTracker 100, got %d", config.MaxEventsPerTracker)
	}
}

// TestSpikeDetector_ICMPWarningThreshold tests ICMP warning spike detection.
func TestSpikeDetector_ICMPWarningThreshold(t *testing.T) {
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

	event := detector.DetectAndRecord(
		"test-target", "icmp",
		600.0,
		now,
		true,
		nil, nil, nil,
		prevSamples,
	)

	if event == nil {
		t.Fatal("Expected spike event to be detected")
	}
	if event.Severity != "warning" {
		t.Errorf("Expected severity 'warning', got '%s'", event.Severity)
	}
	if len(event.Reasons) < 1 {
		t.Errorf("Expected at least one reason, got %v", event.Reasons)
	}
	if event.Reasons[0] != "icmp_warning_absolute_threshold" {
		t.Errorf("Expected first reason 'icmp_warning_absolute_threshold', got %v", event.Reasons)
	}
}

// TestSpikeDetector_ICMPCriticalThreshold tests ICMP critical spike detection.
func TestSpikeDetector_ICMPCriticalThreshold(t *testing.T) {
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

	event := detector.DetectAndRecord(
		"test-target", "icmp",
		2500.0,
		now,
		true,
		nil, nil, nil,
		prevSamples,
	)

	if event == nil {
		t.Fatal("Expected spike event to be detected")
	}
	if event.Severity != "critical" {
		t.Errorf("Expected severity 'critical', got '%s'", event.Severity)
	}
	if event.Reasons[0] != "icmp_critical_absolute_threshold" {
		t.Errorf("Expected first reason 'icmp_critical_absolute_threshold', got %v", event.Reasons)
	}
}

// TestSpikeDetector_RelativeThresholdOnly tests relative 10x median spike detection.
func TestSpikeDetector_RelativeThresholdOnly(t *testing.T) {
	config := SpikeConfig{
		ICMPWarningMs:        1000,
		ICMPCriticalMs:       5000,
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

	// 600ms is below 1000ms warning but 12x median
	event := detector.DetectAndRecord(
		"test-target", "icmp",
		600.0,
		now,
		true,
		nil, nil, nil,
		prevSamples,
	)

	if event == nil {
		t.Fatal("Expected spike event to be detected")
	}
	if event.Severity != "warning" {
		t.Errorf("Expected severity 'warning', got '%s'", event.Severity)
	}
	if len(event.Reasons) != 1 || event.Reasons[0] != "relative_10x_median_threshold" {
		t.Errorf("Expected reason 'relative_10x_median_threshold', got %v", event.Reasons)
	}
}

// TestSpikeDetector_RelativeThreshold_TooFewSamples tests that relative threshold
// is not applied when there aren't enough samples.
func TestSpikeDetector_RelativeThreshold_TooFewSamples(t *testing.T) {
	config := SpikeConfig{
		ICMPWarningMs:        500,
		ICMPCriticalMs:       2000,
		HTTPWarningMs:        1000,
		HTTPCriticalMs:       5000,
		RelativeMultiplier:   10.0,
		MinSamplesForMedian:  20,
		MaxPreviousSamples:   30,
		MaxEventsPerTracker:  100,
	}
	detector := NewSpikeDetectorWithConfig(config)

	now := time.Now().UTC()
	var prevSamples []LatencySample
	for i := 0; i < 10; i++ {
		prevSamples = append(prevSamples, LatencySample{
			Timestamp: now.Add(-time.Duration(10-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		})
	}

	// 600ms is above 500ms warning
	event := detector.DetectAndRecord(
		"test-target", "icmp",
		600.0,
		now,
		true,
		nil, nil, nil,
		prevSamples,
	)

	if event == nil {
		t.Fatal("Expected spike event (absolute threshold should still trigger)")
	}
	if len(event.Reasons) != 1 || event.Reasons[0] != "icmp_warning_absolute_threshold" {
		t.Errorf("Expected absolute threshold reason, got %v", event.Reasons)
	}
}

// TestSpikeDetector_NoSpikeBelowThresholds tests that normal latency doesn't trigger.
func TestSpikeDetector_NoSpikeBelowThresholds(t *testing.T) {
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

	event := detector.DetectAndRecord(
		"test-target", "icmp",
		100.0,
		now,
		true,
		nil, nil, nil,
		prevSamples,
	)

	if event != nil {
		t.Error("Expected no spike event for normal latency")
	}
}

// TestSpikeDetector_HTTPKind tests HTTP spike detection with HTTP thresholds.
func TestSpikeDetector_HTTPKind(t *testing.T) {
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
			LatencyMs: 200.0,
			Reachable: true,
		})
	}

	event := detector.DetectAndRecord(
		"test-target", "http",
		1500.0,
		now,
		true,
		nil, nil, nil,
		prevSamples,
	)

	if event == nil {
		t.Fatal("Expected spike event")
	}
	if event.Kind != "http" {
		t.Errorf("Expected kind 'http', got '%s'", event.Kind)
	}
	if len(event.Reasons) != 1 || event.Reasons[0] != "http_warning_absolute_threshold" {
		t.Errorf("Expected HTTP threshold reason, got %v", event.Reasons)
	}
}

// TestSpikeDetector_RollingMedianCalculation tests median calculation accuracy.
func TestSpikeDetector_RollingMedianCalculation(t *testing.T) {
	config := SpikeConfig{
		ICMPWarningMs:        1000,
		ICMPCriticalMs:       5000,
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
	latencies := []float64{10, 20, 30, 40, 50}
	for i, lat := range latencies {
		prevSamples = append(prevSamples, LatencySample{
			Timestamp: now.Add(-time.Duration(5-i) * time.Second),
			LatencyMs: lat,
			Reachable: true,
		})
	}

	event := detector.DetectAndRecord(
		"test-target", "icmp",
		300.0,
		now,
		true,
		nil, nil, nil,
		prevSamples,
	)

	if event == nil {
		t.Fatal("Expected spike event at exactly 10x median")
	}
	if event.RollingMedianMs != 30.0 {
		t.Errorf("Expected rolling median 30.0, got %f", event.RollingMedianMs)
	}
}

// TestSpikeDetector_FailedSamplesExcludedFromMedian tests that failed probes
// are excluded from median calculation.
func TestSpikeDetector_FailedSamplesExcludedFromMedian(t *testing.T) {
	config := SpikeConfig{
		ICMPWarningMs:        1000,
		ICMPCriticalMs:       5000,
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
		Timestamp: now.Add(-6 * time.Second),
		LatencyMs: 1000.0,
		Reachable: false,
	})
	for i, lat := range []float64{10, 20, 30, 40, 50} {
		prevSamples = append(prevSamples, LatencySample{
			Timestamp: now.Add(-time.Duration(5-i) * time.Second),
			LatencyMs: lat,
			Reachable: true,
		})
	}

	event := detector.DetectAndRecord(
		"test-target", "icmp",
		300.0,
		now,
		true,
		nil, nil, nil,
		prevSamples,
	)

	if event == nil {
		t.Fatal("Expected spike event")
	}
	if event.RollingMedianMs != 30.0 {
		t.Errorf("Expected median 30.0 (from successful samples), got %f", event.RollingMedianMs)
	}
}

// TestSpikeDetector_UnknownKindReturnsNil tests that unknown probe kind returns nil.
func TestSpikeDetector_UnknownKindReturnsNil(t *testing.T) {
	detector := NewSpikeDetector()

	now := time.Now().UTC()
	var prevSamples []LatencySample
	prevSamples = append(prevSamples, LatencySample{
		Timestamp: now,
		LatencyMs: 50.0,
		Reachable: true,
	})

	event := detector.DetectAndRecord(
		"test-target", "unknown",
		10000.0,
		now,
		true,
		nil, nil, nil,
		prevSamples,
	)

	if event != nil {
		t.Error("Expected nil for unknown probe kind")
	}
}

// TestSpikeDetector_SchedulerDelayIncluded tests scheduler delay is captured.
func TestSpikeDetector_SchedulerDelayIncluded(t *testing.T) {
	detector := NewSpikeDetector()

	now := time.Now().UTC()
	var prevSamples []LatencySample
	prevSamples = append(prevSamples, LatencySample{
		Timestamp: now,
		LatencyMs: 50.0,
		Reachable: true,
	})

	delayMs := 350.0
	event := detector.DetectAndRecord(
		"test-target", "icmp",
		2500.0,
		now,
		true,
		&delayMs, nil, nil,
		prevSamples,
	)

	if event == nil {
		t.Fatal("Expected spike event")
	}
	if event.SchedulerDelayMs == nil || *event.SchedulerDelayMs != 350.0 {
		t.Errorf("Expected scheduler delay 350ms, got %v", event.SchedulerDelayMs)
	}
}
