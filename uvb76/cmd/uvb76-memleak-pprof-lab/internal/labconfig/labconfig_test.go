package labconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/s1onique/KGB/uvb76/config"
)

func TestGenerateRealMode(t *testing.T) {
	cfg := Generate("18444", "16060", "18317", false)

	// Verify target ID is real-tovarisch
	if len(cfg.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(cfg.Targets))
	}
	if cfg.Targets[0].ID != "real-tovarisch" {
		t.Errorf("expected target ID 'real-tovarisch', got %q", cfg.Targets[0].ID)
	}
	if cfg.Targets[0].Name != "Real Tovarisch Status Endpoint" {
		t.Errorf("expected target name 'Real Tovarisch Status Endpoint', got %q", cfg.Targets[0].Name)
	}

	// Verify diagnostics peer uses real-tovarisch
	if len(cfg.Diagnostics.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(cfg.Diagnostics.Peers))
	}
	if cfg.Diagnostics.Peers[0].Name != "real-tovarisch-peer" {
		t.Errorf("expected peer name 'real-tovarisch-peer', got %q", cfg.Diagnostics.Peers[0].Name)
	}
	if len(cfg.Diagnostics.Peers[0].Targets) != 1 || cfg.Diagnostics.Peers[0].Targets[0] != "real-tovarisch" {
		t.Errorf("expected peer targets ['real-tovarisch'], got %v", cfg.Diagnostics.Peers[0].Targets)
	}

	// Verify scrape interval is short for smoke
	if cfg.Scrape.IntervalSeconds != 1 {
		t.Errorf("expected scrape interval 1, got %d", cfg.Scrape.IntervalSeconds)
	}
}

func TestGenerateFakeMode(t *testing.T) {
	cfg := Generate("18444", "16060", "18317", true)

	// Verify target ID is fake-tovarisch
	if cfg.Targets[0].ID != "fake-tovarisch" {
		t.Errorf("expected target ID 'fake-tovarisch', got %q", cfg.Targets[0].ID)
	}
	if cfg.Targets[0].Name != "Fake Tovarisch Status Endpoint" {
		t.Errorf("expected target name 'Fake Tovarisch Status Endpoint', got %q", cfg.Targets[0].Name)
	}

	// Verify diagnostics peer uses fake-tovarisch
	if cfg.Diagnostics.Peers[0].Name != "fake-tovarisch-peer" {
		t.Errorf("expected peer name 'fake-tovarisch-peer', got %q", cfg.Diagnostics.Peers[0].Name)
	}
}

func TestGeneratedConfigValidates(t *testing.T) {
	cfg := Generate("18444", "16060", "18317", false)

	// Convert to config.Config for validation
	prodCfg := &config.Config{
		Listen:      cfg.Listen,
		Auth:        cfg.Auth,
		Scrape:      cfg.Scrape,
		Latency:     cfg.Latency,
		Diagnostics: cfg.Diagnostics,
		Targets:     cfg.Targets,
	}

	// Validate with dev mode (allow missing TLS)
	err := prodCfg.Validate(config.ValidationOptions{AllowMissingTLS: true})
	if err != nil {
		t.Errorf("generated config should be valid: %v", err)
	}
}

func TestPProfEnabled(t *testing.T) {
	cfg := Generate("18444", "16060", "18317", false)

	if !cfg.Diagnostics.Enabled {
		t.Error("diagnostics should be enabled")
	}
	if !cfg.Diagnostics.PProf.Enabled {
		t.Error("pprof should be enabled")
	}
	if cfg.Diagnostics.PProf.Listen != "localhost:16060" {
		t.Errorf("expected pprof listen 'localhost:16060', got %q", cfg.Diagnostics.PProf.Listen)
	}
}

func TestDiagPeerHasValidBaseURL(t *testing.T) {
	cfg := Generate("18444", "16060", "18317", false)

	peer := cfg.Diagnostics.Peers[0]
	if peer.BaseURL != "http://localhost:18317" {
		t.Errorf("expected peer base_url 'http://localhost:18317', got %q", peer.BaseURL)
	}
}

func TestTargetURLIsRealTovarisch(t *testing.T) {
	cfg := Generate("18444", "16060", "18317", false)

	target := cfg.Targets[0]
	if target.BaseURL != "http://localhost:18317" {
		t.Errorf("expected target base_url 'http://localhost:18317', got %q", target.BaseURL)
	}
}

func TestScrapeIntervalForSmoke(t *testing.T) {
	cfg := Generate("18444", "16060", "18317", false)

	// 1 second interval for active smoke test
	if cfg.Scrape.IntervalSeconds != 1 {
		t.Errorf("expected scrape interval 1 for smoke, got %d", cfg.Scrape.IntervalSeconds)
	}
}

func TestLatencyDisabled(t *testing.T) {
	cfg := Generate("18444", "16060", "18317", false)

	// HTTP latency should be disabled
	if cfg.Latency.HTTP.Enabled != nil && *cfg.Latency.HTTP.Enabled != false {
		t.Error("HTTP latency should be disabled in lab")
	}

	// ICMP latency should be disabled
	if cfg.Latency.ICMP.Enabled != nil && *cfg.Latency.ICMP.Enabled != false {
		t.Error("ICMP latency should be disabled in lab")
	}
}

func TestFakeToarischAbsentInRealMode(t *testing.T) {
	cfg := Generate("18444", "16060", "18317", false)

	// Verify no fake target ID exists
	for _, target := range cfg.Targets {
		if target.ID == "fake-tovarisch" {
			t.Error("fake-tovarisch should not exist in real mode")
		}
	}

	// Verify no fake peer name exists
	for _, peer := range cfg.Diagnostics.Peers {
		if peer.Name == "fake-tovarisch-peer" {
			t.Error("fake-tovarisch-peer should not exist in real mode")
		}
	}
}

func TestConfigJSONSerialize(t *testing.T) {
	cfg := Generate("18444", "16060", "18317", false)

	// Marshal to JSON and back
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("should serialize to JSON: %v", err)
	}

	var cfg2 Config
	if err := json.Unmarshal(data, &cfg2); err != nil {
		t.Fatalf("should deserialize from JSON: %v", err)
	}

	// Verify key fields preserved
	if cfg2.Targets[0].ID != cfg.Targets[0].ID {
		t.Error("target ID should be preserved")
	}
	if cfg2.Diagnostics.PProf.Listen != cfg.Diagnostics.PProf.Listen {
		t.Error("pprof listen should be preserved")
	}
}

func TestRealConfigFileForUVB76(t *testing.T) {
	// Create a temp file to write the config
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.json")

	cfg := Generate("18444", "16060", "18317", false)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("should marshal: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("should write file: %v", err)
	}

	// Load using production config loader
	prodCfg, err := config.LoadWithOptions(configPath, config.ValidationOptions{AllowMissingTLS: true})
	if err != nil {
		t.Fatalf("production config should load generated config: %v", err)
	}

	// Verify loaded config has correct values
	if prodCfg.Targets[0].ID != "real-tovarisch" {
		t.Errorf("loaded config should have real-tovarisch target, got %q", prodCfg.Targets[0].ID)
	}
}
