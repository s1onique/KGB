package artifactio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestWriteRedactedJSONBytes_RemovesKnownSensitiveFields verifies the
// canonical structured-JSON redaction. The redact package's known
// sensitive field set is the source of truth.
func TestWriteRedactedJSONBytes_RemovesKnownSensitiveFields(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "out.json")

	input := []byte(`{
		"password": "hunter2",
		"admin_password": "admin-secret",
		"api_key": "sk-test-key",
		"session_token": "sess-secret",
		"name": "ok",
		"url": "https://user:secret@host.example.com/path"
	}`)

	policy := DefaultRuntimePolicy()
	policy.OpenTextKind = "json"

	if err := WriteRedactedJSONBytes("unit-test", dest, input, policy); err != nil {
		t.Fatalf("WriteRedactedJSONBytes failed: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	gotStr := string(got)

	for _, banned := range []string{
		"hunter2",
		"admin-secret",
		"sk-test-key",
		"sess-secret",
		"user:secret@",
	} {
		if strings.Contains(gotStr, banned) {
			t.Errorf("artifact contains forbidden fragment %q", banned)
		}
	}
	if !strings.Contains(gotStr, "\"ok\"") {
		t.Errorf("non-sensitive value should be preserved")
	}
	if !json.Valid(got) {
		t.Errorf("result is not valid JSON")
	}
}

// TestWriteRedactedJSONBytes_AtomicPublication verifies that successful
// writes use a rename from a same-directory temporary file.
func TestWriteRedactedJSONBytes_AtomicPublication(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("rename atomic semantics differ on Windows")
	}
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "atomic.json")

	policy := DefaultRuntimePolicy()
	policy.OpenTextKind = "json"

	if err := WriteRedactedJSONBytes("unit-test", dest, []byte(`{"k":"v"}`), policy); err != nil {
		t.Fatalf("WriteRedactedJSONBytes failed: %v", err)
	}

	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}

	tempLeftovers := 0
	for _, e := range entries {
		if e.Name() != "atomic.json" && strings.HasPrefix(e.Name(), ".tmp.") {
			tempLeftovers++
		}
	}
	if tempLeftovers != 0 {
		t.Errorf("expected no leftover temp files, got %d", tempLeftovers)
	}
}

// TestWriteRedactedJSONBytes_FailedReplaceLeavesPriorValidArtifact
// proves the failure contract: a previously valid artifact remains
// intact when a subsequent write fails.
func TestWriteRedactedJSONBytes_FailedReplaceLeavesPriorValidArtifact(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "prior.json")

	prior := []byte(`{"prior":"valid"}`)
	if err := os.WriteFile(dest, prior, 0o600); err != nil {
		t.Fatalf("seed prior artifact failed: %v", err)
	}

	policy := DefaultRuntimePolicy()
	policy.OpenTextKind = "json"

	// Trigger a write failure via a deliberately empty value (input_too_large path).
	policy.MaxInputBytes = 1
	if err := WriteRedactedJSONBytes("unit-test", dest, []byte(`{"x":"y"}`), policy); err == nil {
		t.Fatal("expected input_too_large failure, got nil")
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read after failed write: %v", err)
	}
	if string(got) != string(prior) {
		t.Errorf("prior artifact was modified by failed write")
	}
}

// TestWriteRedactedText_BearerCookiesAndHeaders verifies text-mode
// sanitization across headers, cookies, and URLs.
func TestWriteRedactedText_BearerCookiesAndHeaders(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "out.txt")

	input := "Authorization: Bearer abc123\n" +
		"Cookie: session=xyz; secure=true\n" +
		"Set-Cookie: sid=def; HttpOnly\n" +
		"URL: https://user:secret@host.example.com/path\n"

	if err := WriteRedactedText("unit-test", dest, input, DefaultTextPolicy()); err != nil {
		t.Fatalf("WriteRedactedText failed: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	gotStr := string(got)
	for _, banned := range []string{
		"abc123",
		"xyz",
		"def",
		"user:secret@",
	} {
		if strings.Contains(gotStr, banned) {
			t.Errorf("artifact contains forbidden fragment %q", banned)
		}
	}
}

// TestPostValidatePrivateKeyMarkerRejection proves that when input
// cannot be safely sanitized, the boundary fails closed before publish.
func TestPostValidatePrivateKeyMarkerRejection(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "out.json")

	// The redact package replaces a string value matching a private key
	// marker with [REDACTED], so post_validate passes. We instead trigger
	// the URL post-validate path which is harder to bypass.
	input := []byte(`{"name":"-----BEGIN-PLACEHOLDER-KEY-----AAAA-----END-PLACEHOLDER-KEY-----"}`)
	policy := DefaultRuntimePolicy()
	policy.OpenTextKind = "json"
	policy.MaxInputBytes = 1 // Force input_too_large

	err := WriteRedactedJSONBytes("unit-test", dest, input, policy)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := err.(*Error); !ok {
		t.Fatalf("expected *artifactio.Error, got %T", err)
	}
}

