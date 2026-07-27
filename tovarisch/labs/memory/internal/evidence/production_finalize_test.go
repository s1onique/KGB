// production_finalize_test.go — Comprehensive tests for production finalizer.
//
// Reference: ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION51-CORRECTION03
package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
)

// =============================================================================
// P0-3A: Complete nil dependency proof
// =============================================================================

func TestProductionFinalize_NilExecuteLifecycleRejected(t *testing.T) {
	deps := ProductionQualifiedRunDependencies{
		ExecuteLifecycle:     nil, // exactly one nil
		CollectProvenance:    recordNoopCollectProvenance,
		PersistFinalEvidence: recordNoopPersistFinalEvidence,
		VerifyEvidenceBytes:  recordNoopVerifyEvidenceBytes,
		WriteManifest:        recordNoopWriteManifest,
		WriteChecksums:       recordNoopWriteChecksums,
	}
	opts := validOptions(t)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)

	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, ErrNilDependency) {
		t.Errorf("errors.Is(err, ErrNilDependency): got %v", err)
	}
}

func TestProductionFinalize_NilCollectProvenanceRejected(t *testing.T) {
	deps := ProductionQualifiedRunDependencies{
		ExecuteLifecycle:     recordNoopExecuteLifecycle,
		CollectProvenance:   nil, // exactly one nil
		PersistFinalEvidence: recordNoopPersistFinalEvidence,
		VerifyEvidenceBytes:  recordNoopVerifyEvidenceBytes,
		WriteManifest:        recordNoopWriteManifest,
		WriteChecksums:       recordNoopWriteChecksums,
	}
	opts := validOptions(t)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)

	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, ErrNilDependency) {
		t.Errorf("errors.Is(err, ErrNilDependency): got %v", err)
	}
}

func TestProductionFinalize_NilPersistFinalEvidenceRejected(t *testing.T) {
	deps := ProductionQualifiedRunDependencies{
		ExecuteLifecycle:     recordNoopExecuteLifecycle,
		CollectProvenance:    recordNoopCollectProvenance,
		PersistFinalEvidence: nil, // exactly one nil
		VerifyEvidenceBytes:  recordNoopVerifyEvidenceBytes,
		WriteManifest:        recordNoopWriteManifest,
		WriteChecksums:       recordNoopWriteChecksums,
	}
	opts := validOptions(t)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)

	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, ErrNilDependency) {
		t.Errorf("errors.Is(err, ErrNilDependency): got %v", err)
	}
}

func TestProductionFinalize_NilVerifyEvidenceBytesRejected(t *testing.T) {
	deps := ProductionQualifiedRunDependencies{
		ExecuteLifecycle:     recordNoopExecuteLifecycle,
		CollectProvenance:    recordNoopCollectProvenance,
		PersistFinalEvidence: recordNoopPersistFinalEvidence,
		VerifyEvidenceBytes:  nil, // exactly one nil
		WriteManifest:        recordNoopWriteManifest,
		WriteChecksums:       recordNoopWriteChecksums,
	}
	opts := validOptions(t)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)

	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, ErrNilDependency) {
		t.Errorf("errors.Is(err, ErrNilDependency): got %v", err)
	}
}

func TestProductionFinalize_NilWriteManifestRejected(t *testing.T) {
	deps := ProductionQualifiedRunDependencies{
		ExecuteLifecycle:     recordNoopExecuteLifecycle,
		CollectProvenance:    recordNoopCollectProvenance,
		PersistFinalEvidence: recordNoopPersistFinalEvidence,
		VerifyEvidenceBytes:  recordNoopVerifyEvidenceBytes,
		WriteManifest:        nil, // exactly one nil
		WriteChecksums:       recordNoopWriteChecksums,
	}
	opts := validOptions(t)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)

	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, ErrNilDependency) {
		t.Errorf("errors.Is(err, ErrNilDependency): got %v", err)
	}
}

func TestProductionFinalize_NilWriteChecksumsRejected(t *testing.T) {
	deps := ProductionQualifiedRunDependencies{
		ExecuteLifecycle:     recordNoopExecuteLifecycle,
		CollectProvenance:     recordNoopCollectProvenance,
		PersistFinalEvidence:  recordNoopPersistFinalEvidence,
		VerifyEvidenceBytes:   recordNoopVerifyEvidenceBytes,
		WriteManifest:         recordNoopWriteManifest,
		WriteChecksums:        nil, // exactly one nil
	}
	opts := validOptions(t)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)

	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, ErrNilDependency) {
		t.Errorf("errors.Is(err, ErrNilDependency): got %v", err)
	}
}

func TestProductionFinalize_NilDependenciesRejectedBeforeLifecycle(t *testing.T) {
	lifecycleCalled := 0
	deps := ProductionQualifiedRunDependencies{
		ExecuteLifecycle: func(ctx context.Context, opts dockerlab.LifecycleOptions, version string) (*dockerlab.QualifiedLifecycleOutcome, error) {
			lifecycleCalled++
			return nil, errors.New("lifecycle should not be called")
		},
		CollectProvenance:    recordNoopCollectProvenance,
		PersistFinalEvidence: recordNoopPersistFinalEvidence,
		VerifyEvidenceBytes:  recordNoopVerifyEvidenceBytes,
		WriteManifest:        recordNoopWriteManifest,
		WriteChecksums:       recordNoopWriteChecksums,
	}
	opts := validOptions(t)
	deps.PersistFinalEvidence = nil // trigger nil dependency

	_, _ = FinalizeProductionQualifiedRun(context.Background(), deps, opts)

	if lifecycleCalled != 0 {
		t.Errorf("lifecycle called %d times, want 0", lifecycleCalled)
	}
}

// =============================================================================
// P0-4: Path-authoritative publishers
// =============================================================================

