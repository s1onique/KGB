package certgen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateSelfSigned(t *testing.T) {
	dir, err := os.MkdirTemp("", "certgen-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	files, err := GenerateSelfSigned(dir)
	if err != nil {
		t.Fatalf("GenerateSelfSigned failed: %v", err)
	}

	if files.CertFile == "" || files.KeyFile == "" {
		t.Error("CertFile or KeyFile is empty")
	}

	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	if _, err := os.Stat(certPath); err != nil {
		t.Errorf("cert.pem not created: %v", err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Errorf("key.pem not created: %v", err)
	}

	if err := ValidateCertFiles(certPath, keyPath); err != nil {
		t.Errorf("ValidateCertFiles failed: %v", err)
	}
}

func TestValidateCertFiles_InvalidPaths(t *testing.T) {
	err := ValidateCertFiles("/nonexistent/cert.pem", "/nonexistent/key.pem")
	if err == nil {
		t.Error("Expected error for non-existent files")
	}
}

func TestCertError(t *testing.T) {
	err := &CertError{msg: "test error"}
	if err.Error() != "test error" {
		t.Errorf("CertError.Error() = %q, want %q", err.Error(), "test error")
	}
}
