// Package config provides configuration loading and validation for UVB-76.
package config

import (
	"errors"
	"fmt"
)

// LatencyConfig holds latency measurement settings.
// Supports both HTTP and ICMP probe configurations for independent latency tracking.
type LatencyConfig struct {
	HTTP HTTPProbeConfig `json:"http"`
	ICMP ICMPProbeConfig `json:"icmp"`
}

// HTTPProbeConfig holds HTTP status probe latency measurement settings.
type HTTPProbeConfig struct {
	Enabled              *bool  `json:"enabled"` // pointer so we can distinguish unset from false
	IntervalSeconds      int    `json:"interval_seconds"`
	TimeoutMilliseconds  int    `json:"timeout_milliseconds"`
	HistogramBucketsMS   []int64 `json:"histogram_buckets_ms"`
	RecentSamplesMax     int    `json:"recent_samples_max"`
	WindowSeconds        int    `json:"window_seconds"`
	RetainedRangeSeconds int    `json:"retained_range_seconds"`
}

// ICMPProbeConfig holds ICMP ping latency measurement settings.
type ICMPProbeConfig struct {
	Enabled              *bool  `json:"enabled"` // pointer so we can distinguish unset from false
	IntervalSeconds      int    `json:"interval_seconds"`
	TimeoutSeconds       int    `json:"timeout_seconds"`
	MaxConcurrentOSPing  int    `json:"max_concurrent_os_ping"` // bounds concurrent os/exec ping processes
	Backend              string `json:"backend"` // "native" or "os_ping"; defaults to "native" on Linux
	HistogramBucketsMS   []int64 `json:"histogram_buckets_ms"`
	RecentSamplesMax     int    `json:"recent_samples_max"`
	WindowSeconds        int    `json:"window_seconds"`
	RetainedRangeSeconds int    `json:"retained_range_seconds"`
}

// HTTP defaults
const (
	// DefaultHTTPIntervalSeconds is the default interval for HTTP status probing.
	DefaultHTTPIntervalSeconds = 15
	// DefaultHTTPTimeoutMilliseconds is the default timeout for HTTP probes.
	DefaultHTTPTimeoutMilliseconds = 10000
	// DefaultHTTPWindowSeconds is the default trailing window for HTTP latency aggregation.
	DefaultHTTPWindowSeconds = 300
	// DefaultHTTPRetainedRangeSeconds is the default retained range for HTTP latency (4 hours).
	DefaultHTTPRetainedRangeSeconds = 14400
	// DefaultRecentSamplesMax is the default max number of recent latency samples to keep.
	// Computed dynamically from retained_range / interval in ApplyDefaults.
	DefaultRecentSamplesMax = 0 // 0 means auto-compute
)

// DefaultHistogramBuckets returns standard histogram bucket boundaries in ms.
func DefaultHistogramBuckets() []int64 {
	return []int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}
}

// ICMP defaults
const (
	// DefaultICMPIntervalSeconds is the default interval for ICMP ping probing.
	DefaultICMPIntervalSeconds = 1
	// DefaultICMPTimeoutSeconds is the default timeout for ICMP ping probes.
	DefaultICMPTimeoutSeconds = 3
	// DefaultICMPMaxConcurrentOSPing is the default max concurrent OS ping processes.
	// Default to 1 for router safety - OS ping is diagnostic, not forwarding path.
	DefaultICMPMaxConcurrentOSPing = 1
	// DefaultICMPWindowSeconds is the default trailing window for ICMP latency aggregation.
	DefaultICMPWindowSeconds = 60
	// DefaultICMPRetainedRangeSeconds is the default retained range for ICMP latency (60 minutes).
	DefaultICMPRetainedRangeSeconds = 3600
	// DefaultICMPRecentSamplesMax is the default max number of recent ICMP latency samples to keep.
	// Computed dynamically from retained_range / interval in ApplyDefaults.
	// At 1s interval with 3600s retention, this equals 3600 samples.
	DefaultICMPRecentSamplesMax = 0 // 0 means auto-compute
)

// ICMP backend type constants
const (
	// ICMPBackendNative uses native Go ICMP sockets (preferred on Linux).
	ICMPBackendNative = "native"
	// ICMPBackendOSPing uses os/exec ping (fallback only).
	ICMPBackendOSPing = "os_ping"
)

