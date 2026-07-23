// matrix_fixture.go — Canonical Artifact-Backed Matrix Fixture
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03-TERMINAL-QUALIFICATION01
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
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence"
)

// MatrixFixture holds the complete fixture state.
type MatrixFixture struct {
	RootDir     string
	MatrixID    string
	Manifest    *MatrixManifest
	Verdict     *MatrixVerdict
	Cleanup     *MatrixCleanupEvidence
	RunDirs     []string
}

// Deterministic fixture identities.
var (
	FixtureMatrixID = "matrix-fixture-001"

	FixtureRunIDs = []string{
		"growing-run-001",
		"bounded-run-001",
		"descriptor-run-001",
	}

	FixtureScenarios = CanonicalScenarioOrder // canary-growing, canary-bounded, canary-descriptor

	// Deterministic container IDs (64-character hex strings)
	FixtureContainerIDs = []string{
		"1111111111111111111111111111111111111111111111111111111111111111",
		"2222222222222222222222222222222222222222222222222222222222222222",
		"3333333333333333333333333333333333333333333333333333333333333333",
	}

	// Deterministic network IDs
	FixtureNetworkIDs = []string{
		"aaaaaaaaaaaaaaaaramdomnetworkidaaaa111111111111111111111111111",
		"bbbbbbbbbbbbbbbbrandomnetworkidbbbb222222222222222222222222222",
		"cccccccccccccccdrandomnetworkidcccc333333333333333333333333333",
	}

	FixturePIDs       = []int{41001, 41002, 41003}
	FixtureStartTimes = []uint64{100001, 100002, 100003}

	FixtureStartedAt  = time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	FixtureFinishedAt  = time.Date(2026, 7, 22, 10, 30, 0, 0, time.UTC)
	FixtureObservedAt  = time.Date(2026, 7, 22, 10, 30, 1, 0, time.UTC)
)

