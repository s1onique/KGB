// matrix_verify.go — Single Authority Matrix Verification
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03
//
// This file contains the SINGLE authoritative verification path for matrix bundles.
// Both `matrix` and `verify-matrix` commands MUST use VerifyMatrixBundle.
//
// CORRECTION03 establishes one complete verification authority:
// - One VerifyMatrixBundle function used by both commands
// - Authoritative child verification through VerifiedChildBundle
// - Exact runtime identity binding (container, network, process)
// - Observed cleanup verification (not asserted)
// - Complete verdict comparison with equal-invalid detection
// - Fail-closed terminal semantics

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence"
)

// =============================================================================
// VERIFIED CHILD BUNDLE
// =============================================================================

// VerifiedChildBundle represents a child run that has been verified by the
// authoritative child verifier. This is the only way to construct a VerifiedRun.
type VerifiedChildBundle struct {
	Manifest       *evidence.Manifest
	Verdict        *evidence.Verdict
	ContainerID    string
	ContainerName  string
	NetworkID      string
	NetworkName    string
	SubjectPID     int
	SubjectStart   uint64
	ChecksVerified bool
}

// =============================================================================
// DOCKER COMMAND SEAM
// =============================================================================

// DockerCommandResult represents the result of a Docker CLI command.
type DockerCommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// DockerRunner is a function type for executing Docker commands.
// Used by tests to inject mock Docker runners.
type DockerRunner func(ctx context.Context, args ...string) (DockerCommandResult, error)

// DefaultDockerRunner is the real Docker command executor.
func DefaultDockerRunner(ctx context.Context, args ...string) (DockerCommandResult, error) {
	// This will be implemented with the actual Docker client in cleanup_observation.go
	// For now, return error to indicate not implemented
	return DockerCommandResult{}, errors.New("use real Docker client")
}

// =============================================================================
// OBJECT CLEANUP STATUS TYPES
// =============================================================================

// ObjectCleanupStatus represents the conclusive state of container/network cleanup.
type ObjectCleanupStatus string

const (
	ObjectGone        ObjectCleanupStatus = "gone"
	ObjectExists      ObjectCleanupStatus = "exists"
	ObjectUnavailable ObjectCleanupStatus = "unavailable"
)

// ProcessCleanupStatusCode represents the conclusive state of process cleanup.
type ProcessCleanupStatusCode string

const (
	ProcessGoneCode        ProcessCleanupStatusCode = "gone"
	ProcessPIDReusedCode   ProcessCleanupStatusCode = "pid_reused"
	ProcessStillAliveCode  ProcessCleanupStatusCode = "still_alive"
	ProcessUnavailableCode ProcessCleanupStatusCode = "unavailable"
)

// =============================================================================
// RUNTIME IDENTITY OBSERVATION STRUCTURES
// =============================================================================

// ContainerIdentityObservation captures container identity from Docker inspection.
type ContainerIdentityObservation struct {
	ID     string
	Name   string
	Status ObjectCleanupStatus
}

// NetworkIdentityObservation captures network identity from Docker inspection.
type NetworkIdentityObservation struct {
	ID     string
	Name   string
	Status ObjectCleanupStatus
}

// ProcessIdentityObservation captures process identity from /proc.
type ProcessIdentityObservation struct {
	PID       int
	StartTime uint64
	Status    ProcessCleanupStatusCode
}

// RunCleanupObservation combines all cleanup observations for a single run.
type RunCleanupObservation struct {
	RunID     string
	Scenario  string
	Container ContainerIdentityObservation
	Network   NetworkIdentityObservation
	Process   ProcessIdentityObservation
}

// =============================================================================
// MATRIX VERIFICATION RESULT
// =============================================================================

// MatrixVerificationResult holds the complete verification output.
type MatrixVerificationResult struct {
	Manifest             *MatrixManifest
	StoredVerdict        *MatrixVerdict
	ReconstructedVerdict *MatrixVerdict
	VerifiedRuns         []*VerifiedRun
	Cleanup             *MatrixCleanupEvidence
	AllChildrenVerified bool
	ChecksumVerified    bool
	CleanupValid        bool
}

