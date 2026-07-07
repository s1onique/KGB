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

// Contract tests for latency series query parameter validation.
// These tests verify the query boundary contract defined in ACT-UVB76-HULK03.
//
// Query Boundary Contract:
//   target_id missing        -> 400
//   target_id empty          -> 400
//   target_id unknown        -> 404
//   probe_kind missing       -> default "http"
//   probe_kind invalid       -> 400
//   range_seconds missing    -> default 3600
//   range_seconds <= 0       -> use default or reject
//   range_seconds too large  -> 400
//   step_seconds <= 0        -> clamp to MinStepSeconds or reject
//   step_seconds too large   -> 400
//   window_seconds <= 0      -> clamp to DefaultWindowSeconds
//   window_seconds > MaxWindowSeconds -> clamp to MaxWindowSeconds

func TestQueryContract_MissingTargetID(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, latencySeriesTargetsToPtr(cfg.Targets))
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

func TestQueryContract_EmptyTargetID(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, latencySeriesTargetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	// Construct URL with empty target_id - note that the query path uses a different target
	req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series?target_id=", nil)
	w := httptest.NewRecorder()

	srv.handleTargetLatencySeries(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty target_id, got %d", w.Code)
	}
	assertErrorResponse(t, w.Body.Bytes())
}

func TestQueryContract_UnknownTargetID(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, latencySeriesTargetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series?target_id=nonexistent", nil)
	w := httptest.NewRecorder()

	srv.handleTargetLatencySeries(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown target, got %d", w.Code)
	}
}

func TestQueryContract_MissingProbeKind(t *testing.T) {
	status, body := serveLatencySeriesForTest(t, "test-target", "")
	if status != http.StatusOK {
		t.Errorf("expected 200 for missing probe_kind, got %d", status)
	}
	assertValidJSONResponse(t, body)

	var series state.LatencySeries
	json.Unmarshal(body, &series)
	if series.ProbeKind != "http" {
		t.Errorf("expected default probe_kind=http, got %s", series.ProbeKind)
	}
}

func TestQueryContract_InvalidProbeKind(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.TargetConfig{
			{ID: "test-target", BaseURL: "http://test.local", Enabled: true},
		},
	}
	mgr := state.NewManager()
	client := scraper.NewClient(&config.ScrapeConfig{IntervalSeconds: 60, TimeoutMilliseconds: 5000}, mgr, latencySeriesTargetsToPtr(cfg.Targets))
	srv := NewServer(cfg, mgr, client, true)

	testKinds := []string{"tcp", "udp", "dns", "invalid", "HTTP", "ICMP"}
	for _, kind := range testKinds {
		req := httptest.NewRequest("GET", "/api/v1/targets/test-target/latency-series?target_id=test-target&probe_kind="+kind, nil)
		w := httptest.NewRecorder()

		srv.handleTargetLatencySeries(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for probe_kind=%s, got %d", kind, w.Code)
		}
	}
}

func TestQueryContract_ValidProbeKinds(t *testing.T) {
	testKinds := []string{"http", "icmp"}
	for _, kind := range testKinds {
		status, body := serveLatencySeriesForTest(t, "test-target", "probe_kind="+kind)
		if status != http.StatusOK {
			t.Errorf("expected 200 for probe_kind=%s, got %d", kind, status)
		}
		assertValidJSONResponse(t, body)
	}
}

func TestQueryContract_RangeSecondsMissing(t *testing.T) {
	status, body := serveLatencySeriesForTest(t, "test-target", "")
	if status != http.StatusOK {
		t.Errorf("expected 200 for missing range_seconds, got %d", status)
	}
	assertValidJSONResponse(t, body)
}

func TestQueryContract_RangeSecondsZero(t *testing.T) {
	status, body := serveLatencySeriesForTest(t, "test-target", "range_seconds=0")
	// Zero range should be treated as default (not negative, just invalid)
	if status != http.StatusOK {
		t.Errorf("expected 200 for range_seconds=0, got %d", status)
	}
	assertValidJSONResponse(t, body)
}

func TestQueryContract_RangeSecondsNegative(t *testing.T) {
	status, body := serveLatencySeriesForTest(t, "test-target", "range_seconds=-100")
	if status != http.StatusOK {
		t.Errorf("expected 200 for negative range_seconds, got %d", status)
	}
	assertValidJSONResponse(t, body)
}

func TestQueryContract_RangeSecondsTooLarge(t *testing.T) {
	status, body := serveLatencySeriesForTest(t, "test-target", "range_seconds=999999999")
	if status != http.StatusBadRequest {
		t.Errorf("expected 400 for range_seconds too large, got %d", status)
	}
	assertErrorResponse(t, body)
}

func TestQueryContract_RangeSecondsAtMax(t *testing.T) {
	status, body := serveLatencySeriesForTest(t, "test-target", "range_seconds=86400")
	if status != http.StatusOK {
		t.Errorf("expected 200 for range_seconds=MaxRangeSeconds, got %d", status)
	}
	assertValidJSONResponse(t, body)
}

func TestQueryContract_RangeSecondsNonNumeric(t *testing.T) {
	status, body := serveLatencySeriesForTest(t, "test-target", "range_seconds=abc")
	if status != http.StatusOK {
		t.Errorf("expected 200 for non-numeric range_seconds, got %d", status)
	}
	assertValidJSONResponse(t, body)
}

func TestQueryContract_StepSecondsZero(t *testing.T) {
	status, body := serveLatencySeriesForTest(t, "test-target", "step_seconds=0")
	if status != http.StatusOK {
		t.Errorf("expected 200 for step_seconds=0, got %d", status)
	}
	assertValidJSONResponse(t, body)

	var series state.LatencySeries
	json.Unmarshal(body, &series)
	if series.StepSeconds < MinStepSeconds {
		t.Errorf("step_seconds clamped below minimum: %d", series.StepSeconds)
	}
}

