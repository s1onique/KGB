package diag

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/s1onique/KGB/uvb76/state"
)

// =============================================================================
// ACT-UVB76-HULK02: Capture Service Success Contract Tests
// =============================================================================
//
// These tests verify capture service success scenarios:
// - success with packet
// - success with structured TCP absence
//
// =============================================================================

// TestCaptureServiceContract_SuccessWithPacket verifies success with packet scenario.
func TestCaptureServiceContract_SuccessWithPacket(t *testing.T) {
	networkDiag := `{
		"network_diag": {
			"started_at": "2026-01-01T00:00:00Z",
			"status": "ok",
			"interfaces": [],
			"routes": [],
			"underlay_tcp": [
				{
					"name": "xray",
					"state": "ESTAB",
					"local": "10.0.0.1:443",
					"remote": "10.0.0.2:443",
					"rtt_ms": 10.5,
					"status": "ok"
				}
			],
			"events": []
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

	svc.TriggerCapture("event-success", "target-1", "http")
	captures := waitForCapture(t, store, "event-success")

	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}

	capture := captures[0]

	// Success with packet maps to captured
	if capture.Status != state.DiagCaptureStatusOK {
		t.Errorf("expected ok status, got %s", capture.Status)
	}
	if capture.CaptureStatus != state.CaptureStatusCaptured {
		t.Errorf("expected captured status, got %s", capture.CaptureStatus)
	}
	if capture.NetworkDiag == nil {
		t.Error("captured capture must have NetworkDiag")
	}
	if capture.Error != nil {
		t.Error("captured capture should not have Error")
	}
}

// TestCaptureServiceContract_SuccessWithStructuredTcpAbsence verifies success with structured TCP absence.
func TestCaptureServiceContract_SuccessWithStructuredTcpAbsence(t *testing.T) {
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
					"message": "no socket found",
					"fields": "{\"reason\": \"no_matching_socket\", \"expected_peer\": \"wg0\", \"probe_kind\": \"http\"}"
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

	svc.TriggerCapture("event-absence", "target-1", "http")
	captures := waitForCapture(t, store, "event-absence")

	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}

	capture := captures[0]

	// Success with TCP absence still maps to captured (network diag is present)
	if capture.Status != state.DiagCaptureStatusOK {
		t.Errorf("expected ok status, got %s", capture.Status)
	}
	if capture.CaptureStatus != state.CaptureStatusCaptured {
		t.Errorf("expected captured status, got %s", capture.CaptureStatus)
	}
	if capture.NetworkDiag == nil {
		t.Error("captured capture must have NetworkDiag")
	}
	// TCP absence events should be extracted
	if len(capture.TcpAbsenceEvents) == 0 {
		t.Error("TCP absence events should be extracted")
	}
	if len(capture.TcpAbsenceEvents) > 0 && capture.TcpAbsenceEvents[0].ReasonCode != "no_matching_socket" {
		t.Errorf("expected reason_code 'no_matching_socket', got '%s'", capture.TcpAbsenceEvents[0].ReasonCode)
	}
}
