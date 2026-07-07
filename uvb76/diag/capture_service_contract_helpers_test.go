package diag

import (
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// =============================================================================
// ACT-UVB76-HULK02: Shared Test Helpers for Capture Service Contracts
// =============================================================================
//
// These helpers are shared across capture service contract tests.
//
// =============================================================================

// waitForCapture waits for a capture to appear in the store.
// Uses a polling approach instead of fixed sleeps to reduce flakiness.
func waitForCapture(t *testing.T, store *state.CaptureStore, eventID string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		captures := store.GetCaptures(eventID)
		if len(captures) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Give a bit more time for async operations
	time.Sleep(100 * time.Millisecond)
}

// testCaptureConfig creates a test capture config with the given base URL.
func testCaptureConfig(baseURL string) *config.DiagnosticsConfig {
	return &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       2000,
		CooldownSeconds: 90,
		Peers:           []config.DiagPeerConfig{{Name: "peer-1", BaseURL: baseURL, Targets: []string{"target-1"}}},
	}
}

// testCaptureConfigWithTimeout creates a test capture config with a custom timeout.
func testCaptureConfigWithTimeout(baseURL string, timeoutMs int) *config.DiagnosticsConfig {
	cfg := testCaptureConfig(baseURL)
	cfg.TimeoutMs = timeoutMs
	return cfg
}

// testCaptureConfigWithNoPeers creates a test capture config with no peer mappings.
func testCaptureConfigWithNoPeers() *config.DiagnosticsConfig {
	return &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       2000,
		CooldownSeconds: 90,
		Peers:           []config.DiagPeerConfig{{Name: "peer-1", BaseURL: "http://localhost:8080", Targets: []string{}}}, // No target mappings
	}
}

// testCaptureConfigDisabled creates a test capture config that is disabled.
func testCaptureConfigDisabled() *config.DiagnosticsConfig {
	return &config.DiagnosticsConfig{
		Enabled:         false,
		CaptureOnSpike:  false,
		TimeoutMs:       2000,
		CooldownSeconds: 90,
		Peers:           []config.DiagPeerConfig{{Name: "peer-1", BaseURL: "http://localhost:8080", Targets: []string{"target-1"}}},
	}
}
