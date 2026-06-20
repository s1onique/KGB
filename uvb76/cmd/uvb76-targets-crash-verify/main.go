// Package main implements the artifact verifier for the UVB-76 Targets Crash Lab.
//
// Verifies:
//   - summary.json exists and has valid structure
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

// Summary represents the expected structure of summary.json.
type Summary struct {
	Status         string `json:"status"`
	StartedAt      string `json:"started_at"`
	CompletedAt    string `json:"completed_at"`
	DurationSecs   int    `json:"duration_seconds"`
	Workers        int    `json:"workers"`
	RequestCount   int    `json:"request_count"`
	SuccessCount   int    `json:"success_count"`
	ErrorCount     int    `json:"error_count"`
	Mode           string `json:"mode"`
	HTTP2Disabled  bool   `json:"http2_disabled"`
	ProcessExited  bool   `json:"process_exited"`
	ProcessExitCode *int  `json:"process_exit_code,omitempty"`
	SawSIGSEGV     bool   `json:"saw_sigsegv"`
	SawPanic       bool   `json:"saw_panic"`
	SawFatalError  bool   `json:"saw_fatal_error"`
	SampleResponsePath string `json:"sample_response_path"`
	ArtifactDir    string `json:"artifact_dir"`
}

// TargetResponse represents a target from the sample response.
type TargetResponse struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	DiagnosticPeerName  string `json:"diagnostic_peer_name,omitempty"`
	DiagnosticBaseURL   string `json:"diagnostic_base_url,omitempty"`
	EffectiveCaptureURL string `json:"effective_capture_url,omitempty"`
}

