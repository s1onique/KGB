package state

import (
	"math"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
)

// TestSpikeWindow_Property_NoPanicWithAnyInput verifies that spike detection
// does not panic with any valid input combination.
func TestSpikeWindow_Property_NoPanicWithAnyInput(t *testing.T) {
	detector := NewSpikeDetector()

	testCases := []struct {
		name    string
		latency float64
		ok      bool
		kind    string
	}{
		{"normal_latency_http", 100.0, true, "http"},
		{"high_latency_http", 2000.0, true, "http"},
		{"normal_latency_icmp", 50.0, true, "icmp"},
		{"high_latency_icmp", 600.0, true, "icmp"},
		{"failed_probe", 0.0, false, "http"},
		{"zero_latency", 0.0, true, "http"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Build various window sizes
			for windowSize := 0; windowSize <= 50; windowSize += 10 {
				now := time.Now().UTC()
				prevSamples := make([]domain.Sample, windowSize)
				for i := 0; i < windowSize; i++ {
					lat, _ := domain.NewLatencyMillis(float64(i%100) + 10.0)
					prevSamples[i] = domain.Sample{
						At:      now.Add(-time.Duration(i) * time.Second),
						Kind:    domain.ProbeKindHTTP,
						Latency: lat,
						OK:      true,
					}
				}
				prevWindow := domain.NewSampleWindow(prevSamples)

				// Should not panic
				_ = detector.DetectAndRecordWithWindow(
					"test-target", tc.kind,
					tc.latency,
					now,
					tc.ok,
					nil,
					nil,
					nil,
					prevWindow,
					nil,
				)
			}
		})
	}
}

// TestSpikeWindow_Property_PreviousSamplesBounded verifies that PreviousSamples
// in spike events is bounded by MaxPreviousSamples.
func TestSpikeWindow_Property_PreviousSamplesBounded(t *testing.T) {
	detector := NewSpikeDetectorWithConfig(SpikeConfig{
		MaxPreviousSamples: 30,
	})

	// Build large previous window (100 samples)
	now := time.Now().UTC()
	prevSamples := make([]domain.Sample, 100)
	for i := 0; i < 100; i++ {
		lat, _ := domain.NewLatencyMillis(50.0)
		prevSamples[i] = domain.Sample{
			At:      now.Add(-time.Duration(i) * time.Second),
			Kind:    domain.ProbeKindHTTP,
			Latency: lat,
			OK:      true,
		}
	}
	prevWindow := domain.NewSampleWindow(prevSamples)

	// Trigger spike
	event := detector.DetectAndRecordWithWindow(
		"test-target", "http",
		5000.0, // extreme latency to trigger spike
		now,
		true,
		nil,
		nil,
		nil,
		prevWindow,
		nil,
	)

	if event == nil {
		t.Fatal("Expected spike event")
	}

	// PreviousSamples should be bounded
	if len(event.PreviousSamples) > 30 {
		t.Errorf("PreviousSamples count %d exceeds MaxPreviousSamples (30)", len(event.PreviousSamples))
	}
}

// TestSpikeWindow_Property_NoNaNInEvent verifies that spike events do not contain
// NaN or Inf values.
func TestSpikeWindow_Property_NoNaNInEvent(t *testing.T) {
	detector := NewSpikeDetector()

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
	prevWindow := domain.NewSampleWindow(prevSamples)

	event := detector.DetectAndRecordWithWindow(
		"test-target", "http",
		2000.0,
		now,
		true,
		nil,
		nil,
		nil,
		prevWindow,
		nil,
	)

	if event == nil {
		t.Skip("No spike event produced")
	}

	if math.IsNaN(event.LatencyMs) || math.IsInf(event.LatencyMs, 0) {
		t.Error("Event LatencyMs should not be NaN or Inf")
	}
	if math.IsNaN(event.RollingMedianMs) || math.IsInf(event.RollingMedianMs, 0) {
		t.Error("Event RollingMedianMs should not be NaN or Inf")
	}

	for i, ps := range event.PreviousSamples {
		if math.IsNaN(ps.LatencyMs) || math.IsInf(ps.LatencyMs, 0) {
			t.Errorf("PreviousSample[%d].LatencyMs should not be NaN or Inf", i)
		}
	}
}

// FuzzSpikeWindow tests spike detection with random inputs.
// Run with: go test -fuzz=FuzzSpikeWindow -fuzztime=30s ./state/
func FuzzSpikeWindow(f *testing.F) {
	detector := NewSpikeDetector()

	// Seed with typical cases
	f.Add(float64(100.0), true, "http", 30)
	f.Add(float64(2000.0), true, "http", 30)
	f.Add(float64(600.0), true, "icmp", 30)
	f.Add(float64(0.0), false, "http", 30)

	f.Fuzz(func(t *testing.T, latency float64, reachable bool, kind string, windowSize int) {
		// Clamp inputs to valid ranges
		if windowSize < 0 {
			windowSize = 0
		}
		if windowSize > 100 {
			windowSize = 100
		}
		if kind != "http" && kind != "icmp" {
			kind = "http"
		}
		// Skip invalid latencies
		if reachable && (math.IsNaN(latency) || math.IsInf(latency, 0) || latency < 0) {
			return
		}

		// Build window
		now := time.Now().UTC()
		prevSamples := make([]domain.Sample, windowSize)
		for i := 0; i < windowSize; i++ {
			lat, ok := domain.NewLatencyMillis(float64(i%100) + 10.0)
			if !ok {
				t.Skip("Could not create valid latency")
			}
			prevSamples[i] = domain.Sample{
				At:      now.Add(-time.Duration(i) * time.Second),
				Kind:    domain.ProbeKindHTTP,
				Latency: lat,
				OK:      true,
			}
		}
		prevWindow := domain.NewSampleWindow(prevSamples)

		// Should not panic
		_ = detector.DetectAndRecordWithWindow(
			"fuzz-target", kind,
			latency,
			now,
			reachable,
			nil,
			nil,
			nil,
			prevWindow,
			nil,
		)
	})
}

