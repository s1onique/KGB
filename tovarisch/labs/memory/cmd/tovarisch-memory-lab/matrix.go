// matrix.go — Canary Matrix Execution Types
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION02
//
// Matrix execution identity and evidence schema for the three-canary
// classification matrix (growing → bounded → descriptor).
//
// CORRECTION02 establishes a SINGLE authoritative reconstruction path:
// - One shared ReconstructMatrixVerdict function used by both matrix and verify-matrix
// - Canonical cleanup evidence bound to exact container/network/process identities
// - Real SameSchema reconstruction from verified child manifests
// - Complete field-by-field verdict comparison with field-specific diagnostics
// - Fail-closed CLI exit semantics
//
// Core invariant:
//   one committed implementation
//   → one controller process and executable
//   → one frozen canary image
//   → three fresh isolated subjects
//   → three independently reconstructed verdicts
//   → exact cross-run identity convergence
//   → exact classification matrix

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

// Matrix schema version
const MatrixSchemaVersion = "1.0.0"

// Required scenario order
var matrixScenarioOrder = []string{
	"canary-growing",
	"canary-bounded",
	"canary-descriptor",
}

// MatrixExecutionIdentity captures the frozen execution identity.
// This is captured once and passed into all three scenario executions.
type MatrixExecutionIdentity struct {
	// Git identity
	ImplementationCommitOID   string `json:"implementation_commit_oid"`
	ImplementationTreeOID     string `json:"implementation_tree_oid"`
	GitObjectFormat           string `json:"git_object_format"`

	// Controller identity
	ControllerPID             int    `json:"controller_pid"`
	ControllerExecutableSHA256 string `json:"controller_executable_sha256"`

	// Manifest schema version (all runs must use 1.1.0)
	RunManifestSchemaVersion string `json:"run_manifest_schema_version"`

	// Canary image identity (frozen during preflight)
	ImageReference          string   `json:"image_reference"`
	ImageID                string   `json:"image_id"`
	RepoDigests            []string `json:"repo_digests"`
	RepoDigestStatus       string   `json:"repo_digest_status"`

	// Canary source binding
	CanarySourceCommitOID     string `json:"canary_source_commit_oid"`
	CanaryRepositoryTreeOID string `json:"canary_repository_tree_oid"`
	CanarySourceSubtreeOID   string `json:"canary_source_subtree_oid"`
	CanaryBinarySHA256       string `json:"canary_binary_sha256"`
	CanaryBinarySHA256Label  string `json:"canary_binary_sha256_label"`
	CanaryRevisionLabel      string `json:"canary_revision_label"`
	CanaryTreeLabel         string `json:"canary_tree_label"`
	CanarySubtreeLabel      string `json:"canary_subtree_label"`

	// Host identity
	HostKernelRelease string `json:"host_kernel_release"`
	HostKernelVersion string `json:"host_kernel_version"`
	HostCgroupMode    string `json:"host_cgroup_mode"`

	// Docker identity
	DockerEngineVersion string `json:"docker_engine_version"`
	DockerAPIVersion   string `json:"docker_api_version"`

	// Configuration
	Thresholds   *analysis.Thresholds `json:"thresholds"`
	PhaseConfig  interface{}          `json:"phase_config"`
}

// MatrixManifest represents the matrix-level manifest.json.
type MatrixManifest struct {
	SchemaVersion     string                   `json:"schema_version"`
	MatrixID         string                   `json:"matrix_id"`
	StartedAt        time.Time                `json:"started_at"`
	FinishedAt       time.Time                `json:"finished_at"`
	ExecutionIdentity *MatrixExecutionIdentity `json:"execution_identity"`
	Runs             []MatrixRunDeclaration   `json:"runs"`
}

// MatrixRunDeclaration declares a single scenario run within the matrix.
type MatrixRunDeclaration struct {
	Index          int    `json:"index"`
	Scenario       string `json:"scenario"`
	RunID          string `json:"run_id"`
	Path           string `json:"path"`
	ChecksumsSHA256 string `json:"checksums_sha256"`
}

