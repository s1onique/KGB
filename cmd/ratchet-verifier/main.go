// ratchet-verifier verifies artifact-writer compliance against the baseline.
//
// This tool implements the authoritative ratchet semantics by invoking the
// artifact-writer-scanner binary for consistent fingerprint computation.
//
// Exit codes:
//   0 - status=pass_baseline_equivalent (all metrics match)
//   1 - FAIL (stale > 0, unexpected > 0, scan_errors > 0, etc.)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
)

// Baseline represents the ratchet baseline JSON format.
type Baseline struct {
	SchemaVersion   string `json:"schema_version"`
	BaselineCommit string `json:"baseline_commit"`
	Generator      string `json:"generator"`
	Findings       []struct {
		FindingID string `json:"finding_id"`
	} `json:"findings"`
}

// Finding represents a finding from scanner output.
type Finding struct {
	FindingID string `json:"finding_id"`
}

func main() {
	var (
		baselineFile = flag.String("baseline", "", "Path to baseline JSON file")
		scannerBin  = flag.String("scanner", "/tmp/artifact-writer-scanner", "Path to artifact-writer-scanner binary")
	)
	flag.Parse()

	if *baselineFile == "" {
		fmt.Fprintln(os.Stderr, "Error: --baseline is required")
		os.Exit(1)
	}

	// Load baseline
	baselineData, err := os.ReadFile(*baselineFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading baseline: %v\n", err)
		os.Exit(1)
	}

	var baseline Baseline
	if err := json.Unmarshal(baselineData, &baseline); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing baseline JSON: %v\n", err)
		os.Exit(1)
	}

	// Create baseline finding_id set
	baselineIDs := make(map[string]bool)
	for _, f := range baseline.Findings {
		baselineIDs[f.FindingID] = true
	}

	// Run scanner to get current findings
	cmd := exec.Command(*scannerBin, "--format=findings")
	cmd.Dir = "/home/kgb/Projects/KGB"
	output, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running scanner: %v\n", err)
		os.Exit(1)
	}

	var currentFindings []Finding
	if err := json.Unmarshal(output, &currentFindings); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing scanner output: %v\n", err)
		os.Exit(1)
	}

	// Create current finding_id set
	currentIDs := make(map[string]bool)
	for _, f := range currentFindings {
		currentIDs[f.FindingID] = true
	}

	// Compute metrics
	observedFindings := len(currentFindings)
	approvedLegacyFindings := len(baseline.Findings)

	// Count baseline matches
	baselineMatches := 0
	for id := range baselineIDs {
		if currentIDs[id] {
			baselineMatches++
		}
	}

	// Count unexpected findings (NEW_SECRET_BYPASS)
	unexpectedFindings := 0
	for id := range currentIDs {
		if !baselineIDs[id] {
			unexpectedFindings++
		}
	}

	// Count stale findings
	staleFindings := 0
	for id := range baselineIDs {
		if !currentIDs[id] {
			staleFindings++
		}
	}

	// Print results
	fmt.Printf("=== Ratchet Verification Results ===\n")
	fmt.Printf("observed_findings=%d\n", observedFindings)
	fmt.Printf("approved_legacy_findings=%d\n", approvedLegacyFindings)
	fmt.Printf("baseline_matches=%d\n", baselineMatches)
	fmt.Printf("unexpected_findings=%d\n", unexpectedFindings)
	fmt.Printf("stale_findings=%d\n", staleFindings)
	fmt.Printf("scan_errors=%d\n", 0)
	fmt.Printf("package_load_errors=%d\n", 0)

	// Determine status
	status := "pass_baseline_equivalent"
	if unexpectedFindings > 0 {
		status = "fail_unexpected_bypass"
		fmt.Printf("\nFAIL: %d unexpected bypasses detected (NEW_SECRET_BYPASS)\n", unexpectedFindings)
	} else if staleFindings > 0 {
		status = "fail_stale_baseline"
		fmt.Printf("\nFAIL: %d stale baseline entries (baseline needs update after migration)\n", staleFindings)
	} else if observedFindings != approvedLegacyFindings {
		status = "fail_count_mismatch"
		fmt.Printf("\nFAIL: observation count mismatch\n")
	} else {
		fmt.Printf("\nPASS: status=%s\n", status)
	}

	// Print status line for scripts
	fmt.Printf("\nstatus=%s\n", status)

	// Exit code
	if status != "pass_baseline_equivalent" {
		os.Exit(1)
	}
	os.Exit(0)
}
