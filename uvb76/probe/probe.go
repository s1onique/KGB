// Package probe implements independent latency probing for UVB-76.
// Probes run on their own cadence independent from the status scraper.
package probe

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/diag"
	"github.com/s1onique/KGB/uvb76/state"
)

// Client performs independent HTTP latency probes against tovarisch targets.
type Client struct {
	httpClient     *http.Client
	cfg            *config.HTTPProbeConfig
	state          *state.Manager
	targets        map[string]*config.TargetConfig // keyed by ID, immutable after creation
	diagCapture    *diag.CaptureService // optional diagnostic capture service
	mu             sync.RWMutex
	stopCh         chan struct{}
	wg             sync.WaitGroup
	enabled        bool
	// lastReachability tracks the previous reachability state per target for recovery detection
	// Uses tri-state (unknown/reachable/unreachable) to avoid false-positive recovery
	lastReachability map[string]state.ReachabilityState
}

// NewClient creates a new HTTP probe client.
func NewClient(httpCfg *config.HTTPProbeConfig, st *state.Manager, targets []*config.TargetConfig) *Client {
	client := &Client{
		httpClient: &http.Client{
			Timeout: time.Duration(httpCfg.TimeoutMilliseconds) * time.Millisecond,
		},
		cfg:                 httpCfg,
		state:               st,
		targets:             make(map[string]*config.TargetConfig),
		stopCh:              make(chan struct{}),
		enabled:             httpCfg.IsEnabled(),
		lastReachability:    make(map[string]state.ReachabilityState),
	}

	for _, t := range targets {
		client.targets[t.ID] = t
	}

	return client
}

// SetDiagCapture sets the diagnostic capture service for spike-triggered captures.
func (c *Client) SetDiagCapture(diagCapture *diag.CaptureService) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.diagCapture = diagCapture
}

// IsEnabled returns whether probing is enabled.
func (c *Client) IsEnabled() bool {
	return c.enabled
}

// Start begins periodic probing for all enabled targets.
// Does nothing if probing is disabled.
func (c *Client) Start() {
	if !c.enabled {
		return
	}

	c.wg.Add(1)
	go c.runLoop()
}

// Stop stops the probe client.
// Safe to call even if probing is disabled.
func (c *Client) Stop() {
	close(c.stopCh)
	c.wg.Wait()
}

