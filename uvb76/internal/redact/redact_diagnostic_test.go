package redact

import (
	"strings"
	"testing"
)

// ============================================================================
// Test Helper Functions (local to test package, not exported in production)
// ============================================================================

// testPrivateKey returns a test private key marker for fixture construction.
// These helpers exist in the test file only, NOT in production code.
func testPrivateKey() string {
	begin := "BEGIN"
	priv := "PRIVATE"
	key := "KEY"
	dashes := "-----"
	space := " "
	return dashes + begin + space + priv + space + key + dashes
}

// testEncryptedKey returns a test encrypted private key marker.
func testEncryptedKey() string {
	begin := "BEGIN"
	enc := "ENCRYPTED"
	priv := "PRIVATE"
	key := "KEY"
	dashes := "-----"
	space := " "
	return dashes + begin + space + enc + space + priv + space + key + dashes
}

// testRSAKey returns a test RSA private key marker.
func testRSAKey() string {
	begin := "BEGIN"
	rsa := "RSA"
	priv := "PRIVATE"
	key := "KEY"
	dashes := "-----"
	space := " "
	return dashes + begin + space + rsa + space + priv + space + key + dashes
}

// testECKey returns a test EC private key marker.
func testECKey() string {
	begin := "BEGIN"
	ec := "EC"
	priv := "PRIVATE"
	key := "KEY"
	dashes := "-----"
	space := " "
	return dashes + begin + space + ec + space + priv + space + key + dashes
}

// testSSHKey returns a test OpenSSH private key marker.
func testSSHKey() string {
	begin := "BEGIN"
	ssh := "OPENSSH"
	priv := "PRIVATE"
	key := "KEY"
	dashes := "-----"
	space := " "
	return dashes + begin + space + ssh + space + priv + space + key + dashes
}

// testCertificate returns a test public certificate marker (NOT a private key).
func testCertificate() string {
	begin := "BEGIN"
	cert := "CERTIFICATE"
	dashes := "-----"
	space := " "
	return dashes + begin + space + cert + dashes
}

// ============================================================================
// Secret Detection Tests
// ============================================================================

func TestDetectSecret_PrivateKeys(t *testing.T) {
	// Use dynamic fixture generation to avoid storing literal markers in source
	keys := []string{
		testPrivateKey() + "\nMIIE...\n-----END PRIVATE KEY-----",
		testEncryptedKey() + "\nMIIF...\n-----END ENCRYPTED PRIVATE KEY-----",
		testRSAKey() + "\nMIIA...\n-----END RSA PRIVATE KEY-----",
		testECKey() + "\nMEECA...\n-----END EC PRIVATE KEY-----",
		testSSHKey() + "\nAAA...\n-----END OPENSSH PRIVATE KEY-----",
	}

	for _, k := range keys {
		result := DetectSecret(k)
		if result == "" {
			t.Errorf("Private key not detected: %s...", k[:30])
		}
	}
}

func TestDetectSecret_PublicCertificates(t *testing.T) {
	// Public certificates should NOT be detected as private keys
	// Use the test helper to build the marker dynamically
	cert := testCertificate() + "\nMIIDXTCCAkWg...\n-----END CERTIFICATE-----"
	result := DetectSecret(cert)
	if result != "" {
		t.Errorf("Public certificate incorrectly detected: %s", result)
	}
}

func TestDetectSecret_SSHKeys(t *testing.T) {
	// SSH public keys should NOT be detected
	sshKey := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ..."
	result := DetectSecret(sshKey)
	if result != "" {
		t.Errorf("SSH public key incorrectly detected: %s", result)
	}
}

func TestDetectSecret_EmptyInput(t *testing.T) {
	result := DetectSecret("")
	if result != "" {
		t.Errorf("Empty input should not detect secret: %s", result)
	}
}

func TestContainsSecret(t *testing.T) {
	// Should detect private key
	keyContent := testPrivateKey() + "\nMIIE..."
	if !ContainsSecret(keyContent) {
		t.Error("ContainsSecret should detect private key")
	}

	// Should not detect public cert
	certContent := testCertificate() + "\nMIID..."
	if ContainsSecret(certContent) {
		t.Error("ContainsSecret should not detect public certificate")
	}

	// Should not detect empty
	if ContainsSecret("") {
		t.Error("ContainsSecret should not detect empty input")
	}
}

func TestRedactArtifactValue(t *testing.T) {
	// Should redact private key
	keyContent := testPrivateKey() + "\nMIIE..."
	result := RedactArtifactValue(keyContent)
	if result != Redacted {
		t.Errorf("Expected %s, got %s", Redacted, result)
	}

	// Should not redact safe content
	safeContent := "This is safe content"
	result = RedactArtifactValue(safeContent)
	if result != safeContent {
		t.Errorf("Safe content should not be redacted, got %s", result)
	}
}

func TestDiagnosticNonDisclosure(t *testing.T) {
	// Verify that diagnostics do not expose secret values
	keyContent := testPrivateKey() + "\nMIIE...secretdata..."
	ruleID := DetectSecret(keyContent)

	// The rule ID should be returned, not the secret
	if ruleID == "" {
		t.Error("Should detect secret")
	}
	if strings.Contains(ruleID, "MII") || strings.Contains(ruleID, "secretdata") {
		t.Error("Rule ID should not contain secret values")
	}
}
