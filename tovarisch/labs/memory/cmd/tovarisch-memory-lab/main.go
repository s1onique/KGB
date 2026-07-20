// cmd/tovarisch-memory-lab/main.go — Memory Laboratory CLI
//
// Go-based Docker laboratory for deterministic memory investigation.
// Uses Docker SDK with Engine API version negotiation.
//
// Reference: kgb://factory/workflow

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling"
)

// Allowed scenarios
var allowedScenarios = map[string]struct{}{
	"canary-growing":    {},
	"canary-bounded":    {},
	"canary-descriptor": {},
}

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: %s <run|verify> [options]", args[0])
	}

	switch args[1] {
	case "run":
		return runCommand(args[1:])
	case "verify":
		return verifyCommand(args[1:])
	default:
		return fmt.Errorf("unknown subcommand: %s (expected 'run' or 'verify')", args[1])
	}
}

func runCommand(args []string) error {
	fs := flag.NewFlagSet("memory-lab run", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s run [options]\n\nOptions:\n", args[0])
		fs.PrintDefaults()
	}

	scenario := fs.String("scenario", "", "Scenario (required): canary-growing, canary-bounded, canary-descriptor")
	duration := fs.Int("duration", 60, "Duration in seconds")
	artifactsDir := fs.String("artifacts-dir", "", "Artifacts directory (required)")
	verbose := fs.Bool("v", false, "Verbose output")
	containerImage := fs.String("container-image", "alpine:latest", "Container image")

	if err := fs.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return fmt.Errorf("parse flags: %w", err)
	}

	if *scenario == "" {
		return fmt.Errorf("--scenario is required")
	}
	if _, ok := allowedScenarios[*scenario]; !ok {
		return fmt.Errorf("invalid scenario %q: allowed: canary-growing, canary-bounded, canary-descriptor", *scenario)
	}
	if *artifactsDir == "" {
		return fmt.Errorf("--artifacts-dir is required")
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if *duration < 10 {
		return fmt.Errorf("duration must be >= 10 seconds")
	}

	// Create Docker client
	dockerClient, err := dockerlab.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("create docker client: %w", err)
	}
	defer func() {
		// Cleanup will be done explicitly
	}()

	// Get Docker info
	dockerInfo, err := dockerClient.ServerVersion(ctx)
	if err != nil {
		return fmt.Errorf("get docker version: %w", err)
	}

	if *verbose {
		fmt.Printf("Docker %s (API %s)\n", dockerInfo.Version, dockerClient.ClientVersion())
	}

	// Pull image and get ID
	imageID, err := dockerClient.ImagePull(ctx, *containerImage)
	if err != nil {
		return fmt.Errorf("pull image: %w", err)
	}
	if *verbose {
		fmt.Printf("Pulled image: %s (ID: %s)\n", *containerImage, imageID[:12])
	}

	// Create run ID
	runID := fmt.Sprintf("lab-%s-%d", *scenario, time.Now().Unix())

	// Create artifacts directory
	artifactsPath := filepath.Join(*artifactsDir, runID)
	if err := os.MkdirAll(artifactsPath, 0755); err != nil {
		return fmt.Errorf("create artifacts dir: %w", err)
	}

	// Create evidence writer
	evidenceWriter := evidence.NewWriter(runID, *scenario, artifactsPath)

	// Write initial manifest
	manifest := &evidence.Manifest{
		SchemaVersion: "1.0.0",
		RunID:         runID,
		Scenario:      *scenario,
		StartedAt:     time.Now(),
		DockerID: &evidence.DockerIdentity{
			EngineVersion: dockerInfo.Version,
			APIVersion:    dockerClient.ClientVersion(),
		},
	}
	if err := evidenceWriter.WriteManifest(manifest); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	// Create cleanup manager
	cleanup := dockerlab.NewCleanupManager(dockerClient, runID)

	// Create lab network
	labNet, err := dockerlab.CreateNetwork(ctx, dockerClient, cleanup, runID, "lab")
	if err != nil {
		return fmt.Errorf("create lab network: %w", err)
	}
	if *verbose {
		fmt.Printf("Created network: %s\n", labNet.ID)
	}

	// Determine command based on scenario
	cmd := getScenarioCommand(*scenario)

	// Create container config using dockerlab.ContainerConfig
	containerName := fmt.Sprintf("tovarisch-subject-%s", runID)
	containerCfg := dockerlab.ContainerConfig{
		Name:   containerName,
		Config: dockerlab.NewContainerConfig(containerName, *containerImage).Config,
	}
	containerCfg.Config.Cmd = cmd
	containerCfg.Config.Labels = map[string]string{
		"kgb.dev/lab":          "tovarisch-memory",
		"kgb.dev/lab.run-id":   runID,
		"kgb.dev/lab.scenario": *scenario,
	}
	containerCfg.MemoryLimit = 128 * 1024 * 1024 // 128MB
	containerCfg.CPUQuota = 50000                // 50% CPU

	// Create container
	containerID, err := dockerClient.ContainerCreate(ctx, containerCfg)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	cleanup.RegisterContainer(containerID)

	// Connect to network
	if err := dockerClient.NetworkConnect(ctx, labNet.ID, containerID); err != nil {
		cleanup.Cleanup(ctx)
		return fmt.Errorf("connect to network: %w", err)
	}

	// Start container
	if err := dockerClient.ContainerStart(ctx, containerID); err != nil {
		cleanup.Cleanup(ctx)
		return fmt.Errorf("start container: %w", err)
	}

	// Get container PID
	containerPID, err := dockerClient.ContainerGetPID(ctx, containerID)
	if err != nil {
		cleanup.Cleanup(ctx)
		return fmt.Errorf("get container PID: %w", err)
	}

	if *verbose {
		fmt.Printf("Container %s started with PID %d\n", containerID, containerPID)
	}

	// Start sampling
	phaseCfg := sampling.SmokePhaseConfig()
	phaseCfg.Stimulus = time.Duration(*duration) * time.Second

	sampler := sampling.NewSampler(
		func() int { return containerPID },
		phaseCfg,
	)
	sampler.Start(ctx)

	// Wait for total duration
	totalDuration := phaseCfg.TotalDuration()
	if *verbose {
		fmt.Printf("Sampling for %v\n", totalDuration)
	}

	select {
	case <-ctx.Done():
		sampler.Stop()
		cleanup.Cleanup(ctx)
		return ctx.Err()
	case <-time.After(totalDuration):
		sampler.Stop()
	}

	// Get samples
	samples := sampler.Samples()

	if *verbose {
		fmt.Printf("Collected %d samples\n", len(samples))
	}

	// Write samples CSV
	if err := evidenceWriter.WriteSamplesCSV(samples); err != nil {
		cleanup.Cleanup(ctx)
		return fmt.Errorf("write samples CSV: %w", err)
	}

	// Get container logs
	logs, _ := dockerClient.ContainerLogs(ctx, containerID, "100")
	evidenceWriter.WriteContainerLogs("container", []byte(logs))

	// Inspect container
	inspectData, _ := dockerClient.ContainerInspect(ctx, containerID)
	evidenceWriter.WriteContainerInspect("container", inspectData)

	// Analyze
	thresholds := analysis.DefaultThresholds()
	verdict := analysis.Analyze(samples, thresholds)

	// Determine expected verdict based on scenario
	expectedVerdict := getExpectedVerdict(*scenario, verdict)

	// Write verdict
	scenarioValid := len(samples) > 0 && verdict.Overall != analysis.ClassificationInconclusive
	canariesValid := scenarioValid && verdict.Overall == expectedVerdict

	verdictOutput := &evidence.Verdict{
		OverallClassification:  verdict.Overall,
		Scenario:               *scenario,
		ScenarioValid:          scenarioValid,
		CanariesValid:          canariesValid,
		MemoryClassification:   verdict.Memory,
		ResourceClassification: verdict.Resource,
		SemanticClassification: verdict.Semantic,
		SignalSummaries:        verdict.Signals,
		Thresholds:             &thresholds,
		Failures:               verdict.Failures,
		Warnings:               verdict.Warnings,
		Unknowns:               verdict.Unknowns,
	}
	if err := evidenceWriter.WriteVerdict(verdictOutput); err != nil {
		cleanup.Cleanup(ctx)
		return fmt.Errorf("write verdict: %w", err)
	}

	// Write checksums
	if err := evidenceWriter.WriteChecksums(); err != nil {
		cleanup.Cleanup(ctx)
		return fmt.Errorf("write checksums: %w", err)
	}

	// Print results
	fmt.Printf("\n=== Analysis Result ===\n")
	fmt.Printf("Scenario: %s\n", *scenario)
	fmt.Printf("Expected Verdict: %s\n", expectedVerdict)
	fmt.Printf("Actual Verdict: %s\n", verdict.Overall)
	fmt.Printf("ScenarioValid: %v\n", scenarioValid)
	fmt.Printf("CanariesValid: %v\n", canariesValid)
	fmt.Printf("Samples: %d\n", len(samples))
	fmt.Printf("Signals: %d\n", len(verdict.Signals))

	if len(verdict.Failures) > 0 {
		fmt.Printf("Failures: %v\n", verdict.Failures)
	}

	// Cleanup - fatal if fails after evidence written
	if err := cleanup.Cleanup(ctx); err != nil {
		fmt.Printf("ERROR: cleanup failed: %v\n", err)
		return fmt.Errorf("cleanup failed: %w", err)
	}

	fmt.Printf("\nArtifacts written to: %s\n", artifactsPath)
	fmt.Printf("Run ID: %s\n", runID)

	return nil
}

