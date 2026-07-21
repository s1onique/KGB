// reconstruct_test.go — CORRECTION03 Mutagenesis Tests
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03
//
// These tests verify the single authoritative reconstruction path by testing
// mutations against known-good fixtures. Each test case mutates one artifact
// and expects the reconstruction to fail-closed.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence"
)

// =============================================================================
// FIXTURE HELPERS
// =============================================================================

// goodManifest returns a valid matrix manifest for testing.
func goodManifest() *MatrixManifest {
	return &MatrixManifest{
		SchemaVersion: MatrixSchemaVersion,
		MatrixID:      "test-matrix-123",
		StartedAt:     time.Now().Add(-10 * time.Minute),
		FinishedAt:    time.Now(),
		ExecutionIdentity: &MatrixExecutionIdentity{
			ImplementationCommitOID:     "abc123def456",
			ImplementationTreeOID:       "tree789",
			GitObjectFormat:             "sha256",
			ControllerPID:               12345,
			ControllerExecutableSHA256:  "hash123",
			RunManifestSchemaVersion:    "1.1.0",
			ImageReference:              "test-image:latest",
			ImageID:                    "sha256:abc123",
			CanaryBinarySHA256:          "binaryhash",
			HostKernelRelease:          "6.1.0",
			HostKernelVersion:          "Linux 6.1.0",
			HostCgroupMode:             "2",
			DockerEngineVersion:        "24.0.0",
			DockerAPIVersion:           "1.43",
		},
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1", Path: "runs/run-1"},
			{Index: 2, Scenario: "canary-bounded", RunID: "run-2", Path: "runs/run-2"},
			{Index: 3, Scenario: "canary-descriptor", RunID: "run-3", Path: "runs/run-3"},
		},
	}
}

// goodChildManifest returns a valid child run manifest.
func goodChildManifest(runID, scenario string) *evidence.Manifest {
	return &evidence.Manifest{
		SchemaVersion: "1.1.0",
		RunID:        runID,
		Scenario:     scenario,
		ControllerID: "12345",
		SubjectIdentity: &evidence.SubjectIdentity{
			GitCommit:                 "abc123def456",
			GitTree:                  "tree789",
			GitObjectFormat:          "sha256",
			ControllerExecutableSHA256: "hash123",
		},
		SubjectImageIdentity: &evidence.SubjectImageIdentity{
			ImageReference:        "test-image:latest",
			ImageID:              "sha256:abc123",
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
		StartedAt:     time.Now().Add(-5 * time.Minute),
		FinishedAt:    time.Now(),
	}
}

// goodChildVerdict returns a valid child run verdict with correct classification.
func goodChildVerdict(scenario string) *evidence.Verdict {
	// According to the calibration contract:
	// - canary-growing: overall=growth, memory=growing
	// - canary-bounded: overall=stable, memory=stable
	// - canary-descriptor: overall=resource_growth, memory=stable
	switch scenario {
	case "canary-growing":
		return &evidence.Verdict{
			OverallClassification:  analysis.ClassificationGrowing,
			MemoryClassification:   analysis.ClassificationGrowing,
			ResourceClassification: analysis.ClassificationStable,
			SemanticClassification: analysis.ClassificationStable,
		}
	case "canary-bounded":
		return &evidence.Verdict{
			OverallClassification:  analysis.ClassificationStable,
			MemoryClassification:   analysis.ClassificationStable,
			ResourceClassification: analysis.ClassificationStable,
			SemanticClassification: analysis.ClassificationStable,
		}
	case "canary-descriptor":
		return &evidence.Verdict{
			OverallClassification:  analysis.ClassificationResourceGrowth,
			MemoryClassification:   analysis.ClassificationStable,
			ResourceClassification: analysis.ClassificationResourceGrowth,
			SemanticClassification: analysis.ClassificationStable,
		}
	default:
		return &evidence.Verdict{
			OverallClassification:  analysis.ClassificationStable,
			MemoryClassification:   analysis.ClassificationStable,
			ResourceClassification: analysis.ClassificationStable,
			SemanticClassification: analysis.ClassificationStable,
		}
	}
}

// goodCleanupEvidence returns valid cleanup evidence.
func goodCleanupEvidence(matrixID string) *MatrixCleanupEvidence {
	return &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:        matrixID,
		ObservedAt:      time.Now(),
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:    0,
				Scenario: "canary-growing",
				RunID:    "run-1",
				Container: ContainerCleanupRecord{
					ID:     "container-abc123",
					Name:   "test-container-1",
					Status: "gone",
				},
				Network: NetworkCleanupRecord{
					ID:     "network-def456",
					Name:   "test-network-1",
					Status: "gone",
				},
				Process: ProcessCleanupRecord{
					PID:       54321,
					StartTime: 1234567890,
					Status:    "gone",
				},
			},
			{
				Index:    1,
				Scenario: "canary-bounded",
				RunID:    "run-2",
				Container: ContainerCleanupRecord{
					ID:     "container-ghi789",
					Name:   "test-container-2",
					Status: "gone",
				},
				Network: NetworkCleanupRecord{
					ID:     "network-jkl012",
					Name:   "test-network-2",
					Status: "gone",
				},
				Process: ProcessCleanupRecord{
					PID:       54322,
					StartTime: 1234567891,
					Status:    "gone",
				},
			},
			{
				Index:    2,
				Scenario: "canary-descriptor",
				RunID:    "run-3",
				Container: ContainerCleanupRecord{
					ID:     "container-mno345",
					Name:   "test-container-3",
					Status: "gone",
				},
				Network: NetworkCleanupRecord{
					ID:     "network-pqr678",
					Name:   "test-network-3",
					Status: "gone",
				},
				Process: ProcessCleanupRecord{
					PID:       54323,
					StartTime: 1234567892,
					Status:    "gone",
				},
			},
		},
	}
}

