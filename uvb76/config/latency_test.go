package config

import (
	"testing"
)

func TestICMPDefaultRetention(t *testing.T) {
	cfg := ICMPProbeConfig{}
	cfg.ApplyDefaults()

	// ICMP defaults: 1s interval, 60s window, 3600s retained range
	if cfg.IntervalSeconds != DefaultICMPIntervalSeconds {
		t.Errorf("Expected ICMP interval %d, got %d", DefaultICMPIntervalSeconds, cfg.IntervalSeconds)
	}

	if cfg.RetainedRangeSeconds != DefaultICMPRetainedRangeSeconds {
		t.Errorf("Expected ICMP retained range %d, got %d", DefaultICMPRetainedRangeSeconds, cfg.RetainedRangeSeconds)
	}

	// At 1s interval with 3600s retention, we need at least 3600 samples
	expectedMinSamples := cfg.RetainedRangeSeconds / cfg.IntervalSeconds
	if cfg.RecentSamplesMax < expectedMinSamples {
		t.Errorf("Expected ICMP RecentSamplesMax >= %d, got %d", expectedMinSamples, cfg.RecentSamplesMax)
	}

	// Verify: 3600 seconds / 1s interval = 3600 samples
	if cfg.RecentSamplesMax < 3600 {
		t.Errorf("Expected ICMP RecentSamplesMax >= 3600 for 60m retention at 1s interval, got %d", cfg.RecentSamplesMax)
	}
}

func TestHTTPDefaultRetention(t *testing.T) {
	cfg := HTTPProbeConfig{}
	cfg.ApplyDefaults()

	// HTTP defaults: 15s interval, 300s window, 14400s retained range (4 hours)
	if cfg.IntervalSeconds != DefaultHTTPIntervalSeconds {
		t.Errorf("Expected HTTP interval %d, got %d", DefaultHTTPIntervalSeconds, cfg.IntervalSeconds)
	}

	if cfg.RetainedRangeSeconds != DefaultHTTPRetainedRangeSeconds {
		t.Errorf("Expected HTTP retained range %d, got %d", DefaultHTTPRetainedRangeSeconds, cfg.RetainedRangeSeconds)
	}

	// At 15s interval with 14400s retention, we need at least 960 samples
	expectedMinSamples := cfg.RetainedRangeSeconds / cfg.IntervalSeconds
	if cfg.RecentSamplesMax < expectedMinSamples {
		t.Errorf("Expected HTTP RecentSamplesMax >= %d, got %d", expectedMinSamples, cfg.RecentSamplesMax)
	}

	// Verify: 14400 seconds / 15s interval = 960 samples
	if cfg.RecentSamplesMax < 960 {
		t.Errorf("Expected HTTP RecentSamplesMax >= 960 for 4h retention at 15s interval, got %d", cfg.RecentSamplesMax)
	}
}

func TestICMPRetentionAutoClamp(t *testing.T) {
	// Test that retention auto-clamps upward if window/interval requires more
	// Use retained=3600 with RecentSamplesMax=60 to trigger auto-clamp
	cfg := ICMPProbeConfig{
		IntervalSeconds:      1,
		WindowSeconds:        60,
		RetainedRangeSeconds: 3600, // 60 minutes
		RecentSamplesMax:     60,   // Only 60 samples, insufficient for 3600s retention
	}
	cfg.ApplyDefaults()

	// Should be clamped up to at least 3600 samples for full retention horizon
	if cfg.RecentSamplesMax < 3600 {
		t.Errorf("Expected RecentSamplesMax >= 3600 after auto-clamp, got %d", cfg.RecentSamplesMax)
	}
}

func TestHTTPRetentionAutoClamp(t *testing.T) {
	// Test that retention auto-clamps upward if window/interval requires more
	cfg := HTTPProbeConfig{
		IntervalSeconds:     15,
		WindowSeconds:       300,
		RetainedRangeSeconds: 14400, // 4 hours
		RecentSamplesMax:    100,    // Insufficient for 14400s retention
	}
	cfg.ApplyDefaults()

	// Should be clamped up to at least 960 samples for full retention horizon
	if cfg.RecentSamplesMax < 960 {
		t.Errorf("Expected RecentSamplesMax >= 960 after auto-clamp, got %d", cfg.RecentSamplesMax)
	}
}

func TestValidateRetainedRangeCoversRetention(t *testing.T) {
	// Test that validation rejects if samples can't cover retained range
	// Skip ApplyDefaults to test raw validation behavior
	// 500 * 15 = 7500s < 14400s retained - should fail validation
	err := ValidateHTTPProbeConfig(HTTPProbeConfig{
		IntervalSeconds:      15,
		WindowSeconds:        300,
		RetainedRangeSeconds: 14400, // 4 hours
		RecentSamplesMax:     500,   // 500 * 15 = 7500s < 14400s - should fail
		HistogramBucketsMS:   DefaultHistogramBuckets(),
	})
	if err == nil {
		t.Error("Expected validation error for insufficient samples to cover retention range")
	}
}

