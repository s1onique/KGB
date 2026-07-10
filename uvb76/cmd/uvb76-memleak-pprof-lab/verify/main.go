// Package main implements the artifact verifier for the pprof memory lab.
//
// This verifier checks the artifact directory contract:
// - Required files exist and are non-empty
// - manifest.json parses with correct schema version
// - verdict.json parses
// - CSV headers match expected format
// - baseline and final heap profiles exist
// - pprof diff reports exist
// - classification is one of allowed values
// - duration-aware: checks final profile based on manifest duration
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Allowed classifications
var allowedClassifications = map[string]bool{
	"suspected_go_heap_retention": true,
	"rss_growth_heap_stable":      true,
	"goroutine_growth":            true,
	"no_material_growth":          true,
	"inconclusive":                true,
}

// Manifest represents the manifest.json structure
type Manifest struct {
	SchemaVersion   int    `json:"schema_version"`
	Classification  string `json:"classification"`
	DurationSeconds int    `json:"duration_seconds"`
	ArtifactDir     string `json:"artifact_dir"`
}

// Verdict represents the verdict.json structure
type Verdict struct {
	Summary        string   `json:"summary"`
	RSSGrowthBytes int64    `json:"rss_growth_bytes"`
	Reasons        []string `json:"reasons"`
}

func main() {
	artifactDir := flag.String("artifact-dir", "", "artifact directory to verify (required)")
	flag.Parse()

	if *artifactDir == "" {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "Error: --artifact-dir is required")
		os.Exit(1)
	}

	errors := verify(*artifactDir)

	if len(errors) > 0 {
		fmt.Fprintln(os.Stderr, "=== Verification Failed ===")
		for _, err := range errors {
			fmt.Fprintf(os.Stderr, "  - %s\n", err)
		}
		os.Exit(1)
	}

	fmt.Println("=== Verification Passed ===")
	os.Exit(0)
}