// MatrixVerdict represents the matrix-level verdict.json.
type MatrixVerdict struct {
	MatrixID       string                     `json:"matrix_id"`
	MatrixValid    bool                       `json:"matrix_valid"`
	ScenarioResults map[string]*ScenarioResult `json:"scenario_results"`
	CrossRunChecks *CrossRunChecks            `json:"cross_run_checks"`
	ChecksTotal    int                        `json:"checks_total"`
	ChecksPassed   int                        `json:"checks_passed"`
	ChecksFailed   int                        `json:"checks_failed"`
}

// ScenarioResult holds the verified result for one scenario.
type ScenarioResult struct {
	RunID    string `json:"run_id"`
	Verified bool   `json:"verified"`
	Overall  string `json:"overall"`
	Memory   string `json:"memory"`
	Resource string `json:"resource"`
	Semantic string `json:"semantic"`
}

// CrossRunChecks holds all cross-run identity convergence checks.
type CrossRunChecks struct {
	SameCommitTree         bool `json:"same_commit_tree"`
	SameControllerPID      bool `json:"same_controller_pid"`
	SameControllerHash    bool `json:"same_controller_hash"`
	SameSchema            bool `json:"same_schema"`
	SameThresholds        bool `json:"same_thresholds"`
	SamePhaseConfig       bool `json:"same_phase_config"`
	SameHostIdentity      bool `json:"same_host_identity"`
	SameDockerIdentity    bool `json:"same_docker_identity"`
	SameImageIdentity     bool `json:"same_image_identity"`
	SameCanaryBinary      bool `json:"same_canary_binary"`
	UniqueRunIDs          bool `json:"unique_run_ids"`
	UniqueSubjectProcesses bool `json:"unique_subject_processes"`
	UniqueContainerIDs    bool `json:"unique_container_ids"`
	FixedOrder            bool `json:"fixed_order"`
	NonOverlapping        bool `json:"non_overlapping"`
	CleanupComplete       bool `json:"cleanup_complete"`
	ChecksPassed          int  `json:"checks_passed"`
}

// NewMatrixExecutionIdentity creates a new frozen execution identity.
func NewMatrixExecutionIdentity(
	gitCommit, gitTree, gitFormat string,
	controllerPID int,
	controllerHash string,
	imageRef, imageID string,
	repoDigests []string,
	repoDigestStatus string,
	canaryCommit, canaryTree, canarySubtree string,
	canaryBinary, canaryBinaryLabel string,
	canaryRevLabel, canaryTreeLabel, canarySubtreeLabel string,
	kernelRelease, kernelVersion, cgroupMode string,
	dockerVersion, dockerAPIVersion string,
	thresholds *analysis.Thresholds,
	phaseConfig interface{},
) *MatrixExecutionIdentity {
	return &MatrixExecutionIdentity{
		ImplementationCommitOID:    gitCommit,
		ImplementationTreeOID:     gitTree,
		GitObjectFormat:          gitFormat,
		ControllerPID:            controllerPID,
		ControllerExecutableSHA256: controllerHash,
		RunManifestSchemaVersion: "1.1.0",
		ImageReference:          imageRef,
		ImageID:                imageID,
		RepoDigests:            repoDigests,
		RepoDigestStatus:       repoDigestStatus,
		CanarySourceCommitOID:    canaryCommit,
		CanaryRepositoryTreeOID:  canaryTree,
		CanarySourceSubtreeOID:   canarySubtree,
		CanaryBinarySHA256:       canaryBinary,
		CanaryBinarySHA256Label:  canaryBinaryLabel,
		CanaryRevisionLabel:      canaryRevLabel,
		CanaryTreeLabel:         canaryTreeLabel,
		CanarySubtreeLabel:      canarySubtreeLabel,
		HostKernelRelease:       kernelRelease,
		HostKernelVersion:       kernelVersion,
		HostCgroupMode:          cgroupMode,
		DockerEngineVersion:     dockerVersion,
		DockerAPIVersion:        dockerAPIVersion,
		Thresholds:              thresholds,
		PhaseConfig:             phaseConfig,
	}
}

// LoadMatrixManifest loads and parses matrix-manifest.json.
func LoadMatrixManifest(path string) (*MatrixManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read matrix manifest: %w", err)
	}
	var m MatrixManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse matrix manifest: %w", err)
	}
	return &m, nil
}

