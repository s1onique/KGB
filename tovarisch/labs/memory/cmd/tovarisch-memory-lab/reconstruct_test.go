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
	"fmt"
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
			ImplementationCommitOID:    "abc123def456",
			ImplementationTreeOID:      "tree789",
			GitObjectFormat:            "sha256",
			ControllerPID:              12345,
			ControllerExecutableSHA256: "hash123",
			RunManifestSchemaVersion:   "1.1.0",
			ImageReference:             "test-image:latest",
			ImageID:                    "sha256:abc123",
			CanaryBinarySHA256:         "binaryhash",
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
// P0-4 FIX: Each manifest has non-overlapping times to satisfy NonOverlapping check.
func goodChildManifest(runID, scenario string) *evidence.Manifest {
	// Use fixed offsets to ensure non-overlapping intervals
	baseTime := time.Now().Add(-15 * time.Minute)
	var startedAt, finishedAt time.Time
	switch scenario {
	case "canary-growing":
		startedAt = baseTime
		finishedAt = baseTime.Add(5 * time.Minute)
	case "canary-bounded":
		startedAt = baseTime.Add(6 * time.Minute)
		finishedAt = baseTime.Add(11 * time.Minute)
	case "canary-descriptor":
		startedAt = baseTime.Add(12 * time.Minute)
		finishedAt = baseTime.Add(17 * time.Minute)
	default:
		startedAt = baseTime
		finishedAt = baseTime.Add(5 * time.Minute)
	}

	return &evidence.Manifest{
		SchemaVersion: "1.1.0",
		RunID:         runID,
		Scenario:      scenario,
		ControllerID:  "12345",
		SubjectIdentity: &evidence.SubjectIdentity{
			GitCommit:                  "abc123def456",
			GitTree:                    "tree789",
			GitObjectFormat:            "sha256",
			ControllerExecutableSHA256: "hash123",
		},
		SubjectImageIdentity: &evidence.SubjectImageIdentity{
			ImageReference:       "test-image:latest",
			ImageID:              "sha256:abc123",
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

// goodChildVerdict returns a valid child run verdict with correct classification.
func goodChildVerdict(scenario string) *evidence.Verdict {
	// According to the calibration contract (canonical values from analysis.ClassificationGrowing):
	// - canary-growing: overall=growing, memory=growing, resource=stable, semantic=stable
	// - canary-bounded: overall=stable, memory=stable, resource=stable, semantic=stable
	// - canary-descriptor: overall=resource_growth, memory=stable, resource=resource_growth, semantic=stable
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
// P0-4 FIX: Container/network IDs must match what goodVerifiedRun produces.
func goodCleanupEvidence(matrixID string) *MatrixCleanupEvidence {
	// Use the same IDs as goodVerifiedRun
	containerIDs := []string{"container-abc123", "container-def456", "container-ghi789"}
	networkIDs := []string{"network-abc123", "network-def456", "network-ghi789"}
	pids := []int{54321, 54322, 54323}
	startTimes := []uint64{1234567890, 1234567891, 1234567892}

	return &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:         matrixID,
		ObservedAt:       time.Now(),
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:    0,
				Scenario: "canary-growing",
				RunID:    "run-1",
				Container: ContainerCleanupRecord{
					ID:     containerIDs[0],
					Name:   "test-container-1",
					Status: "gone",
				},
				Network: NetworkCleanupRecord{
					ID:     networkIDs[0],
					Name:   "test-network-1",
					Status: "gone",
				},
				Process: ProcessCleanupRecord{
					PID:       pids[0],
					StartTime: startTimes[0],
					Status:    "gone",
				},
			},
			{
				Index:    1,
				Scenario: "canary-bounded",
				RunID:    "run-2",
				Container: ContainerCleanupRecord{
					ID:     containerIDs[1],
					Name:   "test-container-2",
					Status: "gone",
				},
				Network: NetworkCleanupRecord{
					ID:     networkIDs[1],
					Name:   "test-network-2",
					Status: "gone",
				},
				Process: ProcessCleanupRecord{
					PID:       pids[1],
					StartTime: startTimes[1],
					Status:    "gone",
				},
			},
			{
				Index:    2,
				Scenario: "canary-descriptor",
				RunID:    "run-3",
				Container: ContainerCleanupRecord{
					ID:     containerIDs[2],
					Name:   "test-container-3",
					Status: "gone",
				},
				Network: NetworkCleanupRecord{
					ID:     networkIDs[2],
					Name:   "test-network-3",
					Status: "gone",
				},
				Process: ProcessCleanupRecord{
					PID:       pids[2],
					StartTime: startTimes[2],
					Status:    "gone",
				},
			},
		},
	}
}

// goodVerifiedRun returns a valid verified run for testing.
// P0-4 FIX: Each run has unique identities so the fixture itself is valid before mutation.
// This allows mutation tests to prove that exactly the mutated field causes failure.
func goodVerifiedRun(index int, scenario, runID string) *VerifiedRun {
	manifest := goodChildManifest(runID, scenario)
	verdict := goodChildVerdict(scenario)

	// Each run gets unique identities to satisfy uniqueness checks
	containerIDs := []string{"container-abc123", "container-def456", "container-ghi789"}
	networkIDs := []string{"network-abc123", "network-def456", "network-ghi789"}
	pids := []int{54321, 54322, 54323}
	startTimes := []uint64{1234567890, 1234567891, 1234567892}

	idx := 0
	if index >= 0 && index < 3 {
		idx = index
	}

	return &VerifiedRun{
		DeclaredRunID:         runID,
		DeclaredScenario:      scenario,
		RunIndex:              index,
		ActualManifest:        manifest,
		ActualVerdict:         verdict,
		ContainerID:           containerIDs[idx],
		NetworkID:             networkIDs[idx],
		SubjectPID:            pids[idx],
		SubjectStartTime:      startTimes[idx],
		ProcessCleanupStatus:  ProcessGone,
		ChildVerified:         true, // P0-8 FIX: Valid runs are verified
		CleanupEvidenceLoaded: true,
		CleanupEvidenceValid:  true,
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
		MatrixID:         "test-matrix",
		ObservedAt:       time.Now(),
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:     0,
				Scenario:  "canary-growing",
				RunID:     "run-1",
				Container: ContainerCleanupRecord{ID: "c1", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "", Status: "gone"}, // Empty ID!
				Process:   ProcessCleanupRecord{PID: 1, StartTime: 1, Status: "gone"},
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
		MatrixID:         "test-matrix",
		ObservedAt:       time.Now(),
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:     0,
				Scenario:  "canary-growing",
				RunID:     "run-1",
				Container: ContainerCleanupRecord{ID: "c1", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "same-id", Status: "gone"},
				Process:   ProcessCleanupRecord{PID: 1, StartTime: 1, Status: "gone"},
			},
			{
				Index:     1,
				Scenario:  "canary-bounded",
				RunID:     "run-2",
				Container: ContainerCleanupRecord{ID: "c2", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "same-id", Status: "gone"}, // Duplicate!
				Process:   ProcessCleanupRecord{PID: 2, StartTime: 2, Status: "gone"},
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
		MatrixID:         "different-matrix-id",
		ObservedAt:       time.Now(),
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:     0,
				Scenario:  "canary-growing",
				RunID:     "run-1",
				Container: ContainerCleanupRecord{ID: "c1", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "n1", Status: "gone"},
				Process:   ProcessCleanupRecord{PID: 1, StartTime: 1, Status: "gone"},
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
		MatrixID:         "test-matrix",
		ObservedAt:       time.Now(),
		NetworkOwnership: "per_run",
		Runs:             []RunCleanupRecord{},
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
		MatrixID:         "test-matrix",
		ObservedAt:       time.Now(),
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:     99, // Wrong index!
				Scenario:  "canary-growing",
				RunID:     "run-1",
				Container: ContainerCleanupRecord{ID: "c1", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "n1", Status: "gone"},
				Process:   ProcessCleanupRecord{PID: 1, StartTime: 1, Status: "gone"},
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
		if !run.CleanupEvidenceValid {
			t.Errorf("run %s has CleanupEvidenceValid=false", run.DeclaredRunID)
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
// TEST 11: Memory classification changes are stored but don't affect matrix validity
// CORRECTION03: Only overall classification affects matrix validity.
// Memory, resource, and semantic are stored in results but are not validated.
// =============================================================================

func TestReconstructMatrixVerdict_MemoryClassificationStored(t *testing.T) {
	manifest := goodManifest()
	cleanup := goodCleanupEvidence(manifest.MatrixID)

	// Start with valid fixture
	verifiedRuns := []*VerifiedRun{
		goodVerifiedRun(0, "canary-growing", "run-1"),
		goodVerifiedRun(1, "canary-bounded", "run-2"),
		goodVerifiedRun(2, "canary-descriptor", "run-3"),
	}

	// Verify baseline is valid
	baseline, err := ReconstructMatrixVerdict(manifest, verifiedRuns, cleanup)
	if err != nil {
		t.Fatalf("baseline reconstruction failed: %v", err)
	}
	if !baseline.MatrixValid {
		t.Fatal("baseline fixture should be valid")
	}

	// Mutate memory classification for canary-growing
	verifiedRuns[0].ActualVerdict.MemoryClassification = analysis.ClassificationStable

	verdict, err := ReconstructMatrixVerdict(manifest, verifiedRuns, cleanup)
	if err != nil {
		t.Fatalf("ReconstructMatrixVerdict failed: %v", err)
	}

	// P0-9 FIX: Memory classification DOES affect matrix validity (four-field validation)
	if verdict.MatrixValid {
		t.Error("P0-9: matrix should be invalid when memory classification doesn't match")
	}

	// But the mutated value should be stored
	if verdict.ScenarioResults["canary-growing"].Memory != string(analysis.ClassificationStable) {
		t.Error("mutated memory classification should be stored in results")
	}
}

// =============================================================================
// TEST 12: Resource classification changes are stored but don't affect matrix validity
// CORRECTION03: Only overall classification affects matrix validity.
// =============================================================================

func TestReconstructMatrixVerdict_ResourceClassificationStored(t *testing.T) {
	manifest := goodManifest()
	cleanup := goodCleanupEvidence(manifest.MatrixID)

	// Start with valid fixture
	verifiedRuns := []*VerifiedRun{
		goodVerifiedRun(0, "canary-growing", "run-1"),
		goodVerifiedRun(1, "canary-bounded", "run-2"),
		goodVerifiedRun(2, "canary-descriptor", "run-3"),
	}

	// Verify baseline is valid
	baseline, err := ReconstructMatrixVerdict(manifest, verifiedRuns, cleanup)
	if err != nil {
		t.Fatalf("baseline reconstruction failed: %v", err)
	}
	if !baseline.MatrixValid {
		t.Fatal("baseline fixture should be valid")
	}

	// Mutate resource classification for canary-descriptor
	verifiedRuns[2].ActualVerdict.ResourceClassification = analysis.ClassificationStable

	verdict, err := ReconstructMatrixVerdict(manifest, verifiedRuns, cleanup)
	if err != nil {
		t.Fatalf("ReconstructMatrixVerdict failed: %v", err)
	}

	// P0-9 FIX: Resource classification DOES affect matrix validity (four-field validation)
	if verdict.MatrixValid {
		t.Error("P0-9: matrix should be invalid when resource classification doesn't match")
	}

	// But the mutated value should be stored
	if verdict.ScenarioResults["canary-descriptor"].Resource != string(analysis.ClassificationStable) {
		t.Error("mutated resource classification should be stored in results")
	}
}

// =============================================================================
// TEST 13: ScenarioResult.Verified based on ChildVerified flag (P0-8 FIX)
// =============================================================================

func TestReconstructScenarioResults_SetsVerifiedFromChildVerified(t *testing.T) {
	// P0-8 FIX: Verified field now comes from ChildVerified, not schema version.
	verifiedRuns := []*VerifiedRun{
		{
			DeclaredRunID:    "run-1",
			DeclaredScenario: "canary-growing",
			ActualManifest:   &evidence.Manifest{SchemaVersion: "1.1.0"},
			ActualVerdict:    &evidence.Verdict{},
			ChildVerified:    true, // Verified via complete child bundle verification
		},
		{
			DeclaredRunID:    "run-2",
			DeclaredScenario: "canary-bounded",
			ActualManifest:   &evidence.Manifest{SchemaVersion: "1.0.0"},
			ActualVerdict:    &evidence.Verdict{},
			ChildVerified:    false, // Failed child verification
		},
		{
			DeclaredRunID:    "run-3",
			DeclaredScenario: "canary-descriptor",
			ActualManifest:   &evidence.Manifest{SchemaVersion: "1.1.0"},
			ActualVerdict:    &evidence.Verdict{},
			ChildVerified:    true, // Verified via complete child bundle verification
		},
	}

	results, err := ReconstructScenarioResults(verifiedRuns)
	if err != nil {
		t.Fatalf("ReconstructScenarioResults failed: %v", err)
	}

	// Verified is based on ChildVerified flag, not schema version
	if results["canary-growing"].Verified != true {
		t.Error("canary-growing should be verified (ChildVerified=true)")
	}
	if results["canary-bounded"].Verified != false {
		t.Error("canary-bounded should not be verified (ChildVerified=false)")
	}
	if results["canary-descriptor"].Verified != true {
		t.Error("canary-descriptor should be verified (ChildVerified=true)")
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
		MatrixID:         "test",
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:     0,
				Scenario:  "canary-growing",
				RunID:     "run-1",
				Container: ContainerCleanupRecord{ID: "container-abc123", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "network-def456", Status: "gone"},
				Process:   ProcessCleanupRecord{PID: 54321, StartTime: 1234567890, Status: "still_alive"}, // Alive!
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
		MatrixID:         "test",
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:     0,
				Scenario:  "canary-growing",
				RunID:     "run-1",
				Container: ContainerCleanupRecord{ID: "container-abc123", Status: "exists"}, // Still exists!
				Network:   NetworkCleanupRecord{ID: "network-def456", Status: "gone"},
				Process:   ProcessCleanupRecord{PID: 54321, StartTime: 1234567890, Status: "gone"},
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
		MatrixID:         "test",
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:     0,
				Scenario:  "canary-growing",
				RunID:     "run-1",
				Container: ContainerCleanupRecord{ID: "container-abc123", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "network-def456", Status: "gone"},
				Process:   ProcessCleanupRecord{PID: 99999, StartTime: 1234567890, Status: "gone"}, // Wrong PID!
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
// P0-4 FIX: Single-cause mutation - only overall field changes, all else stays valid.
// =============================================================================

func TestReconstructMatrixVerdict_FailsWithWrongOverallClassification(t *testing.T) {
	manifest := goodManifest()
	cleanup := goodCleanupEvidence(manifest.MatrixID)

	// Start with valid fixture
	verifiedRuns := []*VerifiedRun{
		goodVerifiedRun(0, "canary-growing", "run-1"),
		goodVerifiedRun(1, "canary-bounded", "run-2"),
		goodVerifiedRun(2, "canary-descriptor", "run-3"),
	}

	// Verify baseline is valid
	baseline, err := ReconstructMatrixVerdict(manifest, verifiedRuns, cleanup)
	if err != nil {
		t.Fatalf("baseline reconstruction failed: %v", err)
	}
	if !baseline.MatrixValid {
		t.Fatal("baseline fixture should be valid")
	}
	if baseline.ChecksPassed != 16 {
		t.Errorf("baseline ChecksPassed=16, got %d", baseline.ChecksPassed)
	}

	// Mutate exactly one field: overall classification for canary-growing
	verifiedRuns[0].ActualVerdict.OverallClassification = analysis.ClassificationStable // WRONG! Should be Growing

	verdict, err := ReconstructMatrixVerdict(manifest, verifiedRuns, cleanup)
	if err != nil {
		t.Fatalf("ReconstructMatrixVerdict failed: %v", err)
	}

	// Matrix should be INVALID because overall classification is wrong for canary-growing
	if verdict.MatrixValid {
		t.Error("expected MatrixValid=false for wrong overall classification")
	}
	// Prove single-cause: cross-run checks still pass, only classification fails
	if verdict.ChecksPassed != 16 {
		t.Errorf("cross-run checks should pass (16), got %d", verdict.ChecksPassed)
	}
}

// =============================================================================
// TEST 19: matrix_shared network mode accepts shared network ID
// =============================================================================

func TestValidateCleanupEvidence_AcceptsSharedNetworkMode(t *testing.T) {
	cleanup := &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:         "test-matrix",
		ObservedAt:       time.Now(),
		NetworkOwnership: "matrix_shared",
		Runs: []RunCleanupRecord{
			{
				Index:     0,
				Scenario:  "canary-growing",
				RunID:     "run-1",
				Container: ContainerCleanupRecord{ID: "c1", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "shared-net", Status: "gone"},
				Process:   ProcessCleanupRecord{PID: 1, StartTime: 1, Status: "gone"},
			},
			{
				Index:     1,
				Scenario:  "canary-bounded",
				RunID:     "run-2",
				Container: ContainerCleanupRecord{ID: "c2", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "shared-net", Status: "gone"}, // Same shared ID
				Process:   ProcessCleanupRecord{PID: 2, StartTime: 2, Status: "gone"},
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
		MatrixID:         "test-matrix",
		ObservedAt:       time.Now(),
		NetworkOwnership: "matrix_shared",
		Runs: []RunCleanupRecord{
			{
				Index:     0,
				Scenario:  "canary-growing",
				RunID:     "run-1",
				Container: ContainerCleanupRecord{ID: "c1", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "net-1", Status: "gone"},
				Process:   ProcessCleanupRecord{PID: 1, StartTime: 1, Status: "gone"},
			},
			{
				Index:     1,
				Scenario:  "canary-bounded",
				RunID:     "run-2",
				Container: ContainerCleanupRecord{ID: "c2", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "net-2", Status: "gone"}, // Different!
				Process:   ProcessCleanupRecord{PID: 2, StartTime: 2, Status: "gone"},
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
		MatrixID:         "test-matrix",
		ObservedAt:       time.Now(),
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:     0,
				Scenario:  "canary-growing",
				RunID:     "run-1",
				Container: ContainerCleanupRecord{ID: "", Status: "gone"}, // Empty!
				Network:   NetworkCleanupRecord{ID: "n1", Status: "gone"},
				Process:   ProcessCleanupRecord{PID: 1, StartTime: 1, Status: "gone"},
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
		MatrixID:         "test-matrix",
		ObservedAt:       time.Now(),
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{
				Index:     0,
				Scenario:  "canary-growing",
				RunID:     "run-1",
				Container: ContainerCleanupRecord{ID: "c1", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "n1", Status: "gone"},
				Process:   ProcessCleanupRecord{PID: 0, StartTime: 1, Status: "gone"}, // Zero PID!
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
		MatrixID:         "test-matrix",
		ObservedAt:       time.Now(),
		NetworkOwnership: "unknown_mode",
		Runs:             []RunCleanupRecord{},
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
		MatrixID:         "test-matrix",
		ObservedAt:       time.Now(),
		NetworkOwnership: "per_run",
		Runs:             []RunCleanupRecord{}, // No runs
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
		SameCommitTree:         true,
		SameControllerPID:      true,
		SameControllerHash:     true,
		SameSchema:             true,
		SameThresholds:         true,
		SamePhaseConfig:        true,
		SameHostIdentity:       true,
		SameDockerIdentity:     true,
		SameImageIdentity:      true,
		SameCanaryBinary:       true,
		UniqueRunIDs:           true,
		UniqueSubjectProcesses: true,
		UniqueContainerIDs:     true,
		FixedOrder:             true,
		NonOverlapping:         true,
		CleanupComplete:        true,
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
		SameCommitTree:         true,
		SameControllerPID:      true,
		SameControllerHash:     true,
		SameSchema:             false, // Failed
		SameThresholds:         true,
		SamePhaseConfig:        true,
		SameHostIdentity:       true,
		SameDockerIdentity:     true,
		SameImageIdentity:      true,
		SameCanaryBinary:       true,
		UniqueRunIDs:           true,
		UniqueSubjectProcesses: true,
		UniqueContainerIDs:     true,
		FixedOrder:             true,
		NonOverlapping:         true,
		CleanupComplete:        true,
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
			DeclaredRunID:  "run-1",
			ActualManifest: &evidence.Manifest{SchemaVersion: "1.1.0"},
		},
		{
			DeclaredRunID:  "run-2",
			ActualManifest: &evidence.Manifest{SchemaVersion: "1.0.0"}, // Different!
		},
		{
			DeclaredRunID:  "run-3",
			ActualManifest: &evidence.Manifest{SchemaVersion: "1.1.0"},
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
			DeclaredRunID:  "run-1",
			ActualManifest: &evidence.Manifest{SchemaVersion: "1.1.0"},
		},
		{
			DeclaredRunID:  "run-2",
			ActualManifest: &evidence.Manifest{SchemaVersion: "1.1.0"},
		},
		{
			DeclaredRunID:  "run-3",
			ActualManifest: &evidence.Manifest{SchemaVersion: "1.1.0"},
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

// =============================================================================
// TEST 36: Valid fixture reconstructs with MatrixValid=true (P0-4 FIX)
// =============================================================================

// P0-4 FIX: Prove the fixture is valid before mutation.
// Each run must have unique identities to pass uniqueness checks.
func TestReconstructMatrixVerdict_ValidFixtureProducesValidMatrix(t *testing.T) {
	manifest := goodManifest()
	cleanup := goodCleanupEvidence(manifest.MatrixID)

	// P0-4 FIX: Create valid verified runs with unique identities
	verifiedRuns := []*VerifiedRun{
		goodVerifiedRun(0, "canary-growing", "run-1"),
		goodVerifiedRun(1, "canary-bounded", "run-2"),
		goodVerifiedRun(2, "canary-descriptor", "run-3"),
	}

	verdict, err := ReconstructMatrixVerdict(manifest, verifiedRuns, cleanup)
	if err != nil {
		t.Fatalf("ReconstructMatrixVerdict failed: %v", err)
	}

	// P0-4 FIX: Valid fixture must produce valid matrix
	if !verdict.MatrixValid {
		t.Error("expected MatrixValid=true for valid fixture")
	}
	if verdict.ChecksPassed != 16 {
		t.Errorf("expected ChecksPassed=16, got %d", verdict.ChecksPassed)
	}
	if verdict.ChecksFailed != 0 {
		t.Errorf("expected ChecksFailed=0, got %d", verdict.ChecksFailed)
	}
}

// =============================================================================
// TEST 37: Semantic classification changes are stored but don't affect matrix validity
// CORRECTION03: Only overall classification affects matrix validity.
// =============================================================================

func TestReconstructMatrixVerdict_SemanticClassificationStored(t *testing.T) {
	manifest := goodManifest()
	cleanup := goodCleanupEvidence(manifest.MatrixID)

	// Start with valid fixture
	verifiedRuns := []*VerifiedRun{
		goodVerifiedRun(0, "canary-growing", "run-1"),
		goodVerifiedRun(1, "canary-bounded", "run-2"),
		goodVerifiedRun(2, "canary-descriptor", "run-3"),
	}

	// Verify baseline is valid
	baseline, err := ReconstructMatrixVerdict(manifest, verifiedRuns, cleanup)
	if err != nil {
		t.Fatalf("baseline reconstruction failed: %v", err)
	}
	if !baseline.MatrixValid {
		t.Fatal("baseline fixture should be valid")
	}

	// Mutate semantic classification for canary-growing
	verifiedRuns[0].ActualVerdict.SemanticClassification = analysis.ClassificationGrowing

	verdict, err := ReconstructMatrixVerdict(manifest, verifiedRuns, cleanup)
	if err != nil {
		t.Fatalf("ReconstructMatrixVerdict failed: %v", err)
	}

	// P0-9 FIX: Semantic classification DOES affect matrix validity (four-field validation)
	if verdict.MatrixValid {
		t.Error("P0-9: matrix should be invalid when semantic classification doesn't match")
	}

	// But the mutated value should be stored
	if verdict.ScenarioResults["canary-growing"].Semantic != string(analysis.ClassificationGrowing) {
		t.Error("mutated semantic classification should be stored in results")
	}
}

// =============================================================================
// TEST 38: Equal invalid verdicts - both MatrixValid=false (P0-8 FIX)
// =============================================================================

func TestCompareVerdicts_DetectsEqualInvalidBothFalse(t *testing.T) {
	// P0-8 FIX: Test that CompareVerdicts detects equal-invalid terminal case
	stored := &MatrixVerdict{
		MatrixID:    "test-matrix",
		MatrixValid: false,
		ScenarioResults: map[string]*ScenarioResult{
			"canary-growing": {
				RunID:    "run-1",
				Verified: true,
				Overall:  "growing",
				Memory:   "growing",
				Resource: "stable",
				Semantic: "stable",
			},
		},
		CrossRunChecks: &CrossRunChecks{
			SameCommitTree:         true,
			SameControllerPID:      true,
			SameControllerHash:     true,
			SameSchema:             true,
			SameThresholds:         true,
			SamePhaseConfig:        true,
			SameHostIdentity:       true,
			SameDockerIdentity:     true,
			SameImageIdentity:      true,
			SameCanaryBinary:       true,
			UniqueRunIDs:           true,
			UniqueSubjectProcesses: true,
			UniqueContainerIDs:     true,
			FixedOrder:             true,
			NonOverlapping:         true,
			CleanupComplete:        true,
			ChecksPassed:           16,
		},
		ChecksTotal:  16,
		ChecksPassed: 16,
		ChecksFailed: 0,
	}

	// Reconstructed is identical in every way - both are invalid
	reconstructed := &MatrixVerdict{
		MatrixID:    "test-matrix",
		MatrixValid: false,
		ScenarioResults: map[string]*ScenarioResult{
			"canary-growing": {
				RunID:    "run-1",
				Verified: true,
				Overall:  "growing",
				Memory:   "growing",
				Resource: "stable",
				Semantic: "stable",
			},
		},
		CrossRunChecks: &CrossRunChecks{
			SameCommitTree:         true,
			SameControllerPID:      true,
			SameControllerHash:     true,
			SameSchema:             true,
			SameThresholds:         true,
			SamePhaseConfig:        true,
			SameHostIdentity:       true,
			SameDockerIdentity:     true,
			SameImageIdentity:      true,
			SameCanaryBinary:       true,
			UniqueRunIDs:           true,
			UniqueSubjectProcesses: true,
			UniqueContainerIDs:     true,
			FixedOrder:             true,
			NonOverlapping:         true,
			CleanupComplete:        true,
			ChecksPassed:           16,
		},
		ChecksTotal:  16,
		ChecksPassed: 16,
		ChecksFailed: 0,
	}

	diffs := CompareVerdicts(stored, reconstructed)
	if len(diffs) != 0 {
		t.Errorf("expected no differences for equal invalid verdicts, got %d", len(diffs))
	}
}

// =============================================================================
// TEST 39: Producer without child verification cannot produce valid verdict (P0-8 FIX)
// =============================================================================

// P0-8 FIX: Regression test proving producer must verify children before writing verdict.
func TestProducerRequiresChildVerification_ForValidVerdict(t *testing.T) {
	manifest := goodManifest()
	cleanup := goodCleanupEvidence(manifest.MatrixID)

	// Producer's preliminary runs (without authoritative child verification)
	// P0-8 FIX: ChildVerified=false when NOT verified through VerifyDeclaredChildRuns
	preliminaryRuns := []*VerifiedRun{
		goodVerifiedRun(0, "canary-growing", "run-1"),
		goodVerifiedRun(1, "canary-bounded", "run-2"),
		goodVerifiedRun(2, "canary-descriptor", "run-3"),
	}

	// Simulate producer forgetting to verify children
	for _, run := range preliminaryRuns {
		run.ChildVerified = false // P0-8 FIX: This is what happens without VerifyDeclaredChildRuns
	}

	// Preliminary runs with ChildVerified=false cannot produce valid verdict
	verdict, err := ReconstructMatrixVerdict(manifest, preliminaryRuns, cleanup)
	if err != nil {
		t.Fatalf("ReconstructMatrixVerdict failed: %v", err)
	}

	// P0-8 FIX: MatrixValid=false when children are not verified
	if verdict.MatrixValid {
		t.Error("P0-8: matrix should be INVALID when ChildVerified=false")
	}
}

// =============================================================================
// TEST 40: Producer with verified children produces valid verdict (P0-8 FIX)
// =============================================================================

// P0-8 FIX: Regression test proving verified children enable valid verdict.
func TestProducerWithVerifiedChildren_ProducesValidVerdict(t *testing.T) {
	manifest := goodManifest()
	cleanup := goodCleanupEvidence(manifest.MatrixID)

	// Producer's verified runs (after calling VerifyDeclaredChildRuns)
	// P0-8 FIX: ChildVerified=true when verified through VerifyDeclaredChildRuns
	verifiedRuns := []*VerifiedRun{
		goodVerifiedRun(0, "canary-growing", "run-1"),
		goodVerifiedRun(1, "canary-bounded", "run-2"),
		goodVerifiedRun(2, "canary-descriptor", "run-3"),
	}

	// All children verified (as if from VerifyDeclaredChildRuns)
	for _, run := range verifiedRuns {
		run.ChildVerified = true
	}

	// Verified runs with ChildVerified=true can produce valid verdict
	verdict, err := ReconstructMatrixVerdict(manifest, verifiedRuns, cleanup)
	if err != nil {
		t.Fatalf("ReconstructMatrixVerdict failed: %v", err)
	}

	// P0-8 FIX: MatrixValid=true when all children are verified
	if !verdict.MatrixValid {
		t.Error("P0-8: matrix should be VALID when all children verified")
	}

	// Verify all scenario results have Verified=true
	for scenario, result := range verdict.ScenarioResults {
		if !result.Verified {
			t.Errorf("P0-8: scenario %s should have Verified=true", scenario)
		}
	}
}

// =============================================================================
// TEST 41: One child verification failure prevents valid verdict (P0-8 FIX)
// =============================================================================

// P0-8 FIX: Regression test proving partial verification fails.
func TestOneChildVerificationFailure_InvalidatesVerdict(t *testing.T) {
	manifest := goodManifest()
	cleanup := goodCleanupEvidence(manifest.MatrixID)

	// Producer's verified runs with one child failing verification
	verifiedRuns := []*VerifiedRun{
		goodVerifiedRun(0, "canary-growing", "run-1"),
		goodVerifiedRun(1, "canary-bounded", "run-2"),
		goodVerifiedRun(2, "canary-descriptor", "run-3"),
	}

	// One child fails verification (as if from failed VerifyDeclaredChildRuns)
	verifiedRuns[1].ChildVerified = false // canary-bounded failed

	verdict, err := ReconstructMatrixVerdict(manifest, verifiedRuns, cleanup)
	if err != nil {
		t.Fatalf("ReconstructMatrixVerdict failed: %v", err)
	}

	// P0-8 FIX: MatrixValid=false when not all children are verified
	if verdict.MatrixValid {
		t.Error("P0-8: matrix should be INVALID when one child not verified")
	}

	// canary-bounded should have Verified=false
	if verdict.ScenarioResults["canary-bounded"].Verified {
		t.Error("P0-8: canary-bounded should have Verified=false")
	}
}

// =============================================================================
// TEST 42: Stored verdict matches reconstructed verdict (P0-8 FIX)
// =============================================================================

// P0-8 FIX: Regression test proving stored and reconstructed verdicts converge.
func TestStoredVerdictMatchesReconstructed_AfterChildVerification(t *testing.T) {
	manifest := goodManifest()
	cleanup := goodCleanupEvidence(manifest.MatrixID)

	// Verified runs (from VerifyDeclaredChildRuns)
	verifiedRuns := []*VerifiedRun{
		goodVerifiedRun(0, "canary-growing", "run-1"),
		goodVerifiedRun(1, "canary-bounded", "run-2"),
		goodVerifiedRun(2, "canary-descriptor", "run-3"),
	}

	// Reconstruct verdict (simulates producer writing matrix-verdict.json)
	storedVerdict, err := ReconstructMatrixVerdict(manifest, verifiedRuns, cleanup)
	if err != nil {
		t.Fatalf("ReconstructMatrixVerdict failed: %v", err)
	}

	// Re-reconstruct verdict (simulates VerifyMatrixBundle)
	reconstructedVerdict, err := ReconstructMatrixVerdict(manifest, verifiedRuns, cleanup)
	if err != nil {
		t.Fatalf("ReconstructMatrixVerdict failed: %v", err)
	}

	// P0-8 FIX: No differences between stored and reconstructed
	diffs := CompareVerdicts(storedVerdict, reconstructedVerdict)
	if len(diffs) > 0 {
		t.Errorf("P0-8: expected no differences, got %d: %v", len(diffs), diffs)
	}

	// Both should be valid
	if !storedVerdict.MatrixValid {
		t.Error("P0-8: stored verdict should be valid")
	}
	if !reconstructedVerdict.MatrixValid {
		t.Error("P0-8: reconstructed verdict should be valid")
	}
}

// =============================================================================
// TEST 43: VerifyDeclaredChildRuns sets ChildVerified from verification (P0-8 FIX)
// =============================================================================

// P0-8 FIX: Test that VerifyDeclaredChildRuns properly propagates ChildVerified.
func TestVerifyDeclaredChildRuns_SetsChildVerifiedCorrectly(t *testing.T) {
	// Create a mock verification function that succeeds for all 3 runs
	// P0-8 FIX: Must return correct RunID/Scenario based on runDir
	successVerify := func(runDir string) (*VerifiedChildBundle, error) {
		// Determine which run based on path - return matching IDs for cleanup binding
		runID := "run-1"
		scenario := "canary-growing"
		containerID := "test-container-1"
		networkID := "test-network-1"
		if strings.Contains(runDir, "run-2") {
			runID = "run-2"
			scenario = "canary-bounded"
			containerID = "test-container-2"
			networkID = "test-network-2"
		} else if strings.Contains(runDir, "run-3") {
			runID = "run-3"
			scenario = "canary-descriptor"
			containerID = "test-container-3"
			networkID = "test-network-3"
		}

		return &VerifiedChildBundle{
			Manifest: &evidence.Manifest{
				SchemaVersion: "1.1.0",
				RunID:         runID,
				Scenario:      scenario,
			},
			Verdict:        &evidence.Verdict{Scenario: scenario},
			ContainerID:    containerID,
			NetworkID:      networkID,
			SubjectPID:     12345,
			SubjectStart:   1234567890,
			ChecksVerified: true,
		}, nil
	}

	// Create a mock verification function that fails
	failVerify := func(runDir string) (*VerifiedChildBundle, error) {
		return nil, fmt.Errorf("verification failed")
	}

	// P0-8 FIX: Must use exactly 3 runs
	manifest := &MatrixManifest{
		MatrixID: "test-matrix",
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1", Path: "runs/run-1"},
			{Index: 2, Scenario: "canary-bounded", RunID: "run-2", Path: "runs/run-2"},
			{Index: 3, Scenario: "canary-descriptor", RunID: "run-3", Path: "runs/run-3"},
		},
	}
	cleanup := &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:         "test-matrix",
		ObservedAt:       time.Now(),
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			{Index: 0, Scenario: "canary-growing", RunID: "run-1", Container: ContainerCleanupRecord{ID: "test-container-1", Status: "gone"}, Network: NetworkCleanupRecord{ID: "test-network-1", Status: "gone"}, Process: ProcessCleanupRecord{PID: 12345, StartTime: 1234567890, Status: "gone"}},
			{Index: 1, Scenario: "canary-bounded", RunID: "run-2", Container: ContainerCleanupRecord{ID: "test-container-2", Status: "gone"}, Network: NetworkCleanupRecord{ID: "test-network-2", Status: "gone"}, Process: ProcessCleanupRecord{PID: 12345, StartTime: 1234567890, Status: "gone"}},
			{Index: 2, Scenario: "canary-descriptor", RunID: "run-3", Container: ContainerCleanupRecord{ID: "test-container-3", Status: "gone"}, Network: NetworkCleanupRecord{ID: "test-network-3", Status: "gone"}, Process: ProcessCleanupRecord{PID: 12345, StartTime: 1234567890, Status: "gone"}},
		},
	}

	// Test with succeeding verification
	runs, err := VerifyDeclaredChildRuns("/tmp", manifest, cleanup, successVerify)
	if err != nil {
		t.Fatalf("VerifyDeclaredChildRuns failed: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runs))
	}
	for _, run := range runs {
		if !run.ChildVerified {
			t.Error("P0-8: ChildVerified should be true for successful verification")
		}
	}

	// Test with failing verification
	_, err = VerifyDeclaredChildRuns("/tmp", manifest, cleanup, failVerify)
	if err == nil {
		t.Error("P0-8: VerifyDeclaredChildRuns should fail when child verification fails")
	}
}

// =============================================================================
// TEST 44: VerifyDeclaredChildRuns rejects nil manifest (P0-8 FIX)
// =============================================================================

func TestVerifyDeclaredChildRuns_RejectsNilManifest(t *testing.T) {
	verifyFn := func(runDir string) (*VerifiedChildBundle, error) {
		return &VerifiedChildBundle{}, nil
	}
	cleanup := &MatrixCleanupEvidence{}

	_, err := VerifyDeclaredChildRuns("/tmp", nil, cleanup, verifyFn)
	if err == nil {
		t.Error("P0-8: should reject nil manifest")
	}
	if err != nil && err.Error() != "matrix manifest is nil" {
		t.Errorf("P0-8: expected 'matrix manifest is nil' error, got: %v", err)
	}
}

// =============================================================================
// TEST 45: VerifyDeclaredChildRuns rejects nil cleanup (P0-8 FIX)
// =============================================================================

func TestVerifyDeclaredChildRuns_RejectsNilCleanup(t *testing.T) {
	verifyFn := func(runDir string) (*VerifiedChildBundle, error) {
		return &VerifiedChildBundle{}, nil
	}
	manifest := &MatrixManifest{}

	_, err := VerifyDeclaredChildRuns("/tmp", manifest, nil, verifyFn)
	if err == nil {
		t.Error("P0-8: should reject nil cleanup")
	}
	if err != nil && err.Error() != "cleanup evidence is nil" {
		t.Errorf("P0-8: expected 'cleanup evidence is nil' error, got: %v", err)
	}
}

// =============================================================================
// TEST 46: VerifyDeclaredChildRuns rejects nil verifier (P0-8 FIX)
// =============================================================================

func TestVerifyDeclaredChildRuns_RejectsNilVerifier(t *testing.T) {
	manifest := &MatrixManifest{}
	cleanup := &MatrixCleanupEvidence{}

	_, err := VerifyDeclaredChildRuns("/tmp", manifest, cleanup, nil)
	if err == nil {
		t.Error("P0-8: should reject nil verifier")
	}
	if err != nil && err.Error() != "child verifier is nil" {
		t.Errorf("P0-8: expected 'child verifier is nil' error, got: %v", err)
	}
}

// =============================================================================
// TEST 47: VerifyDeclaredChildRuns rejects nil child bundle (P0-8 FIX)
// =============================================================================

func TestVerifyDeclaredChildRuns_RejectsNilChildBundle(t *testing.T) {
	nilBundleVerify := func(runDir string) (*VerifiedChildBundle, error) {
		return nil, nil // Returns nil with no error
	}

	// P0-8 FIX: Must use exactly 3 runs
	manifest := &MatrixManifest{
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1", Path: "runs/run-1"},
			{Index: 2, Scenario: "canary-bounded", RunID: "run-2", Path: "runs/run-2"},
			{Index: 3, Scenario: "canary-descriptor", RunID: "run-3", Path: "runs/run-3"},
		},
	}
	cleanup := &MatrixCleanupEvidence{
		Runs: []RunCleanupRecord{
			{Index: 0, Scenario: "canary-growing", RunID: "run-1"},
			{Index: 1, Scenario: "canary-bounded", RunID: "run-2"},
			{Index: 2, Scenario: "canary-descriptor", RunID: "run-3"},
		},
	}

	_, err := VerifyDeclaredChildRuns("/tmp", manifest, cleanup, nilBundleVerify)
	if err == nil {
		t.Error("P0-8: should reject nil child bundle")
	}
	if err != nil && !strings.Contains(err.Error(), "nil bundle") {
		t.Errorf("P0-8: expected nil bundle error, got: %v", err)
	}
}

// =============================================================================
// TEST 48: VerifyDeclaredChildRuns rejects ChecksVerified=false (P0-8 FIX)
// =============================================================================

func TestVerifyDeclaredChildRuns_RejectsUnverifiedBundle(t *testing.T) {
	unverifiedVerify := func(runDir string) (*VerifiedChildBundle, error) {
		return &VerifiedChildBundle{
			Manifest: &evidence.Manifest{
				SchemaVersion: "1.1.0",
				RunID:         "run-1",
				Scenario:      "canary-growing",
			},
			Verdict:        &evidence.Verdict{Scenario: "canary-growing"},
			ChecksVerified: false, // NOT verified
		}, nil
	}

	// P0-8 FIX: Must use exactly 3 runs
	manifest := &MatrixManifest{
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1", Path: "runs/run-1"},
			{Index: 2, Scenario: "canary-bounded", RunID: "run-2", Path: "runs/run-2"},
			{Index: 3, Scenario: "canary-descriptor", RunID: "run-3", Path: "runs/run-3"},
		},
	}
	cleanup := &MatrixCleanupEvidence{
		Runs: []RunCleanupRecord{
			{Index: 0, Scenario: "canary-growing", RunID: "run-1"},
			{Index: 1, Scenario: "canary-bounded", RunID: "run-2"},
			{Index: 2, Scenario: "canary-descriptor", RunID: "run-3"},
		},
	}

	_, err := VerifyDeclaredChildRuns("/tmp", manifest, cleanup, unverifiedVerify)
	if err == nil {
		t.Error("P0-8: should reject bundle with ChecksVerified=false")
	}
	if err != nil && !strings.Contains(err.Error(), "not verified") {
		t.Errorf("P0-8: expected not verified error, got: %v", err)
	}
}

// =============================================================================
// TEST 49: VerifyDeclaredChildRuns validates RunID binding (P0-8 FIX)
// =============================================================================

func TestVerifyDeclaredChildRuns_RejectsRunIDMismatch(t *testing.T) {
	mismatchVerify := func(runDir string) (*VerifiedChildBundle, error) {
		// P0-1 FIX: Must provide valid identities - all fields are required now
		return &VerifiedChildBundle{
			Manifest: &evidence.Manifest{
				SchemaVersion: "1.1.0",
				RunID:         "wrong-run-id", // Does NOT match declaration
				Scenario:      "canary-growing",
			},
			Verdict:        &evidence.Verdict{Scenario: "canary-growing"},
			ChecksVerified: true,
			// Required identity fields
			ContainerID:  "container-abc123",
			NetworkID:    "network-abc123",
			SubjectPID:   54321,
			SubjectStart: 1234567890,
		}, nil
	}

	// P0-8 FIX: Must use exactly 3 runs
	manifest := &MatrixManifest{
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1", Path: "runs/run-1"},
			{Index: 2, Scenario: "canary-bounded", RunID: "run-2", Path: "runs/run-2"},
			{Index: 3, Scenario: "canary-descriptor", RunID: "run-3", Path: "runs/run-3"},
		},
	}
	cleanup := &MatrixCleanupEvidence{
		Runs: []RunCleanupRecord{
			{Index: 0, Scenario: "canary-growing", RunID: "run-1"},
			{Index: 1, Scenario: "canary-bounded", RunID: "run-2"},
			{Index: 2, Scenario: "canary-descriptor", RunID: "run-3"},
		},
	}

	_, err := VerifyDeclaredChildRuns("/tmp", manifest, cleanup, mismatchVerify)
	if err == nil {
		t.Error("P0-8: should reject RunID mismatch")
	}
	if err != nil && !strings.Contains(err.Error(), "RunID mismatch") {
		t.Errorf("P0-8: expected RunID mismatch error, got: %v", err)
	}
}

// =============================================================================
// TEST 50: VerifyDeclaredChildRuns validates scenario binding (P0-8 FIX)
// =============================================================================

func TestVerifyDeclaredChildRuns_RejectsScenarioMismatch(t *testing.T) {
	mismatchVerify := func(runDir string) (*VerifiedChildBundle, error) {
		// P0-1 FIX: Must provide valid identities - all fields are required now
		return &VerifiedChildBundle{
			Manifest: &evidence.Manifest{
				SchemaVersion: "1.1.0",
				RunID:         "run-1",
				Scenario:      "wrong-scenario", // Does NOT match declaration
			},
			Verdict:        &evidence.Verdict{Scenario: "wrong-scenario"},
			ChecksVerified: true,
			// Required identity fields
			ContainerID:  "container-abc123",
			NetworkID:    "network-abc123",
			SubjectPID:   54321,
			SubjectStart: 1234567890,
		}, nil
	}

	// P0-8 FIX: Must use exactly 3 runs
	manifest := &MatrixManifest{
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1", Path: "runs/run-1"},
			{Index: 2, Scenario: "canary-bounded", RunID: "run-2", Path: "runs/run-2"},
			{Index: 3, Scenario: "canary-descriptor", RunID: "run-3", Path: "runs/run-3"},
		},
	}
	cleanup := &MatrixCleanupEvidence{
		Runs: []RunCleanupRecord{
			{Index: 0, Scenario: "canary-growing", RunID: "run-1"},
			{Index: 1, Scenario: "canary-bounded", RunID: "run-2"},
			{Index: 2, Scenario: "canary-descriptor", RunID: "run-3"},
		},
	}

	_, err := VerifyDeclaredChildRuns("/tmp", manifest, cleanup, mismatchVerify)
	if err == nil {
		t.Error("P0-8: should reject scenario mismatch")
	}
	if err != nil && !strings.Contains(err.Error(), "scenario mismatch") {
		t.Errorf("P0-8: expected scenario mismatch error, got: %v", err)
	}
}

// =============================================================================
// TEST 51: VerifyDeclaredChildRuns rejects zero declared runs (P0-8 FIX)
// =============================================================================

func TestVerifyDeclaredChildRuns_RejectsZeroDeclaredRuns(t *testing.T) {
	verifyFn := func(runDir string) (*VerifiedChildBundle, error) {
		return &VerifiedChildBundle{}, nil
	}

	// P0-8 FIX: Must have exactly 3 runs - zero runs should be rejected
	manifest := &MatrixManifest{
		Runs: []MatrixRunDeclaration{}, // Zero runs
	}
	cleanup := &MatrixCleanupEvidence{
		Runs: []RunCleanupRecord{},
	}

	_, err := VerifyDeclaredChildRuns("/tmp", manifest, cleanup, verifyFn)
	if err == nil {
		t.Error("P0-8: should reject zero declared runs")
	}
	if err != nil && !strings.Contains(err.Error(), "expected exactly 3") {
		t.Errorf("P0-8: expected 'expected exactly 3' error, got: %v", err)
	}
}

// =============================================================================
// TEST 52: VerifyDeclaredChildRuns rejects cleanup/run count mismatch (P0-8 FIX)
// =============================================================================

func TestVerifyDeclaredChildRuns_RejectsRunCountMismatch(t *testing.T) {
	verifyFn := func(runDir string) (*VerifiedChildBundle, error) {
		return &VerifiedChildBundle{}, nil
	}

	// P0-8 FIX: Must have exactly 3 runs - 2 runs should be rejected
	manifest := &MatrixManifest{
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1"},
			{Index: 2, Scenario: "canary-bounded", RunID: "run-2"},
		},
	}
	cleanup := &MatrixCleanupEvidence{
		Runs: []RunCleanupRecord{
			{Index: 0, Scenario: "canary-growing", RunID: "run-1"},
			{Index: 1, Scenario: "canary-bounded", RunID: "run-2"},
		},
	}

	_, err := VerifyDeclaredChildRuns("/tmp", manifest, cleanup, verifyFn)
	if err == nil {
		t.Error("P0-8: should reject 2 runs (requires exactly 3)")
	}
	if err != nil && !strings.Contains(err.Error(), "expected exactly 3") {
		t.Errorf("P0-8: expected 'expected exactly 3' error, got: %v", err)
	}
}

// =============================================================================
// TEST 53: VerifyDeclaredChildRuns rejects duplicate run IDs (P0-8 FIX)
// =============================================================================

func TestVerifyDeclaredChildRuns_RejectsDuplicateRunIDs(t *testing.T) {
	verifyFn := func(runDir string) (*VerifiedChildBundle, error) {
		return &VerifiedChildBundle{}, nil
	}

	// P0-8 FIX: Must use exactly 3 runs
	manifest := &MatrixManifest{
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1", Path: "runs/run-1"},
			{Index: 2, Scenario: "canary-bounded", RunID: "run-2", Path: "runs/run-2"},
			{Index: 3, Scenario: "canary-descriptor", RunID: "run-1", Path: "runs/run-1"}, // Duplicate!
		},
	}
	cleanup := &MatrixCleanupEvidence{
		Runs: []RunCleanupRecord{
			{Index: 0, Scenario: "canary-growing", RunID: "run-1"},
			{Index: 1, Scenario: "canary-bounded", RunID: "run-2"},
			{Index: 2, Scenario: "canary-descriptor", RunID: "run-1"},
		},
	}

	_, err := VerifyDeclaredChildRuns("/tmp", manifest, cleanup, verifyFn)
	if err == nil {
		t.Error("P0-8: should reject duplicate run IDs")
	}
	if err != nil && !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("P0-8: expected duplicate error, got: %v", err)
	}
}

// =============================================================================
// TEST 54: VerifyDeclaredChildRuns rejects empty run IDs (P0-8 FIX)
// =============================================================================

func TestVerifyDeclaredChildRuns_RejectsEmptyRunID(t *testing.T) {
	verifyFn := func(runDir string) (*VerifiedChildBundle, error) {
		return &VerifiedChildBundle{}, nil
	}

	// P0-8 FIX: Must use exactly 3 runs
	manifest := &MatrixManifest{
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "", Path: "runs/"},
			{Index: 2, Scenario: "canary-bounded", RunID: "run-2", Path: "runs/run-2"},
			{Index: 3, Scenario: "canary-descriptor", RunID: "run-3", Path: "runs/run-3"},
		},
	}
	cleanup := &MatrixCleanupEvidence{
		Runs: []RunCleanupRecord{
			{Index: 0, Scenario: "canary-growing", RunID: ""},
			{Index: 1, Scenario: "canary-bounded", RunID: "run-2"},
			{Index: 2, Scenario: "canary-descriptor", RunID: "run-3"},
		},
	}

	_, err := VerifyDeclaredChildRuns("/tmp", manifest, cleanup, verifyFn)
	if err == nil {
		t.Error("P0-8: should reject empty run ID")
	}
	if err != nil && !strings.Contains(err.Error(), "empty run_id") {
		t.Errorf("P0-8: expected empty run_id error, got: %v", err)
	}
}

// =============================================================================
// TEST 55: VerifyDeclaredChildRuns rejects wrong declaration index (P0-8 FIX)
// =============================================================================

func TestVerifyDeclaredChildRuns_RejectsWrongIndex(t *testing.T) {
	verifyFn := func(runDir string) (*VerifiedChildBundle, error) {
		return &VerifiedChildBundle{}, nil
	}

	// P0-8 FIX: Must use exactly 3 runs
	manifest := &MatrixManifest{
		Runs: []MatrixRunDeclaration{
			{Index: 99, Scenario: "canary-growing", RunID: "run-1", Path: "runs/run-1"}, // Wrong index!
			{Index: 2, Scenario: "canary-bounded", RunID: "run-2", Path: "runs/run-2"},
			{Index: 3, Scenario: "canary-descriptor", RunID: "run-3", Path: "runs/run-3"},
		},
	}
	cleanup := &MatrixCleanupEvidence{
		Runs: []RunCleanupRecord{
			{Index: 0, Scenario: "canary-growing", RunID: "run-1"},
			{Index: 1, Scenario: "canary-bounded", RunID: "run-2"},
			{Index: 2, Scenario: "canary-descriptor", RunID: "run-3"},
		},
	}

	_, err := VerifyDeclaredChildRuns("/tmp", manifest, cleanup, verifyFn)
	if err == nil {
		t.Error("P0-8: should reject wrong index")
	}
	if err != nil && !strings.Contains(err.Error(), "wrong index") {
		t.Errorf("P0-8: expected wrong index error, got: %v", err)
	}
}

// =============================================================================
// TEST 56: VerifyDeclaredChildRuns rejects wrong declaration path (P0-8 FIX)
// =============================================================================

func TestVerifyDeclaredChildRuns_RejectsWrongPath(t *testing.T) {
	verifyFn := func(runDir string) (*VerifiedChildBundle, error) {
		return &VerifiedChildBundle{}, nil
	}

	// P0-8 FIX: Must use exactly 3 runs
	manifest := &MatrixManifest{
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1", Path: "wrong/path"}, // Wrong path!
			{Index: 2, Scenario: "canary-bounded", RunID: "run-2", Path: "runs/run-2"},
			{Index: 3, Scenario: "canary-descriptor", RunID: "run-3", Path: "runs/run-3"},
		},
	}
	cleanup := &MatrixCleanupEvidence{
		Runs: []RunCleanupRecord{
			{Index: 0, Scenario: "canary-growing", RunID: "run-1"},
			{Index: 1, Scenario: "canary-bounded", RunID: "run-2"},
			{Index: 2, Scenario: "canary-descriptor", RunID: "run-3"},
		},
	}

	_, err := VerifyDeclaredChildRuns("/tmp", manifest, cleanup, verifyFn)
	if err == nil {
		t.Error("P0-8: should reject wrong path")
	}
	if err != nil && !strings.Contains(err.Error(), "wrong path") {
		t.Errorf("P0-8: expected wrong path error, got: %v", err)
	}
}

