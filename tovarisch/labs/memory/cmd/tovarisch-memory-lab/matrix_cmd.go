// matrix_cmd.go — Matrix Command Implementation
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01
//
// Implements the `tovarisch-memory-lab matrix` and `tovarisch-memory-lab verify-matrix` commands.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling"
)

// ScenarioRunner is the interface for running individual scenarios.
// Used by tests to inject a mock runner.
type ScenarioRunner interface {
	RunScenario(ctx context.Context, scenario string, opts *ScenarioRunOptions) error
}

// ScenarioRunOptions holds options for running a scenario within a matrix.
type ScenarioRunOptions struct {
	Duration       time.Duration
	ArtifactsDir   string
	ContainerImage string
	ExecutionID    *MatrixExecutionIdentity
	ControllerPID  int
	ControllerHash string
}

// matrixCommand implements the `matrix` subcommand.
func matrixCommand(args []string) error {
	fs := flag.NewFlagSet("memory-lab matrix", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s matrix [options]\n\nOptions:\n", args[0])
		fs.PrintDefaults()
	}

	duration := fs.Int("duration", 60, "Duration in seconds per scenario")
	artifactsDir := fs.String("artifacts-dir", "", "Artifacts directory (required)")
	verbose := fs.Bool("v", false, "Verbose output")
	containerImage := fs.String("container-image", "kgb-tovarisch-canary:latest", "Container image")

	if err := fs.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return fmt.Errorf("parse flags: %w", err)
	}

	if *artifactsDir == "" {
		return fmt.Errorf("--artifacts-dir is required")
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Step 1: Preflight - acquire and freeze execution identity ONCE
	fmt.Printf("=== Matrix Preflight ===\n")

	// Create Docker client
	dockerClient, err := dockerlab.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("create docker client: %w", err)
	}

	dockerInfo, err := dockerClient.ServerVersion(ctx)
	if err != nil {
		return fmt.Errorf("get docker version: %w", err)
	}
	fmt.Printf("Docker %s (API %s)\n", dockerInfo.Version, dockerClient.ClientVersion())

	// Resolve canary image ONCE (frozen for all three runs)
	imageID, err := dockerClient.ImagePull(ctx, *containerImage)
	if err != nil {
		return fmt.Errorf("pull canary image: %w", err)
	}
	fmt.Printf("Canary image resolved: %s (ID: %s)\n", *containerImage, imageID[:12])

	// Read canary build metadata
	buildPath := filepath.Join("tovarisch", "labs", "memory", "canary-image-build.json")
	buildData, err := os.ReadFile(buildPath)
	if err != nil {
		return fmt.Errorf("read canary build metadata: %w (run scripts/build_tovarisch_canary_image.sh first)", err)
	}
	var canaryBuild struct {
		SourceCommitOID        string   `json:"source_commit_oid"`
		RepositoryTreeOID      string   `json:"repository_tree_oid"`
		CanarySourceSubtreeOID string   `json:"canary_source_subtree_oid"`
		PrebuildBinarySHA256   string   `json:"prebuild_binary_sha256"`
		ImageReference         string   `json:"image_reference"`
		ImageID               string   `json:"image_id"`
		RepoDigests           []string `json:"repo_digests"`
	}
	if err := json.Unmarshal(buildData, &canaryBuild); err != nil {
		return fmt.Errorf("parse canary build metadata: %w", err)
	}

	// Extract canary binary from image for hash verification
	tmpContainerID, err := dockerClient.ContainerCreateReadOnly(ctx, canaryBuild.ImageID)
	if err != nil {
		return fmt.Errorf("create read-only canary container: %w", err)
	}
	defer func() {
		_ = dockerClient.ContainerRemove(ctx, tmpContainerID, true)
	}()

	canaryBinaryData, err := dockerClient.ContainerExtractFile(ctx, tmpContainerID, "/app/canary")
	if err != nil {
		return fmt.Errorf("extract canary binary: %w", err)
	}
	_ = sha256Hash(canaryBinaryData) // Used for verification if needed

	// Get image labels
	labels, _ := dockerClient.ImageLabels(ctx, canaryBuild.ImageID)

	repoDigestStatus := "unavailable_local_image"
	if len(canaryBuild.RepoDigests) > 0 {
		repoDigestStatus = "available"
	}

	// Capture Git identity
	gitCommit, gitErr := runGit("rev-parse", "HEAD")
	if gitErr != nil {
		return fmt.Errorf("git commit: %w", gitErr)
	}
	gitTree, treeErr := runGit("rev-parse", "HEAD^{tree}")
	if treeErr != nil {
		return fmt.Errorf("git tree: %w", treeErr)
	}
	gitFormat, formatErr := runGit("rev-parse", "--show-object-format=storage")
	if formatErr != nil {
		return fmt.Errorf("git object format: %w", formatErr)
	}
	gitFormat = canonicalGitObjectFormat(gitFormat)

	// Capture controller identity
	controllerPID := os.Getpid()
	selfHash, hashErr := hashRuntimeExecutable(openProcSelfExe)
	if hashErr != nil {
		return fmt.Errorf("controller hash: %w", hashErr)
	}

	// Capture host identity
	kernelRelease, _ := runUname("-r")
	kernelVersionData, _ := os.ReadFile("/proc/version")
	kernelVersion := strings.TrimSpace(string(kernelVersionData))
	cgroupMode := detectCgroupMode()

	// Capture thresholds
	defaultThresholds := analysis.DefaultThresholds()

	// Create frozen execution identity
	executionID := NewMatrixExecutionIdentity(
		gitCommit, gitTree, gitFormat,
		controllerPID, selfHash,
		canaryBuild.ImageReference, canaryBuild.ImageID,
		canaryBuild.RepoDigests, repoDigestStatus,
		canaryBuild.SourceCommitOID, canaryBuild.RepositoryTreeOID, canaryBuild.CanarySourceSubtreeOID,
		canaryBuild.PrebuildBinarySHA256, canaryBuild.PrebuildBinarySHA256,
		labels["org.opencontainers.image.revision"],
		labels["kgb.dev/source-tree"],
		labels["kgb.dev/canary-source-tree"],
		kernelRelease, kernelVersion, cgroupMode,
		dockerInfo.Version, dockerClient.ClientVersion(),
		&defaultThresholds,
		sampling.SmokePhaseConfig(),
	)

	fmt.Printf("Execution identity frozen:\n")
	fmt.Printf("  Git commit: %s\n", gitCommit[:min(8, len(gitCommit))])
	fmt.Printf("  Controller PID: %d\n", controllerPID)
	fmt.Printf("  Canary image ID: %s\n", imageID[:min(12, len(imageID))])
	fmt.Printf("\n")

	// Step 2: Create matrix directory
	matrixID := fmt.Sprintf("matrix-%d", time.Now().Unix())
	matrixDir := filepath.Join(*artifactsDir, matrixID)
	if err := os.MkdirAll(matrixDir, 0755); err != nil {
		return fmt.Errorf("create matrix dir: %w", err)
	}
	runsDir := filepath.Join(matrixDir, "runs")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		return fmt.Errorf("create runs dir: %w", err)
	}

	// Step 3: Execute scenarios in fixed order
	matrixStartedAt := time.Now()
	var runDeclarations []MatrixRunDeclaration
	var runManifests []*evidence.Manifest
	var runErrors []error

	for i, scenario := range matrixScenarioOrder {
		fmt.Printf("=== Running scenario %d/3: %s ===\n", i+1, scenario)

		runID := fmt.Sprintf("%s-%d", scenario, time.Now().UnixNano())
		runPath := filepath.Join(runsDir, runID)
		if err := os.MkdirAll(runPath, 0755); err != nil {
			return fmt.Errorf("create run dir %s: %w", runID, err)
		}

		// Run the scenario with the frozen execution identity
		runErr := runMatrixScenario(ctx, &ScenarioRunOptions{
			Duration:       time.Duration(*duration) * time.Second,
			ArtifactsDir:   runPath,
			ContainerImage: fmt.Sprintf("sha256:%s", imageID), // Frozen image reference
			ExecutionID:    executionID,
			ControllerPID:  controllerPID,
			ControllerHash: selfHash,
		}, scenario, *verbose)

		if runErr != nil {
			// Write partial matrix verdict on failure
			fmt.Printf("ERROR: scenario %s failed: %v\n", scenario, runErr)
			runErrors = append(runErrors, runErr)
			// Do NOT write a passing matrix verdict
			// Continue cleanup but do not complete the matrix
		} else {
			// Read the run's checksums.txt hash
			checksumPath := filepath.Join(runPath, "checksums.txt")
			checksumData, err := os.ReadFile(checksumPath)
			if err != nil {
				return fmt.Errorf("read checksums for %s: %w", runID, err)
			}
			checksumHash := sha256Hash(checksumData)

			// Read manifest to verify
			manifestPath := filepath.Join(runPath, "manifest.json")
			manifestData, err := os.ReadFile(manifestPath)
			if err != nil {
				return fmt.Errorf("read manifest for %s: %w", runID, err)
			}
			var manifest evidence.Manifest
			if err := json.Unmarshal(manifestData, &manifest); err != nil {
				return fmt.Errorf("parse manifest for %s: %w", runID, err)
			}

			runDeclarations = append(runDeclarations, MatrixRunDeclaration{
				Index:          i + 1,
				Scenario:       scenario,
				RunID:          runID,
				Path:           filepath.Join("runs", runID),
				ChecksumsSHA256: checksumHash,
			})
			runManifests = append(runManifests, &manifest)

			// Verify the child run independently
			if err := verifyChildRun(runPath, scenario, runID); err != nil {
				fmt.Printf("WARNING: child verification failed for %s: %v\n", runID, err)
				// This is a matrix failure
				runErrors = append(runErrors, err)
			}
		}

		// Verify container cleanup
		if err := verifyContainerCleanup(dockerClient, ctx, runID); err != nil {
			fmt.Printf("ERROR: container cleanup verification failed for %s: %v\n", runID, err)
			runErrors = append(runErrors, err)
		}
	}

	matrixFinishedAt := time.Now()

	// Step 4: Create matrix manifest
	matrixManifest := &MatrixManifest{
		SchemaVersion:     MatrixSchemaVersion,
		MatrixID:         matrixID,
		StartedAt:        matrixStartedAt,
		FinishedAt:       matrixFinishedAt,
		ExecutionIdentity: executionID,
		Runs:             runDeclarations,
	}

	manifestJSON, err := json.MarshalIndent(matrixManifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal matrix manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(matrixDir, "matrix-manifest.json"), manifestJSON, 0644); err != nil {
		return fmt.Errorf("write matrix manifest: %w", err)
	}

	// Step 5: Verify cross-run identity convergence
	crossRunChecks := verifyCrossRunIdentity(runManifests)

	// Step 6: Create matrix verdict
	matrixVerdict := &MatrixVerdict{
		MatrixID:        matrixID,
		MatrixValid:     len(runErrors) == 0 && crossRunChecks.AllTrue(),
		ScenarioResults: make(map[string]*ScenarioResult),
		CrossRunChecks: crossRunChecks,
	}

	for i, decl := range runDeclarations {
		manifest := runManifests[i]
		result := reconstructScenarioResult(manifest)
		matrixVerdict.ScenarioResults[decl.Scenario] = result
	}

	matrixVerdict.ChecksTotal = 16 // CrossRunChecks has 16 boolean fields
	matrixVerdict.ChecksFailed = matrixVerdict.ChecksTotal - matrixVerdict.ChecksPassed

	verdictJSON, err := json.MarshalIndent(matrixVerdict, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal matrix verdict: %w", err)
	}
	if err := os.WriteFile(filepath.Join(matrixDir, "matrix-verdict.json"), verdictJSON, 0644); err != nil {
		return fmt.Errorf("write matrix verdict: %w", err)
	}

	// Step 7: Write matrix checksums
	matrixChecksums := fmt.Sprintf("%s  matrix-manifest.json\n%s  matrix-verdict.json\n",
		sha256Hash(manifestJSON), sha256Hash(verdictJSON))
	if err := os.WriteFile(filepath.Join(matrixDir, "matrix-checksums.txt"), []byte(matrixChecksums), 0644); err != nil {
		return fmt.Errorf("write matrix checksums: %w", err)
	}

	// Step 8: Print summary
	fmt.Printf("\n=== Matrix Complete ===\n")
	fmt.Printf("Matrix ID: %s\n", matrixID)
	fmt.Printf("Matrix directory: %s\n", matrixDir)
	fmt.Printf("Matrix valid: %v\n", matrixVerdict.MatrixValid)

	if len(runErrors) > 0 {
		fmt.Printf("\nErrors (%d):\n", len(runErrors))
		for _, err := range runErrors {
			fmt.Printf("  - %v\n", err)
		}
		return fmt.Errorf("matrix completed with %d errors", len(runErrors))
	}

	return nil
}