// LoadMatrixVerdict loads and parses matrix-verdict.json.
func LoadMatrixVerdict(path string) (*MatrixVerdict, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read matrix verdict: %w", err)
	}
	var v MatrixVerdict
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parse matrix verdict: %w", err)
	}
	return &v, nil
}

// ValidateMatrixRootGeometry verifies the matrix root has exactly the required files.
func ValidateMatrixRootGeometry(matrixDir string) error {
	entries, err := os.ReadDir(matrixDir)
	if err != nil {
		return fmt.Errorf("read matrix directory: %w", err)
	}

	expectedFiles := map[string]bool{
		"matrix-manifest.json": false,
		"matrix-verdict.json":  false,
		"matrix-checksums.txt": false,
	}
	var unexpectedFiles []string
	var runsFound bool

	for _, entry := range entries {
		if entry.IsDir() {
			if entry.Name() == "runs" {
				runsFound = true
			} else {
				unexpectedFiles = append(unexpectedFiles, entry.Name())
			}
			continue
		}
		name := entry.Name()
		if _, ok := expectedFiles[name]; !ok {
			unexpectedFiles = append(unexpectedFiles, name)
		} else {
			expectedFiles[name] = true
		}
	}

	for name, found := range expectedFiles {
		if !found {
			return fmt.Errorf("missing required file: %s", name)
		}
	}

	if !runsFound {
		return fmt.Errorf("missing required directory: runs")
	}

	if len(unexpectedFiles) > 0 {
		return fmt.Errorf("unexpected files in matrix root: %v", unexpectedFiles)
	}

	runsDir := filepath.Join(matrixDir, "runs")
	runsEntries, err := os.ReadDir(runsDir)
	if err != nil {
		return fmt.Errorf("read runs directory: %w", err)
	}

	if len(runsEntries) != 3 {
		return fmt.Errorf("expected 3 run directories, got %d", len(runsEntries))
	}

	return nil
}

// ValidateChildChecksums verifies child checksums are bound to matrix manifest.
func ValidateChildChecksums(matrixDir string, manifest *MatrixManifest) error {
	runsDir := filepath.Join(matrixDir, "runs")
	for _, entry := range manifest.Runs {
		runPath := filepath.Join(runsDir, entry.RunID)
		checksumPath := filepath.Join(runPath, "checksums.txt")

		data, err := os.ReadFile(checksumPath)
		if err != nil {
			return fmt.Errorf("read checksums.txt for run %s: %w", entry.RunID, err)
		}

		hash := computeSHA256Hex(data)
		if hash != entry.ChecksumsSHA256 {
			return fmt.Errorf("run %s checksums.txt hash mismatch: expected %s, got %s",
				entry.RunID, entry.ChecksumsSHA256, hash)
		}

		manifestPath := filepath.Join(runPath, "manifest.json")
		manifestData, err := os.ReadFile(manifestPath)
		if err != nil {
			return fmt.Errorf("read manifest for run %s: %w", entry.RunID, err)
		}
		var runManifest evidence.Manifest
		if err := json.Unmarshal(manifestData, &runManifest); err != nil {
			return fmt.Errorf("parse manifest for run %s: %w", entry.RunID, err)
		}

		if runManifest.Scenario != entry.Scenario {
			return fmt.Errorf("run %s scenario mismatch: manifest has %s, matrix declares %s",
				entry.RunID, runManifest.Scenario, entry.Scenario)
		}

		if filepath.Base(runPath) != entry.RunID {
			return fmt.Errorf("run directory name %s does not match run ID %s",
				filepath.Base(runPath), entry.RunID)
		}

		childEntries, err := os.ReadDir(runPath)
		if err != nil {
			return fmt.Errorf("read run directory %s: %w", entry.RunID, err)
		}

		expectedArtifacts := map[string]bool{
			"manifest.json":             false,
			"verdict.json":              false,
			"samples.csv":               false,
			"events.jsonl":              false,
			"container-inspect.json":     false,
			"container-logs.txt":        false,
			"initial-canary-state.json":  false,
			"final-canary-state.json":   false,
			"workload-result.json":      false,
			"checksums.txt":             false,
		}

		for _, e := range childEntries {
			if e.IsDir() {
				return fmt.Errorf("unexpected directory in run %s: %s", entry.RunID, e.Name())
			}
			if _, ok := expectedArtifacts[e.Name()]; !ok {
				return fmt.Errorf("unexpected file in run %s: %s", entry.RunID, e.Name())
			}
			expectedArtifacts[e.Name()] = true
		}

		for name, found := range expectedArtifacts {
			if !found {
				return fmt.Errorf("missing artifact %s in run %s", name, entry.RunID)
			}
		}
	}

	return nil
}

