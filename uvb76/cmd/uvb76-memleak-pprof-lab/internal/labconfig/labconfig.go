// Package labconfig generates hermetic UVB-76 configurations for the pprof lab.
package labconfig

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/s1onique/KGB/uvb76/config"
)

// Sentinel error for credential generation failures.
var ErrCredentialGeneration = errors.New("credential generation failed")

// ErrCredentialEmpty is returned when a generated credential is empty.
var ErrCredentialEmpty = errors.New("generated credential is empty")

// ErrCredentialGeneratorNil is returned when a nil generator is provided.
var ErrCredentialGeneratorNil = errors.New("credential generator is nil")

// Config represents the lab configuration structure.
type Config struct {
	Listen      config.ListenConfig      `json:"listen"`
	Auth        config.AuthConfig        `json:"auth"`
	Scrape      config.ScrapeConfig      `json:"scrape"`
	Latency     config.LatencyConfig     `json:"latency"`
	Diagnostics config.DiagnosticsConfig `json:"diagnostics"`
	Targets     []config.TargetConfig    `json:"targets"`
}

// GeneratedConfigResult holds both the serializable config and the ephemeral secrets.
// P0-1: Ephemeral secrets must not be serialized to disk.
type GeneratedConfigResult struct {
	Config            *Config
	EphemeralPassword []byte `json:"-"` // Never serialized - cleared after auth
}

// CredentialGenerator defines the interface for generating credentials.
type CredentialGenerator interface {
	Generate() ([]byte, error)
}

// CryptoCredentialGenerator generates credentials using crypto/rand.
type CryptoCredentialGenerator struct {
	Read func([]byte) (int, error)
}

// NewCryptoCredentialGenerator creates a generator with the production reader.
func NewCryptoCredentialGenerator() *CryptoCredentialGenerator {
	return &CryptoCredentialGenerator{
		Read: rand.Read,
	}
}

// Generate creates a new per-run credential using crypto/rand.
// P0-1: Allocates 32 random bytes, encodes as base64.RawURLEncoding.
// P0-1C: Raw buffer is cleared via defer immediately after allocation.
// P0-1E: Nil receiver and nil Read function are handled gracefully.
func (g *CryptoCredentialGenerator) Generate() ([]byte, error) {
	// P0-1E: Check for nil receiver (typed-nil interface case)
	if g == nil {
		return nil, fmt.Errorf("random read: %w", ErrCredentialGeneratorNil)
	}

	raw := make([]byte, 32)
	// P0-1C: Defer immediately after allocation - runs on ALL return paths
	defer clearBytes(raw)

	// P0-1E: Check for nil Read function
	if g.Read == nil {
		return nil, fmt.Errorf("random read: %w", ErrCredentialGeneratorNil)
	}

	n, err := g.Read(raw)
	if err != nil {
		return nil, fmt.Errorf("random read: %w", err)
	}
	if n != len(raw) {
		return nil, fmt.Errorf("random short read: got=%d want=%d", n, len(raw))
	}

	// Encode as HTTP/JSON-safe textual representation
	encoded := make([]byte, base64.RawURLEncoding.EncodedLen(len(raw)))
	base64.RawURLEncoding.Encode(encoded, raw)

	return encoded, nil
}

// Generate creates a hermetic configuration for the pprof memory lab.
// When useFakeTovarisch is true, targets the fake server.
// When false, targets the real Tovarisch /status endpoint.
// Returns the config AND the ephemeral password that must not be persisted.
// P0-1: Uses cryptographic random credential generation per run.
// P0-1: Returns error if credential generation fails - no fallback.
func Generate(uvb76Port, pprofPort, tovarischPort string, useFakeTovarisch bool) (*GeneratedConfigResult, error) {
	return GenerateWithGenerator(uvb76Port, pprofPort, tovarischPort, useFakeTovarisch, NewCryptoCredentialGenerator())
}

