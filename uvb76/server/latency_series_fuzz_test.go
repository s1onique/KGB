package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/scraper"
	"github.com/s1onique/KGB/uvb76/state"
)

// Fuzz tests for latency series query parameter handling.
// These tests exercise range/step/window/probe_kind/target_id combinations
// to ensure the handler never panics and returns valid responses.

// FuzzLatencySeriesQueryParams exercises query parameter combinations.
// Uses proper URL construction to avoid URL corruption from fuzz inputs.
func FuzzLatencySeriesQueryParams(f *testing.F) {
	// Seed corpus with known problematic and valid inputs
	f.Add("range_seconds=0")
	f.Add("range_seconds=-1")
	f.Add("range_seconds=1")
	f.Add("range_seconds=60")
	f.Add("range_seconds=3600")
	f.Add("range_seconds=14400")
	f.Add("range_seconds=999999999")
	f.Add("range_seconds=abc")
	f.Add("step_seconds=0")
	f.Add("step_seconds=-1")
	f.Add("step_seconds=1")
	f.Add("step_seconds=60")
	f.Add("step_seconds=999999999")
	f.Add("window_seconds=0")
	f.Add("window_seconds=-1")
	f.Add("window_seconds=1")
	f.Add("window_seconds=60")
	f.Add("window_seconds=999999999")
	f.Add("probe_kind=http")
	f.Add("probe_kind=icmp")
	f.Add("probe_kind=tcp")
	f.Add("probe_kind=")
	f.Add("target_id=")
	f.Add("target_id=test-target")
	f.Add("range_seconds=0&step_seconds=0&window_seconds=0")
	f.Add("range_seconds=-1&step_seconds=-1&window_seconds=-1")
	f.Add("range_seconds=abc&step_seconds=xyz&window_seconds=oops&probe_kind=tcp")

	f.Fuzz(func(t *testing.T, rawQuery string) {
		cfg := &config.Config{
			Targets: []config.TargetConfig{
				{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
			},
		}
		mgr := state.NewManager()
		client := scraper.NewClient(
			&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000},
			mgr,
			latencySeriesTargetsToPtr(cfg.Targets),
		)
		srv := NewServer(cfg, mgr, client, true)

		// Use proper URL construction to avoid corruption from fuzz inputs
		values := url.Values{}
		values.Set("target_id", "test-target")
		
		// Parse the raw query string into values
		if rawQuery != "" {
			if parsed, err := url.ParseQuery(rawQuery); err == nil {
				for k, v := range parsed {
					values.Set(k, v[0]) // Use first value to avoid duplicates
				}
			}
		}

		req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series?"+values.Encode(), nil)
		w := httptest.NewRecorder()

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic in handler: %v", r)
				}
			}()
			srv.handleTargetLatencySeries(w, req)
		}()

		// No HTTP 500 for client-controlled bad input
		if w.Code == http.StatusInternalServerError {
			t.Errorf("got 500 for query: %s", rawQuery)
		}

		// Valid JSON response for all handled requests
		if w.Code != http.StatusInternalServerError {
			var js json.RawMessage
			if err := json.Unmarshal(w.Body.Bytes(), &js); err != nil {
				t.Errorf("invalid JSON response for query %s: %v", rawQuery, err)
			}
		}
	})
}

