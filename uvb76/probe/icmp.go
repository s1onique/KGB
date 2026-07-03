// Package probe implements independent latency probing for UVB-76.
package probe

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/diag"
	"github.com/s1onique/KGB/uvb76/internal/uvb76/domain"
	"github.com/s1onique/KGB/uvb76/state"
)

// ICMPSample represents a single ICMP ping latency measurement.
type ICMPSample struct {
	Timestamp time.Time `json:"timestamp"`
	LatencyMs float64   `json:"latency_ms"`
	Reachable bool      `json:"reachable"`
	Error     string    `json:"error,omitempty"`
}

// ICMPSampleKind is the probe kind identifier for ICMP samples.
const ICMPSampleKind = "icmp"

// ICMPSampleKindHTTP is the probe kind identifier for HTTP samples.
const ICMPSampleKindHTTP = "http"

// MaxICMPSpikeDetectionSamples is the maximum number of samples used for spike detection
// in the hot ICMP probe path.
//
// This is a SMALL BOUNDED WINDOW intentionally smaller than the full retention window
// (3600 samples). Using a bounded window in the per-second probe hot path avoids:
// - Excessive allocation churn on constrained routers (ARM, ~128MB RAM)
// - Potential heap/memory corruption from concurrent makeslice on every probe tick
// - Unnecessary CPU overhead copying large slices on each ICMP tick
//
// The spike detector only needs MinSamplesForMedian=20 samples to compute a reliable
// rolling median. We use 120 to provide headroom for edge cases and future tuning.
// The full 3600-sample history remains available for UI/API reads via
// GetRecentICMPLatencySamples with explicit limit.
const MaxICMPSpikeDetectionSamples = 120

// ICMPSampleRecorder is the interface for recording ICMP samples and detecting spikes.
// Note: Raw []state.LatencySample is NOT exposed outside uvb76/state - only domain.SampleWindow.
type ICMPSampleRecorder interface {
	RecordICMPLatency(targetID string, latencyMs float64, reachable bool)
	GetICMPSampleWindow(targetID string, limit int) domain.SampleWindow
	DetectAndRecordSpikeWithWindow(targetID, kind string, latencyMs float64, sampleTs time.Time, reachable bool, schedulerDelayMs *float64, httpStatus *int, probeError *string, previousWindow domain.SampleWindow, httpTrace *state.HTTPTrace) *state.SpikeEvent
}

// ICMPClient performs independent ICMP ping probes against tovarisch targets.
type ICMPClient struct {
	backend      ICMPProbeBackend
	cfg          *config.ICMPProbeConfig
	state        ICMPSampleRecorder
	targets      map[string]*config.TargetConfig // keyed by ID, immutable after creation
	diagCapture *diag.CaptureService // optional diagnostic capture service
	mu           sync.RWMutex
	stopCh       chan struct{}
	wg           sync.WaitGroup
	enabled      bool
	// Per-target in-flight probe guard to prevent overlapping probes
	inFlight map[string]bool
}

// Global native ICMP telemetry pointer.
// This pointer is set to the actual backend's stats when a native backend is created.
var globalNativeICMPStats *nativeICMPStatsInternal

// Global native telemetry mutex for lazy initialization.
var globalNativeTelemetryMu sync.Mutex

// InitGlobalNativeICMPTelemetry initializes the global native ICMP telemetry.
// Takes a stats pointer from the actual backend to ensure /status reads real data.
func InitGlobalNativeICMPTelemetry(stats *nativeICMPStatsInternal) {
	globalNativeTelemetryMu.Lock()
	defer globalNativeTelemetryMu.Unlock()
	globalNativeICMPStats = stats
}

// GetGlobalNativeICMPTelemetry returns a telemetry wrapper around the global stats pointer.
// Returns nil if no native ICMP backend has been initialized.
func GetGlobalNativeICMPTelemetry() *NativeICMPTelemetry {
	globalNativeTelemetryMu.Lock()
	defer globalNativeTelemetryMu.Unlock()
	if globalNativeICMPStats == nil {
		return nil
	}
	return &NativeICMPTelemetry{stats: globalNativeICMPStats}
}

// ResetGlobalNativeICMPTelemetry resets the global telemetry (for testing).
func ResetGlobalNativeICMPTelemetry() {
	globalNativeTelemetryMu.Lock()
	stats := globalNativeICMPStats
	globalNativeTelemetryMu.Unlock()

	if stats == nil {
		return
	}

	stats.sent.Store(0)
	stats.received.Store(0)
	stats.timeouts.Store(0)
	stats.socketOpenErrors.Store(0)
	stats.permissionErrors.Store(0)
	stats.parseErrors.Store(0)
	stats.unmatchedReplies.Store(0)
	stats.lastRTTMillis.Store(0)
	stats.lastErrorClass.Store("")
}

