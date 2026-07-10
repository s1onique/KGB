// Package labconfig generates hermetic UVB-76 configurations for the pprof lab.
//
// This config enables pprof diagnostics only for the lab environment,
// keeping pprof disabled by default in normal config paths.
package labconfig

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/s1onique/KGB/uvb76/config"
)

// Config represents the lab configuration structure.
type Config struct {
	Listen      config.ListenConfig      `json:"listen"`
	Auth        config.AuthConfig        `json:"auth"`
	Scrape      config.ScrapeConfig      `json:"scrape"`
	Latency     config.LatencyConfig     `json:"latency"`
	Diagnostics config.DiagnosticsConfig `json:"diagnostics"`
	Targets     []config.TargetConfig    `json:"targets"`
}

// Generate creates a hermetic configuration for the pprof memory lab.
func Generate(uvb76Port, pprofPort, tovarischPort string) *Config {
	passHash := hashPassword("lab-password")

	return &Config{
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
			IntervalSeconds:     30,
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
		// pprof is ONLY enabled here in lab config; disabled by default
		// Include a minimal valid peer config to satisfy diagnostics validation.
		Diagnostics: config.DiagnosticsConfig{
			Enabled: true,
			Peers: []config.DiagPeerConfig{
				{
					Name:    "fake-tovarisch-peer",
					BaseURL: "http://localhost:" + tovarischPort,
					Targets: []string{"fake-tovarisch"},
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
				ID:      "fake-tovarisch",
				Name:    "Fake Tovarisch Status Endpoint",
				BaseURL: "http://localhost:" + tovarischPort + "/status",
				Enabled: true,
			},
		},
	}
}

// hashPassword generates a valid sha256:<salt>:<hash> password hash.
func hashPassword(password string) string {
	salt := make([]byte, 16)
	// Deterministic salt for reproducible lab configs
	deterministicSalt := sha256.Sum256([]byte("kgb-uvb76-pprof-lab-salt"))
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