// FuzzLatencySeriesWindowStepRange exercises window/step/range combinations.
// Uses proper URL construction to avoid URL corruption from fuzz inputs.
func FuzzLatencySeriesWindowStepRange(f *testing.F) {
	// Seed corpus with edge case combinations
	f.Add("range_seconds=0&step_seconds=0&window_seconds=0")
	f.Add("range_seconds=1&step_seconds=1&window_seconds=1")
	f.Add("range_seconds=3600&step_seconds=1&window_seconds=1")
	f.Add("range_seconds=3600&step_seconds=60&window_seconds=300")
	f.Add("range_seconds=3600&step_seconds=3600&window_seconds=3600")
	f.Add("range_seconds=86400&step_seconds=1&window_seconds=1")
	f.Add("range_seconds=86400&step_seconds=60&window_seconds=3600")
	f.Add("range_seconds=86400&step_seconds=3600&window_seconds=3600")
	f.Add("range_seconds=60&window_seconds=300") // window > range
	f.Add("range_seconds=1&window_seconds=3600") // extreme window > range

	f.Fuzz(func(t *testing.T, rawQuery string) {
		cfg := &config.Config{
			Targets: []config.TargetConfig{
				{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
			},
		}
		mgr := state.NewManager()
		client := scraper.NewClient(
			&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000},
			mgr,
			latencySeriesTargetsToPtr(cfg.Targets),
		)
		srv := NewServer(cfg, mgr, client, true)

		// Use proper URL construction to avoid corruption from fuzz inputs
		values := url.Values{}
		values.Set("target_id", "test-target")
		
		// Parse the raw query string into values
		if rawQuery != "" {
			if parsed, err := url.ParseQuery(rawQuery); err == nil {
				for k, v := range parsed {
					values.Set(k, v[0]) // Use first value to avoid duplicates
				}
			}
		}

		req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series?"+values.Encode(), nil)
		w := httptest.NewRecorder()

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic in handler: %v", r)
				}
			}()
			srv.handleTargetLatencySeries(w, req)
		}()

		// No HTTP 500 for client-controlled bad input
		if w.Code == http.StatusInternalServerError {
			t.Errorf("got 500 for query: %s", rawQuery)
		}

		// Valid JSON response for non-500 requests
		if w.Code != http.StatusInternalServerError {
			var series state.LatencySeries
			if err := json.Unmarshal(w.Body.Bytes(), &series); err != nil {
				t.Errorf("invalid JSON for query %s: %v", rawQuery, err)
				return
			}

			// Response body size remains bounded
			if w.Body.Len() > 1<<20 { // 1MB limit
				t.Errorf("response body too large for query %s: %d bytes", rawQuery, w.Body.Len())
			}

			// Successful response counts are non-negative
			if series.ReturnedPointCount < 0 {
				t.Errorf("negative returned_point_count for query %s: %d", rawQuery, series.ReturnedPointCount)
			}
			if series.RetainedSampleCount < 0 {
				t.Errorf("negative retained_sample_count for query %s: %d", rawQuery, series.RetainedSampleCount)
			}

			// Successful response point count <= MaxOutputPoints
			if series.ReturnedPointCount > MaxOutputPoints {
				t.Errorf("returned_point_count (%d) exceeds MaxOutputPoints (%d) for query %s",
					series.ReturnedPointCount, MaxOutputPoints, rawQuery)
			}

			// Verify all points have non-negative counts
			for i, point := range series.Points {
				if point.SampleCount < 0 {
					t.Errorf("point[%d] negative sample_count: %d for query %s", i, point.SampleCount, rawQuery)
				}
				if point.ErrorCount < 0 {
					t.Errorf("point[%d] negative error_count: %d for query %s", i, point.ErrorCount, rawQuery)
				}
				if point.ErrorCount > point.SampleCount {
					t.Errorf("point[%d] error_count (%d) > sample_count (%d) for query %s",
						i, point.ErrorCount, point.SampleCount, rawQuery)
				}

				// Percentile ordering
				assertPercentileOrderingFuzz(t, rawQuery, i, point.P50Ms, point.P90Ms, point.P95Ms, point.P99Ms)
			}
		}
	})
}

func assertPercentileOrderingFuzz(t *testing.T, query string, idx int, p50, p90, p95, p99 *float64) {
	if p50 != nil && p90 != nil && *p50 > *p90 {
		t.Errorf("point[%d] p50 (%f) > p90 (%f) for query %s", idx, *p50, *p90, query)
	}
	if p90 != nil && p95 != nil && *p90 > *p95 {
		t.Errorf("point[%d] p90 (%f) > p95 (%f) for query %s", idx, *p90, *p95, query)
	}
	if p95 != nil && p99 != nil && *p95 > *p99 {
		t.Errorf("point[%d] p95 (%f) > p99 (%f) for query %s", idx, *p95, *p99, query)
	}
}

// TestLatencySeriesFuzzCorpus runs the seed corpus as deterministic tests.
func TestLatencySeriesFuzzCorpus(t *testing.T) {
	corpus := []struct {
		name string
		query string
	}{
		{"zero_params", "range_seconds=0&step_seconds=0&window_seconds=0"},
		{"negative_params", "range_seconds=-1&step_seconds=-1&window_seconds=-1"},
		{"min_params", "range_seconds=1&step_seconds=1&window_seconds=1"},
		{"typical_params", "range_seconds=3600&step_seconds=60&window_seconds=300"},
		{"max_params", "range_seconds=86400&step_seconds=3600&window_seconds=3600"},
		{"window_gt_range", "range_seconds=60&window_seconds=300"},
		{"extreme_window", "range_seconds=1&window_seconds=3600"},
		{"non_numeric_range", "range_seconds=abc"},
		{"non_numeric_step", "step_seconds=xyz"},
		{"non_numeric_window", "window_seconds=oops"},
		{"invalid_probe_kind", "probe_kind=tcp"},
		{"all_malformed", "range_seconds=abc&step_seconds=xyz&window_seconds=oops&probe_kind=tcp"},
		{"empty_target_id", "target_id="},
	}

	for _, tc := range corpus {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				Targets: []config.TargetConfig{
					{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
				},
			}
			mgr := state.NewManager()
			client := scraper.NewClient(
				&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000},
				mgr,
				latencySeriesTargetsToPtr(cfg.Targets),
			)
			srv := NewServer(cfg, mgr, client, true)

			query := fmt.Sprintf("/api/v1/targets/test-target/latency-series?target_id=test-target&%s", tc.query)

			req := httptest.NewRequest("GET", query, nil)
			w := httptest.NewRecorder()

			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic for %s: %v", tc.name, r)
				}
			}()

			srv.handleTargetLatencySeries(w, req)

			// No 500 errors
			if w.Code == http.StatusInternalServerError {
				t.Errorf("got 500 for %s", tc.name)
			}

			// Valid JSON
			if w.Code != http.StatusInternalServerError {
				var series state.LatencySeries
				if err := json.Unmarshal(w.Body.Bytes(), &series); err != nil {
					t.Errorf("invalid JSON for %s: %v", tc.name, err)
				}
			}
		})
	}
}