// ApplyDefaults applies sensible defaults to latency config when values are missing.
// Applies defaults to both HTTP and ICMP probe configurations.
// Does NOT overwrite an explicit enabled=false.
func (c *LatencyConfig) ApplyDefaults() {
	c.HTTP.ApplyDefaults()
	c.ICMP.ApplyDefaults()
}

// ApplyDefaults applies sensible defaults to HTTP probe config when values are missing.
// Does NOT overwrite an explicit enabled=false.
func (c *HTTPProbeConfig) ApplyDefaults() {
	// Only set enabled to true if not explicitly set to false
	if c.Enabled == nil {
		enabled := true
		c.Enabled = &enabled
	}
	if c.HistogramBucketsMS == nil || len(c.HistogramBucketsMS) == 0 {
		c.HistogramBucketsMS = DefaultHistogramBuckets()
	}
	if c.RecentSamplesMax <= 0 {
		c.RecentSamplesMax = DefaultRecentSamplesMax
	}
	if c.IntervalSeconds <= 0 {
		c.IntervalSeconds = DefaultHTTPIntervalSeconds
	}
	if c.TimeoutMilliseconds <= 0 {
		c.TimeoutMilliseconds = DefaultHTTPTimeoutMilliseconds
	}
	if c.WindowSeconds <= 0 {
		c.WindowSeconds = DefaultHTTPWindowSeconds
	}
	if c.RetainedRangeSeconds <= 0 {
		c.RetainedRangeSeconds = DefaultHTTPRetainedRangeSeconds
	}
	// Clamp retained_range to at least window_seconds
	if c.RetainedRangeSeconds < c.WindowSeconds {
		c.RetainedRangeSeconds = c.WindowSeconds
	}
	// Auto-clamp recent_samples_max to ensure full retained range can be covered
	// This ensures we have enough samples to span the entire retention horizon
	minSamplesForRetention := (c.RetainedRangeSeconds + c.IntervalSeconds - 1) / c.IntervalSeconds
	if c.RecentSamplesMax < minSamplesForRetention {
		c.RecentSamplesMax = minSamplesForRetention
	}
}