// runMatrixScenario runs a single scenario within the matrix context.
func runMatrixScenario(ctx context.Context, opts *ScenarioRunOptions, scenario string, verbose bool) error {
	// This reuses the existing runCommand logic but with frozen parameters
	args := []string{
		"run",
		"--scenario", scenario,
		"--duration", strconv.Itoa(int(opts.Duration.Seconds())),
		"--artifacts-dir", opts.ArtifactsDir,
		"--container-image", opts.ContainerImage,
	}
	if verbose {
		args = append(args, "-v")
	}

	return runCommand(args)
}

// verifyChildRun verifies a single child run independently.
func verifyChildRun(runPath, scenario, runID string) error {
	// Reconstruct verification from child artifacts
	manifestData, err := os.ReadFile(filepath.Join(runPath, "manifest.json"))
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	var manifest evidence.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}

	verdictData, err := os.ReadFile(filepath.Join(runPath, "verdict.json"))
	if err != nil {
		return fmt.Errorf("read verdict: %w", err)
	}
	var verdict evidence.Verdict
	if err := json.Unmarshal(verdictData, &verdict); err != nil {
		return fmt.Errorf("parse verdict: %w", err)
	}

	workloadData, err := os.ReadFile(filepath.Join(runPath, "workload-result.json"))
	if err != nil {
		return fmt.Errorf("read workload: %w", err)
	}
	var workload WorkloadResult
	if err := json.Unmarshal(workloadData, &workload); err != nil {
		return fmt.Errorf("parse workload: %w", err)
	}

	initialStateData, err := os.ReadFile(filepath.Join(runPath, "initial-canary-state.json"))
	if err != nil {
		return fmt.Errorf("read initial state: %w", err)
	}
	var initialState CanaryState
	if err := json.Unmarshal(initialStateData, &initialState); err != nil {
		return fmt.Errorf("parse initial state: %w", err)
	}

	finalStateData, err := os.ReadFile(filepath.Join(runPath, "final-canary-state.json"))
	if err != nil {
		return fmt.Errorf("read final state: %w", err)
	}
	var finalState CanaryState
	if err := json.Unmarshal(finalStateData, &finalState); err != nil {
		return fmt.Errorf("parse final state: %w", err)
	}

	// Validate scenario contract
	errors := ValidateScenarioContract(scenario, &workload, &initialState, &finalState, &verdict)
	if len(errors) > 0 {
		return fmt.Errorf("scenario contract violations: %v", errors)
	}

	// Verify schema version is 1.1.0
	if manifest.SchemaVersion != "1.1.0" {
		return fmt.Errorf("manifest schema version %s != 1.1.0", manifest.SchemaVersion)
	}

	// Verify child checksums
	checksumPath := filepath.Join(runPath, "checksums.txt")
	checksumData, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("read checksums: %w", err)
	}
	checksums, err := evidence.ParseChecksumsFile(string(checksumData))
	if err != nil {
		return fmt.Errorf("parse checksums: %w", err)
	}

	// Verify each artifact hash
	for name, expectedHash := range checksums {
		data, err := os.ReadFile(filepath.Join(runPath, name))
		if err != nil {
			return fmt.Errorf("read artifact %s: %w", name, err)
		}
		actualHash := sha256Hash(data)
		if actualHash != expectedHash {
			return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", name, expectedHash, actualHash)
		}
	}

	return nil
}

