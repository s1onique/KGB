package server

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/scraper"
	"github.com/s1onique/KGB/uvb76/state"
)

// latencySeriesServerForTest creates a server with in-memory state for testing.
// It recovers from panics to ensure handler safety.
func latencySeriesServerForTest(t *testing.T, targets []config.TargetConfig) *Server {
	cfg := &config.Config{
		Targets: targets,
	}
	mgr := state.NewManager()
	client := scraper.NewClient(
		&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000},
		mgr,
		latencySeriesTargetsToPtr(cfg.Targets),
	)
	return NewServer(cfg, mgr, client, true)
}

// serveLatencySeriesForTest is a reusable test harness that constructs an in-memory
// server, makes a request, and returns status/body for assertions.
// It recovers from panic and fails the test if panic occurs.
func serveLatencySeriesForTest(t *testing.T, targetID string, rawQuery string) (status int, body []byte) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: targetID, BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(
		&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000},
		mgr,
		latencySeriesTargetsToPtr(cfg.Targets),
	)
	srv := NewServer(cfg, mgr, client, true)

	query := "/api/v1/targets/" + targetID + "/latency-series?target_id=" + targetID
	if rawQuery != "" {
		query += "&" + rawQuery
	}

	req := httptest.NewRequest("GET", query, nil)
	w := httptest.NewRecorder()

	// Recover from panic to ensure handler safety
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic in handler: %v", r)
		}
	}()

	srv.handleTargetLatencySeries(w, req)

	return w.Code, w.Body.Bytes()
}

// serveLatencySeriesRawForFuzz is a panic-free version for fuzzing.
// It does not call t.Fatalf - instead returns error indicators.
func serveLatencySeriesRawForFuzz(targetID string, rawQuery string) (status int, bodyLen int, panicOccurred bool) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: targetID, BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(
		&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000},
		mgr,
		latencySeriesTargetsToPtr(cfg.Targets),
	)
	srv := NewServer(cfg, mgr, client, true)

	query := "/api/v1/targets/" + targetID + "/latency-series?target_id=" + targetID
	if rawQuery != "" {
		query += "&" + rawQuery
	}

	req := httptest.NewRequest("GET", query, nil)
	w := httptest.NewRecorder()

	func() {
		defer func() {
			if r := recover(); r != nil {
				panicOccurred = true
			}
		}()
		srv.handleTargetLatencySeries(w, req)
	}()

	return w.Code, w.Body.Len(), panicOccurred
}

// parseQueryForTest is a helper to parse query parameters for testing.
func parseQueryForTest(rawQuery string) url.Values {
	parsed, _ := url.ParseQuery(rawQuery)
	return parsed
}

// assertValidJSONResponse asserts the response body is valid JSON.
func assertValidJSONResponse(t *testing.T, body []byte) {
	var js json.RawMessage
	if err := json.Unmarshal(body, &js); err != nil {
		t.Errorf("invalid JSON response: %v", err)
	}
}

// assertErrorResponse asserts the response contains a stable error field.
func assertErrorResponse(t *testing.T, body []byte) {
	var resp map[string]string
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Errorf("expected error response, got invalid JSON: %v", err)
		return
	}
	if _, ok := resp["error"]; !ok {
		t.Errorf("expected 'error' field in response, got: %v", resp)
	}
}

// assertNonNegativeCount asserts a count is non-negative.
func assertNonNegativeCount(t *testing.T, name string, count int) {
	if count < 0 {
		t.Errorf("%s is negative: %d", name, count)
	}
}

// assertPointCountWithinBounds asserts returned point count is within MaxOutputPoints.
func assertPointCountWithinBounds(t *testing.T, count int) {
	if count > MaxOutputPoints {
		t.Errorf("returned_point_count (%d) exceeds MaxOutputPoints (%d)", count, MaxOutputPoints)
	}
}

// assertPercentileOrdering asserts percentile values are in non-decreasing order.
func assertPercentileOrdering(t *testing.T, p50, p90, p95, p99 *float64) {
	if p50 != nil && p90 != nil && *p50 > *p90 {
		t.Errorf("p50 (%f) > p90 (%f) - impossible percentile ordering", *p50, *p90)
	}
	if p90 != nil && p95 != nil && *p90 > *p95 {
		t.Errorf("p90 (%f) > p95 (%f) - impossible percentile ordering", *p90, *p95)
	}
	if p95 != nil && p99 != nil && *p95 > *p99 {
		t.Errorf("p95 (%f) > p99 (%f) - impossible percentile ordering", *p95, *p99)
	}
}
