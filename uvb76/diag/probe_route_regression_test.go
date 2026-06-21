// Package diag implements diagnostic capture for UVB-76.
// This file provides regression tests for probe_route in diagnostic packets.
package diag

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/state"
)

func TestDiagCapture_WithProbeRoute_Serializes(t *testing.T) {
	mtu := 1420
	capture := state.DiagCapture{
		Source:           "test-peer",
		BaseURL:          "http://10.77.0.2:8317/status",
		CaptureStartedAt: time.Now().UTC(),
		Status:           state.DiagCaptureStatusOK,
		CaptureStatus:    state.CaptureStatusCaptured,
		ProbeRoute: &state.ProbeRoute{
			Kind:         state.ProbeRouteKindHTTP,
			ProbeHost:    "10.77.0.2",
			LookupTarget: "10.77.0.2",
			Ok:           true,
			RouteType:    "unicast",
			Interface:    "wgc1",
			SourceIP:     "redacted",
			Gateway:      "redacted",
			Table:        "main",
			MTU:          &mtu,
			UID:          nil,
			CollectedAt:  "2026-06-21T16:00:00Z",
		},
	}

	data, err := json.Marshal(capture)
	if err != nil {
		t.Fatalf("failed to marshal DiagCapture: %v", err)
	}

	// Verify probe_route is in the JSON
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	probeRoute, ok := decoded["probe_route"].(map[string]interface{})
	if !ok {
		t.Fatal("probe_route not found in JSON output")
	}

	if probeRoute["kind"] != "http" {
		t.Errorf("expected kind 'http', got '%v'", probeRoute["kind"])
	}
	if probeRoute["interface"] != "wgc1" {
		t.Errorf("expected interface 'wgc1', got '%v'", probeRoute["interface"])
	}
	if probeRoute["source_ip"] != "redacted" {
		t.Errorf("expected source_ip 'redacted', got '%v'", probeRoute["source_ip"])
	}
	if probeRoute["mtu"] != float64(1420) {
		t.Errorf("expected mtu 1420, got '%v'", probeRoute["mtu"])
	}
}

func TestDiagCapture_WithoutProbeRoute_Serializes(t *testing.T) {
	capture := state.DiagCapture{
		Source:           "test-peer",
		BaseURL:          "http://10.77.0.2:8317/status",
		CaptureStartedAt: time.Now().UTC(),
		Status:           state.DiagCaptureStatusOK,
		CaptureStatus:    state.CaptureStatusCaptured,
		// No ProbeRoute set
	}

	data, err := json.Marshal(capture)
	if err != nil {
		t.Fatalf("failed to marshal DiagCapture: %v", err)
	}

	// Verify probe_route is NOT in the JSON (omitted when nil)
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if _, ok := decoded["probe_route"]; ok {
		t.Fatal("probe_route should be omitted when nil")
	}
}

func TestDiagCapture_WithProbeRouteError_Serializes(t *testing.T) {
	capture := state.DiagCapture{
		Source:           "test-peer",
		BaseURL:          "http://10.77.0.2:8317/status",
		CaptureStartedAt: time.Now().UTC(),
		Status:           state.DiagCaptureStatusOK,
		CaptureStatus:    state.CaptureStatusCaptured,
		ProbeRoute: &state.ProbeRoute{
			Kind:       state.ProbeRouteKindHTTP,
			ProbeHost:  "10.77.0.2",
			LookupTarget: "10.77.0.2",
			Ok:         false,
			ErrorKind:  state.RouteLookupErrorCommandMissing,
			Error:      "ip command unavailable",
			CollectedAt: "2026-06-21T16:00:00Z",
		},
	}

	data, err := json.Marshal(capture)
	if err != nil {
		t.Fatalf("failed to marshal DiagCapture: %v", err)
	}

	// Verify probe_route with error is in the JSON
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	probeRoute, ok := decoded["probe_route"].(map[string]interface{})
	if !ok {
		t.Fatal("probe_route not found in JSON output")
	}

	if probeRoute["ok"] != false {
		t.Errorf("expected ok=false, got '%v'", probeRoute["ok"])
	}
	if probeRoute["error_kind"] != "command_missing" {
		t.Errorf("expected error_kind 'command_missing', got '%v'", probeRoute["error_kind"])
	}
}

