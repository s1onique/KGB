package domain

import (
	"testing"
	"time"
)

// makeSample creates a Sample with valid latency for testing.
func makeSample(latencyMs float64, ok bool) Sample {
	s := Sample{
		At: time.Now(),
		OK: ok,
	}
	if ok {
		s.Latency = LatencyMillis{v: latencyMs}
	}
	return s
}

func TestNewSampleWindow(t *testing.T) {
	tests := []struct {
		name     string
		input    []Sample
		wantLen  int
		wantOnly bool // true if all input was OK
	}{
		{
			name:    "empty input",
			input:   []Sample{},
			wantLen: 0,
		},
		{
			name: "all successful",
			input: []Sample{
				makeSample(10, true),
				makeSample(20, true),
				makeSample(30, true),
			},
			wantLen: 3,
		},
		{
			name: "all failed",
			input: []Sample{
				makeSample(0, false),
				makeSample(0, false),
				makeSample(0, false),
			},
			wantLen: 0,
		},
		{
			name: "mixed success and failed",
			input: []Sample{
				makeSample(10, true),
				makeSample(0, false),
				makeSample(30, true),
				makeSample(0, false),
				makeSample(50, true),
			},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			window := NewSampleWindow(tt.input)
			if window.Len() != tt.wantLen {
				t.Errorf("NewSampleWindow() len = %d, want %d", window.Len(), tt.wantLen)
			}
		})
	}
}

func TestSampleWindow_Median(t *testing.T) {
	tests := []struct {
		name        string
		input       []Sample
		wantMedian  float64
		wantOK      bool
		description string
	}{
		{
			name:        "empty window",
			input:       []Sample{},
			wantMedian:  0,
			wantOK:      false,
			description: "empty windows return ok=false",
		},
		{
			name: "single sample",
			input: []Sample{
				makeSample(100, true),
			},
			wantMedian: 100,
			wantOK:     true,
			description: "one sample returns that sample",
		},
		{
			name: "odd count - three samples",
			input: []Sample{
				makeSample(50, true),
				makeSample(100, true),
				makeSample(150, true),
			},
			wantMedian: 100,
			wantOK:     true,
			description: "odd count returns middle sorted value",
		},
		{
			name: "odd count - five samples",
			input: []Sample{
				makeSample(10, true),
				makeSample(20, true),
				makeSample(30, true),
				makeSample(40, true),
				makeSample(50, true),
			},
			wantMedian: 30,
			wantOK:     true,
			description: "odd count returns middle sorted value",
		},
		{
			name: "even count - two samples",
			input: []Sample{
				makeSample(10, true),
				makeSample(30, true),
			},
			wantMedian: 20,
			wantOK:     true,
			description: "even count returns average of middle two",
		},
		{
			name: "even count - four samples",
			input: []Sample{
				makeSample(10, true),
				makeSample(20, true),
				makeSample(30, true),
				makeSample(40, true),
			},
			wantMedian: 25,
			wantOK:     true,
			description: "even count returns average of middle two",
		},
		{
			name: "unsorted input",
			input: []Sample{
				makeSample(100, true),
				makeSample(10, true),
				makeSample(50, true),
			},
			wantMedian: 50,
			wantOK:     true,
			description: "median calculation sorts internally",
		},
		{
			name: "failed samples ignored",
			input: []Sample{
				makeSample(10, true),
				makeSample(0, false),
				makeSample(30, true),
				makeSample(0, false),
				makeSample(50, true),
			},
			wantMedian: 30,
			wantOK:     true,
			description: "failed samples are excluded from median",
		},
		{
			name: "all failed returns ok=false",
			input: []Sample{
				makeSample(0, false),
				makeSample(0, false),
			},
			wantMedian: 0,
			wantOK:     false,
			description: "only failed samples returns ok=false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			window := NewSampleWindow(tt.input)
			got, ok := window.Median()
			if ok != tt.wantOK {
				t.Errorf("Median() ok = %v, want %v (%s)", ok, tt.wantOK, tt.description)
				return
			}
			if ok && got.Float64() != tt.wantMedian {
				t.Errorf("Median() = %v, want %v (%s)", got.Float64(), tt.wantMedian, tt.description)
			}
		})
	}
}

