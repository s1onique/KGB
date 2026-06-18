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