func TestProbeRoute_ICMPKind_Serializes(t *testing.T) {
	capture := state.DiagCapture{
		Source:           "test-peer",
		BaseURL:          "http://10.77.0.2:8317/status",
		CaptureStartedAt: time.Now().UTC(),
		Status:           state.DiagCaptureStatusOK,
		CaptureStatus:    state.CaptureStatusCaptured,
		ProbeRoute: &state.ProbeRoute{
			Kind:         state.ProbeRouteKindICMP,
			ProbeHost:    "8.8.8.8",
			LookupTarget: "8.8.8.8",
			Ok:           true,
			RouteType:    "unicast",
			Interface:    "eth0",
			SourceIP:     "redacted",
			Gateway:      "redacted",
			CollectedAt:  "2026-06-21T16:00:00Z",
		},
	}

	data, err := json.Marshal(capture)
	if err != nil {
		t.Fatalf("failed to marshal DiagCapture: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	probeRoute := decoded["probe_route"].(map[string]interface{})
	if probeRoute["kind"] != "icmp" {
		t.Errorf("expected kind 'icmp', got '%v'", probeRoute["kind"])
	}
}

func TestProbeRoute_Deserializes(t *testing.T) {
	jsonData := `{
		"source": "test-peer",
		"base_url": "http://10.77.0.2:8317/status",
		"capture_started_at": "2026-06-21T16:00:00Z",
		"status": "ok",
		"capture_status": "captured",
		"probe_route": {
			"kind": "http",
			"probe_host": "10.77.0.2",
			"lookup_target": "10.77.0.2",
			"ok": true,
			"route_type": "unicast",
			"interface": "wgc1",
			"source_ip": "redacted",
			"gateway": "redacted",
			"table": "main",
			"mtu": 1420,
			"collected_at": "2026-06-21T16:00:00Z"
		}
	}`

	var capture state.DiagCapture
	if err := json.Unmarshal([]byte(jsonData), &capture); err != nil {
		t.Fatalf("failed to unmarshal DiagCapture: %v", err)
	}

	if capture.ProbeRoute == nil {
		t.Fatal("probe_route should be deserialized")
	}
	if capture.ProbeRoute.Kind != state.ProbeRouteKindHTTP {
		t.Errorf("expected kind 'http', got '%s'", capture.ProbeRoute.Kind)
	}
	if capture.ProbeRoute.Interface != "wgc1" {
		t.Errorf("expected interface 'wgc1', got '%s'", capture.ProbeRoute.Interface)
	}
	if capture.ProbeRoute.MTU == nil || *capture.ProbeRoute.MTU != 1420 {
		t.Errorf("expected MTU 1420, got %v", capture.ProbeRoute.MTU)
	}
}

func TestProbeRoute_DeserializesWithoutProbeRoute(t *testing.T) {
	jsonData := `{
		"source": "test-peer",
		"base_url": "http://10.77.0.2:8317/status",
		"capture_started_at": "2026-06-21T16:00:00Z",
		"status": "ok",
		"capture_status": "captured"
	}`

	var capture state.DiagCapture
	if err := json.Unmarshal([]byte(jsonData), &capture); err != nil {
		t.Fatalf("failed to unmarshal DiagCapture: %v", err)
	}

	// Old packets without probe_route should deserialize safely
	if capture.ProbeRoute != nil {
		t.Error("probe_route should be nil for old packets")
	}
}

func TestProbeRoute_DeserializesWithError(t *testing.T) {
	jsonData := `{
		"source": "test-peer",
		"base_url": "http://10.77.0.2:8317/status",
		"capture_started_at": "2026-06-21T16:00:00Z",
		"status": "ok",
		"capture_status": "captured",
		"probe_route": {
			"kind": "http",
			"probe_host": "10.77.0.2",
			"ok": false,
			"error_kind": "command_missing",
			"error": "ip command unavailable",
			"collected_at": "2026-06-21T16:00:00Z"
		}
	}`

	var capture state.DiagCapture
	if err := json.Unmarshal([]byte(jsonData), &capture); err != nil {
		t.Fatalf("failed to unmarshal DiagCapture: %v", err)
	}

	if capture.ProbeRoute == nil {
		t.Fatal("probe_route should be deserialized")
	}
	if capture.ProbeRoute.Ok != false {
		t.Errorf("expected ok=false, got %v", capture.ProbeRoute.Ok)
	}
	if capture.ProbeRoute.ErrorKind != state.RouteLookupErrorCommandMissing {
		t.Errorf("expected error_kind 'command_missing', got '%s'", capture.ProbeRoute.ErrorKind)
	}
}

func TestProbeRoute_FullJSONShape(t *testing.T) {
	mtu := 1420
	route := state.ProbeRoute{
		Kind:         state.ProbeRouteKindHTTP,
		ProbeHost:    "example.com",
		ResolvedIP:   "203.0.113.1",
		LookupTarget: "203.0.113.1",
		Ok:           true,
		RouteType:    "unicast",
		Interface:    "wgc1",
		SourceIP:     "redacted",
		Gateway:      "redacted",
		Table:        "main",
		MTU:          &mtu,
		UID:          nil,
		Error:        "",
		ErrorKind:    "",
		CollectedAt:  "2026-06-21T16:00:00Z",
	}

	data, err := json.Marshal(route)
	if err != nil {
		t.Fatalf("failed to marshal ProbeRoute: %v", err)
	}

	// Verify all expected fields are present
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	expectedFields := []string{
		"kind", "probe_host", "resolved_ip", "lookup_target",
		"ok", "route_type", "interface", "source_ip", "gateway",
		"table", "mtu", "collected_at",
	}

	for _, field := range expectedFields {
		if _, ok := decoded[field]; !ok {
			t.Errorf("expected field '%s' in JSON output", field)
		}
	}

	// Verify raw_redacted is NOT present (removed for privacy)
	if _, ok := decoded["raw_redacted"]; ok {
		t.Error("raw_redacted should not be present (removed for privacy)")
	}

	// Verify optional fields (uid, error, error_kind) are omitted when empty/nil
	if _, ok := decoded["uid"]; ok {
		t.Error("uid should be omitted when nil")
	}
	if _, ok := decoded["error"]; ok {
		t.Error("error should be omitted when empty")
	}
	if _, ok := decoded["error_kind"]; ok {
		t.Error("error_kind should be omitted when empty")
	}
}

func TestExtractHostFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"http://10.77.0.2:8317/status", "10.77.0.2"},
		{"https://example.com:8443/status", "example.com"},
		{"http://192.168.1.1/status", "192.168.1.1"},
		{"", ""},
		{"not-a-url", ""},
	}

	for _, tt := range tests {
		result := extractHostFromURL(tt.url)
		if result != tt.expected {
			t.Errorf("extractHostFromURL(%q): expected %q, got %q", tt.url, tt.expected, result)
		}
	}
}