func TestSampleWindow_Samples(t *testing.T) {
	samples := []Sample{
		makeSample(10, true),
		makeSample(20, true),
		makeSample(30, true),
	}
	window := NewSampleWindow(samples)

	// Get the samples copy
	got := window.Samples()

	// Verify length
	if len(got) != len(samples) {
		t.Errorf("Samples() len = %d, want %d", len(got), len(samples))
	}

	// Verify it's a copy - mutating should not affect window
	got[0] = makeSample(999, true)
	if window.Samples()[0].Latency.Float64() == 999 {
		t.Error("Samples() returned a shared slice - mutation affects window")
	}

	// Verify empty window returns nil
	emptyWindow := NewSampleWindow([]Sample{})
	if emptyWindow.Samples() != nil {
		t.Errorf("Samples() on empty window should return nil, got %v", emptyWindow.Samples())
	}
}

func TestSampleWindow_P50(t *testing.T) {
	window := NewSampleWindow([]Sample{
		makeSample(10, true),
		makeSample(20, true),
		makeSample(30, true),
	})

	median, ok := window.Median()
	p50, p50ok := window.P50()

	if ok != p50ok {
		t.Errorf("P50() ok = %v, Median() ok = %v", p50ok, ok)
	}
	if p50.Float64() != median.Float64() {
		t.Errorf("P50() = %v, Median() = %v", p50.Float64(), median.Float64())
	}
}

func TestSampleWindow_Percentiles(t *testing.T) {
	tests := []struct {
		name     string
		input    []Sample
		testFn   func(SampleWindow) (LatencyMillis, bool)
		wantMin  float64 // minimum expected value
		wantMax  float64 // maximum expected value (for interpolation range)
		wantOK   bool
	}{
		{
			name:  "P90 with 10 samples",
			input: makeSequentialSamples(10),
			testFn: func(w SampleWindow) (LatencyMillis, bool) {
				return w.P90()
			},
			wantMin: 9.0,  // P90 with 10 samples (1-10): rank = 0.9*9+1 = 9.1, interpolates between 9 and 10
			wantMax: 10.0,
			wantOK:  true,
		},
		{
			name:  "P95 with 20 samples",
			input: makeSequentialSamples(20),
			testFn: func(w SampleWindow) (LatencyMillis, bool) {
				return w.P95()
			},
			wantMin: 19.0, // P95 with 20 samples (1-20): rank = 0.95*19+1 = 19.05
			wantMax: 20.0,
			wantOK:  true,
		},
		{
			name:  "P99 with 100 samples",
			input: makeSequentialSamples(100),
			testFn: func(w SampleWindow) (LatencyMillis, bool) {
				return w.P99()
			},
			wantMin: 99.0, // P99 with 100 samples (1-100): rank = 0.99*99+1 = 99.01
			wantMax: 100.0,
			wantOK:  true,
		},
		{
			name:  "P90 empty window",
			input: []Sample{},
			testFn: func(w SampleWindow) (LatencyMillis, bool) {
				return w.P90()
			},
			wantMin: 0,
			wantMax: 0,
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			window := NewSampleWindow(tt.input)
			got, ok := tt.testFn(window)
			if ok != tt.wantOK {
				t.Errorf("%s ok = %v, want %v", tt.name, ok, tt.wantOK)
				return
			}
			if ok {
				val := got.Float64()
				if val < tt.wantMin || val > tt.wantMax {
					t.Errorf("%s = %v, want between %v and %v", tt.name, val, tt.wantMin, tt.wantMax)
				}
			}
		})
	}
}

// makeSequentialSamples creates samples with latency 1, 2, ..., n.
func makeSequentialSamples(n int) []Sample {
	samples := make([]Sample, n)
	for i := 0; i < n; i++ {
		samples[i] = makeSample(float64(i+1), true)
	}
	return samples
}
