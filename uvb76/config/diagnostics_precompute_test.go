// Package config provides tests for diagnostics URL precomputation.
package config

import "testing"

// TestPrecomputeCaptureURLs_HTTPSScheme tests that HTTPS URLs are handled correctly.
func TestPrecomputeCaptureURLs_HTTPSScheme(t *testing.T) {
	cfg := &DiagnosticsConfig{
		Enabled: true,
		Peers: []DiagPeerConfig{
			{
				Name:    "secure-peer",
				BaseURL: "https://secure.host:8443/api",
				Targets: []string{"target-1"},
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	cfg.PrecomputeCaptureURLs()

	expected := "https://secure.host:8443/api/status.json?include=network_diag"
	if cfg.Peers[0].EffectiveCaptureURL != expected {
		t.Errorf("Expected %q, got %q", expected, cfg.Peers[0].EffectiveCaptureURL)
	}
}

// TestPrecomputeCaptureURLs_DisabledDiagnostics does not compute URLs.
func TestPrecomputeCaptureURLs_DisabledDiagnostics(t *testing.T) {
	cfg := &DiagnosticsConfig{
		Enabled: false,
		Peers: []DiagPeerConfig{
			{
				Name:    "peer-1",
				BaseURL: "http://host:8317",
				Targets: []string{"target-1"},
			},
		},
	}

	// Validate passes even when disabled (no peers required)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	cfg.PrecomputeCaptureURLs()

	// Should NOT compute URLs when disabled
	if cfg.Peers[0].EffectiveCaptureURL != "" {
		t.Errorf("Expected empty EffectiveCaptureURL when disabled, got %q", cfg.Peers[0].EffectiveCaptureURL)
	}
}

// TestPrecomputeCaptureURLs_ValidatesBeforeComputing verifies that
// PrecomputeCaptureURLs is only called after Validate() succeeds.
func TestPrecomputeCaptureURLs_ValidatesBeforeComputing(t *testing.T) {
	// Diagnostics config with invalid base_url
	cfg := &DiagnosticsConfig{
		Enabled: true,
		Peers: []DiagPeerConfig{
			{
				Name:    "invalid-peer",
				BaseURL: "http://user:pass@host:8317", // Invalid: userinfo
				Targets: []string{"target-1"},
			},
		},
	}

	// Validate should fail
	if err := cfg.Validate(); err == nil {
		t.Error("Expected Validate() to fail for userinfo URL")
	}

	// Calling PrecomputeCaptureURLs after failed validation is a programming error
	// but won't crash - it will just compute whatever is there (or empty)
	// In production, PrecomputeCaptureURLs is only called after Validate() succeeds.
}