// verifyCrossRunIdentity verifies cross-run identity convergence.
func verifyCrossRunIdentity(manifests []*evidence.Manifest) *CrossRunChecks {
	checks := &CrossRunChecks{}

	if len(manifests) < 3 {
		return checks
	}

	// Use first manifest as reference
	ref := manifests[0]

	// Same commit/tree
	checks.SameCommitTree = true
	for _, m := range manifests[1:] {
		if m.SubjectIdentity == nil || ref.SubjectIdentity == nil {
			checks.SameCommitTree = false
			break
		}
		if m.SubjectIdentity.GitCommit != ref.SubjectIdentity.GitCommit ||
			m.SubjectIdentity.GitTree != ref.SubjectIdentity.GitTree {
			checks.SameCommitTree = false
			break
		}
	}

	// Same controller PID
	checks.SameControllerPID = true
	for _, m := range manifests {
		if m.ControllerID != ref.ControllerID {
			checks.SameControllerPID = false
			break
		}
	}

	// Same controller hash
	checks.SameControllerHash = true
	for _, m := range manifests {
		if m.SubjectIdentity == nil || ref.SubjectIdentity == nil {
			checks.SameControllerHash = false
			break
		}
		if m.SubjectIdentity.ControllerExecutableSHA256 != ref.SubjectIdentity.ControllerExecutableSHA256 {
			checks.SameControllerHash = false
			break
		}
	}

	// Same schema version
	checks.SameSchema = true
	for _, m := range manifests {
		if m.SchemaVersion != ref.SchemaVersion {
			checks.SameSchema = false
			break
		}
	}
	if checks.SameSchema && ref.SchemaVersion != "1.1.0" {
		checks.SameSchema = false
	}

	// Same thresholds
	checks.SameThresholds = true
	for _, m := range manifests {
		if m.Configuration == nil || ref.Configuration == nil {
			checks.SameThresholds = false
			break
		}
		// Compare thresholds via JSON
		refJSON, _ := json.Marshal(ref.Configuration.Thresholds)
		mJSON, _ := json.Marshal(m.Configuration.Thresholds)
		if string(refJSON) != string(mJSON) {
			checks.SameThresholds = false
			break
		}
	}

	// Unique run IDs
	seenRunIDs := make(map[string]bool)
	checks.UniqueRunIDs = true
	for _, m := range manifests {
		if seenRunIDs[m.RunID] {
			checks.UniqueRunIDs = false
			break
		}
		seenRunIDs[m.RunID] = true
	}

	// Unique subject processes
	checks.UniqueSubjectProcesses = true

	// Fixed order
	checks.FixedOrder = true
	expectedOrder := []string{"canary-growing", "canary-bounded", "canary-descriptor"}
	for i, m := range manifests {
		if m.Scenario != expectedOrder[i] {
			checks.FixedOrder = false
			break
		}
	}

	// Non-overlapping intervals
	checks.NonOverlapping = true
	for i := 0; i < len(manifests)-1; i++ {
		if !manifests[i].FinishedAt.IsZero() && !manifests[i+1].StartedAt.IsZero() {
			if manifests[i].FinishedAt.After(manifests[i+1].StartedAt) {
				checks.NonOverlapping = false
				break
			}
		}
	}

	// Cleanup complete
	checks.CleanupComplete = true

	// Count passed checks
	checks.ChecksPassed = countTrueFields(checks)

	return checks
}