// IsHTTPEnabled returns whether HTTP latency measurement is enabled.
// Returns true if Enabled is nil (default) or true, false only if explicitly set to false.
func (c *HTTPProbeConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// ApplyDefaults applies sensible defaults to ICMP probe config when values are missing.
// Does NOT overwrite an explicit enabled=false.
func (c *ICMPProbeConfig) ApplyDefaults() {
	// Only set enabled to true if not explicitly set to false
	if c.Enabled == nil {
		enabled := true
		c.Enabled = &enabled
	}
	if c.HistogramBucketsMS == nil || len(c.HistogramBucketsMS) == 0 {
		c.HistogramBucketsMS = DefaultHistogramBuckets()
	}
	if c.RecentSamplesMax <= 0 {
		c.RecentSamplesMax = DefaultICMPRecentSamplesMax
	}
	if c.IntervalSeconds <= 0 {
		c.IntervalSeconds = DefaultICMPIntervalSeconds
	}
	if c.TimeoutSeconds <= 0 {
		c.TimeoutSeconds = DefaultICMPTimeoutSeconds
	}
	if c.MaxConcurrentOSPing <= 0 {
		c.MaxConcurrentOSPing = DefaultICMPMaxConcurrentOSPing
	}
	if c.WindowSeconds <= 0 {
		c.WindowSeconds = DefaultICMPWindowSeconds
	}
	if c.RetainedRangeSeconds <= 0 {
		c.RetainedRangeSeconds = DefaultICMPRetainedRangeSeconds
	}
	// Clamp retained_range to at least window_seconds
	if c.RetainedRangeSeconds < c.WindowSeconds {
		c.RetainedRangeSeconds = c.WindowSeconds
	}
	// Auto-clamp recent_samples_max to ensure full retained range can be covered
	// This ensures we have enough samples to span the entire retention horizon
	minSamplesForRetention := (c.RetainedRangeSeconds + c.IntervalSeconds - 1) / c.IntervalSeconds
	if c.RecentSamplesMax < minSamplesForRetention {
		c.RecentSamplesMax = minSamplesForRetention
	}
}

// IsICMPEnabled returns whether ICMP latency measurement is enabled.
// Returns true if Enabled is nil (default) or true, false only if explicitly set to false.
func (c *ICMPProbeConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// ValidateLatencyConfig validates the latency configuration.
func ValidateLatencyConfig(c LatencyConfig) error {
	if err := ValidateHTTPProbeConfig(c.HTTP); err != nil {
		return err
	}
	if err := ValidateICMPProbeConfig(c.ICMP); err != nil {
		return err
	}
	return nil
}

// ValidateHTTPProbeConfig validates the HTTP probe configuration.
func ValidateHTTPProbeConfig(c HTTPProbeConfig) error {
	if c.RecentSamplesMax <= 0 {
		return errors.New("latency.http.recent_samples_max must be > 0")
	}
	if len(c.HistogramBucketsMS) == 0 {
		return errors.New("latency.http.histogram_buckets_ms cannot be empty")
	}
	// Check for non-positive bucket values
	for _, bucket := range c.HistogramBucketsMS {
		if bucket <= 0 {
			return errors.New("latency.http.histogram_buckets_ms values must be > 0")
		}
	}
	if c.IntervalSeconds <= 0 {
		return errors.New("latency.http.interval_seconds must be > 0")
	}
	if c.TimeoutMilliseconds <= 0 {
		return errors.New("latency.http.timeout_milliseconds must be > 0")
	}
	if c.WindowSeconds <= 0 {
		return errors.New("latency.http.window_seconds must be > 0")
	}
	if c.RetainedRangeSeconds < c.WindowSeconds {
		return errors.New("latency.http.retained_range_seconds must be >= window_seconds")
	}
	// Enforce invariant: recent_samples_max * interval must cover retained range
	if c.RecentSamplesMax*c.IntervalSeconds < c.RetainedRangeSeconds {
		return errors.New("latency.http.recent_samples_max * interval_seconds must be >= retained_range_seconds")
	}
	return nil
}

// ValidateICMPProbeConfig validates the ICMP probe configuration.
func ValidateICMPProbeConfig(c ICMPProbeConfig) error {
	if c.RecentSamplesMax <= 0 {
		return errors.New("latency.icmp.recent_samples_max must be > 0")
	}
	if len(c.HistogramBucketsMS) == 0 {
		return errors.New("latency.icmp.histogram_buckets_ms cannot be empty")
	}
	// Check for non-positive bucket values
	for _, bucket := range c.HistogramBucketsMS {
		if bucket <= 0 {
			return errors.New("latency.icmp.histogram_buckets_ms values must be > 0")
		}
	}
	if c.IntervalSeconds <= 0 {
		return errors.New("latency.icmp.interval_seconds must be > 0")
	}
	if c.TimeoutSeconds <= 0 {
		return errors.New("latency.icmp.timeout_seconds must be > 0")
	}
	if c.WindowSeconds <= 0 {
		return errors.New("latency.icmp.window_seconds must be > 0")
	}
	if c.RetainedRangeSeconds < c.WindowSeconds {
		return errors.New("latency.icmp.retained_range_seconds must be >= window_seconds")
	}
	// Enforce invariant: recent_samples_max * interval must cover retained range
	if c.RecentSamplesMax*c.IntervalSeconds < c.RetainedRangeSeconds {
		return errors.New("latency.icmp.recent_samples_max * interval_seconds must be >= retained_range_seconds")
	}
	// Validate backend type
	if err := ValidateBackend(c.Backend); err != nil {
		return err
	}
	return nil
}

// IsEnabled is kept for backward compatibility - returns true if either HTTP or ICMP is enabled.
func (c *LatencyConfig) IsEnabled() bool {
	return c.HTTP.IsEnabled() || c.ICMP.IsEnabled()
}

// BackendType returns the configured ICMP backend, applying platform defaults.
// Returns the configured backend string.
func (c *ICMPProbeConfig) BackendType() string {
	if c.Backend == "" {
		return DefaultICMPBackend()
	}
	return c.Backend
}

// DefaultICMPBackend returns the default ICMP backend.
// Currently always returns native - os_ping is only used when explicitly configured.
func DefaultICMPBackend() string {
	return ICMPBackendNative
}

// ValidateBackend validates the ICMP backend configuration.
func ValidateBackend(backend string) error {
	switch backend {
	case "", ICMPBackendNative, ICMPBackendOSPing:
		return nil
	default:
		return fmt.Errorf("latency.icmp.backend must be %q or %q, got %q", ICMPBackendNative, ICMPBackendOSPing, backend)
	}
}