// goodVerifiedRun returns a valid verified run for testing.
func goodVerifiedRun(index int, scenario, runID string) *VerifiedRun {
	manifest := goodChildManifest(runID, scenario)
	verdict := goodChildVerdict(scenario)

	return &VerifiedRun{
		DeclaredRunID:     runID,
		DeclaredScenario:  scenario,
		RunIndex:         index,
		ActualManifest:    manifest,
		ActualVerdict:     verdict,
		ContainerID:       "container-abc123",
		NetworkID:         "network-def456",
		SubjectPID:        54321,
		SubjectStartTime: 1234567890,
		ProcessCleanupStatus: ProcessGone,
		CleanupVerified:   true,
	}
}

// setupTestMatrixDir creates a temporary matrix directory with valid artifacts.
func setupTestMatrixDir(t *testing.T) (string, *MatrixManifest) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "matrix-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	manifest := goodManifest()
	runsDir := filepath.Join(tmpDir, "runs")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create runs dir: %v", err)
	}

	// Write child run artifacts
	scenarios := []string{"canary-growing", "canary-bounded", "canary-descriptor"}
	runIDs := []string{"run-1", "run-2", "run-3"}
	containerIDs := []string{"container-abc123", "container-ghi789", "container-mno345"}

	for i := 0; i < 3; i++ {
		runPath := filepath.Join(runsDir, runIDs[i])
		if err := os.MkdirAll(runPath, 0755); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("failed to create run dir: %v", err)
		}

		// Write manifest.json
		childManifest := goodChildManifest(runIDs[i], scenarios[i])
		manifestData, _ := json.MarshalIndent(childManifest, "", "  ")
		if err := os.WriteFile(filepath.Join(runPath, "manifest.json"), manifestData, 0644); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("failed to write manifest: %v", err)
		}

		// Write verdict.json
		childVerdict := goodChildVerdict(scenarios[i])
		verdictData, _ := json.MarshalIndent(childVerdict, "", "  ")
		if err := os.WriteFile(filepath.Join(runPath, "verdict.json"), verdictData, 0644); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("failed to write verdict: %v", err)
		}

		// Write container-inspect.json
		containerInspect := map[string]string{"Id": containerIDs[i]}
		containerData, _ := json.MarshalIndent(containerInspect, "", "  ")
		if err := os.WriteFile(filepath.Join(runPath, "container-inspect.json"), containerData, 0644); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("failed to write container inspect: %v", err)
		}

		// Write samples.csv
		var samplesContent []byte
		if i == 0 {
			samplesContent = []byte("process_pid,process_start_time\n54321,1234567890\n54321,1234567890\n")
		} else if i == 1 {
			samplesContent = []byte("process_pid,process_start_time\n54322,1234567891\n54322,1234567891\n")
		} else {
			samplesContent = []byte("process_pid,process_start_time\n54323,1234567892\n54323,1234567892\n")
		}
		if err := os.WriteFile(filepath.Join(runPath, "samples.csv"), samplesContent, 0644); err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("failed to write samples: %v", err)
		}
	}

	return tmpDir, manifest
}

// =============================================================================
// TEST 1: Unknown JSON fields are rejected in cleanup evidence
// =============================================================================

func TestDecodeStrictJSON_RejectsUnknownFields(t *testing.T) {
	// Test with unknown field
	dirtyJSON := []byte(`{
		"schema_version": "1.0.0",
		"matrix_id": "test",
		"observed_at": "2024-01-01T00:00:00Z",
		"network_ownership": "per_run",
		"runs": [],
		"unknown_field": "should cause error"
	}`)

	var evidence MatrixCleanupEvidence
	err := decodeStrictJSON(dirtyJSON, &evidence)
	if err == nil {
		t.Error("expected error for unknown field, got nil")
	}
}

// =============================================================================
// TEST 2: Second JSON document is rejected
// =============================================================================

func TestDecodeStrictJSON_RejectsTrailingDocument(t *testing.T) {
	// Test with trailing JSON document
	dirtyJSON := []byte(`{
		"schema_version": "1.0.0",
		"matrix_id": "test",
		"observed_at": "2024-01-01T00:00:00Z",
		"network_ownership": "per_run",
		"runs": []
	}
	{
		"schema_version": "1.0.0",
		"matrix_id": "test2"
	}`)

	var evidence MatrixCleanupEvidence
	err := decodeStrictJSON(dirtyJSON, &evidence)
	if err == nil {
		t.Error("expected error for trailing JSON document, got nil")
	}
}

// =============================================================================
// TEST 3: Empty input is rejected
// =============================================================================

func TestDecodeStrictJSON_RejectsEmptyInput(t *testing.T) {
	var evidence MatrixCleanupEvidence
	err := decodeStrictJSON([]byte{}, &evidence)
	if err == nil {
		t.Error("expected error for empty input, got nil")
	}
}

// =============================================================================
// TEST 4: Cleanup evidence with empty per-run network ID is rejected
// =============================================================================

