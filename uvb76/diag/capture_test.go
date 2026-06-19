package diag

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

func TestTriggerCapture_Disabled(t *testing.T) {
	cfg := &config.DiagnosticsConfig{Enabled: false, CaptureOnSpike: true}
	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	svc.TriggerCapture("event-1", "target-1")
	captures := store.GetCaptures("event-1")
	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}
	if captures[0].Status != state.DiagCaptureStatusDisabled {
		t.Errorf("expected disabled status, got %s", captures[0].Status)
	}
}

func TestTriggerCapture_NoPeerMapping(t *testing.T) {
	cfg := &config.DiagnosticsConfig{
		Enabled:        true,
		CaptureOnSpike: true,
		Peers:         []config.DiagPeerConfig{{Name: "peer1", BaseURL: "http://localhost:8080", Targets: []string{"other-target"}}},
	}
	cfg.ApplyDefaults()
	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	svc.TriggerCapture("event-1", "unknown-target")
	captures := store.GetCaptures("event-1")
	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}
	if captures[0].Status != state.DiagCaptureStatusNoPeerMapping {
		t.Errorf("expected no_peer_mapping status, got %s", captures[0].Status)
	}
}

func TestTriggerCapture_SuccessfulCapture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status.json" {
			t.Errorf("expected /status.json, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("include") != "network_diag" {
			t.Errorf("expected include=network_diag, got %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"network_diag":{"started_at":"2024-01-01T00:00:00Z","status":"ok","interfaces":[],"routes":[],"underlay_tcp":[],"events":[]}}`))
	}))
	defer server.Close()

	cfg := &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       1000,
		CooldownSeconds: 90,
		Peers:          []config.DiagPeerConfig{{Name: "peer1", BaseURL: server.URL, Targets: []string{"target-1"}}},
	}
	cfg.ApplyDefaults()
	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	svc.TriggerCapture("event-1", "target-1")
	time.Sleep(100 * time.Millisecond)

	captures := store.GetCaptures("event-1")
	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}
	capture := captures[0]
	if capture.Status != state.DiagCaptureStatusOK {
		t.Errorf("expected ok status, got %s", capture.Status)
	}
	if capture.NetworkDiag == nil {
		t.Error("expected network_diag to be populated")
	}
	if capture.SuppressedByCooldown {
		t.Error("capture should not be suppressed")
	}
}

func TestTriggerCapture_CooldownSuppression(t *testing.T) {
	cfg := &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       1000,
		CooldownSeconds: 90,
		Peers:          []config.DiagPeerConfig{{Name: "peer1", BaseURL: "http://localhost:8080", Targets: []string{"target-1"}}},
	}
	cfg.ApplyDefaults()
	store := state.NewCaptureStore()

	// Add a successful capture to set cooldown
	now := time.Now().UTC()
	store.AddCapture("event-0", state.DiagCapture{
		Source:           "peer1",
		CaptureStartedAt: now,
		Status:           state.DiagCaptureStatusOK,
		SuppressedByCooldown: false,
	})

	svc := NewCaptureService(cfg, store)

	// Should be suppressed
	if !store.IsInCooldown("peer1", 90) {
		t.Error("expected cooldown to be set")
	}

	svc.TriggerCapture("event-1", "target-1")
	time.Sleep(50 * time.Millisecond)

	captures := store.GetCaptures("event-1")
	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}
	capture := captures[0]
	if !capture.SuppressedByCooldown {
		t.Error("expected suppressed capture")
	}
}

func TestTriggerCapture_InFlightSuppression(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"network_diag":{"started_at":"2024-01-01T00:00:00Z","status":"ok","interfaces":[],"routes":[],"underlay_tcp":[],"events":[]}}`))
	}))
	defer server.Close()

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

	// Trigger first capture (slow)
	svc.TriggerCapture("event-1", "target-1")
	time.Sleep(10 * time.Millisecond)

	// Trigger second capture (should be suppressed due to in-flight race)
	// Note: In-flight suppression is NOT a cooldown scenario, so SuppressedByCooldown is false.
	// Instead, it records not_attempted status since there's no prior successful capture.
	svc.TriggerCapture("event-2", "target-1")
	time.Sleep(50 * time.Millisecond)

	captures := store.GetCaptures("event-2")
	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}
	capture := captures[0]
	// In-flight without prior successful capture should be not_attempted, not skipped_cooldown
	if capture.CaptureStatus == state.CaptureStatusSkippedCooldown {
		t.Error("invariant violation: in-flight capture should not be skipped_cooldown without prior capture")
	}
}

func TestTriggerCapture_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := &config.DiagnosticsConfig{
		Enabled:        true,
		CaptureOnSpike: true,
		TimeoutMs:      1000,
		Peers:         []config.DiagPeerConfig{{Name: "peer1", BaseURL: server.URL, Targets: []string{"target-1"}}},
	}
	cfg.ApplyDefaults()
	store := state.NewCaptureStore()
	svc := NewCaptureService(cfg, store)

	svc.TriggerCapture("event-1", "target-1")
	time.Sleep(50 * time.Millisecond)

	captures := store.GetCaptures("event-1")
	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}
	capture := captures[0]
	if capture.Status != state.DiagCaptureStatusError {
		t.Errorf("expected error status, got %s", capture.Status)
	}
}

func TestSafeErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"short", "short error", "short error"},
		{"max_length", string(make([]byte, 250)), string(make([]byte, 200))},
		{"with_newlines", "error\nwith\nnewlines", "error with newlines"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SafeErrorMessage(tt.input)
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if *result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, *result)
			}
		})
	}
}
