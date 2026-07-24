// matrix_fixture_test.go — Canonical Artifact-Backed Matrix Fixture (Test-Only)
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03-P0-CLI-FIXTURE-CONVERGENCE01
//
// P0-8: This file contains ALL fixture code. It is compiled only by `go test`.
// Production builds MUST NOT contain fixture symbols.
//
// Creates a complete valid matrix bundle from production authority.
// Uses real checksum writers and the real child verifier.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence"
)

// =============================================================================
// FIXTURE TYPES (unexported - test-only)
// =============================================================================

// matrixFixture holds the complete fixture state.
// P0-8: Unexported to keep production surface clean.
type matrixFixture struct {
	rootDir  string
	matrixID string
	manifest *MatrixManifest
	verdict  *MatrixVerdict
	cleanup  *MatrixCleanupEvidence
	runDirs  []string
}

// =============================================================================
// DETERMINISTIC FIXTURE IDENTITIES
// =============================================================================

// P0-8: Unexported fixture constants
var (
	fixtureMatrixID = "matrix-fixture-001"

	fixtureRunIDs = []string{
		"growing-run-001",
		"bounded-run-001",
		"descriptor-run-001",
	}

	fixtureScenarios = CanonicalScenarioOrder // canary-growing, canary-bounded, canary-descriptor

	// Deterministic container IDs (64-character hex strings)
	fixtureContainerIDs = []string{
		"1111111111111111111111111111111111111111111111111111111111111111",
		"2222222222222222222222222222222222222222222222222222222222222222",
		"3333333333333333333333333333333333333333333333333333333333333333",
	}

	// Deterministic network IDs
	fixtureNetworkIDs = []string{
		"aaaaaaaaaaaaaaaaramdomnetworkidaaaa111111111111111111111111111",
		"bbbbbbbbbbbbbbbbrandomnetworkidbbbb222222222222222222222222222",
		"cccccccccccccccdrandomnetworkidcccc333333333333333333333333333",
	}

	fixturePIDs       = []int{41001, 41002, 41003}
	fixtureStartTimes = []uint64{100001, 100002, 100003}

	fixtureStartedAt  = time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	fixtureFinishedAt = time.Date(2026, 7, 22, 10, 30, 0, 0, time.UTC)
	fixtureObservedAt = time.Date(2026, 7, 22, 10, 30, 1, 0, time.UTC)
)

// =============================================================================
// CANONICAL FIXTURE ARTIFACT INVENTORY
// =============================================================================

// P0-7 FIX: Canonical child artifact inventory for single source of truth.
// Used by all fixture writers and regeneration functions.
// NOTE: checksums.txt is NOT included - it's the output, not an input.
var canonicalChildArtifactInventory = []string{
	"manifest.json",
	"verdict.json",
	"samples.csv",
	"events.jsonl",
	"container-inspect.json",
	"container-logs.txt",
	"initial-canary-state.json",
	"final-canary-state.json",
	"workload-result.json",
	"network-identity.json",
}

// =============================================================================
// FIXTURE WRITING HELPERS
// =============================================================================

