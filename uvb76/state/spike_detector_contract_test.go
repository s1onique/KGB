package state

import (
	"math"
	"testing"
	"time"
)

// Contract tests for SpikeDetector safety invariants (part 1).
// These tests verify the runtime state invariants established by ACT-UVB76-HULK01.

func TestSpikeDetectorContract_EmptySampleWindow(t *testing.T) {
	sd := NewSpikeDetector()
	config := DefaultSpikeConfig()
	config.MinSamplesForMedian = 5

	samples := []LatencySample{}

	median := sd.calculateMedian(samples)
	if median != 0 {
		t.Errorf("expected 0 median for empty window, got %f", median)
	}
}

func TestSpikeDetectorContract_OnePriorSample(t *testing.T) {
	sd := NewSpikeDetector()

	baseTime := time.Now().UTC()
	samples := []LatencySample{
		{Timestamp: baseTime, LatencyMs: 50.0, Reachable: true},
	}

	median := sd.calculateMedian(samples)
	if median != 50.0 {
		t.Errorf("expected 50.0 median for single sample, got %f", median)
	}
}

func TestSpikeDetectorContract_AllFailedSamples(t *testing.T) {
	sd := NewSpikeDetector()

	baseTime := time.Now().UTC()
	samples := []LatencySample{
		{Timestamp: baseTime, LatencyMs: 0.0, Reachable: false},
		{Timestamp: baseTime.Add(time.Second), LatencyMs: 0.0, Reachable: false},
		{Timestamp: baseTime.Add(2 * time.Second), LatencyMs: 0.0, Reachable: false},
	}

	median := sd.calculateMedian(samples)
	if median != 0 {
		t.Errorf("expected 0 median for all-failed samples, got %f", median)
	}
}

func TestSpikeDetectorContract_MixedSuccessFailureSamples(t *testing.T) {
	sd := NewSpikeDetector()

	baseTime := time.Now().UTC()
	samples := []LatencySample{
		{Timestamp: baseTime, LatencyMs: 10.0, Reachable: true},
		{Timestamp: baseTime.Add(time.Second), LatencyMs: 0.0, Reachable: false},
		{Timestamp: baseTime.Add(2 * time.Second), LatencyMs: 20.0, Reachable: true},
		{Timestamp: baseTime.Add(3 * time.Second), LatencyMs: 0.0, Reachable: false},
		{Timestamp: baseTime.Add(4 * time.Second), LatencyMs: 30.0, Reachable: true},
	}

	median := sd.calculateMedian(samples)
	if median != 20.0 {
		t.Errorf("expected 20.0 median for mixed samples, got %f", median)
	}
}

func TestSpikeDetectorContract_NaNLatency(t *testing.T) {
	sd := NewSpikeDetector()

	baseTime := time.Now().UTC()
	samples := []LatencySample{
		{Timestamp: baseTime, LatencyMs: math.NaN(), Reachable: true},
		{Timestamp: baseTime.Add(time.Second), LatencyMs: 20.0, Reachable: true},
		{Timestamp: baseTime.Add(2 * time.Second), LatencyMs: 30.0, Reachable: true},
	}

	median := sd.calculateMedian(samples)
	if median != 25.0 {
		t.Errorf("expected 25.0 median (NaN filtered), got %f", median)
	}
}

func TestSpikeDetectorContract_InfLatency(t *testing.T) {
	sd := NewSpikeDetector()

	baseTime := time.Now().UTC()
	samples := []LatencySample{
		{Timestamp: baseTime, LatencyMs: math.Inf(1), Reachable: true},
		{Timestamp: baseTime.Add(time.Second), LatencyMs: 20.0, Reachable: true},
		{Timestamp: baseTime.Add(2 * time.Second), LatencyMs: 30.0, Reachable: true},
	}

	median := sd.calculateMedian(samples)
	if median != 25.0 {
		t.Errorf("expected 25.0 median (+Inf filtered), got %f", median)
	}
}