func TestProductionFinalize_ManifestWriterReceivesExactPath(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-001"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	var receivedPath string
	var receivedManifest *Manifest
	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")
	evidence := minimalEvidence(true)

	deps := validDependencies(t, artifactRoot, runID, evidencePath, evidence)
	deps.WriteManifest = func(path string, manifest *Manifest) error {
		receivedPath = path
		receivedManifest = manifest
		return writeTestManifest(path, manifest)
	}

	// The manifest path should be <artifactRoot>/<runID>/manifest.json
	expectedPath := filepath.Join(artifactRoot, runID, "manifest.json")

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err != nil {
		t.Fatalf("FinalizeProductionQualifiedRun: %v", err)
	}

	if receivedPath != expectedPath {
		t.Errorf("manifest path: got %q, want %q", receivedPath, expectedPath)
	}
	if receivedManifest == nil {
		t.Error("receivedManifest is nil")
	}
}

func TestProductionFinalize_ChecksumWriterReceivesExactPath(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-002"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	var receivedPath string
	var receivedArtifactRoot string
	var receivedInventory []string
	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")
	evidence := minimalEvidence(true)

	deps := validDependencies(t, artifactRoot, runID, evidencePath, evidence)
	deps.WriteChecksums = func(path, artifactRootArg string, inventory []string) error {
		receivedPath = path
		receivedArtifactRoot = artifactRootArg
		receivedInventory = inventory
		return writeTestChecksums(path, artifactRootArg, inventory, evidencePath)
	}

	// The checksum path should be <artifactRoot>/<runID>/checksums.txt
	expectedPath := filepath.Join(artifactRoot, runID, "checksums.txt")

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err != nil {
		t.Fatalf("FinalizeProductionQualifiedRun: %v", err)
	}

	if receivedPath != expectedPath {
		t.Errorf("checksum path: got %q, want %q", receivedPath, expectedPath)
	}
	// P0-2: Writer must receive the run directory (artifactPath), not the parent artifacts directory.
	// Inventory entries are relative to the run directory.
	expectedArtifactRoot := filepath.Join(artifactRoot, runID)
	if receivedArtifactRoot != expectedArtifactRoot {
		t.Errorf("artifactRoot: got %q, want %q (run directory, not parent)", receivedArtifactRoot, expectedArtifactRoot)
	}
	if receivedInventory == nil {
		t.Error("receivedInventory is nil")
	}
}

func TestProductionFinalize_ManifestWriterReceivesRunDirectoryPath(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-003"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	var receivedPath string
	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")
	evidence := minimalEvidence(true)

	deps := validDependencies(t, artifactRoot, runID, evidencePath, evidence)
	deps.WriteManifest = func(path string, manifest *Manifest) error {
		receivedPath = path
		return writeTestManifest(path, manifest)
	}

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err != nil {
		t.Fatalf("FinalizeProductionQualifiedRun: %v", err)
	}

	// Path must contain runID
	if !strings.Contains(receivedPath, runID) {
		t.Errorf("manifest path does not contain runID: %q", receivedPath)
	}
}

func TestProductionFinalize_ChecksumWriterReceivesRunDirectoryPath(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-004"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	var receivedPath string
	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")
	evidence := minimalEvidence(true)

	deps := validDependencies(t, artifactRoot, runID, evidencePath, evidence)
	deps.WriteChecksums = func(path, artifactRootArg string, inventory []string) error {
		receivedPath = path
		return writeTestChecksums(path, artifactRootArg, inventory, evidencePath)
	}

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err != nil {
		t.Fatalf("FinalizeProductionQualifiedRun: %v", err)
	}

	// Path must contain runID
	if !strings.Contains(receivedPath, runID) {
		t.Errorf("checksum path does not contain runID: %q", receivedPath)
	}
}

func TestProductionFinalize_ManifestWriteFailurePropagates(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-005"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	expectedErr := errors.New("manifest write failed")
	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")
	evidence := minimalEvidence(true)

	deps := validDependencies(t, artifactRoot, runID, evidencePath, evidence)
	deps.WriteManifest = func(path string, manifest *Manifest) error {
		return expectedErr
	}

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), "write manifest") {
		t.Errorf("error message: %v", err)
	}
}

func TestProductionFinalize_ChecksumWriteFailurePropagates(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-006"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	expectedErr := errors.New("checksum write failed")
	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")
	evidence := minimalEvidence(true)

	deps := validDependencies(t, artifactRoot, runID, evidencePath, evidence)
	deps.WriteChecksums = func(path, artifactRootArg string, inventory []string) error {
		return expectedErr
	}

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), "write checksums") {
		t.Errorf("error message: %v", err)
	}
}

// =============================================================================
// P0-5: Complete evidence binding
// =============================================================================

func TestProductionFinalize_ReturnedPersistedEvidenceMatch(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-bind-001"

	deps, evidencePath := createBindingTestDependencies(t, artifactRoot, runID, runID)

	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)
	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)

	result, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err != nil {
		t.Fatalf("FinalizeProductionQualifiedRun: %v", err)
	}

	// Verify returned and persisted match
	if result.Evidence == nil {
		t.Fatal("result.Evidence is nil")
	}
	if result.EvidenceBytes == nil {
		t.Fatal("result.EvidenceBytes is nil")
	}

	// Verify evidence file exists
	if _, err := os.Stat(evidencePath); err != nil {
		t.Errorf("evidence file not found: %v", err)
	}
}

