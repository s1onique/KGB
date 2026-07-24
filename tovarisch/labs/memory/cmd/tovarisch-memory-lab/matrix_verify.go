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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
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

	// P0-9 FIX: Use ReadNetworkIdentity for authoritative network identity extraction.
	// This replaces the anonymous decoder with a validated canonical reader.
	networkID, networkName, err := ReadNetworkIdentity(runDir)
	if err != nil {
		return nil, fmt.Errorf("verify network identity: %w", err)
	}
	result.NetworkID = networkID
	result.NetworkName = networkName

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
		"network-identity.json",
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
	// P0-8 FIX: Use CompareVerdictsComplete which detects equal-invalid terminal case
	diffs, equalInvalid := CompareVerdictsComplete(result.StoredVerdict, result.ReconstructedVerdict)
	if len(diffs) > 0 || equalInvalid {
		if equalInvalid {
			// P0-8 FIX: Return result with error for equal-invalid case (both invalid with zero diffs)
			return result, errors.New("equal-invalid verdict: both stored and reconstructed are invalid (forbidden by policy)")
		}
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

// =============================================================================
// TWO-PHASE AUTHORITY: CHILD VERIFICATION AND CLEANUP BINDING
// =============================================================================

// VerifyDeclaredChildBundles authoritatively verifies all child bundles WITHOUT
// requiring cleanup evidence. This enables the correct producer authority order:
//
//   1. verify child artifacts (checksums, inventory, manifests, identities)
//   2. observe runtime cleanup state
//   3. write cleanup evidence
//   4. bind cleanup to verified runs
//   5. reconstruct and verify matrix verdict
//
// Returns verified runs with runtime identities populated from child artifacts.
// Each VerifiedRun has ChildVerified=true on success.
//
// P0 FIX: Splits child verification from cleanup binding to enable authority order.
func VerifyDeclaredChildBundles(
	matrixDir string,
	manifest *MatrixManifest,
	verifyChild func(runDir string) (*VerifiedChildBundle, error),
) ([]*VerifiedRun, error) {
	// FAIL-CLOSED: Input validation
	if manifest == nil {
		return nil, errors.New("matrix manifest is nil")
	}
	if verifyChild == nil {
		return nil, errors.New("child verifier is nil")
	}

	// FAIL-CLOSED: Matrix geometry validation
	expectedRunCount := len(CanonicalScenarioOrder)
	if len(manifest.Runs) != expectedRunCount {
		return nil, fmt.Errorf(
			"expected exactly %d declared runs, got %d",
			expectedRunCount,
			len(manifest.Runs),
		)
	}

	// Validate run order, indices, and paths
	seenRunIDs := make(map[string]bool)
	for i, decl := range manifest.Runs {
		expectedScenario := CanonicalScenarioOrder[i]
		if decl.Scenario != expectedScenario {
			return nil, fmt.Errorf(
				"run[%d] has wrong scenario %q, expected %q",
				i, decl.Scenario, expectedScenario,
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
		expectedPath := path.Join("runs", decl.RunID)
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

		// P0-1 FIX: Reject independently verified identities that are empty.
		// An empty container ID means no verification occurred - the bundle is incomplete.
		if child.ContainerID == "" {
			return nil, fmt.Errorf(
				"child bundle %s has empty container identity",
				decl.RunID,
			)
		}
		// P0-1 FIX: Network identity is required for all child bundles.
		// Both per_run and matrix_shared modes require a non-empty network ID.
		if child.NetworkID == "" {
			return nil, fmt.Errorf(
				"child bundle %s has empty network identity",
				decl.RunID,
			)
		}
		// P0-1 FIX: Reject invalid subject PID - zero or negative is invalid.
		if child.SubjectPID <= 0 {
			return nil, fmt.Errorf(
				"child bundle %s has invalid subject PID %d",
				decl.RunID,
				child.SubjectPID,
			)
		}
		// P0-1 FIX: Reject zero start time - process identity requires valid start time.
		if child.SubjectStart == 0 {
			return nil, fmt.Errorf(
				"child bundle %s has zero subject start time",
				decl.RunID,
			)
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
			RunIndex:         i,
			ActualManifest:    child.Manifest,
			ActualVerdict:    child.Verdict,
			ContainerID:      child.ContainerID,
			ContainerName:    child.ContainerName,
			NetworkID:        child.NetworkID,
			NetworkName:      child.NetworkName,
			SubjectPID:       child.SubjectPID,
			SubjectStartTime: child.SubjectStart,
			ChildVerified:    child.ChecksVerified,
		}

		verifiedRuns[i] = vr
	}

	return verifiedRuns, nil
}

// ValidateCleanupBinding performs complete cleanup binding validation.
// This is the canonical validator used by both BindVerifiedRunsToCleanup and final verification.
// P0 FIX: Single authority for cleanup binding validation.
func ValidateCleanupBinding(
	manifest *MatrixManifest,
	runs []*VerifiedRun,
	cleanup *MatrixCleanupEvidence,
) error {
	// FAIL-CLOSED: Input validation
	if manifest == nil {
		return errors.New("cleanup manifest is nil")
	}
	if runs == nil {
		return errors.New("verified runs is nil")
	}
	if cleanup == nil {
		return errors.New("cleanup evidence is nil")
	}

	// FAIL-CLOSED: Count validation
	expectedRunCount := len(CanonicalScenarioOrder)
	if len(runs) != expectedRunCount {
		return fmt.Errorf(
			"expected exactly %d verified runs, got %d",
			expectedRunCount,
			len(runs),
		)
	}
	if len(cleanup.Runs) != expectedRunCount {
		return fmt.Errorf(
			"expected exactly %d cleanup records, got %d",
			expectedRunCount,
			len(cleanup.Runs),
		)
	}

	// FAIL-CLOSED: Top-level identity validation
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
		return fmt.Errorf("unsupported cleanup schema version: %s", cleanup.SchemaVersion)
	}
	if cleanup.MatrixID != manifest.MatrixID {
		return fmt.Errorf(
			"cleanup matrix_id %q != manifest matrix_id %q",
			cleanup.MatrixID,
			manifest.MatrixID,
		)
	}
	if cleanup.ObservedAt.IsZero() {
		return errors.New("cleanup has zero observed_at timestamp")
	}

	// P0-4 FIX: Require cleanup observation occurs at or after matrix completion.
	// This check must be in the canonical binding validator so direct calls
	// to BindVerifiedRunsToCleanup are also protected.
	if manifest.FinishedAt.After(cleanup.ObservedAt) {
		return fmt.Errorf(
			"cleanup observed_at %s precedes matrix finished_at %s",
			cleanup.ObservedAt,
			manifest.FinishedAt,
		)
	}

	if cleanup.NetworkOwnership != "per_run" && cleanup.NetworkOwnership != "matrix_shared" {
		return fmt.Errorf("invalid network_ownership: %s", cleanup.NetworkOwnership)
	}

	// FAIL-CLOSED: Manifest geometry validation
	if len(manifest.Runs) != expectedRunCount {
		return fmt.Errorf(
			"manifest has %d runs, expected %d",
			len(manifest.Runs),
			expectedRunCount,
		)
	}

	// Track network IDs for per_run mode
	seenNetworkIDs := make(map[string]bool)
	var sharedNetworkID string

	// P0-2 FIX: Reject nil verified-run entries that could cause panic.
	// A malformed slice containing a nil entry must produce a typed error, not panic.
	for i := 0; i < expectedRunCount; i++ {
		if runs[i] == nil {
			return fmt.Errorf(
				"verified run[%d] is nil",
				i,
			)
		}
	}

	// P0-3 FIX: Bind verified runs back to manifest declarations.
	// The exported binding authority must enforce its own contract.
	for i := 0; i < expectedRunCount; i++ {
		decl := manifest.Runs[i]
		vr := runs[i]

		// Verify run-to-manifest binding by index
		if vr.DeclaredRunID != decl.RunID {
			return fmt.Errorf(
				"verified run[%d] DeclaredRunID=%q != manifest.RunID=%q",
				i, vr.DeclaredRunID, decl.RunID,
			)
		}
		if vr.DeclaredScenario != decl.Scenario {
			return fmt.Errorf(
				"verified run[%d] DeclaredScenario=%q != manifest.Scenario=%q",
				i, vr.DeclaredScenario, decl.Scenario,
			)
		}
		if vr.RunIndex != i {
			return fmt.Errorf(
				"verified run[%d] RunIndex=%d != index=%d",
				i, vr.RunIndex, i,
			)
		}
		if decl.Index != i+1 {
			return fmt.Errorf(
				"manifest.Runs[%d].Index=%d != expected %d",
				i, decl.Index, i+1,
			)
		}
		expectedPath := path.Join("runs", decl.RunID)
		if decl.Path != expectedPath {
			return fmt.Errorf(
				"manifest.Runs[%d].Path=%q != expected %q",
				i, decl.Path, expectedPath,
			)
		}
	}

	// FAIL-CLOSED: Per-record binding validation with field-specific diagnostics
	for i := 0; i < expectedRunCount; i++ {
		rec := cleanup.Runs[i]
		vr := runs[i]
		if rec.Index != i {
			return fmt.Errorf(
				"cleanup record[%d] has wrong index %d",
				i, rec.Index,
			)
		}

		// RunID binding validation
		if rec.RunID != vr.DeclaredRunID {
			return fmt.Errorf(
				"cleanup record[%d] RunID mismatch: cleanup=%q, verified=%q",
				i, rec.RunID, vr.DeclaredRunID,
			)
		}

		// Scenario binding validation
		if rec.Scenario != vr.DeclaredScenario {
			return fmt.Errorf(
				"cleanup record[%d] scenario mismatch: cleanup=%q, verified=%q",
				i, rec.Scenario, vr.DeclaredScenario,
			)
		}

		// Container ID binding validation
		if rec.Container.ID != vr.ContainerID {
			return fmt.Errorf(
				"cleanup record[%d] container.id mismatch: cleanup=%q, verified=%q",
				i, rec.Container.ID, vr.ContainerID,
			)
		}

		// PID binding validation
		if rec.Process.PID != vr.SubjectPID {
			return fmt.Errorf(
				"cleanup record[%d] process.pid mismatch: cleanup=%d, verified=%d",
				i, rec.Process.PID, vr.SubjectPID,
			)
		}

		// Process start time binding validation
		if rec.Process.StartTime != vr.SubjectStartTime {
			return fmt.Errorf(
				"cleanup record[%d] process.start_time mismatch: cleanup=%d, verified=%d",
				i, rec.Process.StartTime, vr.SubjectStartTime,
			)
		}

		// Network ID binding validation (if present in verified run)
		if vr.NetworkID != "" && rec.Network.ID != vr.NetworkID {
			return fmt.Errorf(
				"cleanup record[%d] network.id mismatch: cleanup=%q, verified=%q",
				i, rec.Network.ID, vr.NetworkID,
			)
		}

		// Cleanup status validation
		validStatuses := map[string]bool{
			"gone": true, "pid_reused": true, "still_alive": true,
		}
		if !validStatuses[rec.Process.Status] {
			return fmt.Errorf(
				"cleanup record[%d] has invalid process.status: %q",
				i, rec.Process.Status,
			)
		}
		if rec.Container.Status != "gone" {
			return fmt.Errorf(
				"cleanup record[%d] container.status is %q, expected gone",
				i, rec.Container.Status,
			)
		}

		// Network ownership mode validation
		switch cleanup.NetworkOwnership {
		case "per_run":
			if rec.Network.ID == "" {
				return fmt.Errorf("cleanup record[%d] has empty network.id in per_run mode", i)
			}
			if seenNetworkIDs[rec.Network.ID] {
				return fmt.Errorf(
					"cleanup has duplicate network.id %q in per_run mode",
					rec.Network.ID,
				)
			}
			seenNetworkIDs[rec.Network.ID] = true
			if rec.Network.Status != "gone" {
				return fmt.Errorf(
					"cleanup record[%d] network.status is %q, expected gone",
					i, rec.Network.Status,
				)
			}
		case "matrix_shared":
			if rec.Network.ID == "" {
				return fmt.Errorf("cleanup record[%d] has empty network.id in matrix_shared mode", i)
			}
			if sharedNetworkID == "" {
				sharedNetworkID = rec.Network.ID
			} else if rec.Network.ID != sharedNetworkID {
				return fmt.Errorf(
					"cleanup record[%d] network.id %q != shared %q",
					i, rec.Network.ID, sharedNetworkID,
				)
			}
			// P1 FIX: Shared network status must also be "gone"
			if rec.Network.Status != "gone" {
				return fmt.Errorf(
					"cleanup record[%d] shared network.status is %q, expected gone",
					i, rec.Network.Status,
				)
			}
		}
	}

	return nil
}

// BindVerifiedRunsToCleanup binds cleanup evidence to already-verified runs.
// This is the second phase of the two-phase authority pattern.
//
// P0 FIX: Enables cleanup binding after child verification but before verdict reconstruction.
// P0-2 FIX: Returns error on binding failure with field-specific diagnostics.
// P0-4 FIX: Uses two-pass approach - validates ALL bindings before mutating any run.
func BindVerifiedRunsToCleanup(
	manifest *MatrixManifest,
	runs []*VerifiedRun,
	cleanup *MatrixCleanupEvidence,
) ([]*VerifiedRun, error) {
	// FAIL-CLOSED: Validate all bindings BEFORE mutating any run
	if err := ValidateCleanupBinding(manifest, runs, cleanup); err != nil {
		return nil, fmt.Errorf("cleanup binding validation failed: %w", err)
	}

	// PASS: All bindings valid - safe to hydrate
	// Two-pass: validate first, then hydrate (no partial mutation)
	expectedRunCount := len(CanonicalScenarioOrder)
	hydrated := make([]*VerifiedRun, expectedRunCount)

	for i := 0; i < expectedRunCount; i++ {
		vr := runs[i]
		rec := cleanup.Runs[i]

		// Clone the VerifiedRun to avoid mutation
		hydratedRun := &VerifiedRun{
			DeclaredRunID:          vr.DeclaredRunID,
			DeclaredScenario:       vr.DeclaredScenario,
			RunIndex:              vr.RunIndex,
			ActualManifest:        vr.ActualManifest,
			ActualVerdict:         vr.ActualVerdict,
			ContainerID:           vr.ContainerID,
			ContainerName:         vr.ContainerName,
			NetworkID:             vr.NetworkID,
			NetworkName:           vr.NetworkName,
			SubjectPID:            vr.SubjectPID,
			SubjectStartTime:      vr.SubjectStartTime,
			ChildVerified:         vr.ChildVerified,
			CleanupEvidenceLoaded: true,
			CleanupEvidenceValid:  true, // Validated above
		}

		switch rec.Process.Status {
		case "gone":
			hydratedRun.ProcessCleanupStatus = ProcessGone
		case "pid_reused":
			hydratedRun.ProcessCleanupStatus = ProcessPIDReused
		case "still_alive":
			hydratedRun.ProcessCleanupStatus = ProcessStillAlive
		default:
			hydratedRun.ProcessCleanupStatus = ProcessUnavailable
		}

		hydrated[i] = hydratedRun
	}

	return hydrated, nil
}

// VerifyDeclaredChildRuns is the single authoritative function for child verification.
// It is a COMPOSITE wrapper that calls exactly two phases:
//   1. VerifyDeclaredChildBundles - verifies child artifacts (no cleanup required)
//   2. BindVerifiedRunsToCleanup  - binds cleanup evidence to verified runs
//
// Both producer and verifier MUST use this function for child verification.
// No independent verification loop may remain inside this function.
//
// P0 FIX: This function is a literal composition, not an independent implementation.
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

	// Phase 1: Verify child bundles (no cleanup required)
	runs, err := VerifyDeclaredChildBundles(matrixDir, manifest, verifyChild)
	if err != nil {
		return nil, fmt.Errorf("phase 1 (verify children): %w", err)
	}

	// Phase 2: Bind cleanup evidence to verified runs
	runs, err = BindVerifiedRunsToCleanup(manifest, runs, cleanup)
	if err != nil {
		return nil, fmt.Errorf("phase 2 (bind cleanup): %w", err)
	}

	return runs, nil
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

	// P0-4 FIX: Require cleanup observation occurs at or after matrix completion.
	// The ACT explicitly requires post-cleanup observation, so temporal order
	// is part of the evidence contract.
	if cleanup.ObservedAt.Before(manifest.FinishedAt) {
		return fmt.Errorf(
			"cleanup observed_at %s precedes matrix finished_at %s",
			cleanup.ObservedAt,
			manifest.FinishedAt,
		)
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
	// P0-8 FIX: Check matrix_valid for normal mismatch (stored != reconstructed).
	// The equal-invalid case (both false) is handled separately below.
	if stored.MatrixValid != reconstructed.MatrixValid {
		diffs = append(diffs, VerdictDiff{
			Path:           "matrix_valid",
			Stored:         fmt.Sprintf("%v", stored.MatrixValid),
			Reconstructed:  fmt.Sprintf("%v", reconstructed.MatrixValid),
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
	// The bug: stored.MatrixValid != reconstructed.MatrixValid is FALSE when both are FALSE!
	// Fixed: Check that BOTH are false (both invalid) when all other fields match.
	equalInvalid := len(diffs) == 0 && stored.MatrixValid == false && reconstructed.MatrixValid == false

	return diffs, equalInvalid
}
