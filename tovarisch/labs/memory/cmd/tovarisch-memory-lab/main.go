// cmd/tovarisch-memory-lab/main.go — Memory Laboratory CLI
//
// Go-based Docker laboratory for deterministic memory investigation.
// Uses Docker SDK with Engine API version negotiation.
//
// Reference: kgb://factory/workflow

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/procfs"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling"
)

// Canonical artifact inventory - exact set required for verification
var canonicalInventory = []string{
	"manifest.json",
	"verdict.json",
	"samples.csv",
	"events.jsonl",
	"container-inspect.json",
	"container-logs.txt",
	"initial-canary-state.json",
	"final-canary-state.json",
	"workload-result.json",
	"checksums.txt",
}

// Required CSV columns
var requiredCSVColumns = []string{
	"sequence",
	"timestamp",
	"process_pid",
	"process_start_time",
	"phase",
	"docker_memory_usage_bytes",
	"docker_memory_limit_bytes",
	"has_docker_memory",
	"fd_count",
	"has_fd_count",
	"cgroup_current_bytes",
	"cgroup_memory_stat_anon",
	"has_cgroup",
	"has_cgroup_anon",
}

// Allowed scenarios
var allowedScenarios = map[string]struct{}{
	"canary-growing":    {},
	"canary-bounded":    {},
	"canary-descriptor": {},
}

// CanaryState represents the canary's internal state from /state endpoint.
type CanaryState struct {
	Mode           string `json:"mode"`
	RetainedBlocks int    `json:"retained_blocks"`
	RetainedBytes  int64  `json:"retained_bytes"`
	OperationCount int    `json:"operation_count"`
	FDCount        int    `json:"fd_count"`
	BufferCapacity int64  `json:"buffer_capacity,omitempty"`
	Ready          bool   `json:"ready"`
}

