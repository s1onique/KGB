package domain

import (
	"math"
	"testing"
	"time"
)

// FuzzSampleWindowMedianNeverPanics tests that SampleWindow median never panics
// on any input, including empty windows, single samples, and fuzzed values.
func FuzzSampleWindowMedianNeverPanics(f *testing.F) {
	// Seed corpus with known edge cases
	f.Add([]byte{0})                                    // empty
	f.Add([]byte{1, 100})                              // single sample
	f.Add([]byte{2, 100, 200})                         // two samples (even)
	f.Add([]byte{3, 50, 100, 150})                    // three samples (odd)
	f.Add([]byte{5, 10, 20, 30, 40, 50})             // five samples
	f.Add([]byte{10, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100}) // ten samples

	f.Fuzz(func(t *testing.T, data []byte) {
		// Parse data format: [count, val1, val2, ...]
		// count is number of samples (0-255)
		// values are uint8 (0-255) representing latency in ms
		if len(data) == 0 {
			return
		}

		count := int(data[0])
		maxValues := len(data) - 1
		if count > maxValues {
			count = maxValues
		}

		// Build samples array - use all successful samples for median calculation
		var samples []Sample
		for i := 0; i < count; i++ {
			latency := float64(data[i+1])
			s := Sample{
				At: time.Now(),
				OK: true,
			}
			// Construct valid LatencyMillis - skip invalid values
			if lat, ok := NewLatencyMillis(latency); ok {
				s.Latency = lat
				samples = append(samples, s)
			}
		}

		// Create window
		window := NewSampleWindow(samples)

		// Call Median - should never panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Median panicked with %d samples: %v", len(samples), r)
			}
		}()

		median, ok := window.Median()

		// Verify properties
		if !ok && window.Len() == 0 {
			// Empty window should return ok=false - this is correct
		} else if ok {
			// Valid median should be non-negative and within range of input
			if median.Float64() < 0 {
				t.Errorf("Median returned negative value: %v", median.Float64())
			}
		}

		// Call Samples - should never panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Samples panicked with %d samples: %v", len(samples), r)
			}
		}()

		returnedSamples := window.Samples()
		if window.Len() > 0 && returnedSamples == nil {
			t.Errorf("Non-empty window returned nil Samples()")
		}
		if returnedSamples != nil && len(returnedSamples) != window.Len() {
			t.Errorf("Samples() length mismatch: got %d, want %d", len(returnedSamples), window.Len())
		}
	})
}

// FuzzLatencyMillisNeverPanics tests that LatencyMillis construction never panics.
func FuzzLatencyMillisNeverPanics(f *testing.F) {
	// Seed with edge cases
	f.Add(float64(0))
	f.Add(float64(100))
	f.Add(float64(-1))
	f.Add(math.MaxFloat64)
	f.Add(math.SmallestNonzeroFloat64)
	f.Add(math.NaN())
	f.Add(math.Inf(1))
	f.Add(math.Inf(-1))

	f.Fuzz(func(t *testing.T, v float64) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("NewLatencyMillis panicked with %v: %v", v, r)
			}
		}()

		lat, ok := NewLatencyMillis(v)
		_ = lat.Float64()

		// Validate invariants
		if ok {
			if math.IsNaN(lat.Float64()) || math.IsInf(lat.Float64(), 0) || lat.Float64() < 0 {
				t.Errorf("Valid LatencyMillis has invalid value: %v", lat.Float64())
			}
		}
	})
}

// FuzzDecideSpikeNeverPanics tests that DecideSpike never panics on any input.
func FuzzDecideSpikeNeverPanics(f *testing.F) {
	// Seed with edge cases
	cfg := SpikeConfig{
		WarningAbsoluteMillis:  LatencyMillis{v: 1000},
		CriticalAbsoluteMillis: LatencyMillis{v: 5000},
		RelativeMultiplier:     10.0,
		MinSamplesForMedian:   5,
	}

	f.Fuzz(func(t *testing.T, currentOK bool, currentLatency float64, prevCount int) {
		// Build current sample
		current := Sample{
			At: time.Now(),
			OK: currentOK,
		}
		if currentOK {
			if lat, ok := NewLatencyMillis(currentLatency); ok {
				current.Latency = lat
			}
		}

		// Build previous window with bounded size
		if prevCount < 0 {
			prevCount = 0
		}
		if prevCount > 100 {
			prevCount = 100
		}

		var samples []Sample
		for i := 0; i < prevCount; i++ {
			samples = append(samples, Sample{
				At: time.Now(),
				OK: true,
				Latency: LatencyMillis{v: float64(i + 1)},
			})
		}
		window := NewSampleWindow(samples)

		// DecideSpike should never panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("DecideSpike panicked: %v", r)
			}
		}()

		decision := DecideSpike(current, window, cfg)
		_ = decision.Kind
	})
}