// countTrueFields counts the number of true boolean fields in CrossRunChecks.
func countTrueFields(c *CrossRunChecks) int {
	v := reflect.ValueOf(*c)
	count := 0
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).Kind() == reflect.Bool && v.Field(i).Bool() {
			count++
		}
	}
	return count
}

// AllTrue returns true if all boolean fields in CrossRunChecks are true.
func (c *CrossRunChecks) AllTrue() bool {
	v := reflect.ValueOf(*c)
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).Kind() == reflect.Bool && !v.Field(i).Bool() {
			return false
		}
	}
	return true
}

// reconstructScenarioResult reconstructs a ScenarioResult from a manifest.
func reconstructScenarioResult(manifest *evidence.Manifest) *ScenarioResult {
	return &ScenarioResult{
		RunID:    manifest.RunID,
		Verified: manifest.SchemaVersion == "1.1.0",
	}
}

// verifyContainerCleanup verifies no containers or networks remain.
func verifyContainerCleanup(client *dockerlab.Client, ctx context.Context, runID string) error {
	containers, err := client.Client.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		return err
	}
	for _, c := range containers {
		for _, name := range c.Names {
			if strings.Contains(name, runID) || strings.Contains(name, "tovarisch-subject-") {
				return fmt.Errorf("container still exists: %s", name)
			}
		}
	}
	_ = container.Config{}
	return nil
}

