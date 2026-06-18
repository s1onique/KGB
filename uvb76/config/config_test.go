package config

import (
	"testing"
)

func TestHTTPProbeConfig_ApplyDefaults(t *testing.T) {
	cfg := &HTTPProbeConfig{}
	cfg.ApplyDefaults()

	if cfg.IntervalSeconds != DefaultHTTPIntervalSeconds {
		t.Errorf("expected interval %d, got %d", DefaultHTTPIntervalSeconds, cfg.IntervalSeconds)
	}
	if cfg.TimeoutMilliseconds != DefaultHTTPTimeoutMilliseconds {
		t.Errorf("expected timeout %d, got %d", DefaultHTTPTimeoutMilliseconds, cfg.TimeoutMilliseconds)
	}
	if cfg.WindowSeconds != DefaultHTTPWindowSeconds {
		t.Errorf("expected window %d, got %d", DefaultHTTPWindowSeconds, cfg.WindowSeconds)
	}
	if cfg.RetainedRangeSeconds != DefaultHTTPRetainedRangeSeconds {
		t.Errorf("expected retained %d, got %d", DefaultHTTPRetainedRangeSeconds, cfg.RetainedRangeSeconds)
	}
	if !cfg.IsEnabled() {
		t.Error("expected enabled by default")
	}
}

func TestICMPProbeConfig_ApplyDefaults(t *testing.T) {
	cfg := &ICMPProbeConfig{}
	cfg.ApplyDefaults()

	if cfg.IntervalSeconds != DefaultICMPIntervalSeconds {
		t.Errorf("expected interval %d, got %d", DefaultICMPIntervalSeconds, cfg.IntervalSeconds)
	}
	if cfg.TimeoutSeconds != DefaultICMPTimeoutSeconds {
		t.Errorf("expected timeout %d, got %d", DefaultICMPTimeoutSeconds, cfg.TimeoutSeconds)
	}
	if cfg.WindowSeconds != DefaultICMPWindowSeconds {
		t.Errorf("expected window %d, got %d", DefaultICMPWindowSeconds, cfg.WindowSeconds)
	}
	if cfg.RetainedRangeSeconds != DefaultICMPRetainedRangeSeconds {
		t.Errorf("expected retained %d, got %d", DefaultICMPRetainedRangeSeconds, cfg.RetainedRangeSeconds)
	}
	if !cfg.IsEnabled() {
		t.Error("expected enabled by default")
	}
}

func TestHTTPProbeConfig_ExplicitDisable(t *testing.T) {
	disabled := false
	cfg := &HTTPProbeConfig{
		Enabled: &disabled,
	}
	cfg.ApplyDefaults()

	if cfg.IsEnabled() {
		t.Error("expected disabled when explicitly set to false")
	}
}

func TestICMPProbeConfig_ExplicitDisable(t *testing.T) {
	disabled := false
	cfg := &ICMPProbeConfig{
		Enabled: &disabled,
	}
	cfg.ApplyDefaults()

	if cfg.IsEnabled() {
		t.Error("expected disabled when explicitly set to false")
	}
}

func TestLatencyConfig_ApplyDefaults(t *testing.T) {
	cfg := &LatencyConfig{}
	cfg.ApplyDefaults()

	// Both HTTP and ICMP should be enabled by default
	if !cfg.HTTP.IsEnabled() {
		t.Error("expected HTTP enabled by default")
	}
	if !cfg.ICMP.IsEnabled() {
		t.Error("expected ICMP enabled by default")
	}

	// LatencyConfig.IsEnabled should return true if either is enabled
	if !cfg.IsEnabled() {
		t.Error("expected LatencyConfig.IsEnabled true when HTTP enabled")
	}
}

func TestLatencyConfig_IsEnabled_NeitherEnabled(t *testing.T) {
	httpDisabled := false
	icmpDisabled := false
	cfg := &LatencyConfig{
		HTTP: HTTPProbeConfig{Enabled: &httpDisabled},
		ICMP: ICMPProbeConfig{Enabled: &icmpDisabled},
	}

	if cfg.IsEnabled() {
		t.Error("expected LatencyConfig.IsEnabled false when both disabled")
	}
}

