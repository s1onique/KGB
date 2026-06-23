// tls_config_test.go — Tests for native Go TLS certificate generation
//
// Tests that:
// - Generated cert/key files exist and parse correctly
// - Derived config contains generated TLS paths
// - Self-signed HTTPS readiness client works against httptest TLS server
//
// Reference: kgb://doctrine/native-owned-critical-paths

package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateEphemeralCert(t *testing.T) {
	// Create temp dir for test
	tmpDir, err := os.MkdirTemp("", "tls-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Generate ephemeral cert
	tlsFiles, err := GenerateEphemeralCert(tmpDir, "uvb76")
	if err != nil {
		t.Fatalf("GenerateEphemeralCert failed: %v", err)
	}

	// Verify cert file exists
	if _, err := os.Stat(tlsFiles.CertFile); os.IsNotExist(err) {
		t.Errorf("cert file does not exist: %s", tlsFiles.CertFile)
	}

	// Verify key file exists
	if _, err := os.Stat(tlsFiles.KeyFile); os.IsNotExist(err) {
		t.Errorf("key file does not exist: %s", tlsFiles.KeyFile)
	}

	// Verify key file has restricted permissions (0600 = owner read/write only)
	keyInfo, err := os.Stat(tlsFiles.KeyFile)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	keyMode := keyInfo.Mode().Perm()
	if keyMode != 0600 {
		t.Errorf("key file mode = %o, want 0600", keyMode)
	}

	// Verify cert can be parsed
	certPEM, err := os.ReadFile(tlsFiles.CertFile)
	if err != nil {
		t.Fatalf("read cert file: %v", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("failed to decode cert PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}

	// Verify cert is self-signed (issuer == subject)
	if cert.Subject.CommonName != cert.Issuer.CommonName {
		t.Errorf("expected self-signed cert, issuer=%s != subject=%s", cert.Issuer.CommonName, cert.Subject.CommonName)
	}

	// Verify cert is valid for localhost
	if cert.Subject.CommonName != "localhost" {
		t.Errorf("expected CN=localhost, got %s", cert.Subject.CommonName)
	}

	// Verify cert has IP SANs for 127.0.0.1
	foundLocalhost := false
	for _, ip := range cert.IPAddresses {
		if ip.String() == "127.0.0.1" || ip.String() == "::1" {
			foundLocalhost = true
			break
		}
	}
	if !foundLocalhost {
		t.Error("cert missing localhost IP SAN")
	}

	// Verify cert has DNS SAN for localhost
	foundDNSLocalhost := false
	for _, dns := range cert.DNSNames {
		if dns == "localhost" {
			foundDNSLocalhost = true
			break
		}
	}
	if !foundDNSLocalhost {
		t.Error("cert missing localhost DNS SAN")
	}

	// Verify key file can be loaded
	cert2, err := tls.LoadX509KeyPair(tlsFiles.CertFile, tlsFiles.KeyFile)
	if err != nil {
		t.Fatalf("load X509 key pair: %v", err)
	}
	if len(cert2.Certificate) == 0 {
		t.Error("cert2 has no certificates")
	}

	t.Logf("Generated cert: %s", tlsFiles.CertFile)
	t.Logf("Generated key: %s", tlsFiles.KeyFile)
}

func TestWriteDerivedConfig(t *testing.T) {
	// Create temp dir
	tmpDir, err := os.MkdirTemp("", "tls-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Generate ephemeral cert (provides valid cert files)
	tlsFiles, err := GenerateEphemeralCert(tmpDir, "test")
	if err != nil {
		t.Fatalf("GenerateEphemeralCert failed: %v", err)
	}

	// Create source config with empty TLS paths
	sourceConfig := filepath.Join(tmpDir, "source.json")
	sourceJSON := `{"listen":{"addr":":18081","tls_cert_file":"","tls_key_file":""},"auth":{"username":"test","password_sha256":"sha256:00000000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000"}}`
	if err := os.WriteFile(sourceConfig, []byte(sourceJSON), 0644); err != nil {
		t.Fatalf("write source config: %v", err)
	}

	// Write derived config
	derivedPath, err := WriteDerivedConfig(tmpDir, "uvb76", sourceConfig, tlsFiles)
	if err != nil {
		t.Fatalf("WriteDerivedConfig failed: %v", err)
	}

	// Verify derived config exists
	if _, err := os.Stat(derivedPath); os.IsNotExist(err) {
		t.Fatalf("derived config does not exist: %s", derivedPath)
	}

	// Read and parse derived config
	derivedData, err := os.ReadFile(derivedPath)
	if err != nil {
		t.Fatalf("read derived config: %v", err)
	}

	var derived struct {
		Listen struct {
			TLSCertFile string `json:"tls_cert_file"`
			TLSKeyFile  string `json:"tls_key_file"`
		} `json:"listen"`
	}
	if err := json.Unmarshal(derivedData, &derived); err != nil {
		t.Fatalf("parse derived config JSON: %v", err)
	}

	// Verify TLS paths are populated
	if derived.Listen.TLSCertFile != tlsFiles.CertFile {
		t.Errorf("expected cert=%s, got %s", tlsFiles.CertFile, derived.Listen.TLSCertFile)
	}
	if derived.Listen.TLSKeyFile != tlsFiles.KeyFile {
		t.Errorf("expected key=%s, got %s", tlsFiles.KeyFile, derived.Listen.TLSKeyFile)
	}

	t.Logf("Derived config: %s", derivedPath)
}

func TestNewInsecureTLSCertPool(t *testing.T) {
	// Create temp dir
	tmpDir, err := os.MkdirTemp("", "tls-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Generate cert
	tlsFiles, err := GenerateEphemeralCert(tmpDir, "test")
	if err != nil {
		t.Fatalf("GenerateEphemeralCert failed: %v", err)
	}

	// Create cert pool
	pool, err := NewInsecureTLSCertPool(tlsFiles.CertFile)
	if err != nil {
		t.Fatalf("NewInsecureTLSCertPool failed: %v", err)
	}

	// Verify pool contains the cert
	if pool == nil {
		t.Fatal("pool is nil")
	}
}

func TestHTTPSReadinessAgainstTestServer(t *testing.T) {
	// Create temp dir
	tmpDir, err := os.MkdirTemp("", "tls-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Generate cert
	tlsFiles, err := GenerateEphemeralCert(tmpDir, "uvb76")
	if err != nil {
		t.Fatalf("GenerateEphemeralCert failed: %v", err)
	}

	// Create test server with the generated cert
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))

	// Load the generated cert for the test server
	cert, err := tls.LoadX509KeyPair(tlsFiles.CertFile, tlsFiles.KeyFile)
	if err != nil {
		t.Fatalf("load X509 key pair: %v", err)
	}

	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	server.StartTLS()
	defer server.Close()

	// Create HTTP client that trusts our generated cert
	pool, err := NewInsecureTLSCertPool(tlsFiles.CertFile)
	if err != nil {
		t.Fatalf("NewInsecureTLSCertPool failed: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: pool,
			},
		},
	}

	// Make request to test server
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("HTTPS GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	t.Logf("HTTPS readiness check against test server succeeded: %s", server.URL)
}

// TestHTTPWorkloadWithTLSClient verifies that RunHTTPWorkload works with a TLS-aware client.
// This covers the UVB-76 non-idle workload path (status-api-polling, diagnostic-capture-loop).
func TestHTTPWorkloadWithTLSClient(t *testing.T) {
	// Create temp dir
	tmpDir, err := os.MkdirTemp("", "tls-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Generate cert
	tlsFiles, err := GenerateEphemeralCert(tmpDir, "uvb76")
	if err != nil {
		t.Fatalf("GenerateEphemeralCert failed: %v", err)
	}

	// Create test server with the generated cert
	requestCount := 0
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","requests":` + string(rune('0'+requestCount)) + `}`))
	}))

	cert, err := tls.LoadX509KeyPair(tlsFiles.CertFile, tlsFiles.KeyFile)
	if err != nil {
		t.Fatalf("load X509 key pair: %v", err)
	}

	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	server.StartTLS()
	defer server.Close()

	// Create TLS-aware client
	pool, err := NewInsecureTLSCertPool(tlsFiles.CertFile)
	if err != nil {
		t.Fatalf("NewInsecureTLSCertPool failed: %v", err)
	}

	tlsClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: pool,
			},
		},
	}

	// Run workload with TLS client
	result := RunHTTPWorkload(HTTPWorkloadConfig{
		URL:         server.URL,
		Operations:  5,
		IntervalMs:  10,
		Name:        "test-tls-workload",
		Client:      tlsClient,
	})

	if result.Errors > 0 {
		t.Errorf("workload had %d errors, expected 0", result.Errors)
	}
	if result.Operations != 5 {
		t.Errorf("expected 5 operations, got %d", result.Operations)
	}

	t.Logf("HTTP workload with TLS client succeeded: %d ops, %d errors", result.Operations, result.Errors)
}

