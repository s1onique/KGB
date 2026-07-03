package domain

import (
	"math"
	"testing"
	"time"
)

func TestNewLatencyMillis(t *testing.T) {
	tests := []struct {
		name    string
		input   float64
		wantOK  bool
		wantVal float64
	}{
		{
			name:    "valid positive value",
			input:   100.5,
			wantOK:  true,
			wantVal: 100.5,
		},
		{
			name:    "zero is valid",
			input:   0,
			wantOK:  true,
			wantVal: 0,
		},
		{
			name:    "small positive value",
			input:   0.001,
			wantOK:  true,
			wantVal: 0.001,
		},
		{
			name:    "large value",
			input:   1000000.0,
			wantOK:  true,
			wantVal: 1000000.0,
		},
		{
			name:   "NaN is rejected",
			input:  math.NaN(),
			wantOK: false,
		},
		{
			name:   "positive infinity is rejected",
			input:  math.Inf(1),
			wantOK: false,
		},
		{
			name:   "negative infinity is rejected",
			input:  math.Inf(-1),
			wantOK: false,
		},
		{
			name:   "negative value is rejected",
			input:  -1.0,
			wantOK: false,
		},
		{
			name:   "negative small value is rejected",
			input:  -0.001,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NewLatencyMillis(tt.input)
			if ok != tt.wantOK {
				t.Errorf("NewLatencyMillis(%v) ok = %v, want %v", tt.input, ok, tt.wantOK)
				return
			}
			if ok && got.Float64() != tt.wantVal {
				t.Errorf("NewLatencyMillis(%v) = %v, want %v", tt.input, got.Float64(), tt.wantVal)
			}
		})
	}
}

func TestLatencyMillis_Float64(t *testing.T) {
	tests := []struct {
		name  string
		input float64
	}{
		{"zero", 0},
		{"small", 0.001},
		{"typical", 50.5},
		{"large", 5000.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, ok := NewLatencyMillis(tt.input)
			if !ok {
				t.Fatalf("NewLatencyMillis(%v) failed unexpectedly", tt.input)
			}
			if got := l.Float64(); got != tt.input {
				t.Errorf("LatencyMillis.Float64() = %v, want %v", got, tt.input)
			}
		})
	}
}

func TestSampleFromState(t *testing.T) {
	ts := time.Now()
	probeKind := ProbeKindHTTP

	tests := []struct {
		name      string
		latencyMs float64
		reachable bool
		wantOK    bool
	}{
		{
			name:      "successful with valid latency",
			latencyMs: 100.0,
			reachable: true,
			wantOK:    true,
		},
		{
			name:      "successful with zero latency",
			latencyMs: 0,
			reachable: true,
			wantOK:    true,
		},
		{
			name:      "failed probe",
			latencyMs: 0,
			reachable: false,
			wantOK:    true,
		},
		{
			name:      "successful with NaN latency",
			latencyMs: math.NaN(),
			reachable: true,
			wantOK:    false,
		},
		{
			name:      "successful with negative latency",
			latencyMs: -1.0,
			reachable: true,
			wantOK:    false,
		},
		{
			name:      "successful with infinity latency",
			latencyMs: math.Inf(1),
			reachable: true,
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := SampleFromState(ts, tt.latencyMs, tt.reachable, probeKind)
			if ok != tt.wantOK {
				t.Errorf("SampleFromState() ok = %v, want %v", ok, tt.wantOK)
				return
			}
			if ok {
				if got.At != ts {
					t.Errorf("Sample.At = %v, want %v", got.At, ts)
				}
				if got.Kind != probeKind {
					t.Errorf("Sample.Kind = %v, want %v", got.Kind, probeKind)
				}
				if got.OK != tt.reachable {
					t.Errorf("Sample.OK = %v, want %v", got.OK, tt.reachable)
				}
				if tt.reachable {
					if got.Latency.Float64() != tt.latencyMs {
						t.Errorf("Sample.Latency = %v, want %v", got.Latency.Float64(), tt.latencyMs)
					}
				}
			}
		})
	}
}
