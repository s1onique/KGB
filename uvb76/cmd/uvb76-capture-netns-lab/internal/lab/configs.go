package lab

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// generateConfigs generates configuration files for tovarisch and uvb76.
func (o *Orchestrator) generateConfigs(ctx context.Context) error {
	// Tovarisch config: lab mode with probe failure file
	labProbeFailureFile := o.labDir + "/tovarisch-lab-probe-failing"

	tovarischConfig := fmt.Sprintf(`[server]
listen = "0.0.0.0:%d"

[lab]
lab_mode = true
lab_probe_failure_file = "%s"
`, o.Config.TovarischPort, labProbeFailureFile)

	configPath := filepath.Join(o.labDir, "tovarisch.conf")
	if err := os.WriteFile(configPath, []byte(tovarischConfig), 0644); err != nil {
		return fmt.Errorf("write tovarisch config: %w", err)
	}
	log.Printf("Tovarisch config: %s", configPath)

	// UVB-76 config: HTTP probes, diagnostics, target pointing to tovarisch
	probeURL := fmt.Sprintf("http://%s:%d/lab/probe",
		o.Config.TovarischNS.IPCIDR[:len(o.Config.TovarischNS.IPCIDR)-3],
		o.Config.TovarischPort)
	baseURL := fmt.Sprintf("http://%s:%d",
		o.Config.TovarischNS.IPCIDR[:len(o.Config.TovarischNS.IPCIDR)-3],
		o.Config.TovarischPort)

	uvb76Config := map[string]interface{}{
		"listen": map[string]string{
			"addr":          fmt.Sprintf(":%d", o.Config.UVB76Port),
			"tls_cert_file": "",
			"tls_key_file":  "",
		},
		"auth": map[string]string{
			"username":        o.Options.APIUser,
			"password_sha256": "sha256:ad31a00094d25f7b5b3fa5ba2a4998db:ae3908b2ae4825fc884248f29385f4497ca9f3ff0c3d1416c6a216f3a400c4e1",
		},
		"scrape": map[string]int{
			"interval_seconds":     60,
			"timeout_milliseconds": 5000,
		},
		"latency": map[string]interface{}{
			"http": map[string]interface{}{
				"enabled":               true,
				"interval_seconds":      2,
				"timeout_milliseconds":   1000,
				"window_seconds":        60,
				"retained_range_seconds": 120,
				"histogram_buckets_ms":  []int{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000},
				"recent_samples_max":    120,
			},
			"icmp": map[string]interface{}{
				"enabled":               false,
				"interval_seconds":      1,
				"timeout_seconds":        3,
				"window_seconds":         60,
				"retained_range_seconds": 300,
				"histogram_buckets_ms":   []int{1, 5, 10, 25, 50, 100, 250, 500, 1000},
				"recent_samples_max":     300,
			},
		},
		"diagnostics": map[string]interface{}{
			"enabled":              true,
			"capture_on_spike":     true,
			"timeout_ms":           2000,
			"cooldown_seconds":      5,
			"max_uncaptured_spikes": 50,
			"peers": []map[string]interface{}{
				{
					"name":     "tovarisch-lab",
					"base_url": baseURL,
					"targets":  []string{"lab-tovarisch"},
				},
			},
		},
		"targets": []map[string]interface{}{
			{
				"id":       "lab-tovarisch",
				"name":     "Lab Tovarisch",
				"base_url": baseURL,
				"probe_url": probeURL,
				"enabled":  true,
			},
		},
	}

	configJSON, err := json.MarshalIndent(uvb76Config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal uvb76 config: %w", err)
	}

	uvb76ConfigPath := filepath.Join(o.labDir, "uvb76.json")
	if err := os.WriteFile(uvb76ConfigPath, configJSON, 0644); err != nil {
		return fmt.Errorf("write uvb76 config: %w", err)
	}
	log.Printf("UVB-76 config: %s", uvb76ConfigPath)

	return nil
}
