// matrix_test.go — Matrix Command Tests
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence"
)

func TestMatrixSchemaVersion(t *testing.T) {
	if MatrixSchemaVersion != "1.0.0" {
		t.Errorf("MatrixSchemaVersion = %q, want 1.0.0", MatrixSchemaVersion)
	}
}

func TestMatrixScenarioOrder(t *testing.T) {
	want := []string{"canary-growing", "canary-bounded", "canary-descriptor"}
	for i, s := range matrixScenarioOrder {
		if s != want[i] {
			t.Errorf("matrixScenarioOrder[%d] = %q, want %q", i, s, want[i])
		}
	}
	if len(matrixScenarioOrder) != 3 {
		t.Errorf("len(matrixScenarioOrder) = %d, want 3", len(matrixScenarioOrder))
	}
}

func TestExpectedClassificationMatrix(t *testing.T) {
	cases := map[string]struct {
		Overall, Memory, Resource, Semantic string
	}{
		"canary-growing":    {"growing", "growing", "stable", "stable"},
		"canary-bounded":    {"stable", "stable", "stable", "stable"},
		"canary-descriptor": {"resource_growth", "stable", "resource_growth", "stable"},
	}

	for scenario, want := range cases {
		got, ok := ExpectedClassificationMatrix[scenario]
		if !ok {
			t.Errorf("ExpectedClassificationMatrix missing scenario %q", scenario)
			continue
		}
		if got.Overall != want.Overall {
			t.Errorf("ExpectedClassificationMatrix[%q].Overall = %q, want %q", scenario, got.Overall, want.Overall)
		}
		if got.Memory != want.Memory {
			t.Errorf("ExpectedClassificationMatrix[%q].Memory = %q, want %q", scenario, got.Memory, want.Memory)
		}
		if got.Resource != want.Resource {
			t.Errorf("ExpectedClassificationMatrix[%q].Resource = %q, want %q", scenario, got.Resource, want.Resource)
		}
		if got.Semantic != want.Semantic {
			t.Errorf("ExpectedClassificationMatrix[%q].Semantic = %q, want %q", scenario, got.Semantic, want.Semantic)
		}
	}
}

func TestValidateScenarioContractGrowing(t *testing.T) {
	workload := &WorkloadResult{
		Requested: 32, Attempted: 32, Completed: 32, Failed: 0, Returned: 32,
	}
	initialState := &CanaryState{Mode: "growing", RetainedBlocks: 0, RetainedBytes: 0}
	finalState := &CanaryState{Mode: "growing", RetainedBlocks: 32, RetainedBytes: 33554432}
	verdict := &evidence.Verdict{
		OverallClassification:  analysis.ClassificationGrowing,
		MemoryClassification:   analysis.ClassificationGrowing,
		ResourceClassification: analysis.ClassificationStable,
		SemanticClassification: analysis.ClassificationStable,
		ScenarioValid:         true,
		CanariesValid:         true,
		ProvenanceValid:       true,
	}

	errors := ValidateScenarioContract("canary-growing", workload, initialState, finalState, verdict)
	if len(errors) > 0 {
		t.Errorf("unexpected errors: %v", errors)
	}
}

func TestValidateScenarioContractBounded(t *testing.T) {
	workload := &WorkloadResult{
		Requested: 100, Attempted: 100, Completed: 100, Failed: 0, Returned: 100,
	}
	initialState := &CanaryState{Mode: "bounded", RetainedBlocks: 0, RetainedBytes: 0, BufferCapacity: 8192}
	finalState := &CanaryState{Mode: "bounded", RetainedBlocks: 0, RetainedBytes: 0, BufferCapacity: 8192}
	verdict := &evidence.Verdict{
		OverallClassification:  analysis.ClassificationStable,
		MemoryClassification:   analysis.ClassificationStable,
		ResourceClassification: analysis.ClassificationStable,
		SemanticClassification: analysis.ClassificationStable,
		ScenarioValid:         true,
		CanariesValid:         true,
		ProvenanceValid:       true,
	}

	errors := ValidateScenarioContract("canary-bounded", workload, initialState, finalState, verdict)
	if len(errors) > 0 {
		t.Errorf("unexpected errors: %v", errors)
	}
}

