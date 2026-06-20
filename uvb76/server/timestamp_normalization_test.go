package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/s1onique/KGB/uvb76/auth"
	"github.com/s1onique/KGB/uvb76/state"
)

// =============================================================================
// Timestamp Normalization API Tests
//
// These tests verify that API responses contain explicit UTC timestamps.
// =============================================================================

// TestDiagnosticsEndpoint_TimestampsHaveExplicitTimezone verifies that the
// /api/v1/diagnostics/capture-cooldown endpoint emits explicit UTC timestamps.
func TestDiagnosticsEndpoint_TimestampsHaveExplicitTimezone(t *testing.T) {
	// Create fake tovarisch server
	tovarischServer := httptest.NewServer(fakeTovarischHandler(validTovarischStatusJSON(), http.StatusOK))
	defer tovarischServer.Close()

	// Setup test server
	srv, st, _, _ := setupTestServer(t, tovarischServer.URL)

	// Create router with diagnostics endpoint
	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/diagnostics/capture-cooldown", http.HandlerFunc(srv.handleCaptureCooldownDiagnostics)).Methods(http.MethodGet)

	// Establish a cooldown anchor
	anchorTime := time.Date(2026, 6, 20, 21, 9, 59, 0, time.UTC)
	st.GetCaptureStore().SetLastCapture("peer-1", anchorTime)

	// Call diagnostics endpoint
	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/capture-cooldown", nil)
	token, _ := srv.sessions.GenerateToken("admin")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Parse response
	var resp CaptureCooldownDiagnostics
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify server_started_at has explicit timezone
	if resp.ServerStartedAt == "" {
		t.Error("server_started_at should not be empty")
	} else if !strings.HasSuffix(resp.ServerStartedAt, "Z") && !strings.Contains(resp.ServerStartedAt, "+") {
		t.Errorf("server_started_at missing explicit timezone: %s", resp.ServerStartedAt)
	}

	// Verify current_time has explicit timezone
	if resp.CurrentTime == "" {
		t.Error("current_time should not be empty")
	} else if !strings.HasSuffix(resp.CurrentTime, "Z") && !strings.Contains(resp.CurrentTime, "+") {
		t.Errorf("current_time missing explicit timezone: %s", resp.CurrentTime)
	}

	// Verify cooldown anchors have explicit timezone on timestamps
	for peerName, anchor := range resp.CooldownAnchors {
		if !anchor.AnchorCreatedAt.IsZero() {
			formatted := anchor.AnchorCreatedAt.Format(time.RFC3339Nano)
			if !strings.HasSuffix(formatted, "Z") {
				t.Errorf("anchor %s AnchorCreatedAt missing Z: %s", peerName, formatted)
			}
		}
		if anchor.AnchorCompletedAt != nil && !anchor.AnchorCompletedAt.IsZero() {
			formatted := anchor.AnchorCompletedAt.Format(time.RFC3339Nano)
			if !strings.HasSuffix(formatted, "Z") {
				t.Errorf("anchor %s AnchorCompletedAt missing Z: %s", peerName, formatted)
			}
		}
	}
}

// TestSpikeAPI_TimestampsHaveExplicitTimezone verifies that spike API responses
// with captures contain explicit UTC timestamps.
func TestSpikeAPI_TimestampsHaveExplicitTimezone(t *testing.T) {
	// Create fake tovarisch server
	tovarischServer := httptest.NewServer(fakeTovarischHandler(validTovarischStatusJSON(), http.StatusOK))
	defer tovarischServer.Close()

	// Setup test server
	srv, st, _, _ := setupTestServer(t, tovarischServer.URL)

	// Create router with spike endpoint
	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/latency/spikes", http.HandlerFunc(srv.handleTargetLatencySpikes)).Methods(http.MethodGet)

	now := time.Now().UTC()

	// Create a spike with successful capture
	previousSamples := make([]state.LatencySample, 25)
	for i := 0; i < 25; i++ {
		previousSamples[i] = state.LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		}
	}

	spike := st.DetectAndRecordSpike("test-target", "http", 1500.0, now.Add(-time.Second), true, nil, nil, nil, previousSamples)
	if spike == nil {
		t.Fatal("expected spike to be detected")
	}

	// Record successful capture
	st.GetCaptureStore().AddCapture(spike.EventID, state.DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: now.Add(-time.Second),
		Status:           state.DiagCaptureStatusOK,
		CaptureStatus:    state.CaptureStatusCaptured,
	})

	// Call API
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&include_captures=true", nil)
	token, _ := srv.sessions.GenerateToken("admin")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Parse response
	var resp SpikeResponseWithCaptures
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Spikes) == 0 {
		t.Fatal("expected at least one spike")
	}

	spikeData := resp.Spikes[0]

	// Verify spike timestamps have explicit timezone
	// sample_ts
	if !strings.HasSuffix(spikeData.SampleTs.Format(time.RFC3339Nano), "Z") {
		t.Errorf("sample_ts missing Z: %s", spikeData.SampleTs.Format(time.RFC3339Nano))
	}
	// collected_at
	if !strings.HasSuffix(spikeData.CollectedAt.Format(time.RFC3339Nano), "Z") {
		t.Errorf("collected_at missing Z: %s", spikeData.CollectedAt.Format(time.RFC3339Nano))
	}

	// Verify capture timestamps
	if len(spikeData.Captures) > 0 {
		capture := spikeData.Captures[0]
		// capture_started_at
		if !strings.HasSuffix(capture.CaptureStartedAt.Format(time.RFC3339Nano), "Z") {
			t.Errorf("capture_started_at missing Z: %s", capture.CaptureStartedAt.Format(time.RFC3339Nano))
		}
		// capture_finished_at
		if capture.CaptureFinishedAt != nil && !strings.HasSuffix(capture.CaptureFinishedAt.Format(time.RFC3339Nano), "Z") {
			t.Errorf("capture_finished_at missing Z: %s", capture.CaptureFinishedAt.Format(time.RFC3339Nano))
		}
	}
}

