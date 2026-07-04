// Package main implements the UVB-76 Capture Netns Lab CLI.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/s1onique/KGB/uvb76/cmd/uvb76-capture-netns-lab/internal/lab"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Parse CLI flags
	artifactDir := flag.String("artifact-dir", "", "Optional artifact directory path")
	keepNamespaces := flag.Bool("keep-namespaces", false, "Keep namespaces after lab completion")
	skipCleanup := flag.Bool("skip-cleanup", false, "Skip cleanup on exit")
	timeout := flag.Duration("timeout", 10*time.Minute, "Overall lab timeout")
	phaseTimeout := flag.Duration("phase-timeout", 30*time.Second, "Phase timeout")
	uvb76Bin := flag.String("uvb76-bin", "", "Path to uvb76 binary")
	tovarischBin := flag.String("tovarisch-bin", "", "Path to tovarisch binary")
	verbose := flag.Bool("verbose", false, "Verbose output")
	flag.Parse()

	// Get positional arg for artifact dir
	args := flag.Args()
	if len(args) > 0 && *artifactDir == "" {
		*artifactDir = args[0]
	}

	// Check environment
	if err := lab.CheckLinux(); err != nil {
		log.Printf("Environment check failed: %v", err)
		log.Printf("Note: This lab requires Linux with CAP_NET_ADMIN.")
		log.Printf("On macOS, run in GitHub Actions or a Linux VM.")
		os.Exit(1)
	}

	// Create command runner
	runner := lab.NewRealCommandRunner()

	// Check dependencies
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := lab.CheckDependencies(ctx, runner); err != nil {
		log.Printf("Dependency check failed: %v", err)
		log.Printf("Install: iproute2 jq curl")
		os.Exit(1)
	}

	// Configure orchestrator
	config := lab.OrchestratorConfig{
		ArtifactDir:    *artifactDir,
		UVB76Bin:      *uvb76Bin,
		TovarischBin:  *tovarischBin,
		Timeout:       *timeout,
		PhaseTimeout:  *phaseTimeout,
		KeepNamespaces: *keepNamespaces,
		SkipCleanup:   *skipCleanup,
		Verbose:       *verbose,
	}

	// Create orchestrator
	orchestrator, err := lab.NewOrchestrator(runner, config)
	if err != nil {
		log.Printf("Failed to create orchestrator: %v", err)
		os.Exit(1)
	}

	// Run with timeout
	runCtx, runCancel := context.WithTimeout(context.Background(), *timeout)
	defer runCancel()

	if err := orchestrator.Run(runCtx); err != nil {
		// Run cleanup on error
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		orchestrator.CleanupOnError(cleanupCtx, err)

		log.Printf("Lab failed: %v", err)
		if orchestrator.Artifacts != nil {
			log.Printf("Artifacts available at: %s", orchestrator.Artifacts.Root)
		}
		os.Exit(1)
	}

	log.Printf("Lab completed successfully")
	log.Printf("Artifacts available at: %s", orchestrator.Artifacts.Root)
	fmt.Println()
	fmt.Println("Lab Result Summary:")
	fmt.Println("  Artifact directory:", orchestrator.Artifacts.Root)
	fmt.Println("  Phases executed: baseline, defect, recovery")
	fmt.Println()

	os.Exit(0)
}