// NewICMPClient creates a new ICMP probe client.
func NewICMPClient(cfg *config.ICMPProbeConfig, st ICMPSampleRecorder, targets []*config.TargetConfig) (*ICMPClient, error) {
	var backend ICMPProbeBackend
	var backendErr error

	if cfg.IsEnabled() {
		backendType := cfg.BackendType()

		switch backendType {
		case config.ICMPBackendNative:
			// Try native ICMP backend first
			native, err := NewNativeICMPBackend()
			if err != nil {
				// Native ICMP failed - this is a startup error, not a silent fallback
				// Return the error so the caller can decide how to handle it
				backendErr = fmt.Errorf("native ICMP backend unavailable: %w; set icmp.backend=os_ping to use legacy fallback explicitly", err)
				return nil, backendErr
			}
			backend = native

		case config.ICMPBackendOSPing:
			// Explicitly use OS ping
			backend = NewOSPingBackendWithLimit(cfg.MaxConcurrentOSPing)

		default:
			// Unknown backend type - this shouldn't happen with validation, but be defensive
			backendErr = fmt.Errorf("unknown ICMP backend type: %q", backendType)
			return nil, backendErr
		}
	}

	client := &ICMPClient{
		backend:  backend,
		cfg:      cfg,
		state:    st,
		targets:  make(map[string]*config.TargetConfig),
		stopCh:   make(chan struct{}),
		enabled:  cfg.IsEnabled(),
		inFlight: make(map[string]bool),
	}

	for _, t := range targets {
		client.targets[t.ID] = t
	}

	return client, nil
}

// IsEnabled returns whether ICMP probing is enabled.
func (c *ICMPClient) IsEnabled() bool {
	return c.enabled
}

// Start begins periodic ICMP probing for all enabled targets.
// Does nothing if probing is disabled or no backend is available.
func (c *ICMPClient) Start() {
	if !c.enabled || c.backend == nil {
		return
	}

	c.wg.Add(1)
	go c.runLoop()
}

// Stop stops the ICMP probe client.
// Safe to call even if probing is disabled.
func (c *ICMPClient) Stop() {
	close(c.stopCh)
	c.wg.Wait()
}

// runLoop periodically probes all enabled targets with ICMP ping.
func (c *ICMPClient) runLoop() {
	defer c.wg.Done()

	// Initial probe
	c.probeAll()

	ticker := time.NewTicker(time.Duration(c.cfg.IntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.probeAll()
		}
	}
}

// probeAll probes all enabled targets.
// Uses per-target in-flight guard to prevent overlapping probes.
func (c *ICMPClient) probeAll() {
	if !c.enabled {
		return
	}

	c.mu.Lock()
	for _, t := range c.targets {
		if t.Enabled && !c.inFlight[t.ID] {
			c.inFlight[t.ID] = true
			go func(targetID string) {
				c.probeTarget(targetID)
				c.mu.Lock()
				c.inFlight[targetID] = false
				c.mu.Unlock()
			}(t.ID)
		}
	}
	c.mu.Unlock()
}