func TestValidateScenarioContractDescriptor(t *testing.T) {
	workload := &WorkloadResult{
		Requested: 100, Attempted: 100, Completed: 100, Failed: 0, Returned: 100,
	}
	initialState := &CanaryState{Mode: "descriptor", FDCount: 5, RetainedBlocks: 0, RetainedBytes: 0}
	finalState := &CanaryState{Mode: "descriptor", FDCount: 205, RetainedBlocks: 0, RetainedBytes: 0}
	verdict := &evidence.Verdict{
		OverallClassification:  analysis.ClassificationResourceGrowth,
		MemoryClassification:   analysis.ClassificationStable,
		ResourceClassification: analysis.ClassificationResourceGrowth,
		SemanticClassification: analysis.ClassificationStable,
		ScenarioValid:         true,
		CanariesValid:         true,
		ProvenanceValid:       true,
		SignalSummaries: []analysis.SignalSummary{
			{
				Name:           "descriptor_state_invariant",
				SampleCount:    2,
				AvailableCount: 2,
				MissingCount:   0,
				AbsoluteDelta:  200,
				IsPrimary:      true,
			},
		},
	}

	errors := ValidateScenarioContract("canary-descriptor", workload, initialState, finalState, verdict)
	if len(errors) > 0 {
		t.Errorf("unexpected errors: %v", errors)
	}
}

func TestValidateScenarioContractUnknownScenario(t *testing.T) {
	errors := ValidateScenarioContract("unknown", nil, nil, nil, nil)
	if len(errors) != 1 {
		t.Errorf("expected 1 error for unknown scenario, got %d", len(errors))
	}
}

func TestValidateScenarioContractGrowingWrongWorkload(t *testing.T) {
	workload := &WorkloadResult{
		Requested: 31, Attempted: 31, Completed: 31, Failed: 0, Returned: 31,
	}
	initialState := &CanaryState{Mode: "growing", RetainedBlocks: 0, RetainedBytes: 0}
	finalState := &CanaryState{Mode: "growing", RetainedBlocks: 31, RetainedBytes: 32512}
	verdict := &evidence.Verdict{
		OverallClassification:  analysis.ClassificationGrowing,
		MemoryClassification:   analysis.ClassificationGrowing,
		ResourceClassification: analysis.ClassificationStable,
		SemanticClassification: analysis.ClassificationStable,
		ScenarioValid:         true,
		CanariesValid:         true,
		ProvenanceValid:       true,
	}

	errors := ValidateScenarioContract("canary-growing", workload, initialState, finalState, verdict)
	if len(errors) == 0 {
		t.Error("expected errors for wrong workload count")
	}
}

func TestValidateScenarioContractGrowingWrongClassification(t *testing.T) {
	workload := &WorkloadResult{
		Requested: 32, Attempted: 32, Completed: 32, Failed: 0, Returned: 32,
	}
	initialState := &CanaryState{Mode: "growing", RetainedBlocks: 0, RetainedBytes: 0}
	finalState := &CanaryState{Mode: "growing", RetainedBlocks: 32, RetainedBytes: 33554432}
	verdict := &evidence.Verdict{
		OverallClassification:  analysis.ClassificationStable, // Wrong!
		MemoryClassification:   analysis.ClassificationGrowing,
		ResourceClassification: analysis.ClassificationStable,
		SemanticClassification: analysis.ClassificationStable,
		ScenarioValid:         true,
		CanariesValid:         true,
		ProvenanceValid:       true,
	}

	errors := ValidateScenarioContract("canary-growing", workload, initialState, finalState, verdict)
	if len(errors) == 0 {
		t.Error("expected errors for wrong classification")
	}
}

func TestValidateScenarioContractBoundedRetainedBlocks(t *testing.T) {
	workload := &WorkloadResult{
		Requested: 100, Attempted: 100, Completed: 100, Failed: 0, Returned: 100,
	}
	initialState := &CanaryState{Mode: "bounded", RetainedBlocks: 0, RetainedBytes: 0, BufferCapacity: 8192}
	finalState := &CanaryState{Mode: "bounded", RetainedBlocks: 5, RetainedBytes: 0, BufferCapacity: 8192} // Wrong!
	verdict := &evidence.Verdict{
		OverallClassification:  analysis.ClassificationStable,
		MemoryClassification:   analysis.ClassificationStable,
		ResourceClassification: analysis.ClassificationStable,
		SemanticClassification: analysis.ClassificationStable,
		ScenarioValid:         true,
		CanariesValid:         true,
		ProvenanceValid:       true,
	}

	errors := ValidateScenarioContract("canary-bounded", workload, initialState, finalState, verdict)
	if len(errors) == 0 {
		t.Error("expected errors for bounded with retained blocks")
	}
}

