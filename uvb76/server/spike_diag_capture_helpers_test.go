package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/diag"
	"github.com/s1onique/KGB/uvb76/state"
)

// fakeTovarischHandler returns a handler that serves fake tovarisch status responses.
// This handler only accepts the canonical tovarisch endpoint: /status.json
// with include=network_diag query param. All other paths return 404.
func fakeTovarischHandler(networkDiagJSON string, statusCode int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Must be GET to /status.json with include=network_diag
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/status.json" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("include") != "network_diag" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.WriteHeader(statusCode)
		if statusCode == http.StatusOK {
			w.Write([]byte(networkDiagJSON))
		}
	}
}

// validTovarischStatusJSON returns a valid tovarisch status response with network_diag.
func validTovarischStatusJSON() string {
	return `{
		"service": "tovarisch",
		"status": "ok",
		"network_diag": {
			"started_at": "2026-06-18T12:00:00Z",
			"status": "ok",
			"wireguard": null,
			"interfaces": [],
			"routes": [],
			"underlay_tcp": [
				{
					"name": "xray",
					"state": "ESTAB",
					"local": "redacted:443",
					"remote": "redacted:443",
					"rtt_ms": 123.4,
					"rto_ms": 456,
					"retransmits": 7,
					"status": "ok"
				}
			],
			"events": []
		}
	}`
}

// waitForCaptures polls the capture store until captures are available or timeout.
func waitForCaptures(store *state.CaptureStore, eventID string, timeout time.Duration) []state.DiagCapture {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		captures := store.GetCaptures(eventID)
		if len(captures) > 0 {
			return captures
		}
		time.Sleep(10 * time.Millisecond)
	}
	return store.GetCaptures(eventID)
}

// RecordSpikeForTest is a test helper that wraps state.Manager.DetectAndRecordSpike
// with nil HTTP trace. Use this in tests where HTTP phase timing is irrelevant.
// This centralizes the test API so future signature changes only need one place updated.
func RecordSpikeForTest(
	st *state.Manager,
	targetID, kind string,
	latencyMs float64,
	sampleTs time.Time,
	reachable bool,
	schedulerDelayMs *float64,
	httpStatus *int,
	probeError *string,
	previousSamples []state.LatencySample,
) *state.SpikeEvent {
	return st.DetectAndRecordSpike(
		targetID, kind,
		latencyMs, sampleTs, reachable,
		schedulerDelayMs, httpStatus, probeError,
		previousSamples,
		nil, // httpTrace
	)
}

// setupTestServer creates a test server with spike API and capture service.
func setupTestServer(t *testing.T, tovarischServerURL string) (*Server, *state.Manager, *diag.CaptureService, string) {
	salt := []byte("1234567890abcdef")
	hash, _ := config.HashPassword("correct-password", salt)

	cfg := &config.Config{
		Listen:  config.ListenConfig{Addr: ":0"},
		Auth:    config.AuthConfig{Username: "admin", PasswordSHA256: hash},
		Scrape:  config.ScrapeConfig{IntervalSeconds: 30, TimeoutMilliseconds: 5000},
		Latency: config.LatencyConfig{},
		Diagnostics: config.DiagnosticsConfig{
			Enabled:         true,
			CaptureOnSpike:  true,
			TimeoutMs:       2000,
			CooldownSeconds: 90,
			Peers:           []config.DiagPeerConfig{{Name: "tovarisch-peer", BaseURL: tovarischServerURL, Targets: []string{"test-target"}}},
		},
		Targets: []config.TargetConfig{{ID: "test-target", Name: "Test Target", BaseURL: "http://localhost:8080", Enabled: true}},
	}
	cfg.Latency.ApplyDefaults()
	cfg.Diagnostics.ApplyDefaults()

	st := state.NewManager()
	captureStore := st.GetCaptureStore()
	captureSvc := diag.NewCaptureService(&cfg.Diagnostics, captureStore)
	srv := NewServer(cfg, st, nil, true)
	token, _ := srv.sessions.GenerateToken("admin")

	return srv, st, captureSvc, token
}