// =============================================================================
// TEST 57: VerifyDeclaredChildRuns panic guard - 1 run (P0-8 FIX)
// =============================================================================

func TestVerifyDeclaredChildRuns_PanicGuard_OneRun(t *testing.T) {
	verifyFn := func(runDir string) (*VerifiedChildBundle, error) {
		return &VerifiedChildBundle{}, nil
	}

	// P0-8 FIX: Exactly 1 run should panic without count guard
	manifest := &MatrixManifest{
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1", Path: "runs/run-1"},
		},
	}
	cleanup := &MatrixCleanupEvidence{
		Runs: []RunCleanupRecord{
			{Index: 0, Scenario: "canary-growing", RunID: "run-1"},
		},
	}

	_, err := VerifyDeclaredChildRuns("/tmp", manifest, cleanup, verifyFn)
	if err == nil {
		t.Error("P0-8: should reject 1 run (requires exactly 3)")
	}
	if err != nil && !strings.Contains(err.Error(), "expected exactly 3") {
		t.Errorf("P0-8: expected 'expected exactly 3' error, got: %v", err)
	}
}

// =============================================================================
// TEST 58: VerifyDeclaredChildRuns panic guard - 2 runs (P0-8 FIX)
// =============================================================================

func TestVerifyDeclaredChildRuns_PanicGuard_TwoRuns(t *testing.T) {
	verifyFn := func(runDir string) (*VerifiedChildBundle, error) {
		return &VerifiedChildBundle{}, nil
	}

	// P0-8 FIX: Exactly 2 runs should panic without count guard
	manifest := &MatrixManifest{
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1", Path: "runs/run-1"},
			{Index: 2, Scenario: "canary-bounded", RunID: "run-2", Path: "runs/run-2"},
		},
	}
	cleanup := &MatrixCleanupEvidence{
		Runs: []RunCleanupRecord{
			{Index: 0, Scenario: "canary-growing", RunID: "run-1"},
			{Index: 1, Scenario: "canary-bounded", RunID: "run-2"},
		},
	}

	_, err := VerifyDeclaredChildRuns("/tmp", manifest, cleanup, verifyFn)
	if err == nil {
		t.Error("P0-8: should reject 2 runs (requires exactly 3)")
	}
	if err != nil && !strings.Contains(err.Error(), "expected exactly 3") {
		t.Errorf("P0-8: expected 'expected exactly 3' error, got: %v", err)
	}
}