func TestValidateCleanupEvidence_RequiresNetworkIDsForPerRunMode(t *testing.T) {
	cleanup := &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:        "test-matrix",
		ObservedAt:      time.Now(),
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:    0,
				Scenario: "canary-growing",
				RunID:    "run-1",
				Container: ContainerCleanupRecord{ID: "c1", Status: "gone"},
				Network:  NetworkCleanupRecord{ID: "", Status: "gone"}, // Empty ID!
				Process:  ProcessCleanupRecord{PID: 1, StartTime: 1, Status: "gone"},
			},
		},
	}

	manifest := &MatrixManifest{
		MatrixID: "test-matrix",
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1"},
		},
	}

	err := ValidateCleanupEvidence(cleanup, manifest)
	if err == nil {
		t.Error("expected error for empty network ID in per_run mode, got nil")
	}
}

// =============================================================================
// TEST 5: Duplicate per-run network IDs are rejected
// =============================================================================

func TestValidateCleanupEvidence_RejectsDuplicateNetworkIDs(t *testing.T) {
	cleanup := &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:        "test-matrix",
		ObservedAt:      time.Now(),
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:    0,
				Scenario: "canary-growing",
				RunID:    "run-1",
				Container: ContainerCleanupRecord{ID: "c1", Status: "gone"},
				Network:  NetworkCleanupRecord{ID: "same-id", Status: "gone"},
				Process:  ProcessCleanupRecord{PID: 1, StartTime: 1, Status: "gone"},
			},
			{
				Index:    1,
				Scenario: "canary-bounded",
				RunID:    "run-2",
				Container: ContainerCleanupRecord{ID: "c2", Status: "gone"},
				Network:  NetworkCleanupRecord{ID: "same-id", Status: "gone"}, // Duplicate!
				Process:  ProcessCleanupRecord{PID: 2, StartTime: 2, Status: "gone"},
			},
		},
	}

	manifest := &MatrixManifest{
		MatrixID: "test-matrix",
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1"},
			{Index: 2, Scenario: "canary-bounded", RunID: "run-2"},
		},
	}

	err := ValidateCleanupEvidence(cleanup, manifest)
	if err == nil {
		t.Error("expected error for duplicate network IDs in per_run mode, got nil")
	}
}

// =============================================================================
// TEST 6: Matrix ID mismatch is rejected
// =============================================================================

func TestValidateCleanupEvidence_RejectsMatrixIDMismatch(t *testing.T) {
	cleanup := &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:        "different-matrix-id",
		ObservedAt:      time.Now(),
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:    0,
				Scenario: "canary-growing",
				RunID:    "run-1",
				Container: ContainerCleanupRecord{ID: "c1", Status: "gone"},
				Network:  NetworkCleanupRecord{ID: "n1", Status: "gone"},
				Process:  ProcessCleanupRecord{PID: 1, StartTime: 1, Status: "gone"},
			},
		},
	}

	manifest := &MatrixManifest{
		MatrixID: "test-matrix",
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1"},
		},
	}

	err := ValidateCleanupEvidence(cleanup, manifest)
	if err == nil {
		t.Error("expected error for matrix ID mismatch, got nil")
	}
}

// =============================================================================
// TEST 7: Unsupported cleanup schema is rejected
// =============================================================================

func TestValidateCleanupEvidence_RejectsUnsupportedSchema(t *testing.T) {
	cleanup := &MatrixCleanupEvidence{
		SchemaVersion:    "99.99.99",
		MatrixID:        "test-matrix",
		ObservedAt:      time.Now(),
		NetworkOwnership: "per_run",
		Runs:            []RunCleanupRecord{},
	}

	manifest := &MatrixManifest{
		MatrixID: "test-matrix",
		Runs:     []MatrixRunDeclaration{},
	}

	err := ValidateCleanupEvidence(cleanup, manifest)
	if err == nil {
		t.Error("expected error for unsupported schema version, got nil")
	}
}

// =============================================================================
// TEST 8: Wrong cleanup record index is rejected
// =============================================================================

func TestValidateCleanupEvidence_RejectsWrongIndex(t *testing.T) {
	cleanup := &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:        "test-matrix",
		ObservedAt:      time.Now(),
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:    99, // Wrong index!
				Scenario: "canary-growing",
				RunID:    "run-1",
				Container: ContainerCleanupRecord{ID: "c1", Status: "gone"},
				Network:  NetworkCleanupRecord{ID: "n1", Status: "gone"},
				Process:  ProcessCleanupRecord{PID: 1, StartTime: 1, Status: "gone"},
			},
		},
	}

	manifest := &MatrixManifest{
		MatrixID: "test-matrix",
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1"},
		},
	}

	err := ValidateCleanupEvidence(cleanup, manifest)
	if err == nil {
		t.Error("expected error for wrong record index, got nil")
	}
}

// =============================================================================
// TEST 9: Verify child bundle through BuildVerifiedRunsFromMatrix
// =============================================================================