func TestProbeKindToRouteKind(t *testing.T) {
	tests := []struct {
		probeKind string
		expected  state.ProbeRouteKind
	}{
		{"http", state.ProbeRouteKindHTTP},
		{"icmp", state.ProbeRouteKindICMP},
		{"HTTP", state.ProbeRouteKindHTTP},
		{"ICMP", state.ProbeRouteKindICMP},
		{"Http", state.ProbeRouteKindHTTP},
		{"Icmp", state.ProbeRouteKindICMP},
		{"", state.ProbeRouteKindHTTP},
		{"unknown", state.ProbeRouteKindHTTP},
	}

	for _, tt := range tests {
		result := probeKindToRouteKind(tt.probeKind)
		if result != tt.expected {
			t.Errorf("probeKindToRouteKind(%q): expected %q, got %q", tt.probeKind, tt.expected, result)
		}
	}
}

func TestRouteLookupParser_ParseRouteGetOutput_NoSourceIP(t *testing.T) {
	parser := NewRouteLookupParser()
	// Some route outputs don't include src field
	input := "10.77.0.2 via 192.0.2.1 dev eth0"

	result, err := parser.ParseRouteGetOutput("10.77.0.2", input, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.RouteType != "unicast" {
		t.Errorf("expected route type 'unicast', got '%s'", result.RouteType)
	}
	if result.Interface != "eth0" {
		t.Errorf("expected interface 'eth0', got '%s'", result.Interface)
	}
	if result.SourceIP != "" {
		t.Errorf("expected no source for direct route, got '%s'", result.SourceIP)
	}
}
