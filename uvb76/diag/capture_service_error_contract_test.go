package diag

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/state"
)

// =============================================================================
// ACT-UVB76-HULK02: Capture Service Error Contract Tests
// =============================================================================
//
// These tests verify capture service error scenarios:
// - command failed (HTTP errors from peer)
// - timeout
// - target mapping missing
// - disabled
// - context canceled
// - backend errors map to canonical statuses
//
// Production behavior (source of truth):
// - HTTP non-200 -> DiagCaptureStatusError, Error set, CaptureStatus NOT set
// - Timeout -> DiagCaptureStatusTimeout, Error set, CaptureStatus NOT set
// - Disabled -> DiagCaptureStatusDisabled, no NetworkDiag, CaptureStatus NOT set
// - No peer mapping -> DiagCaptureStatusNoPeerMapping, CaptureStatus NOT set
//
// Note: CaptureStatus is only set when NetworkDiag is present (success case).
// For error cases, only Status and Error are set.
// =============================================================================

// TestCaptureServiceContract_CommandFailed verifies HTTP error responses result in error capture.
func TestCaptureServiceContract_CommandFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	cfg := testCaptureConfig(server.URL)
	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	svc.TriggerCapture("event-failed", "target-1", "http")
	captures := waitForCapture(t, store, "event-failed")

	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}

	capture := captures[0]

	// HTTP 500 maps to error status
	if capture.Status != state.DiagCaptureStatusError {
		t.Errorf("expected error status for HTTP 500, got %s", capture.Status)
	}
	if capture.Error == nil {
		t.Error("error capture must have Error")
	}
	if capture.NetworkDiag != nil {
		t.Error("error capture should not have NetworkDiag")
	}
	// Note: CaptureStatus is not set for error cases (only for success with NetworkDiag)
}

// TestCaptureServiceContract_Timeout verifies timeout scenario with bounded slow handler.
func TestCaptureServiceContract_Timeout(t *testing.T) {
	// Use bounded slow handler instead of 5-second sleep to avoid slow tests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(250 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := testCaptureConfigWithTimeout(server.URL, 50) // 50ms timeout
	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	svc.TriggerCapture("event-timeout", "target-1", "http")
	captures := waitForCapture(t, store, "event-timeout")

	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}

	capture := captures[0]

	// Timeout maps to timeout status
	if capture.Status != state.DiagCaptureStatusTimeout {
		t.Errorf("expected timeout status, got %s", capture.Status)
	}
	if capture.Error == nil {
		t.Error("timeout capture should have Error")
	}
	if capture.NetworkDiag != nil {
		t.Error("timeout capture should not have NetworkDiag")
	}
}

// TestCaptureServiceContract_TargetMappingMissing verifies target mapping missing scenario.
func TestCaptureServiceContract_TargetMappingMissing(t *testing.T) {
	cfg := testCaptureConfigWithNoPeers()
	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	svc.TriggerCapture("event-no-mapping", "unknown-target", "http")
	captures := waitForCapture(t, store, "event-no-mapping")

	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}

	capture := captures[0]

	// No mapping maps to no_peer_mapping status
	if capture.Status != state.DiagCaptureStatusNoPeerMapping {
		t.Errorf("expected no_peer_mapping status, got %s", capture.Status)
	}
	if capture.NetworkDiag != nil {
		t.Error("no mapping capture should not have NetworkDiag")
	}
}

// TestCaptureServiceContract_Disabled verifies disabled scenario.
func TestCaptureServiceContract_Disabled(t *testing.T) {
	cfg := testCaptureConfigDisabled()
	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	svc.TriggerCapture("event-disabled", "target-1", "http")
	captures := waitForCapture(t, store, "event-disabled")

	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}

	capture := captures[0]

	// Disabled maps to disabled status
	if capture.Status != state.DiagCaptureStatusDisabled {
		t.Errorf("expected disabled status, got %s", capture.Status)
	}
	if capture.NetworkDiag != nil {
		t.Error("disabled capture should not have NetworkDiag")
	}
}

// TestCaptureServiceContract_ContextCanceled verifies context cancellation (timeout-equivalent).
func TestCaptureServiceContract_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Respond slowly with context cancellation check
		select {
		case <-r.Context().Done():
			return
		case <-time.After(250 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := testCaptureConfigWithTimeout(server.URL, 50) // 50ms timeout
	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	svc.TriggerCapture("event-canceled", "target-1", "http")
	captures := waitForCapture(t, store, "event-canceled")

	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}

	capture := captures[0]

	// Context cancellation (timeout) maps to timeout status
	if capture.Status != state.DiagCaptureStatusTimeout {
		t.Errorf("expected timeout status for context cancellation, got %s", capture.Status)
	}
	if capture.Error == nil {
		t.Error("context canceled capture should have Error")
	}
	if capture.NetworkDiag != nil {
		t.Error("context canceled capture should not have NetworkDiag")
	}
}

// TestCaptureServiceContract_BackendErrorsMapToCanonicalStatuses verifies
// backend errors map to canonical statuses deterministically.
func TestCaptureServiceContract_BackendErrorsMapToCanonicalStatuses(t *testing.T) {
	testCases := []struct {
		name           string
		handler        http.HandlerFunc
		expectedStatus state.DiagCaptureStatus
	}{
		{
			name: "http_500_internal_error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			expectedStatus: state.DiagCaptureStatusError,
		},
		{
			name: "http_404_not_found",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "not found", http.StatusNotFound)
			},
			expectedStatus: state.DiagCaptureStatusError,
		},
		{
			name: "http_403_forbidden",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "forbidden", http.StatusForbidden)
			},
			expectedStatus: state.DiagCaptureStatusError,
		},
		{
			name: "http_503_service_unavailable",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			},
			expectedStatus: state.DiagCaptureStatusError,
		},
		{
			name: "malformed_json",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("not json"))
			},
			expectedStatus: state.DiagCaptureStatusError,
		},
		{
			name: "empty_body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			expectedStatus: state.DiagCaptureStatusError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(tc.handler)
			defer server.Close()

			cfg := testCaptureConfigWithTimeout(server.URL, 2000)
			store := state.NewCaptureStore()
			svc := NewCaptureService(cfg, store)

			svc.TriggerCapture("event-"+tc.name, "target-1", "http")
			captures := waitForCapture(t, store, "event-"+tc.name)

			if len(captures) != 1 {
				t.Fatalf("expected 1 capture, got %d", len(captures))
			}

			capture := captures[0]

			if capture.Status != tc.expectedStatus {
				t.Errorf("expected status %s, got %s", tc.expectedStatus, capture.Status)
			}
			if capture.Error == nil {
				t.Error("error case should have Error set")
			}
			if capture.NetworkDiag != nil {
				t.Error("error case should not have NetworkDiag")
			}
		})
	}
}