func TestProductionFinalize_ReturnedPersistedSchemaMismatchRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-bind-002"

	// Returned evidence has different schema version than persisted
	returnedEvidence := minimalEvidence(true)
	returnedEvidence.SchemaVersion = "0.0.0-RETURNED"
	persistedEvidence := minimalEvidence(true)
	persistedEvidence.SchemaVersion = "0.0.0-PERSISTED" // different

	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")

	deps := createMismatchedBindingTestDependencies(t, artifactRoot, runID, evidencePath, returnedEvidence, persistedEvidence)

	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)
	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for schema mismatch")
	}
	if !errors.Is(err, ErrProductionEvidenceMismatch) {
		t.Errorf("errors.Is(err, ErrProductionEvidenceMismatch): got %v", err)
	}
}

func TestProductionFinalize_ReturnedPersistedSourceCommitMismatchRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-bind-003"

	// Returned evidence has different source commit than persisted
	returnedEvidence := minimalEvidence(true)
	returnedEvidence.Provenance.SourceCommit = "aaa0000000000000000000000000000000000000"
	persistedEvidence := minimalEvidence(true)
	persistedEvidence.Provenance.SourceCommit = "bbb0000000000000000000000000000000000000"

	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")

	deps := createMismatchedBindingTestDependencies(t, artifactRoot, runID, evidencePath, returnedEvidence, persistedEvidence)

	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)
	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for source commit mismatch")
	}
	if !errors.Is(err, ErrProductionEvidenceMismatch) {
		t.Errorf("errors.Is(err, ErrProductionEvidenceMismatch): got %v", err)
	}
}

func TestProductionFinalize_ReturnedPersistedImageMismatchRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-bind-004"

	// Returned evidence says true, persisted says false
	returnedEvidence := minimalEvidence(true)
	returnedEvidence.ImageExactIDMatch = true
	persistedEvidence := minimalEvidence(true)
	persistedEvidence.ImageExactIDMatch = false

	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")

	deps := createMismatchedBindingTestDependencies(t, artifactRoot, runID, evidencePath, returnedEvidence, persistedEvidence)

	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)
	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for image mismatch")
	}
	if !errors.Is(err, ErrProductionEvidenceMismatch) {
		t.Errorf("errors.Is(err, ErrProductionEvidenceMismatch): got %v", err)
	}
}

func TestProductionFinalize_PersistedUnknownFieldRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-bind-005"

	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)
	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)

	// Write evidence with unknown field
	badJSON := `{
		"schema_version": "1.0.0",
		"image_exact_id_match": true,
		"network_exact_id_match": true,
		"cleanup_complete": true,
		"pass": true,
		"unknown_field": "should be rejected"
	}`

	if err := os.WriteFile(evidencePath, []byte(badJSON), 0644); err != nil {
		t.Fatalf("write evidence: %v", err)
	}

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for unknown field")
	}
}

func TestProductionFinalize_PersistedSecondDocumentRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-bind-006"

	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)
	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)

	// Write evidence with second JSON document
	badJSON := `{"schema_version":"1.0.0","image_exact_id_match":true,"network_exact_id_match":true,"cleanup_complete":true,"pass":true}{"schema_version":"1.0.0","image_exact_id_match":true}`

	if err := os.WriteFile(evidencePath, []byte(badJSON), 0644); err != nil {
		t.Fatalf("write evidence: %v", err)
	}

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for second document")
	}
}

func TestProductionFinalize_PersistedTrailingDataRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-bind-007"

	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)
	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)

	// Write evidence with trailing data
	badJSON := `{"schema_version":"1.0.0","image_exact_id_match":true,"network_exact_id_match":true,"cleanup_complete":true,"pass":true} trailing garbage`

	if err := os.WriteFile(evidencePath, []byte(badJSON), 0644); err != nil {
		t.Fatalf("write evidence: %v", err)
	}

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for trailing data")
	}
}

// =============================================================================
// P0-6: Strict physical manifest verification
// =============================================================================

func TestProductionFinalize_PhysicalManifestContainsEvidenceExactlyOnce(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-manifest-001"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)
	deps.WriteManifest = func(path string, manifest *Manifest) error {
		return writeTestManifest(path, manifest)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err != nil {
		t.Fatalf("FinalizeProductionQualifiedRun: %v", err)
	}

	// Verify manifest contains evidence exactly once
	manifestPath := filepath.Join(artifactRoot, runID, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}

	count := 0
	for _, item := range manifest.ArtifactInventory {
		if item == "qualified-execution-evidence.json" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("evidence count in manifest: got %d, want 1", count)
	}
}

func TestProductionFinalize_PhysicalManifestWrongRunIDRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-manifest-002"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	// Write a manifest with wrong run ID
	deps.WriteManifest = func(path string, manifest *Manifest) error {
		manifest.RunID = "wrong-run-id"
		return writeTestManifest(path, manifest)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for wrong run ID")
	}
}

func TestProductionFinalize_PhysicalManifestUnknownFieldRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-manifest-003"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	// Write a manifest with unknown field
	deps.WriteManifest = func(path string, manifest *Manifest) error {
		return os.WriteFile(path, []byte(`{
			"schema_version": "1.1.0",
			"run_id": "`+runID+`",
			"scenario": "test-scenario",
			"artifact_inventory": ["qualified-execution-evidence.json"],
			"unknown_field": "should be rejected"
		}`), 0644)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for unknown field in manifest")
	}
}

// =============================================================================
// P0-7: Strict checksum verification
// =============================================================================