// ValidateMatrixChecksums verifies matrix-level checksums.
func ValidateMatrixChecksums(matrixDir string) error {
	checksumPath := filepath.Join(matrixDir, "matrix-checksums.txt")
	data, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("read matrix-checksums.txt: %w", err)
	}

	entries, err := evidence.ParseChecksumsFile(string(data))
	if err != nil {
		return fmt.Errorf("parse matrix-checksums.txt: %w", err)
	}

	manifestHash, ok := entries["matrix-manifest.json"]
	if !ok {
		return fmt.Errorf("missing checksum for matrix-manifest.json")
	}

	verdictHash, ok := entries["matrix-verdict.json"]
	if !ok {
		return fmt.Errorf("missing checksum for matrix-verdict.json")
	}

	manifestData, err := os.ReadFile(filepath.Join(matrixDir, "matrix-manifest.json"))
	if err != nil {
		return fmt.Errorf("read matrix-manifest.json: %w", err)
	}
	if computeSHA256Hex(manifestData) != manifestHash {
		return fmt.Errorf("matrix-manifest.json checksum mismatch")
	}

	verdictData, err := os.ReadFile(filepath.Join(matrixDir, "matrix-verdict.json"))
	if err != nil {
		return fmt.Errorf("read matrix-verdict.json: %w", err)
	}
	if computeSHA256Hex(verdictData) != verdictHash {
		return fmt.Errorf("matrix-verdict.json checksum mismatch")
	}

	if len(entries) != 2 {
		return fmt.Errorf("matrix-checksums.txt contains %d entries, expected 2", len(entries))
	}

	return nil
}