// MatrixVerificationDeps holds dependencies for matrix verification.
type MatrixVerificationDeps struct {
	VerifyChildRun func(runDir string) (*VerifiedChildBundle, error)
}

// =============================================================================
// VERIFY CHILD RUN - AUTHORITATIVE CHILD VERIFICATION
// =============================================================================

// verifyChildRunBundle is the authoritative child verification function.
// It performs complete verification of a child run bundle including:
// - Child geometry validation
// - Child checksum verification
// - Strict canonical artifact decoding
// - Manifest/verdict consistency
// - Child verdict contract validation
// - Identity extraction
func verifyChildRunBundle(runDir string) (*VerifiedChildBundle, error) {
	result := &VerifiedChildBundle{}

	// 1. Validate child geometry - check required files exist
	if err := validateChildGeometry(runDir); err != nil {
		return nil, fmt.Errorf("child geometry: %w", err)
	}

	// 2. Verify child checksums before semantic decoding
	if err := verifyChildChecksums(runDir); err != nil {
		return nil, fmt.Errorf("child checksums: %w", err)
	}

	// 3. Strict canonical artifact decoding
	// Load manifest.json with strict JSON parsing
	manifestData, err := os.ReadFile(filepath.Join(runDir, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var manifest evidence.Manifest
	if err := matrixDecodeStrictJSON(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	result.Manifest = &manifest

	// Load verdict.json with strict JSON parsing
	verdictData, err := os.ReadFile(filepath.Join(runDir, "verdict.json"))
	if err != nil {
		return nil, fmt.Errorf("read verdict: %w", err)
	}
	var verdict evidence.Verdict
	if err := matrixDecodeStrictJSON(verdictData, &verdict); err != nil {
		return nil, fmt.Errorf("parse verdict: %w", err)
	}
	result.Verdict = &verdict

	// 4. Validate manifest/verdict consistency
	if manifest.Scenario != verdict.Scenario {
		return nil, fmt.Errorf("manifest scenario %q != verdict scenario %q", manifest.Scenario, verdict.Scenario)
	}

	// 5. Validate child verdict contract
	if err := validateChildVerdictContract(&verdict); err != nil {
		return nil, fmt.Errorf("child verdict contract: %w", err)
	}

	// 6. Extract identities
	// Container identity from container-inspect.json
	containerData, err := os.ReadFile(filepath.Join(runDir, "container-inspect.json"))
	if err != nil {
		return nil, fmt.Errorf("read container-inspect: %w", err)
	}
	var containerInspect struct {
		ID string `json:"Id"`
	}
	if err := matrixDecodeStrictJSON(containerData, &containerInspect); err != nil {
		return nil, fmt.Errorf("parse container-inspect: %w", err)
	}
	result.ContainerID = containerInspect.ID

	// Process identity from samples.csv
	pid, startTime, err := extractProcessIdentity(filepath.Join(runDir, "samples.csv"))
	if err != nil {
		return nil, fmt.Errorf("extract process identity: %w", err)
	}
	result.SubjectPID = pid
	result.SubjectStart = startTime

	// Network identity - look for network-identity.json
	networkData, err := os.ReadFile(filepath.Join(runDir, "network-identity.json"))
	if err == nil {
		var networkIdentity struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := matrixDecodeStrictJSON(networkData, &networkIdentity); err == nil {
			result.NetworkID = networkIdentity.ID
			result.NetworkName = networkIdentity.Name
		}
	}

	result.ChecksVerified = true
	return result, nil
}

// validateChildGeometry checks that all required child artifacts exist.
func validateChildGeometry(runDir string) error {
	requiredFiles := []string{
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

	entries, err := os.ReadDir(runDir)
	if err != nil {
		return fmt.Errorf("read run directory: %w", err)
	}

	found := make(map[string]bool)
	for _, e := range entries {
		if !e.IsDir() {
			found[e.Name()] = true
		}
	}

	var missing []string
	for _, name := range requiredFiles {
		if !found[name] {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required files: %v", missing)
	}

	return nil
}

// verifyChildChecksums verifies child checksums match actual file contents.
func verifyChildChecksums(runDir string) error {
	checksumPath := filepath.Join(runDir, "checksums.txt")
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
		data, err := os.ReadFile(filepath.Join(runDir, name))
		if err != nil {
			return fmt.Errorf("read artifact %s: %w", name, err)
		}
		actualHash := sha256Hash(data)
		if actualHash != expectedHash {
			return fmt.Errorf("checksum mismatch for %s", name)
		}
	}

	return nil
}

// validateChildVerdictContract validates the child verdict meets its contract.
func validateChildVerdictContract(verdict *evidence.Verdict) error {
	// The verdict must have valid provenance
	if !verdict.ProvenanceValid {
		if verdict.ProvenanceError != "" {
			return fmt.Errorf("provenance invalid: %s", verdict.ProvenanceError)
		}
		return errors.New("provenance invalid")
	}

	// The verdict must have valid scenario
	if !verdict.ScenarioValid {
		return errors.New("scenario invalid")
	}

	// The verdict must have valid canaries
	if !verdict.CanariesValid {
		return errors.New("canaries invalid")
	}

	return nil
}

// =============================================================================
// STRICT JSON DECODING (matrix-specific helper)
// =============================================================================

// matrixDecodeStrictJSON parses JSON with DisallowUnknownFields and single-document enforcement.
// Uses the generic version from reconstruct.go for actual decoding.
func matrixDecodeStrictJSON(data []byte, dst any) error {
	if len(data) == 0 {
		return errors.New("empty input")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	// Ensure exactly one top-level JSON value
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("unexpected second JSON value in document")
		}
		return fmt.Errorf("trailing JSON data: %w", err)
	}

	return nil
}

// =============================================================================
// VERIFY MATRIX BUNDLE - SINGLE AUTHORITY
// =============================================================================

// VerifyMatrixBundle performs complete matrix verification.
// This is the SINGLE authoritative function for matrix verification.
func VerifyMatrixBundle(
	matrixDir string,
	deps MatrixVerificationDeps,
) (*MatrixVerificationResult, error) {
	result := &MatrixVerificationResult{}

	// 1. Validate matrix root geometry
	if err := ValidateMatrixRootGeometry(matrixDir); err != nil {
		return nil, fmt.Errorf("root geometry: %w", err)
	}

	// 2. Verify matrix checksums before semantic decoding
	if err := ValidateMatrixChecksums(matrixDir); err != nil {
		return nil, fmt.Errorf("matrix checksums: %w", err)
	}
	result.ChecksumVerified = true

	// 3. Strict matrix artifact decoding
	// Load matrix manifest
	manifestData, err := os.ReadFile(filepath.Join(matrixDir, "matrix-manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var manifest MatrixManifest
	if err := matrixDecodeStrictJSON(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	result.Manifest = &manifest

	// Validate matrix manifest contract
	if err := validateMatrixManifestContract(&manifest); err != nil {
		return nil, fmt.Errorf("manifest contract: %w", err)
	}

	// Load stored verdict
	verdictData, err := os.ReadFile(filepath.Join(matrixDir, "matrix-verdict.json"))
	if err != nil {
		return nil, fmt.Errorf("read verdict: %w", err)
	}
	var storedVerdict MatrixVerdict
	if err := matrixDecodeStrictJSON(verdictData, &storedVerdict); err != nil {
		return nil, fmt.Errorf("parse verdict: %w", err)
	}
	result.StoredVerdict = &storedVerdict

	// Load cleanup evidence
	cleanupData, err := os.ReadFile(filepath.Join(matrixDir, "matrix-cleanup.json"))
	if err != nil {
		return nil, fmt.Errorf("read cleanup: %w", err)
	}
	var cleanup MatrixCleanupEvidence
	if err := matrixDecodeStrictJSON(cleanupData, &cleanup); err != nil {
		return nil, fmt.Errorf("parse cleanup: %w", err)
	}
	result.Cleanup = &cleanup

	// 4. Authoritative verification of every child bundle using shared helper
	// P0-8 FIX: Use VerifyDeclaredChildRuns for single authority
	verifyFn := deps.VerifyChildRun
	if verifyFn == nil {
		verifyFn = verifyChildRunBundle
	}

	verifiedRuns, err := VerifyDeclaredChildRuns(matrixDir, &manifest, &cleanup, verifyFn)
	if err != nil {
		return nil, fmt.Errorf("child bundle verification: %w", err)
	}

	result.VerifiedRuns = verifiedRuns
	result.AllChildrenVerified = true // VerifyDeclaredChildRuns ensures all pass

	// 5. Validate cleanup evidence against manifest and verified runs
	if err := validateCompleteCleanupEvidence(&manifest, verifiedRuns, &cleanup); err != nil {
		return nil, fmt.Errorf("cleanup validation: %w", err)
	}
	result.CleanupValid = true

	// 6. Reconstruct verdict through ReconstructMatrixVerdict
	reconstructed, err := ReconstructMatrixVerdict(&manifest, verifiedRuns, &cleanup)
	if err != nil {
		return nil, fmt.Errorf("reconstruct verdict: %w", err)
	}
	result.ReconstructedVerdict = reconstructed

	// P0-5 FIX: Complete terminal enforcement inside VerifyMatrixBundle
	// 7. Compare stored and reconstructed verdicts - reject any difference
	diffs := CompareVerdicts(result.StoredVerdict, result.ReconstructedVerdict)
	if len(diffs) > 0 {
		return nil, fmt.Errorf("stored verdict does not match reconstruction:\n%s", FormatVerdictDiffs(diffs))
	}

	// 8. Fail-closed: reject if not all children verified
	if !result.AllChildrenVerified {
		return nil, errors.New("not all child bundles verified")
	}

	// 9. Fail-closed: reject if cleanup invalid
	if !result.CleanupValid {
		return nil, errors.New("cleanup evidence invalid")
	}

	// 10. Fail-closed: reject if reconstructed verdict is invalid
	if !result.ReconstructedVerdict.MatrixValid {
		return nil, errors.New("reconstructed matrix verdict is invalid")
	}

	// P0-8 FIX: ChildVerified is now set during ReconstructScenarioResults.
	// NO post-comparison mutation - the reconstructed verdict is immutable after comparison.

	return result, nil
}

// validateMatrixManifestContract validates the matrix manifest meets its contract.
func validateMatrixManifestContract(manifest *MatrixManifest) error {
	// Validate schema version
	schemaValid := false
	for _, v := range SupportedMatrixSchemaVersions {
		if manifest.SchemaVersion == v {
			schemaValid = true
			break
		}
	}
	if !schemaValid {
		return fmt.Errorf("unsupported matrix schema version: %s", manifest.SchemaVersion)
	}

	// Validate run count
	if len(manifest.Runs) != 3 {
		return fmt.Errorf("expected 3 runs, got %d", len(manifest.Runs))
	}

	// Validate run order
	expectedOrder := []string{"canary-growing", "canary-bounded", "canary-descriptor"}
	for i, decl := range manifest.Runs {
		if decl.Scenario != expectedOrder[i] {
			return fmt.Errorf("run[%d] has wrong scenario %q, expected %q", i, decl.Scenario, expectedOrder[i])
		}
		if decl.Index != i+1 {
			return fmt.Errorf("run[%d] has wrong index %d, expected %d", i, decl.Index, i+1)
		}
	}

	return nil
}

// validateRunCleanupRecord validates a single cleanup record matches the verified run.
func validateRunCleanupRecord(rec RunCleanupRecord, run *VerifiedRun) bool {
	// Check geometry
	if rec.RunID != run.DeclaredRunID {
		return false
	}
	if rec.Scenario != run.DeclaredScenario {
		return false
	}

	// Check container identity binding
	if rec.Container.ID != run.ContainerID {
		return false
	}

	// Check process identity binding
	if rec.Process.PID != run.SubjectPID {
		return false
	}
	if rec.Process.StartTime != run.SubjectStartTime {
		return false
	}

	// Check network identity binding if present
	if run.NetworkID != "" && rec.Network.ID != run.NetworkID {
		return false
	}

	return true
}

// =============================================================================
// SHARED CHILD VERIFICATION HELPER
// =============================================================================

// VerifyDeclaredChildRuns authoritatively verifies all child bundles.
// This is the single authority for child verification used by both:
// - The matrix producer (before writing matrix-verdict.json)
// - The matrix verifier (VerifyMatrixBundle)
//
// Returns verified runs with ChildVerified set from authoritative verification.
// P0-8 FIX: ChildVerified must come from successful verification, not assertion.
//
// FAIL-CLOSED INPUT VALIDATION:
// - Rejects nil manifest, cleanup, or verifyChild
// - Rejects nil child bundles or bundles with ChecksVerified=false
// - Validates declaration-to-child binding (RunID and scenario)
// - Validates matrix geometry (run count, order, indices, paths)
//
// This ensures the helper cannot be called with invalid state or produce
// partially-verified results that later fail at matrix level.
func VerifyDeclaredChildRuns(
	matrixDir string,
	manifest *MatrixManifest,
	cleanup *MatrixCleanupEvidence,
	verifyChild func(runDir string) (*VerifiedChildBundle, error),
) ([]*VerifiedRun, error) {
	// FAIL-CLOSED: Input validation
	if manifest == nil {
		return nil, errors.New("matrix manifest is nil")
	}
	if cleanup == nil {
		return nil, errors.New("cleanup evidence is nil")
	}
	if verifyChild == nil {
		return nil, errors.New("child verifier is nil")
	}

	// FAIL-CLOSED: Matrix geometry validation
	// Validate run count
	if len(manifest.Runs) == 0 {
		return nil, errors.New("manifest has zero declared runs")
	}
	if len(manifest.Runs) != len(cleanup.Runs) {
		return nil, fmt.Errorf(
			"cleanup run count %d != manifest run count %d",
			len(cleanup.Runs),
			len(manifest.Runs),
		)
	}

	// Validate run order, indices, and paths
	expectedOrder := []string{"canary-growing", "canary-bounded", "canary-descriptor"}
	seenRunIDs := make(map[string]bool)
	for i, decl := range manifest.Runs {
		if decl.Scenario != expectedOrder[i] {
			return nil, fmt.Errorf(
				"run[%d] has wrong scenario %q, expected %q",
				i, decl.Scenario, expectedOrder[i],
			)
		}
		if decl.Index != i+1 {
			return nil, fmt.Errorf(
				"run[%d] has wrong index %d, expected %d",
				i, decl.Index, i+1,
			)
		}
		if decl.RunID == "" {
			return nil, fmt.Errorf("run[%d] has empty run_id", i)
		}
		if seenRunIDs[decl.RunID] {
			return nil, fmt.Errorf("duplicate run_id %q in declaration", decl.RunID)
		}
		seenRunIDs[decl.RunID] = true
		// Validate declaration path follows canonical pattern
		expectedPath := filepath.Join("runs", decl.RunID)
		if decl.Path != expectedPath {
			return nil, fmt.Errorf(
				"run[%d] has wrong path %q, expected %q",
				i, decl.Path, expectedPath,
			)
		}
	}

	runsDir := filepath.Join(matrixDir, "runs")
	verifiedRuns := make([]*VerifiedRun, len(manifest.Runs))

	for i, decl := range manifest.Runs {
		runPath := filepath.Join(runsDir, decl.RunID)

		// Authoritative child verification
		child, err := verifyChild(runPath)
		if err != nil {
			return nil, fmt.Errorf("child verification for %s: %w", decl.Scenario, err)
		}

		// FAIL-CLOSED: Reject nil bundle
		if child == nil {
			return nil, fmt.Errorf("child verifier returned nil bundle for %s", decl.Scenario)
		}

		// FAIL-CLOSED: Reject unverified bundle
		// A successful return means ALL children verified
		if !child.ChecksVerified {
			return nil, fmt.Errorf("child bundle %s was not verified (ChecksVerified=false)", decl.Scenario)
		}

		// FAIL-CLOSED: Reject nil manifest or verdict in bundle
		if child.Manifest == nil {
			return nil, fmt.Errorf("child bundle %s has nil manifest", decl.Scenario)
		}
		if child.Verdict == nil {
			return nil, fmt.Errorf("child bundle %s has nil verdict", decl.Scenario)
		}

		// DECLARATION-TO-CHILD BINDING: Verify RunID and scenario match
		if child.Manifest.RunID != decl.RunID {
			return nil, fmt.Errorf(
				"child bundle RunID mismatch: manifest has %q, declaration expects %q",
				child.Manifest.RunID, decl.RunID,
			)
		}
		if child.Manifest.Scenario != decl.Scenario {
			return nil, fmt.Errorf(
				"child bundle scenario mismatch: manifest has %q, declaration expects %q",
				child.Manifest.Scenario, decl.Scenario,
			)
		}
		if child.Verdict.Scenario != decl.Scenario {
			return nil, fmt.Errorf(
				"child verdict scenario mismatch: verdict has %q, declaration expects %q",
				child.Verdict.Scenario, decl.Scenario,
			)
		}

		// Build VerifiedRun from VerifiedChildBundle
		vr := &VerifiedRun{
			DeclaredRunID:     decl.RunID,
			DeclaredScenario:  decl.Scenario,
			RunIndex:          i,
			ActualManifest:    child.Manifest,
			ActualVerdict:    child.Verdict,
			ContainerID:      child.ContainerID,
			ContainerName:    child.ContainerName,
			NetworkID:        child.NetworkID,
			NetworkName:      child.NetworkName,
			SubjectPID:       child.SubjectPID,
			SubjectStartTime: child.SubjectStart,
			ChildVerified:    child.ChecksVerified, // P0-8 FIX: From authoritative verification
		}

		// Hydrate cleanup status from cleanup evidence
		if i < len(cleanup.Runs) {
			rec := cleanup.Runs[i]
			vr.CleanupEvidenceLoaded = true
			vr.CleanupEvidenceValid = validateRunCleanupRecord(rec, vr)

			switch rec.Process.Status {
			case "gone":
				vr.ProcessCleanupStatus = ProcessGone
			case "pid_reused":
				vr.ProcessCleanupStatus = ProcessPIDReused
			case "still_alive":
				vr.ProcessCleanupStatus = ProcessStillAlive
			default:
				vr.ProcessCleanupStatus = ProcessUnavailable
			}
		}

		verifiedRuns[i] = vr
	}

	return verifiedRuns, nil
}

// validateCompleteCleanupEvidence performs full cleanup validation.
func validateCompleteCleanupEvidence(manifest *MatrixManifest, runs []*VerifiedRun, cleanup *MatrixCleanupEvidence) error {
	// 1. Schema version
	if cleanup.SchemaVersion == "" {
		return errors.New("cleanup has empty schema_version")
	}
	schemaValid := false
	for _, v := range SupportedCleanupSchemaVersions {
		if cleanup.SchemaVersion == v {
			schemaValid = true
			break
		}
	}
	if !schemaValid {
		return fmt.Errorf("unsupported cleanup schema: %s", cleanup.SchemaVersion)
	}

	// 2. Matrix ID binding
	if cleanup.MatrixID != manifest.MatrixID {
		return fmt.Errorf("cleanup matrix_id %q != manifest matrix_id %q", cleanup.MatrixID, manifest.MatrixID)
	}

	// 3. Observation timestamp
	if cleanup.ObservedAt.IsZero() {
		return errors.New("cleanup has zero observed_at timestamp")
	}

	// 4. Network ownership mode
	if cleanup.NetworkOwnership != "per_run" && cleanup.NetworkOwnership != "matrix_shared" {
		return fmt.Errorf("invalid network_ownership: %s", cleanup.NetworkOwnership)
	}

	// 5. Run count
	if len(cleanup.Runs) != len(manifest.Runs) {
		return fmt.Errorf("cleanup has %d runs, manifest has %d", len(cleanup.Runs), len(manifest.Runs))
	}

	// Track network IDs for per_run mode
	seenNetworkIDs := make(map[string]bool)
	var sharedNetworkID string

	// 6. Per-record validation
	for i, rec := range cleanup.Runs {
		// Record geometry
		if rec.Index != i {
			return fmt.Errorf("cleanup run[%d] has index %d", i, rec.Index)
		}
		if rec.RunID != manifest.Runs[i].RunID {
			return fmt.Errorf("cleanup run[%d] run_id mismatch", i)
		}
		if rec.Scenario != manifest.Runs[i].Scenario {
			return fmt.Errorf("cleanup run[%d] scenario mismatch", i)
		}

		// Required identities
		if rec.Container.ID == "" {
			return fmt.Errorf("cleanup run[%d] has empty container.id", i)
		}
		if rec.Process.PID == 0 {
			return fmt.Errorf("cleanup run[%d] has zero process.pid", i)
		}
		if rec.Process.StartTime == 0 {
			return fmt.Errorf("cleanup run[%d] has zero process.start_time", i)
		}

		// Independent identity binding - verify cleanup matches verified run
		run := runs[i]
		if rec.Container.ID != run.ContainerID {
			return fmt.Errorf("cleanup run[%d] container.id %q != verified %q", i, rec.Container.ID, run.ContainerID)
		}
		if rec.Process.PID != run.SubjectPID {
			return fmt.Errorf("cleanup run[%d] process.pid %d != verified %d", i, rec.Process.PID, run.SubjectPID)
		}
		if rec.Process.StartTime != run.SubjectStartTime {
			return fmt.Errorf("cleanup run[%d] process.start_time %d != verified %d", i, rec.Process.StartTime, run.SubjectStartTime)
		}

		// Status requirements
		if rec.Container.Status != "gone" {
			return fmt.Errorf("cleanup run[%d] container.status is %q, expected gone", i, rec.Container.Status)
		}
		if rec.Process.Status != "gone" && rec.Process.Status != "pid_reused" {
			return fmt.Errorf("cleanup run[%d] process.status is %q, expected gone or pid_reused", i, rec.Process.Status)
		}

		// Network ownership mode validation
		switch cleanup.NetworkOwnership {
		case "per_run":
			if rec.Network.ID == "" {
				return fmt.Errorf("cleanup run[%d] has empty network.id in per_run mode", i)
			}
			if seenNetworkIDs[rec.Network.ID] {
				return fmt.Errorf("cleanup has duplicate network.id %q in per_run mode", rec.Network.ID)
			}
			seenNetworkIDs[rec.Network.ID] = true
			if rec.Network.Status != "gone" {
				return fmt.Errorf("cleanup run[%d] network.status is %q, expected gone", i, rec.Network.Status)
			}
		case "matrix_shared":
			if rec.Network.ID == "" {
				return fmt.Errorf("cleanup run[%d] has empty network.id in matrix_shared mode", i)
			}
			if sharedNetworkID == "" {
				sharedNetworkID = rec.Network.ID
			} else if rec.Network.ID != sharedNetworkID {
				return fmt.Errorf("cleanup run[%d] network.id %q != shared %q", i, rec.Network.ID, sharedNetworkID)
			}
		}
	}

	return nil
}

// =============================================================================
// COMPLETE VERDICT COMPARISON
// =============================================================================

// CompareVerdictsComplete performs complete field-by-field comparison.
// P0-8 FIX: Returns diffs AND detects equal-invalid terminal case.
func CompareVerdictsComplete(stored, reconstructed *MatrixVerdict) ([]VerdictDiff, bool) {
	var diffs []VerdictDiff

	// Top-level fields
	if stored.MatrixID != reconstructed.MatrixID {
		diffs = append(diffs, VerdictDiff{
			Path:           "matrix_id",
			Stored:         stored.MatrixID,
			Reconstructed:  reconstructed.MatrixID,
		})
	}
	if stored.ChecksTotal != reconstructed.ChecksTotal {
		diffs = append(diffs, VerdictDiff{
			Path:           "checks_total",
			Stored:         fmt.Sprintf("%d", stored.ChecksTotal),
			Reconstructed:  fmt.Sprintf("%d", reconstructed.ChecksTotal),
		})
	}
	if stored.ChecksPassed != reconstructed.ChecksPassed {
		diffs = append(diffs, VerdictDiff{
			Path:           "checks_passed",
			Stored:         fmt.Sprintf("%d", stored.ChecksPassed),
			Reconstructed:  fmt.Sprintf("%d", reconstructed.ChecksPassed),
		})
	}
	if stored.ChecksFailed != reconstructed.ChecksFailed {
		diffs = append(diffs, VerdictDiff{
			Path:           "checks_failed",
			Stored:         fmt.Sprintf("%d", stored.ChecksFailed),
			Reconstructed:  fmt.Sprintf("%d", reconstructed.ChecksFailed),
		})
	}

	// Scenario results - verify exact keys match
	storedScenarios := make(map[string]bool)
	for k := range stored.ScenarioResults {
		storedScenarios[k] = true
	}
	reconScenarios := make(map[string]bool)
	for k := range reconstructed.ScenarioResults {
		reconScenarios[k] = true
	}

	for _, s := range CanonicalScenarioOrder {
		storedPresent := storedScenarios[s]
		reconPresent := reconScenarios[s]

		if storedPresent != reconPresent {
			diffs = append(diffs, VerdictDiff{
				Path:          fmt.Sprintf("scenario_results.%s", s),
				Stored:        fmt.Sprintf("present=%v", storedPresent),
				Reconstructed: fmt.Sprintf("present=%v", reconPresent),
			})
			continue
		}
		if !storedPresent {
			continue
		}

		sr := stored.ScenarioResults[s]
		rr := reconstructed.ScenarioResults[s]

		if sr.RunID != rr.RunID {
			diffs = append(diffs, VerdictDiff{
				Path:          fmt.Sprintf("scenario_results.%s.run_id", s),
				Stored:        sr.RunID,
				Reconstructed: rr.RunID,
			})
		}
		if sr.Verified != rr.Verified {
			diffs = append(diffs, VerdictDiff{
				Path:          fmt.Sprintf("scenario_results.%s.verified", s),
				Stored:        fmt.Sprintf("%v", sr.Verified),
				Reconstructed: fmt.Sprintf("%v", rr.Verified),
			})
		}
		if sr.Overall != rr.Overall {
			diffs = append(diffs, VerdictDiff{
				Path:          fmt.Sprintf("scenario_results.%s.overall", s),
				Stored:        sr.Overall,
				Reconstructed: rr.Overall,
			})
		}
		if sr.Memory != rr.Memory {
			diffs = append(diffs, VerdictDiff{
				Path:          fmt.Sprintf("scenario_results.%s.memory", s),
				Stored:        sr.Memory,
				Reconstructed: rr.Memory,
			})
		}
		if sr.Resource != rr.Resource {
			diffs = append(diffs, VerdictDiff{
				Path:          fmt.Sprintf("scenario_results.%s.resource", s),
				Stored:        sr.Resource,
				Reconstructed: rr.Resource,
			})
		}
		if sr.Semantic != rr.Semantic {
			diffs = append(diffs, VerdictDiff{
				Path:          fmt.Sprintf("scenario_results.%s.semantic", s),
				Stored:        sr.Semantic,
				Reconstructed: rr.Semantic,
			})
		}
	}

	// Cross-run checks - compare every canonical boolean
	if stored.CrossRunChecks != nil && reconstructed.CrossRunChecks != nil {
		namedStored := CanonicalMatrixChecks(stored.CrossRunChecks)
		namedRecon := CanonicalMatrixChecks(reconstructed.CrossRunChecks)

		for i := range namedStored {
			if namedStored[i].Passed != namedRecon[i].Passed {
				diffs = append(diffs, VerdictDiff{
					Path:          fmt.Sprintf("cross_run_checks.%s", namedStored[i].Name),
					Stored:        fmt.Sprintf("%v", namedStored[i].Passed),
					Reconstructed: fmt.Sprintf("%v", namedRecon[i].Passed),
				})
			}
		}
	}

	// P0-8 FIX: Check for equal but invalid terminal case
	equalInvalid := len(diffs) == 0 && stored.MatrixValid != reconstructed.MatrixValid

	return diffs, equalInvalid
}