func TestProductionFinalize_PhysicalChecksumsContainEvidence(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-checksum-001"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)
	deps.WriteChecksums = func(path, artifactRootArg string, inventory []string) error {
		return writeTestChecksums(path, artifactRootArg, inventory, evidencePath)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err != nil {
		t.Fatalf("FinalizeProductionQualifiedRun: %v", err)
	}

	// Verify checksums contain evidence
	checksumsPath := filepath.Join(artifactRoot, runID, "checksums.txt")
	checksumData, err := os.ReadFile(checksumsPath)
	if err != nil {
		t.Fatalf("read checksums: %v", err)
	}

	if !strings.Contains(string(checksumData), "qualified-execution-evidence.json") {
		t.Error("checksums do not contain evidence path")
	}
}

func TestProductionFinalize_EvidenceChecksumMatchesBytes(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-checksum-002"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)
	deps.WriteChecksums = func(path, artifactRootArg string, inventory []string) error {
		return writeTestChecksums(path, artifactRootArg, inventory, evidencePath)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err != nil {
		t.Fatalf("FinalizeProductionQualifiedRun: %v", err)
	}

	// Verify checksum matches actual bytes
	evidenceBytes, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatalf("read evidence: %v", err)
	}

	expectedDigest := sha256.Sum256(evidenceBytes)
	expectedHex := hex.EncodeToString(expectedDigest[:])

	checksumsPath := filepath.Join(artifactRoot, runID, "checksums.txt")
	checksumData, err := os.ReadFile(checksumsPath)
	if err != nil {
		t.Fatalf("read checksums: %v", err)
	}

	if !strings.Contains(string(checksumData), expectedHex) {
		t.Errorf("checksums do not contain correct digest: %s", expectedHex)
	}
}

func TestProductionFinalize_EvidenceChecksumMismatchRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-checksum-003"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	// Write wrong checksum
	deps.WriteChecksums = func(path, artifactRootArg string, inventory []string) error {
		return os.WriteFile(path, []byte("0000000000000000000000000000000000000000000000000000000000000000  qualified-execution-evidence.json\n"), 0644)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for checksum mismatch")
	}
}

func TestProductionFinalize_MalformedChecksumsRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-checksum-004"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	// Write malformed checksums (missing hex digest)
	deps.WriteChecksums = func(path, artifactRootArg string, inventory []string) error {
		return os.WriteFile(path, []byte("not-a-hex-digest  qualified-execution-evidence.json\n"), 0644)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	// Create the directory structure
	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for malformed checksums")
	}
}

// =============================================================================
// Helper functions
// =============================================================================

func validOptions(t *testing.T) ProductionQualifiedRunOptions {
	return ProductionQualifiedRunOptions{
		RepositoryRoot:  "/fake/repo",
		ArtifactRoot:    t.TempDir(),
		RunID:           "test-run",
		Scenario:        "test-scenario",
		ProducerVersion: "1.0.0",
		DockerVersion:   "25.0.3",
		LifecycleOptions: dockerlab.LifecycleOptions{},
		ExpectedInventory: []string{"qualified-execution-evidence.json"},
	}
}

func validOptionsWithArtifactRoot(t *testing.T, artifactRoot, runID string) ProductionQualifiedRunOptions {
	return ProductionQualifiedRunOptions{
		RepositoryRoot:  "/fake/repo",
		ArtifactRoot:    artifactRoot,
		RunID:           runID,
		Scenario:        "test-scenario",
		ProducerVersion: "1.0.0",
		DockerVersion:   "25.0.3",
		LifecycleOptions: dockerlab.LifecycleOptions{},
		ExpectedInventory: []string{"qualified-execution-evidence.json"},
	}
}

func recordNoopExecuteLifecycle(ctx context.Context, opts dockerlab.LifecycleOptions, version string) (*dockerlab.QualifiedLifecycleOutcome, error) {
	return nil, errors.New("should not be called")
}

func recordNoopCollectProvenance(opts ProvenanceOptions) (ControllerProvenance, error) {
	return ControllerProvenance{}, errors.New("should not be called")
}

func recordNoopPersistFinalEvidence(ctx context.Context, outcome *dockerlab.QualifiedLifecycleOutcome, cp ControllerProvenance, artifactDir string) (*QualifiedExecutionEvidence, error) {
	return nil, errors.New("should not be called")
}

func recordNoopVerifyEvidenceBytes(data []byte) (VerifyQualifiedExecutionResult, error) {
	return VerifyQualifiedExecutionResult{}, errors.New("should not be called")
}

func recordNoopWriteManifest(path string, manifest *Manifest) error {
	return errors.New("should not be called")
}

func recordNoopWriteChecksums(path, artifactRoot string, inventory []string) error {
	return errors.New("should not be called")
}

func minimalEvidence(pass bool) *QualifiedExecutionEvidence {
	return &QualifiedExecutionEvidence{
		SchemaVersion:      QualifiedExecutionSchemaVersion,
		ImageExactIDMatch:  true,
		NetworkExactIDMatch: true,
		CleanupComplete:    true,
		Pass:               pass,
		Provenance: ProvenanceBinding{
			SourceCommit:        "abc123def456abc123def456abc123def456abc1",
			SourceTree:          "clean",
			GitObjectFormat:     "sha256",
			WorkingTreeDirty:    false,
			SourceCommitDirty:   false,
			VCSModified:         false,
			DockerServerVersion: "25.0.3",
			ProducerVersion:     "1.0.0",
			ExecutableSHA256:     "0000000000000000000000000000000000000000000000000000000000000000",
		},
		Reachability: ReachabilityObservations{
			Success: true,
		},
		Pull: PullObservations{
			AttemptCount: 0,
		},
	}
}

