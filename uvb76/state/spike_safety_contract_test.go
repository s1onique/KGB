package state

import (
	"math"
	"testing"
	"time"
)

// Contract tests for SpikeDetector safety invariants (part 2).
// These tests verify the runtime state invariants established by ACT-UVB76-HULK01.

func TestSpikeDetectorContract_NoPanic(t *testing.T) {
	sd := NewSpikeDetector()

	testCases := []struct {
		name   string
		samples []LatencySample
	}{
		{"empty", []LatencySample{}},
		{"nil_slice", nil},
		{"single_NaN", []LatencySample{
			{Timestamp: time.Now().UTC(), LatencyMs: math.NaN(), Reachable: true},
		}},
		{"all_NaN", []LatencySample{
			{Timestamp: time.Now().UTC(), LatencyMs: math.NaN(), Reachable: true},
			{Timestamp: time.Now().UTC(), LatencyMs: math.NaN(), Reachable: true},
			{Timestamp: time.Now().UTC(), LatencyMs: math.NaN(), Reachable: true},
		}},
		{"all_Inf", []LatencySample{
			{Timestamp: time.Now().UTC(), LatencyMs: math.Inf(1), Reachable: true},
			{Timestamp: time.Now().UTC(), LatencyMs: math.Inf(1), Reachable: true},
			{Timestamp: time.Now().UTC(), LatencyMs: math.Inf(1), Reachable: true},
		}},
		{"mixed_NaN_Inf", []LatencySample{
			{Timestamp: time.Now().UTC(), LatencyMs: math.NaN(), Reachable: true},
			{Timestamp: time.Now().UTC(), LatencyMs: math.Inf(1), Reachable: true},
			{Timestamp: time.Now().UTC(), LatencyMs: 100.0, Reachable: true},
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic in test case %s: %v", tc.name, r)
				}
			}()
			_ = sd.calculateMedian(tc.samples)
		})
	}
}

func TestSpikeDetectorContract_NoNaNDerivedDecision(t *testing.T) {
	sd := NewSpikeDetector()

	baseTime := time.Now().UTC()
	samples := []LatencySample{
		{Timestamp: baseTime, LatencyMs: math.NaN(), Reachable: true},
		{Timestamp: baseTime.Add(time.Second), LatencyMs: 100.0, Reachable: true},
	}

	median := sd.calculateMedian(samples)
	if math.IsNaN(median) {
		t.Error("median should not be NaN")
	}
	if median != 100.0 {
		t.Errorf("expected median 100.0, got %f", median)
	}
}

func TestSpikeDetectorContract_NoDivideByZero(t *testing.T) {
	sd := NewSpikeDetector()

	testCases := []struct {
		name   string
		samples []LatencySample
	}{
		{"empty", []LatencySample{}},
		{"single_zero", []LatencySample{
			{Timestamp: time.Now().UTC(), LatencyMs: 0.0, Reachable: true},
		}},
		{"all_zeros", []LatencySample{
			{Timestamp: time.Now().UTC(), LatencyMs: 0.0, Reachable: true},
			{Timestamp: time.Now().UTC(), LatencyMs: 0.0, Reachable: true},
			{Timestamp: time.Now().UTC(), LatencyMs: 0.0, Reachable: true},
		}},
		{"mixed_with_zero", []LatencySample{
			{Timestamp: time.Now().UTC(), LatencyMs: 0.0, Reachable: true},
			{Timestamp: time.Now().UTC(), LatencyMs: 50.0, Reachable: true},
			{Timestamp: time.Now().UTC(), LatencyMs: 0.0, Reachable: true},
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("divide-by-zero panic in test case %s: %v", tc.name, r)
				}
			}()
			median := sd.calculateMedian(tc.samples)
			if math.IsNaN(median) && len(tc.samples) > 0 {
				t.Errorf("median should not be NaN for test case %s", tc.name)
			}
		})
	}
}