// writeValidMatrixBundleFixture creates a complete valid matrix directory.
// P0-8: Uses testing.TB and TempDir for automatic cleanup.
// P0-7: All operations are fail-closed with checked errors.
func writeValidMatrixBundleFixture(tb testing.TB) *matrixFixture {
	tb.Helper()

	// Use TempDir for automatic cleanup
	rootDir := tb.TempDir()

	// Create directory structure
	runsDir := filepath.Join(rootDir, "runs")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		tb.Fatalf("create runs dir: %v", err)
	}

	// Create manifest
	manifest := buildFixtureManifest()

	// Create child runs and collect artifacts
	var runDirs []string
	var manifests []*evidence.Manifest
	var verdicts []*evidence.Verdict

	for i := 0; i < 3; i++ {
		runDir := filepath.Join(runsDir, fixtureRunIDs[i])
		if err := os.MkdirAll(runDir, 0755); err != nil {
			tb.Fatalf("create run dir %s: %v", fixtureRunIDs[i], err)
		}
		runDirs = append(runDirs, runDir)

		// Write child artifacts
		childManifest := buildChildManifest(i)
		childVerdict := buildChildVerdict(fixtureScenarios[i])
		manifests = append(manifests, childManifest)
		verdicts = append(verdicts, childVerdict)

		writeChildArtifacts(tb, runDir, i, childManifest, childVerdict)
	}

	// Write matrix manifest - P0-7: checked errors
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		tb.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "matrix-manifest.json"), manifestJSON, 0644); err != nil {
		tb.Fatalf("write manifest: %v", err)
	}

	// Build and write cleanup evidence - P0-7: checked errors
	cleanup := buildFixtureCleanup()
	cleanupJSON, err := json.MarshalIndent(cleanup, "", "  ")
	if err != nil {
		tb.Fatalf("marshal cleanup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "matrix-cleanup.json"), cleanupJSON, 0644); err != nil {
		tb.Fatalf("write cleanup: %v", err)
	}

	// Reconstruct verdict using production authority
	verifiedRuns := buildVerifiedRuns(manifests, verdicts)
	reconstructedVerdict, err := ReconstructMatrixVerdict(manifest, verifiedRuns, cleanup)
	if err != nil {
		tb.Fatalf("reconstruct verdict: %v", err)
	}

	// Write matrix verdict - P0-7: checked errors
	verdictJSON, err := json.MarshalIndent(reconstructedVerdict, "", "  ")
	if err != nil {
		tb.Fatalf("marshal verdict: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "matrix-verdict.json"), verdictJSON, 0644); err != nil {
		tb.Fatalf("write verdict: %v", err)
	}

	// Compute and write matrix checksums - P0-7: checked errors
	checksumContent, err := computeMatrixChecksumsContent(rootDir)
	if err != nil {
		tb.Fatalf("compute matrix checksums: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "matrix-checksums.txt"), []byte(checksumContent), 0644); err != nil {
		tb.Fatalf("write checksums: %v", err)
	}

	return &matrixFixture{
		rootDir:  rootDir,
		matrixID: fixtureMatrixID,
		manifest: manifest,
		verdict:  reconstructedVerdict,
		cleanup:  cleanup,
		runDirs:  runDirs,
	}
}

// buildFixtureManifest creates the matrix manifest.
func buildFixtureManifest() *MatrixManifest {
	execIdentity := &MatrixExecutionIdentity{
		ImplementationCommitOID:    "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		ImplementationTreeOID:      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		GitObjectFormat:            "sha256",
		ControllerPID:              12345,
		ControllerExecutableSHA256: "cafebabe",
		RunManifestSchemaVersion:   "1.1.0",
		ImageReference:             "test-image:latest",
		ImageID:                    "sha256:abcdef123456",
		CanaryBinarySHA256:         "binaryhash",
		HostKernelRelease:          "6.1.0",
		HostKernelVersion:          "Linux 6.1.0",
		HostCgroupMode:             "2",
		DockerEngineVersion:        "24.0.0",
		DockerAPIVersion:           "1.43",
		Thresholds:                 &analysis.Thresholds{},
	}

	runs := make([]MatrixRunDeclaration, 3)
	for i := 0; i < 3; i++ {
		runs[i] = MatrixRunDeclaration{
			Index:           i + 1,
			Scenario:        fixtureScenarios[i],
			RunID:           fixtureRunIDs[i],
			Path:            filepath.Join("runs", fixtureRunIDs[i]),
			ChecksumsSHA256: "", // Will be computed after writing child artifacts
		}
	}

	return &MatrixManifest{
		SchemaVersion:     MatrixSchemaVersion,
		MatrixID:          fixtureMatrixID,
		StartedAt:         fixtureStartedAt,
		FinishedAt:        fixtureFinishedAt,
		ExecutionIdentity: execIdentity,
		Runs:              runs,
	}
}

// buildChildManifest creates a child run manifest.
func buildChildManifest(index int) *evidence.Manifest {
	startedAt := fixtureStartedAt.Add(time.Duration(index*6) * time.Minute)
	finishedAt := startedAt.Add(5 * time.Minute)

	return &evidence.Manifest{
		SchemaVersion: "1.1.0",
		RunID:         fixtureRunIDs[index],
		Scenario:      fixtureScenarios[index],
		ControllerID:  "12345",
		SubjectIdentity: &evidence.SubjectIdentity{
			GitCommit:                  "deadbeef",
			GitTree:                    "0123456789",
			GitObjectFormat:            "sha256",
			ControllerExecutableSHA256: "cafebabe",
		},
		SubjectImageIdentity: &evidence.SubjectImageIdentity{
			ImageReference:       "test-image:latest",
			ImageID:              "sha256:abcdef123456",
			PrebuildBinarySHA256: "binaryhash",
		},
		HostID: &evidence.HostIdentity{
			KernelRelease: "6.1.0",
			CgroupMode:    "2",
		},
		DockerID: &evidence.DockerIdentity{
			EngineVersion: "24.0.0",
		},
		Configuration: &evidence.LabConfiguration{},
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
	}
}

// buildChildVerdict creates a child verdict with correct classification.
func buildChildVerdict(scenario string) *evidence.Verdict {
	switch scenario {
	case "canary-growing":
		return &evidence.Verdict{
			Scenario:               scenario,
			OverallClassification:  analysis.ClassificationGrowing,
			MemoryClassification:   analysis.ClassificationGrowing,
			ResourceClassification: analysis.ClassificationStable,
			SemanticClassification: analysis.ClassificationStable,
			ScenarioValid:          true,
			CanariesValid:          true,
			ProvenanceValid:        true,
		}
	case "canary-bounded":
		return &evidence.Verdict{
			Scenario:               scenario,
			OverallClassification:  analysis.ClassificationStable,
			MemoryClassification:   analysis.ClassificationStable,
			ResourceClassification: analysis.ClassificationStable,
			SemanticClassification: analysis.ClassificationStable,
			ScenarioValid:          true,
			CanariesValid:          true,
			ProvenanceValid:        true,
		}
	case "canary-descriptor":
		return &evidence.Verdict{
			Scenario:               scenario,
			OverallClassification:  analysis.ClassificationResourceGrowth,
			MemoryClassification:   analysis.ClassificationStable,
			ResourceClassification: analysis.ClassificationResourceGrowth,
			SemanticClassification: analysis.ClassificationStable,
			ScenarioValid:          true,
			CanariesValid:          true,
			ProvenanceValid:        true,
		}
	default:
		return &evidence.Verdict{
			Scenario:        scenario,
			ScenarioValid:   true,
			CanariesValid:   true,
			ProvenanceValid: true,
		}
	}
}

// writeChildArtifacts writes all required child artifacts.
// P0-7: All operations are fail-closed with checked errors.
func writeChildArtifacts(tb testing.TB, runDir string, index int, manifest *evidence.Manifest, verdict *evidence.Verdict) {
	tb.Helper()

	// manifest.json - P0-7: checked errors
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		tb.Fatalf("marshal child manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "manifest.json"), manifestJSON, 0644); err != nil {
		tb.Fatalf("write child manifest: %v", err)
	}

	// verdict.json - P0-7: checked errors
	verdictJSON, err := json.MarshalIndent(verdict, "", "  ")
	if err != nil {
		tb.Fatalf("marshal child verdict: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "verdict.json"), verdictJSON, 0644); err != nil {
		tb.Fatalf("write child verdict: %v", err)
	}

	// container-inspect.json - P0-7: checked errors
	containerInspect := map[string]string{"Id": fixtureContainerIDs[index]}
	containerJSON, err := json.MarshalIndent(containerInspect, "", "  ")
	if err != nil {
		tb.Fatalf("marshal container inspect: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "container-inspect.json"), containerJSON, 0644); err != nil {
		tb.Fatalf("write container inspect: %v", err)
	}

	// network-identity.json - P0-7: checked errors
	networkJSONData, err := json.MarshalIndent(struct {
		SchemaVersion string `json:"schema_version"`
		ID            string `json:"id"`
		Name          string `json:"name"`
	}{
		SchemaVersion: "1.0.0",
		ID:            fixtureNetworkIDs[index],
		Name:          "test-network-" + fixtureRunIDs[index],
	}, "", "  ")
	if err != nil {
		tb.Fatalf("marshal network identity: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "network-identity.json"), networkJSONData, 0644); err != nil {
		tb.Fatalf("write network identity: %v", err)
	}

	// samples.csv - P0-7: checked errors
	samples := fmt.Sprintf("process_pid,process_start_time\n%d,%d\n%d,%d\n",
		fixturePIDs[index], fixtureStartTimes[index],
		fixturePIDs[index], fixtureStartTimes[index])
	if err := os.WriteFile(filepath.Join(runDir, "samples.csv"), []byte(samples), 0644); err != nil {
		tb.Fatalf("write samples: %v", err)
	}

	// events.jsonl - P0-7: checked errors
	if err := os.WriteFile(filepath.Join(runDir, "events.jsonl"), []byte("{}\n{}\n"), 0644); err != nil {
		tb.Fatalf("write events: %v", err)
	}

	// container-logs.txt - P0-7: checked errors
	if err := os.WriteFile(filepath.Join(runDir, "container-logs.txt"), []byte("container logs\n"), 0644); err != nil {
		tb.Fatalf("write container logs: %v", err)
	}

	// initial-canary-state.json - P0-7: checked errors
	initialState := buildCanaryState(fixtureScenarios[index], 0)
	initialJSON, err := json.MarshalIndent(initialState, "", "  ")
	if err != nil {
		tb.Fatalf("marshal initial canary state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "initial-canary-state.json"), initialJSON, 0644); err != nil {
		tb.Fatalf("write initial canary state: %v", err)
	}

	// final-canary-state.json - P0-7: checked errors
	finalState := buildCanaryState(fixtureScenarios[index], 1)
	finalJSON, err := json.MarshalIndent(finalState, "", "  ")
	if err != nil {
		tb.Fatalf("marshal final canary state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "final-canary-state.json"), finalJSON, 0644); err != nil {
		tb.Fatalf("write final canary state: %v", err)
	}

	// workload-result.json - P0-7: checked errors
	workload := buildWorkloadResult(fixtureScenarios[index])
	workloadJSON, err := json.MarshalIndent(workload, "", "  ")
	if err != nil {
		tb.Fatalf("marshal workload result: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "workload-result.json"), workloadJSON, 0644); err != nil {
		tb.Fatalf("write workload result: %v", err)
	}

	// Compute and write child checksums - P0-7: checked errors
	checksumContent, err := computeChildChecksumsContent(runDir)
	if err != nil {
		tb.Fatalf("compute child checksums: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "checksums.txt"), []byte(checksumContent), 0644); err != nil {
		tb.Fatalf("write child checksums: %v", err)
	}
}

// buildCanaryState creates a canary state.
func buildCanaryState(scenario string, phase int) CanaryState {
	state := CanaryState{
		Mode:           scenario,
		BufferCapacity: 32 * 1024 * 1024,
		RetainedBlocks: 0,
		RetainedBytes:  0,
		FDCount:        10,
	}

	switch scenario {
	case "canary-growing":
		if phase == 1 {
			state.RetainedBlocks = 32
			state.RetainedBytes = 32 * 1024 * 1024
		}
	case "canary-bounded":
		// Bounded: no retention
	case "canary-descriptor":
		if phase == 1 {
			state.FDCount = 210
		}
	}

	return state
}

// buildWorkloadResult creates a workload result.
func buildWorkloadResult(scenario string) WorkloadResult {
	result := WorkloadResult{
		Requested: 100,
		Attempted: 100,
		Completed: 100,
		Failed:    0,
		Returned:  100,
	}

	switch scenario {
	case "canary-growing":
		result.Requested = 32
		result.Attempted = 32
		result.Completed = 32
		result.Returned = 32
	}

	return result
}

// buildFixtureCleanup creates the cleanup evidence.
func buildFixtureCleanup() *MatrixCleanupEvidence {
	records := make([]RunCleanupRecord, 3)
	for i := 0; i < 3; i++ {
		records[i] = RunCleanupRecord{
			Index:    i,
			Scenario: fixtureScenarios[i],
			RunID:    fixtureRunIDs[i],
			Container: ContainerCleanupRecord{
				ID:     fixtureContainerIDs[i],
				Status: "gone",
			},
			Network: NetworkCleanupRecord{
				ID:     fixtureNetworkIDs[i],
				Status: "gone",
			},
			Process: ProcessCleanupRecord{
				PID:       fixturePIDs[i],
				StartTime: fixtureStartTimes[i],
				Status:    "gone",
			},
		}
	}

	return &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:         fixtureMatrixID,
		ObservedAt:       fixtureObservedAt,
		NetworkOwnership: "per_run",
		Runs:             records,
	}
}

// buildVerifiedRuns creates VerifiedRun slice from manifests and verdicts.
func buildVerifiedRuns(manifests []*evidence.Manifest, verdicts []*evidence.Verdict) []*VerifiedRun {
	runs := make([]*VerifiedRun, 3)
	for i := 0; i < 3; i++ {
		runs[i] = &VerifiedRun{
			DeclaredRunID:         fixtureRunIDs[i],
			DeclaredScenario:      fixtureScenarios[i],
			RunIndex:              i,
			ActualManifest:        manifests[i],
			ActualVerdict:         verdicts[i],
			ContainerID:           fixtureContainerIDs[i],
			NetworkID:             fixtureNetworkIDs[i],
			SubjectPID:            fixturePIDs[i],
			SubjectStartTime:      fixtureStartTimes[i],
			ProcessCleanupStatus:  ProcessGone,
			ChildVerified:         true,
			CleanupEvidenceLoaded: true,
			CleanupEvidenceValid:  true,
		}
	}
	return runs
}

// buildVerifiedRunsFromFixture loads child manifests and verdicts from fixture and builds VerifiedRun slice.
func buildVerifiedRunsFromFixture(fixture *matrixFixture) []*VerifiedRun {
	runs := make([]*VerifiedRun, 3)
	for i := 0; i < 3; i++ {
		runDir := fixture.runDirs[i]

		// Load child manifest
		manifestData, err := os.ReadFile(filepath.Join(runDir, "manifest.json"))
		if err != nil {
			panic(fmt.Sprintf("read child manifest %d: %v", i, err))
		}
		var childManifest evidence.Manifest
		if err := json.Unmarshal(manifestData, &childManifest); err != nil {
			panic(fmt.Sprintf("unmarshal child manifest %d: %v", i, err))
		}

		// Load child verdict
		verdictData, err := os.ReadFile(filepath.Join(runDir, "verdict.json"))
		if err != nil {
			panic(fmt.Sprintf("read child verdict %d: %v", i, err))
		}
		var childVerdict evidence.Verdict
		if err := json.Unmarshal(verdictData, &childVerdict); err != nil {
			panic(fmt.Sprintf("unmarshal child verdict %d: %v", i, err))
		}

		runs[i] = &VerifiedRun{
			DeclaredRunID:         fixtureRunIDs[i],
			DeclaredScenario:      fixtureScenarios[i],
			RunIndex:              i,
			ActualManifest:        &childManifest,
			ActualVerdict:         &childVerdict,
			ContainerID:           fixtureContainerIDs[i],
			NetworkID:             fixtureNetworkIDs[i],
			SubjectPID:            fixturePIDs[i],
			SubjectStartTime:      fixtureStartTimes[i],
			ProcessCleanupStatus:  ProcessGone,
			ChildVerified:         true,
			CleanupEvidenceLoaded: true,
			CleanupEvidenceValid:  true,
		}
	}
	return runs
}

// =============================================================================
// CHECKSUM COMPUTATION (fail-closed)
// =============================================================================

// computeChildChecksumsContent computes checksums for child artifacts.
// P0-7: Returns error instead of panicking on missing artifacts.
func computeChildChecksumsContent(runDir string) (string, error) {
	return computeChecksumsContent(runDir, canonicalChildArtifactInventory)
}

// computeMatrixChecksumsContent computes checksums for matrix artifacts.
// P0-7: Returns error instead of panicking on missing artifacts.
func computeMatrixChecksumsContent(matrixDir string) (string, error) {
	return computeChecksumsContent(matrixDir, canonicalMatrixArtifactInventory[:])
}

// computeChildChecksumsContentWithOps computes checksums for child artifacts using injected ops.
// P0-7: All file reads go through ops.readFile for testability.
func computeChildChecksumsContentWithOps(runDir string, artifacts []string, ops fixtureFileOps) (string, error) {
	return computeChecksumsContentWithOps(runDir, artifacts, ops)
}

// computeMatrixChecksumsContentWithOps computes checksums for matrix artifacts using injected ops.
// P0-7: All file reads go through ops.readFile for testability.
func computeMatrixChecksumsContentWithOps(matrixDir string, artifacts []string, ops fixtureFileOps) (string, error) {
	return computeChecksumsContentWithOps(matrixDir, artifacts, ops)
}

// computeChecksumsContent computes SHA256 checksums for listed files.
// P0-7: Returns error instead of panicking on missing artifacts.
func computeChecksumsContent(dir string, artifacts []string) (string, error) {
	return computeChecksumsContentWithOps(dir, artifacts, defaultFixtureFileOps)
}

// computeChecksumsContentWithOps computes SHA256 checksums using injected file operations.
// P0-7: All file reads go through ops.readFile for testability.
func computeChecksumsContentWithOps(dir string, artifacts []string, ops fixtureFileOps) (string, error) {
	var content string
	for _, name := range artifacts {
		path := filepath.Join(dir, name)
		data, err := ops.readFile(path)
		if err != nil {
			return "", fmt.Errorf("read artifact %q: %w", name, err)
		}
		hash := sha256.Sum256(data)
		content += fmt.Sprintf("%s  %s\n", hex.EncodeToString(hash[:]), name)
	}
	return content, nil
}

// =============================================================================
// FIXTURE FILE OPERATIONS (fail-closed)
// =============================================================================

// P0-7: Separate low-level fixture operations from test convenience wrappers.

// writeFixtureJSON marshals a value and writes it to a file.
// Returns error for testability.
func writeFixtureJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal fixture JSON: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write fixture JSON: %w", err)
	}
	return nil
}

// readFixtureJSON reads a file and unmarshals it into a value.
// Returns error for testability.
func readFixtureJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read fixture JSON: %w", err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("unmarshal fixture JSON: %w", err)
	}
	return nil
}

// =============================================================================
// CHECKSUM REGENERATION (fail-closed)
// =============================================================================

// P0-7: Fail-closed regeneration with proper error propagation.

// regenerateChildChecksums regenerates checksums for a child run.
// Returns the new checksum content and error.
func regenerateChildChecksums(runDir string) (string, error) {
	content, err := computeChildChecksumsContent(runDir)
	if err != nil {
		return "", fmt.Errorf("compute child checksums: %w", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "checksums.txt"), []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write child checksums: %w", err)
	}
	return content, nil
}

// updateDeclaredChildChecksum updates the manifest's checksums_sha256 for a run.
// P0-7: Returns error for testability.
func updateDeclaredChildChecksum(manifest *MatrixManifest, runIndex int, childChecksumDigest string) error {
	if runIndex < 0 || runIndex >= len(manifest.Runs) {
		return fmt.Errorf("invalid run index %d", runIndex)
	}
	manifest.Runs[runIndex].ChecksumsSHA256 = childChecksumDigest
	return nil
}

// regenerateMatrixChecksums regenerates matrix-level checksums.
// P0-7: Returns error for testability.
func regenerateMatrixChecksums(matrixDir string) error {
	content, err := computeMatrixChecksumsContent(matrixDir)
	if err != nil {
		return fmt.Errorf("compute matrix checksums: %w", err)
	}
	if err := os.WriteFile(filepath.Join(matrixDir, "matrix-checksums.txt"), []byte(content), 0644); err != nil {
		return fmt.Errorf("write matrix checksums: %w", err)
	}
	return nil
}

// regenerateAllChecksums regenerates both child and matrix checksums.
// P0-7 FIX: Delegates to regenerateAllChecksumsWithOps using defaultFixtureFileOps.
// This ensures all operations go through the seam for testability.
func regenerateAllChecksums(fixture *matrixFixture) error {
	return regenerateAllChecksumsWithOps(fixture, defaultFixtureFileOps)
}

// mustRegenerateAllChecksums is a convenience wrapper that fails on error.
// P0-7: Convenience wrappers may call tb.Fatal, but the underlying operation returns an error.
func mustRegenerateAllChecksums(tb testing.TB, fixture *matrixFixture) {
	tb.Helper()
	if err := regenerateAllChecksums(fixture); err != nil {
		tb.Fatalf("regenerate all checksums: %v", err)
	}
}

// =============================================================================
// FIXTURE MUTATION HELPERS
// =============================================================================

// P0-7: Typed mutation helpers with explicit error returns.

// childArtifactMutation describes a mutation to apply to a child artifact.
type childArtifactMutation struct {
	runIndex int
	filename string
	mutate   func([]byte) ([]byte, error)
}

// applyChildSemanticMutation applies a mutation to a child artifact and regenerates checksums.
// P0-7: Returns error for testability.
func applyChildSemanticMutation(fixture *matrixFixture, mutation childArtifactMutation) error {
	if mutation.runIndex < 0 || mutation.runIndex >= len(fixture.runDirs) {
		return fmt.Errorf("invalid run index %d", mutation.runIndex)
	}

	runDir := fixture.runDirs[mutation.runIndex]
	path := filepath.Join(runDir, mutation.filename)

	// Read current content
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read artifact %s: %w", mutation.filename, err)
	}

	// Apply mutation
	mutated, err := mutation.mutate(data)
	if err != nil {
		return fmt.Errorf("mutate artifact %s: %w", mutation.filename, err)
	}

	// Write mutated content
	if err := os.WriteFile(path, mutated, 0644); err != nil {
		return fmt.Errorf("write mutated artifact %s: %w", mutation.filename, err)
	}

	// Regenerate checksums for this child run
	childChecksumContent, err := regenerateChildChecksums(runDir)
	if err != nil {
		return fmt.Errorf("regenerate checksums after mutation: %w", err)
	}

	// Update manifest's checksums_sha256 for this run
	checksumHash := sha256.Sum256([]byte(childChecksumContent))
	childDigest := hex.EncodeToString(checksumHash[:])
	if err := updateDeclaredChildChecksum(fixture.manifest, mutation.runIndex, childDigest); err != nil {
		return fmt.Errorf("update child checksum digest: %w", err)
	}

	return nil
}

// matrixArtifactMutation describes a mutation to apply to a matrix artifact.
type matrixArtifactMutation struct {
	filename string
	mutate   func([]byte) ([]byte, error)
}

// applyMatrixSemanticMutation applies a mutation to a matrix artifact and regenerates checksums.
// P0-7: Returns error for testability.
func applyMatrixSemanticMutation(fixture *matrixFixture, mutation matrixArtifactMutation) error {
	path := filepath.Join(fixture.rootDir, mutation.filename)

	// Read current content
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read artifact %s: %w", mutation.filename, err)
	}

	// Apply mutation
	mutated, err := mutation.mutate(data)
	if err != nil {
		return fmt.Errorf("mutate artifact %s: %w", mutation.filename, err)
	}

	// Write mutated content
	if err := os.WriteFile(path, mutated, 0644); err != nil {
		return fmt.Errorf("write mutated artifact %s: %w", mutation.filename, err)
	}

	// Regenerate matrix checksums
	if err := regenerateMatrixChecksums(fixture.rootDir); err != nil {
		return fmt.Errorf("regenerate matrix checksums: %w", err)
	}

	return nil
}

// mustApplyChildSemanticMutation is a convenience wrapper that fails on error.
func mustApplyChildSemanticMutation(tb testing.TB, fixture *matrixFixture, mutation childArtifactMutation) {
	tb.Helper()
	if err := applyChildSemanticMutation(fixture, mutation); err != nil {
		tb.Fatalf("apply child semantic mutation: %v", err)
	}
}

// mustApplyMatrixSemanticMutation is a convenience wrapper that fails on error.
func mustApplyMatrixSemanticMutation(tb testing.TB, fixture *matrixFixture, mutation matrixArtifactMutation) {
	tb.Helper()
	if err := applyMatrixSemanticMutation(fixture, mutation); err != nil {
		tb.Fatalf("apply matrix semantic mutation: %v", err)
	}
}

// =============================================================================
// EQUAL-INVALID FIXTURE CONSTRUCTION
// =============================================================================

// P0-6: Construct a genuine equal-invalid fixture.

// writeEqualInvalidMatrixBundleFixture creates a complete equal-invalid matrix bundle.
// The stored and reconstructed verdicts both have MatrixValid=false with zero differences.
func writeEqualInvalidMatrixBundleFixture(tb testing.TB) *matrixFixture {
	tb.Helper()

	// Start from a valid fixture
	fixture := writeValidMatrixBundleFixture(tb)

	// Mutate child verdict to cause classification mismatch
	// Use descriptor scenario - mutate its classification to be "growing" instead of "resource_growth"
	// This will cause a scenario classification mismatch during reconstruction
	descriptorIndex := 2 // canary-descriptor is index 2

	mutation := childArtifactMutation{
		runIndex: descriptorIndex,
		filename: "verdict.json",
		mutate: func(data []byte) ([]byte, error) {
			var verdict evidence.Verdict
			if err := json.Unmarshal(data, &verdict); err != nil {
				return nil, err
			}
			// Change classification to cause mismatch with expected descriptor classification
			// Expected: ClassificationResourceGrowth
			// Mutated: ClassificationGrowing (conflicts with descriptor's semantic expectations)
			verdict.OverallClassification = analysis.ClassificationGrowing
			return json.MarshalIndent(verdict, "", "  ")
		},
	}
	mustApplyChildSemanticMutation(tb, fixture, mutation)

	// P0-6 FIX: Rewrite manifest to disk with updated child checksums
	// This must happen before reconstruction so the manifest has the new checksums
	mustRegenerateAllChecksums(tb, fixture)

	// P0-6 FIX: Use VerifyDeclaredChildRuns for authoritative verification.
	// This replaces manual VerifiedRun construction with the single authoritative path.
	verifiedRuns, err := VerifyDeclaredChildRuns(fixture.rootDir, fixture.manifest, fixture.cleanup, verifyChildRunBundle)
	if err != nil {
		tb.Fatalf("authoritative child verification failed: %v", err)
	}

	// Reconstruct verdict - should be invalid due to classification mismatch
	reconstructedVerdict, err := ReconstructMatrixVerdict(fixture.manifest, verifiedRuns, fixture.cleanup)
	if err != nil {
		tb.Fatalf("reconstruct verdict: %v", err)
	}

	// Verify it's invalid (this is the expected outcome)
	if reconstructedVerdict.MatrixValid {
		tb.Fatal("reconstructed verdict should be invalid")
	}

	// P0-6 FIX: Update fixture.verdict BEFORE regeneration so the invalid verdict is written to disk
	fixture.verdict = reconstructedVerdict
	// NOTE: fixture.manifest and fixture.cleanup already point to the authoritative objects
	// used by VerifyDeclaredChildRuns, so no need to reassign them

	// P0-6 FIX: Write the exact reconstructed invalid verdict to disk
	verdictJSON, err := json.MarshalIndent(reconstructedVerdict, "", "  ")
	if err != nil {
		tb.Fatalf("marshal reconstructed verdict: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixture.rootDir, "matrix-verdict.json"), verdictJSON, 0644); err != nil {
		tb.Fatalf("write reconstructed verdict: %v", err)
	}

	// Regenerate all checksums - now fixture.verdict points to the invalid verdict
	mustRegenerateAllChecksums(tb, fixture)

	// Verify stored equals reconstructed (both from disk)
	freshVerdictData, err := os.ReadFile(filepath.Join(fixture.rootDir, "matrix-verdict.json"))
	if err != nil {
		tb.Fatalf("read verdict for equality check: %v", err)
	}
	var storedVerdict MatrixVerdict
	if err := json.Unmarshal(freshVerdictData, &storedVerdict); err != nil {
		tb.Fatalf("unmarshal stored verdict: %v", err)
	}

	// Both should be invalid
	if storedVerdict.MatrixValid {
		tb.Fatal("stored verdict should be invalid")
	}

	diffs := CompareVerdicts(&storedVerdict, reconstructedVerdict)
	if len(diffs) > 0 {
		tb.Fatalf("stored and reconstructed verdicts should have zero differences, got:\n%s", FormatVerdictDiffs(diffs))
	}

	// P0-6 FIX: Use authoritative VerifyChildRun dependency and require non-nil result
	result, err := VerifyMatrixBundle(fixture.rootDir, MatrixVerificationDeps{
		VerifyChildRun: verifyChildRunBundle,
	})
	if err == nil {
		tb.Fatal("VerifyMatrixBundle should reject equal-invalid fixture")
	}

	// P0-6 FIX: Result must be non-nil (we must reach the terminal rule)
	if result == nil {
		tb.Fatal("result must be non-nil when reaching equal-invalid terminal rule")
	}

	// P0-6 FIX: Assert both stored and reconstructed are invalid
	if result.StoredVerdict == nil {
		tb.Fatal("stored verdict must be present")
	}
	if result.ReconstructedVerdict == nil {
		tb.Fatal("reconstructed verdict must be present")
	}
	if result.StoredVerdict.MatrixValid {
		tb.Fatal("stored verdict should be invalid")
	}
	if result.ReconstructedVerdict.MatrixValid {
		tb.Fatal("reconstructed verdict should be invalid")
	}

	return fixture
}

// =============================================================================
// MUTATION CONVENIENCE HELPERS
// =============================================================================

// mutateChildVerdictClassification changes a child verdict's classification.
// P0-6: Uses semantic mutation API with proper checksum regeneration.
func mutateChildVerdictClassification(tb testing.TB, fixture *matrixFixture, runIndex int, newClassification analysis.Classification) {
	tb.Helper()

	mutation := childArtifactMutation{
		runIndex: runIndex,
		filename: "verdict.json",
		mutate: func(data []byte) ([]byte, error) {
			var verdict evidence.Verdict
			if err := json.Unmarshal(data, &verdict); err != nil {
				return nil, err
			}
			verdict.OverallClassification = newClassification
			verdict.MemoryClassification = newClassification
			return json.MarshalIndent(verdict, "", "  ")
		},
	}
	mustApplyChildSemanticMutation(tb, fixture, mutation)
}

// corruptChildArtifactWithoutRefreshingChecksums corrupts an artifact WITHOUT checksum refresh.
// P0-7: Clearly distinct from semantic mutation to avoid accidental misuse.
func corruptChildArtifactWithoutRefreshingChecksums(tb testing.TB, fixture *matrixFixture, runIndex int, filename string, corruption []byte) {
	tb.Helper()

	if runIndex < 0 || runIndex >= len(fixture.runDirs) {
		tb.Fatalf("invalid run index %d", runIndex)
	}

	runDir := fixture.runDirs[runIndex]
	path := filepath.Join(runDir, filename)

	if err := os.WriteFile(path, corruption, 0644); err != nil {
		tb.Fatalf("write corrupted artifact: %v", err)
	}
	// NOTE: Checksums are NOT refreshed - this is intentional for checksum corruption tests
}

// corruptMatrixArtifactWithoutRefreshingChecksums corrupts a matrix artifact WITHOUT checksum refresh.
// P0-7: Clearly distinct from semantic mutation to avoid accidental misuse.
func corruptMatrixArtifactWithoutRefreshingChecksums(tb testing.TB, fixture *matrixFixture, filename string, corruption []byte) {
	tb.Helper()

	path := filepath.Join(fixture.rootDir, filename)

	if err := os.WriteFile(path, corruption, 0644); err != nil {
		tb.Fatalf("write corrupted artifact: %v", err)
	}
	// NOTE: Checksums are NOT refreshed - this is intentional for checksum corruption tests
}

// =============================================================================
// TEST-ONLY FILE OPERATIONS SEAM
// =============================================================================

// P0-7: Test-only operation bundle for deterministic error injection.

// fixtureFileOps encapsulates file operations for testability.
type fixtureFileOps struct {
	readFile  func(string) ([]byte, error)
	writeFile func(string, []byte, os.FileMode) error
	marshal   func(any, string, string) ([]byte, error)
}

// defaultFixtureFileOps provides standard file operations.
var defaultFixtureFileOps = fixtureFileOps{
	readFile:  os.ReadFile,
	writeFile: os.WriteFile,
	marshal:   json.MarshalIndent,
}

// =============================================================================
// EXPORTED TEST HELPERS (for same-package tests)
// =============================================================================

// FixtureRunIDs returns the deterministic run IDs for tests.
// P0-8: Exported for same-package test access.
func FixtureRunIDs() []string {
	return fixtureRunIDs
}

// FixtureContainerIDs returns the deterministic container IDs for tests.
func FixtureContainerIDs() []string {
	return fixtureContainerIDs
}

// FixtureNetworkIDs returns the deterministic network IDs for tests.
func FixtureNetworkIDs() []string {
	return fixtureNetworkIDs
}

// FixturePIDs returns the deterministic PIDs for tests.
func FixturePIDs() []int {
	return fixturePIDs
}

// FixtureStartTimes returns the deterministic start times for tests.
func FixtureStartTimes() []uint64 {
	return fixtureStartTimes
}

// getModuleRoot returns the package directory for building the CLI.
// P0-5 FIX: Uses os.Getwd to find the current package directory dynamically.
func getModuleRoot() string {
	// Tests are run from the package directory: /home/kgb/Projects/KGB/tovarisch/labs/memory/cmd/tovarisch-memory-lab
	// We need to pass "." as the Dir to go build
	wd, err := os.Getwd()
	if err != nil {
		// Fallback: use "." for current directory
		return "."
	}
	return wd
}

// createEqualInvalidFixture creates a fixture with both stored and reconstructed
// verdicts having MatrixValid=false. This is forbidden by policy.
func createEqualInvalidFixture(tb testing.TB) *matrixFixture {
	tb.Helper()
	return writeEqualInvalidMatrixBundleFixture(tb)
}