func TestQueryContract_StepSecondsNegative(t *testing.T) {
	status, body := serveLatencySeriesForTest(t, "test-target", "step_seconds=-60")
	if status != http.StatusOK {
		t.Errorf("expected 200 for negative step_seconds, got %d", status)
	}
	assertValidJSONResponse(t, body)
}

func TestQueryContract_StepSecondsTooLarge(t *testing.T) {
	status, body := serveLatencySeriesForTest(t, "test-target", "step_seconds=999999999")
	if status != http.StatusBadRequest {
		t.Errorf("expected 400 for step_seconds too large, got %d", status)
	}
	assertErrorResponse(t, body)
}

func TestQueryContract_StepSecondsAtMax(t *testing.T) {
	status, body := serveLatencySeriesForTest(t, "test-target", "step_seconds=3600")
	if status != http.StatusOK {
		t.Errorf("expected 200 for step_seconds=MaxStepSeconds, got %d", status)
	}
	assertValidJSONResponse(t, body)
}

func TestQueryContract_StepSecondsNonNumeric(t *testing.T) {
	status, body := serveLatencySeriesForTest(t, "test-target", "step_seconds=abc")
	if status != http.StatusOK {
		t.Errorf("expected 200 for non-numeric step_seconds, got %d", status)
	}
	assertValidJSONResponse(t, body)
}

func TestQueryContract_WindowSecondsZero(t *testing.T) {
	status, body := serveLatencySeriesForTest(t, "test-target", "window_seconds=0")
	if status != http.StatusOK {
		t.Errorf("expected 200 for window_seconds=0, got %d", status)
	}
	assertValidJSONResponse(t, body)
}

func TestQueryContract_WindowSecondsNegative(t *testing.T) {
	status, body := serveLatencySeriesForTest(t, "test-target", "window_seconds=-300")
	if status != http.StatusOK {
		t.Errorf("expected 200 for negative window_seconds, got %d", status)
	}
	assertValidJSONResponse(t, body)
}

func TestQueryContract_WindowSecondsGreaterThanRange(t *testing.T) {
	status, body := serveLatencySeriesForTest(t, "test-target", "range_seconds=60&window_seconds=300")
	if status != http.StatusOK {
		t.Errorf("expected 200 for window > range, got %d", status)
	}
	assertValidJSONResponse(t, body)

	var series state.LatencySeries
	json.Unmarshal(body, &series)
	if series.WindowSeconds > MaxWindowSeconds {
		t.Errorf("window_seconds exceeds max: %d", series.WindowSeconds)
	}
}

func TestQueryContract_WindowSecondsAtMax(t *testing.T) {
	status, body := serveLatencySeriesForTest(t, "test-target", "window_seconds=3600")
	if status != http.StatusOK {
		t.Errorf("expected 200 for window_seconds=MaxWindowSeconds, got %d", status)
	}
	assertValidJSONResponse(t, body)
}

func TestQueryContract_WindowSecondsNonNumeric(t *testing.T) {
	status, body := serveLatencySeriesForTest(t, "test-target", "window_seconds=abc")
	if status != http.StatusOK {
		t.Errorf("expected 200 for non-numeric window_seconds, got %d", status)
	}
	assertValidJSONResponse(t, body)
}

func TestQueryContract_AllParamsMalformedTogether(t *testing.T) {
	status, body := serveLatencySeriesForTest(t, "test-target", "range_seconds=abc&step_seconds=xyz&window_seconds=oops&probe_kind=tcp")
	if status != http.StatusBadRequest {
		t.Errorf("expected 400 for all malformed params, got %d", status)
	}
	assertErrorResponse(t, body)
}

func TestQueryContract_ParseLatencySeriesQuery_Success(t *testing.T) {
	values := parseQueryForTest("target_id=test&probe_kind=http&range_seconds=3600&step_seconds=60&window_seconds=300")
	q, err := ParseLatencySeriesQuery(values)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if q.TargetID != "test" {
		t.Errorf("expected TargetID=test, got %s", q.TargetID)
	}
}

func TestQueryContract_ParseLatencySeriesQuery_MissingTargetID(t *testing.T) {
	values := parseQueryForTest("")
	_, err := ParseLatencySeriesQuery(values)
	if err != ErrTargetIDRequired {
		t.Errorf("expected ErrTargetIDRequired, got %v", err)
	}
}

func TestQueryContract_ParseLatencySeriesQuery_InvalidProbeKind(t *testing.T) {
	values := parseQueryForTest("target_id=test&probe_kind=tcp")
	_, err := ParseLatencySeriesQuery(values)
	if err != ErrInvalidProbeKind {
		t.Errorf("expected ErrInvalidProbeKind, got %v", err)
	}
}

func TestQueryContract_ParseLatencySeriesQuery_RangeExceedsMax(t *testing.T) {
	values := parseQueryForTest("target_id=test&range_seconds=999999999")
	_, err := ParseLatencySeriesQuery(values)
	if err != ErrRangeExceedsMaximum {
		t.Errorf("expected ErrRangeExceedsMaximum, got %v", err)
	}
}

func TestQueryContract_ParseLatencySeriesQuery_StepExceedsMax(t *testing.T) {
	values := parseQueryForTest("target_id=test&step_seconds=999999999")
	_, err := ParseLatencySeriesQuery(values)
	if err != ErrStepExceedsMaximum {
		t.Errorf("expected ErrStepExceedsMaximum, got %v", err)
	}
}