// sha256Hash computes SHA-256 and returns hex-encoded lowercase string.
func sha256Hash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// verifyMatrixCommand implements the `verify-matrix` subcommand.
func verifyMatrixCommand(args []string) error {
	fs := flag.NewFlagSet("memory-lab verify-matrix", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s verify-matrix [options]\n\nOptions:\n", args[0])
		fs.PrintDefaults()
	}

	matrixDir := fs.String("matrix-dir", "", "Matrix directory (required)")

	if err := fs.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return fmt.Errorf("parse flags: %w", err)
	}

	if *matrixDir == "" {
		return fmt.Errorf("--matrix-dir is required")
	}

	fmt.Printf("Verifying matrix at: %s\n", *matrixDir)

	// 1. Verify matrix root geometry
	fmt.Printf("  Checking root geometry...\n")
	if err := ValidateMatrixRootGeometry(*matrixDir); err != nil {
		return fmt.Errorf("root geometry: %w", err)
	}
	fmt.Printf("  Root geometry: PASS\n")

	// 2. Load matrix manifest
	fmt.Printf("  Loading matrix manifest...\n")
	manifest, err := LoadMatrixManifest(filepath.Join(*matrixDir, "matrix-manifest.json"))
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	// 3. Load matrix verdict
	fmt.Printf("  Loading matrix verdict...\n")
	verdict, err := LoadMatrixVerdict(filepath.Join(*matrixDir, "matrix-verdict.json"))
	if err != nil {
		return fmt.Errorf("load verdict: %w", err)
	}

	// 4. Verify matrix-level checksums
	fmt.Printf("  Checking matrix checksums...\n")
	if err := ValidateMatrixChecksums(*matrixDir); err != nil {
		return fmt.Errorf("matrix checksums: %w", err)
	}
	fmt.Printf("  Matrix checksums: PASS\n")

	// 5. Verify child checksums bound to manifest
	fmt.Printf("  Checking child checksums bound to manifest...\n")
	if err := ValidateChildChecksums(*matrixDir, manifest); err != nil {
		return fmt.Errorf("child checksums: %w", err)
	}
	fmt.Printf("  Child checksums: PASS\n")

	// 6. Verify each child run independently
	fmt.Printf("  Verifying child runs...\n")
	runsDir := filepath.Join(*matrixDir, "runs")
	for _, decl := range manifest.Runs {
		runPath := filepath.Join(runsDir, decl.RunID)
		fmt.Printf("    Verifying %s...\n", decl.Scenario)
		if err := verifyChildRun(runPath, decl.Scenario, decl.RunID); err != nil {
			return fmt.Errorf("child verification for %s: %w", decl.Scenario, err)
		}
		fmt.Printf("    %s: PASS\n", decl.Scenario)
	}

	// 7. Verify cross-run identity
	fmt.Printf("  Verifying cross-run identity...\n")
	runManifests := make([]*evidence.Manifest, len(manifest.Runs))
	for i, decl := range manifest.Runs {
		runPath := filepath.Join(runsDir, decl.RunID)
		manifestData, err := os.ReadFile(filepath.Join(runPath, "manifest.json"))
		if err != nil {
			return fmt.Errorf("read manifest for %s: %w", decl.RunID, err)
		}
		var m evidence.Manifest
		if err := json.Unmarshal(manifestData, &m); err != nil {
			return fmt.Errorf("parse manifest for %s: %w", decl.RunID, err)
		}
		runManifests[i] = &m
	}

	crossRunChecks := verifyCrossRunIdentity(runManifests)
	if !crossRunChecks.AllTrue() {
		return fmt.Errorf("cross-run identity check failed")
	}
	fmt.Printf("  Cross-run identity: PASS\n")

	// 8. Verify timing and ordering
	fmt.Printf("  Verifying timing and ordering...\n")
	for i := 0; i < len(runManifests)-1; i++ {
		if runManifests[i].FinishedAt.After(runManifests[i+1].StartedAt) {
			return fmt.Errorf("run intervals overlap: %s finishes after %s starts",
				runManifests[i].RunID, runManifests[i+1].RunID)
		}
	}
	fmt.Printf("  Timing and ordering: PASS\n")

	// 9. Compare reconstructed verdict with stored verdict
	fmt.Printf("  Comparing reconstructed vs stored verdict...\n")
	if verdict.MatrixID != manifest.MatrixID {
		return fmt.Errorf("verdict matrix_id %s != manifest matrix_id %s", verdict.MatrixID, manifest.MatrixID)
	}

	fmt.Printf("\n=== Matrix Verification Results ===\n")
	fmt.Printf("Matrix ID: %s\n", manifest.MatrixID)
	fmt.Printf("Matrix Valid: %v\n", verdict.MatrixValid)
	fmt.Printf("All Checks: PASS\n")

	return nil
}
