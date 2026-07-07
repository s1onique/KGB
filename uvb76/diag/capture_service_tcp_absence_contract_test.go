package diag

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/s1onique/KGB/uvb76/state"
)

// =============================================================================
// ACT-UVB76-HULK02: Capture Service TCP Absence Contract Tests
// =============================================================================
//
// These tests verify TCP absence reason handling:
// - known absence reasons are preserved
// - unknown absence reason is preserved (not rejected)
//
// TCP absence reasons (canonical 8, per HULK02 contract):
// - no_matching_socket
// - socket_closed_before_capture
// - command_failed
// - not_configured
// - permission_denied
// - target_not_tcp
// - target_mapping_missing
// - unsupported_platform
//
// Production behavior (source of truth):
// - Known reasons are preserved in TcpAbsenceEvents.ReasonCode
// - Unknown reasons are also preserved (not rejected) - capture still succeeds
// - JSON parse failures result in reason_code "parse_failed"
//
// =============================================================================

// TestCaptureServiceContract_KnownAbsenceReasonsPass verifies known absence reasons are preserved.
func TestCaptureServiceContract_KnownAbsenceReasonsPass(t *testing.T) {
	allowedReasons := []string{
		"no_matching_socket",
		"socket_closed_before_capture",
		"command_failed",
		"not_configured",
		"permission_denied",
		"target_not_tcp",
		"target_mapping_missing",
		"unsupported_platform",
	}

	for _, reason := range allowedReasons {
		t.Run(reason, func(t *testing.T) {
			networkDiag := `{
				"network_diag": {
					"started_at": "2026-01-01T00:00:00Z",
					"status": "ok",
					"interfaces": [],
					"routes": [],
					"underlay_tcp": [],
					"events": [
						{
							"ts": "2026-01-01T00:00:00Z",
							"severity": "info",
							"source": "underlay_tcp",
							"message": "capture failed",
							"fields": "{\"reason\": \"` + reason + `\"}"
						}
					]
				}
			}`

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(networkDiag))
			}))
			defer server.Close()

			cfg := testCaptureConfig(server.URL)
			store := state.NewCaptureStore()
			svc := NewCaptureService(cfg, store)

			svc.TriggerCapture("event-"+reason, "target-1", "http")
			captures := waitForCapture(t, store, "event-"+reason)

			if len(captures) != 1 {
				t.Fatalf("expected 1 capture, got %d", len(captures))
			}

			capture := captures[0]

			// Known reasons should preserve reason code
			if len(capture.TcpAbsenceEvents) == 0 {
				t.Error("TCP absence events should be present")
				return
			}
			if capture.TcpAbsenceEvents[0].ReasonCode != reason {
				t.Errorf("expected reason_code '%s', got '%s'", reason, capture.TcpAbsenceEvents[0].ReasonCode)
			}
		})
	}
}

// TestCaptureServiceContract_UnknownAbsenceReasonIsPreserved verifies unknown reason is preserved.
// Production behavior: unknown reasons are NOT rejected, they are preserved with the unknown code.
// The capture still succeeds since network_diag was returned successfully.
func TestCaptureServiceContract_UnknownAbsenceReasonIsPreserved(t *testing.T) {
	networkDiag := `{
		"network_diag": {
			"started_at": "2026-01-01T00:00:00Z",
			"status": "ok",
			"interfaces": [],
			"routes": [],
			"underlay_tcp": [],
			"events": [
				{
					"ts": "2026-01-01T00:00:00Z",
					"severity": "info",
					"source": "underlay_tcp",
					"message": "unknown error",
					"fields": "{\"reason\": \"completely_unknown_reason_xyz\"}"
				}
			]
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(networkDiag))
	}))
	defer server.Close()

	cfg := testCaptureConfig(server.URL)
	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	svc.TriggerCapture("event-unknown-reason", "target-1", "http")
	captures := waitForCapture(t, store, "event-unknown-reason")

	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}

	capture := captures[0]

	// Unknown reason is preserved (not rejected) - capture still succeeded
	// since the network_diag was returned
	if capture.Status != state.DiagCaptureStatusOK {
		t.Errorf("expected ok status, got %s", capture.Status)
	}
	if capture.CaptureStatus != state.CaptureStatusCaptured {
		t.Errorf("expected captured status, got %s", capture.CaptureStatus)
	}
	if len(capture.TcpAbsenceEvents) == 0 {
		t.Error("TCP absence events should be present for unknown reason")
	}
	// Verify unknown reason is preserved
	if len(capture.TcpAbsenceEvents) > 0 && capture.TcpAbsenceEvents[0].ReasonCode != "completely_unknown_reason_xyz" {
		t.Errorf("expected unknown reason preserved, got '%s'", capture.TcpAbsenceEvents[0].ReasonCode)
	}
}