func TestValidateICMPRetainedRangeCoversRetention(t *testing.T) {
	// Test that validation rejects if ICMP samples can't cover retained range
	// Use larger interval so 3000 * 10 = 30000 < 36000 (60 min) - should fail
	// Skip ApplyDefaults to test raw validation
	err := ValidateICMPProbeConfig(ICMPProbeConfig{
		IntervalSeconds:      10,
		TimeoutSeconds:       3,
		WindowSeconds:        60,
		RetainedRangeSeconds: 3600,  // 60 minutes
		RecentSamplesMax:     300,   // 300 * 10 = 3000s < 3600s - should fail
		HistogramBucketsMS:   DefaultHistogramBuckets(),
	})
	if err == nil {
		t.Error("Expected validation error for insufficient ICMP samples to cover retention range")
	}
}

func TestValidateRetainedRangeAtLeastWindow(t *testing.T) {
	// Test that retained_range must be >= window
	// Use large RecentSamplesMax so ApplyDefaults doesn't trigger retention clamp
	cfg := HTTPProbeConfig{
		IntervalSeconds:      15,
		WindowSeconds:        300,
		RetainedRangeSeconds: 100,  // Less than window (100 < 300)
		RecentSamplesMax:     1000, // Large enough not to be auto-clamped
	}
	cfg.ApplyDefaults()

	// After ApplyDefaults: retained should be clamped to window (300)
	// So validation should pass since now retained >= window
	// Instead test that we CANNOT have retained < window in validation
	err := ValidateHTTPProbeConfig(HTTPProbeConfig{
		IntervalSeconds:      15,
		WindowSeconds:        300,
		RetainedRangeSeconds: 100,  // Intentionally < window
		RecentSamplesMax:     1000,
	})
	if err == nil {
		t.Error("Expected validation error when retained_range < window")
	}
}

func TestRetainedRangeComputationFromHorizon(t *testing.T) {
	// Test that sample count is correctly computed from retention horizon and interval
	tests := []struct {
		name            string
		intervalSec     int
		retentionSec    int
		expectedMinSamp int
	}{
		{"ICMP 1s interval 60m retention", 1, 3600, 3600},
		{"HTTP 15s interval 4h retention", 15, 14400, 960},
		{"HTTP 30s interval 2h retention", 30, 7200, 240},
		{"ICMP 5s interval 30m retention", 5, 1800, 360},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := HTTPProbeConfig{
				IntervalSeconds:     tc.intervalSec,
				WindowSeconds:       tc.intervalSec * 10, // window = 10 intervals
				RetainedRangeSeconds: tc.retentionSec,
				RecentSamplesMax:    0, // auto-compute
			}
			cfg.ApplyDefaults()

			if cfg.RecentSamplesMax < tc.expectedMinSamp {
				t.Errorf("Expected RecentSamplesMax >= %d for %s, got %d",
					tc.expectedMinSamp, tc.name, cfg.RecentSamplesMax)
			}
		})
	}
}

func TestLatencyConfigApplyDefaults(t *testing.T) {
	// Test that LatencyConfig.ApplyDefaults() applies to both HTTP and ICMP
	cfg := LatencyConfig{}
	cfg.ApplyDefaults()

	// Verify HTTP defaults applied
	if cfg.HTTP.IntervalSeconds != DefaultHTTPIntervalSeconds {
		t.Errorf("Expected HTTP interval %d, got %d", DefaultHTTPIntervalSeconds, cfg.HTTP.IntervalSeconds)
	}
	if cfg.HTTP.RetainedRangeSeconds != DefaultHTTPRetainedRangeSeconds {
		t.Errorf("Expected HTTP retained range %d, got %d", DefaultHTTPRetainedRangeSeconds, cfg.HTTP.RetainedRangeSeconds)
	}

	// Verify ICMP defaults applied
	if cfg.ICMP.IntervalSeconds != DefaultICMPIntervalSeconds {
		t.Errorf("Expected ICMP interval %d, got %d", DefaultICMPIntervalSeconds, cfg.ICMP.IntervalSeconds)
	}
	if cfg.ICMP.RetainedRangeSeconds != DefaultICMPRetainedRangeSeconds {
		t.Errorf("Expected ICMP retained range %d, got %d", DefaultICMPRetainedRangeSeconds, cfg.ICMP.RetainedRangeSeconds)
	}

	// Verify ICMP has enough samples for full retention
	expectedICMPSamples := cfg.ICMP.RetainedRangeSeconds / cfg.ICMP.IntervalSeconds
	if cfg.ICMP.RecentSamplesMax < expectedICMPSamples {
		t.Errorf("Expected ICMP RecentSamplesMax >= %d, got %d", expectedICMPSamples, cfg.ICMP.RecentSamplesMax)
	}
}
