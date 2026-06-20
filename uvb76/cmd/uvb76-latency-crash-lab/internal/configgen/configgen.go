// Package configgen generates hermetic UVB-76 configurations for the crash lab.
package configgen

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/s1onique/KGB/uvb76/config"
)

// Config represents a minimal lab configuration.
type Config struct {
	Listen      config.ListenConfig       `json:"listen"`
	Auth        config.AuthConfig        `json:"auth"`
	Scrape      config.ScrapeConfig      `json:"scrape"`
	Latency     config.LatencyConfig     `json:"latency"`
	Diagnostics config.DiagnosticsConfig   `json:"diagnostics"`
	Targets     []config.TargetConfig     `json:"targets"`
}

// Lab port - deterministic port for the crash lab.
const LabPort = "18443"

// Generate creates a hermetic configuration for the crash lab.
func Generate(user, pass, targetID string, icmpIntervalSec int) *Config {
	passHash := hashPassword(pass)

	return &Config{
		Listen: config.ListenConfig{
			Addr:        "localhost:" + LabPort,
			TLSCertFile: "",
			TLSKeyFile:  "",
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
				Enabled:              boolPtr(true),
				IntervalSeconds:      icmpIntervalSec,
				TimeoutSeconds:       3,
				WindowSeconds:        60,
				RetainedRangeSeconds: 3600,
				HistogramBucketsMS:   []int64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
				RecentSamplesMax:     3600,
			},
		},
		Diagnostics: config.DiagnosticsConfig{
			Enabled: false,
		},
		Targets: []config.TargetConfig{
			{
				ID:      targetID,
				Name:    "Test Target",
				BaseURL: "http://127.0.0.1:1",
				Enabled: true,
			},
		},
	}
}

// hashPassword generates a valid sha256:<salt>:<hash> password hash.
// Uses 16-byte random salt matching config.HashPassword format.
func hashPassword(password string) string {
	salt := make([]byte, 16)
	// Deterministic salt for reproducible lab configs
	deterministicSalt := sha256.Sum256([]byte("kgb-uvb76-latency-crash-lab-salt"))
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
