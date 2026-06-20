// Package configgen generates hermetic UVB-76 configurations for the targets crash lab.
package configgen

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/s1onique/KGB/uvb76/config"
)

// LabPort is the deterministic port for the targets crash lab.
const LabPort = "19443"

// GenerateHTTPS creates a hermetic HTTPS configuration for the targets crash lab.
// Returns a validated *config.Config that has passed through production validation.
// This ensures diagnostics URLs are precomputed at config load time.
func GenerateHTTPS(user, pass string, certFile, keyFile string) (*config.Config, error) {
	passHash := hashPassword(pass)

	// Build config using the real config.Config type
	cfg := &config.Config{
		Listen: config.ListenConfig{
			Addr:        "localhost:" + LabPort,
			TLSCertFile: certFile,
			TLSKeyFile:  keyFile,
		},
		Auth: config.AuthConfig{
			Username:       user,
			PasswordSHA256: passHash,
		},
		Scrape: config.ScrapeConfig{
			IntervalSeconds:     5,
			TimeoutMilliseconds: 5000,
		},
		Latency: config.LatencyConfig{
			HTTP: config.HTTPProbeConfig{
				Enabled:             boolPtr(false),
				IntervalSeconds:     15,
				TimeoutMilliseconds: 10000,
				WindowSeconds:       60,
				RetainedRangeSeconds: 3600,
				HistogramBucketsMS:  []int64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000},
				RecentSamplesMax:    100,
			},
			ICMP: config.ICMPProbeConfig{
				Enabled:              boolPtr(false),
				IntervalSeconds:      1,
				TimeoutSeconds:       3,
				WindowSeconds:        60,
				RetainedRangeSeconds: 3600,
				HistogramBucketsMS:   []int64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
				RecentSamplesMax:     100,
			},
		},
		Diagnostics: config.DiagnosticsConfig{
			Enabled:         true,
			CaptureOnSpike:  false,
			TimeoutMs:       1500,
			CooldownSeconds: 90,
			MaxUncapturedSpikes: 200,
			Peers: []config.DiagPeerConfig{
				{
					Name:    "diag-peer-home",
					BaseURL: "http://127.0.0.1:19980",
					Targets: []string{"target-with-diag"},
				},
			},
		},
		Targets: []config.TargetConfig{
			{
				ID:      "target-with-diag",
				Name:    "Target With Diagnostic Peer",
				BaseURL: "http://127.0.0.1:19980",
				Enabled: true,
			},
			{
				ID:      "target-plain",
				Name:    "Target Without Diagnostic",
				BaseURL: "http://127.0.0.1:19981",
				Enabled: true,
			},
		},
	}

	// Validate the config - this is critical!
	// It will call PrecomputeCaptureURLs() internally
	if err := cfg.Validate(config.ValidationOptions{}); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	// Verify EffectiveCaptureURL was precomputed
	if len(cfg.Diagnostics.Peers) == 0 {
		return nil, fmt.Errorf("no diagnostic peers configured")
	}
	if cfg.Diagnostics.Peers[0].EffectiveCaptureURL == "" {
		return nil, fmt.Errorf("diagnostic EffectiveCaptureURL was not precomputed")
	}

	return cfg, nil
}

// hashPassword generates a valid sha256:<salt>:<hash> password hash.
func hashPassword(password string) string {
	salt := make([]byte, 16)
	deterministicSalt := sha256.Sum256([]byte("kgb-uvb76-targets-crash-lab-salt-v1"))
	copy(salt, deterministicSalt[:16])

	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(password))
	hash := h.Sum(nil)

	return "sha256:" + hex.EncodeToString(salt) + ":" + hex.EncodeToString(hash)
}

func boolPtr(b bool) *bool {
	return &b
}