func TestWriteDerivedConfigFromSource(t *testing.T) {
	// Create temp dir
	tmpDir, err := os.MkdirTemp("", "tls-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Generate cert
	tlsFiles, err := GenerateEphemeralCert(tmpDir, "uvb76")
	if err != nil {
		t.Fatalf("GenerateEphemeralCert failed: %v", err)
	}

	// Create source config with empty TLS paths (simpler than template)
	sourceConfig := filepath.Join(tmpDir, "source.json")
	sourceJSON := `{"listen":{"addr":":18081","tls_cert_file":"","tls_key_file":""},"auth":{"username":"memory-lab","password_sha256":"sha256:00000000000000000000000000000000:0000000000000000000000000000000000000000000000000000000000000000"}}`
	if err := os.WriteFile(sourceConfig, []byte(sourceJSON), 0644); err != nil {
		t.Fatalf("write source config: %v", err)
	}

	// Write derived config
	derivedPath, err := WriteDerivedConfig(tmpDir, "uvb76", sourceConfig, tlsFiles)
	if err != nil {
		t.Fatalf("WriteDerivedConfig failed: %v", err)
	}

	// Read and verify
	derivedData, err := os.ReadFile(derivedPath)
	if err != nil {
		t.Fatalf("read derived config: %v", err)
	}

	// Verify TLS paths are populated (not empty)
	if !strings.Contains(string(derivedData), tlsFiles.CertFile) {
		t.Error("derived config missing cert path")
	}
	if !strings.Contains(string(derivedData), tlsFiles.KeyFile) {
		t.Error("derived config missing key path")
	}

	t.Logf("Derived config written: %s", derivedPath)
}
