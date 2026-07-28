// Package main implements the UVB-76 pprof memory leak lab.
//
// This lab runs real Tovarisch and UVB-76 processes for cross-component
// memory profiling. It captures heap profiles, RSS samples, and proves
// that UVB-76 successfully scrapes real Tovarisch /status endpoint.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
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

	// Validate flags
	if err := validateFlags(); err != nil {
		log.Fatalf("Flag validation failed: %v", err)
	}

	artifactDir = *flagArtifactDir

	log.Printf("=== UVB-76 Memory Leak pprof Lab ===")
	log.Printf("Use fake Tovarisch: %v", *flagUseFakeTovarisch)
	log.Printf("Duration: %s", *flagDuration)
	log.Printf("Sample interval: %s", *flagSampleInterval)
	log.Printf("Profile interval: %s", *flagProfileInterval)
	log.Printf("Artifact dir: %s", artifactDir)

	// Setup context for cancellation
	labCtx, labCancel = context.WithTimeout(context.Background(), *flagDuration+5*time.Minute)
	defer labCancel()

	// Setup cleanup channels
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
	cfg := labconfig.Generate(*flagUVB76Port, *flagPProfPort, *flagTovarischPort, *flagUseFakeTovarisch)
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
	log.Printf("Classification: %s", result.Classification)
	log.Printf("Real Tovarisch started: %v", result.RealTovarischStarted)
	log.Printf("Real Tovarisch ready: %v", result.RealTovarischReady)
	log.Printf("Real UVB-76 started: %v", result.RealUVB76Started)
	log.Printf("UVB-76 pprof ready: %v", result.UVB76PProfReady)
	log.Printf("Real target observed: %v", result.RealTargetObserved)
	log.Printf("Scrape attempted: %v", result.ScrapeAttempted)
	log.Printf("Scrape completed: %v", result.ScrapeCompleted)
	log.Printf("Process samples present: %v", result.ProcessSamplesPresent)
	log.Printf("Profiles present: %v", result.ProfilesPresent)
	log.Printf("UVB-76 removed: %v", result.UVB76Removed)
	log.Printf("Tovarisch removed: %v", result.TovarischRemoved)
	log.Printf("Ports released: %v", result.PortsReleased)
	log.Printf("Artifact dir: %s", artifactDir)

	// Print any errors that occurred
	if len(result.Errors) > 0 {
		log.Printf("")
		log.Printf("=== Errors ===")
		for _, err := range result.Errors {
			log.Printf("  - %s", err)
		}
	}

	if !result.OK {
		os.Exit(1)
	}
}

// validateFlags ensures required flags are set correctly.
func validateFlags() error {
	if *flagUseFakeTovarisch {
		return nil // Fake mode is hermetic
	}

	// Real mode requires explicit binary paths
	if *flagTovarischBin == "" {
		return fmt.Errorf("--tovarisch-bin is required in real mode")
	}
	if *flagUVB76Bin == "" {
		return fmt.Errorf("--uvb76-bin is required in real mode")
	}

	// Validate binaries exist and are executable
	if err := validateBinary(*flagTovarischBin, "Tovarisch"); err != nil {
		return err
	}
	if err := validateBinary(*flagUVB76Bin, "UVB-76"); err != nil {
		return err
	}

	// Reject identical paths for different programs
	if *flagTovarischBin == *flagUVB76Bin {
		return fmt.Errorf("--tovarisch-bin and --uvb76-bin must be different binaries")
	}

	// Check ports are not in use
	if err := checkPortFree(*flagTovarischPort); err != nil {
		return fmt.Errorf("Tovarisch port: %w", err)
	}
	if err := checkPortFree(*flagUVB76Port); err != nil {
		return fmt.Errorf("UVB-76 port: %w", err)
	}
	if err := checkPortFree(*flagPProfPort); err != nil {
		return fmt.Errorf("pprof port: %w", err)
	}

	return nil
}

// validateBinary checks a binary exists and is executable.
func validateBinary(path, name string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s binary does not exist: %s", name, path)
		}
		return fmt.Errorf("%s binary not accessible: %w", name, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s path is a directory, not a binary: %s", name, path)
	}
	// Check execute permission (not exhaustive but catches common issues)
	if info.Mode()&0111 == 0 {
		return fmt.Errorf("%s binary is not executable: %s", name, path)
	}
	return nil
}

// checkPortFree checks if a port is available.
func checkPortFree(port string) error {
	// Simple check - just try to listen
	addr := fmt.Sprintf("localhost:%s", port)
	ln, err := listenerForPort(addr)
	if err != nil {
		return fmt.Errorf("port %s is already in use: %w", port, err)
	}
	ln.Close()
	return nil
}

func listenerForPort(addr string) (interface{ Close() error }, error) {
	// Use net.Listen and return the closer
	// We import net in process.go, but for simplicity here we'll use a simple approach
	return net.Listen("tcp", addr)
}
