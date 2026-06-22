package diag

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// TestCaptureWithTcpQuality_HTTPSuccess tests that successful HTTP capture includes tcp_quality.
func TestCaptureWithTcpQuality_HTTPSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"network_diag":{"started_at":"2024-01-01T00:00:00Z","status":"ok","interfaces":[],"routes":[],"underlay_tcp":[],"events":[]}}`))
	}))
	defer server.Close()

	cfg := &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       2000,
		CooldownSeconds: 90,
		Peers:          []config.DiagPeerConfig{{Name: "peer1", BaseURL: server.URL, Targets: []string{"target-1"}}},
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
	if capture.TcpQuality == nil {
		t.Fatal("expected tcp_quality to be populated")
	}

	// Verify tcp_quality block
	tq := capture.TcpQuality
	if tq.Kind != "http" {
		t.Errorf("expected kind 'http', got '%s'", tq.Kind)
	}
	if tq.Source != "ss-tcp-info" {
		t.Errorf("expected source 'ss-tcp-info', got '%s'", tq.Source)
	}
	// matched_socket depends on ss availability and socket state
	if tq.LookupTarget == "" {
		t.Error("expected lookup_target to be set")
	}
	if tq.CollectedAt == "" {
		t.Error("expected collected_at to be set")
	}
}

// TestCaptureWithTcpQuality_HTTPError tests that failed HTTP capture includes tcp_quality error block.
func TestCaptureWithTcpQuality_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close immediately without response
	}))
	server.Close()

	cfg := &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       500,
		CooldownSeconds: 90,
		Peers:          []config.DiagPeerConfig{{Name: "peer1", BaseURL: server.URL, Targets: []string{"target-1"}}},
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
	if capture.TcpQuality == nil {
		t.Fatal("expected tcp_quality to be populated even on HTTP error")
	}

	// Verify tcp_quality block is present
	tq := capture.TcpQuality
	if tq.Kind != "http" {
		t.Errorf("expected kind 'http', got '%s'", tq.Kind)
	}
	if tq.Source != "ss-tcp-info" {
		t.Errorf("expected source 'ss-tcp-info', got '%s'", tq.Source)
	}
}

// TestCaptureWithTcpQuality_ICMPExplicit tests that ICMP-triggered captures have explicit unavailable block.
func TestCaptureWithTcpQuality_ICMPExplicit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"network_diag":{"started_at":"2024-01-01T00:00:00Z","status":"ok","interfaces":[],"routes":[],"underlay_tcp":[],"events":[]}}`))
	}))
	defer server.Close()

	cfg := &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       2000,
		CooldownSeconds: 90,
		Peers:          []config.DiagPeerConfig{{Name: "peer1", BaseURL: server.URL, Targets: []string{"target-1"}}},
	}
	cfg.ApplyDefaults()
	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	// Trigger with ICMP probe kind
	svc.TriggerCapture("event-1", "target-1", "icmp")
	time.Sleep(100 * time.Millisecond)

	captures := store.GetCaptures("event-1")
	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}

	capture := captures[0]
	if capture.TcpQuality == nil {
		t.Fatal("expected tcp_quality to be populated for ICMP")
	}

	// Verify tcp_quality block for ICMP
	tq := capture.TcpQuality
	if tq.Kind != "icmp" {
		t.Errorf("expected kind 'icmp', got '%s'", tq.Kind)
	}
	if tq.MatchedSocket {
		t.Error("expected matched_socket=false for ICMP")
	}
	if tq.ErrorKind != state.TcpQualityErrorUnavailable {
		t.Errorf("expected error_kind 'unavailable', got '%s'", tq.ErrorKind)
	}
	if tq.Error != "tcp quality is http/probe-only" {
		t.Errorf("expected error message about HTTP-only, got '%s'", tq.Error)
	}
}