func createBindingTestDependencies(t *testing.T, artifactRoot, runID, provenanceCommit string) (ProductionQualifiedRunDependencies, string) {
	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")
	evidence := minimalEvidence(true)

	return ProductionQualifiedRunDependencies{
		ExecuteLifecycle: func(ctx context.Context, opts dockerlab.LifecycleOptions, version string) (*dockerlab.QualifiedLifecycleOutcome, error) {
			return RecordOutcome(true, true, true), nil
		},
		CollectProvenance: func(opts ProvenanceOptions) (ControllerProvenance, error) {
			return ControllerProvenance{}, nil
		},
		PersistFinalEvidence: func(ctx context.Context, outcome *dockerlab.QualifiedLifecycleOutcome, cp ControllerProvenance, artifactDir string) (*QualifiedExecutionEvidence, error) {
			data, err := json.Marshal(evidence)
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(evidencePath, data, 0644); err != nil {
				return nil, err
			}
			return evidence, nil
		},
		VerifyEvidenceBytes: func(data []byte) (VerifyQualifiedExecutionResult, error) {
			return VerifyQualifiedExecutionResult{Pass: true}, nil
		},
		WriteManifest: func(path string, manifest *Manifest) error {
			return writeTestManifest(path, manifest)
		},
		WriteChecksums: func(path, artifactRootArg string, inventory []string) error {
			return writeTestChecksums(path, artifactRootArg, inventory, evidencePath)
		},
	}, evidencePath
}

func validDependencies(t *testing.T, artifactRoot, runID string, evidencePath string, evidence *QualifiedExecutionEvidence) ProductionQualifiedRunDependencies {
	return ProductionQualifiedRunDependencies{
		ExecuteLifecycle: func(ctx context.Context, opts dockerlab.LifecycleOptions, version string) (*dockerlab.QualifiedLifecycleOutcome, error) {
			return RecordOutcome(true, true, true), nil
		},
		CollectProvenance: func(opts ProvenanceOptions) (ControllerProvenance, error) {
			return ControllerProvenance{}, nil
		},
		PersistFinalEvidence: func(ctx context.Context, outcome *dockerlab.QualifiedLifecycleOutcome, cp ControllerProvenance, artifactDir string) (*QualifiedExecutionEvidence, error) {
			data, _ := json.Marshal(evidence)
			_ = os.WriteFile(evidencePath, data, 0644)
			return evidence, nil
		},
		VerifyEvidenceBytes: func(data []byte) (VerifyQualifiedExecutionResult, error) {
			return VerifyQualifiedExecutionResult{Pass: true}, nil
		},
		WriteManifest: func(path string, manifest *Manifest) error {
			return writeTestManifest(path, manifest)
		},
		WriteChecksums: func(path, artifactRootArg string, inventory []string) error {
			return writeTestChecksums(path, artifactRootArg, inventory, evidencePath)
		},
	}
}

func createMismatchedBindingTestDependencies(t *testing.T, artifactRoot, runID string, evidencePath string, returnedEvidence, persistedEvidence *QualifiedExecutionEvidence) ProductionQualifiedRunDependencies {
	return ProductionQualifiedRunDependencies{
		ExecuteLifecycle: func(ctx context.Context, opts dockerlab.LifecycleOptions, version string) (*dockerlab.QualifiedLifecycleOutcome, error) {
			return RecordOutcome(true, true, true), nil
		},
		CollectProvenance: func(opts ProvenanceOptions) (ControllerProvenance, error) {
			return ControllerProvenance{}, nil
		},
		PersistFinalEvidence: func(ctx context.Context, outcome *dockerlab.QualifiedLifecycleOutcome, cp ControllerProvenance, artifactDir string) (*QualifiedExecutionEvidence, error) {
			// Persist the persistedEvidence, but return returnedEvidence
			data, _ := json.Marshal(persistedEvidence)
			_ = os.WriteFile(evidencePath, data, 0644)
			return returnedEvidence, nil
		},
		VerifyEvidenceBytes: func(data []byte) (VerifyQualifiedExecutionResult, error) {
			return VerifyQualifiedExecutionResult{Pass: true}, nil
		},
		WriteManifest: func(path string, manifest *Manifest) error {
			return writeTestManifest(path, manifest)
		},
		WriteChecksums: func(path, artifactRootArg string, inventory []string) error {
			return writeTestChecksums(path, artifactRootArg, inventory, evidencePath)
		},
	}
}

func createStrictBindingTestDependencies(t *testing.T, artifactRoot, runID string) (ProductionQualifiedRunDependencies, string) {
	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")
	evidence := minimalEvidence(true)

	return ProductionQualifiedRunDependencies{
		ExecuteLifecycle: func(ctx context.Context, opts dockerlab.LifecycleOptions, version string) (*dockerlab.QualifiedLifecycleOutcome, error) {
			return RecordOutcome(true, true, true), nil
		},
		CollectProvenance: func(opts ProvenanceOptions) (ControllerProvenance, error) {
			return ControllerProvenance{}, nil
		},
		PersistFinalEvidence: func(ctx context.Context, outcome *dockerlab.QualifiedLifecycleOutcome, cp ControllerProvenance, artifactDir string) (*QualifiedExecutionEvidence, error) {
			return evidence, nil
		},
		VerifyEvidenceBytes: func(data []byte) (VerifyQualifiedExecutionResult, error) {
			return VerifyQualifiedExecutionResult{Pass: true}, nil
		},
		WriteManifest: func(path string, manifest *Manifest) error {
			return writeTestManifest(path, manifest)
		},
		WriteChecksums: func(path, artifactRootArg string, inventory []string) error {
			return writeTestChecksums(path, artifactRootArg, inventory, evidencePath)
		},
	}, evidencePath
}

