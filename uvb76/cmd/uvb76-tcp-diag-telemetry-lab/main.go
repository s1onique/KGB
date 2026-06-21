// Package main implements the UVB-76 TCP Diagnostic Telemetry Lab.
//
// This lab proves that TCP telemetry is collected in diagnostic packets by:
// 1. Starting a hermetic diagnostic peer server
// 2. Fetching /status.json?include=network_diag
// 3. Verifying the response contains TCP telemetry in underlay_tcp
// 4. Persisting the captured diagnostic packet artifact
// 5. Running structural verification on the artifact
package main

import (
	"flag"
	"log"
	"os"

	"github.com/s1onique/KGB/uvb76/cmd/uvb76-tcp-diag-telemetry-lab/internal/runner"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Parse CLI flags
	artifactDir := flag.String("artifact-dir", "", "Optional artifact directory path")
	verifyOnly := flag.Bool("verify", false, "Verify existing artifacts only (requires artifact-dir)")
	flag.Parse()

	// Get positional arg for artifact dir (alternative to --artifact-dir)
	args := flag.Args()
	if len(args) > 0 && *artifactDir == "" {
		*artifactDir = args[0]
	}

	var result *runner.Result
	var err error

	if *verifyOnly && *artifactDir != "" {
		// Verify mode: just run the verifier on existing artifacts
		result, err = runner.Verify(*artifactDir)
	} else if *artifactDir != "" {
		// Run with specified artifact directory
		result, err = runner.RunWithDir(*artifactDir)
	} else {
		// Default: create new temp directory
		result, err = runner.Run()
	}

	if err != nil {
		log.Printf("Lab failed: %v", err)
		if result != nil && result.ArtifactDir != "" {
			log.Printf("Artifacts available at: %s", result.ArtifactDir)
		}
		os.Exit(1)
	}

	if !result.OK {
		log.Printf("Lab result: ok=false")
		if result.FailureReason != "" {
			log.Printf("Failure reason: %s", result.FailureReason)
		}
		if result.ArtifactDir != "" {
			log.Printf("Artifacts available at: %s", result.ArtifactDir)
		}
		os.Exit(1)
	}

	log.Printf("Lab completed successfully")
	log.Printf("Artifacts available at: %s", result.ArtifactDir)
	os.Exit(0)
}
