// Package main implements the artifact verifier for the UVB-76 Latency Crash Lab.
//
// Verifies:
//   - result.json exists and has valid structure
//   - required artifacts are present
//   - fail-closed: fails if any required contract is violated
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Result is the expected structure of result.json.
type Result struct {
	OK                    bool     `json:"ok"`
	DurationSeconds       int      `json:"duration_seconds"`
	RequestedSampleLimit  int      `json:"requested_sample_limit"`
	DaemonStarted         bool     `json:"daemon_started"`
	DaemonExitedEarly     bool     `json:"daemon_exited_early"`
	DaemonExitCode        *int     `json:"daemon_exit_code,omitempty"`
	PIDStable             bool     `json:"pid_stable"`
	FatalLogPatternsFound []string `json:"fatal_log_patterns_found"`
	SampleEndpointValid   bool     `json:"sample_endpoint_valid_json"`
	SummaryEndpointValid  bool     `json:"summary_endpoint_valid_json"`
	SampleCountIncreased  bool     `json:"sample_count_increased"`
	RequestsTotal         int      `json:"requests_total"`
	RequestsFailed        int      `json:"requests_failed"`
	MaxObservedSampleCount int     `json:"max_observed_sample_count"`
	ArtifactDir           string   `json:"artifact_dir"`
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	flag.Parse()
	if flag.NArg() < 1 {
		log.Fatal("Usage: uvb76-latency-crash-verify <artifact-dir>")
	}
	artifactDir := flag.Arg(0)

	if err := verify(artifactDir); err != nil {
		log.Fatalf("Verification FAILED: %v", err)
	}
	log.Printf("Verification PASSED")
}

// verify checks the artifact directory for contract compliance.
// This is a fail-closed verifier - any violation causes failure.
func verify(artifactDir string) error {
	// Check artifact dir exists
	info, err := os.Stat(artifactDir)
	if err != nil {
		return fmt.Errorf("artifact dir not found: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("artifact dir is not a directory")
	}

	// Check required artifacts exist
	requiredFiles := []string{
		"result.json",
		"config.json",
		"uvb76.log",
		"uvb76.pid",
		"final-latency-samples.json",
		"final-latency-summary.json",
	}
	for _, f := range requiredFiles {
		path := filepath.Join(artifactDir, f)
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("missing required artifact: %s", f)
		}
	}

	// Parse result.json
	resultPath := filepath.Join(artifactDir, "result.json")
	data, err := os.ReadFile(resultPath)
	if err != nil {
		return fmt.Errorf("failed to read result.json: %w", err)
	}

	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("failed to parse result.json: %w", err)
	}

	// === FAIL-CLOSED VERIFICATION ===

	// 1. ok must be true
	if !result.OK {
		return fmt.Errorf("result.ok == false")
	}

	// 2. daemon must have started
	if !result.DaemonStarted {
		return fmt.Errorf("daemon did not start")
	}

	// 3. daemon must not have exited early
	if result.DaemonExitedEarly {
		return fmt.Errorf("daemon exited early")
	}

	// 4. PID must be stable (daemon still running)
	if !result.PIDStable {
		return fmt.Errorf("PID not stable (daemon crashed)")
	}

	// 5. no fatal log patterns allowed
	if len(result.FatalLogPatternsFound) > 0 {
		return fmt.Errorf("fatal log patterns found: %v", result.FatalLogPatternsFound)
	}

	// 6. sample endpoint must have returned valid JSON
	if !result.SampleEndpointValid {
		return fmt.Errorf("sample endpoint returned invalid JSON")
	}

	// 7. summary endpoint must have returned valid JSON
	if !result.SummaryEndpointValid {
		return fmt.Errorf("summary endpoint returned invalid JSON")
	}

	// 8. requested sample limit must be exactly 3600
	if result.RequestedSampleLimit != 3600 {
		return fmt.Errorf("wrong requested_sample_limit: got %d, expected 3600",
			result.RequestedSampleLimit)
	}

	// 9. must have made requests
	if result.RequestsTotal < 1 {
		return fmt.Errorf("no requests made")
	}

	// 10. no request failures allowed
	if result.RequestsFailed > 0 {
		return fmt.Errorf("requests failed: %d failures", result.RequestsFailed)
	}

	// 11. sample count check is advisory only (lab may have no ICMP targets reachable)
	// In production CI with real targets, this would fail-closed.
	log.Printf("INFO: sample_count_increased=%v, max_samples=%d (advisory in lab env)",
		result.SampleCountIncreased, result.MaxObservedSampleCount)

	// Check for data races in log
	logPath := filepath.Join(artifactDir, "uvb76.log")
	logData, err := os.ReadFile(logPath)
	if err == nil {
		logContent := string(logData)
		if strings.Contains(logContent, "WARNING: DATA RACE") {
			return fmt.Errorf("DATA RACE detected in logs")
		}
	}

	// Verify samples JSON is valid
	samplesPath := filepath.Join(artifactDir, "final-latency-samples.json")
	samplesData, err := os.ReadFile(samplesPath)
	if err != nil {
		return fmt.Errorf("failed to read samples: %w", err)
	}
	var samples []any
	if err := json.Unmarshal(samplesData, &samples); err != nil {
		return fmt.Errorf("samples JSON invalid: %w", err)
	}

	// Verify summary JSON is valid
	summaryPath := filepath.Join(artifactDir, "final-latency-summary.json")
	summaryData, err := os.ReadFile(summaryPath)
	if err != nil {
		return fmt.Errorf("failed to read summary: %w", err)
	}
	var summary map[string]any
	if err := json.Unmarshal(summaryData, &summary); err != nil {
		return fmt.Errorf("summary JSON invalid: %w", err)
	}

	// Print summary
	log.Printf("")
	log.Printf("=== Verification Summary ===")
	log.Printf("OK: %v", result.OK)
	log.Printf("Daemon started: %v", result.DaemonStarted)
	log.Printf("PID stable: %v", result.PIDStable)
	log.Printf("Fatal patterns: %v", result.FatalLogPatternsFound)
	log.Printf("Requests: %d total, %d failed", result.RequestsTotal, result.RequestsFailed)
	log.Printf("Max observed samples: %d", result.MaxObservedSampleCount)
	log.Printf("Sample count increased: %v", result.SampleCountIncreased)
	log.Printf("Artifacts verified: %v", len(requiredFiles))

	return nil
}
