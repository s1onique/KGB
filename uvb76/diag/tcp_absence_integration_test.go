package diag

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// =============================================================================
// Integration tests for production path
// =============================================================================

func TestTriggerCapture_EmptyUnderlayTcpWithEvents_PopulatesTcpAbsenceEvents(t *testing.T) {
	// Simulate a tovarisch response with empty underlay_tcp but with absence events
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status.json" {
			t.Errorf("expected /status.json, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("include") != "network_diag" {
			t.Errorf("expected include=network_diag, got %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
		// Response with empty underlay_tcp but with an underlay_tcp event explaining why
		w.Write([]byte(`{
			"network_diag": {
				"started_at": "2024-01-01T00:00:00Z",
				"status": "warning",
				"interfaces": [],
				"routes": [],
				"underlay_tcp": [],
				"events": [
					{
						"source": "underlay_tcp",
						"message": "no matching socket",
						"fields": "{\"reason\":\"no_matching_socket\",\"expected_peer\":\"kamatera-tovarisch\",\"probe_kind\":\"http\"}"
					}
				]
			}
		}`))
	}))
	defer server.Close()

	cfg := &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       1000,
		CooldownSeconds: 90,
		Peers:           []config.DiagPeerConfig{{Name: "kamatera-tovarisch", BaseURL: server.URL, Targets: []string{"target-1"}}},
	}
	cfg.ApplyDefaults()
	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	svc.TriggerCapture("event-1", "target-1", "http")
	time.Sleep(100 * time.Millisecond)

	captures := store.GetCaptures("event-1")
	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}
	capture := captures[0]

	// Verify capture status
	if capture.Status != state.DiagCaptureStatusOK {
		t.Errorf("expected ok status, got %s", capture.Status)
	}
	if capture.CaptureStatus != state.CaptureStatusCaptured {
		t.Errorf("expected captured status, got %s", capture.CaptureStatus)
	}

	// Verify network_diag is populated
	if capture.NetworkDiag == nil {
		t.Fatal("expected network_diag to be populated")
	}

	// Verify underlay_tcp is empty
	if len(capture.NetworkDiag.UnderlayTCP) != 0 {
		t.Errorf("expected empty underlay_tcp, got %d sockets", len(capture.NetworkDiag.UnderlayTCP))
	}

	// Verify TcpAbsenceEvents is populated
	if len(capture.TcpAbsenceEvents) != 1 {
		t.Fatalf("expected 1 TcpAbsenceEvent, got %d", len(capture.TcpAbsenceEvents))
	}

	event := capture.TcpAbsenceEvents[0]
	if event.ReasonCode != "no_matching_socket" {
		t.Errorf("expected reason_code 'no_matching_socket', got '%s'", event.ReasonCode)
	}
	if event.ExpectedPeer != "kamatera-tovarisch" {
		t.Errorf("expected expected_peer 'kamatera-tovarisch', got '%s'", event.ExpectedPeer)
	}
	if event.ProbeKind != "http" {
		t.Errorf("expected probe_kind 'http', got '%s'", event.ProbeKind)
	}
}

func TestTriggerCapture_MalformedEventFields_ProducesParseFailed(t *testing.T) {
	// Simulate a tovarisch response with malformed fields in the event
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Response with malformed JSON in fields
		w.Write([]byte(`{
			"network_diag": {
				"started_at": "2024-01-01T00:00:00Z",
				"status": "warning",
				"interfaces": [],
				"routes": [],
				"underlay_tcp": [],
				"events": [
					{
						"source": "underlay_tcp",
						"message": "socket parsing failed",
						"fields": "{invalid json"
					}
				]
			}
		}`))
	}))
	defer server.Close()

	cfg := &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       1000,
		CooldownSeconds: 90,
		Peers:           []config.DiagPeerConfig{{Name: "peer1", BaseURL: server.URL, Targets: []string{"target-1"}}},
	}
	cfg.ApplyDefaults()
	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	svc.TriggerCapture("event-1", "target-1", "http")
	time.Sleep(100 * time.Millisecond)

	captures := store.GetCaptures("event-1")
	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}
	capture := captures[0]

	// Verify TcpAbsenceEvents is populated with parse_failed
	if len(capture.TcpAbsenceEvents) != 1 {
		t.Fatalf("expected 1 TcpAbsenceEvent, got %d", len(capture.TcpAbsenceEvents))
	}

	event := capture.TcpAbsenceEvents[0]
	if event.ReasonCode != "parse_failed" {
		t.Errorf("expected reason_code 'parse_failed', got '%s'", event.ReasonCode)
	}
	// Original message should be preserved as detail
	if event.Detail != "socket parsing failed" {
		t.Errorf("expected detail 'socket parsing failed', got '%s'", event.Detail)
	}
}

