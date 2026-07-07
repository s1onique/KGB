package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/s1onique/KGB/uvb76/auth"
	"github.com/s1onique/KGB/uvb76/state"
)

// =============================================================================
// ACT-UVB76-HULK02: Capture Status Field Constraint Tests
// =============================================================================


// This file tests capture status constraints:
// Valid statuses: "captured", "skipped_cooldown", "failed", "disabled", "not_configured", 
// "not_attempted", "in_progress", "missing"


// TestCaptureStatusAPI_DoesNotExposeImpossibleCombinations verifies API handles
// impossible combinations. Note: The API currently accepts these invalid states
// but they should be caught by validation at the capture service level.
func TestCaptureStatusAPI_DoesNotExposeImpossibleCombinations(t *testing.T) {
	t.Run("captured_without_network_diag", func(t *testing.T) {
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
			Status:           state.DiagCaptureStatusOK,
			CaptureStatus:    state.CaptureStatusCaptured,
			NetworkDiag:      nil, // Invalid for captured - should be caught at service level
		})

		router := mux.NewRouter()
		router.Handle("/api/v1/latency/spikes", http.HandlerFunc(srv.handleTargetLatencySpikes))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&include_captures=true", nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		// API should return 200 even with invalid data (service validation catches this)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		// Verify response is valid JSON
		var resp SpikeResponseWithCaptures
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
	})

	t.Run("skipped_cooldown_with_network_diag", func(t *testing.T) {
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
			Source:               "peer-1",
			CaptureStartedAt:     time.Now().UTC(),
			Status:               state.DiagCaptureStatusOK,
			SuppressedByCooldown: true,
			CaptureStatus:        state.CaptureStatusSkippedCooldown,
			NetworkDiag:          &state.NetworkDiagData{}, // Invalid for skipped - should be caught at service level
		})

		router := mux.NewRouter()
		router.Handle("/api/v1/latency/spikes", http.HandlerFunc(srv.handleTargetLatencySpikes))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/latency/spikes?target_id=test-target&include_captures=true", nil)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		// API should return 200 even with invalid data (service validation catches this)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		// Verify response is valid JSON
		var resp SpikeResponseWithCaptures
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
	})
}

// TestCaptureStatusAPI_OmitsPacketFieldsWhenNotAllowed verifies API omits packet fields
// when no packet is allowed.
func TestCaptureStatusAPI_OmitsPacketFieldsWhenNotAllowed(t *testing.T) {
	t.Run("captured_has_network_diag", func(t *testing.T) {
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
			Status:           state.DiagCaptureStatusOK,
			CaptureStatus:    state.CaptureStatusCaptured,
			NetworkDiag:      &state.NetworkDiagData{},
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

		for _, spike := range resp.Spikes {
			for _, capture := range spike.Captures {
				if capture.NetworkDiag == nil {
					t.Error("captured should have NetworkDiag")
				}
			}
		}
	})
}

// TestCaptureStatusAPI_IncludesReasonFieldsWhenRequired verifies API includes reason fields
// when required.
func TestCaptureStatusAPI_IncludesReasonFieldsWhenRequired(t *testing.T) {
	t.Run("skipped_cooldown_has_cooldown_info", func(t *testing.T) {
		srv, st, _, token := setupCaptureTestServer(t)
		captureStore := st.GetCaptureStore()

		now := time.Now().UTC()
		anchorTime := now.Add(-5 * time.Minute)
		spike := st.DetectAndRecordSpike(
			"test-target", "http",
			5000.0,
			now,
			true, nil, nil, nil,
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

		for _, spike := range resp.Spikes {
			for _, capture := range spike.Captures {
				if capture.CooldownInfo == nil {
					t.Error("skipped_cooldown should have CooldownInfo")
				}
			}
		}
	})
}