func TestBuildVerifiedRunsFromMatrix_LoadsAllArtifacts(t *testing.T) {
	tmpDir, manifest := setupTestMatrixDir(t)
	defer os.RemoveAll(tmpDir)

	cleanup := goodCleanupEvidence(manifest.MatrixID)

	runs, err := BuildVerifiedRunsFromMatrix(tmpDir, manifest, cleanup)
	if err != nil {
		t.Fatalf("BuildVerifiedRunsFromMatrix failed: %v", err)
	}

	if len(runs) != 3 {
		t.Errorf("expected 3 runs, got %d", len(runs))
	}

	// Verify each run has all artifacts loaded
	for _, run := range runs {
		if run.ActualManifest == nil {
			t.Errorf("run %s has nil manifest", run.DeclaredRunID)
		}
		if run.ActualVerdict == nil {
			t.Errorf("run %s has nil verdict", run.DeclaredRunID)
		}
		if run.ContainerID == "" {
			t.Errorf("run %s has empty container ID", run.DeclaredRunID)
		}
		if run.SubjectPID == 0 {
			t.Errorf("run %s has zero PID", run.DeclaredRunID)
		}
		if !run.CleanupVerified {
			t.Errorf("run %s has CleanupVerified=false", run.DeclaredRunID)
		}
	}
}

// =============================================================================
// TEST 10: Unknown fields in child manifest are rejected
// =============================================================================

func TestBuildVerifiedRunsFromMatrix_RejectsUnknownFieldsInManifest(t *testing.T) {
	tmpDir, manifest := setupTestMatrixDir(t)
	defer os.RemoveAll(tmpDir)

	// Corrupt manifest.json with unknown field by appending one
	runPath := filepath.Join(tmpDir, "runs", "run-1")
	manifestPath := filepath.Join(runPath, "manifest.json")

	originalData, _ := os.ReadFile(manifestPath)
	// Parse and re-serialize with an extra unknown field
	var m map[string]interface{}
	json.Unmarshal(originalData, &m)
	m["unknown_extra_field"] = "should cause error" // Unknown field

	corruptData, _ := json.Marshal(&m)
	os.WriteFile(manifestPath, corruptData, 0644)

	cleanup := goodCleanupEvidence(manifest.MatrixID)

	_, err := BuildVerifiedRunsFromMatrix(tmpDir, manifest, cleanup)
	if err == nil {
		t.Error("expected error for unknown field in manifest, got nil")
	}
}

// =============================================================================
// TEST 11: Incorrect memory classification invalidates scenario
// =============================================================================

func TestReconstructMatrixVerdict_RejectsWrongMemoryClassification(t *testing.T) {
	manifest := goodManifest()
	cleanup := goodCleanupEvidence(manifest.MatrixID)

	// Create verified runs with wrong memory classification for canary-growing
	verifiedRuns := []*VerifiedRun{
		{
			DeclaredRunID:    "run-1",
			DeclaredScenario: "canary-growing",
			RunIndex:        0,
			ActualManifest:   goodChildManifest("run-1", "canary-growing"),
			ActualVerdict: &evidence.Verdict{
				OverallClassification:  analysis.ClassificationGrowing,
				MemoryClassification:   analysis.ClassificationStable, // WRONG! Should be Growing
				ResourceClassification: analysis.ClassificationStable,
				SemanticClassification: analysis.ClassificationStable,
			},
			ContainerID:           "container-abc123",
			NetworkID:             "network-def456",
			SubjectPID:            54321,
			SubjectStartTime:      1234567890,
			ProcessCleanupStatus:  ProcessGone,
			CleanupVerified:       true,
		},
		goodVerifiedRun(1, "canary-bounded", "run-2"),
		goodVerifiedRun(2, "canary-descriptor", "run-3"),
	}

	verdict, err := ReconstructMatrixVerdict(manifest, verifiedRuns, cleanup)
	if err != nil {
		t.Fatalf("ReconstructMatrixVerdict failed: %v", err)
	}

	// Matrix should be INVALID because memory classification is wrong for canary-growing
	// The implementation is fail-closed and validates all classifications
	if verdict.MatrixValid {
		t.Error("expected MatrixValid=false for wrong memory classification")
	}
}

// =============================================================================
// TEST 12: Incorrect resource classification in descriptor scenario
// =============================================================================

func TestReconstructMatrixVerdict_RejectsWrongResourceClassification(t *testing.T) {
	manifest := goodManifest()
	cleanup := goodCleanupEvidence(manifest.MatrixID)

	// Create verified runs with wrong resource classification for canary-descriptor
	verifiedRuns := []*VerifiedRun{
		goodVerifiedRun(0, "canary-growing", "run-1"),
		goodVerifiedRun(1, "canary-bounded", "run-2"),
		{
			DeclaredRunID:    "run-3",
			DeclaredScenario: "canary-descriptor",
			RunIndex:        2,
			ActualManifest:   goodChildManifest("run-3", "canary-descriptor"),
			ActualVerdict: &evidence.Verdict{
				OverallClassification:  analysis.ClassificationResourceGrowth,
				MemoryClassification:   analysis.ClassificationStable,
				ResourceClassification: analysis.ClassificationStable, // WRONG! Should be ResourceGrowth
				SemanticClassification: analysis.ClassificationStable,
			},
			ContainerID:           "container-mno345",
			NetworkID:             "network-pqr678",
			SubjectPID:            54323,
			SubjectStartTime:      1234567892,
			ProcessCleanupStatus:  ProcessGone,
			CleanupVerified:       true,
		},
	}

	verdict, err := ReconstructMatrixVerdict(manifest, verifiedRuns, cleanup)
	if err != nil {
		t.Fatalf("ReconstructMatrixVerdict failed: %v", err)
	}

	// Matrix should be INVALID because resource classification is wrong for canary-descriptor
	// The implementation is fail-closed and validates all classifications
	if verdict.MatrixValid {
		t.Error("expected MatrixValid=false for wrong resource classification")
	}
}