func verify(dir string) []string {
	var errors []string

	// Read manifest for duration-aware checks
	var manifest Manifest
	manifestPath := filepath.Join(dir, "manifest.json")
	if data, err := os.ReadFile(manifestPath); err == nil {
		if err := json.Unmarshal(data, &manifest); err != nil {
			errors = append(errors, fmt.Sprintf("manifest.json: invalid JSON: %v", err))
		}
	}

	// Compute final suffix from duration (t%03d format)
	finalSuffix := "t600" // default
	if manifest.DurationSeconds > 0 {
		finalSuffix = fmt.Sprintf("t%03d", manifest.DurationSeconds)
	}

	// Check core required files (always required)
	coreRequired := []string{
		"manifest.json",
		"verdict.json",
		"rss-series.csv",
		"goroutine-count-series.csv",
		"heap-t000.pb.gz",
		"uvb76.log",
		"uvb76-lab-config.json",
		"result.json",
		"goroutine-t000.txt",
		fmt.Sprintf("goroutine-%s.txt", finalSuffix),
	}
	for _, fname := range coreRequired {
		fpath := filepath.Join(dir, fname)
		info, err := os.Stat(fpath)
		if err != nil {
			if os.IsNotExist(err) {
				errors = append(errors, fmt.Sprintf("required file missing: %s", fname))
			} else {
				errors = append(errors, fmt.Sprintf("error checking %s: %v", fname, err))
			}
			continue
		}
		if info.Size() == 0 {
			errors = append(errors, fmt.Sprintf("required file is empty: %s", fname))
		}
	}

	// Check tovarisch.log (required for default fake tovarisch mode)
	tovarischLog := filepath.Join(dir, "tovarisch.log")
	if info, err := os.Stat(tovarischLog); err != nil {
		if os.IsNotExist(err) {
			errors = append(errors, "tovarisch.log missing (required for default fake tovarisch mode)")
		} else {
			errors = append(errors, fmt.Sprintf("error checking tovarisch.log: %v", err))
		}
	} else if info.Size() == 0 {
		errors = append(errors, "tovarisch.log is empty")
	}

	// Verify manifest.json
	if data, err := os.ReadFile(manifestPath); err == nil {
		var m struct {
			SchemaVersion int `json:"schema_version"`
		}
		if err := json.Unmarshal(data, &m); err != nil {
			errors = append(errors, fmt.Sprintf("manifest.json: invalid JSON: %v", err))
		} else if m.SchemaVersion != 1 {
			errors = append(errors, fmt.Sprintf("manifest.json: expected schema_version=1, got %d", m.SchemaVersion))
		}
	}

	// Verify verdict.json
	verdictPath := filepath.Join(dir, "verdict.json")
	if data, err := os.ReadFile(verdictPath); err == nil {
		var v Verdict
		if err := json.Unmarshal(data, &v); err != nil {
			errors = append(errors, fmt.Sprintf("verdict.json: invalid JSON: %v", err))
		}
	}

	// Verify result.json structure
	resultPath := filepath.Join(dir, "result.json")
	if data, err := os.ReadFile(resultPath); err == nil {
		var r struct {
			OK                 bool `json:"ok"`
			UVB76Started       bool `json:"uvb76_started"`
			PProfReachable     bool `json:"pprof_reachable"`
			TovarischReachable bool `json:"tovarisch_reachable"`
			CollectorSucceeded bool `json:"collector_succeeded"`
			PProfDiffSucceeded bool `json:"pprof_diff_succeeded"`
			ManifestValid      bool `json:"manifest_valid"`
			VerdictValid       bool `json:"verdict_valid"`
		}
		if err := json.Unmarshal(data, &r); err != nil {
			errors = append(errors, fmt.Sprintf("result.json: invalid JSON: %v", err))
		} else {
			// Require all critical booleans to be true for a passing lab
			if !r.OK {
				errors = append(errors, "result.json: ok field is false")
			}
			if !r.UVB76Started {
				errors = append(errors, "result.json: uvb76_started is false")
			}
			if !r.PProfReachable {
				errors = append(errors, "result.json: pprof_reachable is false")
			}
			if !r.TovarischReachable {
				errors = append(errors, "result.json: tovarisch_reachable is false")
			}
			if !r.CollectorSucceeded {
				errors = append(errors, "result.json: collector_succeeded is false")
			}
			if !r.PProfDiffSucceeded {
				errors = append(errors, "result.json: pprof_diff_succeeded is false")
			}
			if !r.ManifestValid {
				errors = append(errors, "result.json: manifest_valid is false")
			}
			if !r.VerdictValid {
				errors = append(errors, "result.json: verdict_valid is false")
			}
		}
	}

	// Verify rss-series.csv header
	rssCSVPath := filepath.Join(dir, "rss-series.csv")
	expectedRSSHeader := "elapsed_seconds,pid,rss_bytes,vsz_bytes,threads,fd_count"
	if err := verifyCSVHeader(rssCSVPath, expectedRSSHeader); err != nil {
		errors = append(errors, fmt.Sprintf("rss-series.csv: %v", err))
	}

	// Verify goroutine-count-series.csv header
	grpcCSVPath := filepath.Join(dir, "goroutine-count-series.csv")
	expectedGrpcHeader := "elapsed_seconds,goroutines"
	if err := verifyCSVHeader(grpcCSVPath, expectedGrpcHeader); err != nil {
		errors = append(errors, fmt.Sprintf("goroutine-count-series.csv: %v", err))
	}

	// Verify baseline heap profile exists
	heapBase := filepath.Join(dir, "heap-t000.pb.gz")
	if _, err := os.Stat(heapBase); os.IsNotExist(err) {
		errors = append(errors, "baseline heap profile missing: heap-t000.pb.gz")
	}

	// Verify final heap profile exists (duration-aware)
	heapFinal := filepath.Join(dir, fmt.Sprintf("heap-%s.pb.gz", finalSuffix))
	if _, err := os.Stat(heapFinal); os.IsNotExist(err) {
		errors = append(errors, fmt.Sprintf("final heap profile missing: heap-%s.pb.gz", finalSuffix))
	}

	// Verify final allocs profile exists
	allocsFinal := filepath.Join(dir, fmt.Sprintf("allocs-%s.pb.gz", finalSuffix))
	if _, err := os.Stat(allocsFinal); os.IsNotExist(err) {
		errors = append(errors, fmt.Sprintf("final allocs profile missing: allocs-%s.pb.gz", finalSuffix))
	}

	// Verify pprof diff reports exist and are non-empty
	for _, name := range []string{"heap-diff-inuse-space.txt", "heap-diff-inuse-objects.txt", "allocs-final-alloc-space.txt"} {
		fpath := filepath.Join(dir, name)
		info, err := os.Stat(fpath)
		if err != nil {
			if os.IsNotExist(err) {
				errors = append(errors, fmt.Sprintf("pprof diff report missing: %s", name))
			} else {
				errors = append(errors, fmt.Sprintf("error checking pprof diff report %s: %v", name, err))
			}
			continue
		}
		if info.Size() == 0 {
			errors = append(errors, fmt.Sprintf("pprof diff report is empty: %s", name))
		}
	}

	// Verify classification from manifest
	if manifest.Classification == "" {
		errors = append(errors, "classification is empty")
	} else if !allowedClassifications[manifest.Classification] {
		errors = append(errors, fmt.Sprintf("unknown classification: %s", manifest.Classification))
	}

	return errors
}

func verifyCSVHeader(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return fmt.Errorf("empty file")
	}

	header := strings.TrimSpace(scanner.Text())
	if header != expected {
		return fmt.Errorf("header mismatch: got %q, want %q", header, expected)
	}

	return nil
}