func TestSpikeDetectorContract_NegativeLatency(t *testing.T) {
	sd := NewSpikeDetector()

	baseTime := time.Now().UTC()
	samples := []LatencySample{
		{Timestamp: baseTime, LatencyMs: -10.0, Reachable: true},
		{Timestamp: baseTime.Add(time.Second), LatencyMs: 20.0, Reachable: true},
		{Timestamp: baseTime.Add(2 * time.Second), LatencyMs: 30.0, Reachable: true},
	}

	median := sd.calculateMedian(samples)
	if median != 20.0 {
		t.Errorf("expected 20.0 median, got %f", median)
	}
}

func TestSpikeDetectorContract_ZeroMedian(t *testing.T) {
	sd := NewSpikeDetector()

	baseTime := time.Now().UTC()
	samples := []LatencySample{
		{Timestamp: baseTime, LatencyMs: 0.0, Reachable: true},
		{Timestamp: baseTime.Add(time.Second), LatencyMs: 0.0, Reachable: true},
		{Timestamp: baseTime.Add(2 * time.Second), LatencyMs: 0.0, Reachable: true},
	}

	median := sd.calculateMedian(samples)
	if median != 0.0 {
		t.Errorf("expected 0.0 median for all-zero samples, got %f", median)
	}
}

func TestSpikeDetectorContract_ThresholdEqualWarning(t *testing.T) {
	config := SpikeConfig{
		ICMPWarningMs:       500,
		ICMPCriticalMs:      2000,
		HTTPWarningMs:       1000,
		HTTPCriticalMs:      5000,
		RelativeMultiplier:   10.0,
		MinSamplesForMedian: 20,
		MaxPreviousSamples:  30,
		MaxEventsPerTracker: 100,
	}
	sd := NewSpikeDetectorWithConfig(config)

	samples := make([]LatencySample, 20)
	baseTime := time.Now().UTC()
	for i := 0; i < 20; i++ {
		samples[i] = LatencySample{
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			LatencyMs: 100.0,
			Reachable: true,
		}
	}

	event := sd.DetectAndRecord(
		"test-target", "http",
		1000.0,
		baseTime.Add(21*time.Second),
		true, nil, nil, nil, samples, nil)

	_ = event
}

func TestSpikeDetectorContract_ThresholdEqualCritical(t *testing.T) {
	config := SpikeConfig{
		ICMPWarningMs:       500,
		ICMPCriticalMs:      2000,
		HTTPWarningMs:       1000,
		HTTPCriticalMs:      5000,
		RelativeMultiplier:   10.0,
		MinSamplesForMedian: 20,
		MaxPreviousSamples:  30,
		MaxEventsPerTracker: 100,
	}
	sd := NewSpikeDetectorWithConfig(config)

	samples := make([]LatencySample, 20)
	baseTime := time.Now().UTC()
	for i := 0; i < 20; i++ {
		samples[i] = LatencySample{
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			LatencyMs: 100.0,
			Reachable: true,
		}
	}

	event := sd.DetectAndRecord(
		"test-target", "http",
		5000.0,
		baseTime.Add(21*time.Second),
		true, nil, nil, nil, samples, nil)

	_ = event
}

func TestSpikeDetectorContract_CriticalBelowWarningConfig(t *testing.T) {
	config := SpikeConfig{
		ICMPWarningMs:       2000,
		ICMPCriticalMs:      500,
		HTTPWarningMs:       5000,
		HTTPCriticalMs:      1000,
		RelativeMultiplier:   10.0,
		MinSamplesForMedian: 20,
		MaxPreviousSamples:  30,
		MaxEventsPerTracker: 100,
	}
	sd := NewSpikeDetectorWithConfig(config)

	samples := make([]LatencySample, 20)
	baseTime := time.Now().UTC()
	for i := 0; i < 20; i++ {
		samples[i] = LatencySample{
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			LatencyMs: 100.0,
			Reachable: true,
		}
	}

	event := sd.DetectAndRecord(
		"test-target", "http",
		3000.0,
		baseTime.Add(21*time.Second),
		true, nil, nil, nil, samples, nil)

	_ = event
}