// =============================================================================
// TEST 13: ScenarioResult.Verified based on schema version
// =============================================================================

func TestReconstructScenarioResults_SetsVerifiedFromSchemaVersion(t *testing.T) {
	verifiedRuns := []*VerifiedRun{
		{
			DeclaredRunID:    "run-1",
			DeclaredScenario: "canary-growing",
			ActualManifest:   &evidence.Manifest{SchemaVersion: "1.1.0"},
			ActualVerdict:    &evidence.Verdict{},
		},
		{
			DeclaredRunID:    "run-2",
			DeclaredScenario: "canary-bounded",
			ActualManifest:   &evidence.Manifest{SchemaVersion: "1.0.0"}, // Wrong version
			ActualVerdict:    &evidence.Verdict{},
		},
		{
			DeclaredRunID:    "run-3",
			DeclaredScenario: "canary-descriptor",
			ActualManifest:   &evidence.Manifest{SchemaVersion: "1.1.0"},
			ActualVerdict:    &evidence.Verdict{},
		},
	}

	results, err := ReconstructScenarioResults(verifiedRuns)
	if err != nil {
		t.Fatalf("ReconstructScenarioResults failed: %v", err)
	}

	if results["canary-growing"].Verified != true {
		t.Error("canary-growing should be verified (schema 1.1.0)")
	}
	if results["canary-bounded"].Verified != false {
		t.Error("canary-bounded should not be verified (schema 1.0.0)")
	}
	if results["canary-descriptor"].Verified != true {
		t.Error("canary-descriptor should be verified (schema 1.1.0)")
	}
}

// =============================================================================
// TEST 14: Equal but invalid verdicts detected by CompareVerdicts
// =============================================================================

func TestCompareVerdicts_DetectsEqualInvalidVerdicts(t *testing.T) {
	stored := &MatrixVerdict{
		MatrixID:    "test",
		MatrixValid: true, // Both claim valid
		ScenarioResults: map[string]*ScenarioResult{
			"canary-growing": {Overall: "growth", Verified: true},
		},
		CrossRunChecks: &CrossRunChecks{},
	}

	reconstructed := &MatrixVerdict{
		MatrixID:    "test",
		MatrixValid: false, // But actually invalid
		ScenarioResults: map[string]*ScenarioResult{
			"canary-growing": {Overall: "wrong", Verified: false}, // Different!
		},
		CrossRunChecks: &CrossRunChecks{},
	}

	diffs := CompareVerdicts(stored, reconstructed)
	if len(diffs) == 0 {
		t.Error("expected differences between verdicts")
	}

	// Should detect the matrix_valid difference
	foundValidDiff := false
	for _, d := range diffs {
		if d.Path == "matrix_valid" {
			foundValidDiff = true
			break
		}
	}
	if !foundValidDiff {
		t.Error("expected matrix_valid difference")
	}
}

// =============================================================================
// TEST 15: CleanupComplete fails with still_alive process status
// =============================================================================

func TestReconstructCleanupComplete_RejectsStillAliveProcess(t *testing.T) {
	verifiedRuns := []*VerifiedRun{
		goodVerifiedRun(0, "canary-growing", "run-1"),
	}

	cleanup := &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:        "test",
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:    0,
				Scenario: "canary-growing",
				RunID:    "run-1",
				Container: ContainerCleanupRecord{ID: "container-abc123", Status: "gone"},
				Network:  NetworkCleanupRecord{ID: "network-def456", Status: "gone"},
				Process:  ProcessCleanupRecord{PID: 54321, StartTime: 1234567890, Status: "still_alive"}, // Alive!
			},
		},
	}

	result := reconstructCleanupComplete(verifiedRuns, cleanup)
	if result {
		t.Error("expected CleanupComplete=false for still_alive process")
	}
}

// =============================================================================
// TEST 16: CleanupComplete fails when container still exists
// =============================================================================

func TestReconstructCleanupComplete_RejectsExistingContainer(t *testing.T) {
	verifiedRuns := []*VerifiedRun{
		goodVerifiedRun(0, "canary-growing", "run-1"),
	}

	cleanup := &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:        "test",
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:    0,
				Scenario: "canary-growing",
				RunID:    "run-1",
				Container: ContainerCleanupRecord{ID: "container-abc123", Status: "exists"}, // Still exists!
				Network:  NetworkCleanupRecord{ID: "network-def456", Status: "gone"},
				Process:  ProcessCleanupRecord{PID: 54321, StartTime: 1234567890, Status: "gone"},
			},
		},
	}

	result := reconstructCleanupComplete(verifiedRuns, cleanup)
	if result {
		t.Error("expected CleanupComplete=false for existing container")
	}
}

// =============================================================================
// TEST 17: CleanupComplete fails when PID mismatch
// =============================================================================

func TestReconstructCleanupComplete_RejectsPIDMismatch(t *testing.T) {
	verifiedRuns := []*VerifiedRun{
		goodVerifiedRun(0, "canary-growing", "run-1"),
	}

	cleanup := &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:        "test",
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:    0,
				Scenario: "canary-growing",
				RunID:    "run-1",
				Container: ContainerCleanupRecord{ID: "container-abc123", Status: "gone"},
				Network:  NetworkCleanupRecord{ID: "network-def456", Status: "gone"},
				Process:  ProcessCleanupRecord{PID: 99999, StartTime: 1234567890, Status: "gone"}, // Wrong PID!
			},
		},
	}

	result := reconstructCleanupComplete(verifiedRuns, cleanup)
	if result {
		t.Error("expected CleanupComplete=false for PID mismatch")
	}
}