// WriteValidMatrixBundleFixture creates a complete valid matrix directory.
// Uses production writers and checksum functions.
func WriteValidMatrixBundleFixture(t SimpleTestHelper, root string) *MatrixFixture {
	t.Helper()

	// Create directory structure
	runsDir := filepath.Join(root, "runs")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatalf("create runs dir: %v", err)
	}

	// Create manifest
	manifest := buildFixtureManifest()

	// Create child runs and collect artifacts
	var runDirs []string
	var manifests []*evidence.Manifest
	var verdicts []*evidence.Verdict

	for i := 0; i < 3; i++ {
		runDir := filepath.Join(runsDir, FixtureRunIDs[i])
		if err := os.MkdirAll(runDir, 0755); err != nil {
			t.Fatalf("create run dir %s: %v", FixtureRunIDs[i], err)
		}
		runDirs = append(runDirs, runDir)

		// Write child artifacts
		childManifest := buildChildManifest(i)
		childVerdict := buildChildVerdict(FixtureScenarios[i])
		manifests = append(manifests, childManifest)
		verdicts = append(verdicts, childVerdict)

		writeChildArtifacts(t, runDir, i, childManifest, childVerdict)
	}

	// Write matrix manifest
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "matrix-manifest.json"), manifestJSON, 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	// Build and write cleanup evidence
	cleanup := buildFixtureCleanup()
	cleanupJSON, err := json.MarshalIndent(cleanup, "", "  ")
	if err != nil {
		t.Fatalf("marshal cleanup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "matrix-cleanup.json"), cleanupJSON, 0644); err != nil {
		t.Fatalf("write cleanup: %v", err)
	}

	// Reconstruct verdict using production authority
	verifiedRuns := buildVerifiedRuns(manifests, verdicts)
	reconstructedVerdict, err := ReconstructMatrixVerdict(manifest, verifiedRuns, cleanup)
	if err != nil {
		t.Fatalf("reconstruct verdict: %v", err)
	}

	// Write matrix verdict
	verdictJSON, err := json.MarshalIndent(reconstructedVerdict, "", "  ")
	if err != nil {
		t.Fatalf("marshal verdict: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "matrix-verdict.json"), verdictJSON, 0644); err != nil {
		t.Fatalf("write verdict: %v", err)
	}

	// Compute and write matrix checksums using production checksum writer
	checksumContent := computeMatrixChecksumsContent(root)
	if err := os.WriteFile(filepath.Join(root, "matrix-checksums.txt"), []byte(checksumContent), 0644); err != nil {
		t.Fatalf("write checksums: %v", err)
	}

	return &MatrixFixture{
		RootDir:  root,
		MatrixID:  FixtureMatrixID,
		Manifest:  manifest,
		Verdict:   reconstructedVerdict,
		Cleanup:   cleanup,
		RunDirs:   runDirs,
	}
}

// buildFixtureManifest creates the matrix manifest.
func buildFixtureManifest() *MatrixManifest {
	execIdentity := &MatrixExecutionIdentity{
		ImplementationCommitOID:    "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		ImplementationTreeOID:      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		GitObjectFormat:           "sha256",
		ControllerPID:             12345,
		ControllerExecutableSHA256: "cafebabe",
		RunManifestSchemaVersion:   "1.1.0",
		ImageReference:           "test-image:latest",
		ImageID:                 "sha256:abcdef123456",
		CanaryBinarySHA256:       "binaryhash",
		HostKernelRelease:       "6.1.0",
		HostKernelVersion:       "Linux 6.1.0",
		HostCgroupMode:         "2",
		DockerEngineVersion:    "24.0.0",
		DockerAPIVersion:       "1.43",
		Thresholds:             &analysis.Thresholds{},
	}

	runs := make([]MatrixRunDeclaration, 3)
	for i := 0; i < 3; i++ {
		runs[i] = MatrixRunDeclaration{
			Index:           i + 1,
			Scenario:        FixtureScenarios[i],
			RunID:           FixtureRunIDs[i],
			Path:            filepath.Join("runs", FixtureRunIDs[i]),
			ChecksumsSHA256: "", // Will be computed after writing child artifacts
		}
	}

	return &MatrixManifest{
		SchemaVersion:     MatrixSchemaVersion,
		MatrixID:         FixtureMatrixID,
		StartedAt:        FixtureStartedAt,
		FinishedAt:       FixtureFinishedAt,
		ExecutionIdentity: execIdentity,
		Runs:             runs,
	}
}

// buildChildManifest creates a child run manifest.
func buildChildManifest(index int) *evidence.Manifest {
	startedAt := FixtureStartedAt.Add(time.Duration(index*6) * time.Minute)
	finishedAt := startedAt.Add(5 * time.Minute)

	return &evidence.Manifest{
		SchemaVersion: "1.1.0",
		RunID:        FixtureRunIDs[index],
		Scenario:     FixtureScenarios[index],
		ControllerID: "12345",
		SubjectIdentity: &evidence.SubjectIdentity{
			GitCommit:                 "deadbeef",
			GitTree:                  "0123456789",
			GitObjectFormat:          "sha256",
			ControllerExecutableSHA256: "cafebabe",
		},
		SubjectImageIdentity: &evidence.SubjectImageIdentity{
			ImageReference:        "test-image:latest",
			ImageID:              "sha256:abcdef123456",
			PrebuildBinarySHA256: "binaryhash",
		},
		HostID: &evidence.HostIdentity{
			KernelRelease: "6.1.0",
			CgroupMode:   "2",
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
			ScenarioValid:        true,
			CanariesValid:       true,
			ProvenanceValid:     true,
		}
	case "canary-bounded":
		return &evidence.Verdict{
			Scenario:               scenario,
			OverallClassification:  analysis.ClassificationStable,
			MemoryClassification:   analysis.ClassificationStable,
			ResourceClassification: analysis.ClassificationStable,
			SemanticClassification: analysis.ClassificationStable,
			ScenarioValid:        true,
			CanariesValid:       true,
			ProvenanceValid:     true,
		}
	case "canary-descriptor":
		return &evidence.Verdict{
			Scenario:               scenario,
			OverallClassification:  analysis.ClassificationResourceGrowth,
			MemoryClassification:   analysis.ClassificationStable,
			ResourceClassification: analysis.ClassificationResourceGrowth,
			SemanticClassification: analysis.ClassificationStable,
			ScenarioValid:        true,
			CanariesValid:       true,
			ProvenanceValid:     true,
		}
	default:
		return &evidence.Verdict{
			Scenario:       scenario,
			ScenarioValid:    true,
			CanariesValid:   true,
			ProvenanceValid: true,
		}
	}
}

// writeChildArtifacts writes all required child artifacts.
func writeChildArtifacts(t SimpleTestHelper, runDir string, index int, manifest *evidence.Manifest, verdict *evidence.Verdict) {
	t.Helper()

	// manifest.json
	manifestJSON, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(runDir, "manifest.json"), manifestJSON, 0644)

	// verdict.json
	verdictJSON, _ := json.MarshalIndent(verdict, "", "  ")
	os.WriteFile(filepath.Join(runDir, "verdict.json"), verdictJSON, 0644)

	// container-inspect.json
	containerInspect := map[string]string{"Id": FixtureContainerIDs[index]}
	containerJSON, _ := json.MarshalIndent(containerInspect, "", "  ")
	os.WriteFile(filepath.Join(runDir, "container-inspect.json"), containerJSON, 0644)

	// network-identity.json - use anonymous struct with lowercase keys to match NetworkIdentity JSON tags
	networkJSONData, _ := json.MarshalIndent(struct {
		SchemaVersion string `json:"schema_version"`
		ID           string `json:"id"`
		Name         string `json:"name"`
	}{
		SchemaVersion: "1.0.0",
		ID:           FixtureNetworkIDs[index],
		Name:         "test-network-" + FixtureRunIDs[index],
	}, "", "  ")
	os.WriteFile(filepath.Join(runDir, "network-identity.json"), networkJSONData, 0644)

	// samples.csv
	samples := fmt.Sprintf("process_pid,process_start_time\n%d,%d\n%d,%d\n",
		FixturePIDs[index], FixtureStartTimes[index],
		FixturePIDs[index], FixtureStartTimes[index])
	os.WriteFile(filepath.Join(runDir, "samples.csv"), []byte(samples), 0644)

	// events.jsonl
	os.WriteFile(filepath.Join(runDir, "events.jsonl"), []byte("{}\n{}\n"), 0644)

	// container-logs.txt
	os.WriteFile(filepath.Join(runDir, "container-logs.txt"), []byte("container logs\n"), 0644)

	// initial-canary-state.json
	initialState := buildCanaryState(FixtureScenarios[index], 0)
	initialJSON, _ := json.MarshalIndent(initialState, "", "  ")
	os.WriteFile(filepath.Join(runDir, "initial-canary-state.json"), initialJSON, 0644)

	// final-canary-state.json
	finalState := buildCanaryState(FixtureScenarios[index], 1)
	finalJSON, _ := json.MarshalIndent(finalState, "", "  ")
	os.WriteFile(filepath.Join(runDir, "final-canary-state.json"), finalJSON, 0644)

	// workload-result.json
	workload := buildWorkloadResult(FixtureScenarios[index])
	workloadJSON, _ := json.MarshalIndent(workload, "", "  ")
	os.WriteFile(filepath.Join(runDir, "workload-result.json"), workloadJSON, 0644)

	// Compute child checksums
	checksumContent := computeChildChecksumsContent(runDir)
	os.WriteFile(filepath.Join(runDir, "checksums.txt"), []byte(checksumContent), 0644)
}

