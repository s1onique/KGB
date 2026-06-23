// tls_config.go — Native Go TLS certificate generation for memory labs
//
// Generates ephemeral self-signed localhost TLS certificates using Go's
// crypto/x509 and crypto/tls packages. No shell/openssl required.
//
// This enables memory-lab to run UVB-76 (which requires TLS in production mode)
// without checking in test certificates or weakening production validation.
//
// Reference: kgb://doctrine/native-owned-critical-paths

package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// TLSCertFiles holds paths to generated certificate and key files.
type TLSCertFiles struct {
	CertFile string
	KeyFile  string
}

// GenerateEphemeralCert generates a self-signed localhost certificate/key pair.
// Returns paths to the generated cert and key files under artifactDir.
func GenerateEphemeralCert(artifactDir, service string) (*TLSCertFiles, error) {
	// Ensure artifact directory exists
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return nil, fmt.Errorf("create artifact dir: %w", err)
	}

	// Generate RSA private key (2048-bit for good security)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate RSA key: %w", err)
	}

	// Build certificate template for localhost self-signed cert
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"KGB Memory Lab"},
			CommonName:   "localhost",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour), // 24-hour validity for lab use
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:              []string{"localhost"},
	}

	// Self-signed: parent cert is the template itself
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	// Write certificate file (PEM format)
	certPath := filepath.Join(artifactDir, fmt.Sprintf("%s-localhost.crt", service))
	certOut, err := os.Create(certPath)
	if err != nil {
		return nil, fmt.Errorf("create cert file: %w", err)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return nil, fmt.Errorf("encode cert PEM: %w", err)
	}

	// Write private key file (PEM format) with restricted permissions (0600 = owner read/write only)
	keyPath := filepath.Join(artifactDir, fmt.Sprintf("%s-localhost.key", service))
	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("create key file: %w", err)
	}
	defer keyOut.Close()

	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}); err != nil {
		return nil, fmt.Errorf("encode key PEM: %w", err)
	}

	return &TLSCertFiles{
		CertFile: certPath,
		KeyFile:  keyPath,
	}, nil
}

// LoadTLSCertPair loads a certificate/key pair from files.
// Returns a tls.Certificate suitable for HTTPS servers.
func LoadTLSCertPair(certFile, keyFile string) (tls.Certificate, error) {
	return tls.LoadX509KeyPair(certFile, keyFile)
}

// NewInsecureTLSCertPool creates an x509.CertPool containing the given certificate.
// This is for lab use only - allows HTTPS client to trust self-signed localhost cert.
func NewInsecureTLSCertPool(certFile string) (*x509.CertPool, error) {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("read cert file: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certPEM) {
		return nil, fmt.Errorf("no certificates added from PEM")
	}

	return pool, nil
}

// WriteDerivedConfig writes a UVB-76 config file with TLS paths populated.
// Returns the path to the derived config file.
// Uses proper JSON unmarshal/mutate/marshal to avoid fragile string replacement.
func WriteDerivedConfig(artifactDir, service, sourceConfigPath string, tlsFiles *TLSCertFiles) (string, error) {
	// Read source config
	data, err := os.ReadFile(sourceConfigPath)
	if err != nil {
		return "", fmt.Errorf("read source config: %w", err)
	}

	// Unmarshal JSON into generic map for typed mutation
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("parse config JSON: %w", err)
	}

	// Mutate listen section with TLS paths
	listen, ok := cfg["listen"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("config missing listen section")
	}
	listen["tls_cert_file"] = tlsFiles.CertFile
	listen["tls_key_file"] = tlsFiles.KeyFile

	// Marshal back to JSON
	derived, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal derived config: %w", err)
	}

	// Write derived config
	derivedPath := filepath.Join(artifactDir, fmt.Sprintf("%s.memory-lab.derived.json", service))
	if err := os.WriteFile(derivedPath, derived, 0644); err != nil {
		return "", fmt.Errorf("write derived config: %w", err)
	}

	return derivedPath, nil
}