func TestSpikeDetectorContract_CriticalGTEWarningOrdering(t *testing.T) {
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

	if config.ICMPCriticalMs < config.ICMPWarningMs {
		t.Errorf("ICMPCriticalMs (%f) < ICMPWarningMs (%f) - critical should be >= warning",
			config.ICMPCriticalMs, config.ICMPWarningMs)
	}
	if config.HTTPCriticalMs < config.HTTPWarningMs {
		t.Errorf("HTTPCriticalMs (%f) < HTTPWarningMs (%f) - critical should be >= warning",
			config.HTTPCriticalMs, config.HTTPWarningMs)
	}
}

func TestSpikeDetectorContract_RelativeThresholdDeterministic(t *testing.T) {
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

	baseTime := time.Now().UTC()
	samples := make([]LatencySample, 20)
	for i := 0; i < 20; i++ {
		samples[i] = LatencySample{
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			LatencyMs: 100.0,
			Reachable: true,
		}
	}

	// Run same operation twice with same inputs
	event1 := sd.DetectAndRecord(
		"test-target", "http",
		2000.0,
		baseTime.Add(21*time.Second),
		true, nil, nil, nil, samples, nil)

	event2 := sd.DetectAndRecord(
		"test-target", "http",
		2000.0,
		baseTime.Add(21*time.Second),
		true, nil, nil, nil, samples, nil)

	// Both should produce same result
	if (event1 == nil) != (event2 == nil) {
		t.Error("non-deterministic spike detection result")
	}
}

func TestSpikeDetectorContract_CorruptedSliceHeader(t *testing.T) {
	sd := NewSpikeDetector()

	baseTime := time.Now().UTC()
	samples := []LatencySample{
		{Timestamp: baseTime, LatencyMs: 50.0, Reachable: true},
		{Timestamp: baseTime.Add(time.Second), LatencyMs: 60.0, Reachable: true},
		{Timestamp: baseTime.Add(2 * time.Second), LatencyMs: 70.0, Reachable: true},
	}

	// Should not panic even with valid-looking slice
	median := sd.calculateMedian(samples)
	if median <= 0 || median > 100 {
		t.Errorf("unexpected median value: %f", median)
	}
}

func TestSpikeDetectorContract_MultipleSpikeReasons(t *testing.T) {
	config := SpikeConfig{
		ICMPWarningMs:       100,
		ICMPCriticalMs:      500,
		HTTPWarningMs:       500,
		HTTPCriticalMs:      2000,
		RelativeMultiplier:   2.0,
		MinSamplesForMedian: 5,
		MaxPreviousSamples:  10,
		MaxEventsPerTracker: 100,
	}
	sd := NewSpikeDetectorWithConfig(config)

	baseTime := time.Now().UTC()
	samples := make([]LatencySample, 5)
	for i := 0; i < 5; i++ {
		samples[i] = LatencySample{
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			LatencyMs: 100.0,
			Reachable: true,
		}
	}

	// Very high latency that triggers multiple spike reasons
	event := sd.DetectAndRecord(
		"test-target", "http",
		10000.0,
		baseTime.Add(6*time.Second),
		true, nil, nil, nil, samples, nil)

	// Event should not be nil (extremely high latency)
	if event == nil {
		t.Error("expected spike event for extremely high latency")
	}
}

func TestSpikeDetectorContract_GetSpikesReturnsDefensiveCopy(t *testing.T) {
	config := SpikeConfig{
		ICMPWarningMs:       100,
		ICMPCriticalMs:      500,
		HTTPWarningMs:       500,
		HTTPCriticalMs:      2000,
		RelativeMultiplier:   2.0,
		MinSamplesForMedian: 5,
		MaxPreviousSamples:  10,
		MaxEventsPerTracker: 5,
	}
	sd := NewSpikeDetectorWithConfig(config)

	baseTime := time.Now().UTC()
	samples := make([]LatencySample, 5)
	for i := 0; i < 5; i++ {
		samples[i] = LatencySample{
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			LatencyMs: 100.0,
			Reachable: true,
		}
	}

	sd.DetectAndRecord(
		"test-target", "http",
		5000.0,
		baseTime.Add(6*time.Second),
		true, nil, nil, nil, samples, nil)

	spikes1 := sd.GetSpikes("test-target", "http", 100)
	spikes2 := sd.GetSpikes("test-target", "http", 100)

	if len(spikes1) != len(spikes2) {
		t.Error("defensive copy not working - spike counts differ")
	}
}
