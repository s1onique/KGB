// Package main implements the UVB-76 pprof memory leak lab.
//
// This lab runs a real UVB-76 process with pprof diagnostics enabled,
// a fake tovarisch-compatible status endpoint, and the memory lab collector
// to capture heap profiles, RSS samples, and pprof diff reports.
//
// Artifact directory structure:
//   - startup_evidence.json  - launch metadata (timestamp, PID, ports, durations)
//   - exit.json             - crash evidence if target exits unexpectedly
//   - uvb76.log             - UVB-76 process output
//   - tovarisch.log         - fake-tovarisch output (if used)
//   - uvb76.pid             - PID file
//   - uvb76-lab-config.json - generated lab configuration
//   - heap-t000.pb.gz       - baseline heap profile
//   - heap-t600.pb.gz       - final heap profile (for 10m run)
//   - allocs-t000.pb.gz     - baseline allocs profile
//   - allocs-t600.pb.gz     - final allocs profile
//   - goroutine-t000.txt    - baseline goroutine dump
//   - goroutine-t600.txt    - final goroutine dump
//   - rss-series.csv        - RSS/VSZ/threads/fd over time
//   - goroutine-count-series.csv - goroutine counts over time
//   - manifest.json         - lab run metadata
//   - verdict.json          - classification verdict
//   - heap-diff-inuse-space.txt  - pprof diff (inuse_space)
//   - heap-diff-inuse-objects.txt - pprof diff (inuse_objects)
//   - allocs-final-alloc-space.txt - final allocs top
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/s1onique/KGB/uvb76/cmd/uvb76-memleak-pprof-lab/internal/labconfig"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	flag.Parse()

	if *flagArtifactDir == "" {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "Error: --artifact-dir is required")
		os.Exit(1)
	}

	artifactDir = *flagArtifactDir

	log.Printf("=== UVB-76 Memory Leak pprof Lab ===")
	log.Printf("Duration: %s", *flagDuration)
	log.Printf("Sample interval: %s", *flagSampleInterval)
	log.Printf("Profile interval: %s", *flagProfileInterval)
	log.Printf("Artifact dir: %s", artifactDir)

	// Setup context for cancellation
	labCtx, labCancel = context.WithTimeout(context.Background(), *flagDuration+5*time.Minute)
	defer labCancel()

	// Setup cleanup
	uvb76Done = make(chan struct{})
	tovarischDone = make(chan struct{})

	// Create artifact directory
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		log.Fatalf("Failed to create artifact dir: %v", err)
	}

	configFile = filepath.Join(artifactDir, "uvb76-lab-config.json")
	uvb76LogFile = filepath.Join(artifactDir, "uvb76.log")
	tovarischLogFile = filepath.Join(artifactDir, "tovarisch.log")

	// Generate lab config
	cfg := labconfig.Generate(uvb76Port, pprofPort, tovarischPort)
	cfgBytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal config: %v", err)
	}
	if err := os.WriteFile(configFile, cfgBytes, 0644); err != nil {
		log.Fatalf("Failed to write config: %v", err)
	}
	log.Printf("Generated lab config: %s", configFile)

	// Run the lab with full lifecycle management
	result := runLab()

	// Write result
	resultBytes, _ := json.MarshalIndent(result, "", "  ")
	resultFile := filepath.Join(artifactDir, "result.json")
	os.WriteFile(resultFile, resultBytes, 0644)

	log.Printf("")
	log.Printf("=== Lab Result ===")
	log.Printf("OK: %v", result.OK)
	log.Printf("UVB-76 started: %v", result.UVB76Started)
	log.Printf("pprof reachable: %v", result.PProfReachable)
	log.Printf("Tovarisch reachable: %v", result.TovarischReachable)
	log.Printf("Collector succeeded: %v", result.CollectorSucceeded)
	log.Printf("pprof diff succeeded: %v", result.PProfDiffSucceeded)
	log.Printf("Manifest valid: %v", result.ManifestValid)
	log.Printf("Verdict valid: %v", result.VerdictValid)
	log.Printf("Artifact dir: %s", artifactDir)

	// Print any errors that occurred
	if len(result.Errors) > 0 {
		log.Printf("")
		log.Printf("=== Errors ===")
		for _, err := range result.Errors {
			log.Printf("  - %s", err)
		}
	}

	// Provide diagnostic hints based on failures
	if !result.OK {
		log.Printf("")
		log.Printf("=== Diagnostic Hints ===")
		if !result.UVB76Started {
			log.Printf("  - Check startup_evidence.json for launch details")
			log.Printf("  - Check uvb76.log for stderr output")
			log.Printf("  - Verify UVB-76 binary exists and is executable")
		}
		if !result.PProfReachable {
			log.Printf("  - Check if pprof port (%s) is already in use", pprofPort)
			log.Printf("  - Check uvb76.log for 'address already in use' errors")
			log.Printf("  - Verify pprof is enabled in config")
		}
		if !result.TovarischReachable {
			log.Printf("  - Check if tovarisch port (%s) is accessible", tovarischPort)
			log.Printf("  - Verify tovarisch is running and /status endpoint responds")
		}
		if !result.CollectorSucceeded {
			log.Printf("  - Check if UVB-76 exited during collection")
			log.Printf("  - Check exit.json for crash evidence")
			log.Printf("  - Verify collector binary exists")
		}
		if !result.PProfDiffSucceeded {
			log.Printf("  - Check if heap profiles were captured")
			log.Printf("  - Verify 'go tool pprof' is available")
		}
		if !result.ManifestValid {
			log.Printf("  - Check manifest.json schema version")
		}
		if !result.VerdictValid {
			log.Printf("  - Check verdict.json was generated")
		}
	}

	if !result.OK {
		os.Exit(1)
	}
}
