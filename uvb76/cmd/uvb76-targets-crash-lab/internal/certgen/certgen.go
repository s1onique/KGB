package certgen

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

type CertFiles struct {
	CertFile string
	KeyFile  string
}

func GenerateSelfSigned(dir string) (*CertFiles, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{Organization: []string{"KGB Lab"}, CommonName: "localhost"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost", "127.0.0.1"},
		IPAddresses:           []net.IP{{127, 0, 0, 1}},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}

	certFile := filepath.Join(dir, "cert.pem")
	certOut, err := os.Create(certFile)
	if err != nil {
		return nil, err
	}
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()
	if err != nil {
		return nil, err
	}

	keyFile := filepath.Join(dir, "key.pem")
	keyOut, err := os.Create(keyFile)
	if err != nil {
		return nil, err
	}
	privBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	err = pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})
	keyOut.Close()
	if err != nil {
		return nil, err
	}

	return &CertFiles{CertFile: certFile, KeyFile: keyFile}, nil
}

func ValidateCertFiles(certFile, keyFile string) error {
	certData, err := os.ReadFile(certFile)
	if err != nil {
		return err
	}
	_, err = os.ReadFile(keyFile)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(certData)
	if block == nil || block.Type != "CERTIFICATE" {
		return &CertError{"no PEM block in cert file"}
	}
	_, err = x509.ParseCertificate(block.Bytes)
	return err
}

type CertError struct{ msg string }

func (e *CertError) Error() string { return e.msg }