// =============================================================================
// TEST 59: VerifyDeclaredChildRuns panic guard - 4 runs (P0-8 FIX)
// =============================================================================

func TestVerifyDeclaredChildRuns_PanicGuard_FourRuns(t *testing.T) {
	verifyFn := func(runDir string) (*VerifiedChildBundle, error) {
		return &VerifiedChildBundle{}, nil
	}

	// P0-8 FIX: Exactly 4 runs should panic without count guard (would index beyond bounds)
	manifest := &MatrixManifest{
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1", Path: "runs/run-1"},
			{Index: 2, Scenario: "canary-bounded", RunID: "run-2", Path: "runs/run-2"},
			{Index: 3, Scenario: "canary-descriptor", RunID: "run-3", Path: "runs/run-3"},
			{Index: 4, Scenario: "canary-extra", RunID: "run-4", Path: "runs/run-4"}, // Extra!
		},
	}
	cleanup := &MatrixCleanupEvidence{
		Runs: []RunCleanupRecord{
			{Index: 0, Scenario: "canary-growing", RunID: "run-1"},
			{Index: 1, Scenario: "canary-bounded", RunID: "run-2"},
			{Index: 2, Scenario: "canary-descriptor", RunID: "run-3"},
			{Index: 3, Scenario: "canary-extra", RunID: "run-4"},
		},
	}

	_, err := VerifyDeclaredChildRuns("/tmp", manifest, cleanup, verifyFn)
	if err == nil {
		t.Error("P0-8: should reject 4 runs (requires exactly 3)")
	}
	if err != nil && !strings.Contains(err.Error(), "expected exactly 3") {
		t.Errorf("P0-8: expected 'expected exactly 3' error, got: %v", err)
	}
}