// CompareExecutionIdentity compares two execution identities for equality.
func CompareExecutionIdentity(a, b *MatrixExecutionIdentity) []string {
	var diffs []string

	if a.ImplementationCommitOID != b.ImplementationCommitOID {
		diffs = append(diffs, "implementation_commit_oid differs")
	}
	if a.ImplementationTreeOID != b.ImplementationTreeOID {
		diffs = append(diffs, "implementation_tree_oid differs")
	}
	if a.GitObjectFormat != b.GitObjectFormat {
		diffs = append(diffs, "git_object_format differs")
	}
	if a.ControllerPID != b.ControllerPID {
		diffs = append(diffs, "controller_pid differs")
	}
	if a.ControllerExecutableSHA256 != b.ControllerExecutableSHA256 {
		diffs = append(diffs, "controller_executable_sha256 differs")
	}
	if a.RunManifestSchemaVersion != b.RunManifestSchemaVersion {
		diffs = append(diffs, "run_manifest_schema_version differs")
	}
	if a.ImageReference != b.ImageReference {
		diffs = append(diffs, "image_reference differs")
	}
	if a.ImageID != b.ImageID {
		diffs = append(diffs, "image_id differs")
	}
	if !stringSliceEqual(a.RepoDigests, b.RepoDigests) {
		diffs = append(diffs, "repo_digests differs")
	}
	if a.RepoDigestStatus != b.RepoDigestStatus {
		diffs = append(diffs, "repo_digest_status differs")
	}
	if a.CanarySourceCommitOID != b.CanarySourceCommitOID {
		diffs = append(diffs, "canary_source_commit_oid differs")
	}
	if a.CanaryRepositoryTreeOID != b.CanaryRepositoryTreeOID {
		diffs = append(diffs, "canary_repository_tree_oid differs")
	}
	if a.CanarySourceSubtreeOID != b.CanarySourceSubtreeOID {
		diffs = append(diffs, "canary_source_subtree_oid differs")
	}
	if a.CanaryBinarySHA256 != b.CanaryBinarySHA256 {
		diffs = append(diffs, "canary_binary_sha256 differs")
	}
	if a.CanaryBinarySHA256Label != b.CanaryBinarySHA256Label {
		diffs = append(diffs, "canary_binary_sha256_label differs")
	}
	if a.CanaryRevisionLabel != b.CanaryRevisionLabel {
		diffs = append(diffs, "canary_revision_label differs")
	}
	if a.CanaryTreeLabel != b.CanaryTreeLabel {
		diffs = append(diffs, "canary_tree_label differs")
	}
	if a.CanarySubtreeLabel != b.CanarySubtreeLabel {
		diffs = append(diffs, "canary_subtree_label differs")
	}
	if a.HostKernelRelease != b.HostKernelRelease {
		diffs = append(diffs, "host_kernel_release differs")
	}
	if a.HostKernelVersion != b.HostKernelVersion {
		diffs = append(diffs, "host_kernel_version differs")
	}
	if a.HostCgroupMode != b.HostCgroupMode {
		diffs = append(diffs, "host_cgroup_mode differs")
	}
	if a.DockerEngineVersion != b.DockerEngineVersion {
		diffs = append(diffs, "docker_engine_version differs")
	}
	if a.DockerAPIVersion != b.DockerAPIVersion {
		diffs = append(diffs, "docker_api_version differs")
	}
	if !thresholdsEqual(*a.Thresholds, *b.Thresholds) {
		diffs = append(diffs, "thresholds differ")
	}

	return diffs
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func thresholdsEqual(a, b analysis.Thresholds) bool {
	return a.MemoryGrowthKibPerHour == b.MemoryGrowthKibPerHour &&
		a.MemoryGrowthPercentPerHour == b.MemoryGrowthPercentPerHour &&
		a.ResourceGrowthPerHour == b.ResourceGrowthPerHour &&
		a.CorroborationCount == b.CorroborationCount &&
		a.SampleCountMinimum == b.SampleCountMinimum &&
		a.WindowMinimum == b.WindowMinimum
}

// ExpectedClassificationMatrix holds the expected verdicts for each scenario.
var ExpectedClassificationMatrix = map[string]struct {
	Overall, Memory, Resource, Semantic string
}{
	"canary-growing":    {"growing", "growing", "stable", "stable"},
	"canary-bounded":    {"stable", "stable", "stable", "stable"},
	"canary-descriptor": {"resource_growth", "stable", "resource_growth", "stable"},
}

// ValidateScenarioContract validates a single scenario meets its contract.
func ValidateScenarioContract(
	scenario string,
	workload *WorkloadResult,
	initialState, finalState *CanaryState,
	verdict *evidence.Verdict,
) []string {
	var errors []string

	expected, ok := ExpectedClassificationMatrix[scenario]
	if !ok {
		errors = append(errors, fmt.Sprintf("unknown scenario: %s", scenario))
		return errors
	}

	switch scenario {
	case "canary-growing":
		if workload.Requested != 32 || workload.Attempted != 32 ||
			workload.Completed != 32 || workload.Failed != 0 || workload.Returned != 32 {
			errors = append(errors, fmt.Sprintf(
				"growing workload: expected 32/32/32/0/32, got %d/%d/%d/%d/%d",
				workload.Requested, workload.Attempted, workload.Completed, workload.Failed, workload.Returned))
		}
		if finalState.RetainedBlocks != 32 {
			errors = append(errors, fmt.Sprintf("growing retained_blocks: expected 32, got %d", finalState.RetainedBlocks))
		}
		if finalState.RetainedBytes != 33554432 {
			errors = append(errors, fmt.Sprintf("growing retained_bytes: expected 33554432, got %d", finalState.RetainedBytes))
		}
		if initialState.Mode != "growing" || finalState.Mode != "growing" {
			errors = append(errors, fmt.Sprintf("growing mode: expected growing/growing, got %s/%s",
				initialState.Mode, finalState.Mode))
		}

	case "canary-bounded":
		if workload.Requested != 100 || workload.Attempted != 100 ||
			workload.Completed != 100 || workload.Failed != 0 || workload.Returned != 100 {
			errors = append(errors, fmt.Sprintf(
				"bounded workload: expected 100/100/100/0/100, got %d/%d/%d/%d/%d",
				workload.Requested, workload.Attempted, workload.Completed, workload.Failed, workload.Returned))
		}
		if initialState.BufferCapacity != finalState.BufferCapacity {
			errors = append(errors, fmt.Sprintf("bounded buffer_capacity changed: %d -> %d",
				initialState.BufferCapacity, finalState.BufferCapacity))
		}
		if finalState.RetainedBlocks != 0 || finalState.RetainedBytes != 0 {
			errors = append(errors, fmt.Sprintf("bounded retained: expected 0/0, got %d/%d",
				finalState.RetainedBlocks, finalState.RetainedBytes))
		}
		if initialState.Mode != "bounded" || finalState.Mode != "bounded" {
			errors = append(errors, fmt.Sprintf("bounded mode: expected bounded/bounded, got %s/%s",
				initialState.Mode, finalState.Mode))
		}

	case "canary-descriptor":
		if workload.Requested != 100 || workload.Attempted != 100 ||
			workload.Completed != 100 || workload.Failed != 0 || workload.Returned != 100 {
			errors = append(errors, fmt.Sprintf(
				"descriptor workload: expected 100/100/100/0/100, got %d/%d/%d/%d/%d",
				workload.Requested, workload.Attempted, workload.Completed, workload.Failed, workload.Returned))
		}
		fdDelta := finalState.FDCount - initialState.FDCount
		if fdDelta != 200 {
			errors = append(errors, fmt.Sprintf("descriptor fd_delta: expected 200, got %d", fdDelta))
		}
		if finalState.RetainedBlocks != 0 || finalState.RetainedBytes != 0 {
			errors = append(errors, fmt.Sprintf("descriptor retained: expected 0/0, got %d/%d",
				finalState.RetainedBlocks, finalState.RetainedBytes))
		}
		if initialState.Mode != "descriptor" || finalState.Mode != "descriptor" {
			errors = append(errors, fmt.Sprintf("descriptor mode: expected descriptor/descriptor, got %s/%s",
				initialState.Mode, finalState.Mode))
		}
		descInvariantCount := 0
		for _, sig := range verdict.SignalSummaries {
			if sig.Name == "descriptor_state_invariant" {
				descInvariantCount++
				if sig.SampleCount != 2 {
					errors = append(errors, fmt.Sprintf("descriptor_state_invariant sample_count: expected 2, got %d", sig.SampleCount))
				}
				if sig.AvailableCount != 2 {
					errors = append(errors, fmt.Sprintf("descriptor_state_invariant available_count: expected 2, got %d", sig.AvailableCount))
				}
				if sig.MissingCount != 0 {
					errors = append(errors, fmt.Sprintf("descriptor_state_invariant missing_count: expected 0, got %d", sig.MissingCount))
				}
				if sig.AbsoluteDelta != 200 {
					errors = append(errors, fmt.Sprintf("descriptor_state_invariant absolute_delta: expected 200, got %d", sig.AbsoluteDelta))
				}
				if !sig.IsPrimary {
					errors = append(errors, "descriptor_state_invariant must be primary")
				}
			}
		}
		if descInvariantCount != 1 {
			errors = append(errors, fmt.Sprintf("descriptor_state_invariant count: expected 1, got %d", descInvariantCount))
		}
	}

	if verdict.OverallClassification != analysis.Classification(expected.Overall) {
		errors = append(errors, fmt.Sprintf("overall: expected %s, got %s", expected.Overall, verdict.OverallClassification))
	}
	if verdict.MemoryClassification != analysis.Classification(expected.Memory) {
		errors = append(errors, fmt.Sprintf("memory: expected %s, got %s", expected.Memory, verdict.MemoryClassification))
	}
	if verdict.ResourceClassification != analysis.Classification(expected.Resource) {
		errors = append(errors, fmt.Sprintf("resource: expected %s, got %s", expected.Resource, verdict.ResourceClassification))
	}
	if verdict.SemanticClassification != analysis.Classification(expected.Semantic) {
		errors = append(errors, fmt.Sprintf("semantic: expected %s, got %s", expected.Semantic, verdict.SemanticClassification))
	}
	if !verdict.ScenarioValid {
		errors = append(errors, "scenario_valid is false")
	}
	if !verdict.CanariesValid {
		errors = append(errors, "canaries_valid is false")
	}
	if !verdict.ProvenanceValid {
		errors = append(errors, fmt.Sprintf("provenance_valid is false: %s", verdict.ProvenanceError))
	}

	return errors
}

// computeSHA256Hex computes the SHA-256 hash and returns hex-encoded lowercase string.
func computeSHA256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
