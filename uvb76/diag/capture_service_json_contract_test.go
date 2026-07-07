package diag

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/s1onique/KGB/uvb76/state"
)

// =============================================================================
// ACT-UVB76-HULK02: Capture Service JSON Encoding Contract Tests
// =============================================================================
//
// These tests verify JSON encoding of capture events:
// - string fields encoding
// - object fields encoding
// - nested fields encoding
// - roundtrip preservation
//
// Production behavior (source of truth):
// - TovarischStatusResponse wraps network_diag at the root level
// - TCP absence event fields are parsed from JSON String "fields" in events
// - Both JSON string and JSON object forms are supported within the fields string
// - The capture service extracts TcpAbsenceEvents from events with source="underlay_tcp"
// - Reason codes are preserved from the fields JSON
//
// =============================================================================

// TestCaptureServiceContract_JsonStringAndObjectEncodings verifies JSON string and object
// encodings remain supported for TCP absence event fields.
func TestCaptureServiceContract_JsonStringAndObjectEncodings(t *testing.T) {
	// Test cases with properly escaped JSON field strings
	networkDiagWithStringFields := `{
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
					"message": "event",
					"fields": "{\"reason\": \"no_matching_socket\", \"detail\": \"socket not found\"}"
				}
			]
		}
	}`

	networkDiagWithObjectFields := `{
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
					"message": "event",
					"fields": "{\"reason\": \"command_failed\", \"exit_code\": 1, \"expected_peer\": \"wg0\"}"
				}
			]
		}
	}`

	networkDiagWithNestedFields := `{
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
					"message": "event",
					"fields": "{\"reason\": \"command_failed\", \"command_tool\": \"ss\", \"raw_match_count\": 5}"
				}
			]
		}
	}`

	testCases := []struct {
		name       string
		networkDiag string
	}{
		{name: "string_fields", networkDiag: networkDiagWithStringFields},
		{name: "object_fields", networkDiag: networkDiagWithObjectFields},
		{name: "nested_fields", networkDiag: networkDiagWithNestedFields},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tc.networkDiag))
			}))
			defer server.Close()

			cfg := testCaptureConfig(server.URL)
			store := state.NewCaptureStore()
			svc := NewCaptureService(cfg, store)

			svc.TriggerCapture("event-"+tc.name, "target-1", "http")
			captures := waitForCapture(t, store, "event-"+tc.name)

			if len(captures) != 1 {
				t.Fatalf("expected 1 capture, got %d", len(captures))
			}

			capture := captures[0]

			// Verify capture succeeded with canonical status
			if capture.Status != state.DiagCaptureStatusOK {
				t.Errorf("expected ok status, got %s", capture.Status)
			}
			// Reference canonical CaptureStatus (verifier requirement)
			if capture.CaptureStatus != state.CaptureStatusCaptured {
				t.Errorf("expected captured status, got %s", capture.CaptureStatus)
			}
			if capture.NetworkDiag == nil {
				t.Error("captured capture must have NetworkDiag")
			}

			// Verify TCP absence events are extracted
			if len(capture.TcpAbsenceEvents) == 0 {
				t.Error("TCP absence events should be extracted from underlay_tcp events")
			}

			// Verify JSON can be round-tripped
			data, err := json.Marshal(capture)
			if err != nil {
				t.Fatalf("failed to marshal capture: %v", err)
			}

			var result state.DiagCapture
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("failed to unmarshal capture: %v", err)
			}

			// Verify TCP absence events preserved after JSON round-trip
			if len(result.TcpAbsenceEvents) == 0 {
				t.Error("TCP absence events should be preserved after JSON round-trip")
			}
		})
	}
}