// probeTarget performs a single ICMP ping against a target.
// It does NOT update snapshots - only records latency measurements.
// Note: Caller is responsible for managing in-flight state.
func (c *ICMPClient) probeTarget(targetID string) {
	c.mu.RLock()
	t, ok := c.targets[targetID]
	c.mu.RUnlock()
	if !ok || !t.Enabled {
		return
	}

	// Get previous sample window BEFORE recording (for spike detection)
	// Use the BOUNDED SPIKE WINDOW to avoid allocation churn on every ICMP tick.
	// MaxICMPSpikeDetectionSamples=120 is intentionally smaller than the full
	// retention window (3600 samples) to reduce:
	// - Memory allocation pressure on constrained routers (ARM, ~128MB RAM)
	// - Potential SIGSEGV from concurrent makeslice during heap pressure
	// - CPU overhead from copying large slices on each probe tick
	//
	// The spike detector needs only MinSamplesForMedian=20 samples for reliable
	// rolling median. 120 provides comfortable headroom.
	//
	// Full history is still available for UI/API via GetRecentICMPLatencySamples.
	previousWindow := c.state.GetICMPSampleWindow(t.ID, MaxICMPSpikeDetectionSamples)

	// Extract hostname from base URL
	host := extractHost(t.BaseURL)
	if host == "" {
		sampleTs := time.Now().UTC()
		c.state.RecordICMPLatency(t.ID, float64(c.cfg.TimeoutSeconds*1000), false)
		errStr := "failed to extract host from base_url"
		if spike := c.state.DetectAndRecordSpikeWithWindow(t.ID, "icmp", float64(c.cfg.TimeoutSeconds*1000), sampleTs, false, nil, nil, &errStr, previousWindow, nil); spike != nil {
			c.triggerDiagCapture(spike.EventID, t.ID, "icmp")
		}
		return
	}

	timeout := time.Duration(c.cfg.TimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout+time.Second) // extra second for process overhead
	defer cancel()

	latency, err := c.backend.Ping(ctx, host, timeout)
	// Use float division to preserve sub-millisecond precision
	latencyMs := float64(latency) / float64(time.Millisecond)
	sampleTs := time.Now().UTC()

	if err != nil {
		// Record failure - timeout or unreachable
		c.state.RecordICMPLatency(t.ID, latencyMs, false)
		errStr := fmt.Sprintf("ping failed: %v", err)
		if spike := c.state.DetectAndRecordSpikeWithWindow(t.ID, "icmp", latencyMs, sampleTs, false, nil, nil, &errStr, previousWindow, nil); spike != nil {
			c.triggerDiagCapture(spike.EventID, t.ID, "icmp")
		}
		return
	}

	// Record successful latency measurement
	c.state.RecordICMPLatency(t.ID, latencyMs, true)

	// Spike detection for successful ICMP
	if spike := c.state.DetectAndRecordSpikeWithWindow(t.ID, "icmp", latencyMs, sampleTs, true, nil, nil, nil, previousWindow, nil); spike != nil {
		c.triggerDiagCapture(spike.EventID, t.ID, "icmp")
	}
}

// ICMPProbeResult contains detailed probe results for diagnostics.
type ICMPProbeResult struct {
	TargetID   string    `json:"target_id"`
	Timestamp  time.Time `json:"timestamp"`
	LatencyMs  float64   `json:"latency_ms"`
	Reachable  bool      `json:"reachable"`
	Error      string    `json:"error,omitempty"`
}

// ProbeTarget performs a single blocking ICMP probe and returns detailed results.
// Useful for on-demand probing or testing.
func (c *ICMPClient) ProbeTarget(t *config.TargetConfig) ICMPProbeResult {
	result := ICMPProbeResult{
		TargetID:  t.ID,
		Timestamp: time.Now().UTC(),
	}

	// Extract hostname from base URL
	host := extractHost(t.BaseURL)
	if host == "" {
		result.LatencyMs = float64(c.cfg.TimeoutSeconds * 1000)
		result.Reachable = false
		result.Error = "failed to extract host from base_url"
		c.state.RecordICMPLatency(t.ID, result.LatencyMs, false)
		return result
	}

	timeout := time.Duration(c.cfg.TimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout+time.Second)
	defer cancel()

	latency, err := c.backend.Ping(ctx, host, timeout)
	// Use float division to preserve sub-millisecond precision
	result.LatencyMs = float64(latency) / float64(time.Millisecond)

	if err != nil {
		result.Reachable = false
		result.Error = fmt.Sprintf("ping failed: %v", err)
		c.state.RecordICMPLatency(t.ID, result.LatencyMs, false)
		return result
	}

	result.Reachable = true
	c.state.RecordICMPLatency(t.ID, result.LatencyMs, true)
	return result
}

// SetDiagCapture sets the diagnostic capture service for spike-triggered captures.
func (c *ICMPClient) SetDiagCapture(diagCapture *diag.CaptureService) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.diagCapture = diagCapture
}

// triggerDiagCapture triggers async diagnostic capture if diagCapture is configured.
// This does not block the probe loop.
func (c *ICMPClient) triggerDiagCapture(eventID, targetID, probeKind string) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.diagCapture != nil {
		c.diagCapture.TriggerCapture(eventID, targetID, probeKind)
	}
}

// extractHost extracts the hostname/IP from a base URL for ICMP ping.
// http://10.149.149.1:8317/status -> 10.149.149.1
// https://example.com:8443/status -> example.com
func extractHost(baseURL string) string {
	// Parse the URL
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// GetNativeICMPStats returns the native ICMP backend's stats if using native backend.
// This allows main.go to wire the actual backend stats to the global telemetry.
func (c *ICMPClient) GetNativeICMPStats() *nativeICMPStatsInternal {
	if native, ok := c.backend.(*NativeICMPBackend); ok {
		return native.stats
	}
	return nil
}