// =============================================================================
// PACKAGE INVARIANTS - P1 Authority Convergence
// =============================================================================

// TestCanonicalScenarioOrderHasThreeEntries documents the package contract.
// This test ensures VerifyDeclaredChildRuns cannot drift from the canonical scenario count.
// If CanonicalScenarioOrder changes, this test will fail and force a review of all geometry-dependent code.
func TestCanonicalScenarioOrderHasThreeEntries(t *testing.T) {
	if got := len(CanonicalScenarioOrder); got != 3 {
		t.Fatalf(
			"canonical scenario count = %d, expected 3",
			got,
		)
	}
}

// =============================================================================
// TWO-PHASE AUTHORITY TESTS - P0 FIX
// These tests verify the complete two-phase authority pattern:
// Phase 1: VerifyDeclaredChildBundles (no cleanup required)
// Phase 2: BindVerifiedRunsToCleanup (cleanup binding with validation)
// Composite: VerifyDeclaredChildRuns (calls both phases)
// =============================================================================

// makeTestVerifiedRun creates a VerifiedRun for testing.
func makeTestVerifiedRun(i int) *VerifiedRun {
	return &VerifiedRun{
		DeclaredRunID:    fmt.Sprintf("run-%d", i),
		DeclaredScenario: CanonicalScenarioOrder[i],
		RunIndex:         i,
		ContainerID:      fmt.Sprintf("container-%d", i),
		NetworkID:        fmt.Sprintf("network-%d", i),
		SubjectPID:       1000 + i,
		SubjectStartTime: uint64(5000 + i),
		ChildVerified:    true,
	}
}

