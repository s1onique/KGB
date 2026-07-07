package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/scraper"
	"github.com/s1onique/KGB/uvb76/state"
)

// Contract tests for latency series API handler invariants (part 1 - parameter validation).
// These tests verify the runtime state invariants established by ACT-UVB76-HULK01.
// Tests exercise the server/API read path without requiring the full router runtime.

// targetsToPtr converts a slice of TargetConfig to a slice of pointers.
func targetsToPtr(targets []config.TargetConfig) []*config.TargetConfig {
	result := make([]*config.TargetConfig, len(targets))
	for i := range targets {
		result[i] = &targets[i]
	}
	return result
}

func TestLatencySeriesContract_MissingTargetID(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, targetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series", nil)
	w := httptest.NewRecorder()

	srv.handleTargetLatencySeries(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing target_id, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "target_id is required" {
		t.Errorf("expected error 'target_id is required', got %s", resp["error"])
	}
}

func TestLatencySeriesContract_RangeSecondsZero(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, targetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series?target_id=test-target&range_seconds=0", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic on range_seconds=0: %v", r)
		}
	}()

	srv.handleTargetLatencySeries(w, req)

	var series state.LatencySeries
	if err := json.NewDecoder(w.Body).Decode(&series); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}
}

func TestLatencySeriesContract_RangeSecondsNegative(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, targetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series?target_id=test-target&range_seconds=-100", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic on negative range_seconds: %v", r)
		}
	}()

	srv.handleTargetLatencySeries(w, req)

	var series state.LatencySeries
	if err := json.NewDecoder(w.Body).Decode(&series); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}
}

func TestLatencySeriesContract_RangeSecondsTooLarge(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, targetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series?target_id=test-target&range_seconds=999999999", nil)
	w := httptest.NewRecorder()

	srv.handleTargetLatencySeries(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for range_seconds too large, got %d", w.Code)
	}
}

func TestLatencySeriesContract_StepSecondsZero(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, targetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series?target_id=test-target&step_seconds=0", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic on step_seconds=0: %v", r)
		}
	}()

	srv.handleTargetLatencySeries(w, req)

	var series state.LatencySeries
	if err := json.NewDecoder(w.Body).Decode(&series); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	if series.StepSeconds < MinStepSeconds {
		t.Errorf("step_seconds clamped below minimum: %d", series.StepSeconds)
	}
}

func TestLatencySeriesContract_StepSecondsNegative(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, targetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series?target_id=test-target&step_seconds=-60", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic on negative step_seconds: %v", r)
		}
	}()

	srv.handleTargetLatencySeries(w, req)

	var series state.LatencySeries
	if err := json.NewDecoder(w.Body).Decode(&series); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}
}

func TestLatencySeriesContract_WindowSecondsZero(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, targetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series?target_id=test-target&window_seconds=0", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic on window_seconds=0: %v", r)
		}
	}()

	srv.handleTargetLatencySeries(w, req)

	var series state.LatencySeries
	if err := json.NewDecoder(w.Body).Decode(&series); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}
}

func TestLatencySeriesContract_WindowSecondsNegative(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, targetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series?target_id=test-target&window_seconds=-300", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic on negative window_seconds: %v", r)
		}
	}()

	srv.handleTargetLatencySeries(w, req)

	var series state.LatencySeries
	if err := json.NewDecoder(w.Body).Decode(&series); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}
}

func TestLatencySeriesContract_WindowSecondsGreaterThanRange(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, targetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series?target_id=test-target&range_seconds=60&window_seconds=300", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic on window > range: %v", r)
		}
	}()

	srv.handleTargetLatencySeries(w, req)

	var series state.LatencySeries
	if err := json.NewDecoder(w.Body).Decode(&series); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	if series.WindowSeconds > MaxWindowSeconds {
		t.Errorf("window_seconds exceeds max: %d", series.WindowSeconds)
	}
}

