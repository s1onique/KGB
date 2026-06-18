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
		RecentSamplesMax:    100,
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
		RecentSamplesMax:     100,
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