func TestTriggerCapture_TcpDiagnosticsDisabledByConfig_PopulatesUnderlayTcpDisabled(t *testing.T) {
	// Simulate a tovarisch response with underlay_tcp disabled by config
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status.json" {
			t.Errorf("expected /status.json, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		// Response with empty underlay_tcp and event indicating diagnostics are disabled
		w.Write([]byte(`{
			"network_diag": {
				"started_at": "2024-01-01T00:00:00Z",
				"status": "warning",
				"interfaces": [],
				"routes": [],
				"underlay_tcp": [],
				"events": [
					{
						"source": "underlay_tcp",
						"message": "underlay TCP diagnostics disabled by config",
						"fields": "{\"reason\":\"underlay_tcp_disabled\",\"expected_peer\":\"kamatera-tovarisch\"}"
					}
				]
			}
		}`))
	}))
	defer server.Close()

	cfg := &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       1000,
		CooldownSeconds: 90,
		Peers:           []config.DiagPeerConfig{{Name: "kamatera-tovarisch", BaseURL: server.URL, Targets: []string{"target-1"}}},
	}
	cfg.ApplyDefaults()
	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	svc.TriggerCapture("event-1", "target-1", "http")
	time.Sleep(100 * time.Millisecond)

	captures := store.GetCaptures("event-1")
	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}
	capture := captures[0]

	// Verify TcpAbsenceEvents is populated
	if len(capture.TcpAbsenceEvents) != 1 {
		t.Fatalf("expected 1 TcpAbsenceEvent, got %d", len(capture.TcpAbsenceEvents))
	}

	event := capture.TcpAbsenceEvents[0]

	// Verify reason code is underlay_tcp_disabled
	if event.ReasonCode != "underlay_tcp_disabled" {
		t.Errorf("expected reason_code 'underlay_tcp_disabled', got '%s'", event.ReasonCode)
	}

	// Verify expected peer is preserved
	if event.ExpectedPeer != "kamatera-tovarisch" {
		t.Errorf("expected expected_peer 'kamatera-tovarisch', got '%s'", event.ExpectedPeer)
	}

	// Verify source is underlay_tcp
	if event.Source != "underlay_tcp" {
		t.Errorf("expected source 'underlay_tcp', got '%s'", event.Source)
	}
}

func TestTriggerCapture_NotConfigured_PopulatesNotConfigured(t *testing.T) {
	// Simulate a tovarisch response with TCP diagnostics not configured
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Response with empty underlay_tcp and not_configured event
		w.Write([]byte(`{
			"network_diag": {
				"started_at": "2024-01-01T00:00:00Z",
				"status": "warning",
				"interfaces": [],
				"routes": [],
				"underlay_tcp": [],
				"events": [
					{
						"source": "underlay_tcp",
						"message": "TCP diagnostics not configured",
						"fields": "{\"reason\":\"not_configured\",\"expected_peer\":\"test-peer\"}"
					}
				]
			}
		}`))
	}))
	defer server.Close()

	cfg := &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       1000,
		CooldownSeconds: 90,
		Peers:           []config.DiagPeerConfig{{Name: "test-peer", BaseURL: server.URL, Targets: []string{"target-1"}}},
	}
	cfg.ApplyDefaults()
	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	svc.TriggerCapture("event-1", "target-1", "http")
	time.Sleep(100 * time.Millisecond)

	captures := store.GetCaptures("event-1")
	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}
	capture := captures[0]

	// Verify TcpAbsenceEvents is populated
	if len(capture.TcpAbsenceEvents) != 1 {
		t.Fatalf("expected 1 TcpAbsenceEvent, got %d", len(capture.TcpAbsenceEvents))
	}

	event := capture.TcpAbsenceEvents[0]

	// Verify reason code is not_configured
	if event.ReasonCode != "not_configured" {
		t.Errorf("expected reason_code 'not_configured', got '%s'", event.ReasonCode)
	}

	// Verify expected peer is preserved
	if event.ExpectedPeer != "test-peer" {
		t.Errorf("expected expected_peer 'test-peer', got '%s'", event.ExpectedPeer)
	}
}
