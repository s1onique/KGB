package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	content := `{
		"listen": {"addr": ":8443", "tls_cert_file": "/certs/server.crt", "tls_key_file": "/certs/server.key"},
		"auth": {"username": "admin", "password_sha256": "sha256:00000000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000"},
		"scrape": {"interval_seconds": 60, "timeout_milliseconds": 5000},
		"latency": {"http": {}, "icmp": {}},
		"targets": [{"id": "test-target", "name": "Test Target", "base_url": "https://192.168.1.1:8443", "enabled": true}]
	}`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Listen.Addr != ":8443" {
		t.Errorf("expected listen addr :8443, got %s", cfg.Listen.Addr)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte("not json"), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoad_MissingListenAddr(t *testing.T) {
	content := `{
		"listen": {"addr": "", "tls_cert_file": "/certs/server.crt", "tls_key_file": "/certs/server.key"},
		"auth": {"username": "admin", "password_sha256": "sha256:00000000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000"},
		"scrape": {"interval_seconds": 60, "timeout_milliseconds": 5000},
		"targets": []
	}`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for missing listen addr")
	}
}

func TestLoad_MissingTLSCert(t *testing.T) {
	content := `{
		"listen": {"addr": ":8443", "tls_cert_file": "", "tls_key_file": "/certs/server.key"},
		"auth": {"username": "admin", "password_sha256": "sha256:00000000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000"},
		"scrape": {"interval_seconds": 60, "timeout_milliseconds": 5000},
		"targets": []
	}`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for missing TLS cert")
	}
}

func TestLoad_InvalidInterval(t *testing.T) {
	content := `{
		"listen": {"addr": ":8443", "tls_cert_file": "/certs/server.crt", "tls_key_file": "/certs/server.key"},
		"auth": {"username": "admin", "password_sha256": "sha256:00000000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000"},
		"scrape": {"interval_seconds": 0, "timeout_milliseconds": 5000},
		"targets": []
	}`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for invalid interval")
	}
}

// TestLoad_ZeroedLatencyConfigDefaults verifies that LatencyConfig.ApplyDefaults()
// correctly populates zeroed fields from a loaded config.
// NOTE: Load() itself does NOT apply latency defaults - it returns raw parsed JSON.
// main.go is responsible for calling ApplyDefaults() and writing the result back
// to cfg.Latency before passing the config to the server. This test exercises
// ApplyDefaults() to verify the default values are correct.
func TestLoad_ZeroedLatencyConfigDefaults(t *testing.T) {
	content := `{
		"listen": {"addr": ":8443", "tls_cert_file": "/certs/server.crt", "tls_key_file": "/certs/server.key"},
		"auth": {"username": "admin", "password_sha256": "sha256:00000000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000"},
		"scrape": {"interval_seconds": 60, "timeout_milliseconds": 5000},
		"latency": {"http": {}, "icmp": {}},
		"targets": [{"id": "test-target", "name": "Test Target", "base_url": "https://192.168.1.1:8443", "enabled": true}]
	}`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Apply defaults (simulating what main.go should do)
	cfg.Latency.ApplyDefaults()

	// HTTP defaults
	if cfg.Latency.HTTP.IntervalSeconds != DefaultHTTPIntervalSeconds {
		t.Errorf("expected HTTP interval=%d, got %d", DefaultHTTPIntervalSeconds, cfg.Latency.HTTP.IntervalSeconds)
	}
	if cfg.Latency.HTTP.TimeoutMilliseconds != DefaultHTTPTimeoutMilliseconds {
		t.Errorf("expected HTTP timeout=%d, got %d", DefaultHTTPTimeoutMilliseconds, cfg.Latency.HTTP.TimeoutMilliseconds)
	}
	if cfg.Latency.HTTP.WindowSeconds != DefaultHTTPWindowSeconds {
		t.Errorf("expected HTTP window=%d, got %d", DefaultHTTPWindowSeconds, cfg.Latency.HTTP.WindowSeconds)
	}
	if cfg.Latency.HTTP.RetainedRangeSeconds <= 0 {
		t.Errorf("expected HTTP retained_range > 0, got %d", cfg.Latency.HTTP.RetainedRangeSeconds)
	}
	if len(cfg.Latency.HTTP.HistogramBucketsMS) == 0 {
		t.Error("expected HTTP histogram buckets to be populated")
	}

	// ICMP defaults
	if cfg.Latency.ICMP.IntervalSeconds != DefaultICMPIntervalSeconds {
		t.Errorf("expected ICMP interval=%d, got %d", DefaultICMPIntervalSeconds, cfg.Latency.ICMP.IntervalSeconds)
	}
	if cfg.Latency.ICMP.TimeoutSeconds != DefaultICMPTimeoutSeconds {
		t.Errorf("expected ICMP timeout=%d, got %d", DefaultICMPTimeoutSeconds, cfg.Latency.ICMP.TimeoutSeconds)
	}
	if cfg.Latency.ICMP.WindowSeconds != DefaultICMPWindowSeconds {
		t.Errorf("expected ICMP window=%d, got %d", DefaultICMPWindowSeconds, cfg.Latency.ICMP.WindowSeconds)
	}
	if cfg.Latency.ICMP.RetainedRangeSeconds <= 0 {
		t.Errorf("expected ICMP retained_range > 0, got %d", cfg.Latency.ICMP.RetainedRangeSeconds)
	}
	if len(cfg.Latency.ICMP.HistogramBucketsMS) == 0 {
		t.Error("expected ICMP histogram buckets to be populated")
	}

	// Validation should pass with defaults
	if err := ValidateLatencyConfig(cfg.Latency); err != nil {
		t.Errorf("validation failed after defaults: %v", err)
	}
}

func TestLoad_DevMode_AllowsMissingTLS(t *testing.T) {
	content := `{
		"listen": {"addr": ":8080", "tls_cert_file": "", "tls_key_file": ""},
		"auth": {"username": "admin", "password_sha256": "sha256:00000000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000"},
		"scrape": {"interval_seconds": 60, "timeout_milliseconds": 5000},
		"targets": []
	}`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := LoadWithOptions(configPath, ValidationOptions{AllowMissingTLS: true})
	if err != nil {
		t.Fatalf("LoadWithOptions dev mode failed: %v", err)
	}
	if cfg.Listen.Addr != ":8080" {
		t.Errorf("expected listen addr :8080, got %s", cfg.Listen.Addr)
	}
}
