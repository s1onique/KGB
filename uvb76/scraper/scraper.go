// Package scraper implements the tovarisch scrape client for UVB-76.
package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// TovarischStatus represents the JSON status from tovarisch /status --json.
type TovarischStatus struct {
	Service string `json:"service"`
	Version string `json:"version"`
	NodeID  string `json:"node_id"`
	Status  string `json:"status"`
	Checks  []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Detail string `json:"detail"`
	} `json:"checks"`
	Runtime struct {
		PID    int  `json:"pid"`
		RSSKIB *int `json:"rss_kib"`
	} `json:"runtime"`
}

// Client scrapes tovarisch status endpoints.
type Client struct {
	httpClient *http.Client
	cfg        *config.ScrapeConfig
	state      *state.Manager
	targets    map[string]*config.TargetConfig // keyed by ID, immutable after creation
	mu         sync.RWMutex
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// NewClient creates a new scraper client.
func NewClient(scrapeCfg *config.ScrapeConfig, st *state.Manager, targets []*config.TargetConfig) *Client {
	client := &Client{
		httpClient: &http.Client{
			Timeout: time.Duration(scrapeCfg.TimeoutMilliseconds) * time.Millisecond,
		},
		cfg:     scrapeCfg,
		state:   st,
		targets: make(map[string]*config.TargetConfig),
		stopCh:  make(chan struct{}),
	}

	for _, t := range targets {
		client.targets[t.ID] = t
	}

	return client
}

// Start begins periodic scraping for all enabled targets.
func (c *Client) Start() {
	c.wg.Add(1)
	go c.runLoop()
}

// Stop stops the scraper.
func (c *Client) Stop() {
	close(c.stopCh)
	c.wg.Wait()
}

// runLoop periodically scrapes all enabled targets.
func (c *Client) runLoop() {
	defer c.wg.Done()
	
	// Initial scrape
	c.scrapeAll()

	ticker := time.NewTicker(time.Duration(c.cfg.IntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.scrapeAll()
		}
	}
}

// scrapeAll scrapes all enabled targets.
func (c *Client) scrapeAll() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, t := range c.targets {
		if t.Enabled {
			go c.scrapeTarget(t)
		}
	}
}

// scrapeTarget scrapes a single target and stores the result.
func (c *Client) scrapeTarget(t *config.TargetConfig) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.cfg.TimeoutMilliseconds)*time.Millisecond)
	defer cancel()

	snap := &state.TargetSnapshot{
		TargetID:  t.ID,
		ScrapedAt: time.Now().UTC(),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.BaseURL+"/status", nil)
	if err != nil {
		snap.Reachable = false
		snap.Error = fmt.Sprintf("failed to create request: %v", err)
		c.state.UpdateSnapshot(t.ID, snap)
		return
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		snap.Reachable = false
		snap.Error = fmt.Sprintf("request failed: %v", err)
		c.state.UpdateSnapshot(t.ID, snap)
		return
	}
	defer resp.Body.Close()

	snap.Reachable = true

	if resp.StatusCode != http.StatusOK {
		snap.Error = fmt.Sprintf("unexpected status code: %d", resp.StatusCode)
		c.state.UpdateSnapshot(t.ID, snap)
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024)) // 64KB limit
	if err != nil {
		snap.Error = fmt.Sprintf("failed to read response: %v", err)
		c.state.UpdateSnapshot(t.ID, snap)
		return
	}

	var status TovarischStatus
	if err := json.Unmarshal(body, &status); err != nil {
		snap.Error = fmt.Sprintf("failed to parse JSON: %v", err)
		snap.RawResponse = string(body)
		c.state.UpdateSnapshot(t.ID, snap)
		return
	}

	snap.Status = status.Status
	snap.Version = status.Version
	snap.NodeID = status.NodeID

	for _, check := range status.Checks {
		snap.Checks = append(snap.Checks, state.CheckResult{
			Name:   check.Name,
			Status: check.Status,
			Detail: check.Detail,
		})
	}

	c.state.UpdateSnapshot(t.ID, snap)
}