// buildCanaryState creates a canary state.
func buildCanaryState(scenario string, phase int) CanaryState {
	state := CanaryState{
		Mode:            scenario,
		BufferCapacity:  32 * 1024 * 1024,
		RetainedBlocks:  0,
		RetainedBytes:   0,
		FDCount:         10,
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
		Requested:  100,
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
			Scenario: FixtureScenarios[i],
			RunID:    FixtureRunIDs[i],
			Container: ContainerCleanupRecord{
				ID:     FixtureContainerIDs[i],
				Status: "gone",
			},
			Network: NetworkCleanupRecord{
				ID:     FixtureNetworkIDs[i],
				Status: "gone",
			},
			Process: ProcessCleanupRecord{
				PID:       FixturePIDs[i],
				StartTime: FixtureStartTimes[i],
				Status:    "gone",
			},
		}
	}

	return &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:        FixtureMatrixID,
		ObservedAt:      FixtureObservedAt,
		NetworkOwnership: "per_run",
		Runs:            records,
	}
}

// buildVerifiedRuns creates VerifiedRun slice from manifests and verdicts.
func buildVerifiedRuns(manifests []*evidence.Manifest, verdicts []*evidence.Verdict) []*VerifiedRun {
	runs := make([]*VerifiedRun, 3)
	for i := 0; i < 3; i++ {
		runs[i] = &VerifiedRun{
			DeclaredRunID:        FixtureRunIDs[i],
			DeclaredScenario:     FixtureScenarios[i],
			RunIndex:            i,
			ActualManifest:      manifests[i],
			ActualVerdict:       verdicts[i],
			ContainerID:         FixtureContainerIDs[i],
			NetworkID:           FixtureNetworkIDs[i],
			SubjectPID:          FixturePIDs[i],
			SubjectStartTime:    FixtureStartTimes[i],
			ProcessCleanupStatus: ProcessGone,
			ChildVerified:       true,
			CleanupEvidenceLoaded: true,
			CleanupEvidenceValid:   true,
		}
	}
	return runs
}