// =============================================================================
// TEST 18: MatrixValid fails with wrong overall classification
// =============================================================================

func TestReconstructMatrixVerdict_FailsWithWrongOverallClassification(t *testing.T) {
	manifest := goodManifest()
	cleanup := goodCleanupEvidence(manifest.MatrixID)

	// Create verified runs with wrong overall classification for canary-growing
	verifiedRuns := []*VerifiedRun{
		{
			DeclaredRunID:    "run-1",
			DeclaredScenario: "canary-growing",
			RunIndex:        0,
			ActualManifest:   goodChildManifest("run-1", "canary-growing"),
			ActualVerdict: &evidence.Verdict{
				OverallClassification:  analysis.ClassificationStable, // WRONG! Should be Growth
				MemoryClassification:   analysis.ClassificationGrowing,
				ResourceClassification: analysis.ClassificationStable,
				SemanticClassification: analysis.ClassificationStable,
			},
			ContainerID:           "container-abc123",
			NetworkID:             "network-def456",
			SubjectPID:            54321,
			SubjectStartTime:      1234567890,
			ProcessCleanupStatus:  ProcessGone,
			CleanupVerified:       true,
		},
		goodVerifiedRun(1, "canary-bounded", "run-2"),
		goodVerifiedRun(2, "canary-descriptor", "run-3"),
	}

	verdict, err := ReconstructMatrixVerdict(manifest, verifiedRuns, cleanup)
	if err != nil {
		t.Fatalf("ReconstructMatrixVerdict failed: %v", err)
	}

	if verdict.MatrixValid {
		t.Error("expected MatrixValid=false for wrong overall classification")
	}
}

// =============================================================================
// TEST 19: matrix_shared network mode accepts shared network ID
// =============================================================================

func TestValidateCleanupEvidence_AcceptsSharedNetworkMode(t *testing.T) {
	cleanup := &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:        "test-matrix",
		ObservedAt:      time.Now(),
		NetworkOwnership: "matrix_shared",
		Runs: []RunCleanupRecord{
			{
				Index:    0,
				Scenario: "canary-growing",
				RunID:    "run-1",
				Container: ContainerCleanupRecord{ID: "c1", Status: "gone"},
				Network:  NetworkCleanupRecord{ID: "shared-net", Status: "gone"},
				Process:  ProcessCleanupRecord{PID: 1, StartTime: 1, Status: "gone"},
			},
			{
				Index:    1,
				Scenario: "canary-bounded",
				RunID:    "run-2",
				Container: ContainerCleanupRecord{ID: "c2", Status: "gone"},
				Network:  NetworkCleanupRecord{ID: "shared-net", Status: "gone"}, // Same shared ID
				Process:  ProcessCleanupRecord{PID: 2, StartTime: 2, Status: "gone"},
			},
		},
	}

	manifest := &MatrixManifest{
		MatrixID: "test-matrix",
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1"},
			{Index: 2, Scenario: "canary-bounded", RunID: "run-2"},
		},
	}

	err := ValidateCleanupEvidence(cleanup, manifest)
	if err != nil {
		t.Errorf("unexpected error for shared network mode: %v", err)
	}
}

// =============================================================================
// TEST 20: matrix_shared mode rejects different network IDs
// =============================================================================

func TestValidateCleanupEvidence_RejectsDifferentNetworkIDsInSharedMode(t *testing.T) {
	cleanup := &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:        "test-matrix",
		ObservedAt:      time.Now(),
		NetworkOwnership: "matrix_shared",
		Runs: []RunCleanupRecord{
			{
				Index:    0,
				Scenario: "canary-growing",
				RunID:    "run-1",
				Container: ContainerCleanupRecord{ID: "c1", Status: "gone"},
				Network:  NetworkCleanupRecord{ID: "net-1", Status: "gone"},
				Process:  ProcessCleanupRecord{PID: 1, StartTime: 1, Status: "gone"},
			},
			{
				Index:    1,
				Scenario: "canary-bounded",
				RunID:    "run-2",
				Container: ContainerCleanupRecord{ID: "c2", Status: "gone"},
				Network:  NetworkCleanupRecord{ID: "net-2", Status: "gone"}, // Different!
				Process:  ProcessCleanupRecord{PID: 2, StartTime: 2, Status: "gone"},
			},
		},
	}

	manifest := &MatrixManifest{
		MatrixID: "test-matrix",
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1"},
			{Index: 2, Scenario: "canary-bounded", RunID: "run-2"},
		},
	}

	err := ValidateCleanupEvidence(cleanup, manifest)
	if err == nil {
		t.Error("expected error for different network IDs in matrix_shared mode")
	}
}

// =============================================================================
// TEST 21: Empty container ID in cleanup is rejected
// =============================================================================

func TestValidateCleanupEvidence_RequiresContainerID(t *testing.T) {
	cleanup := &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:        "test-matrix",
		ObservedAt:      time.Now(),
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:    0,
				Scenario: "canary-growing",
				RunID:    "run-1",
				Container: ContainerCleanupRecord{ID: "", Status: "gone"}, // Empty!
				Network:  NetworkCleanupRecord{ID: "n1", Status: "gone"},
				Process:  ProcessCleanupRecord{PID: 1, StartTime: 1, Status: "gone"},
			},
		},
	}

	manifest := &MatrixManifest{
		MatrixID: "test-matrix",
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1"},
		},
	}

	err := ValidateCleanupEvidence(cleanup, manifest)
	if err == nil {
		t.Error("expected error for empty container ID")
	}
}

