package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/s1onique/KGB/uvb76/auth"
	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// =============================================================================
// ACT-UVB76-HULK02: Canonical Capture Status Contract Tests
// =============================================================================

// TestCaptureStatusAPI_CapturedEmitsCanonicalStatus verifies GET spikes with captured capture
// emits canonical "captured" status string.
func TestCaptureStatusAPI_CapturedEmitsCanonicalStatus(t *testing.T) {
	srv, st, _, token := setupCaptureTestServer(t)
	captureStore := st.GetCaptureStore()

	// Create spike with proper previous samples
	spike := st.DetectAndRecordSpike(
		"test-target", "http",
		5000.0, // High latency spike
		time.Now().UTC(),
		true,   // reachable
		nil,    // scheduler delay
		nil,    // http status
		nil,    // probe error
		createPreviousSamples(20, 50.0), // Low baseline samples
		nil,    // httpTrace
	)
	if spike == nil {
		t.Fatal("expected spike to be recorded")
	}

	// Add capture for the same event ID
	captureStore.AddCapture(spike.EventID, state.DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           state.DiagCaptureStatusOK,
		CaptureStatus:    state.CaptureStatusCaptured,
		NetworkDiag:      &state.NetworkDiagData{},
	})

	// Get spikes via API
	router := mux.NewRouter()
	router.Handle("/api/v1/latency/spikes", http.HandlerFunc(srv.handleTargetLatencySpikes))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&include_captures=true", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp SpikeResponseWithCaptures
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Spikes) == 0 {
		t.Fatal("expected at least one spike row")
	}

	foundCaptured := false
	for _, spike := range resp.Spikes {
		for _, capture := range spike.Captures {
			if capture.CaptureStatus == state.CaptureStatusCaptured {
				foundCaptured = true
				if capture.NetworkDiag == nil {
					t.Error("captured capture must have NetworkDiag")
				}
			}
		}
	}

	if !foundCaptured {
		t.Fatal("expected captured capture in API response")
	}
}

// TestCaptureStatusAPI_SkippedCooldownEmitsCanonicalStatus verifies GET spikes with skipped_cooldown
// emits canonical "skipped_cooldown" status String with cooldown metadata.
func TestCaptureStatusAPI_SkippedCooldownEmitsCanonicalStatus(t *testing.T) {
	srv, st, _, token := setupCaptureTestServer(t)
	captureStore := st.GetCaptureStore()

	now := time.Now().UTC()
	anchorTime := now.Add(-5 * time.Minute)
	spike := st.DetectAndRecordSpike(
		"test-target", "http",
		5000.0,
		now,
		true,
		nil, nil, nil,
		createPreviousSamples(20, 50.0),
		nil,
	)
	if spike == nil {
		t.Fatal("expected spike to be recorded")
	}

	captureStore.AddCapture(spike.EventID, state.DiagCapture{
		Source:               "peer-1",
		CaptureStartedAt:     now,
		Status:               state.DiagCaptureStatusOK,
		SuppressedByCooldown: true,
		CaptureStatus:        state.CaptureStatusSkippedCooldown,
		CooldownInfo: &state.CaptureCooldownInfo{
			Scope:                    "per_diagnostic_peer",
			LastSuccessfulCaptureAt:   &anchorTime,
			CooldownSeconds:          90,
			AnchorVisible:            true,
			AnchorVisibilityReason:    state.AnchorVisibilityReasonRetained,
		},
	})

	router := mux.NewRouter()
	router.Handle("/api/v1/latency/spikes", http.HandlerFunc(srv.handleTargetLatencySpikes))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&include_captures=true", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp SpikeResponseWithCaptures
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Spikes) == 0 {
		t.Fatal("expected at least one spike row")
	}

	foundSkipped := false
	for _, spike := range resp.Spikes {
		for _, capture := range spike.Captures {
			if capture.CaptureStatus == state.CaptureStatusSkippedCooldown {
				foundSkipped = true
				if capture.CooldownInfo == nil {
					t.Error("skipped_cooldown capture must have CooldownInfo")
				}
				if capture.NetworkDiag != nil {
					t.Error("skipped_cooldown capture should not have NetworkDiag")
				}
			}
		}
	}

	if !foundSkipped {
		t.Fatal("expected skipped_cooldown capture in API response")
	}
}

// TestCaptureStatusAPI_FailedEmitsCanonicalStatus verifies GET spikes with failed capture
// emits canonical "failed" status String with failure reason.
func TestCaptureStatusAPI_FailedEmitsCanonicalStatus(t *testing.T) {
	srv, st, _, token := setupCaptureTestServer(t)
	captureStore := st.GetCaptureStore()

	spike := st.DetectAndRecordSpike(
		"test-target", "http",
		5000.0,
		time.Now().UTC(),
		true, nil, nil, nil,
		createPreviousSamples(20, 50.0),
		nil,
	)
	if spike == nil {
		t.Fatal("expected spike to be recorded")
	}

	errMsg := "connection refused: dial tcp 10.0.0.5:8080: connection refused"
	captureStore.AddCapture(spike.EventID, state.DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           state.DiagCaptureStatusError,
		CaptureStatus:    state.CaptureStatusFailed,
		Error:            &errMsg,
	})

	router := mux.NewRouter()
	router.Handle("/api/v1/latency/spikes", http.HandlerFunc(srv.handleTargetLatencySpikes))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&include_captures=true", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp SpikeResponseWithCaptures
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Spikes) == 0 {
		t.Fatal("expected at least one spike row")
	}

	foundFailed := false
	for _, spike := range resp.Spikes {
		for _, capture := range spike.Captures {
			if capture.CaptureStatus == state.CaptureStatusFailed {
				foundFailed = true
				if capture.Error == nil {
					t.Error("failed capture must have Error")
				}
				if capture.NetworkDiag != nil {
					t.Error("failed capture should not have NetworkDiag")
				}
			}
		}
	}

	if !foundFailed {
		t.Fatal("expected failed capture in API response")
	}
}