func TestValidateScenarioContractDescriptorWrongFDDelta(t *testing.T) {
	workload := &WorkloadResult{
		Requested: 100, Attempted: 100, Completed: 100, Failed: 0, Returned: 100,
	}
	initialState := &CanaryState{Mode: "descriptor", FDCount: 5, RetainedBlocks: 0, RetainedBytes: 0}
	finalState := &CanaryState{Mode: "descriptor", FDCount: 100, RetainedBlocks: 0, RetainedBytes: 0} // Wrong!
	verdict := &evidence.Verdict{
		OverallClassification:  analysis.ClassificationResourceGrowth,
		MemoryClassification:   analysis.ClassificationStable,
		ResourceClassification: analysis.ClassificationResourceGrowth,
		SemanticClassification: analysis.ClassificationStable,
		ScenarioValid:         true,
		CanariesValid:         true,
		ProvenanceValid:       true,
		SignalSummaries: []analysis.SignalSummary{
			{
				Name:           "descriptor_state_invariant",
				SampleCount:    2,
				AvailableCount: 2,
				MissingCount:   0,
				AbsoluteDelta:  95, // Wrong!
				IsPrimary:      true,
			},
		},
	}

	errors := ValidateScenarioContract("canary-descriptor", workload, initialState, finalState, verdict)
	if len(errors) == 0 {
		t.Error("expected errors for descriptor with wrong FD delta")
	}
}

func TestValidateMatrixRootGeometry(t *testing.T) {
	// Create temp directory structure
	tmpDir, err := os.MkdirTemp("", "matrix-geometry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Test empty directory
	err = ValidateMatrixRootGeometry(tmpDir)
	if err == nil {
		t.Error("expected error for empty directory")
	}

	// Create runs directory
	runsDir := filepath.Join(tmpDir, "runs")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Test missing files
	err = ValidateMatrixRootGeometry(tmpDir)
	if err == nil {
		t.Error("expected error for missing files")
	}

	// Create required files
	for _, name := range []string{"matrix-manifest.json", "matrix-verdict.json", "matrix-checksums.txt"} {
		f, err := os.Create(filepath.Join(tmpDir, name))
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}

	// Test missing run directories
	err = ValidateMatrixRootGeometry(tmpDir)
	if err == nil {
		t.Error("expected error for missing run directories")
	}

	// Create 3 run directories
	for i := 0; i < 3; i++ {
		if err := os.MkdirAll(filepath.Join(runsDir, "run-"+string(rune('a'+i))), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Should now pass
	err = ValidateMatrixRootGeometry(tmpDir)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Add extra file
	extra, err := os.Create(filepath.Join(tmpDir, "extra.txt"))
	if err != nil {
		t.Fatal(err)
	}
	extra.Close()

	err = ValidateMatrixRootGeometry(tmpDir)
	if err == nil {
		t.Error("expected error for unexpected file")
	}
}

func TestValidateMatrixRootGeometryExtraDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "matrix-geometry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	runsDir := filepath.Join(tmpDir, "runs")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"matrix-manifest.json", "matrix-verdict.json", "matrix-checksums.txt"} {
		f, err := os.Create(filepath.Join(tmpDir, name))
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}

	// Add extra directory
	extraDir := filepath.Join(tmpDir, "extra-dir")
	if err := os.MkdirAll(extraDir, 0755); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		if err := os.MkdirAll(filepath.Join(runsDir, "run-"+string(rune('a'+i))), 0755); err != nil {
			t.Fatal(err)
		}
	}

	err = ValidateMatrixRootGeometry(tmpDir)
	if err == nil {
		t.Error("expected error for extra directory")
	}
}

func TestCompareExecutionIdentity(t *testing.T) {
	thresholds1 := analysis.DefaultThresholds()
	thresholds2 := analysis.DefaultThresholds()

	id1 := NewMatrixExecutionIdentity(
		"abc123", "tree123", "sha1",
		12345, "hash123",
		"image-ref", "image-id",
		[]string{"digest1"}, "available",
		"canary-commit", "canary-tree", "canary-subtree",
		"binary-hash", "binary-label",
		"rev-label", "tree-label", "subtree-label",
		"5.4.0", "Linux 5.4.0", "cgroup2",
		"24.0.0", "1.44",
		&thresholds1,
		nil,
	)

	id2 := NewMatrixExecutionIdentity(
		"abc123", "tree123", "sha1",
		12345, "hash123",
		"image-ref", "image-id",
		[]string{"digest1"}, "available",
		"canary-commit", "canary-tree", "canary-subtree",
		"binary-hash", "binary-label",
		"rev-label", "tree-label", "subtree-label",
		"5.4.0", "Linux 5.4.0", "cgroup2",
		"24.0.0", "1.44",
		&thresholds2,
		nil,
	)

	// Same
	diffs := CompareExecutionIdentity(id1, id2)
	if len(diffs) > 0 {
		t.Errorf("unexpected differences: %v", diffs)
	}

	// Different controller PID
	id2.ControllerPID = 54321
	diffs = CompareExecutionIdentity(id1, id2)
	if len(diffs) != 1 {
		t.Errorf("expected 1 difference, got %d", len(diffs))
	}

	// Different image
	id2.ControllerPID = id1.ControllerPID
	id2.ImageID = "different-image-id"
	diffs = CompareExecutionIdentity(id1, id2)
	if len(diffs) != 1 {
		t.Errorf("expected 1 difference, got %d", len(diffs))
	}
}