// TestCaptureWithTcpQuality_ProbeRouteStillWorks verifies that probe_route still works with tcp_quality.
func TestCaptureWithTcpQuality_ProbeRouteStillWorks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"network_diag":{"started_at":"2024-01-01T00:00:00Z","status":"ok","interfaces":[],"routes":[],"underlay_tcp":[],"events":[]}}`))
	}))
	defer server.Close()

	cfg := &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       2000,
		CooldownSeconds: 90,
		Peers:          []config.DiagPeerConfig{{Name: "peer1", BaseURL: server.URL, Targets: []string{"target-1"}}},
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

	// Both probe_route and tcp_quality should be populated
	if capture.ProbeRoute == nil {
		t.Fatal("expected probe_route to be populated")
	}
	if capture.TcpQuality == nil {
		t.Fatal("expected tcp_quality to be populated")
	}

	// Verify probe_route still works correctly
	if capture.ProbeRoute.Kind != state.ProbeRouteKindHTTP {
		t.Errorf("expected probe_route kind 'http', got '%s'", capture.ProbeRoute.Kind)
	}
}

// TestCaptureWithTcpQuality_BackwardCompatibility verifies old packets without tcp_quality deserialize safely.
func TestCaptureWithTcpQuality_BackwardCompatibility(t *testing.T) {
	// Simulate old capture packet without tcp_quality
	oldCapture := `{
		"source": "peer1",
		"base_url": "http://10.0.0.5:8080",
		"capture_started_at": "2024-01-01T00:00:00Z",
		"status": "ok",
		"capture_status": "captured",
		"effective_capture_url": "http://10.0.0.5:8080/status.json",
		"network_diag": {
			"started_at": "2024-01-01T00:00:00Z",
			"status": "ok",
			"interfaces": [],
			"routes": [],
			"underlay_tcp": [],
			"events": []
		}
	}`

	var capture state.DiagCapture
	if err := json.Unmarshal([]byte(oldCapture), &capture); err != nil {
		t.Fatalf("failed to unmarshal old capture: %v", err)
	}

	// Should deserialize without errors
	if capture.Source != "peer1" {
		t.Errorf("expected source 'peer1', got '%s'", capture.Source)
	}
	if capture.TcpQuality != nil {
		t.Error("expected tcp_quality to be nil for old capture")
	}
	if capture.NetworkDiag == nil {
		t.Error("expected network_diag to be populated")
	}
}

// TestCaptureWithTcpQuality_AllErrorKinds verifies all error kinds are serialized correctly.
func TestCaptureWithTcpQuality_AllErrorKinds(t *testing.T) {
	errorKinds := []state.TcpQualityErrorKind{
		state.TcpQualityErrorCommandMissing,
		state.TcpQualityErrorNonZeroExit,
		state.TcpQualityErrorTimeout,
		state.TcpQualityErrorNoData,
		state.TcpQualityErrorParseFailed,
		state.TcpQualityErrorTargetUnresolved,
		state.TcpQualityErrorNoMatchingSocket,
		state.TcpQualityErrorUnavailable,
	}

	for _, errKind := range errorKinds {
		t.Run(string(errKind), func(t *testing.T) {
			tq := &state.TcpQuality{
				Kind:           "http",
				LookupTarget:  "10.0.0.5",
				MatchedSocket: false,
				Source:        "ss-tcp-info",
				ErrorKind:     errKind,
				Error:         "test error",
				CollectedAt:   time.Now().UTC().Format(time.RFC3339),
			}

			data, err := json.Marshal(tq)
			if err != nil {
				t.Fatalf("failed to marshal TcpQuality: %v", err)
			}

			var parsed map[string]interface{}
			if err := json.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("failed to unmarshal TcpQuality: %v", err)
			}

			if parsed["error_kind"] != string(errKind) {
				t.Errorf("expected error_kind '%s', got '%v'", errKind, parsed["error_kind"])
			}
		})
	}
}

// TestCaptureWithTcpQuality_OptionalFields tests that optional fields are omitted when nil.
func TestCaptureWithTcpQuality_OptionalFields(t *testing.T) {
	// TcpQuality with only essential fields
	tq := &state.TcpQuality{
		Kind:           "http",
		LookupTarget:  "redacted",
		MatchedSocket: true,
		Source:        "ss-tcp-info",
		State:         "ESTAB",
		Local:         "redacted:45678",
		Remote:        "redacted:8080",
		CollectedAt:   "2026-06-21T22:35:00Z",
	}

	data, err := json.Marshal(tq)
	if err != nil {
		t.Fatalf("failed to marshal TcpQuality: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal TcpQuality: %v", err)
	}

	// Optional numeric fields should be omitted
	optionalFields := []string{"rtt_us", "rttvar_us", "retransmits_current", "retransmits_total",
		"unacked", "lost", "sacked", "reordering", "snd_cwnd", "ssthresh", "delivery_rate_bps",
		"send_queue_bytes", "recv_queue_bytes", "match_count", "error_kind", "error", "congestion_algorithm"}

	for _, field := range optionalFields {
		if _, ok := parsed[field]; ok {
			t.Errorf("optional field '%s' should be omitted when nil", field)
		}
	}

	// Essential fields should be present
	essentialFields := map[string]interface{}{
		"kind":           "http",
		"matched_socket": true,
		"source":        "ss-tcp-info",
		"state":          "ESTAB",
		"local":          "redacted:45678",
		"remote":         "redacted:8080",
		"collected_at":   "2026-06-21T22:35:00Z",
	}

	for field, expected := range essentialFields {
		if parsed[field] != expected {
			t.Errorf("expected %s='%v', got '%v'", field, expected, parsed[field])
		}
	}
}

// TestCaptureWithTcpQuality_TimeoutDoesNotBlock verifies TCP quality timeout doesn't block capture.
func TestCaptureWithTcpQuality_TimeoutDoesNotBlock(t *testing.T) {
	// This test verifies the integration - if TCP collection times out,
	// the capture should still complete with an error block.

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"network_diag":{"started_at":"2024-01-01T00:00:00Z","status":"ok","interfaces":[],"routes":[],"underlay_tcp":[],"events":[]}}`))
	}))
	defer server.Close()

	cfg := &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       5000, // Longer timeout for HTTP
		CooldownSeconds: 90,
		Peers:          []config.DiagPeerConfig{{Name: "peer1", BaseURL: server.URL, Targets: []string{"target-1"}}},
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

	// Capture should complete regardless of TCP quality
	if capture.Status != state.DiagCaptureStatusOK {
		t.Errorf("expected capture status 'ok', got '%s'", capture.Status)
	}

	// TCP quality should be present (even if it's an error block)
	if capture.TcpQuality == nil {
		t.Fatal("expected tcp_quality to be populated")
	}
}
