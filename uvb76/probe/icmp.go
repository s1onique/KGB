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

// ICMPSampleRecorder is the interface for recording ICMP samples and detecting spikes.
type ICMPSampleRecorder interface {
	RecordICMPLatency(targetID string, latencyMs float64, reachable bool)
	GetRecentICMPLatencySamples(targetID string, limit int) []state.LatencySample
	DetectAndRecordSpike(targetID, kind string, latencyMs float64, sampleTs time.Time, reachable bool, schedulerDelayMs *float64, httpStatus *int, probeError *string, previousSamples []state.LatencySample) *state.SpikeEvent
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

// NewICMPClient creates a new ICMP probe client.
func NewICMPClient(cfg *config.ICMPProbeConfig, st ICMPSampleRecorder, targets []*config.TargetConfig) *ICMPClient {
	var backend ICMPProbeBackend
	if cfg.IsEnabled() {
		backend = NewOSPingBackend()
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

	return client
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

	// Get previous samples BEFORE recording (for spike detection)
	// Use RecentSamplesMax from config, with a sensible minimum for spike detection
	maxSamples := c.cfg.RecentSamplesMax
	if maxSamples < 30 {
		maxSamples = 30
	}
	previousSamples := c.state.GetRecentICMPLatencySamples(t.ID, maxSamples)

	// Extract hostname from base URL
	host := extractHost(t.BaseURL)
	if host == "" {
		sampleTs := time.Now().UTC()
		c.state.RecordICMPLatency(t.ID, float64(c.cfg.TimeoutSeconds*1000), false)
		errStr := "failed to extract host from base_url"
		if spike := c.state.DetectAndRecordSpike(t.ID, "icmp", float64(c.cfg.TimeoutSeconds*1000), sampleTs, false, nil, nil, &errStr, previousSamples); spike != nil {
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
		if spike := c.state.DetectAndRecordSpike(t.ID, "icmp", latencyMs, sampleTs, false, nil, nil, &errStr, previousSamples); spike != nil {
			c.triggerDiagCapture(spike.EventID, t.ID, "icmp")
		}
		return
	}

	// Record successful latency measurement
	c.state.RecordICMPLatency(t.ID, latencyMs, true)

	// Spike detection for successful ICMP
	if spike := c.state.DetectAndRecordSpike(t.ID, "icmp", latencyMs, sampleTs, true, nil, nil, nil, previousSamples); spike != nil {
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