// =============================================================================
// TEST 22: Zero PID in cleanup is rejected
// =============================================================================

func TestValidateCleanupEvidence_RequiresProcessPID(t *testing.T) {
	cleanup := &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:        "test-matrix",
		ObservedAt:      time.Now(),
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:    0,
				Scenario: "canary-growing",
				RunID:    "run-1",
				Container: ContainerCleanupRecord{ID: "c1", Status: "gone"},
				Network:  NetworkCleanupRecord{ID: "n1", Status: "gone"},
				Process:  ProcessCleanupRecord{PID: 0, StartTime: 1, Status: "gone"}, // Zero PID!
			},
		},
	}

	manifest := &MatrixManifest{
		MatrixID: "test-matrix",
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1"},
		},
	}

	err := ValidateCleanupEvidence(cleanup, manifest)
	if err == nil {
		t.Error("expected error for zero process PID")
	}
}

// =============================================================================
// TEST 23: Unknown network ownership mode is rejected
// =============================================================================

func TestValidateCleanupEvidence_RejectsUnknownNetworkMode(t *testing.T) {
	cleanup := &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:        "test-matrix",
		ObservedAt:      time.Now(),
		NetworkOwnership: "unknown_mode",
		Runs:            []RunCleanupRecord{},
	}

	manifest := &MatrixManifest{
		MatrixID: "test-matrix",
		Runs:     []MatrixRunDeclaration{},
	}

	err := ValidateCleanupEvidence(cleanup, manifest)
	if err == nil {
		t.Error("expected error for unknown network ownership mode")
	}
}

// =============================================================================
// TEST 24: Run count mismatch is rejected
// =============================================================================

func TestValidateCleanupEvidence_RejectsRunCountMismatch(t *testing.T) {
	cleanup := &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:        "test-matrix",
		ObservedAt:      time.Now(),
		NetworkOwnership: "per_run",
		Runs:            []RunCleanupRecord{}, // No runs
	}

	manifest := &MatrixManifest{
		MatrixID: "test-matrix",
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1"},
		},
	}

	err := ValidateCleanupEvidence(cleanup, manifest)
	if err == nil {
		t.Error("expected error for run count mismatch")
	}
}

// =============================================================================
// TEST 25: CrossRunCheckCount constant is accurate
// =============================================================================

func TestCrossRunCheckCount_Is16(t *testing.T) {
	if CrossRunCheckCount != 16 {
		t.Errorf("expected CrossRunCheckCount=16, got %d", CrossRunCheckCount)
	}
}

// =============================================================================
// TEST 26: CanonicalScenarioOrder is correct
// =============================================================================

func TestCanonicalScenarioOrder_IsCorrect(t *testing.T) {
	expected := []string{"canary-growing", "canary-bounded", "canary-descriptor"}
	for i, name := range CanonicalScenarioOrder {
		if name != expected[i] {
			t.Errorf("scenario %d: expected %q, got %q", i, expected[i], name)
		}
	}
}

// =============================================================================
// TEST 27: CanonicalCrossRunCheckNames contains all expected checks
// =============================================================================

func TestCanonicalCrossRunCheckNames_ContainsAllChecks(t *testing.T) {
	expected := []string{
		"SameCommitTree", "SameControllerPID", "SameControllerHash", "SameSchema",
		"SameThresholds", "SamePhaseConfig", "SameHostIdentity", "SameDockerIdentity",
		"SameImageIdentity", "SameCanaryBinary", "UniqueRunIDs", "UniqueSubjectProcesses",
		"UniqueContainerIDs", "FixedOrder", "NonOverlapping", "CleanupComplete",
	}

	if len(CanonicalCrossRunCheckNames) != len(expected) {
		t.Fatalf("expected %d check names, got %d", len(expected), len(CanonicalCrossRunCheckNames))
	}

	for i, name := range CanonicalCrossRunCheckNames {
		if name != expected[i] {
			t.Errorf("check %d: expected %q, got %q", i, expected[i], name)
		}
	}
}

// =============================================================================
// TEST 28: CountCanonicalChecks returns correct counts
// =============================================================================

func TestCountCanonicalChecks_ReturnsCorrectCounts(t *testing.T) {
	checks := &CrossRunChecks{
		SameCommitTree:       true,
		SameControllerPID:    true,
		SameControllerHash:   true,
		SameSchema:           true,
		SameThresholds:       true,
		SamePhaseConfig:      true,
		SameHostIdentity:     true,
		SameDockerIdentity:   true,
		SameImageIdentity:    true,
		SameCanaryBinary:     true,
		UniqueRunIDs:         true,
		UniqueSubjectProcesses: true,
		UniqueContainerIDs:   true,
		FixedOrder:           true,
		NonOverlapping:       true,
		CleanupComplete:      true,
	}

	total, passed, failed := CountCanonicalChecks(checks)
	if total != 16 {
		t.Errorf("expected total=16, got %d", total)
	}
	if passed != 16 {
		t.Errorf("expected passed=16, got %d", passed)
	}
	if failed != 0 {
		t.Errorf("expected failed=0, got %d", failed)
	}
}