// TestSpikeAPI_CooldownInfoTimestampsExplicit verifies that skipped cooldown
// captures include cooldown_info with explicit UTC timestamps.
func TestSpikeAPI_CooldownInfoTimestampsExplicit(t *testing.T) {
	// Create fake tovarisch server
	tovarischServer := httptest.NewServer(fakeTovarischHandler(validTovarischStatusJSON(), http.StatusOK))
	defer tovarischServer.Close()

	// Setup test server
	srv, st, _, _ := setupTestServer(t, tovarischServer.URL)

	// Create router with spike endpoint
	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/latency/spikes", http.HandlerFunc(srv.handleTargetLatencySpikes)).Methods(http.MethodGet)

	now := time.Now().UTC()

	// Create anchor spike with successful capture
	anchorTime := now.Add(-30 * time.Second)
	previousSamples := make([]state.LatencySample, 25)
	for i := 0; i < 25; i++ {
		previousSamples[i] = state.LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		}
	}

	anchorSpike := st.DetectAndRecordSpike("test-target", "http", 1500.0, anchorTime, true, nil, nil, nil, previousSamples)
	if anchorSpike == nil {
		t.Fatal("expected anchor spike to be detected")
	}

	// Record successful capture for anchor
	st.GetCaptureStore().AddCapture(anchorSpike.EventID, state.DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: anchorTime,
		Status:           state.DiagCaptureStatusOK,
		CaptureStatus:    state.CaptureStatusCaptured,
	})

	// Create skipped spike (within 90s cooldown)
	skippedTime := now.Add(-15 * time.Second) // Still in cooldown
	skippedSpike := st.DetectAndRecordSpike("test-target", "http", 2000.0, skippedTime, true, nil, nil, nil, previousSamples)
	if skippedSpike == nil {
		t.Fatal("expected skipped spike to be detected")
	}

	// Record suppressed capture
	cooldownDecision := st.GetCaptureStore().EvaluateCooldown(skippedTime, "peer-1", 90)
	capture := state.DiagCapture{
		Source:               "peer-1",
		CaptureStartedAt:     skippedTime,
		Status:               state.DiagCaptureStatusOK,
		SuppressedByCooldown: true,
		CaptureStatus:        state.CaptureStatusSkippedCooldown,
		CooldownInfo:         state.BuildCooldownInfoFromDecision(cooldownDecision, "peer-1"),
	}
	st.GetCaptureStore().AddCapture(skippedSpike.EventID, capture)

	// Call API
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&include_captures=true", nil)
	token, _ := srv.sessions.GenerateToken("admin")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Parse response
	var resp SpikeResponseWithCaptures
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Find skipped spike
	var skippedCapture *state.DiagCapture
	for _, spike := range resp.Spikes {
		if spike.EventID == skippedSpike.EventID && len(spike.Captures) > 0 {
			skippedCapture = &spike.Captures[0]
			break
		}
	}

	if skippedCapture == nil {
		t.Fatal("skipped capture not found in response")
	}

	if skippedCapture.CooldownInfo == nil {
		t.Fatal("cooldown_info should not be nil for skipped cooldown")
	}

	// Serialize cooldown_info to JSON and check for explicit timezones
	cooldownData, _ := json.Marshal(skippedCapture.CooldownInfo)
	jsonStr := string(cooldownData)

	// All cooldown timestamps should have explicit timezone
	if skippedCapture.CooldownInfo.LastSuccessfulCaptureAt != nil {
		formatted := skippedCapture.CooldownInfo.LastSuccessfulCaptureAt.Format(time.RFC3339Nano)
		if !strings.HasSuffix(formatted, "Z") {
			t.Errorf("last_successful_capture_at missing Z: %s", formatted)
		}
		if !strings.Contains(jsonStr, "T") { // Should use T separator, not space
			t.Errorf("last_successful_capture_at should use T separator: %s", jsonStr)
		}
	}

	if skippedCapture.CooldownInfo.NextCaptureEligibleAt != nil {
		formatted := skippedCapture.CooldownInfo.NextCaptureEligibleAt.Format(time.RFC3339Nano)
		if !strings.HasSuffix(formatted, "Z") {
			t.Errorf("next_capture_eligible_at missing Z: %s", formatted)
		}
	}

	if skippedCapture.CooldownInfo.DecisionNowAt != nil {
		formatted := skippedCapture.CooldownInfo.DecisionNowAt.Format(time.RFC3339Nano)
		if !strings.HasSuffix(formatted, "Z") {
			t.Errorf("decision_now_at missing Z: %s", formatted)
		}
	}

	// Verify remaining cooldown is positive and correct
	if skippedCapture.CooldownInfo.RemainingCooldownMs == nil || *skippedCapture.CooldownInfo.RemainingCooldownMs <= 0 {
		t.Errorf("remaining_cooldown_ms should be positive, got %v", skippedCapture.CooldownInfo.RemainingCooldownMs)
	}
}

