package config

import (
	"os"
	"testing"
)

func TestValidateTargetBaseURLScheme_ValidHTTP(t *testing.T) {
	err := ValidateTargetBaseURLScheme("http://example.com:8080/path")
	if err != nil {
		t.Errorf("expected no error for http, got %v", err)
	}
}

func TestValidateTargetBaseURLScheme_ValidHTTPS(t *testing.T) {
	err := ValidateTargetBaseURLScheme("https://example.com:8443/path")
	if err != nil {
		t.Errorf("expected no error for https, got %v", err)
	}
}

func TestValidateTargetBaseURLScheme_MissingScheme(t *testing.T) {
	err := ValidateTargetBaseURLScheme("example.com:8080")
	if err == nil {
		t.Error("expected error for missing scheme")
	}
}

func TestValidateTargetBaseURLScheme_InvalidScheme(t *testing.T) {
	err := ValidateTargetBaseURLScheme("ftp://example.com")
	if err == nil {
		t.Error("expected error for invalid scheme")
	}
}

func TestValidateTargetBaseURLScheme_Empty(t *testing.T) {
	err := ValidateTargetBaseURLScheme("")
	if err == nil {
		t.Error("expected error for empty URL")
	}
}

func TestLoad_TargetMissingID(t *testing.T) {
	content := `{"listen": {"addr": ":8443", "tls_cert_file": "/certs/server.crt", "tls_key_file": "/certs/server.key"}, "auth": {"username": "admin", "password_sha256": "sha256:00000000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000"}, "scrape": {"interval_seconds": 60, "timeout_milliseconds": 5000}, "targets": [{"id": "", "name": "Test", "base_url": "https://192.168.1.1:8443", "enabled": true}]}`
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for missing target ID")
	}
}

func TestLoad_TargetMissingName(t *testing.T) {
	content := `{"listen": {"addr": ":8443", "tls_cert_file": "/certs/server.crt", "tls_key_file": "/certs/server.key"}, "auth": {"username": "admin", "password_sha256": "sha256:00000000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000"}, "scrape": {"interval_seconds": 60, "timeout_milliseconds": 5000}, "targets": [{"id": "test", "name": "", "base_url": "https://192.168.1.1:8443", "enabled": true}]}`
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for missing target name")
	}
}

func TestLoad_TargetMissingBaseURL(t *testing.T) {
	content := `{"listen": {"addr": ":8443", "tls_cert_file": "/certs/server.crt", "tls_key_file": "/certs/server.key"}, "auth": {"username": "admin", "password_sha256": "sha256:00000000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000"}, "scrape": {"interval_seconds": 60, "timeout_milliseconds": 5000}, "targets": [{"id": "test", "name": "Test", "base_url": "", "enabled": true}]}`
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for missing target base_url")
	}
}

func TestLoad_TargetInvalidBaseURLScheme(t *testing.T) {
	content := `{"listen": {"addr": ":8443", "tls_cert_file": "/certs/server.crt", "tls_key_file": "/certs/server.key"}, "auth": {"username": "admin", "password_sha256": "sha256:00000000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000"}, "scrape": {"interval_seconds": 60, "timeout_milliseconds": 5000}, "targets": [{"id": "test", "name": "Test", "base_url": "ftp://192.168.1.1:8443", "enabled": true}]}`
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for invalid base_url scheme")
	}
}

func TestLoad_DuplicateTargetID(t *testing.T) {
	content := `{"listen": {"addr": ":8443", "tls_cert_file": "/certs/server.crt", "tls_key_file": "/certs/server.key"}, "auth": {"username": "admin", "password_sha256": "sha256:00000000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000"}, "scrape": {"interval_seconds": 60, "timeout_milliseconds": 5000}, "targets": [{"id": "test", "name": "Test 1", "base_url": "https://192.168.1.1:8443", "enabled": true}, {"id": "test", "name": "Test 2", "base_url": "https://192.168.1.2:8443", "enabled": true}]}`
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for duplicate target ID")
	}
}

func TestTargetStatusURL(t *testing.T) {
	tests := []struct {
		baseURL     string
		expectedURL string
	}{
		{"http://example.com", "http://example.com/status"},
		{"https://example.com/", "https://example.com/status"},
		{"http://example.com:8080/path/", "http://example.com:8080/path/status"},
	}
	for _, tc := range tests {
		result := TargetStatusURL(tc.baseURL)
		if result != tc.expectedURL {
			t.Errorf("TargetStatusURL(%q) = %q, want %q", tc.baseURL, result, tc.expectedURL)
		}
	}
}