// WorkloadResult represents the result of a stimulus workload.
type WorkloadResult struct {
	Requested int `json:"requested"`
	Attempted int `json:"attempted"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Returned  int `json:"returned"`
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
	containerImage := fs.String("container-image", "kgb-tovarisch-canary:latest", "Container image")
	canaryPort := fs.Int("canary-port", 8080, "Canary HTTP port")

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

	// Write initial manifest (will be finalized later)
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

	// Determine command based on scenario - use mode argument only
	cmd := getScenarioCommand(*scenario)

	// Create container config
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

	// Discover canary address using the lab network name
	containerIP, err := dockerClient.ContainerIP(ctx, containerID, labNet.Name)
	if err != nil {
		cleanup.Cleanup(ctx)
		return fmt.Errorf("get container IP: %w", err)
	}
	canaryURL := fmt.Sprintf("http://%s:%d", containerIP, *canaryPort)

	if *verbose {
		fmt.Printf("Canary URL: %s\n", canaryURL)
	}

	// HTTP client with Proxy=nil for deterministic networking
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
		},
	}

	// Wait for canary health
	if err := waitForCanaryHealth(ctx, httpClient, canaryURL, 30*time.Second, *verbose); err != nil {
		cleanup.Cleanup(ctx)
		return fmt.Errorf("canary health check failed: %w", err)
	}

	// Read initial canary state
	initialState, err := fetchCanaryState(ctx, httpClient, canaryURL)
	if err != nil {
		cleanup.Cleanup(ctx)
		return fmt.Errorf("fetch initial canary state: %w", err)
	}

	// Verify mode matches scenario
	expectedMode := scenarioToMode(*scenario)
	if initialState.Mode != expectedMode {
		cleanup.Cleanup(ctx)
		return fmt.Errorf("canary mode mismatch: expected %s, got %s", expectedMode, initialState.Mode)
	}

	if *verbose {
		fmt.Printf("Initial canary state: mode=%s, operations=%d, retained_blocks=%d\n",
			initialState.Mode, initialState.OperationCount, initialState.RetainedBlocks)
	}

	// Write initial state
	if err := evidenceWriter.WriteCanaryState("initial", initialState); err != nil {
		cleanup.Cleanup(ctx)
		return fmt.Errorf("write initial canary state: %w", err)
	}

	// Configure phase machine for this run
	phaseCfg := sampling.SmokePhaseConfig()
	phaseCfg.Stimulus = time.Duration(*duration) * time.Second

	// Create sampler
	sampler := sampling.NewSamplerWithDocker(
		containerID,
		func() int { return containerPID },
		dockerClient,
		phaseCfg,
	)

	// Resolve cgroup v2 path for container memory metrics
	// This enables reading memory.current, memory.stat anon, and pids.current
	cgroupPath, cgroupErr := procfs.ResolveCgroupV2Path(containerPID)
	controllerPIDInt := os.Getpid()

	// Classify with namespace comparison and collect proof
	capability, proof := classifyCgroupFailureWithNamespace(cgroupErr, containerPID, controllerPIDInt)

	if cgroupErr != nil {
		// Record as structured event with proof
		sampler.RecordCgroupCapability(ctx, containerPID, capability, "", cgroupErr, controllerPIDInt, proof)
		if *verbose {
			fmt.Printf("CGROUP RESOLUTION FAILED: pid=%d capability=%s error=%v\n", containerPID, capability, cgroupErr)
		}
		// Continue without cgroup - Docker stats will still work as fallback
	} else {
		if *verbose {
			fmt.Printf("CGROUP RESOLVED: pid=%d path=%s\n", containerPID, cgroupPath)
		}
		sampler.SetCgroupPath(cgroupPath)
		sampler.RecordCgroupCapability(ctx, containerPID, sampling.CgroupCapabilityAvailable, cgroupPath, nil, controllerPIDInt, nil)
	}

	// Start sampler
	sampler.Start(ctx)

	// CORRECTED ORCHESTRATION ORDER:
	// 1. Wait for stimulus
	if *verbose {
		fmt.Printf("Waiting for stimulus phase...\n")
	}
	select {
	case <-ctx.Done():
		sampler.Stop()
		cleanup.Cleanup(ctx)
		return ctx.Err()
	case <-sampler.StimulusReady():
	}

	// 2. Perform workload
	if *verbose {
		fmt.Printf("Stimulus phase started, triggering workload\n")
	}
	workloadResult, err := operateCanary(ctx, httpClient, canaryURL, getScenarioOperationCount(*scenario))
	if err != nil {
		sampler.Stop()
		cleanup.Cleanup(ctx)
		return fmt.Errorf("operate canary: %w", err)
	}
	if *verbose {
		fmt.Printf("Workload completed: requested=%d attempted=%d completed=%d\n",
			workloadResult.Requested, workloadResult.Attempted, workloadResult.Completed)
	}

	// 3. Wait for settling
	if err := sampler.WaitForPhase(ctx, sampling.PhaseSettling); err != nil {
		sampler.Stop()
		cleanup.Cleanup(ctx)
		return fmt.Errorf("wait settling: %w", err)
	}

	// 4. Wait for final
	if err := sampler.WaitForPhase(ctx, sampling.PhaseFinal); err != nil {
		sampler.Stop()
		cleanup.Cleanup(ctx)
		return fmt.Errorf("wait final: %w", err)
	}

	// 5. Wait for complete
	if err := sampler.WaitForComplete(ctx); err != nil {
		sampler.Stop()
		cleanup.Cleanup(ctx)
		return fmt.Errorf("wait complete: %w", err)
	}

	// 6. Fetch final canary state
	finalState, err := fetchCanaryState(ctx, httpClient, canaryURL)
	if err != nil {
		sampler.Stop()
		cleanup.Cleanup(ctx)
		return fmt.Errorf("fetch final canary state: %w", err)
	}

	if *verbose {
		fmt.Printf("Final canary state: mode=%s, operations=%d, retained_blocks=%d\n",
			finalState.Mode, finalState.OperationCount, finalState.RetainedBlocks)
	}

	// 7. Stop sampler
	sampler.Stop()

	// 8. Get samples and events
	samples := sampler.Samples()
	events := sampler.Events()

	if *verbose {
		fmt.Printf("Collected %d samples\n", len(samples))
	}

	// Validate phase contract
	phaseValid := validatePhaseContract(samples, phaseCfg)
	if !phaseValid {
		fmt.Printf("WARNING: Phase contract validation failed\n")
	}

	// Validate workload contract
	workloadValid := workloadResult.Completed == workloadResult.Requested

	// Validate process identity stability
	identityStable := validateProcessIdentity(samples)

	// Write evidence artifacts
	if err := evidenceWriter.WriteCanaryState("final", finalState); err != nil {
		cleanup.Cleanup(ctx)
		return fmt.Errorf("write final canary state: %w", err)
	}

	if err := evidenceWriter.WriteWorkloadResult(workloadResult); err != nil {
		cleanup.Cleanup(ctx)
		return fmt.Errorf("write workload result: %w", err)
	}

	if err := evidenceWriter.WriteSamplesCSV(samples); err != nil {
		cleanup.Cleanup(ctx)
		return fmt.Errorf("write samples CSV: %w", err)
	}

	if err := evidenceWriter.WriteEventsJSONL(events); err != nil {
		cleanup.Cleanup(ctx)
		return fmt.Errorf("write events JSONL: %w", err)
	}

	// Get container logs
	logs, _ := dockerClient.ContainerLogs(ctx, containerID, "100")
	evidenceWriter.WriteContainerLogs("container", []byte(logs))

	// Inspect container
	inspectData, _ := dockerClient.ContainerInspect(ctx, containerID)
	evidenceWriter.WriteContainerInspect("container", inspectData)

	// Analyze with state invariant validation
	thresholds := analysis.DefaultThresholds()
	invariantResult := validateStateInvariant(*scenario, initialState, finalState, workloadResult)
	verdict := analysis.AnalyzeWithInvariant(samples, thresholds, invariantResult)

	// Determine expected verdict based on scenario
	expectedVerdict := getExpectedVerdict(*scenario)

	// CORRECTED VALIDITY COMPOSITION
	scenarioValid := phaseValid &&
		workloadValid &&
		identityStable &&
		len(samples) > 0 &&
		verdict.Overall != analysis.ClassificationInvalid &&
		verdict.Overall != analysis.ClassificationInconclusive

	canariesValid := scenarioValid &&
		verdict.Overall == expectedVerdict &&
		invariantResult.Valid

	// Write verdict (without duplicating invariant failures)
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
		Failures:               verdict.Failures, // Don't duplicate
		Warnings:               verdict.Warnings,
		Unknowns:               verdict.Unknowns,
	}
	if err := evidenceWriter.WriteVerdict(verdictOutput); err != nil {
		cleanup.Cleanup(ctx)
		return fmt.Errorf("write verdict: %w", err)
	}

	// Collect provenance (fail-closed: error means evidence is incomplete)
	subject, host, controllerPID, err := collectProvenance()
	if err != nil {
		cleanup.Cleanup(ctx)
		return fmt.Errorf("collect provenance: %w", err)
	}

	// Finalize manifest with all metadata (must be done BEFORE checksums)
	finalizedManifest := &evidence.Manifest{
		SchemaVersion:   "1.0.0",
		RunID:           runID,
		Scenario:        *scenario,
		StartedAt:       manifest.StartedAt,
		FinishedAt:      time.Now(),
		SubjectIdentity: subject,
		ControllerID:    controllerPID,
		HostID:          host,
		DockerID: &evidence.DockerIdentity{
			EngineVersion: dockerInfo.Version,
			APIVersion:    dockerClient.ClientVersion(),
		},
		Configuration: &evidence.LabConfiguration{
			PhaseConfig: phaseCfg,
			Thresholds:  thresholds,
		},
		ArtifactInventory: []string{
			"manifest.json",
			"verdict.json",
			"samples.csv",
			"events.jsonl",
			"container-inspect.json",
			"container-logs.txt",
			"initial-canary-state.json",
			"final-canary-state.json",
			"workload-result.json",
			"checksums.txt",
		},
	}
	if err := evidenceWriter.WriteManifest(finalizedManifest); err != nil {
		cleanup.Cleanup(ctx)
		return fmt.Errorf("write finalized manifest: %w", err)
	}

	// Write checksums for exact inventory (must be done after manifest is finalized)
	if err := evidenceWriter.WriteChecksumsForInventory(finalizedManifest.ArtifactInventory); err != nil {
		cleanup.Cleanup(ctx)
		return fmt.Errorf("write checksums: %w", err)
	}

	// Determine exit status based on validity
	runFailed := !scenarioValid || !canariesValid || !invariantResult.Valid || verdict.Overall != expectedVerdict

	// Print results
	fmt.Printf("\n=== Analysis Result ===\n")
	fmt.Printf("Scenario: %s\n", *scenario)
	fmt.Printf("Expected Verdict: %s\n", expectedVerdict)
	fmt.Printf("Actual Verdict: %s\n", verdict.Overall)
	fmt.Printf("ScenarioValid: %v\n", scenarioValid)
	fmt.Printf("CanariesValid: %v\n", canariesValid)
	fmt.Printf("InvariantValid: %v\n", invariantResult.Valid)
	fmt.Printf("PhaseValid: %v\n", phaseValid)
	fmt.Printf("WorkloadValid: %v\n", workloadValid)
	fmt.Printf("IdentityStable: %v\n", identityStable)
	fmt.Printf("Samples: %d\n", len(samples))
	fmt.Printf("Signals: %d\n", len(verdict.Signals))

	if len(verdict.Failures) > 0 {
		fmt.Printf("Failures: %v\n", verdict.Failures)
	}
	if len(invariantResult.Failures) > 0 {
		fmt.Printf("Invariant Failures: %v\n", invariantResult.Failures)
	}

	// Cleanup
	if err := cleanup.Cleanup(ctx); err != nil {
		fmt.Printf("ERROR: cleanup failed: %v\n", err)
		return fmt.Errorf("cleanup failed: %w", err)
	}

	fmt.Printf("\nArtifacts written to: %s\n", artifactsPath)
	fmt.Printf("Run ID: %s\n", runID)

	// Fail closed: return error if canary did not pass
	if runFailed {
		return fmt.Errorf("canary calibration failed: scenario_valid=%v canaries_valid=%v invariant_valid=%v verdict=%s expected=%s",
			scenarioValid, canariesValid, invariantResult.Valid, verdict.Overall, expectedVerdict)
	}

	return nil
}

// validatePhaseContract checks that we have samples from required phases
func validatePhaseContract(samples []sampling.Sample, cfg sampling.PhaseConfig) bool {
	hasBaseline := false
	hasFinal := false
	finalCount := 0

	for _, s := range samples {
		if s.Phase == sampling.PhaseBaseline {
			hasBaseline = true
		}
		if s.Phase == sampling.PhaseFinal {
			hasFinal = true
			finalCount++
		}
	}

	// Require minimum samples in final phase
	return hasBaseline && hasFinal && finalCount >= 3
}

// validateProcessIdentity checks that PID and start time are stable
func validateProcessIdentity(samples []sampling.Sample) bool {
	if len(samples) < 2 {
		return true
	}
	first := samples[0]
	for _, s := range samples[1:] {
		if s.PID != first.PID || s.ProcessStartTime != first.ProcessStartTime {
			return false
		}
	}
	return true
}

func getScenarioCommand(scenario string) []string {
	switch scenario {
	case "canary-growing":
		return []string{"--mode=growing"}
	case "canary-bounded":
		return []string{"--mode=bounded"}
	case "canary-descriptor":
		return []string{"--mode=descriptor"}
	default:
		return []string{"--mode=bounded"}
	}
}

func getExpectedVerdict(scenario string) analysis.Classification {
	switch scenario {
	case "canary-growing":
		return analysis.ClassificationGrowing
	case "canary-bounded":
		return analysis.ClassificationStable
	case "canary-descriptor":
		return analysis.ClassificationResourceGrowth
	default:
		return analysis.ClassificationStable
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

	// 1. Parse and validate manifest FIRST (source of truth)
	manifestData, err := os.ReadFile(filepath.Join(artifactPath, "manifest.json"))
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	var manifest evidence.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}

	// Validate manifest run ID
	if manifest.RunID != *runID {
		return fmt.Errorf("run ID mismatch: manifest=%s, expected=%s", manifest.RunID, *runID)
	}

	// Validate manifest is finalized
	if manifest.FinishedAt.IsZero() {
		return fmt.Errorf("manifest not finalized: missing finished_at")
	}

	// Validate inventory entries (clean paths only)
	seenPaths := make(map[string]bool)
	for _, path := range manifest.ArtifactInventory {
		if path == "" {
			return fmt.Errorf("empty path in inventory")
		}
		if filepath.IsAbs(path) {
			return fmt.Errorf("absolute path in inventory: %s", path)
		}
		if strings.Contains(path, "..") {
			return fmt.Errorf("path traversal in inventory: %s", path)
		}
		if strings.HasPrefix(path, ".") {
			return fmt.Errorf("hidden path in inventory: %s", path)
		}
		if seenPaths[path] {
			return fmt.Errorf("duplicate path in inventory: %s", path)
		}
		seenPaths[path] = true
	}

	// 2. Enumerate actual files and verify geometry
	entries, err := os.ReadDir(artifactPath)
	if err != nil {
		return fmt.Errorf("read artifact directory: %w", err)
	}

	actualFiles := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			return fmt.Errorf("unexpected directory in artifacts: %s", entry.Name())
		}
		actualFiles[entry.Name()] = true
	}

	// Require exactly manifest inventory + checksums.txt
	expectedFiles := make(map[string]bool)
	for _, p := range manifest.ArtifactInventory {
		expectedFiles[p] = true
	}
	// checksums.txt is always required
	if !expectedFiles["checksums.txt"] {
		return fmt.Errorf("checksums.txt not in inventory")
	}

	// Verify set equality: manifest = actual files
	for path := range expectedFiles {
		if !actualFiles[path] {
			return fmt.Errorf("missing file from inventory: %s", path)
		}
	}
	for path := range actualFiles {
		if !expectedFiles[path] {
			return fmt.Errorf("unexpected file not in inventory: %s", path)
		}
	}

	// 3. Generate checksums from inventory (not directory scan)
	evidenceWriter := evidence.NewWriter(*runID, "", artifactPath)
	checksums, err := evidenceWriter.GenerateChecksumsForInventory(manifest.ArtifactInventory)
	if err != nil {
		return fmt.Errorf("generate checksums: %w", err)
	}

	// 4. Load and verify stored checksums match
	checksumPath := filepath.Join(artifactPath, "checksums.txt")
	existingData, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("read checksums: %w", err)
	}

	existingChecksums, err := evidence.ParseChecksumsFile(string(existingData))
	if err != nil {
		return fmt.Errorf("parse checksums: %w", err)
	}

	for _, c := range checksums {
		expectedHash, exists := existingChecksums[c.Path]
		if !exists {
			return fmt.Errorf("missing checksum for: %s", c.Path)
		}
		if expectedHash != c.SHA256 {
			return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", c.Path, expectedHash, c.SHA256)
		}
	}

	// Verify no extra checksum entries
	for path := range existingChecksums {
		found := false
		for _, c := range checksums {
			if c.Path == path {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unexpected checksum entry: %s", path)
		}
	}

	// 5. Reconstruct evidence claims from artifacts
	// Load and verify verdict
	verdictData, err := os.ReadFile(filepath.Join(artifactPath, "verdict.json"))
	if err != nil {
		return fmt.Errorf("read verdict: %w", err)
	}
	var verdict evidence.Verdict
	if err := json.Unmarshal(verdictData, &verdict); err != nil {
		return fmt.Errorf("parse verdict: %w", err)
	}

	// Load workload result
	workloadData, err := os.ReadFile(filepath.Join(artifactPath, "workload-result.json"))
	if err != nil {
		return fmt.Errorf("read workload result: %w", err)
	}
	var workload WorkloadResult
	if err := json.Unmarshal(workloadData, &workload); err != nil {
		return fmt.Errorf("parse workload result: %w", err)
	}

	// Load initial canary state
	initialStateData, err := os.ReadFile(filepath.Join(artifactPath, "initial-canary-state.json"))
	if err != nil {
		return fmt.Errorf("read initial canary state: %w", err)
	}
	var initialState CanaryState
	if err := json.Unmarshal(initialStateData, &initialState); err != nil {
		return fmt.Errorf("parse initial canary state: %w", err)
	}

	// Load final canary state
	finalStateData, err := os.ReadFile(filepath.Join(artifactPath, "final-canary-state.json"))
	if err != nil {
		return fmt.Errorf("read final canary state: %w", err)
	}
	var finalState CanaryState
	if err := json.Unmarshal(finalStateData, &finalState); err != nil {
		return fmt.Errorf("parse final canary state: %w", err)
	}

	// Load container inspect
	inspectData, err := os.ReadFile(filepath.Join(artifactPath, "container-inspect.json"))
	if err != nil {
		return fmt.Errorf("read container inspect: %w", err)
	}
	var inspect ContainerInspect
	if err := json.Unmarshal(inspectData, &inspect); err != nil {
		return fmt.Errorf("parse container inspect: %w", err)
	}

	// 6. Reconstruct claims and verify
	var verifyErrors []string

	// Verify scenario consistency
	if verdict.Scenario != manifest.Scenario {
		verifyErrors = append(verifyErrors, fmt.Sprintf("verdict scenario=%s != manifest scenario=%s", verdict.Scenario, manifest.Scenario))
	}

	// Verify canary mode matches scenario
	expectedMode := scenarioToMode(manifest.Scenario)
	if initialState.Mode != expectedMode {
		verifyErrors = append(verifyErrors, fmt.Sprintf("initial mode=%s != expected=%s", initialState.Mode, expectedMode))
	}
	if finalState.Mode != expectedMode {
		verifyErrors = append(verifyErrors, fmt.Sprintf("final mode=%s != expected=%s", finalState.Mode, expectedMode))
	}

	// Verify container command matches scenario
	expectedCmd := getScenarioCommand(manifest.Scenario)
	if len(inspect.Config.Cmd) != len(expectedCmd) {
		verifyErrors = append(verifyErrors, fmt.Sprintf("inspect Cmd length=%d != expected=%d", len(inspect.Config.Cmd), len(expectedCmd)))
	} else {
		for i, cmd := range inspect.Config.Cmd {
			if cmd != expectedCmd[i] {
				verifyErrors = append(verifyErrors, fmt.Sprintf("inspect Cmd[%d]=%s != expected=%s", i, cmd, expectedCmd[i]))
			}
		}
	}

	// Verify workload counts
	if workload.Requested != workload.Attempted || workload.Attempted != workload.Completed || workload.Failed != 0 {
		verifyErrors = append(verifyErrors, fmt.Sprintf("workload counts: req=%d att=%d com=%d fail=%d (expected req=att=com, fail=0)",
			workload.Requested, workload.Attempted, workload.Completed, workload.Failed))
	}

	// Verify state deltas match scenario
	opDelta := finalState.OperationCount - initialState.OperationCount
	if opDelta != workload.Completed {
		verifyErrors = append(verifyErrors, fmt.Sprintf("operation_count_delta=%d != completed=%d", opDelta, workload.Completed))
	}

	switch manifest.Scenario {
	case "canary-growing":
		blocksDelta := finalState.RetainedBlocks - initialState.RetainedBlocks
		if blocksDelta != workload.Completed {
			verifyErrors = append(verifyErrors, fmt.Sprintf("growing: blocks_delta=%d != completed=%d", blocksDelta, workload.Completed))
		}
		bytesDelta := finalState.RetainedBytes - initialState.RetainedBytes
		expectedBytes := int64(workload.Completed) * 1048576
		if bytesDelta != expectedBytes {
			verifyErrors = append(verifyErrors, fmt.Sprintf("growing: bytes_delta=%d != expected=%d", bytesDelta, expectedBytes))
		}

	case "canary-bounded":
		if finalState.RetainedBlocks != 0 || finalState.RetainedBytes != 0 {
			verifyErrors = append(verifyErrors, fmt.Sprintf("bounded: retained should be 0, got blocks=%d bytes=%d",
				finalState.RetainedBlocks, finalState.RetainedBytes))
		}
	}

	// Load and verify samples with strict parser
	samplesData, err := os.ReadFile(filepath.Join(artifactPath, "samples.csv"))
	if err != nil {
		return fmt.Errorf("read samples: %w", err)
	}
	csvSamples, err := ParseSamplesCSVStream(strings.NewReader(string(samplesData)))
	if err != nil {
		return fmt.Errorf("parse samples CSV: %w", err)
	}

	// Verify required phases exist
	hasBaseline := false
	hasFinal := false
	finalCount := 0
	for _, s := range csvSamples {
		if s.Phase == "baseline" {
			hasBaseline = true
		}
		if s.Phase == "final" {
			hasFinal = true
			finalCount++
		}
	}
	if !hasBaseline {
		verifyErrors = append(verifyErrors, "missing baseline phase samples")
	}
	if !hasFinal {
		verifyErrors = append(verifyErrors, "missing final phase samples")
	}
	if finalCount < 3 {
		verifyErrors = append(verifyErrors, fmt.Sprintf("insufficient final samples: %d < 3", finalCount))
	}

	// Verify PID stability using correct field names
	if len(csvSamples) >= 2 {
		firstPID := csvSamples[0].PID
		firstStartTime := csvSamples[0].ProcessStartTime
		for _, s := range csvSamples[1:] {
			if s.PID != firstPID || s.ProcessStartTime != firstStartTime {
				verifyErrors = append(verifyErrors, fmt.Sprintf("PID instability: PID changed from %d or start time changed", firstPID))
				break
			}
		}
	}

	// Verify stored verdict matches reconstruction
	if verdict.ScenarioValid != verifyScenarioValid(manifest.Scenario, csvSamples, workload, verifyErrors) {
		verifyErrors = append(verifyErrors, "stored ScenarioValid does not match reconstruction")
	}

	// Print verification results
	fmt.Printf("=== Verification Results ===\n")
	fmt.Printf("Run ID: %s\n", *runID)
	fmt.Printf("Scenario: %s\n", verdict.Scenario)
	fmt.Printf("Reconstructed Claims: %d checks passed\n", len(manifest.ArtifactInventory)+5-len(verifyErrors))

	if len(verifyErrors) > 0 {
		fmt.Printf("Verification Errors:\n")
		for _, e := range verifyErrors {
			fmt.Printf("  - %s\n", e)
		}
		return fmt.Errorf("evidence verification failed: %d errors", len(verifyErrors))
	}

	fmt.Printf("All Verifications: PASS\n")
	fmt.Printf("ScenarioValid: %v\n", verdict.ScenarioValid)
	fmt.Printf("CanariesValid: %v\n", verdict.CanariesValid)
	fmt.Printf("Overall: %s\n", verdict.OverallClassification)
	fmt.Printf("Memory: %s\n", verdict.MemoryClassification)
	fmt.Printf("Checksums: PASS\n")
	fmt.Printf("Artifact Geometry: PASS\n")
	fmt.Printf("Evidence Reconstruction: PASS\n")

	if verdict.ScenarioValid && verdict.CanariesValid {
		fmt.Printf("PASS: Evidence verified\n")
		return nil
	}

	return fmt.Errorf("verdict indicates scenario or canaries not valid")
}

// verifyScenarioValid reconstructs the scenario validity from evidence
func verifyScenarioValid(scenario string, samples []sampling.Sample, workload WorkloadResult, verifyErrors []string) bool {
	if len(verifyErrors) > 0 {
		return false
	}
	hasBaseline := false
	hasFinal := false
	for _, s := range samples {
		if s.Phase == sampling.PhaseBaseline {
			hasBaseline = true
		}
		if s.Phase == sampling.PhaseFinal {
			hasFinal = true
		}
	}
	return hasBaseline && hasFinal && workload.Completed == workload.Requested && len(samples) > 0
}

// ContainerInspect represents the container inspect data
type ContainerInspect struct {
	Path   string `json:"Path"`
	Config struct {
		Cmd []string `json:"Cmd"`
	} `json:"Config"`
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	return lines
}

func scenarioToMode(scenario string) string {
	switch scenario {
	case "canary-growing":
		return "growing"
	case "canary-bounded":
		return "bounded"
	case "canary-descriptor":
		return "descriptor"
	default:
		return "bounded"
	}
}

// waitForCanaryHealth waits for the canary to be healthy.
func waitForCanaryHealth(ctx context.Context, client *http.Client, url string, timeout time.Duration, verbose bool) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url+"/health", nil)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				if verbose {
					fmt.Printf("Canary is healthy\n")
				}
				return nil
			}
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for canary health")
}

// fetchCanaryState fetches the current canary state.
func fetchCanaryState(ctx context.Context, client *http.Client, url string) (*CanaryState, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url+"/state", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /state: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var state CanaryState
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}

	return &state, nil
}

// operateCanary sends operate requests to the canary.
func operateCanary(ctx context.Context, client *http.Client, url string, count int) (*WorkloadResult, error) {
	opClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
		},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url+"/operate?count="+fmt.Sprintf("%d", count), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := opClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST /operate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("operate failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	// Parse operate response
	var opResult struct {
		Attempted int `json:"attempted"`
		Completed int `json:"completed"`
	}
	if err := json.Unmarshal(body, &opResult); err != nil {
		return nil, fmt.Errorf("parse operate response: %w", err)
	}

	return &WorkloadResult{
		Requested: count,
		Attempted: opResult.Attempted,
		Completed: opResult.Completed,
		Failed:    count - opResult.Completed,
		Returned:  opResult.Completed,
	}, nil
}

// getScenarioOperationCount returns the operation count for each scenario.
func getScenarioOperationCount(scenario string) int {
	switch scenario {
	case "canary-growing":
		return 32 // 32 MiB retained (32 blocks × 1 MiB)
	case "canary-bounded":
		return 100 // 100 operations against fixed buffer
	case "canary-descriptor":
		return 100 // 100 operations = 200 retained descriptors
	default:
		return 32
	}
}

// classifyCgroupFailureWithNamespace classifies cgroup failure with namespace comparison.
// Returns capability and proof for verifier to reconstruct the decision.
// Uses typed errors for reliable classification.
//
// Classification rules:
// - Observed mismatch in required namespace → corresponding mismatch result
// - Any required identity unavailable → namespace_identity_unavailable
// - No mismatch observed → not_mounted
func classifyCgroupFailureWithNamespace(err error, targetPID, controllerPID int) (sampling.CgroupCapability, *sampling.NamespaceProof) {
	proof := &sampling.NamespaceProof{}

	if err == nil {
		return sampling.CgroupCapabilityAvailable, nil
	}

	// Classify by error type using typed errors
	var capability sampling.CgroupCapability
	switch {
	case errors.Is(err, procfs.ErrNoCgroup2Mount):
		capability = sampling.CgroupCapabilityCgroupNotVisible
	case errors.Is(err, procfs.ErrNoUnifiedCgroup):
		capability = sampling.CgroupCapabilityNoUnifiedHierarchy
	case errors.Is(err, procfs.ErrPathTraversal):
		capability = sampling.CgroupCapabilityPathTraversal
	default:
		// Check for permission/parse errors
		errStr := err.Error()
		if strings.Contains(errStr, "permission denied") || strings.Contains(errStr, "permission") {
			capability = sampling.CgroupCapabilityPermissionDenied
		} else if strings.Contains(errStr, "parse") {
			capability = sampling.CgroupCapabilityParseFailure
		} else {
			capability = sampling.CgroupCapabilityPathAbsent
		}
	}

	// Read namespace IDs for both processes
	targetNS, _ := procfs.ReadNamespaceIDs(targetPID)
	controllerNS, _ := procfs.ReadNamespaceIDs(controllerPID)

	// Populate proof with what we read
	if targetNS != nil {
		proof.TargetMountNamespace = targetNS.MountNamespace
		proof.TargetCgroupNamespace = targetNS.CgroupNamespace
		if targetNS.MountNamespaceErr != nil {
			proof.TargetMountNamespaceErr = targetNS.MountNamespaceErr.Error()
		}
		if targetNS.CgroupNamespaceErr != nil {
			proof.TargetCgroupNamespaceErr = targetNS.CgroupNamespaceErr.Error()
		}
	}
	if controllerNS != nil {
		proof.ControllerMountNamespace = controllerNS.MountNamespace
		proof.ControllerCgroupNamespace = controllerNS.CgroupNamespace
		if controllerNS.MountNamespaceErr != nil {
			proof.ControllerMountNamespaceErr = controllerNS.MountNamespaceErr.Error()
		}
		if controllerNS.CgroupNamespaceErr != nil {
			proof.ControllerCgroupNamespaceErr = controllerNS.CgroupNamespaceErr.Error()
		}
	}

	// For cgroup visibility errors, attempt namespace comparison
	if capability == sampling.CgroupCapabilityCgroupNotVisible ||
		capability == sampling.CgroupCapabilityNoUnifiedHierarchy {
		
		canProveMount := targetNS != nil && targetNS.MountNamespaceErr == nil &&
			controllerNS != nil && controllerNS.MountNamespaceErr == nil
		canProveCgroup := targetNS != nil && targetNS.CgroupNamespaceErr == nil &&
			controllerNS != nil && controllerNS.CgroupNamespaceErr == nil

		// Check mount namespace mismatch first
		if canProveMount &&
			proof.TargetMountNamespace != "" && proof.ControllerMountNamespace != "" &&
			proof.TargetMountNamespace != proof.ControllerMountNamespace {
			proof.DecisionReason = "mount_namespace_differ"
			return sampling.CgroupCapabilityMountNamespaceMismatch, proof
		}

		// Check cgroup namespace mismatch
		if canProveCgroup &&
			proof.TargetCgroupNamespace != "" && proof.ControllerCgroupNamespace != "" &&
			proof.TargetCgroupNamespace != proof.ControllerCgroupNamespace {
			proof.DecisionReason = "cgroup_namespace_differ"
			return sampling.CgroupCapabilityCgroupNamespaceMismatch, proof
		}

		// Fail-closed: if we needed to prove identity but couldn't, return unavailable
		// The error was cgroup visibility, but we couldn't complete the namespace comparison
		if !canProveMount || !canProveCgroup {
			proof.DecisionReason = "namespace_identity_unavailable"
			return sampling.CgroupCapabilityNamespaceIdentityUnavail, proof
		}

		// Both identities proven equal → cgroup not visible to this namespace
		proof.DecisionReason = "namespaces_equal_cgroup_not_visible"
		return sampling.CgroupCapabilityNotMounted, proof
	}

	// For non-visibility errors, return the classified capability with proof
	proof.DecisionReason = capability.String()
	return capability, proof
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// collectProvenance collects git, kernel, and binary provenance information.
// Returns error if required provenance is missing - fail-closed for evidence integrity.
func collectProvenance() (*evidence.SubjectIdentity, *evidence.HostIdentity, string, error) {
	var errs []string

	// Git commit and tree
	gitCommit, gitErr := runGit("rev-parse", "HEAD")
	if gitErr != nil {
		errs = append(errs, fmt.Sprintf("git commit: %v", gitErr))
	}
	gitTree, treeErr := runGit("rev-parse", "HEAD^{tree}")
	if treeErr != nil {
		errs = append(errs, fmt.Sprintf("git tree: %v", treeErr))
	}

	// Controller PID
	controllerPID := fmt.Sprintf("%d", os.Getpid())

	// Controller executable path and hash
	selfPath, pathErr := os.Readlink("/proc/self/exe")
	selfHash := ""
	selfHashErr := error(nil)
	if pathErr != nil {
		errs = append(errs, fmt.Sprintf("executable path: %v", pathErr))
	} else if selfPath == "" {
		errs = append(errs, "executable path: empty")
	} else {
		var hashErr error
		selfHash, hashErr = hashFile(selfPath)
		if hashErr != nil {
			// Hash failure is critical - add to errs so status is not "complete"
			errs = append(errs, fmt.Sprintf("executable hash: %v", hashErr))
			selfHashErr = hashErr
		}
	}

	// Kernel release using uname -r semantics
	kernelRelease, krErr := runUname("-r")
	if krErr != nil {
		kernelRelease = ""
		errs = append(errs, fmt.Sprintf("kernel release: %v", krErr))
	}

	// Kernel version (full string)
	kernelVersion, err := os.ReadFile("/proc/version")
	kernelVersionStr := ""
	if err == nil {
		kernelVersionStr = strings.TrimSpace(string(kernelVersion))
	} else {
		errs = append(errs, fmt.Sprintf("kernel version: %v", err))
	}

	// Cgroup mode - prefer reading from /proc/1/cgroup or mountinfo
	cgroupMode := detectCgroupMode()

	// Provenance status - fail-closed: if hash failed, status is NOT "complete"
	status := "complete"
	if len(errs) > 0 {
		status = fmt.Sprintf("partial: %s", strings.Join(errs, "; "))
	}

	subject := &evidence.SubjectIdentity{
		GitCommit:                 gitCommit,
		GitTree:                   gitTree,
		ControllerExecutablePath:   selfPath,
		ControllerExecutableSHA256: selfHash,
	}
	host := &evidence.HostIdentity{
		KernelRelease:    kernelRelease,
		KernelVersion:    kernelVersionStr,
		CgroupMode:       cgroupMode,
		CollectionStatus: status,
	}

	// Return error if executable hash failed - this is required provenance
	if selfHashErr != nil {
		return subject, host, controllerPID, fmt.Errorf("required provenance unavailable: executable hash failed: %w", selfHashErr)
	}

	return subject, host, controllerPID, nil
}

// runGit runs a git command and returns the trimmed output.
func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = "/home/kgb/Projects/KGB"
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// runUname runs the uname command and returns the trimmed output.
func runUname(args ...string) (string, error) {
	cmd := exec.Command("uname", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// detectCgroupMode determines cgroup mode using authoritative sources.
// Priority: /proc/1/cgroup > mountinfo > /proc/cgroups.
// Returns one of: "cgroup1", "cgroup2", "hybrid", "unknown".
// Never returns a default when detection is inconclusive.
func detectCgroupMode() string {
	// Try to read /proc/1/cgroup to determine host cgroup mode
	// Format: hierarchy-id:controllers:control-group-path
	// - hierarchy-id == 0 with empty controllers = v2 (unified)
	// - hierarchy-id > 0 with controllers = v1
	// - both present = hybrid
	data, err := os.ReadFile("/proc/1/cgroup")
	if err == nil {
		hasV2 := false
		hasV1 := false
		for _, line := range strings.Split(string(data), "\n") {
			if line == "" {
				continue
			}
			// Parse cgroup record: hierarchy-id:controllers:path
			parts := strings.SplitN(line, ":", 3)
			if len(parts) < 2 {
				continue
			}
			hierarchyID := strings.TrimSpace(parts[0])
			controllers := ""
			if len(parts) >= 2 {
				controllers = strings.TrimSpace(parts[1])
			}

			// v2: hierarchy-id is "0" and controllers is empty or "unified"
			if hierarchyID == "0" && (controllers == "" || controllers == "unified" || strings.HasPrefix(controllers, "0::")) {
				hasV2 = true
			}
			// v1: hierarchy-id is non-zero or controllers has controller names
			if hierarchyID != "0" || (controllers != "" && controllers != "unified") {
				// Also check for named hierarchies (e.g., "1:name=systemd:")
				if strings.Contains(controllers, "name=") || hierarchyID != "0" {
					hasV1 = true
				}
			}
		}
		if hasV2 && hasV1 {
			return "hybrid"
		}
		if hasV2 {
			return "cgroup2"
		}
		if hasV1 {
			return "cgroup1"
		}
	}

	// Fallback: check mountinfo for cgroup2 mount
	mountData, err := os.ReadFile("/proc/self/mountinfo")
	if err == nil {
		hasCgroup2 := false
		hasCgroup1 := false
		for _, line := range strings.Split(string(mountData), "\n") {
			if line == "" {
				continue
			}
			// Parse mountinfo: find the '-' separator
			parts := strings.Split(line, " - ")
			if len(parts) != 2 {
				continue
			}
			// The filesystem type is the first field after '-'
			fsParts := strings.Split(strings.TrimSpace(parts[1]), " ")
			if len(fsParts) < 1 {
				continue
			}
			fsType := fsParts[0]
			if fsType == "cgroup2" || fsType == "cgroup2fs" {
				hasCgroup2 = true
			}
			if fsType == "cgroup" || fsType == "cgroupfs" {
				hasCgroup1 = true
			}
		}
		if hasCgroup2 && hasCgroup1 {
			return "hybrid"
		}
		if hasCgroup2 {
			return "cgroup2"
		}
		if hasCgroup1 {
			return "cgroup1"
		}
	}

	// Cannot determine - return unknown instead of guessing
	return "unknown"
}

// hashFile computes SHA-256 hash of a file.
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// validateStateInvariant validates the state changes match expected invariants.
func validateStateInvariant(scenario string, initial, final *CanaryState, workload *WorkloadResult) *analysis.StateInvariantResult {
	result := &analysis.StateInvariantResult{Valid: true}

	opDelta := final.OperationCount - initial.OperationCount
	if opDelta != workload.Completed {
		result.Valid = false
		result.Failures = append(result.Failures,
			fmt.Sprintf("operation_count_delta mismatch: expected %d, got %d", workload.Completed, opDelta))
	}

	switch scenario {
	case "canary-growing":
		if opDelta != workload.Completed {
			result.Valid = false
			result.Failures = append(result.Failures, "growing: operation_count_delta != completed")
		}
		blocksDelta := final.RetainedBlocks - initial.RetainedBlocks
		if blocksDelta != workload.Completed {
			result.Valid = false
			result.Failures = append(result.Failures,
				fmt.Sprintf("growing: retained_blocks_delta=%d != completed=%d", blocksDelta, workload.Completed))
		}
		bytesDelta := final.RetainedBytes - initial.RetainedBytes
		expectedBytes := int64(workload.Completed) * 1048576 // 1 MiB block size
		if bytesDelta != expectedBytes {
			result.Valid = false
			result.Failures = append(result.Failures,
				fmt.Sprintf("growing: retained_bytes_delta=%d != expected=%d", bytesDelta, expectedBytes))
		}

	case "canary-bounded":
		if opDelta != workload.Completed {
			result.Valid = false
			result.Failures = append(result.Failures, "bounded: operation_count_delta != completed")
		}
		if initial.BufferCapacity != final.BufferCapacity {
			result.Valid = false
			result.Failures = append(result.Failures,
				fmt.Sprintf("bounded: buffer_capacity changed from %d to %d", initial.BufferCapacity, final.BufferCapacity))
		}
		if final.RetainedBlocks != 0 {
			result.Valid = false
			result.Failures = append(result.Failures,
				fmt.Sprintf("bounded: retained_blocks should be 0, got %d", final.RetainedBlocks))
		}
		if final.RetainedBytes != 0 {
			result.Valid = false
			result.Failures = append(result.Failures,
				fmt.Sprintf("bounded: retained_bytes should be 0, got %d", final.RetainedBytes))
		}

	case "canary-descriptor":
		if opDelta != workload.Completed {
			result.Valid = false
			result.Failures = append(result.Failures, "descriptor: operation_count_delta != completed")
		}
		fdDelta := final.FDCount - initial.FDCount
		expectedFDDelta := workload.Completed * 2 // Each operation leaks 2 FDs
		if fdDelta != expectedFDDelta {
			result.Valid = false
			result.Failures = append(result.Failures,
				fmt.Sprintf("descriptor: fd_delta=%d != expected=%d", fdDelta, expectedFDDelta))
		}
	}

	return result
}