// TestCaptureStatusAPI_DisabledEmitsCanonicalStatus verifies GET spikes with disabled capture
// emits canonical "disabled" status String.
func TestCaptureStatusAPI_DisabledEmitsCanonicalStatus(t *testing.T) {
	srv, st, _, token := setupCaptureTestServer(t)
	captureStore := st.GetCaptureStore()

	spike := st.DetectAndRecordSpike(
		"test-target", "http",
		5000.0,
		time.Now().UTC(),
		true, nil, nil, nil,
		createPreviousSamples(20, 50.0),
		nil,
	)
	if spike == nil {
		t.Fatal("expected spike to be recorded")
	}

	captureStore.AddCapture(spike.EventID, state.DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           state.DiagCaptureStatusDisabled,
		CaptureStatus:    state.CaptureStatusDisabled,
	})

	router := mux.NewRouter()
	router.Handle("/api/v1/latency/spikes", http.HandlerFunc(srv.handleTargetLatencySpikes))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&include_captures=true", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp SpikeResponseWithCaptures
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Spikes) == 0 {
		t.Fatal("expected at least one spike row")
	}

	foundDisabled := false
	for _, spike := range resp.Spikes {
		for _, capture := range spike.Captures {
			if capture.CaptureStatus == state.CaptureStatusDisabled {
				foundDisabled = true
			}
		}
	}

	if !foundDisabled {
		t.Fatal("expected disabled capture in API response")
	}
}

// TestCaptureStatusAPI_MissingEmitsCanonicalStatus verifies GET spikes with missing capture artifact
// emits canonical "missing" status String with missing artifact reason.
func TestCaptureStatusAPI_MissingEmitsCanonicalStatus(t *testing.T) {
	srv, st, _, token := setupCaptureTestServer(t)
	captureStore := st.GetCaptureStore()

	spike := st.DetectAndRecordSpike(
		"test-target", "http",
		5000.0,
		time.Now().UTC(),
		true, nil, nil, nil,
		createPreviousSamples(20, 50.0),
		nil,
	)
	if spike == nil {
		t.Fatal("expected spike to be recorded")
	}

	errMsg := "artifact not found: /var/log/uvb76/captures/event-missing.pcap"
	captureStore.AddCapture(spike.EventID, state.DiagCapture{
		Source:           "peer-1",
		CaptureStartedAt: time.Now().UTC(),
		Status:           state.DiagCaptureStatusError,
		CaptureStatus:    state.CaptureStatusMissing,
		Error:            &errMsg,
	})

	router := mux.NewRouter()
	router.Handle("/api/v1/latency/spikes", http.HandlerFunc(srv.handleTargetLatencySpikes))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&include_captures=true", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp SpikeResponseWithCaptures
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Spikes) == 0 {
		t.Fatal("expected at least one spike row")
	}

	foundMissing := false
	for _, spike := range resp.Spikes {
		for _, capture := range spike.Captures {
			if capture.CaptureStatus == state.CaptureStatusMissing {
				foundMissing = true
				if capture.Error == nil {
					t.Error("missing capture must have Error")
				}
			}
		}
	}

	if !foundMissing {
		t.Fatal("expected missing capture in API response")
	}
}


// =============================================================================
// Test Helpers
// =============================================================================


// =============================================================================
// Test Helpers
// =============================================================================

// createPreviousSamples creates baseline latency samples for spike detection.
func createPreviousSamples(count int, latencyMs float64) []state.LatencySample {
	samples := make([]state.LatencySample, count)
	now := time.Now().UTC()
	for i := 0; i < count; i++ {
		samples[i] = state.LatencySample{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			LatencyMs:  latencyMs,
			Reachable: true,
		}
	}
	return samples
}

// setupCaptureTestServer creates a test server with spike API and capture service.
func setupCaptureTestServer(t *testing.T) (*Server, *state.Manager, string, string) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:  config.ListenConfig{Addr: ":0"},
		Auth:    config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:  config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Latency: config.LatencyConfig{},
		Diagnostics: config.DiagnosticsConfig{
			Enabled:         true,
			CaptureOnSpike:  true,
			TimeoutMs:       2000,
			CooldownSeconds: 90,
			Peers:           []config.DiagPeerConfig{{Name: "peer-1", BaseURL: "http://localhost:8080", Targets: []string{"test-target"}}},
		},
		Targets: []config.TargetConfig{{ID: "test-target", Name: "Test Target", BaseURL: "http://localhost:8080", Enabled: true}},
	}
	cfg.Latency.ApplyDefaults()
	cfg.Diagnostics.ApplyDefaults()

	st := state.NewManager()
	srv := NewServer(cfg, st, nil, true)
	token, _ := srv.sessions.GenerateToken("admin")

	return srv, st, token, token
}