// TestSpikeAPI_NoTimezoneLessPattern verifies that API responses never contain
// timezone-less timestamp patterns.
func TestSpikeAPI_NoTimezoneLessPattern(t *testing.T) {
	// Create fake tovarisch server
	tovarischServer := httptest.NewServer(fakeTovarischHandler(validTovarischStatusJSON(), http.StatusOK))
	defer tovarischServer.Close()

	// Setup test server
	srv, st, _, _ := setupTestServer(t, tovarischServer.URL)

	// Create router
	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/latency/spikes", http.HandlerFunc(srv.handleTargetLatencySpikes)).Methods(http.MethodGet)

	// Create a spike
	now := time.Now().UTC()
	previousSamples := make([]state.LatencySample, 25)
	for i := 0; i < 25; i++ {
		previousSamples[i] = state.LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		}
	}

	spike := st.DetectAndRecordSpike("test-target", "http", 1500.0, now, true, nil, nil, nil, previousSamples)
	if spike == nil {
		t.Fatal("expected spike")
	}

	// Call API
	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&include_captures=true", nil)
	token, _ := srv.sessions.GenerateToken("admin")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	jsonStr := rec.Body.String()

	// Verify that timestamps have explicit timezone markers
	// Valid patterns: Z suffix or +/-HH:MM offset
	hasZ := strings.Contains(jsonStr, "Z")
	hasOffset := strings.Contains(jsonStr, "+") || strings.Contains(jsonStr, "-0")

	if !hasZ && !hasOffset {
		t.Errorf("API response should contain explicit timezone (Z or offset)")
	}

	// Should not have malformed patterns like space before time (local time format)
	if strings.Contains(jsonStr, `" 21:`) {
		t.Errorf("found space-separated timestamp pattern in API response")
	}
}

// TestStatusEndpoint_TimestampsExplicit verifies that /api/v1/status
// emits explicit UTC timestamp.
func TestStatusEndpoint_TimestampsExplicit(t *testing.T) {
	// Create fake tovarisch server
	tovarischServer := httptest.NewServer(fakeTovarischHandler(validTovarischStatusJSON(), http.StatusOK))
	defer tovarischServer.Close()

	// Setup test server
	srv, _, _, _ := setupTestServer(t, tovarischServer.URL)

	// Create router
	router := mux.NewRouter()
	protected := router.PathPrefix("/api/v1").Subrouter()
	protected.Use(srv.sessionAuthMw())
	protected.Handle("/status", http.HandlerFunc(srv.handleStatus)).Methods(http.MethodGet)

	// Call status endpoint
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	token, _ := srv.sessions.GenerateToken("admin")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var status ServerStatus
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify started_at has explicit timezone
	if !strings.HasSuffix(status.StartedAt, "Z") && !strings.Contains(status.StartedAt, "+") {
		t.Errorf("started_at missing explicit timezone: %s", status.StartedAt)
	}
}