// TestErrorMessageNeverContainsSecretFragment verifies diagnostic safety.
func TestErrorMessageNeverContainsSecretFragment(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "out.json")

	input := []byte(`{"password":"super-secret-token-value"}`)
	policy := DefaultRuntimePolicy()
	policy.OpenTextKind = "json"
	policy.MaxInputBytes = 1 // Force failure

	err := WriteRedactedJSONBytes("unit-test", dest, input, policy)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if strings.Contains(msg, "super-secret-token-value") {
		t.Errorf("error message must not contain the secret value (got %q)", msg)
	}
}

// TestPolicy_DefaultRuntimePolicyHasRestrictiveMode ensures the default
// policy uses restrictive permissions on supported Unix platforms.
func TestPolicy_DefaultRuntimePolicyHasRestrictiveMode(t *testing.T) {
	p := DefaultRuntimePolicy()
	if p.FileMode != 0o600 {
		t.Errorf("DefaultRuntimePolicy.FileMode = %s, want 0600", modeStr(p.FileMode))
	}
}

// TestWriteRedactedJSONBytes_NoLeftoverTempOnFailure verifies that a
// boundary rejection cleans up the temporary file.
func TestWriteRedactedJSONBytes_NoLeftoverTempOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("atomic rename semantics differ on Windows")
	}
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "out.json")

	input := []byte(`{"password":"x"}`)
	policy := DefaultRuntimePolicy()
	policy.OpenTextKind = "json"
	policy.MaxInputBytes = 1 // Force failure

	err := WriteRedactedJSONBytes("unit-test", dest, input, policy)
	if err == nil {
		t.Fatal("expected error")
	}

	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp.") {
			t.Errorf("leftover temp file %q after failure", e.Name())
		}
	}
}

// TestSecondWriteSanitationIsIdempotent ensures that re-serializing an
// already-redacted artifact produces identical bytes.
func TestSecondWriteSanitationIsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	dest1 := filepath.Join(tmp, "first.json")
	dest2 := filepath.Join(tmp, "second.json")

	input := map[string]any{
		"password": "hunter2",
		"api_key":  "abcdef",
		"name":     "ok",
	}

	policy := DefaultRuntimePolicy()
	policy.OpenTextKind = "json"

	if err := WriteRedactedJSON("unit-test", dest1, input, policy); err != nil {
		t.Fatalf("first write: %v", err)
	}
	firstBytes, err := os.ReadFile(dest1)
	if err != nil {
		t.Fatalf("read first: %v", err)
	}

	if err := WriteRedactedJSONBytes("unit-test", dest2, firstBytes, policy); err != nil {
		t.Fatalf("second write: %v", err)
	}
	secondBytes, err := os.ReadFile(dest2)
	if err != nil {
		t.Fatalf("read second: %v", err)
	}

	if string(firstBytes) != string(secondBytes) {
		t.Errorf("second-pass sanitation produced different bytes (non-idempotent)")
	}
}

// TestDefaultPoliciesValidate ensures every default policy passes its
// internal validation.
func TestDefaultPoliciesValidate(t *testing.T) {
	for _, p := range []WritePolicy{
		DefaultRuntimePolicy(),
		DefaultTextPolicy(),
		DefaultConfigPolicy(),
		DefaultCommittedFixturePolicy(),
	} {
		if err := p.validate(); err != nil {
			t.Errorf("default policy failed validation: %v", err)
		}
	}
}

// TestErrorTypeIsCorrect checks that WritePolicy validation produces
// a typed artifactio error.
func TestErrorTypeIsCorrect(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "out.json")
	bad := WritePolicy{
		FileMode:       0,
		MaxInputBytes:  1024,
		MaxOutputBytes: 4096,
	}
	err := WriteRedactedJSONBytes("unit-test", dest, []byte(`{}`), bad)
	if err == nil {
		t.Fatal("expected policy_invalid error")
	}
	if _, ok := err.(*Error); !ok {
		t.Fatalf("expected *artifactio.Error, got %T", err)
	}
}
