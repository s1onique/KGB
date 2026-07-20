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

	"github.com/s1onique/KGB/internal/artifactwriterbaseline"
)

// Finding represents a finding from scanner output.
type Finding struct {
	FindingID string `json:"finding_id"`
}

func main() {
	var (
		baselineDir = flag.String("baseline-dir", "", "Path to baseline directory with manifest.json")
		scannerBin  = flag.String("scanner", "", "Path to artifact-writer-scanner binary")
		repoRoot   = flag.String("repo-root", "", "Repository root containing the source tree to scan")
	)
	flag.Parse()

	if *baselineDir == "" {
		fmt.Fprintln(os.Stderr, "Error: --baseline-dir is required")
		os.Exit(1)
	}

	if *scannerBin == "" {
		fmt.Fprintln(os.Stderr, "Error: --scanner is required")
		os.Exit(1)
	}

	if *repoRoot == "" {
		fmt.Fprintln(os.Stderr, "Error: --repo-root is required")
		os.Exit(1)
	}

	// Validate repo root is a directory
	info, err := os.Stat(*repoRoot)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: --repo-root %q is not a valid directory\n", *repoRoot)
		os.Exit(1)
	}

	// Load baseline using the production loader
	baselineFindings, err := artifactwriterbaseline.LoadAll(*baselineDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading baseline: %v\n", err)
		os.Exit(1)
	}

	// Create baseline finding_id set
	baselineIDs := make(map[string]bool)
	for _, f := range baselineFindings {
		baselineIDs[f.FindingID] = true
	}

	// Run scanner to get current findings
	cmd := exec.Command(*scannerBin, "--format=findings")
	cmd.Dir = *repoRoot
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
	approvedLegacyFindings := len(baselineFindings)

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
