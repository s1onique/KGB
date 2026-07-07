package diag

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/state"
)

// =============================================================================
// ACT-UVB76-HULK02R4: Capture Service Error Contract Tests
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
// Layer contract (HULK02R4):
// - DiagCaptureStatus records the low-level capture operation result
// - CaptureStatus records the canonical lifecycle/projection status
// - Both are set on service-created capture rows
//
// Mapping rules:
//   DiagCaptureStatusError         -> CaptureStatusFailed
//   DiagCaptureStatusTimeout       -> CaptureStatusFailed
//   DiagCaptureStatusDisabled      -> CaptureStatusDisabled
//   DiagCaptureStatusNoPeerMapping -> CaptureStatusNotConfigured
//
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
	// HULK02R4: CaptureStatus is now set for error cases
	if capture.CaptureStatus != state.CaptureStatusFailed {
		t.Errorf("expected failed capture status, got %s", capture.CaptureStatus)
	}
	if capture.Error == nil {
		t.Error("error capture must have Error")
	}
	if capture.NetworkDiag != nil {
		t.Error("error capture should not have NetworkDiag")
	}
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
	// HULK02R4: CaptureStatus is now set for timeout cases
	if capture.CaptureStatus != state.CaptureStatusFailed {
		t.Errorf("expected failed capture status for timeout, got %s", capture.CaptureStatus)
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
	// HULK02R4: CaptureStatus is now set for no-mapping cases
	if capture.CaptureStatus != state.CaptureStatusNotConfigured {
		t.Errorf("expected not_configured capture status, got %s", capture.CaptureStatus)
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
	// HULK02R4: CaptureStatus is now set for disabled cases
	if capture.CaptureStatus != state.CaptureStatusDisabled {
		t.Errorf("expected disabled capture status, got %s", capture.CaptureStatus)
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
	// HULK02R4: CaptureStatus is now set for context canceled cases
	if capture.CaptureStatus != state.CaptureStatusFailed {
		t.Errorf("expected failed capture status for context canceled, got %s", capture.CaptureStatus)
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
		name                  string
		handler               http.HandlerFunc
		expectedDiagStatus    state.DiagCaptureStatus
		expectedCaptureStatus state.CaptureStatus
	}{
		{
			name: "http_500_internal_error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			expectedDiagStatus:    state.DiagCaptureStatusError,
			expectedCaptureStatus: state.CaptureStatusFailed,
		},
		{
			name: "http_404_not_found",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "not found", http.StatusNotFound)
			},
			expectedDiagStatus:    state.DiagCaptureStatusError,
			expectedCaptureStatus: state.CaptureStatusFailed,
		},
		{
			name: "http_403_forbidden",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "forbidden", http.StatusForbidden)
			},
			expectedDiagStatus:    state.DiagCaptureStatusError,
			expectedCaptureStatus: state.CaptureStatusFailed,
		},
		{
			name: "http_503_service_unavailable",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			},
			expectedDiagStatus:    state.DiagCaptureStatusError,
			expectedCaptureStatus: state.CaptureStatusFailed,
		},
		{
			name: "malformed_json",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("not json"))
			},
			expectedDiagStatus:    state.DiagCaptureStatusError,
			expectedCaptureStatus: state.CaptureStatusFailed,
		},
		{
			name: "empty_body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			expectedDiagStatus:    state.DiagCaptureStatusError,
			expectedCaptureStatus: state.CaptureStatusFailed,
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

			// Assert low-level DiagCaptureStatus
			if capture.Status != tc.expectedDiagStatus {
				t.Errorf("expected diag status %s, got %s", tc.expectedDiagStatus, capture.Status)
			}
			// HULK02R4: Assert canonical CaptureStatus
			if capture.CaptureStatus != tc.expectedCaptureStatus {
				t.Errorf("expected capture status %s, got %s", tc.expectedCaptureStatus, capture.CaptureStatus)
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
