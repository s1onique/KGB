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

// Contract tests for latency series API handler invariants (part 2 - data/state validation).
// These tests verify the runtime state invariants established by ACT-UVB76-HULK01.

func latencySeriesTargetsToPtr(targets []config.TargetConfig) []*config.TargetConfig {
	result := make([]*config.TargetConfig, len(targets))
	for i := range targets {
		result[i] = &targets[i]
	}
	return result
}

func TestLatencySeriesContract_TargetHasNoSamples(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, latencySeriesTargetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series?target_id=test-target", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic on target with no samples: %v", r)
		}
	}()

	srv.handleTargetLatencySeries(w, req)

	var series state.LatencySeries
	if err := json.NewDecoder(w.Body).Decode(&series); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	if series.ReturnedPointCount < 0 {
		t.Errorf("returned_point_count is negative: %d", series.ReturnedPointCount)
	}
	if series.RetainedSampleCount < 0 {
		t.Errorf("retained_sample_count is negative: %d", series.RetainedSampleCount)
	}
}

func TestLatencySeriesContract_TargetNotFound(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, latencySeriesTargetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	req := httptest.NewRequest("GET", "/api/v1/targets/nonexistent/latency-series?target_id=nonexistent", nil)
	w := httptest.NewRecorder()

	srv.handleTargetLatencySeries(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent target, got %d", w.Code)
	}
}

func TestLatencySeriesContract_InvalidProbeKind(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, latencySeriesTargetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series?target_id=test-target&probe_kind=invalid", nil)
	w := httptest.NewRecorder()

	srv.handleTargetLatencySeries(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid probe_kind, got %d", w.Code)
	}
}

func TestLatencySeriesContract_ImpossiblePercentileOutput(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, latencySeriesTargetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	for i := 0; i < 100; i++ {
		mgr.RecordLatency("test-target", float64(i+1), true)
	}

	req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series?target_id=test-target", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic on percentile calculation: %v", r)
		}
	}()

	srv.handleTargetLatencySeries(w, req)

	var series state.LatencySeries
	if err := json.NewDecoder(w.Body).Decode(&series); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	for _, point := range series.Points {
		if point.P50Ms != nil && point.P90Ms != nil {
			if *point.P50Ms > *point.P90Ms {
				t.Errorf("p50 (%f) > p90 (%f) - impossible percentile ordering", *point.P50Ms, *point.P90Ms)
			}
		}
		if point.P90Ms != nil && point.P95Ms != nil {
			if *point.P90Ms > *point.P95Ms {
				t.Errorf("p90 (%f) > p95 (%f) - impossible percentile ordering", *point.P90Ms, *point.P95Ms)
			}
		}
		if point.P95Ms != nil && point.P99Ms != nil {
			if *point.P95Ms > *point.P99Ms {
				t.Errorf("p95 (%f) > p99 (%f) - impossible percentile ordering", *point.P95Ms, *point.P99Ms)
			}
		}

		if point.SampleCount < 0 {
			t.Errorf("sample_count is negative: %d", point.SampleCount)
		}
		if point.ErrorCount < 0 {
			t.Errorf("error_count is negative: %d", point.ErrorCount)
		}
	}
}

func TestLatencySeriesContract_ReturnedPointCountNonNegative(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, latencySeriesTargetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	for i := 0; i < 50; i++ {
		mgr.RecordLatency("test-target", float64(i), true)
	}

	req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series?target_id=test-target&range_seconds=3600&step_seconds=60", nil)
	w := httptest.NewRecorder()

	srv.handleTargetLatencySeries(w, req)

	var series state.LatencySeries
	if err := json.NewDecoder(w.Body).Decode(&series); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	if series.ReturnedPointCount < 0 {
		t.Errorf("returned_point_count is negative: %d", series.ReturnedPointCount)
	}

	if len(series.Points) != series.ReturnedPointCount {
		t.Errorf("returned_point_count (%d) != len(points) (%d)", series.ReturnedPointCount, len(series.Points))
	}

	if series.ReturnedPointCount > MaxOutputPoints {
		t.Errorf("returned_point_count (%d) exceeds max (%d)", series.ReturnedPointCount, MaxOutputPoints)
	}
}

func TestLatencySeriesContract_ErrorCountNeverNegative(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, latencySeriesTargetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	for i := 0; i < 50; i++ {
		reachable := (i % 3) != 0
		mgr.RecordLatency("test-target", float64(i), reachable)
	}

	req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series?target_id=test-target", nil)
	w := httptest.NewRecorder()

	srv.handleTargetLatencySeries(w, req)

	var series state.LatencySeries
	if err := json.NewDecoder(w.Body).Decode(&series); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	for i, point := range series.Points {
		if point.ErrorCount < 0 {
			t.Errorf("point[%d] error_count is negative: %d", i, point.ErrorCount)
		}
		if point.SampleCount < 0 {
			t.Errorf("point[%d] sample_count is negative: %d", i, point.SampleCount)
		}
		if point.ErrorCount > point.SampleCount {
			t.Errorf("point[%d] error_count (%d) > sample_count (%d)", i, point.ErrorCount, point.SampleCount)
		}
	}
}

func TestLatencySeriesContract_EmptyDataProducesValidResponse(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "empty-target", BaseURL: "http://empty.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, latencySeriesTargetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	req := httptest.NewRequest("GET", "/api/v1/targets/empty-target/latency-series?target_id=empty-target", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic on empty data: %v", r)
		}
	}()

	srv.handleTargetLatencySeries(w, req)

	var series state.LatencySeries
	if err := json.NewDecoder(w.Body).Decode(&series); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	if series.TargetID != "empty-target" {
		t.Errorf("expected target_id 'empty-target', got %s", series.TargetID)
	}
	if series.ReturnedPointCount < 0 {
		t.Errorf("returned_point_count is negative: %d", series.ReturnedPointCount)
	}
	if series.RetainedSampleCount < 0 {
		t.Errorf("retained_sample_count is negative: %d", series.RetainedSampleCount)
	}
}

func TestLatencySeriesContract_InvalidParamsRejectedConsistently(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, latencySeriesTargetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	testCases := []struct {
		name         string
		queryParams  string
		expectStatus int
	}{
		{"invalid_probe_kind", "target_id=test-target&probe_kind=tcp", 400},
		{"nonexistent_target", "target_id=nonexistent", 404},
		{"range_too_large", "target_id=test-target&range_seconds=999999", 400},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series?"+tc.queryParams, nil)
			w := httptest.NewRecorder()

			srv.handleTargetLatencySeries(w, req)

			if w.Code != tc.expectStatus {
				t.Errorf("expected status %d for %s, got %d", tc.expectStatus, tc.name, w.Code)
			}
		})
	}
}