// =============================================================================
// TEST 29: CountCanonicalChecks with some failures
// =============================================================================

func TestCountCanonicalChecks_WithFailures(t *testing.T) {
	checks := &CrossRunChecks{
		SameCommitTree:       true,
		SameControllerPID:    true,
		SameControllerHash:   true,
		SameSchema:           false, // Failed
		SameThresholds:       true,
		SamePhaseConfig:      true,
		SameHostIdentity:     true,
		SameDockerIdentity:   true,
		SameImageIdentity:    true,
		SameCanaryBinary:     true,
		UniqueRunIDs:         true,
		UniqueSubjectProcesses: true,
		UniqueContainerIDs:   true,
		FixedOrder:           true,
		NonOverlapping:       true,
		CleanupComplete:      true,
	}

	total, passed, failed := CountCanonicalChecks(checks)
	if total != 16 {
		t.Errorf("expected total=16, got %d", total)
	}
	if passed != 15 {
		t.Errorf("expected passed=15, got %d", passed)
	}
	if failed != 1 {
		t.Errorf("expected failed=1, got %d", failed)
	}
}

// =============================================================================
// TEST 30: SameSchema reconstruction requires 3 runs
// =============================================================================

func TestReconstructSameSchema_RequiresThreeRuns(t *testing.T) {
	manifest := goodManifest()
	verifiedRuns := []*VerifiedRun{
		goodVerifiedRun(0, "canary-growing", "run-1"),
		// Only 1 run, not 3
	}

	_, err := ReconstructSameSchema(manifest, verifiedRuns)
	if err == nil {
		t.Error("expected error for not enough runs")
	}
}

// =============================================================================
// TEST 31: SameSchema reconstruction requires matching schemas
// =============================================================================

func TestReconstructSameSchema_RequiresMatchingSchemas(t *testing.T) {
	manifest := goodManifest()
	verifiedRuns := []*VerifiedRun{
		{
			DeclaredRunID:    "run-1",
			ActualManifest:   &evidence.Manifest{SchemaVersion: "1.1.0"},
		},
		{
			DeclaredRunID:    "run-2",
			ActualManifest:   &evidence.Manifest{SchemaVersion: "1.0.0"}, // Different!
		},
		{
			DeclaredRunID:    "run-3",
			ActualManifest:   &evidence.Manifest{SchemaVersion: "1.1.0"},
		},
	}

	// Function returns error when schemas don't match
	_, err := ReconstructSameSchema(manifest, verifiedRuns)
	if err == nil {
		t.Error("expected error for mismatched schemas")
	}
}

// =============================================================================
// TEST 32: SameSchema reconstruction requires declared schema to match
// =============================================================================

func TestReconstructSameSchema_RequiresDeclaredSchemaMatch(t *testing.T) {
	manifest := goodManifest()
	manifest.ExecutionIdentity.RunManifestSchemaVersion = "1.0.0" // Different from actual

	verifiedRuns := []*VerifiedRun{
		{
			DeclaredRunID:    "run-1",
			ActualManifest:   &evidence.Manifest{SchemaVersion: "1.1.0"},
		},
		{
			DeclaredRunID:    "run-2",
			ActualManifest:   &evidence.Manifest{SchemaVersion: "1.1.0"},
		},
		{
			DeclaredRunID:    "run-3",
			ActualManifest:   &evidence.Manifest{SchemaVersion: "1.1.0"},
		},
	}

	// Function returns error when declared schema doesn't match
	_, err := ReconstructSameSchema(manifest, verifiedRuns)
	if err == nil {
		t.Error("expected error when declared schema doesn't match actual")
	}
}

// =============================================================================
// TEST 33: ProcessCleanupStatus string mapping
// =============================================================================

func TestProcessCleanupStatusString_MapsCorrectly(t *testing.T) {
	tests := []struct {
		status   ProcessCleanupStatus
		expected string
	}{
		{ProcessGone, "gone"},
		{ProcessPIDReused, "pid_reused"},
		{ProcessStillAlive, "still_alive"},
		{ProcessUnavailable, "unavailable"},
		{ProcessCleanupStatus(99), "unknown"},
	}

	for _, tc := range tests {
		result := processCleanupStatusString(tc.status)
		if result != tc.expected {
			t.Errorf("status %d: expected %q, got %q", tc.status, tc.expected, result)
		}
	}
}

// =============================================================================
// TEST 34: FormatVerdictDiffs returns empty for no diffs
// =============================================================================

func TestFormatVerdictDiffs_EmptyForNoDiffs(t *testing.T) {
	result := FormatVerdictDiffs([]VerdictDiff{})
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

// =============================================================================
// TEST 35: FormatVerdictDiffs formats correctly
// =============================================================================

func TestFormatVerdictDiffs_FormatsCorrectly(t *testing.T) {
	diffs := []VerdictDiff{
		{Path: "matrix_valid", Stored: "true", Reconstructed: "false"},
		{Path: "checks_total", Stored: "16", Reconstructed: "15"},
	}

	result := FormatVerdictDiffs(diffs)
	if result == "" {
		t.Error("expected non-empty result")
	}

	// Should contain both differences
	if !strings.Contains(result, "matrix_valid") {
		t.Error("expected output to contain matrix_valid")
	}
	if !strings.Contains(result, "checks_total") {
		t.Error("expected output to contain checks_total")
	}
}