func validDependenciesWithManifestWriter(t *testing.T, artifactRoot, runID string, inventory []string) ProductionQualifiedRunDependencies {
	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")
	evidence := minimalEvidence(true)

	return ProductionQualifiedRunDependencies{
		ExecuteLifecycle: func(ctx context.Context, opts dockerlab.LifecycleOptions, version string) (*dockerlab.QualifiedLifecycleOutcome, error) {
			return RecordOutcome(true, true, true), nil
		},
		CollectProvenance: func(opts ProvenanceOptions) (ControllerProvenance, error) {
			return ControllerProvenance{}, nil
		},
		PersistFinalEvidence: func(ctx context.Context, outcome *dockerlab.QualifiedLifecycleOutcome, cp ControllerProvenance, artifactDir string) (*QualifiedExecutionEvidence, error) {
			data, _ := json.Marshal(evidence)
			_ = os.WriteFile(evidencePath, data, 0644)
			return evidence, nil
		},
		VerifyEvidenceBytes: func(data []byte) (VerifyQualifiedExecutionResult, error) {
			return VerifyQualifiedExecutionResult{Pass: true}, nil
		},
		WriteManifest: func(path string, manifest *Manifest) error {
			return writeTestManifest(path, manifest)
		},
		WriteChecksums: func(path, artifactRootArg string, inventory []string) error {
			return writeTestChecksums(path, artifactRootArg, inventory, evidencePath)
		},
	}
}

func validDependenciesWithChecksumWriter(t *testing.T, artifactRoot, runID string, inventory []string) ProductionQualifiedRunDependencies {
	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")
	evidence := minimalEvidence(true)

	return ProductionQualifiedRunDependencies{
		ExecuteLifecycle: func(ctx context.Context, opts dockerlab.LifecycleOptions, version string) (*dockerlab.QualifiedLifecycleOutcome, error) {
			return RecordOutcome(true, true, true), nil
		},
		CollectProvenance: func(opts ProvenanceOptions) (ControllerProvenance, error) {
			return ControllerProvenance{}, nil
		},
		PersistFinalEvidence: func(ctx context.Context, outcome *dockerlab.QualifiedLifecycleOutcome, cp ControllerProvenance, artifactDir string) (*QualifiedExecutionEvidence, error) {
			data, _ := json.Marshal(evidence)
			_ = os.WriteFile(evidencePath, data, 0644)
			return evidence, nil
		},
		VerifyEvidenceBytes: func(data []byte) (VerifyQualifiedExecutionResult, error) {
			return VerifyQualifiedExecutionResult{Pass: true}, nil
		},
		WriteManifest: func(path string, manifest *Manifest) error {
			return writeTestManifest(path, manifest)
		},
		WriteChecksums: func(path, artifactRootArg string, inventory []string) error {
			return writeTestChecksums(path, artifactRootArg, inventory, evidencePath)
		},
	}
}

func persistEvidence(t *testing.T, path string, evidence *QualifiedExecutionEvidence) {
	data, err := json.Marshal(evidence)
	if err != nil {
		t.Fatalf("marshal evidence: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write evidence: %v", err)
	}
}

func writeTestManifest(path string, manifest *Manifest) error {
	manifest.SchemaVersion = "1.1.0"
	if manifest.ArtifactInventory == nil {
		manifest.ArtifactInventory = []string{}
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// =============================================================================
// P0-4B: Error cause preservation tests
// =============================================================================

func TestProductionFinalize_ManifestWriteFailurePreservesCause(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-err-001"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	expectedErr := errors.New("manifest permission denied")
	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")
	evidence := minimalEvidence(true)

	deps := validDependencies(t, artifactRoot, runID, evidencePath, evidence)
	deps.WriteManifest = func(path string, manifest *Manifest) error {
		return expectedErr
	}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("errors.Is(err, expectedErr): got %v", err)
	}
}

func TestProductionFinalize_ChecksumWriteFailurePreservesCause(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-err-002"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	expectedErr := errors.New("checksum permission denied")
	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")
	evidence := minimalEvidence(true)

	deps := validDependencies(t, artifactRoot, runID, evidencePath, evidence)
	deps.WriteChecksums = func(path, artifactRootArg string, inventory []string) error {
		return expectedErr
	}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("errors.Is(err, expectedErr): got %v", err)
	}
}

// =============================================================================
// P0-5A: Canonical exact-one JSON decoder tests
// =============================================================================

func TestDecodeQualifiedExecutionEvidenceExactlyOne_EmptyRejected(t *testing.T) {
	_, err := DecodeQualifiedExecutionEvidenceExactlyOne([]byte{})
	if err == nil {
		t.Fatal("expected non-nil error for empty input")
	}
}

func TestDecodeQualifiedExecutionEvidenceExactlyOne_UnknownFieldRejected(t *testing.T) {
	badJSON := `{"schema_version":"1.0.0","image_exact_id_match":true,"network_exact_id_match":true,"cleanup_complete":true,"pass":true,"unknown_field":"rejected"}`
	_, err := DecodeQualifiedExecutionEvidenceExactlyOne([]byte(badJSON))
	if err == nil {
		t.Fatal("expected non-nil error for unknown field")
	}
}

func TestDecodeQualifiedExecutionEvidenceExactlyOne_SecondObjectRejected(t *testing.T) {
	badJSON := `{"schema_version":"1.0.0","image_exact_id_match":true,"network_exact_id_match":true,"cleanup_complete":true,"pass":true}{"schema_version":"1.0.0"}`
	_, err := DecodeQualifiedExecutionEvidenceExactlyOne([]byte(badJSON))
	if err == nil {
		t.Fatal("expected non-nil error for second object")
	}
}

func TestDecodeQualifiedExecutionEvidenceExactlyOne_SecondScalarRejected(t *testing.T) {
	badJSON := `{"schema_version":"1.0.0","image_exact_id_match":true,"network_exact_id_match":true,"cleanup_complete":true,"pass":true}123`
	_, err := DecodeQualifiedExecutionEvidenceExactlyOne([]byte(badJSON))
	if err == nil {
		t.Fatal("expected non-nil error for second scalar")
	}
}