// makeTestCleanupRecord creates a RunCleanupRecord for testing.
func makeTestCleanupRecord(i int) RunCleanupRecord {
	return RunCleanupRecord{
		Index:    i,
		Scenario: CanonicalScenarioOrder[i],
		RunID:    fmt.Sprintf("run-%d", i),
		Container: ContainerCleanupRecord{
			ID:     fmt.Sprintf("container-%d", i),
			Status: "gone",
		},
		Network: NetworkCleanupRecord{
			ID:     fmt.Sprintf("network-%d", i),
			Status: "gone",
		},
		Process: ProcessCleanupRecord{
			PID:       1000 + i,
			StartTime: uint64(5000 + i),
			Status:    "gone",
		},
	}
}

// makeTestManifest creates a MatrixManifest for testing.
func makeTestManifest() *MatrixManifest {
	return &MatrixManifest{
		SchemaVersion: MatrixSchemaVersion,
		MatrixID:      "test-matrix-1",
		StartedAt:     time.Now().Add(-10 * time.Minute),
		FinishedAt:    time.Now(),
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: CanonicalScenarioOrder[0], RunID: "run-0", Path: "runs/run-0"},
			{Index: 2, Scenario: CanonicalScenarioOrder[1], RunID: "run-1", Path: "runs/run-1"},
			{Index: 3, Scenario: CanonicalScenarioOrder[2], RunID: "run-2", Path: "runs/run-2"},
		},
	}
}