// ConfigJSON represents the lab's serialized config artifact.
type ConfigJSON struct {
	Diagnostics struct {
		Peers []struct {
			Name                  string `json:"name"`
			EffectiveCaptureURL   string `json:"effective_capture_url"`
		} `json:"peers"`
	} `json:"diagnostics"`
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	flag.Parse()
	if flag.NArg() < 1 {
		log.Fatal("Usage: uvb76-targets-crash-verify <artifact-dir>")
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
		"summary.json",
		"config.json",
		"cert.pem",
		"key.pem",
		"uvb76.stdout.log",
		"uvb76.stderr.log",
		"targets-response-sample.json",
		"workload-summary.json",
	}
	for _, f := range requiredFiles {
		path := filepath.Join(artifactDir, f)
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("missing required artifact: %s", f)
		}
	}

	// Parse summary.json
	summaryPath := filepath.Join(artifactDir, "summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		return fmt.Errorf("failed to read summary.json: %w", err)
	}

	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		return fmt.Errorf("failed to parse summary.json: %w", err)
	}

	// === FAIL-CLOSED VERIFICATION ===

	// 1. status must be "pass"
	if summary.Status != "pass" {
		return fmt.Errorf("summary.status = %q, want %q", summary.Status, "pass")
	}

	// 2. request_count must be > 0
	if summary.RequestCount <= 0 {
		return fmt.Errorf("request_count = %d, must be > 0", summary.RequestCount)
	}

	// 3. success_count must be > 0
	if summary.SuccessCount <= 0 {
		return fmt.Errorf("success_count = %d, must be > 0", summary.SuccessCount)
	}

	// 4. error_count must be 0
	if summary.ErrorCount != 0 {
		return fmt.Errorf("error_count = %d, want 0", summary.ErrorCount)
	}

	// 5. process_exited must be false
	if summary.ProcessExited {
		return fmt.Errorf("process_exited = true, want false")
	}

	// 6. saw_sigsegv must be false
	if summary.SawSIGSEGV {
		return fmt.Errorf("saw_sigsegv = true, crash detected")
	}

	// 7. saw_panic must be false
	if summary.SawPanic {
		return fmt.Errorf("saw_panic = true, panic detected")
	}

	// 8. saw_fatal_error must be false
	if summary.SawFatalError {
		return fmt.Errorf("saw_fatal_error = true, fatal error detected")
	}

	// 9. sample response must exist and contain expected diagnostic fields
	samplePath := filepath.Join(artifactDir, "targets-response-sample.json")
	sampleData, err := os.ReadFile(samplePath)
	if err != nil {
		return fmt.Errorf("failed to read sample response: %w", err)
	}

	var targets []TargetResponse
	if err := json.Unmarshal(sampleData, &targets); err != nil {
		return fmt.Errorf("sample response is not valid JSON: %w", err)
	}

	if len(targets) < 2 {
		return fmt.Errorf("sample response has %d targets, want at least 2", len(targets))
	}

	// Find the diagnostic target
	var diagTarget *TargetResponse
	for i := range targets {
		if targets[i].ID == "target-with-diag" {
			diagTarget = &targets[i]
			break
		}
	}

	if diagTarget == nil {
		return fmt.Errorf("diagnostic target 'target-with-diag' not found in sample")
	}

	// Verify diagnostic fields
	if diagTarget.DiagnosticPeerName == "" {
		return fmt.Errorf("DiagnosticPeerName is empty in sample response")
	}
	if diagTarget.DiagnosticBaseURL == "" {
		return fmt.Errorf("DiagnosticBaseURL is empty in sample response")
	}
	if diagTarget.EffectiveCaptureURL == "" {
		return fmt.Errorf("EffectiveCaptureURL is empty in sample response")
	}

	// 10. Verify expected target IDs exist
	targetIDs := make(map[string]bool)
	for _, t := range targets {
		targetIDs[t.ID] = true
	}
	if !targetIDs["target-with-diag"] {
		return fmt.Errorf("target 'target-with-diag' not found")
	}
	if !targetIDs["target-plain"] {
		return fmt.Errorf("target 'target-plain' not found")
	}

	// Check for DATA RACE in logs
	for _, logFile := range []string{"uvb76.stdout.log", "uvb76.stderr.log"} {
		logPath := filepath.Join(artifactDir, logFile)
		logData, err := os.ReadFile(logPath)
		if err == nil {
			logContent := string(logData)
			if strings.Contains(logContent, "WARNING: DATA RACE") {
				return fmt.Errorf("DATA RACE detected in %s", logFile)
			}
		}
	}

	// 11. Verify config.json artifact contains precomputed EffectiveCaptureURL
	configPath := filepath.Join(artifactDir, "config.json")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config.json: %w", err)
	}

	var cfg ConfigJSON
	if err := json.Unmarshal(configData, &cfg); err != nil {
		return fmt.Errorf("config.json is not valid JSON: %w", err)
	}

	if len(cfg.Diagnostics.Peers) == 0 {
		return fmt.Errorf("config.json has no diagnostic peers")
	}
	if cfg.Diagnostics.Peers[0].EffectiveCaptureURL == "" {
		return fmt.Errorf("config.json diagnostic peer EffectiveCaptureURL is empty (precomputation failed)")
	}
	log.Printf("Config EffectiveCaptureURL verified: %s", cfg.Diagnostics.Peers[0].EffectiveCaptureURL)

	// Print summary
	log.Printf("")
	log.Printf("=== Verification Summary ===")
	log.Printf("Status: %s", summary.Status)
	log.Printf("Mode: %s", summary.Mode)
	log.Printf("HTTP/2 disabled: %v", summary.HTTP2Disabled)
	log.Printf("Workers: %d", summary.Workers)
	log.Printf("Requests: %d total, %d success, %d errors", summary.RequestCount, summary.SuccessCount, summary.ErrorCount)
	log.Printf("Process exited: %v", summary.ProcessExited)
	log.Printf("SIGSEGV: %v, Panic: %v, Fatal: %v", summary.SawSIGSEGV, summary.SawPanic, summary.SawFatalError)
	log.Printf("Diagnostic target validated: %v", diagTarget != nil)
	log.Printf("EffectiveCaptureURL present: %v", diagTarget != nil && diagTarget.EffectiveCaptureURL != "")
	log.Printf("Artifacts verified: %d files", len(requiredFiles))

	return nil
}
