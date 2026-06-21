package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/s1onique/KGB/uvb76/config"
	"log/slog"
)

func TestSanitizeBaseURLForLog(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "http scheme stripped",
			input:    "http://10.149.149.1:8317",
			expected: "10.149.149.1:8317",
		},
		{
			name:     "https scheme stripped",
			input:    "https://secure.host:8443/api",
			expected: "secure.host:8443/api",
		},
		{
			name:     "no scheme unchanged",
			input:    "host:8317/path",
			expected: "host:8317/path",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeBaseURLForLog(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeBaseURLForLog(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestDiagnosticsStartupLogSecretsAbsent verifies that sensitive config fields
// are NOT present in the diagnostics startup log output.
func TestDiagnosticsStartupLogSecretsAbsent(t *testing.T) {
	// Build a diagnostics config with a single peer
	cfg := &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       25000,
		CooldownSeconds: 90,
		Peers: []config.DiagPeerConfig{
			{
				Name:    "kamatera-tovarisch",
				BaseURL: "http://10.149.149.1:8317",
				Targets: []string{"kamatera"},
			},
		},
	}

	// Use a real slog handler to capture actual production output
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Call the production logging function
	logDiagnosticsStartup(logger, cfg)

	logOutput := buf.String()

	// Verify expected fields ARE present
	if !strings.Contains(logOutput, "uvb76 diagnostics configured") {
		t.Error("expected 'uvb76 diagnostics configured' in log output")
	}
	if !strings.Contains(logOutput, "peer_count=1") {
		t.Error("expected 'peer_count=1' in log output")
	}
	if !strings.Contains(logOutput, "timeout_ms=25000") {
		t.Error("expected 'timeout_ms=25000' in log output")
	}
	if !strings.Contains(logOutput, "cooldown_seconds=90") {
		t.Error("expected 'cooldown_seconds=90' in log output")
	}
	if !strings.Contains(logOutput, "capture_on_spike=true") {
		t.Error("expected 'capture_on_spike=true' in log output")
	}
	if !strings.Contains(logOutput, "name=kamatera-tovarisch") {
		t.Error("expected 'name=kamatera-tovarisch' in peer log")
	}
	if !strings.Contains(logOutput, "base_url=10.149.149.1:8317") {
		t.Error("expected 'base_url=10.149.149.1:8317' (scheme stripped) in peer log")
	}

	// Verify sensitive fields are ABSENT
	if strings.Contains(logOutput, "sha256:") {
		t.Error("password hash 'sha256:' must NOT appear in diagnostics log")
	}
	if strings.Contains(logOutput, "password") {
		t.Error("password field must NOT appear in diagnostics log")
	}
	if strings.Contains(logOutput, "secret") {
		t.Error("secret field must NOT appear in diagnostics log")
	}
	if strings.Contains(logOutput, "token") {
		t.Error("token field must NOT appear in diagnostics log")
	}
	if strings.Contains(logOutput, "cookie") {
		t.Error("cookie field must NOT appear in diagnostics log")
	}
	if strings.Contains(logOutput, "Authorization") {
		t.Error("Authorization header must NOT appear in diagnostics log")
	}
	if strings.Contains(logOutput, "key.pem") {
		t.Error("TLS key path 'key.pem' must NOT appear in diagnostics log")
	}
	if strings.Contains(logOutput, "cert.pem") {
		// cert paths might appear in other parts of the log, but not in diagnostics config
		// We check this is absent from the diagnostics-specific log lines
		for _, line := range strings.Split(logOutput, "\n") {
			if strings.Contains(line, "uvb76 diagnostics") && strings.Contains(line, "cert.pem") {
				t.Error("TLS cert path 'cert.pem' must NOT appear in diagnostics log")
			}
		}
	}
}

// TestDiagnosticsStartupLogNoTCPClaim verifies that the startup log does NOT
// claim TCP diagnostics are enabled, since that is controlled by tovarisch.
func TestDiagnosticsStartupLogNoTCPClaim(t *testing.T) {
	cfg := &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       1500,
		CooldownSeconds: 90,
		Peers: []config.DiagPeerConfig{
			{
				Name:    "test-peer",
				BaseURL: "http://localhost:8317",
				Targets: []string{"test-target"},
			},
		},
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	logDiagnosticsStartup(logger, cfg)

	logOutput := buf.String()

	// These phrases would incorrectly claim peer-side capability
	incorrectPhrases := []string{
		"TCP diag enabled",
		"tcp_diag enabled",
		"underlay_tcp enabled",
		"network_diag enabled",
		"peer TCP",
		"tovarisch TCP",
	}

	for _, phrase := range incorrectPhrases {
		if strings.Contains(logOutput, phrase) {
			t.Errorf("log must NOT claim peer-side TCP diagnostics: found %q", phrase)
		}
	}
}

// TestDiagnosticsStartupLogMultiplePeers verifies that multiple peers are logged correctly.
func TestDiagnosticsStartupLogMultiplePeers(t *testing.T) {
	cfg := &config.DiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       1500,
		CooldownSeconds: 90,
		Peers: []config.DiagPeerConfig{
			{
				Name:    "peer-one",
				BaseURL: "http://10.0.0.1:8317",
				Targets: []string{"target-a", "target-b"},
			},
			{
				Name:    "peer-two",
				BaseURL: "https://10.0.0.2:8317/api",
				Targets: []string{"target-c"},
			},
		},
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	logDiagnosticsStartup(logger, cfg)

	logOutput := buf.String()

	// Verify peer count is correct
	if !strings.Contains(logOutput, "peer_count=2") {
		t.Error("expected 'peer_count=2' in log output")
	}

	// Verify both peers are logged
	if !strings.Contains(logOutput, "name=peer-one") {
		t.Error("expected 'name=peer-one' in log output")
	}
	if !strings.Contains(logOutput, "name=peer-two") {
		t.Error("expected 'name=peer-two' in log output")
	}

	// Verify base URLs are sanitized (scheme stripped)
	if !strings.Contains(logOutput, "base_url=10.0.0.1:8317") {
		t.Error("expected 'base_url=10.0.0.1:8317' for peer-one")
	}
	if !strings.Contains(logOutput, "base_url=10.0.0.2:8317/api") {
		t.Error("expected 'base_url=10.0.0.2:8317/api' for peer-two")
	}

	// Verify targets are joined with comma
	if !strings.Contains(logOutput, "targets=target-a,target-b") {
		t.Error("expected 'targets=target-a,target-b' in peer-one log")
	}
}