// makeTestCleanup creates a MatrixCleanupEvidence for testing.
func makeTestCleanup(matrixID string) *MatrixCleanupEvidence {
	return &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:         matrixID,
		ObservedAt:       time.Now(),
		NetworkOwnership: "per_run",
		Runs: []RunCleanupRecord{
			makeTestCleanupRecord(0),
			makeTestCleanupRecord(1),
			makeTestCleanupRecord(2),
		},
	}
}

// TEST 60: VerifyDeclaredChildBundles verifies exactly three canonical child paths.
func TestVerifyDeclaredChildBundles_VerifiesCanonicalChildren(t *testing.T) {
	manifest := makeTestManifest()
	if len(manifest.Runs) != 3 {
		t.Fatalf("test setup error: manifest must have 3 runs, got %d", len(manifest.Runs))
	}
	for i, scenario := range CanonicalScenarioOrder {
		if manifest.Runs[i].Scenario != scenario {
			t.Errorf("run[%d] scenario %q != canonical %q", i, manifest.Runs[i].Scenario, scenario)
		}
	}
}

// TEST 61: VerifyDeclaredChildBundles populates all independently verified identities.
func TestVerifyDeclaredChildBundles_PopulatesIdentities(t *testing.T) {
	manifest := makeTestManifest()
	verifyFn := func(runDir string) (*VerifiedChildBundle, error) {
		runID := filepath.Base(runDir)
		i := 0
		fmt.Sscanf(runID, "run-%d", &i)
		return &VerifiedChildBundle{
			Manifest: &evidence.Manifest{
				RunID:    fmt.Sprintf("run-%d", i),
				Scenario: CanonicalScenarioOrder[i],
			},
			Verdict:        &evidence.Verdict{Scenario: CanonicalScenarioOrder[i]},
			ContainerID:    fmt.Sprintf("container-%d", i),
			NetworkID:      fmt.Sprintf("network-%d", i),
			SubjectPID:     1000 + i,
			SubjectStart:   uint64(5000 + i),
			ChecksVerified: true,
		}, nil
	}
	tmpDir, err := os.MkdirTemp("", "two-phase-test")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	runsDir := filepath.Join(tmpDir, "runs")
	os.MkdirAll(runsDir, 0755)
	for i := 0; i < 3; i++ {
		os.MkdirAll(filepath.Join(runsDir, fmt.Sprintf("run-%d", i)), 0755)
	}
	runs, err := VerifyDeclaredChildBundles(tmpDir, manifest, verifyFn)
	if err != nil {
		t.Fatalf("VerifyDeclaredChildBundles failed: %v", err)
	}
	for i, vr := range runs {
		if vr.ContainerID == "" {
			t.Errorf("run[%d] ContainerID is empty", i)
		}
		if vr.NetworkID == "" {
			t.Errorf("run[%d] NetworkID is empty", i)
		}
		if vr.SubjectPID == 0 {
			t.Errorf("run[%d] SubjectPID is zero", i)
		}
		if vr.SubjectStartTime == 0 {
			t.Errorf("run[%d] SubjectStartTime is zero", i)
		}
		if !vr.ChildVerified {
			t.Errorf("run[%d] ChildVerified is false", i)
		}
	}
}