func getScenarioCommand(scenario string) []string {
	// Use the compiled canary binary with appropriate mode
	switch scenario {
	case "canary-growing":
		return []string{"/app/canary", "--mode=growing"}
	case "canary-bounded":
		return []string{"/app/canary", "--mode=bounded"}
	case "canary-descriptor":
		return []string{"/app/canary", "--mode=descriptor"}
	default:
		return []string{"/app/canary", "--mode=bounded"}
	}
}

func getExpectedVerdict(scenario string, verdict *analysis.Verdict) analysis.Classification {
	switch scenario {
	case "canary-growing":
		return analysis.ClassificationGrowing
	case "canary-bounded":
		return analysis.ClassificationStable
	case "canary-descriptor":
		return analysis.ClassificationResourceGrowth
	default:
		return verdict.Overall
	}
}

func verifyCommand(args []string) error {
	fs := flag.NewFlagSet("memory-lab verify", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s verify [options]\n\nOptions:\n", args[0])
		fs.PrintDefaults()
	}

	artifactsDir := fs.String("artifacts-dir", "", "Artifacts directory (required)")
	runID := fs.String("run-id", "", "Run ID to verify (required)")

	if err := fs.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return fmt.Errorf("parse flags: %w", err)
	}

	if *artifactsDir == "" {
		return fmt.Errorf("--artifacts-dir is required")
	}
	if *runID == "" {
		return fmt.Errorf("--run-id is required")
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}

	artifactPath := filepath.Join(*artifactsDir, *runID)

	// Check all required artifacts exist
	requiredArtifacts := []string{"manifest.json", "verdict.json", "samples.csv", "checksums.txt"}
	for _, name := range requiredArtifacts {
		path := filepath.Join(artifactPath, name)
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("missing artifact: %s", name)
		}
	}

	// Load and verify manifest
	manifestData, err := os.ReadFile(filepath.Join(artifactPath, "manifest.json"))
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	var manifest evidence.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}

	// Verify manifest consistency
	if manifest.RunID != *runID {
		return fmt.Errorf("run ID mismatch: manifest=%s, expected=%s", manifest.RunID, *runID)
	}

	// Load and verify verdict
	verdictData, err := os.ReadFile(filepath.Join(artifactPath, "verdict.json"))
	if err != nil {
		return fmt.Errorf("read verdict: %w", err)
	}
	var verdict evidence.Verdict
	if err := json.Unmarshal(verdictData, &verdict); err != nil {
		return fmt.Errorf("parse verdict: %w", err)
	}

	fmt.Printf("=== Verification Results ===\n")
	fmt.Printf("Run ID: %s\n", *runID)
	fmt.Printf("Scenario: %s\n", verdict.Scenario)
	fmt.Printf("ScenarioValid: %v\n", verdict.ScenarioValid)
	fmt.Printf("CanariesValid: %v\n", verdict.CanariesValid)
	fmt.Printf("Overall: %s\n", verdict.OverallClassification)
	fmt.Printf("Memory: %s\n", verdict.MemoryClassification)

	if verdict.ScenarioValid && verdict.CanariesValid {
		fmt.Printf("PASS: Evidence verified\n")
	} else {
		fmt.Printf("WARN: Scenario or canaries not valid\n")
	}

	return nil
}