func TestValidateHTTPProbeConfig(t *testing.T) {
	cfg := HTTPProbeConfig{
		IntervalSeconds:     15,
		TimeoutMilliseconds: 10000,
		WindowSeconds:       300,
		RetainedRangeSeconds: 3000,
		RecentSamplesMax:    200, // must satisfy: 200 * 15 = 3000 >= 3000
		HistogramBucketsMS:  DefaultHistogramBuckets(),
	}

	err := ValidateHTTPProbeConfig(cfg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateICMPProbeConfig(t *testing.T) {
	cfg := ICMPProbeConfig{
		IntervalSeconds:      1,
		TimeoutSeconds:       3,
		WindowSeconds:        300,
		RetainedRangeSeconds: 3000,
		RecentSamplesMax:     3000, // must satisfy: 3000 * 1 >= 3000
		HistogramBucketsMS:   DefaultHistogramBuckets(),
	}

	err := ValidateICMPProbeConfig(cfg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateHTTPProbeConfig_RetainedLessThanWindow(t *testing.T) {
	cfg := HTTPProbeConfig{
		IntervalSeconds:     15,
		TimeoutMilliseconds: 10000,
		WindowSeconds:       300,
		RetainedRangeSeconds: 100, // less than window
		RecentSamplesMax:     100,
		HistogramBucketsMS:   DefaultHistogramBuckets(),
	}

	err := ValidateHTTPProbeConfig(cfg)
	if err == nil {
		t.Error("expected error when retained_range_seconds < window_seconds")
	}
}

func TestValidateICMPProbeConfig_RetainedLessThanWindow(t *testing.T) {
	cfg := ICMPProbeConfig{
		IntervalSeconds:      1,
		TimeoutSeconds:       3,
		WindowSeconds:        300,
		RetainedRangeSeconds: 100, // less than window
		RecentSamplesMax:      100,
		HistogramBucketsMS:    DefaultHistogramBuckets(),
	}

	err := ValidateICMPProbeConfig(cfg)
	if err == nil {
		t.Error("expected error when retained_range_seconds < window_seconds")
	}
}

func TestHTTPProbeConfig_RetainedClampedToWindow(t *testing.T) {
	cfg := &HTTPProbeConfig{
		WindowSeconds:       300,
		RetainedRangeSeconds: 100, // less than window
	}
	cfg.ApplyDefaults()

	// Should be clamped to window_seconds
	if cfg.RetainedRangeSeconds != cfg.WindowSeconds {
		t.Errorf("expected retained clamped to window %d, got %d", cfg.WindowSeconds, cfg.RetainedRangeSeconds)
	}
}

func TestICMPProbeConfig_RetainedClampedToWindow(t *testing.T) {
	cfg := &ICMPProbeConfig{
		WindowSeconds:        300,
		RetainedRangeSeconds: 100, // less than window
	}
	cfg.ApplyDefaults()

	// Should be clamped to window_seconds
	if cfg.RetainedRangeSeconds != cfg.WindowSeconds {
		t.Errorf("expected retained clamped to window %d, got %d", cfg.WindowSeconds, cfg.RetainedRangeSeconds)
	}
}

func TestICMPProbeConfig_DefaultRecentSamplesMax(t *testing.T) {
	cfg := &ICMPProbeConfig{}
	cfg.ApplyDefaults()

	// DefaultICMPRecentSamplesMax is now 0 (auto-compute)
	// ApplyDefaults computes it based on retained_range / interval
	// Expected: ceil(3600 / 1) = 3600
	if cfg.RecentSamplesMax != 3600 {
		t.Errorf("expected recent_samples_max 3600 (auto-computed for 60m retention), got %d", cfg.RecentSamplesMax)
	}
}

func TestICMPProbeConfig_RecentSamplesMaxClampedToRetention(t *testing.T) {
	// recent_samples_max=10, interval=1, retained=60 should be clamped to 60
	cfg := &ICMPProbeConfig{
		RecentSamplesMax: 10,
		IntervalSeconds:  1,
		RetainedRangeSeconds: 60,
	}
	cfg.ApplyDefaults()

	// Should be clamped to cover retention: minSamples = ceil(60/1) = 60
	if cfg.RecentSamplesMax != 60 {
		t.Errorf("expected recent_samples_max clamped to 60, got %d", cfg.RecentSamplesMax)
	}
}

func TestHTTPProbeConfig_RecentSamplesMaxClampedToRetention(t *testing.T) {
	// recent_samples_max=10, interval=15, retained=300 should be clamped to 20
	cfg := &HTTPProbeConfig{
		RecentSamplesMax: 10,
		IntervalSeconds: 15,
		RetainedRangeSeconds: 300,
	}
	cfg.ApplyDefaults()

	// Should be clamped to cover retention: minSamples = ceil(300/15) = 20
	if cfg.RecentSamplesMax != 20 {
		t.Errorf("expected recent_samples_max clamped to 20, got %d", cfg.RecentSamplesMax)
	}
}

func TestValidateICMPProbeConfig_InvariantViolation(t *testing.T) {
	// recent_samples_max=10, interval=1, window=60 violates invariant: 10*1 < 60
	cfg := ICMPProbeConfig{
		IntervalSeconds:      1,
		TimeoutSeconds:       3,
		WindowSeconds:        60,
		RetainedRangeSeconds: 60,
		RecentSamplesMax:     10,
		HistogramBucketsMS:   DefaultHistogramBuckets(),
	}

	err := ValidateICMPProbeConfig(cfg)
	if err == nil {
		t.Error("expected error when recent_samples_max * interval < window")
	}
}

func TestValidateHTTPProbeConfig_InvariantViolation(t *testing.T) {
	// recent_samples_max=10, interval=15, window=300 violates invariant: 10*15 < 300
	cfg := HTTPProbeConfig{
		IntervalSeconds:     15,
		TimeoutMilliseconds: 10000,
		WindowSeconds:       300,
		RetainedRangeSeconds: 300,
		RecentSamplesMax:    10,
		HistogramBucketsMS:  DefaultHistogramBuckets(),
	}

	err := ValidateHTTPProbeConfig(cfg)
	if err == nil {
		t.Error("expected error when recent_samples_max * interval < window")
	}
}
