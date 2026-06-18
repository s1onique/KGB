package config

import (
	"os"
	"testing"
)

func TestValidatePasswordHashFormat_Valid(t *testing.T) {
	hash := "sha256:00000000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000"
	err := ValidatePasswordHashFormat(hash)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidatePasswordHashFormat_Empty(t *testing.T) {
	err := ValidatePasswordHashFormat("")
	if err != ErrEmptyPasswordFormat {
		t.Errorf("expected ErrEmptyPasswordFormat, got %v", err)
	}
}

func TestValidatePasswordHashFormat_WrongPrefix(t *testing.T) {
	hash := "md5:00000000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000"
	err := ValidatePasswordHashFormat(hash)
	if err != ErrInvalidPasswordFormat {
		t.Errorf("expected ErrInvalidPasswordFormat, got %v", err)
	}
}

func TestValidatePasswordHashFormat_TooShort(t *testing.T) {
	hash := "sha256:abc"
	err := ValidatePasswordHashFormat(hash)
	if err != ErrInvalidPasswordFormat {
		t.Errorf("expected ErrInvalidPasswordFormat, got %v", err)
	}
}

func TestValidatePasswordHashFormat_WrongHashLength(t *testing.T) {
	hash := "sha256:00000000000000000000000000000000:00000000000000000000000000000000"
	err := ValidatePasswordHashFormat(hash)
	if err != ErrInvalidHashLength {
		t.Errorf("expected ErrInvalidHashLength, got %v", err)
	}
}

func TestValidatePasswordHashFormat_InvalidSaltHex(t *testing.T) {
	hash := "sha256:zzzz0000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000"
	err := ValidatePasswordHashFormat(hash)
	if err != ErrInvalidSaltHex {
		t.Errorf("expected ErrInvalidSaltHex, got %v", err)
	}
}

func TestValidatePasswordHashFormat_InvalidHashHex(t *testing.T) {
	hash := "sha256:00000000000000000000000000000000:zzzz0000000000000000000000000000000000000000000000000000000000000"
	err := ValidatePasswordHashFormat(hash)
	if err == nil {
		t.Error("expected error for invalid hash hex")
	}
}

func TestLoad_MissingUsername(t *testing.T) {
	content := `{
		"listen": {"addr": ":8443", "tls_cert_file": "/certs/server.crt", "tls_key_file": "/certs/server.key"},
		"auth": {"username": "", "password_sha256": "sha256:00000000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000"},
		"scrape": {"interval_seconds": 60, "timeout_milliseconds": 5000},
		"targets": []
	}`
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for missing username")
	}
}

func TestLoad_MissingPassword(t *testing.T) {
	content := `{
		"listen": {"addr": ":8443", "tls_cert_file": "/certs/server.crt", "tls_key_file": "/certs/server.key"},
		"auth": {"username": "admin", "password_sha256": ""},
		"scrape": {"interval_seconds": 60, "timeout_milliseconds": 5000},
		"targets": []
	}`
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for missing password")
	}
}

func TestLoad_InvalidPasswordFormat(t *testing.T) {
	content := `{
		"listen": {"addr": ":8443", "tls_cert_file": "/certs/server.crt", "tls_key_file": "/certs/server.key"},
		"auth": {"username": "admin", "password_sha256": "invalid-format"},
		"scrape": {"interval_seconds": 60, "timeout_milliseconds": 5000},
		"targets": []
	}`
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for invalid password format")
	}
}
