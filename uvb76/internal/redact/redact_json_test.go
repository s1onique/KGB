package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

// ============================================================================
// JSON Redaction Tests
// ============================================================================

func TestRedactStructuredJSON_NestedObjects(t *testing.T) {
	input := []byte(`{
		"user": {
			"name": "admin",
			"credentials": {
				"password": "secret123",
				"api_key": "sk-api-key"
			}
		},
		"public_data": "visible"
	}`)

	result, err := RedactStructuredJSON(input)
	if err != nil {
		t.Fatalf("RedactStructuredJSON failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	// Check nested password is redacted
	user := parsed["user"].(map[string]interface{})
	creds := user["credentials"].(map[string]interface{})
	if creds["password"] != Redacted {
		t.Errorf("Nested password not redacted: %v", creds["password"])
	}
	if creds["api_key"] != Redacted {
		t.Errorf("Nested api_key not redacted: %v", creds["api_key"])
	}
	// public_data should be preserved
	if parsed["public_data"] != "visible" {
		t.Errorf("public_data was modified: %v", parsed["public_data"])
	}
}

func TestRedactStructuredJSON_Arrays(t *testing.T) {
	input := []byte(`{
		"users": [
			{"name": "alice", "password": "secret1"},
			{"name": "bob", "password": "secret2"}
		]
	}`)

	result, err := RedactStructuredJSON(input)
	if err != nil {
		t.Fatalf("RedactStructuredJSON failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	users := parsed["users"].([]interface{})
	for i, u := range users {
		user := u.(map[string]interface{})
		if user["password"] != Redacted {
			t.Errorf("Array element %d password not redacted: %v", i, user["password"])
		}
	}
}

func TestRedactStructuredJSON_InvalidJSON(t *testing.T) {
	input := []byte(`{not valid json`)

	result, err := RedactStructuredJSON(input)
	if err == nil {
		if string(result) == string(input) {
			t.Errorf("Invalid JSON should not return original bytes unchanged: %s", result)
		}
	}
}

func TestRedactStructuredJSON_EmptyInput(t *testing.T) {
	result, err := RedactStructuredJSON([]byte{})
	if err != nil {
		t.Fatalf("Empty input should not error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Empty input should return empty result: %s", result)
	}
}

func TestRedactStructuredJSON_PrivateKeyInString(t *testing.T) {
	// Use dynamic fixture generation to avoid literal PEM markers in source
	// testPrivateKey() is defined in redact_diagnostic_test.go in the same package
	keyMarker := testPrivateKey()
	input := []byte(`{"key": "` + keyMarker + `\nMIIE...\n-----END PRIVATE KEY-----"}`)

	result, err := RedactStructuredJSON(input)
	if err != nil {
		t.Fatalf("RedactStructuredJSON failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	if parsed["key"] != Redacted {
		t.Errorf("Private key in string not redacted: %v", parsed["key"])
	}
}

func TestRedactStructuredJSON_CompleteExample(t *testing.T) {
	input := []byte(`{
		"server": {
			"host": "localhost",
			"port": 8080
		},
		"auth": {
			"password_sha256": "sha256:abc123:def456",
			"session_key": "session_secret_abc123"
		},
		"proxy": {
			"host": "proxy.example.com",
			"port": 8443
		}
	}`)

	result, err := RedactStructuredJSON(input)
	if err != nil {
		t.Fatalf("RedactStructuredJSON failed: %v", err)
	}

	resultStr := string(result)
	sensitive := []string{
		"abc123",
		"def456",
		"session_secret_abc123",
	}

	for _, s := range sensitive {
		if strings.Contains(resultStr, s) {
			t.Errorf("Sensitive value %q found in output:\n%s", s, resultStr)
		}
	}

	// Verify safe values are preserved
	if !strings.Contains(resultStr, "localhost") {
		t.Errorf("Safe value 'localhost' not preserved in output:\n%s", resultStr)
	}
	if !strings.Contains(resultStr, "proxy.example.com") {
		t.Errorf("Safe value 'proxy.example.com' not preserved in output:\n%s", resultStr)
	}
}