// FuzzSpikeWindowParityProperty tests domain vs state parity properties for normal latency spikes.
// Run with: go test -fuzz=FuzzSpikeWindowParityProperty -fuzztime=30s ./state/
// This fuzz test is excluded from HTTP failure/recovery since those are state-owned special cases.
func FuzzSpikeWindowParityProperty(f *testing.F) {
	detector := NewSpikeDetector()

	// Seed with typical cases
	f.Add(float64(100.0), true, "http", 30)
	f.Add(float64(2000.0), true, "http", 30)
	f.Add(float64(600.0), true, "icmp", 30)
	f.Add(float64(300.0), true, "http", 30) // relative spike case

	f.Fuzz(func(t *testing.T, latency float64, reachable bool, kind string, windowSize int) {
		// Only test successful probes (HTTP failure is state-owned)
		if !reachable {
			return
		}

		// Clamp inputs to valid ranges
		if windowSize < 0 {
			windowSize = 0
		}
		if windowSize > 100 {
			windowSize = 100
		}
		if kind != "http" && kind != "icmp" {
			kind = "http"
		}
		// Skip invalid latencies
		if math.IsNaN(latency) || math.IsInf(latency, 0) || latency < 0 {
			return
		}

		// Build window
		now := time.Now().UTC()
		prevSamples := make([]domain.Sample, windowSize)
		for i := 0; i < windowSize; i++ {
			lat, ok := domain.NewLatencyMillis(float64(i%100) + 10.0)
			if !ok {
				t.Skip("Could not create valid latency")
			}
			prevSamples[i] = domain.Sample{
				At:      now.Add(-time.Duration(i) * time.Second),
				Kind:    domain.ProbeKindHTTP,
				Latency: lat,
				OK:      true,
			}
		}
		prevWindow := domain.NewSampleWindow(prevSamples)

		// Get domain config
		stateCfg := detector.config
		domainCfg, ok := domainSpikeConfigForState(kind, stateCfg)
		if !ok {
			return
		}

		// Build domain sample
		domainLat, ok := domain.NewLatencyMillis(latency)
		if !ok {
			return
		}
		var probeKind domain.ProbeKind
		if kind == "http" {
			probeKind = domain.ProbeKindHTTP
		} else {
			probeKind = domain.ProbeKindICMP
		}
		current := domain.Sample{
			At:      now,
			Kind:    probeKind,
			Latency: domainLat,
			OK:      true,
		}

		// Get domain decision
		decision := domain.DecideSpike(current, prevWindow, domainCfg)

		// Get state event
		stateEvent := detector.DetectAndRecordWithWindow(
			"fuzz-target", kind,
			latency,
			now,
			reachable,
			nil,
			nil,
			nil,
			prevWindow,
			nil,
		)

		// Property 1: domain decision none implies no state event for normal latency
		if decision.Kind == domain.SpikeDecisionNone && stateEvent != nil {
			t.Errorf("Parity violation: domain=none but state produced event with severity=%s", stateEvent.Severity)
		}

		// Property 2: domain warning/critical implies state event severity matches
		if decision.Kind != domain.SpikeDecisionNone {
			if stateEvent == nil {
				t.Errorf("Parity violation: domain=%s but state produced no event", decision.Kind)
			} else {
				expectedSeverity := severityFromDomainDecision(decision.Kind)
				if stateEvent.Severity != expectedSeverity {
					t.Errorf("Parity violation: domain=%s but state severity=%s (expected %s)",
						decision.Kind, stateEvent.Severity, expectedSeverity)
				}
			}
		}

		// Property 3: event LatencyMs is never NaN/Inf
		if stateEvent != nil {
			if math.IsNaN(stateEvent.LatencyMs) || math.IsInf(stateEvent.LatencyMs, 0) {
				t.Errorf("Event LatencyMs should not be NaN or Inf: %v", stateEvent.LatencyMs)
			}
		}

		// Property 4: event RollingMedianMs is never NaN/Inf (when baseline exists)
		if stateEvent != nil && decision.PreviousCount > 0 {
			if math.IsNaN(stateEvent.RollingMedianMs) || math.IsInf(stateEvent.RollingMedianMs, 0) {
				t.Errorf("Event RollingMedianMs should not be NaN or Inf: %v", stateEvent.RollingMedianMs)
			}
		}

		// Property 5: PreviousSamples count <= MaxPreviousSamples
		if stateEvent != nil {
			if len(stateEvent.PreviousSamples) > detector.config.MaxPreviousSamples {
				t.Errorf("PreviousSamples count %d exceeds MaxPreviousSamples %d",
					len(stateEvent.PreviousSamples), detector.config.MaxPreviousSamples)
			}
		}
	})
}
