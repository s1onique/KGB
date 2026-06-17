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
	"github.com/s1onique/KGB/uvb76/state"
)

// Client performs independent latency probes against tovarisch targets.
type Client struct {
	httpClient *http.Client
	cfg        *config.LatencyConfig
	state      *state.Manager
	targets    map[string]*config.TargetConfig // keyed by ID, immutable after creation
	mu         sync.RWMutex
	stopCh     chan struct{}
	wg         sync.WaitGroup
	enabled    bool
}

// NewClient creates a new probe client.
func NewClient(latencyCfg *config.LatencyConfig, st *state.Manager, targets []*config.TargetConfig) *Client {
	client := &Client{
		httpClient: &http.Client{
			Timeout: time.Duration(latencyCfg.TimeoutMilliseconds) * time.Millisecond,
		},
		cfg:     latencyCfg,
		state:   st,
		targets: make(map[string]*config.TargetConfig),
		stopCh:  make(chan struct{}),
		enabled: latencyCfg.IsEnabled(),
	}

	for _, t := range targets {
		client.targets[t.ID] = t
	}

	return client
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
func (c *Client) probeTarget(t *config.TargetConfig) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.cfg.TimeoutMilliseconds)*time.Millisecond)
	defer cancel()

	// Probe endpoint - use the same URL construction as latency series metadata
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, config.TargetStatusURL(t.BaseURL), nil)
	if err != nil {
		// Record timeout-style latency for request creation failures
		c.state.RecordLatency(t.ID, float64(c.cfg.TimeoutMilliseconds), false)
		return
	}

	// Measure request latency
	start := time.Now()
	resp, err := c.httpClient.Do(req)
	latencyMs := float64(time.Since(start).Milliseconds())

	if err != nil {
		// Record latency for failed request (still useful for timeout monitoring)
		c.state.RecordLatency(t.ID, latencyMs, false)
		return
	}
	defer resp.Body.Close()

	// Record successful latency measurement
	c.state.RecordLatency(t.ID, latencyMs, true)
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
