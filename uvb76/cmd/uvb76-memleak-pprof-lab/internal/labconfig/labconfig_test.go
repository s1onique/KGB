// Package labconfig provides tests for lab config generation.
package labconfig

import (
	"encoding/json"
	"testing"

	"github.com/s1onique/KGB/uvb76/config"
)

func TestGenerate_PProfEnabled(t *testing.T) {
	cfg := Generate("18444", "16060", "18317")

	// Verify diagnostics is enabled
	if !cfg.Diagnostics.Enabled {
		t.Error("Expected Diagnostics.Enabled to be true")
	}

	// Verify pprof is enabled in lab config
	if !cfg.Diagnostics.PProf.Enabled {
		t.Error("Expected Diagnostics.PProf.Enabled to be true in lab config")
	}

	// Verify pprof listen address
	if cfg.Diagnostics.PProf.Listen != "localhost:16060" {
		t.Errorf("Expected PProf.Listen=localhost:16060, got %s", cfg.Diagnostics.PProf.Listen)
	}

	// Verify mem profile rate
	if cfg.Diagnostics.PProf.MemProfileRate != 65536 {
		t.Errorf("Expected MemProfileRate=65536, got %d", cfg.Diagnostics.PProf.MemProfileRate)
	}
}

func TestGenerate_TargetConfigured(t *testing.T) {
	cfg := Generate("18444", "16060", "18317")

	if len(cfg.Targets) != 1 {
		t.Fatalf("Expected 1 target, got %d", len(cfg.Targets))
	}

	target := cfg.Targets[0]
	if target.ID != "fake-tovarisch" {
		t.Errorf("Expected target ID=fake-tovarisch, got %s", target.ID)
	}
	if target.BaseURL != "http://localhost:18317/status" {
		t.Errorf("Expected BaseURL=http://localhost:18317/status, got %s", target.BaseURL)
	}
	if !target.Enabled {
		t.Error("Expected target to be enabled")
	}
}

func TestGenerate_ScrapeInterval(t *testing.T) {
	cfg := Generate("18444", "16060", "18317")

	if cfg.Scrape.IntervalSeconds != 30 {
		t.Errorf("Expected Scrape.IntervalSeconds=30, got %d", cfg.Scrape.IntervalSeconds)
	}
}

func TestGenerate_ListenAddress(t *testing.T) {
	cfg := Generate("18444", "16060", "18317")

	if cfg.Listen.Addr != "localhost:18444" {
		t.Errorf("Expected Listen.Addr=localhost:18444, got %s", cfg.Listen.Addr)
	}
}

func TestGenerate_NoTLSCert(t *testing.T) {
	cfg := Generate("18444", "16060", "18317")

	if cfg.Listen.TLSCertFile != "" {
		t.Errorf("Expected TLSCertFile to be empty, got %s", cfg.Listen.TLSCertFile)
	}
}

func TestGenerate_ValidJSON(t *testing.T) {
	cfg := Generate("18444", "16060", "18317")

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	// Verify it can be unmarshaled
	var decoded Config
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	// Verify key fields
	if decoded.Diagnostics.PProf.Listen != "localhost:16060" {
		t.Errorf("Round-trip failed: PProf.Listen=%s", decoded.Diagnostics.PProf.Listen)
	}
}

func TestGenerate_DiagnosticsConfigType(t *testing.T) {
	cfg := Generate("18444", "16060", "18317")

	// Verify the type is compatible with config.DiagnosticsConfig
	var diags config.DiagnosticsConfig = cfg.Diagnostics
	if !diags.Enabled {
		t.Error("DiagnosticsConfig should be enabled")
	}
	if !diags.PProf.Enabled {
		t.Error("DiagnosticsConfig.PProf should be enabled")
	}
}

func TestGenerate_AuthConfigured(t *testing.T) {
	cfg := Generate("18444", "16060", "18317")

	if cfg.Auth.Username == "" {
		t.Error("Expected Auth.Username to be set")
	}
	if cfg.Auth.PasswordSHA256 == "" {
		t.Error("Expected Auth.PasswordSHA256 to be set")
	}
}