// runLoop periodically probes all enabled targets.
func (c *Client) runLoop() {
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
func (c *Client) probeAll() {
	if !c.enabled {
		return
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, t := range c.targets {
		if t.Enabled {
			go c.probeTarget(t)
		}
	}
}

// probeTarget performs a single latency probe against a target.
// It does NOT update snapshots - only records latency measurements.
// Triggers diagnostic capture asynchronously if spike is detected.
func (c *Client) probeTarget(t *config.TargetConfig) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.cfg.TimeoutMilliseconds)*time.Millisecond)
	defer cancel()

	// Get previous samples BEFORE recording (for spike detection)
	// Use RecentSamplesMax from config, with a sensible minimum for spike detection
	maxSamples := c.cfg.RecentSamplesMax
	if maxSamples < 30 {
		maxSamples = 30
	}
	previousSamples := c.state.GetRecentLatencySamples(t.ID, maxSamples)

	// Probe endpoint - use the same URL construction as latency series metadata
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, config.TargetStatusURL(t.BaseURL), nil)
	if err != nil {
		// Record timeout-style latency for request creation failures
		sampleTs := time.Now().UTC()
		c.state.RecordLatency(t.ID, float64(c.cfg.TimeoutMilliseconds), false)
		// Spike detection for failed request
		var errStr string = fmt.Sprintf("request creation failed: %v", err)
		if spike := c.state.DetectAndRecordSpike(t.ID, "http", float64(c.cfg.TimeoutMilliseconds), sampleTs, false, nil, nil, &errStr, previousSamples); spike != nil {
			c.triggerDiagCapture(spike.EventID, t.ID)
		}
		return
	}

	// Measure request latency
	start := time.Now()
	resp, err := c.httpClient.Do(req)
	latencyMs := float64(time.Since(start).Milliseconds())
	sampleTs := time.Now().UTC()

	if err != nil {
		// Record latency for failed request (still useful for timeout monitoring)
		c.state.RecordLatency(t.ID, latencyMs, false)
		
		// Track last reachability state for recovery detection
		c.mu.Lock()
		c.lastReachability[t.ID] = state.ReachabilityUnreachable
		c.mu.Unlock()
		
		// Spike detection for failed request - this now creates diagnostic events for failures
		errStr := fmt.Sprintf("request failed: %v", err)
		if spike := c.state.DetectAndRecordSpike(t.ID, "http", latencyMs, sampleTs, false, nil, nil, &errStr, previousSamples); spike != nil {
			c.triggerDiagCapture(spike.EventID, t.ID)
		}
		return
	}
	defer resp.Body.Close()

	// Get HTTP status code for recording
	var httpStatus *int
	if resp != nil {
		httpStatus = &resp.StatusCode
	}

	// Check for recovery: was previously unreachable, now succeeding
	c.mu.Lock()
	wasUnreachable := c.lastReachability[t.ID] == state.ReachabilityUnreachable
	c.lastReachability[t.ID] = state.ReachabilityReachable
	c.mu.Unlock()

	// Record successful latency measurement
	c.state.RecordLatency(t.ID, latencyMs, true)

	// Spike detection for successful request (latency spikes)
	if spike := c.state.DetectAndRecordSpike(t.ID, "http", latencyMs, sampleTs, true, nil, httpStatus, nil, previousSamples); spike != nil {
		c.triggerDiagCapture(spike.EventID, t.ID)
	}
	
	// Recovery detection: if we were unreachable and now succeeded, trigger a recovery capture
	// Use dedicated RecordRecoveryEvent() instead of fake error injection
	if wasUnreachable {
		if recoverySpike := c.state.RecordRecoveryEvent(t.ID, "http", latencyMs, sampleTs, httpStatus, previousSamples); recoverySpike != nil {
			c.triggerDiagCapture(recoverySpike.EventID, t.ID)
		}
	}
}

// triggerDiagCapture triggers async diagnostic capture if diagCapture is configured.
// This does not block the probe loop.
func (c *Client) triggerDiagCapture(eventID, targetID string) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.diagCapture != nil {
		c.diagCapture.TriggerCapture(eventID, targetID)
	}
}

// ProbeResult contains detailed probe results for diagnostics.
type ProbeResult struct {
	TargetID   string    `json:"target_id"`
	Timestamp  time.Time `json:"timestamp"`
	LatencyMs  float64   `json:"latency_ms"`
	Reachable  bool      `json:"reachable"`
	StatusCode int       `json:"status_code,omitempty"`
	Error      string    `json:"error,omitempty"`
}

// ProbeTarget performs a single blocking probe and returns detailed results.
// Useful for on-demand probing or testing.
func (c *Client) ProbeTarget(t *config.TargetConfig) ProbeResult {
	result := ProbeResult{
		TargetID:  t.ID,
		Timestamp: time.Now().UTC(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.cfg.TimeoutMilliseconds)*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, config.TargetStatusURL(t.BaseURL), nil)
	if err != nil {
		result.LatencyMs = float64(c.cfg.TimeoutMilliseconds)
		result.Reachable = false
		result.Error = fmt.Sprintf("failed to create request: %v", err)
		c.state.RecordLatency(t.ID, result.LatencyMs, false)
		return result
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	result.LatencyMs = float64(time.Since(start).Milliseconds())

	if err != nil {
		result.Reachable = false
		result.Error = fmt.Sprintf("request failed: %v", err)
		c.state.RecordLatency(t.ID, result.LatencyMs, false)
		return result
	}
	defer resp.Body.Close()

	result.Reachable = true
	result.StatusCode = resp.StatusCode

	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Sprintf("unexpected status code: %d", resp.StatusCode)
	}

	c.state.RecordLatency(t.ID, result.LatencyMs, true)
	return result
}