func TestDecodeQualifiedExecutionEvidenceExactlyOne_TrailingGarbageRejected(t *testing.T) {
	badJSON := `{"schema_version":"1.0.0","image_exact_id_match":true,"network_exact_id_match":true,"cleanup_complete":true,"pass":true} GARBAGE`
	_, err := DecodeQualifiedExecutionEvidenceExactlyOne([]byte(badJSON))
	if err == nil {
		t.Fatal("expected non-nil error for trailing garbage")
	}
}

func TestDecodeQualifiedExecutionEvidenceExactlyOne_TrailingWhitespaceAccepted(t *testing.T) {
	goodJSON := `{"schema_version":"1.0.0","image_exact_id_match":true,"network_exact_id_match":true,"cleanup_complete":true,"pass":true}  `
	_, err := DecodeQualifiedExecutionEvidenceExactlyOne([]byte(goodJSON))
	if err != nil {
		t.Fatalf("unexpected error for trailing whitespace: %v", err)
	}
}

// =============================================================================
// P0-6: Complete manifest verification tests
// =============================================================================

func TestProductionFinalize_PhysicalManifestMissingEvidenceRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-manifest-004"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	// Write manifest WITHOUT evidence entry
	deps.WriteManifest = func(path string, manifest *Manifest) error {
		manifest.ArtifactInventory = []string{} // missing evidence
		return writeTestManifest(path, manifest)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for missing evidence in manifest")
	}
}

func TestProductionFinalize_PhysicalManifestDuplicateEvidenceRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-manifest-005"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	// Write manifest with duplicate evidence entry
	deps.WriteManifest = func(path string, manifest *Manifest) error {
		manifest.ArtifactInventory = []string{
			"qualified-execution-evidence.json",
			"other-file.json",
			"qualified-execution-evidence.json", // duplicate
		}
		return writeTestManifest(path, manifest)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for duplicate evidence")
	}
	if !errors.Is(err, ErrDuplicateInventoryEntry) {
		t.Errorf("errors.Is(err, ErrDuplicateInventoryEntry): got %v", err)
	}
}

func TestProductionFinalize_PhysicalManifestWrongSchemaRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-manifest-006"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	// Write manifest with wrong schema version
	deps.WriteManifest = func(path string, manifest *Manifest) error {
		return os.WriteFile(path, []byte(`{
			"schema_version": "0.0.0-WRONG",
			"run_id": "`+runID+`",
			"scenario": "test-scenario",
			"artifact_inventory": ["qualified-execution-evidence.json"]
		}`), 0644)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for wrong schema")
	}
}

func TestProductionFinalize_PhysicalManifestWrongScenarioRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-manifest-007"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	// Write manifest with wrong scenario
	deps.WriteManifest = func(path string, manifest *Manifest) error {
		manifest.Scenario = "wrong-scenario"
		return writeTestManifest(path, manifest)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for wrong scenario")
	}
}

func TestProductionFinalize_PhysicalManifestInventorySubstitutionRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-manifest-008"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	// Write manifest with different inventory item
	deps.WriteManifest = func(path string, manifest *Manifest) error {
		manifest.ArtifactInventory = []string{"different-file.json"} // substitution
		return writeTestManifest(path, manifest)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for inventory substitution")
	}
}

func TestProductionFinalize_PhysicalManifestSecondDocumentRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-manifest-009"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	// Write manifest with second document
	deps.WriteManifest = func(path string, manifest *Manifest) error {
		return os.WriteFile(path, []byte(`{"schema_version":"1.1.0","run_id":"`+runID+`","scenario":"test-scenario","artifact_inventory":["qualified-execution-evidence.json"]}{"schema_version":"1.1.0"}`), 0644)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for second document")
	}
}

func TestProductionFinalize_PhysicalManifestTrailingDataRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-manifest-010"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	// Write manifest with trailing data
	deps.WriteManifest = func(path string, manifest *Manifest) error {
		return os.WriteFile(path, []byte(`{"schema_version":"1.1.0","run_id":"`+runID+`","scenario":"test-scenario","artifact_inventory":["qualified-execution-evidence.json"]} GARBAGE`), 0644)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for trailing data")
	}
}

// =============================================================================
// P0-7: Complete checksum authority tests
// =============================================================================

func TestProductionFinalize_EvidenceChecksumMissingRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-checksum-005"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	// Write checksums WITHOUT evidence entry
	deps.WriteChecksums = func(path, artifactRootArg string, inventory []string) error {
		return os.WriteFile(path, []byte(""), 0644)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for missing evidence in checksums")
	}
}

func TestProductionFinalize_EvidenceChecksumDuplicateRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-checksum-006"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	// Write checksums with duplicate entry
	deps.WriteChecksums = func(path, artifactRootArg string, inventory []string) error {
		evBytes, _ := os.ReadFile(evidencePath)
		digest := sha256.Sum256(evBytes)
		digestHex := hex.EncodeToString(digest[:])
		content := digestHex + "  qualified-execution-evidence.json\n" + digestHex + "  qualified-execution-evidence.json\n"
		return os.WriteFile(path, []byte(content), 0644)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for duplicate checksum entry")
	}
}

func TestProductionFinalize_ChecksumExtraPathRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-checksum-007"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	// Write checksums with extra path not in inventory
	deps.WriteChecksums = func(path, artifactRootArg string, inventory []string) error {
		evBytes, _ := os.ReadFile(evidencePath)
		digest := sha256.Sum256(evBytes)
		digestHex := hex.EncodeToString(digest[:])
		content := digestHex + "  qualified-execution-evidence.json\n" + digestHex + "  extra-file.json\n"
		return os.WriteFile(path, []byte(content), 0644)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for extra path in checksums")
	}
}

