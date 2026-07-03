package domain

import (
	"testing"
	"time"
)

func TestDecideSpike(t *testing.T) {
	cfg := SpikeConfig{
		WarningAbsoluteMillis:  LatencyMillis{v: 1000},
		CriticalAbsoluteMillis: LatencyMillis{v: 5000},
		RelativeMultiplier:     10.0,
		MinSamplesForMedian:   5,
	}

	makeCurrentSample := func(latencyMs float64, ok bool) Sample {
		s := Sample{
			At: time.Now(),
			OK: ok,
		}
		if ok {
			s.Latency = LatencyMillis{v: latencyMs}
		}
		return s
	}

	makePreviousWindow := func(latencies []float64) SampleWindow {
		samples := make([]Sample, len(latencies))
		for i, lat := range latencies {
			samples[i] = Sample{
				At: time.Now(),
				OK: true,
				Latency: LatencyMillis{v: lat},
			}
		}
		return NewSampleWindow(samples)
	}

	tests := []struct {
		name           string
		current        Sample
		previous       SampleWindow
		cfg            SpikeConfig
		wantKind       SpikeDecisionKind
		description    string
	}{
		{
			name:        "no previous samples means no relative spike",
			current:     makeCurrentSample(100, true),
			previous:    makePreviousWindow([]float64{}),
			cfg:         cfg,
			wantKind:    SpikeDecisionNone,
			description: "empty window cannot produce relative spike",
		},
		{
			name:        "insufficient samples for relative threshold",
			current:     makeCurrentSample(500, true), // Below warning threshold (1000ms)
			previous:    makePreviousWindow([]float64{100, 100, 100}),
			cfg:         cfg,
			wantKind:    SpikeDecisionNone,
			description: "less than MinSamplesForMedian does not trigger relative spike",
		},
		{
			name:        "critical absolute threshold",
			current:     makeCurrentSample(6000, true),
			previous:    makePreviousWindow([]float64{}),
			cfg:         cfg,
			wantKind:    SpikeDecisionCritical,
			description: "exceeds critical absolute threshold",
		},
		{
			name:        "warning absolute threshold",
			current:     makeCurrentSample(2000, true),
			previous:    makePreviousWindow([]float64{}),
			cfg:         cfg,
			wantKind:    SpikeDecisionWarning,
			description: "between warning and critical thresholds",
		},
		{
			name:        "critical beats warning",
			current:     makeCurrentSample(6000, true),
			previous:    makePreviousWindow([]float64{100, 100, 100, 100, 100}),
			cfg:         cfg,
			wantKind:    SpikeDecisionCritical,
			description: "critical takes precedence over warning",
		},
		{
			name:        "relative threshold triggers warning",
			current:     makeCurrentSample(2000, true),
			previous:    makePreviousWindow([]float64{100, 100, 100, 100, 100}),
			cfg:         cfg,
			wantKind:    SpikeDecisionWarning,
			description: "relative spike when current >= 10x median (100 * 10 = 1000)",
		},
		{
			name:        "relative threshold triggers critical",
			current:     makeCurrentSample(5000, true),
			previous:    makePreviousWindow([]float64{100, 100, 100, 100, 100}),
			cfg:         cfg,
			wantKind:    SpikeDecisionCritical,
			description: "relative spike also exceeds absolute critical",
		},
		{
			name:        "failed current sample does not panic",
			current:     makeCurrentSample(0, false),
			previous:    makePreviousWindow([]float64{100, 100, 100, 100, 100}),
			cfg:         cfg,
			wantKind:    SpikeDecisionNone,
			description: "failed samples do not produce latency spikes",
		},
		{
			name:        "empty previous window does not panic",
			current:     makeCurrentSample(10000, true),
			previous:    makePreviousWindow([]float64{}),
			cfg:         cfg,
			wantKind:    SpikeDecisionCritical,
			description: "can still trigger absolute threshold with empty window",
		},
		{
			name:        "previous window with only failed samples does not panic",
			current:     makeCurrentSample(10000, true),
			previous:    NewSampleWindow([]Sample{
				{At: time.Now(), OK: false},
				{At: time.Now(), OK: false},
			}),
			cfg:         cfg,
			wantKind:    SpikeDecisionCritical,
			description: "failed samples excluded from median calculation",
		},
		{
			name:        "below all thresholds",
			current:     makeCurrentSample(100, true),
			previous:    makePreviousWindow([]float64{50, 50, 50, 50, 50}),
			cfg:         cfg,
			wantKind:    SpikeDecisionNone,
			description: "normal latency below all thresholds",
		},
		{
			name:        "zero multiplier disables relative threshold",
			current:     makeCurrentSample(10000, true),
			previous:    makePreviousWindow([]float64{100, 100, 100, 100, 100}),
			cfg:         SpikeConfig{WarningAbsoluteMillis: LatencyMillis{v: 1000}, CriticalAbsoluteMillis: LatencyMillis{v: 5000}, RelativeMultiplier: 0, MinSamplesForMedian: 5},
			wantKind:    SpikeDecisionCritical,
			description: "zero multiplier means relative threshold cannot trigger",
		},
		{
			name:        "negative multiplier disables relative threshold",
			current:     makeCurrentSample(10000, true),
			previous:    makePreviousWindow([]float64{100, 100, 100, 100, 100}),
			cfg:         SpikeConfig{WarningAbsoluteMillis: LatencyMillis{v: 1000}, CriticalAbsoluteMillis: LatencyMillis{v: 5000}, RelativeMultiplier: -1, MinSamplesForMedian: 5},
			wantKind:    SpikeDecisionCritical,
			description: "negative multiplier means relative threshold cannot trigger",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("DecideSpike panicked: %v (%s)", r, tt.description)
				}
			}()

			got := DecideSpike(tt.current, tt.previous, tt.cfg)
			if got.Kind != tt.wantKind {
				t.Errorf("DecideSpike() = %v, want %v (%s)", got.Kind, tt.wantKind, tt.description)
			}
		})
	}
}

func TestDefaultSpikeConfigs(t *testing.T) {
	httpCfg := DefaultSpikeConfigHTTP()
	if httpCfg.WarningAbsoluteMillis.Float64() != 1000 {
		t.Errorf("HTTP warning threshold = %v, want 1000", httpCfg.WarningAbsoluteMillis.Float64())
	}
	if httpCfg.CriticalAbsoluteMillis.Float64() != 5000 {
		t.Errorf("HTTP critical threshold = %v, want 5000", httpCfg.CriticalAbsoluteMillis.Float64())
	}

	icmpCfg := DefaultSpikeConfigICMP()
	if icmpCfg.WarningAbsoluteMillis.Float64() != 500 {
		t.Errorf("ICMP warning threshold = %v, want 500", icmpCfg.WarningAbsoluteMillis.Float64())
	}
	if icmpCfg.CriticalAbsoluteMillis.Float64() != 2000 {
		t.Errorf("ICMP critical threshold = %v, want 2000", icmpCfg.CriticalAbsoluteMillis.Float64())
	}
}