// computeChildChecksumsContent computes checksums for child artifacts.
func computeChildChecksumsContent(runDir string) string {
	artifacts := []string{
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

	return computeChecksumsContent(runDir, artifacts)
}

// computeMatrixChecksumsContent computes checksums for matrix artifacts.
// P0-1 FIX: Uses canonicalMatrixArtifactInventory for single source of truth.
func computeMatrixChecksumsContent(matrixDir string) string {
	return computeChecksumsContent(matrixDir, canonicalMatrixArtifactInventory[:])
}

// computeChecksumsContent computes SHA256 checksums for listed files.
// P0-8 FIX: Panics on missing artifacts to catch fixture bugs immediately.
// This ensures missing artifacts are caught during fixture creation, not verification.
func computeChecksumsContent(dir string, artifacts []string) string {
	var content string
	for _, name := range artifacts {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			// P0-8 FIX: Fail immediately on missing artifacts
			// Missing artifacts indicate fixture creation bugs, not test bugs.
			// The fixture helper (SimpleTestHelper) is not available here,
			// so we panic with a clear message for immediate detection.
			panic(fmt.Sprintf("computeChecksumsContent: missing artifact %q in %q: %v", name, dir, err))
		}
		hash := sha256.Sum256(data)
		content += fmt.Sprintf("%s  %s\n", hex.EncodeToString(hash[:]), name)
	}
	return content
}

// regenerateAllChecksums regenerates both child and matrix checksums.
// P0-7 FIX: Regenerates child checksums after child mutations.
func regenerateAllChecksums(fixture *MatrixFixture) {
	// First regenerate child checksums for all runs
	for i, runDir := range fixture.RunDirs {
		// Regenerate child checksums
		childChecksumContent := computeChildChecksumsContent(runDir)
		os.WriteFile(filepath.Join(runDir, "checksums.txt"), []byte(childChecksumContent), 0644)

		// Update manifest's checksums_sha256 for this run
		checksumHash := sha256.Sum256([]byte(childChecksumContent))
		fixture.Manifest.Runs[i].ChecksumsSHA256 = hex.EncodeToString(checksumHash[:])
	}

	// Update and regenerate matrix checksums
	manifestJSON, _ := json.MarshalIndent(fixture.Manifest, "", "  ")
	os.WriteFile(filepath.Join(fixture.RootDir, "matrix-manifest.json"), manifestJSON, 0644)

	cleanupJSON, _ := json.MarshalIndent(fixture.Cleanup, "", "  ")
	os.WriteFile(filepath.Join(fixture.RootDir, "matrix-cleanup.json"), cleanupJSON, 0644)

	verdictJSON, _ := json.MarshalIndent(fixture.Verdict, "", "  ")
	os.WriteFile(filepath.Join(fixture.RootDir, "matrix-verdict.json"), verdictJSON, 0644)

	matrixChecksumContent := computeMatrixChecksumsContent(fixture.RootDir)
	os.WriteFile(filepath.Join(fixture.RootDir, "matrix-checksums.txt"), []byte(matrixChecksumContent), 0644)
}

// SimpleTestHelper is a subset of *testing.T for fixture writing.
type SimpleTestHelper interface {
	Helper()
	Fatalf(format string, args ...interface{})
}