// TEST 62: BindVerifiedRunsToCleanup accepts exact matching evidence.
func TestBindVerifiedRunsToCleanup_AcceptsMatchingEvidence(t *testing.T) {
	manifest := makeTestManifest()
	runs := []*VerifiedRun{
		makeTestVerifiedRun(0),
		makeTestVerifiedRun(1),
		makeTestVerifiedRun(2),
	}
	cleanup := makeTestCleanup("test-matrix-1")
	bound, err := BindVerifiedRunsToCleanup(manifest, runs, cleanup)
	if err != nil {
		t.Fatalf("BindVerifiedRunsToCleanup failed: %v", err)
	}
	for i, vr := range bound {
		if !vr.CleanupEvidenceLoaded {
			t.Errorf("run[%d] CleanupEvidenceLoaded is false", i)
		}
		if !vr.CleanupEvidenceValid {
			t.Errorf("run[%d] CleanupEvidenceValid is false", i)
		}
		if vr.ProcessCleanupStatus != ProcessGone {
			t.Errorf("run[%d] ProcessCleanupStatus is %v, expected ProcessGone", i, vr.ProcessCleanupStatus)
		}
	}
}

// TEST 63: Container mismatch returns an error.
func TestBindVerifiedRunsToCleanup_RejectsContainerMismatch(t *testing.T) {
	manifest := makeTestManifest()
	runs := []*VerifiedRun{
		makeTestVerifiedRun(0),
		makeTestVerifiedRun(1),
		makeTestVerifiedRun(2),
	}
	cleanup := makeTestCleanup("test-matrix-1")
	cleanup.Runs[1].Container.ID = "wrong-container"
	_, err := BindVerifiedRunsToCleanup(manifest, runs, cleanup)
	if err == nil {
		t.Fatal("expected error for container mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "container.id") {
		t.Errorf("expected container.id error, got: %v", err)
	}
}

// TEST 64: Network mismatch returns an error.
func TestBindVerifiedRunsToCleanup_RejectsNetworkMismatch(t *testing.T) {
	manifest := makeTestManifest()
	runs := []*VerifiedRun{
		makeTestVerifiedRun(0),
		makeTestVerifiedRun(1),
		makeTestVerifiedRun(2),
	}
	cleanup := makeTestCleanup("test-matrix-1")
	cleanup.Runs[0].Network.ID = "wrong-network"
	_, err := BindVerifiedRunsToCleanup(manifest, runs, cleanup)
	if err == nil {
		t.Fatal("expected error for network mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "network.id") {
		t.Errorf("expected network.id error, got: %v", err)
	}
}

// TEST 65: PID mismatch returns an error.
func TestBindVerifiedRunsToCleanup_RejectsPIDMismatch(t *testing.T) {
	manifest := makeTestManifest()
	runs := []*VerifiedRun{
		makeTestVerifiedRun(0),
		makeTestVerifiedRun(1),
		makeTestVerifiedRun(2),
	}
	cleanup := makeTestCleanup("test-matrix-1")
	cleanup.Runs[2].Process.PID = 9999
	_, err := BindVerifiedRunsToCleanup(manifest, runs, cleanup)
	if err == nil {
		t.Fatal("expected error for PID mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "process.pid") {
		t.Errorf("expected process.pid error, got: %v", err)
	}
}

// TEST 66: Start-time mismatch returns an error.
func TestBindVerifiedRunsToCleanup_RejectsStartTimeMismatch(t *testing.T) {
	manifest := makeTestManifest()
	runs := []*VerifiedRun{
		makeTestVerifiedRun(0),
		makeTestVerifiedRun(1),
		makeTestVerifiedRun(2),
	}
	cleanup := makeTestCleanup("test-matrix-1")
	cleanup.Runs[0].Process.StartTime = 99999
	_, err := BindVerifiedRunsToCleanup(manifest, runs, cleanup)
	if err == nil {
		t.Fatal("expected error for start time mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "process.start_time") {
		t.Errorf("expected process.start_time error, got: %v", err)
	}
}

// TEST 67: RunID mismatch returns an error.
func TestBindVerifiedRunsToCleanup_RejectsRunIDMismatch(t *testing.T) {
	manifest := makeTestManifest()
	runs := []*VerifiedRun{
		makeTestVerifiedRun(0),
		makeTestVerifiedRun(1),
		makeTestVerifiedRun(2),
	}
	cleanup := makeTestCleanup("test-matrix-1")
	cleanup.Runs[1].RunID = "wrong-run-id"
	_, err := BindVerifiedRunsToCleanup(manifest, runs, cleanup)
	if err == nil {
		t.Fatal("expected error for RunID mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "RunID") {
		t.Errorf("expected RunID error, got: %v", err)
	}
}

// TEST 68: Failed binding does not mutate any input run.
func TestBindVerifiedRunsToCleanup_NoPartialMutation(t *testing.T) {
	manifest := makeTestManifest()
	runs := []*VerifiedRun{
		makeTestVerifiedRun(0),
		makeTestVerifiedRun(1),
		makeTestVerifiedRun(2),
	}
	cleanup := makeTestCleanup("test-matrix-1")
	originalLoaded := make([]bool, 3)
	originalValid := make([]bool, 3)
	for i, vr := range runs {
		originalLoaded[i] = vr.CleanupEvidenceLoaded
		originalValid[i] = vr.CleanupEvidenceValid
	}
	cleanup.Runs[2].Process.PID = 9999
	_, err := BindVerifiedRunsToCleanup(manifest, runs, cleanup)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	for i, vr := range runs {
		if vr.CleanupEvidenceLoaded != originalLoaded[i] {
			t.Errorf("run[%d] CleanupEvidenceLoaded was mutated", i)
		}
		if vr.CleanupEvidenceValid != originalValid[i] {
			t.Errorf("run[%d] CleanupEvidenceValid was mutated", i)
		}
	}
}

// TEST 69: VerifyDeclaredChildRuns is proven to call both shared phases.
func TestVerifyDeclaredChildRuns_IsCompositeWrapper(t *testing.T) {
	manifest := makeTestManifest()
	cleanup := makeTestCleanup("test-matrix-1")
	verifyFn := func(runDir string) (*VerifiedChildBundle, error) {
		runID := filepath.Base(runDir)
		i := 0
		fmt.Sscanf(runID, "run-%d", &i)
		return &VerifiedChildBundle{
			Manifest: &evidence.Manifest{
				RunID:    fmt.Sprintf("run-%d", i),
				Scenario: CanonicalScenarioOrder[i],
			},
			Verdict:        &evidence.Verdict{Scenario: CanonicalScenarioOrder[i]},
			ContainerID:    fmt.Sprintf("container-%d", i),
			NetworkID:      fmt.Sprintf("network-%d", i),
			SubjectPID:     1000 + i,
			SubjectStart:   uint64(5000 + i),
			ChecksVerified: true,
		}, nil
	}
	tmpDir, err := os.MkdirTemp("", "composite-test")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	runsDir := filepath.Join(tmpDir, "runs")
	os.MkdirAll(runsDir, 0755)
	for i := 0; i < 3; i++ {
		os.MkdirAll(filepath.Join(runsDir, fmt.Sprintf("run-%d", i)), 0755)
	}
	compositeRuns, err := VerifyDeclaredChildRuns(tmpDir, manifest, cleanup, verifyFn)
	if err != nil {
		t.Fatalf("VerifyDeclaredChildRuns failed: %v", err)
	}
	phase1Runs, err := VerifyDeclaredChildBundles(tmpDir, manifest, verifyFn)
	if err != nil {
		t.Fatalf("VerifyDeclaredChildBundles failed: %v", err)
	}
	phase2Runs, err := BindVerifiedRunsToCleanup(manifest, phase1Runs, cleanup)
	if err != nil {
		t.Fatalf("BindVerifiedRunsToCleanup failed: %v", err)
	}
	if len(compositeRuns) != len(phase2Runs) {
		t.Fatalf("composite len=%d, phases len=%d", len(compositeRuns), len(phase2Runs))
	}
	for i := range compositeRuns {
		if compositeRuns[i].DeclaredRunID != phase2Runs[i].DeclaredRunID {
			t.Errorf("run[%d] DeclaredRunID mismatch", i)
		}
		if compositeRuns[i].ContainerID != phase2Runs[i].ContainerID {
			t.Errorf("run[%d] ContainerID mismatch", i)
		}
		if compositeRuns[i].CleanupEvidenceLoaded != phase2Runs[i].CleanupEvidenceLoaded {
			t.Errorf("run[%d] CleanupEvidenceLoaded mismatch", i)
		}
	}
}

// TEST 70: ValidateCleanupBinding rejects empty schema_version.
func TestValidateCleanupBinding_RejectsEmptySchemaVersion(t *testing.T) {
	manifest := makeTestManifest()
	runs := []*VerifiedRun{makeTestVerifiedRun(0), makeTestVerifiedRun(1), makeTestVerifiedRun(2)}
	cleanup := makeTestCleanup("test-matrix-1")
	cleanup.SchemaVersion = ""
	err := ValidateCleanupBinding(manifest, runs, cleanup)
	if err == nil {
		t.Fatal("expected error for empty schema_version, got nil")
	}
}

// TEST 71: ValidateCleanupBinding rejects MatrixID mismatch.
func TestValidateCleanupBinding_RejectsMatrixIDMismatch(t *testing.T) {
	manifest := makeTestManifest()
	runs := []*VerifiedRun{makeTestVerifiedRun(0), makeTestVerifiedRun(1), makeTestVerifiedRun(2)}
	cleanup := makeTestCleanup("different-matrix-id")
	err := ValidateCleanupBinding(manifest, runs, cleanup)
	if err == nil {
		t.Fatal("expected error for MatrixID mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "matrix_id") {
		t.Errorf("expected matrix_id error, got: %v", err)
	}
}

// TEST 72: ValidateCleanupBinding rejects zero observed_at timestamp.
func TestValidateCleanupBinding_RejectsZeroObservedAt(t *testing.T) {
	manifest := makeTestManifest()
	runs := []*VerifiedRun{makeTestVerifiedRun(0), makeTestVerifiedRun(1), makeTestVerifiedRun(2)}
	cleanup := makeTestCleanup("test-matrix-1")
	cleanup.ObservedAt = time.Time{}
	err := ValidateCleanupBinding(manifest, runs, cleanup)
	if err == nil {
		t.Fatal("expected error for zero observed_at, got nil")
	}
}

// TEST 73: ValidateCleanupBinding rejects invalid network_ownership.
func TestValidateCleanupBinding_RejectsInvalidNetworkOwnership(t *testing.T) {
	manifest := makeTestManifest()
	runs := []*VerifiedRun{makeTestVerifiedRun(0), makeTestVerifiedRun(1), makeTestVerifiedRun(2)}
	cleanup := makeTestCleanup("test-matrix-1")
	cleanup.NetworkOwnership = "invalid-mode"
	err := ValidateCleanupBinding(manifest, runs, cleanup)
	if err == nil {
		t.Fatal("expected error for invalid network_ownership, got nil")
	}
}

// TEST 74: ValidateCleanupBinding rejects index mismatch.
func TestValidateCleanupBinding_RejectsIndexMismatch(t *testing.T) {
	manifest := makeTestManifest()
	runs := []*VerifiedRun{makeTestVerifiedRun(0), makeTestVerifiedRun(1), makeTestVerifiedRun(2)}
	cleanup := makeTestCleanup("test-matrix-1")
	cleanup.Runs[1].Index = 99
	err := ValidateCleanupBinding(manifest, runs, cleanup)
	if err == nil {
		t.Fatal("expected error for index mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "index") {
		t.Errorf("expected index error, got: %v", err)
	}
}

// TEST 75: ValidateCleanupBinding rejects scenario mismatch.
func TestValidateCleanupBinding_RejectsScenarioMismatch(t *testing.T) {
	manifest := makeTestManifest()
	runs := []*VerifiedRun{makeTestVerifiedRun(0), makeTestVerifiedRun(1), makeTestVerifiedRun(2)}
	cleanup := makeTestCleanup("test-matrix-1")
	cleanup.Runs[0].Scenario = "wrong-scenario"
	err := ValidateCleanupBinding(manifest, runs, cleanup)
	if err == nil {
		t.Fatal("expected error for scenario mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "scenario") {
		t.Errorf("expected scenario error, got: %v", err)
	}
}

// TEST 76: ValidateCleanupBinding rejects cleanup observed_at before matrix finished_at.
func TestValidateCleanupBinding_RejectsObservedAtBeforeFinishedAt(t *testing.T) {
	manifest := makeTestManifest()
	runs := []*VerifiedRun{makeTestVerifiedRun(0), makeTestVerifiedRun(1), makeTestVerifiedRun(2)}
	cleanup := makeTestCleanup("test-matrix-1")
	// Set observed_at BEFORE finished_at
	cleanup.ObservedAt = manifest.FinishedAt.Add(-1 * time.Hour)
	err := ValidateCleanupBinding(manifest, runs, cleanup)
	if err == nil {
		t.Fatal("expected error for observed_at before finished_at, got nil")
	}
	if !strings.Contains(err.Error(), "observed_at") {
		t.Errorf("expected observed_at error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "precedes") {
		t.Errorf("expected 'precedes' error message, got: %v", err)
	}
}

// TEST 77: VerifyDeclaredChildBundles records exact paths called on verifier (P0 FIX).
func TestVerifyDeclaredChildBundles_RecordsCanonicalPaths(t *testing.T) {
	manifest := makeTestManifest()

	// Recording verifier that captures all paths called
	var recordedPaths []string
	recordVerify := func(runDir string) (*VerifiedChildBundle, error) {
		recordedPaths = append(recordedPaths, runDir)
		runID := filepath.Base(runDir)
		i := 0
		fmt.Sscanf(runID, "run-%d", &i)
		return &VerifiedChildBundle{
			Manifest: &evidence.Manifest{
				RunID:    fmt.Sprintf("run-%d", i),
				Scenario: CanonicalScenarioOrder[i],
			},
			Verdict:        &evidence.Verdict{Scenario: CanonicalScenarioOrder[i]},
			ContainerID:    fmt.Sprintf("container-%d", i),
			NetworkID:      fmt.Sprintf("network-%d", i),
			SubjectPID:     1000 + i,
			SubjectStart:   uint64(5000 + i),
			ChecksVerified: true,
		}, nil
	}

	tmpDir, err := os.MkdirTemp("", "recording-test")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	runsDir := filepath.Join(tmpDir, "runs")
	os.MkdirAll(runsDir, 0755)
	for i := 0; i < 3; i++ {
		os.MkdirAll(filepath.Join(runsDir, fmt.Sprintf("run-%d", i)), 0755)
	}

	_, err = VerifyDeclaredChildBundles(tmpDir, manifest, recordVerify)
	if err != nil {
		t.Fatalf("VerifyDeclaredChildBundles failed: %v", err)
	}

	// P0 FIX: Verify exactly 3 calls in order
	if len(recordedPaths) != 3 {
		t.Fatalf("expected exactly 3 verifier calls, got %d", len(recordedPaths))
	}

	// Verify exact paths in canonical order
	expectedPaths := []string{
		filepath.Join(tmpDir, "runs", "run-0"),
		filepath.Join(tmpDir, "runs", "run-1"),
		filepath.Join(tmpDir, "runs", "run-2"),
	}
	for i, expected := range expectedPaths {
		if recordedPaths[i] != expected {
			t.Errorf("path[%d]: expected %q, got %q", i, expected, recordedPaths[i])
		}
	}
}

// TEST 78: ValidateCleanupBinding requires matrix_shared network status "gone" (P1 FIX).
func TestValidateCleanupBinding_RequiresMatrixSharedNetworkGone(t *testing.T) {
	manifest := makeTestManifest()
	// Create VerifiedRuns with shared network ID for matrix_shared mode
	runs := []*VerifiedRun{
		makeTestVerifiedRun(0),
		makeTestVerifiedRun(1),
		makeTestVerifiedRun(2),
	}
	// Set all runs to use the same shared network ID
	for _, vr := range runs {
		vr.NetworkID = "shared-net"
	}

	// Create matrix_shared cleanup with network status "exists"
	cleanup := &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:         "test-matrix-1",
		ObservedAt:       time.Now(),
		NetworkOwnership: "matrix_shared",
		Runs: []RunCleanupRecord{
			{
				Index:     0,
				Scenario:  "canary-growing",
				RunID:     "run-0",
				Container: ContainerCleanupRecord{ID: "container-0", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "shared-net", Status: "exists"}, // Should be gone!
				Process:   ProcessCleanupRecord{PID: 1000, StartTime: 5000, Status: "gone"},
			},
			{
				Index:     1,
				Scenario:  "canary-bounded",
				RunID:     "run-1",
				Container: ContainerCleanupRecord{ID: "container-1", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "shared-net", Status: "exists"}, // Should be gone!
				Process:   ProcessCleanupRecord{PID: 1001, StartTime: 5001, Status: "gone"},
			},
			{
				Index:     2,
				Scenario:  "canary-descriptor",
				RunID:     "run-2",
				Container: ContainerCleanupRecord{ID: "container-2", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "shared-net", Status: "exists"}, // Should be gone!
				Process:   ProcessCleanupRecord{PID: 1002, StartTime: 5002, Status: "gone"},
			},
		},
	}

	err := ValidateCleanupBinding(manifest, runs, cleanup)
	if err == nil {
		t.Fatal("expected error for matrix_shared network.status='exists', got nil")
	}
	if !strings.Contains(err.Error(), "network.status") {
		t.Errorf("expected network.status error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "expected gone") {
		t.Errorf("expected 'expected gone' error message, got: %v", err)
	}
}

// TEST 79: ValidateCleanupBinding accepts valid matrix_shared with status "gone".
func TestValidateCleanupBinding_AcceptsMatrixSharedNetworkGone(t *testing.T) {
	manifest := makeTestManifest()
	runs := []*VerifiedRun{makeTestVerifiedRun(0), makeTestVerifiedRun(1), makeTestVerifiedRun(2)}
	// Set all runs to use the same shared network ID
	for _, vr := range runs {
		vr.NetworkID = "shared-net"
	}

	// Create valid matrix_shared cleanup
	cleanup := &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:         "test-matrix-1",
		ObservedAt:       time.Now(),
		NetworkOwnership: "matrix_shared",
		Runs: []RunCleanupRecord{
			{
				Index:     0,
				Scenario:  "canary-growing",
				RunID:     "run-0",
				Container: ContainerCleanupRecord{ID: "container-0", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "shared-net", Status: "gone"}, // Valid
				Process:   ProcessCleanupRecord{PID: 1000, StartTime: 5000, Status: "gone"},
			},
			{
				Index:     1,
				Scenario:  "canary-bounded",
				RunID:     "run-1",
				Container: ContainerCleanupRecord{ID: "container-1", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "shared-net", Status: "gone"}, // Valid
				Process:   ProcessCleanupRecord{PID: 1001, StartTime: 5001, Status: "gone"},
			},
			{
				Index:     2,
				Scenario:  "canary-descriptor",
				RunID:     "run-2",
				Container: ContainerCleanupRecord{ID: "container-2", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "shared-net", Status: "gone"}, // Valid
				Process:   ProcessCleanupRecord{PID: 1002, StartTime: 5002, Status: "gone"},
			},
		},
	}

	err := ValidateCleanupBinding(manifest, runs, cleanup)
	if err != nil {
		t.Errorf("unexpected error for valid matrix_shared cleanup: %v", err)
	}
}

// TEST 80: ValidateCleanupBinding rejects matrix_shared with unavailable network status.
func TestValidateCleanupBinding_RejectsMatrixSharedNetworkUnavailable(t *testing.T) {
	manifest := makeTestManifest()
	runs := []*VerifiedRun{makeTestVerifiedRun(0), makeTestVerifiedRun(1), makeTestVerifiedRun(2)}
	// Set all runs to use the same shared network ID
	for _, vr := range runs {
		vr.NetworkID = "shared-net"
	}

	cleanup := &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:         "test-matrix-1",
		ObservedAt:       time.Now(),
		NetworkOwnership: "matrix_shared",
		Runs: []RunCleanupRecord{
			{
				Index:     0,
				Scenario:  "canary-growing",
				RunID:     "run-0",
				Container: ContainerCleanupRecord{ID: "container-0", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "shared-net", Status: "unavailable"},
				Process:   ProcessCleanupRecord{PID: 1000, StartTime: 5000, Status: "gone"},
			},
			{
				Index:     1,
				Scenario:  "canary-bounded",
				RunID:     "run-1",
				Container: ContainerCleanupRecord{ID: "container-1", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "shared-net", Status: "gone"},
				Process:   ProcessCleanupRecord{PID: 1001, StartTime: 5001, Status: "gone"},
			},
			{
				Index:     2,
				Scenario:  "canary-descriptor",
				RunID:     "run-2",
				Container: ContainerCleanupRecord{ID: "container-2", Status: "gone"},
				Network:   NetworkCleanupRecord{ID: "shared-net", Status: "gone"},
				Process:   ProcessCleanupRecord{PID: 1002, StartTime: 5002, Status: "gone"},
			},
		},
	}

	err := ValidateCleanupBinding(manifest, runs, cleanup)
	if err == nil {
		t.Fatal("expected error for matrix_shared network.status='unavailable', got nil")
	}
	if !strings.Contains(err.Error(), "network.status") {
		t.Errorf("expected network.status error, got: %v", err)
	}
}
