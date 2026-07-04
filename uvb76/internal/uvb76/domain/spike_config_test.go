package domain

import (
	"math"
	"testing"
)

func TestNewSpikeConfig(t *testing.T) {
	tests := []struct {
		name               string
		warningMs          float64
		criticalMs         float64
		relativeMultiplier float64
		minSamples         int
		wantOK             bool
		description        string
	}{
		// Valid configs
		{
			name:               "valid HTTP config",
			warningMs:          1000,
			criticalMs:         5000,
			relativeMultiplier: 10.0,
			minSamples:         20,
			wantOK:             true,
			description:        "standard HTTP thresholds accepted",
		},
		{
			name:               "valid ICMP config",
			warningMs:          500,
			criticalMs:         2000,
			relativeMultiplier: 10.0,
			minSamples:         20,
			wantOK:             true,
			description:        "standard ICMP thresholds accepted",
		},
		{
			name:               "zero relative multiplier accepted",
			warningMs:          1000,
			criticalMs:         5000,
			relativeMultiplier: 0,
			minSamples:         20,
			wantOK:             true,
			description:        "zero multiplier disables relative threshold",
		},
		{
			name:               "zero min samples accepted",
			warningMs:          1000,
			criticalMs:         5000,
			relativeMultiplier: 10.0,
			minSamples:         0,
			wantOK:             true,
			description:        "zero min samples may be used",
		},

		// Invalid warning threshold
		{
			name:               "NaN warning rejected",
			warningMs:          math.NaN(),
			criticalMs:         5000,
			relativeMultiplier: 10.0,
			minSamples:         20,
			wantOK:             false,
			description:        "NaN warning threshold rejected",
		},
		{
			name:               "Inf warning rejected",
			warningMs:          math.Inf(1),
			criticalMs:         5000,
			relativeMultiplier: 10.0,
			minSamples:         20,
			wantOK:             false,
			description:        "positive Inf warning threshold rejected",
		},
		{
			name:               "negative Inf warning rejected",
			warningMs:          math.Inf(-1),
			criticalMs:         5000,
			relativeMultiplier: 10.0,
			minSamples:         20,
			wantOK:             false,
			description:        "negative Inf warning threshold rejected",
		},
		{
			name:               "negative warning rejected",
			warningMs:          -100,
			criticalMs:         5000,
			relativeMultiplier: 10.0,
			minSamples:         20,
			wantOK:             false,
			description:        "negative warning threshold rejected",
		},

		// Invalid critical threshold
		{
			name:               "NaN critical rejected",
			warningMs:          1000,
			criticalMs:         math.NaN(),
			relativeMultiplier: 10.0,
			minSamples:         20,
			wantOK:             false,
			description:        "NaN critical threshold rejected",
		},
		{
			name:               "Inf critical rejected",
			warningMs:          1000,
			criticalMs:         math.Inf(1),
			relativeMultiplier: 10.0,
			minSamples:         20,
			wantOK:             false,
			description:        "positive Inf critical threshold rejected",
		},
		{
			name:               "negative critical rejected",
			warningMs:          1000,
			criticalMs:         -500,
			relativeMultiplier: 10.0,
			minSamples:         20,
			wantOK:             false,
			description:        "negative critical threshold rejected",
		},

		// Critical must be >= warning
		{
			name:               "critical below warning rejected",
			warningMs:          5000,
			criticalMs:         1000,
			relativeMultiplier: 10.0,
			minSamples:         20,
			wantOK:             false,
			description:        "critical must be >= warning",
		},

		// Invalid relative multiplier
		{
			name:               "NaN relative multiplier rejected",
			warningMs:          1000,
			criticalMs:         5000,
			relativeMultiplier: math.NaN(),
			minSamples:         20,
			wantOK:             false,
			description:        "NaN relative multiplier rejected",
		},
		{
			name:               "Inf relative multiplier rejected",
			warningMs:          1000,
			criticalMs:         5000,
			relativeMultiplier: math.Inf(1),
			minSamples:         20,
			wantOK:             false,
			description:        "positive Inf relative multiplier rejected",
		},
		{
			name:               "negative relative multiplier rejected",
			warningMs:          1000,
			criticalMs:         5000,
			relativeMultiplier: -5.0,
			minSamples:         20,
			wantOK:             false,
			description:        "negative relative multiplier rejected",
		},

		// Invalid min samples
		{
			name:               "negative MinSamplesForMedian rejected",
			warningMs:          1000,
			criticalMs:         5000,
			relativeMultiplier: 10.0,
			minSamples:         -1,
			wantOK:             false,
			description:        "negative MinSamplesForMedian rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, ok := NewSpikeConfig(tt.warningMs, tt.criticalMs, tt.relativeMultiplier, tt.minSamples)
			if ok != tt.wantOK {
				t.Errorf("NewSpikeConfig() ok = %v, want %v (%s)", ok, tt.wantOK, tt.description)
			}
			if ok && tt.wantOK {
				// Verify values are correctly stored
				if cfg.WarningAbsoluteMillis.Float64() != tt.warningMs {
					t.Errorf("WarningAbsoluteMillis = %v, want %v", cfg.WarningAbsoluteMillis.Float64(), tt.warningMs)
				}
				if cfg.CriticalAbsoluteMillis.Float64() != tt.criticalMs {
					t.Errorf("CriticalAbsoluteMillis = %v, want %v", cfg.CriticalAbsoluteMillis.Float64(), tt.criticalMs)
				}
				if cfg.RelativeMultiplier != tt.relativeMultiplier {
					t.Errorf("RelativeMultiplier = %v, want %v", cfg.RelativeMultiplier, tt.relativeMultiplier)
				}
				if cfg.MinSamplesForMedian != tt.minSamples {
					t.Errorf("MinSamplesForMedian = %v, want %v", cfg.MinSamplesForMedian, tt.minSamples)
				}
			}
		})
	}
}