func TestLoadMatrixManifest(t *testing.T) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "matrix-manifest-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	manifest := &MatrixManifest{
		SchemaVersion: MatrixSchemaVersion,
		MatrixID:     "test-matrix-123",
		StartedAt:    time.Now(),
		FinishedAt:   time.Now().Add(10 * time.Minute),
		Runs: []MatrixRunDeclaration{
			{Index: 1, Scenario: "canary-growing", RunID: "run-1", ChecksumsSHA256: "abc123"},
			{Index: 2, Scenario: "canary-bounded", RunID: "run-2", ChecksumsSHA256: "def456"},
			{Index: 3, Scenario: "canary-descriptor", RunID: "run-3", ChecksumsSHA256: "ghi789"},
		},
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmpFile.Write(data); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	// Load
	loaded, err := LoadMatrixManifest(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadMatrixManifest failed: %v", err)
	}

	if loaded.SchemaVersion != MatrixSchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", loaded.SchemaVersion, MatrixSchemaVersion)
	}
	if loaded.MatrixID != "test-matrix-123" {
		t.Errorf("MatrixID = %q, want test-matrix-123", loaded.MatrixID)
	}
	if len(loaded.Runs) != 3 {
		t.Errorf("len(Runs) = %d, want 3", len(loaded.Runs))
	}
}

func TestLoadMatrixVerdict(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "matrix-verdict-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	verdict := &MatrixVerdict{
		MatrixID:    "test-matrix-123",
		MatrixValid: true,
		ScenarioResults: map[string]*ScenarioResult{
			"canary-growing":    {RunID: "run-1", Verified: true, Overall: "growth"},
			"canary-bounded":    {RunID: "run-2", Verified: true, Overall: "stable"},
			"canary-descriptor": {RunID: "run-3", Verified: true, Overall: "resource_growth"},
		},
		ChecksTotal:  16,
		ChecksPassed: 16,
		ChecksFailed: 0,
	}

	data, err := json.MarshalIndent(verdict, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmpFile.Write(data); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	loaded, err := LoadMatrixVerdict(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadMatrixVerdict failed: %v", err)
	}

	if !loaded.MatrixValid {
		t.Error("MatrixValid = false, want true")
	}
	if loaded.ChecksFailed != 0 {
		t.Errorf("ChecksFailed = %d, want 0", loaded.ChecksFailed)
	}
}

func TestCrossRunChecksAllTrue(t *testing.T) {
	checks := &CrossRunChecks{
		SameCommitTree:          true,
		SameControllerPID:       true,
		SameControllerHash:      true,
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
	}

	if !checks.AllTrue() {
		t.Error("AllTrue() = false, want true")
	}

	checks.SameCommitTree = false
	if checks.AllTrue() {
		t.Error("AllTrue() = true, want false")
	}
}

func TestComputeSHA256Hex(t *testing.T) {
	hash := computeSHA256Hex([]byte("test data"))
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}

	// Verify consistency
	hash2 := computeSHA256Hex([]byte("test data"))
	if hash != hash2 {
		t.Error("hash not deterministic")
	}

	// Empty input
	emptyHash := computeSHA256Hex([]byte{})
	if emptyHash == "" {
		t.Error("empty input hash should not be empty")
	}
}

func TestStringSliceEqual(t *testing.T) {
	a := []string{"a", "b", "c"}
	b := []string{"a", "b", "c"}
	c := []string{"a", "b", "d"}

	if !stringSliceEqual(a, b) {
		t.Error("stringSliceEqual(a, b) = false, want true")
	}
	if stringSliceEqual(a, c) {
		t.Error("stringSliceEqual(a, c) = true, want false")
	}

	// Different lengths
	d := []string{"a", "b"}
	if stringSliceEqual(a, d) {
		t.Error("stringSliceEqual with different lengths = true, want false")
	}
}

func TestThresholdsEqual(t *testing.T) {
	t1 := analysis.DefaultThresholds()
	t2 := analysis.DefaultThresholds()

	if !thresholdsEqual(t1, t2) {
		t.Error("thresholdsEqual same = false, want true")
	}

	t2.MemoryGrowthKibPerHour = 999
	if thresholdsEqual(t1, t2) {
		t.Error("thresholdsEqual different = true, want false")
	}
}
