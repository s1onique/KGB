package redact

import (
	"testing"
)

// ============================================================================
// Configuration Field Tests
// ============================================================================

func TestRedactConfigValue_PasswordSha256(t *testing.T) {
	result := RedactConfigValue("password_sha256", "sha256:abc123:def456")
	if result != Redacted {
		t.Errorf("password_sha256 not redacted: got %q, want %q", result, Redacted)
	}
}

func TestRedactConfigValue_SensitiveFields(t *testing.T) {
	fields := []string{
		"password", "admin_password", "password_hash", "admin_password_hash",
		"passwd", "secret", "client_secret", "api_key", "api_token",
		"access_token", "refresh_token", "session_token", "session_id",
		"csrf_token", "bearer_token", "private_key", "private_key_data",
		"client_key_data",
	}

	for _, field := range fields {
		result := RedactConfigValue(field, "secret_value")
		if result != Redacted {
			t.Errorf("%s not redacted: got %q, want %q", field, result, Redacted)
		}
	}
}

func TestRedactConfigValue_SafeFields(t *testing.T) {
	fields := []string{
		"username", "email", "host", "port", "path", "name", "description",
		"enabled", "timeout", "max_retries",
	}

	for _, field := range fields {
		result := RedactConfigValue(field, "some_value")
		if result != "some_value" {
			t.Errorf("%s should not be redacted: got %q, want %q", field, result, "some_value")
		}
	}
}

func TestRedactConfigValue_EmptyField(t *testing.T) {
	result := RedactConfigValue("", "value")
	if result != "value" {
		t.Errorf("Empty field should not modify value: got %q", result)
	}
}

func TestRedactConfigValue_EmptyValue(t *testing.T) {
	result := RedactConfigValue("password", "")
	if result != "" {
		t.Errorf("Empty value should remain empty: got %q", result)
	}
}
