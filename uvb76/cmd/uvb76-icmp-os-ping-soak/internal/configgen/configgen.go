// Package configgen generates hermetic test configurations for the ICMP OS ping soak lab.
package configgen

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/s1onique/KGB/uvb76/config"
)

// LabPort is the fixed port for the soak lab.
const LabPort = "18317"

// Generate creates a hermetic test configuration with ICMP enabled.
func Generate(adminUser, adminPass, targetID string, icmpIntervalSecs, icmpTimeoutSecs, icmpMaxConcurrent int) *config.Config {
	// Generate random credentials
	salt := randomHex(16)
	passHash := randomHex(32)

	cfg := &config.Config{
		Listen: config.ListenConfig{
			Addr:        "localhost:" + LabPort,
			TLSCertFile: "", // Dev mode
			TLSKeyFile:  "", // Dev mode
		},
		Auth: config.AuthConfig{
			Username:       adminUser,
			PasswordSHA256: "sha256:" + salt + ":" + passHash,
		},
		Scrape: config.ScrapeConfig{
			IntervalSeconds:     10,
			TimeoutMilliseconds: 5000,
		},
		Latency: config.LatencyConfig{
			ICMP: config.ICMPProbeConfig{
				Enabled:             boolPtr(true),
				IntervalSeconds:    icmpIntervalSecs,
				TimeoutSeconds:     icmpTimeoutSecs,
				MaxConcurrentOSPing: icmpMaxConcurrent,
				HistogramBucketsMS: config.DefaultHistogramBuckets(),
				RecentSamplesMax:   3600,
				WindowSeconds:      60,
				RetainedRangeSeconds: 3600,
			},
		},
		Targets: []config.TargetConfig{
			{
				ID:      targetID,
				Name:    "ICMP Test Target",
				BaseURL: "http://127.0.0.1:8317/status",
				Enabled: true,
			},
		},
		Diagnostics: config.DiagnosticsConfig{
			Enabled: false,
		},
	}

	// Apply defaults
	cfg.Latency.ICMP.ApplyDefaults()

	return cfg
}

// randomHex generates a random hex string of the given byte length.
func randomHex(byteLen int) string {
	b := make([]byte, byteLen)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}