// clearBytes zeros a byte slice in place.
func clearBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// GenerateWithGenerator creates configuration using the provided credential generator.
// P0-1: Allows injection of deterministic readers for testing.
// P0-1: Returns error if credential generation fails - no fallback.
func GenerateWithGenerator(uvb76Port, pprofPort, tovarischPort string, useFakeTovarisch bool, gen CredentialGenerator) (*GeneratedConfigResult, error) {
	// P0-1E: Reject nil generator without panicking
	if gen == nil {
		return nil, errors.Join(ErrCredentialGeneration, ErrCredentialGeneratorNil)
	}

	// P0-1: Generate per-run credential - fail closed if generation fails
	plaintext, err := gen.Generate()
	if err != nil {
		// P0-1D: Clear any partial data before returning
		clearBytes(plaintext)
		return nil, errors.Join(ErrCredentialGeneration, err)
	}
	// P0-1: Reject empty credentials from defective generators
	if len(plaintext) == 0 {
		return nil, errors.Join(ErrCredentialGeneration, ErrCredentialEmpty)
	}

	passHash := hashPassword(plaintext)

	targetID := "real-tovarisch"
	targetName := "Real Tovarisch Status Endpoint"
	peerName := "real-tovarisch-peer"
	peerBaseURL := "http://localhost:" + tovarischPort

	if useFakeTovarisch {
		targetID = "fake-tovarisch"
		targetName = "Fake Tovarisch Status Endpoint"
		peerName = "fake-tovarisch-peer"
	}

	return &GeneratedConfigResult{
		Config: &Config{
			Listen: config.ListenConfig{
				Addr:        "localhost:" + uvb76Port,
				TLSCertFile: "",
				TLSKeyFile:  "",
			},
			Auth: config.AuthConfig{
				Username:       "lab-user",
				PasswordSHA256: passHash,
			},
			Scrape: config.ScrapeConfig{
				IntervalSeconds:     1, // Short interval for smoke test
				TimeoutMilliseconds: 5000,
			},
			Latency: config.LatencyConfig{
				HTTP: config.HTTPProbeConfig{
					Enabled:              boolPtr(false),
					IntervalSeconds:      15,
					TimeoutMilliseconds:  10000,
					WindowSeconds:        60,
					RetainedRangeSeconds: 3600,
					HistogramBucketsMS:   []int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000},
					RecentSamplesMax:     100,
				},
				ICMP: config.ICMPProbeConfig{
					Enabled:              boolPtr(false),
					IntervalSeconds:      1,
					TimeoutSeconds:       3,
					WindowSeconds:        60,
					RetainedRangeSeconds: 3600,
					HistogramBucketsMS:   []int64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
					RecentSamplesMax:     3600,
				},
			},
			// pprof is enabled; diagnostics peer config is required
			Diagnostics: config.DiagnosticsConfig{
				Enabled: true,
				Peers: []config.DiagPeerConfig{
					{
						Name:    peerName,
						BaseURL: peerBaseURL,
						Targets: []string{targetID},
					},
				},
				PProf: config.PProfConfig{
					Enabled:        true,
					Listen:         "localhost:" + pprofPort,
					MemProfileRate: 65536,
				},
			},
			Targets: []config.TargetConfig{
				{
					ID:      targetID,
					Name:    targetName,
					BaseURL: "http://localhost:" + tovarischPort,
					Enabled: true,
				},
			},
		},
		EphemeralPassword: plaintext,
	}, nil
}

// hashPassword generates a valid sha256:<salt>:<hash> password hash.
func hashPassword(password []byte) string {
	salt := make([]byte, 16)
	// Deterministic salt for reproducible lab configs
	deterministicSalt := sha256.Sum256([]byte("kgb-uvb76-pprof-lab-salt"))
	copy(salt, deterministicSalt[:16])

	h := sha256.New()
	h.Write(salt)
	h.Write(password)
	hash := h.Sum(nil)

	return "sha256:" + hex.EncodeToString(salt) + ":" + hex.EncodeToString(hash)
}

func boolPtr(b bool) *bool {
	return &b
}
