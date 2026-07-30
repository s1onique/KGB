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

	uvb76LogFile = filepath.Join(artifactDir, "uvb76.log")
	tovarischLogFile = filepath.Join(artifactDir, "tovarisch.log")

	// Run with proper defer for credential cleanup
	if err := runMain(); err != nil {
		log.Printf("memory lab failed: %v", err)
		os.Exit(1)
	}
}

// runMain is the main entry point that returns errors for proper credential cleanup.
// P0-1: Uses return-based error handling so defer runs before os.Exit.
func runMain() error {
	// === PHASE 0: Generate and validate configuration authority ===
	// P0-1: Create authority bundle before any process startup
	// P0-3: Validate before any side effects
	// P0-4: Strict reread and equality proof

	configFile := filepath.Join(artifactDir, "uvb76-lab-config.json")

	// Determine execution mode
	mode := ExecutionModeReal
	if *flagUseFakeTovarisch {
		mode = ExecutionModeFake
	}

	// Generate configuration
	// P0-1: Returns error if credential generation fails - no fallback
	labResult, genErr := labconfig.Generate(*flagUVB76Port, *flagPProfPort, *flagTovarischPort, *flagUseFakeTovarisch)
	if genErr != nil {
		return fmt.Errorf("generate configuration: %w", genErr)
	}

	// P0-1: Install credential cleanup via defer - runs on ALL return paths
	ephemeralPassword := labResult.EphemeralPassword
	defer func() {
		for i := range ephemeralPassword {
			ephemeralPassword[i] = 0
		}
	}()

	// Convert to GeneratedConfig
	generatedCfg := &GeneratedConfig{
		Listen:      ConfigListenConfig(labResult.Config.Listen),
		Auth:        ConfigAuthConfig(labResult.Config.Auth),
		Scrape:      ConfigScrapeConfig(labResult.Config.Scrape),
		Latency:     ConfigLatencyConfig(labResult.Config.Latency),
		Diagnostics: ConfigDiagnosticsConfig(labResult.Config.Diagnostics),
		Targets:     make([]ConfigTargetConfig, len(labResult.Config.Targets)),
	}
	for i, t := range labResult.Config.Targets {
		generatedCfg.Targets[i] = ConfigTargetConfig(t)
	}

	// P0-3: Validate generated config before publication
	if err := ValidateGeneratedConfig(generatedCfg, mode); err != nil {
		return fmt.Errorf("validate generated config: %w", err)
	}

	// P0-2: Extract target binding from generated config
	targetBinding, err := ExtractTargetBinding(generatedCfg, mode)
	if err != nil {
		return fmt.Errorf("extract target binding: %w", err)
	}

	// P0-5: Derive canonical URLs from config
	uvb76APIBaseURL, err := DeriveUVB76APIBaseURL(generatedCfg)
	if err != nil {
		return fmt.Errorf("derive UVB-76 API base URL: %w", err)
	}

	// P0-1: Authority bundle created before process startup
	// P0-3: Authentication deferred until after UVB-76 starts and API is ready
	// P0-7: Auth resolver NOT resolved here - will be resolved inside runLab after UVB-76 ready
	authority := &GeneratedLabAuthority{
		Config:          generatedCfg,
		ConfigPath:      configFile,
		Mode:            mode,
		Target:          targetBinding,
		UVB76APIBaseURL: uvb76APIBaseURL,
	}

	// P0-3: Atomic config publication
	if err := atomicPublishConfig(generatedCfg, configFile); err != nil {
		return fmt.Errorf("publish config: %w", err)
	}
	log.Printf("Generated and published lab config: %s", configFile)

	// P0-3: Strict reread
	rereadCfg, err := StrictlyReadConfig(configFile)
	if err != nil {
		return fmt.Errorf("reread config: %w", err)
	}

	// P0-3: Prove equality
	if err := ProveConfigEquality(generatedCfg, rereadCfg); err != nil {
		return fmt.Errorf("prove config equality: %w", err)
	}

	// P0-3: Re-extract binding from reread config
	rereadBinding, err := ExtractTargetBinding(rereadCfg, mode)
	if err != nil {
		return fmt.Errorf("reread binding: %w", err)
	}

	// P0-3: Prove binding equality
	if err := ProveTargetBindingEquality(targetBinding, rereadBinding); err != nil {
		return fmt.Errorf("prove binding equality: %w", err)
	}

	// === PHASE 1: Run the lab with authority ===
	// P0-7-fix: Pass ephemeral password to runLab for post-startup authentication
	result := runLab(authority, ephemeralPassword)

	// Write result
	resultBytes, _ := json.MarshalIndent(result, "", "  ")
	resultFile := filepath.Join(artifactDir, "result.json")
	if err := os.WriteFile(resultFile, resultBytes, 0644); err != nil {
		return fmt.Errorf("write result: %w", err)
	}

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
		return fmt.Errorf("lab result not OK: classification=%s", result.Classification)
	}

	return nil
}

// atomicPublishConfig atomically publishes the config file.
func atomicPublishConfig(cfg *GeneratedConfig, path string) error {
	// Marshal to JSON
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// Write to temp file
	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("%w: write temp: %v", ErrGeneratedConfigWrite, err)
	}

	// Sync to disk
	f, err := os.Open(tmpFile)
	if err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("%w: open temp: %v", ErrGeneratedConfigWrite, err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpFile)
		return fmt.Errorf("%w: sync temp: %v", ErrGeneratedConfigWrite, err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("%w: close temp: %v", ErrGeneratedConfigWrite, err)
	}

	// Atomic rename
	if err := os.Rename(tmpFile, path); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("%w: rename: %v", ErrGeneratedConfigWrite, err)
	}

	return nil
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
	// Check execute permission
	if info.Mode()&0111 == 0 {
		return fmt.Errorf("%s binary is not executable: %s", name, path)
	}
	return nil
}

// checkPortFree checks if a port is available.
func checkPortFree(port string) error {
	addr := fmt.Sprintf("localhost:%s", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("port %s is already in use: %w", port, err)
	}
	ln.Close()
	return nil
}