func TestProductionFinalize_ChecksumInventorySubstitutionRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-checksum-008"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	// Write checksums with different content
	deps.WriteChecksums = func(path, artifactRootArg string, inventory []string) error {
		// Use wrong digest
		content := "0000000000000000000000000000000000000000000000000000000000000000  qualified-execution-evidence.json\n"
		return os.WriteFile(path, []byte(content), 0644)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for checksum mismatch")
	}
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Errorf("errors.Is(err, ErrChecksumMismatch): got %v", err)
	}
}

func TestProductionFinalize_ChecksumUppercaseDigestRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-checksum-009"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	// Write checksums with uppercase hex
	deps.WriteChecksums = func(path, artifactRootArg string, inventory []string) error {
		content := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA  qualified-execution-evidence.json\n"
		return os.WriteFile(path, []byte(content), 0644)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for uppercase digest")
	}
}

func TestProductionFinalize_ChecksumMalformedLineRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-checksum-010"
	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)

	evidence := minimalEvidence(true)
	deps, evidencePath := createStrictBindingTestDependencies(t, artifactRoot, runID)

	// Write checksums with malformed line
	deps.WriteChecksums = func(path, artifactRootArg string, inventory []string) error {
		content := "not-valid-format qualified-execution-evidence.json\n"
		return os.WriteFile(path, []byte(content), 0644)
	}

	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)
	persistEvidence(t, evidencePath, evidence)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for malformed checksum line")
	}
}

// =============================================================================
// P0-5: Additional field binding tests
// =============================================================================

func TestProductionFinalize_ReturnedPersistedReachabilityMismatchRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-bind-008"

	returnedEvidence := minimalEvidence(true)
	returnedEvidence.Reachability.Success = true
	persistedEvidence := minimalEvidence(true)
	persistedEvidence.Reachability.Success = false

	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")

	deps := createMismatchedBindingTestDependencies(t, artifactRoot, runID, evidencePath, returnedEvidence, persistedEvidence)

	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)
	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for reachability mismatch")
	}
	if !errors.Is(err, ErrProductionEvidenceMismatch) {
		t.Errorf("errors.Is(err, ErrProductionEvidenceMismatch): got %v", err)
	}
}

func TestProductionFinalize_ReturnedPersistedPullMismatchRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-bind-009"

	returnedEvidence := minimalEvidence(true)
	returnedEvidence.Pull.AttemptCount = 0
	persistedEvidence := minimalEvidence(true)
	persistedEvidence.Pull.AttemptCount = 1

	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")

	deps := createMismatchedBindingTestDependencies(t, artifactRoot, runID, evidencePath, returnedEvidence, persistedEvidence)

	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)
	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for pull mismatch")
	}
	if !errors.Is(err, ErrProductionEvidenceMismatch) {
		t.Errorf("errors.Is(err, ErrProductionEvidenceMismatch): got %v", err)
	}
}

func TestProductionFinalize_ReturnedPersistedNetworkMismatchRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-bind-010"

	returnedEvidence := minimalEvidence(true)
	returnedEvidence.NetworkExactIDMatch = true
	persistedEvidence := minimalEvidence(true)
	persistedEvidence.NetworkExactIDMatch = false

	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")

	deps := createMismatchedBindingTestDependencies(t, artifactRoot, runID, evidencePath, returnedEvidence, persistedEvidence)

	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)
	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for network mismatch")
	}
	if !errors.Is(err, ErrProductionEvidenceMismatch) {
		t.Errorf("errors.Is(err, ErrProductionEvidenceMismatch): got %v", err)
	}
}

func TestProductionFinalize_ReturnedPersistedCleanupMismatchRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-bind-011"

	returnedEvidence := minimalEvidence(true)
	returnedEvidence.CleanupComplete = true
	persistedEvidence := minimalEvidence(true)
	persistedEvidence.CleanupComplete = false

	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")

	deps := createMismatchedBindingTestDependencies(t, artifactRoot, runID, evidencePath, returnedEvidence, persistedEvidence)

	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)
	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for cleanup mismatch")
	}
	if !errors.Is(err, ErrProductionEvidenceMismatch) {
		t.Errorf("errors.Is(err, ErrProductionEvidenceMismatch): got %v", err)
	}
}

func TestProductionFinalize_ReturnedPersistedSourceTreeMismatchRejected(t *testing.T) {
	tmpDir := t.TempDir()
	artifactRoot := filepath.Join(tmpDir, "artifacts")
	runID := "test-run-bind-012"

	returnedEvidence := minimalEvidence(true)
	returnedEvidence.Provenance.SourceTree = "clean"
	persistedEvidence := minimalEvidence(true)
	persistedEvidence.Provenance.SourceTree = "dirty"

	evidencePath := filepath.Join(artifactRoot, runID, "qualified-execution-evidence.json")

	deps := createMismatchedBindingTestDependencies(t, artifactRoot, runID, evidencePath, returnedEvidence, persistedEvidence)

	opts := validOptionsWithArtifactRoot(t, artifactRoot, runID)
	opts.ExpectedInventory = []string{"qualified-execution-evidence.json"}

	_ = os.MkdirAll(filepath.Join(artifactRoot, runID), 0755)

	_, err := FinalizeProductionQualifiedRun(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected non-nil error for source tree mismatch")
	}
	if !errors.Is(err, ErrProductionEvidenceMismatch) {
		t.Errorf("errors.Is(err, ErrProductionEvidenceMismatch): got %v", err)
	}
}

func writeTestChecksums(path, artifactRoot string, inventory []string, evidencePath string) error {
	var lines []string
	for _, item := range inventory {
		if item == "qualified-execution-evidence.json" {
			evBytes, err := os.ReadFile(evidencePath)
			if err != nil {
				return err
			}
			digest := sha256.Sum256(evBytes)
			digestHex := hex.EncodeToString(digest[:])
			lines = append(lines, digestHex+"  "+item)
		}
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}
