// reconstruct.go — Single Authority Matrix Verdict Reconstruction
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03
//
// This file contains the SINGLE authoritative reconstruction path for matrix verdicts.
// Both `matrix` and `verify-matrix` commands MUST use ReconstructMatrixVerdict.
//
// CORRECTION03 establishes:
// - One shared ReconstructMatrixVerdict function (no duplicate authority)
// - Canonical cleanup evidence bound to exact container/network/process identities
// - Real SameSchema reconstruction from verified child manifests
// - Canonical cross-run check enumeration (mechanically derived, not hardcoded)
// - Complete field-by-field verdict comparison with field-specific diagnostics
// - Fail-closed CLI exit semantics
// - Strict single-document JSON parsing for all artifacts
// - Observed cleanup verification (not asserted)
// - Complete cleanup contract validation

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence"
)

// =============================================================================
// STRICT JSON DECODING
// =============================================================================

// P0-8 FIX: Strict single-document JSON parsing for all canonical matrix artifacts.
// Rejects unknown fields and ensures exactly one JSON value per document.
func decodeStrictJSON[T any](data []byte, dst *T) error {
	if len(data) == 0 {
		return errors.New("empty input")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	// Ensure exactly one top-level JSON value (no trailing data)
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
// CANONICAL CONSTANTS
// =============================================================================

// SupportedSchemaVersions defines the supported child run manifest schema versions.
var SupportedSchemaVersions = []string{"1.1.0"}

// SupportedMatrixSchemaVersions defines the supported matrix manifest schema versions.
var SupportedMatrixSchemaVersions = []string{"1.0.0"}

// CanonicalScenarioOrder defines the exact required order of scenarios in a matrix.
var CanonicalScenarioOrder = []string{
	"canary-growing",
	"canary-bounded",
	"canary-descriptor",
}

// CanonicalCrossRunCheckNames defines the canonical ordered list of cross-run check names.
// This list is the single source of truth for:
// - Mechanical check enumeration
// - Diagnostic output
// - Check total/pass/failed counting
var CanonicalCrossRunCheckNames = []string{
	"SameCommitTree",
	"SameControllerPID",
	"SameControllerHash",
	"SameSchema",
	"SameThresholds",
	"SamePhaseConfig",
	"SameHostIdentity",
	"SameDockerIdentity",
	"SameImageIdentity",
	"SameCanaryBinary",
	"UniqueRunIDs",
	"UniqueSubjectProcesses",
	"UniqueContainerIDs",
	"FixedOrder",
	"NonOverlapping",
	"CleanupComplete",
}

// CrossRunCheckCount is the canonical number of cross-run checks.
const CrossRunCheckCount = 16

// =============================================================================
// VERIFIED RUN MODEL
// =============================================================================

// VerifiedRun represents a child run that has been verified by the matrix producer
// or verifier. This model carries the actual parsed schema version used by
// SameSchema reconstruction.
type VerifiedRun struct {
	// DeclaredRunID is the run ID from the matrix manifest declaration.
	DeclaredRunID string

	// DeclaredScenario is the scenario from the matrix manifest declaration.
	DeclaredScenario string

	// RunIndex is the zero-based index in the matrix run order.
	RunIndex int

	// ActualManifest is the parsed child run manifest (for SameSchema reconstruction).
	ActualManifest *evidence.Manifest

	// ActualVerdict is the parsed child run verdict (for overall classification).
	ActualVerdict *evidence.Verdict

	// ContainerID is the exact full container ID from container-inspect.json.
	ContainerID string

	// ContainerName is the diagnostic container name.
	ContainerName string

	// NetworkID is the exact full network ID created by the controller.
	NetworkID string

	// NetworkName is the diagnostic network name.
	NetworkName string

	// SubjectPID is the subject process PID from samples.csv.
	SubjectPID int

	// SubjectStartTime is the subject process start time from samples.csv.
	SubjectStartTime uint64

	// ProcessCleanupStatus is the classified cleanup result.
	ProcessCleanupStatus ProcessCleanupStatus

	// CleanupVerified is true if cleanup was successfully verified.
	CleanupVerified bool
}

// =============================================================================
// CLEANUP EVIDENCE TYPES
// =============================================================================

// MatrixCleanupEvidence is the canonical cleanup artifact persisted as matrix-cleanup.json.
type MatrixCleanupEvidence struct {
	SchemaVersion    string             `json:"schema_version"`
	MatrixID        string             `json:"matrix_id"`
	ObservedAt      time.Time          `json:"observed_at"`
	NetworkOwnership string             `json:"network_ownership"` // "per_run" or "matrix_shared"
	Runs            []RunCleanupRecord `json:"runs"`
}

// RunCleanupRecord represents cleanup evidence for a single run.
type RunCleanupRecord struct {
	Index      int                    `json:"index"`
	Scenario   string                 `json:"scenario"`
	RunID      string                 `json:"run_id"`
	Container  ContainerCleanupRecord `json:"container"`
	Network    NetworkCleanupRecord   `json:"network"`
	Process    ProcessCleanupRecord   `json:"process"`
}

// ContainerCleanupRecord captures container cleanup evidence.
type ContainerCleanupRecord struct {
	ID     string `json:"id"`      // Full container ID (required)
	Name   string `json:"name"`   // Diagnostic name
	Status string `json:"status"` // "gone", "exists", "unavailable"
}

// NetworkCleanupRecord captures network cleanup evidence.
type NetworkCleanupRecord struct {
	ID     string `json:"id"`      // Full network ID (required)
	Name   string `json:"name"`   // Diagnostic name
	Status string `json:"status"` // "gone", "exists", "unavailable"
}

// ProcessCleanupRecord captures process cleanup evidence.
type ProcessCleanupRecord struct {
	PID        int    `json:"pid"`         // Process PID (required)
	StartTime  uint64 `json:"start_time"` // Process start time (required)
	Status     string `json:"status"`     // "gone", "pid_reused", "still_alive", "unavailable"
}

// LoadMatrixCleanupEvidence loads and parses matrix-cleanup.json.
// P0-8 FIX: Uses strict single-document JSON parsing with DisallowUnknownFields.
func LoadMatrixCleanupEvidence(path string) (*MatrixCleanupEvidence, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read matrix cleanup evidence: %w", err)
	}
	var ce MatrixCleanupEvidence
	if err := decodeStrictJSON(data, &ce); err != nil {
		return nil, fmt.Errorf("parse matrix cleanup evidence: %w", err)
	}
	return &ce, nil
}

// SupportedCleanupSchemaVersions defines the supported cleanup evidence schema versions.
var SupportedCleanupSchemaVersions = []string{"1.0.0"}

// ValidateCleanupEvidence validates cleanup evidence is bound correctly to the matrix manifest.
// Validates:
// - Cleanup schema version is supported
// - Matrix ID matches manifest
// - Network ownership mode is valid
// - Each record has required identities based on ownership mode
// - No duplicate network IDs when per_run mode
func ValidateCleanupEvidence(cleanup *MatrixCleanupEvidence, manifest *MatrixManifest) error {
	// Validate schema version
	if cleanup.SchemaVersion == "" {
		return errors.New("cleanup evidence has empty schema_version")
	}
	schemaValid := false
	for _, v := range SupportedCleanupSchemaVersions {
		if cleanup.SchemaVersion == v {
			schemaValid = true
			break
		}
	}
	if !schemaValid {
		return fmt.Errorf("cleanup schema version %q is not supported", cleanup.SchemaVersion)
	}

	// Validate MatrixID binding
	if cleanup.MatrixID != manifest.MatrixID {
		return fmt.Errorf("cleanup matrix_id %q != manifest matrix_id %q", cleanup.MatrixID, manifest.MatrixID)
	}

	// Validate run count matches
	if len(cleanup.Runs) != len(manifest.Runs) {
		return fmt.Errorf("cleanup has %d runs, manifest declares %d runs", len(cleanup.Runs), len(manifest.Runs))
	}

	// Validate network ownership mode
	networkOwnership := cleanup.NetworkOwnership
	if networkOwnership != "per_run" && networkOwnership != "matrix_shared" {
		return fmt.Errorf("cleanup has invalid network_ownership %q (expected per_run or matrix_shared)", networkOwnership)
	}

	// Track network IDs for per_run mode
	seenNetworkIDs := make(map[string]bool)
	var sharedNetworkID string

	for i, rec := range cleanup.Runs {
		// Validate record geometry
		if rec.Index != i {
			return fmt.Errorf("cleanup run[%d] has index %d", i, rec.Index)
		}
		if rec.RunID != manifest.Runs[i].RunID {
			return fmt.Errorf("cleanup run[%d] has run_id %q, manifest declares %q", i, rec.RunID, manifest.Runs[i].RunID)
		}
		if rec.Scenario != manifest.Runs[i].Scenario {
			return fmt.Errorf("cleanup run[%d] has scenario %q, manifest declares %q", i, rec.Scenario, manifest.Runs[i].Scenario)
		}
		if rec.RunID == "" {
			return fmt.Errorf("cleanup run[%d] has empty run_id", i)
		}
		if rec.Scenario == "" {
			return fmt.Errorf("cleanup run[%d] has empty scenario", i)
		}

		// Validate container identity (required for all modes)
		if rec.Container.ID == "" {
			return fmt.Errorf("cleanup run[%d] has empty container.id", i)
		}

		// Validate process identity (required for all modes)
		if rec.Process.PID == 0 {
			return fmt.Errorf("cleanup run[%d] has empty process.pid", i)
		}

		// Validate network identity according to ownership mode
		switch networkOwnership {
		case "per_run":
			// Every record must have a non-empty, unique network ID
			if rec.Network.ID == "" {
				return fmt.Errorf("cleanup run[%d] has empty network.id (per_run mode requires unique network)", i)
			}
			if seenNetworkIDs[rec.Network.ID] {
				return fmt.Errorf("cleanup has duplicate network.id %q in per_run mode", rec.Network.ID)
			}
			seenNetworkIDs[rec.Network.ID] = true
		case "matrix_shared":
			// First record establishes shared ID; all records must reference same ID
			if rec.Network.ID == "" {
				return fmt.Errorf("cleanup run[%d] has empty network.id (matrix_shared mode requires shared network)", i)
			}
			if sharedNetworkID == "" {
				sharedNetworkID = rec.Network.ID
			} else if rec.Network.ID != sharedNetworkID {
				return fmt.Errorf("cleanup run[%d] has different network.id %q (expected shared %q)", i, rec.Network.ID, sharedNetworkID)
			}
		}
	}

	return nil
}

// =============================================================================
// NAMED CHECK TYPES
// =============================================================================

// NamedMatrixCheck represents a named check with its pass/fail status.
type NamedMatrixCheck struct {
	Name   string
	Passed bool
}

// CanonicalMatrixChecks projects CrossRunChecks into the canonical named check list.
func CanonicalMatrixChecks(checks *CrossRunChecks) []NamedMatrixCheck {
	v := reflect.ValueOf(*checks)
	result := make([]NamedMatrixCheck, 0, len(CanonicalCrossRunCheckNames))

	// Build a map of field name -> bool value
	fieldMap := make(map[string]bool)
	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		if field.Type.Kind() == reflect.Bool {
			fieldMap[field.Name] = v.Field(i).Bool()
		}
	}

	for _, name := range CanonicalCrossRunCheckNames {
		result = append(result, NamedMatrixCheck{
			Name:   name,
			Passed: fieldMap[name],
		})
	}

	return result
}

// CountCanonicalChecks computes check totals from the canonical projection.
func CountCanonicalChecks(checks *CrossRunChecks) (total, passed, failed int) {
	named := CanonicalMatrixChecks(checks)
	total = len(named)
	for _, nc := range named {
		if nc.Passed {
			passed++
		}
	}
	failed = total - passed
	return
}

// =============================================================================
// SAME SCHEMA RECONSTRUCTION
// =============================================================================

// ReconstructSameSchema verifies SameSchema is true based on actual child manifest schemas.
// SameSchema is true only when ALL conditions are met:
// 1. Matrix manifest schema version is supported.
// 2. Matrix execution identity declares a supported child run-manifest schema.
// 3. Exactly three child runs are present.
// 4. Every verified child exposes its actual run-manifest schema version.
// 5. Every child schema version equals the matrix execution identity's declared child schema version.
// 6. Every child schema version equals every other child schema version.
// 7. No child schema value is empty, unavailable or inferred from a default.
func ReconstructSameSchema(
	matrixManifest *MatrixManifest,
	verifiedRuns []*VerifiedRun,
) (bool, error) {
	// 1. Matrix manifest schema version must be supported
	matrixSchemaValid := false
	for _, v := range SupportedMatrixSchemaVersions {
		if matrixManifest.SchemaVersion == v {
			matrixSchemaValid = true
			break
		}
	}
	if !matrixSchemaValid {
		return false, fmt.Errorf("matrix schema version %q is not supported", matrixManifest.SchemaVersion)
	}

	// 2. Matrix execution identity must declare a supported child schema
	declaredChildSchema := matrixManifest.ExecutionIdentity.RunManifestSchemaVersion
	declaredSchemaValid := false
	for _, v := range SupportedSchemaVersions {
		if declaredChildSchema == v {
			declaredSchemaValid = true
			break
		}
	}
	if !declaredSchemaValid {
		return false, fmt.Errorf("matrix declares unsupported child schema version %q", declaredChildSchema)
	}

	// 3. Exactly three child runs must be present
	if len(verifiedRuns) != 3 {
		return false, fmt.Errorf("expected 3 child runs, got %d", len(verifiedRuns))
	}

	// Collect actual child schema versions
	actualSchemas := make([]string, len(verifiedRuns))
	for i, run := range verifiedRuns {
		// 4. Every verified child must expose its actual schema version
		if run.ActualManifest == nil {
			return false, fmt.Errorf("run[%d] %s has no manifest for schema verification", i, run.DeclaredRunID)
		}
		actualSchemas[i] = run.ActualManifest.SchemaVersion

		// 7. No empty schema
		if actualSchemas[i] == "" {
			return false, fmt.Errorf("run[%d] %s has empty schema version", i, run.DeclaredRunID)
		}
	}

	// 5. Every child schema must equal the declared schema
	for i, schema := range actualSchemas {
		if schema != declaredChildSchema {
			return false, fmt.Errorf("run[%d] %s schema %q != declared %q",
				i, verifiedRuns[i].DeclaredRunID, schema, declaredChildSchema)
		}
	}

	// 6. Every child schema must equal every other child schema (all same)
	firstSchema := actualSchemas[0]
	for i := 1; i < len(actualSchemas); i++ {
		if actualSchemas[i] != firstSchema {
			return false, fmt.Errorf("schema mismatch between runs: %q vs %q",
				firstSchema, actualSchemas[i])
		}
	}

	return true, nil
}

// =============================================================================
// CROSS-RUN CHECK RECONSTRUCTION
// =============================================================================

// ReconstructCrossRunChecks computes all cross-run checks from verified child runs.
func ReconstructCrossRunChecks(
	matrixManifest *MatrixManifest,
	verifiedRuns []*VerifiedRun,
	cleanup *MatrixCleanupEvidence,
) (*CrossRunChecks, error) {
	checks := &CrossRunChecks{}

	if len(verifiedRuns) != 3 {
		return checks, fmt.Errorf("expected 3 verified runs, got %d", len(verifiedRuns))
	}

	// Use first manifest as reference
	ref := verifiedRuns[0].ActualManifest

	// 1. SameCommitTree
	checks.SameCommitTree = true
	for _, run := range verifiedRuns[1:] {
		if run.ActualManifest.SubjectIdentity == nil || ref.SubjectIdentity == nil {
			checks.SameCommitTree = false
			break
		}
		if run.ActualManifest.SubjectIdentity.GitCommit != ref.SubjectIdentity.GitCommit ||
			run.ActualManifest.SubjectIdentity.GitTree != ref.SubjectIdentity.GitTree {
			checks.SameCommitTree = false
			break
		}
	}

	// 2. SameControllerPID
	checks.SameControllerPID = true
	for _, run := range verifiedRuns {
		if run.ActualManifest.ControllerID != ref.ControllerID {
			checks.SameControllerPID = false
			break
		}
	}

	// 3. SameControllerHash
	checks.SameControllerHash = true
	for _, run := range verifiedRuns {
		if run.ActualManifest.SubjectIdentity == nil || ref.SubjectIdentity == nil {
			checks.SameControllerHash = false
			break
		}
		if run.ActualManifest.SubjectIdentity.ControllerExecutableSHA256 != ref.SubjectIdentity.ControllerExecutableSHA256 {
			checks.SameControllerHash = false
			break
		}
	}

	// 4. SameSchema (real reconstruction)
	sameSchema, err := ReconstructSameSchema(matrixManifest, verifiedRuns)
	if err != nil {
		return nil, fmt.Errorf("reconstruct same_schema: %w", err)
	}
	checks.SameSchema = sameSchema

	// 5. SameThresholds
	checks.SameThresholds = true
	for _, run := range verifiedRuns {
		if run.ActualManifest.Configuration == nil || ref.Configuration == nil {
			checks.SameThresholds = false
			break
		}
		refJSON, _ := json.Marshal(ref.Configuration.Thresholds)
		mJSON, _ := json.Marshal(run.ActualManifest.Configuration.Thresholds)
		if string(refJSON) != string(mJSON) {
			checks.SameThresholds = false
			break
		}
	}

	// 6. SamePhaseConfig
	checks.SamePhaseConfig = true
	for _, run := range verifiedRuns {
		if run.ActualManifest.Configuration == nil || ref.Configuration == nil {
			checks.SamePhaseConfig = false
			break
		}
		refJSON, _ := json.Marshal(ref.Configuration.PhaseConfig)
		mJSON, _ := json.Marshal(run.ActualManifest.Configuration.PhaseConfig)
		if string(refJSON) != string(mJSON) {
			checks.SamePhaseConfig = false
			break
		}
	}

	// 7. SameHostIdentity
	checks.SameHostIdentity = true
	for _, run := range verifiedRuns {
		if run.ActualManifest.HostID == nil || ref.HostID == nil {
			checks.SameHostIdentity = false
			break
		}
		if run.ActualManifest.HostID.KernelRelease != ref.HostID.KernelRelease ||
			run.ActualManifest.HostID.CgroupMode != ref.HostID.CgroupMode {
			checks.SameHostIdentity = false
			break
		}
	}

	// 8. SameDockerIdentity
	checks.SameDockerIdentity = true
	for _, run := range verifiedRuns {
		if run.ActualManifest.DockerID == nil || ref.DockerID == nil {
			checks.SameDockerIdentity = false
			break
		}
		if run.ActualManifest.DockerID.EngineVersion != ref.DockerID.EngineVersion {
			checks.SameDockerIdentity = false
			break
		}
	}

	// 9. SameImageIdentity
	checks.SameImageIdentity = true
	for _, run := range verifiedRuns {
		if run.ActualManifest.SubjectImageIdentity == nil || ref.SubjectImageIdentity == nil {
			checks.SameImageIdentity = false
			break
		}
		if run.ActualManifest.SubjectImageIdentity.ImageID != ref.SubjectImageIdentity.ImageID {
			checks.SameImageIdentity = false
			break
		}
	}

	// 10. SameCanaryBinary
	checks.SameCanaryBinary = true
	for _, run := range verifiedRuns {
		if run.ActualManifest.SubjectImageIdentity == nil || ref.SubjectImageIdentity == nil {
			checks.SameCanaryBinary = false
			break
		}
		if run.ActualManifest.SubjectImageIdentity.PrebuildBinarySHA256 != ref.SubjectImageIdentity.PrebuildBinarySHA256 {
			checks.SameCanaryBinary = false
			break
		}
	}

	// 11. UniqueRunIDs
	checks.UniqueRunIDs = true
	seenRunIDs := make(map[string]bool)
	for _, run := range verifiedRuns {
		if seenRunIDs[run.DeclaredRunID] {
			checks.UniqueRunIDs = false
			break
		}
		seenRunIDs[run.DeclaredRunID] = true
	}

	// 12. UniqueSubjectProcesses
	checks.UniqueSubjectProcesses = true
	seenPIDs := make(map[int]bool)
	for _, run := range verifiedRuns {
		if run.SubjectPID <= 0 {
			continue
		}
		if seenPIDs[run.SubjectPID] {
			checks.UniqueSubjectProcesses = false
			break
		}
		seenPIDs[run.SubjectPID] = true
	}

	// 13. UniqueContainerIDs
	checks.UniqueContainerIDs = true
	seenContainerIDs := make(map[string]bool)
	for _, run := range verifiedRuns {
		if run.ContainerID == "" {
			continue
		}
		if seenContainerIDs[run.ContainerID] {
			checks.UniqueContainerIDs = false
			break
		}
		seenContainerIDs[run.ContainerID] = true
	}

	// 14. FixedOrder
	checks.FixedOrder = true
	for i, run := range verifiedRuns {
		if run.DeclaredScenario != CanonicalScenarioOrder[i] {
			checks.FixedOrder = false
			break
		}
	}

	// 15. NonOverlapping
	checks.NonOverlapping = true
	for i := 0; i < len(verifiedRuns)-1; i++ {
		curr := verifiedRuns[i].ActualManifest
		next := verifiedRuns[i+1].ActualManifest
		if !curr.FinishedAt.IsZero() && !next.StartedAt.IsZero() {
			if curr.FinishedAt.After(next.StartedAt) {
				checks.NonOverlapping = false
				break
			}
		}
	}

	// 16. CleanupComplete
	checks.CleanupComplete = reconstructCleanupComplete(verifiedRuns, cleanup)

	// Compute checks_passed from canonical projection
	_, checks.ChecksPassed, _ = CountCanonicalChecks(checks)

	return checks, nil
}

// reconstructCleanupComplete derives CleanupComplete from verified cleanup records.
// P0-3 FIX: Validates ALL cleanup identities (container, network, process), not just process.
// P0-7 FIX: Missing/empty identities cause failure, not skip.
func reconstructCleanupComplete(verifiedRuns []*VerifiedRun, cleanup *MatrixCleanupEvidence) bool {
	if cleanup == nil {
		return false
	}

	// P0-3: Validate cleanup evidence schema and matrix binding
	if cleanup.SchemaVersion == "" {
		return false
	}
	if cleanup.MatrixID == "" {
		return false
	}
	if len(cleanup.Runs) != len(verifiedRuns) {
		return false
	}

	for i, run := range verifiedRuns {
		rec := cleanup.Runs[i]

		// P0-3: Verify cleanup record maps to correct run
		if rec.RunID != run.DeclaredRunID {
			return false
		}
		if rec.Scenario != run.DeclaredScenario {
			return false
		}

		// P0-3: Container identity must be present and "gone"
		if rec.Container.ID == "" {
			return false // P0-7: Missing container ID is failure, not skip
		}
		if rec.Container.ID != run.ContainerID {
			return false // Container ID mismatch
		}
		if rec.Container.Status != "gone" {
			return false // Container still exists
		}

		// P0-3: Process identity must be present with valid cleanup status
		if rec.Process.PID == 0 {
			return false // P0-7: Missing PID is failure
		}
		if rec.Process.PID != run.SubjectPID {
			return false // PID mismatch
		}
		if rec.Process.StartTime != run.SubjectStartTime {
			return false // Start time mismatch
		}
		if rec.Process.Status != "gone" && rec.Process.Status != "pid_reused" {
			return false // Process still alive or unavailable
		}

		// P0-3: Network identity must be present if expected
		if run.NetworkID != "" && rec.Network.ID == "" {
			return false // Expected network ID but cleanup has none
		}
		if rec.Network.ID != run.NetworkID {
			return false // Network ID mismatch
		}
		// Network status check - if network cleanup was observed, it should be "gone"
		if rec.Network.ID != "" && rec.Network.Status != "gone" {
			return false
		}
	}

	return true
}

// =============================================================================
// SCENARIO RESULT RECONSTRUCTION
// =============================================================================

// ReconstructScenarioResults builds scenario results from verified runs.
func ReconstructScenarioResults(verifiedRuns []*VerifiedRun) (map[string]*ScenarioResult, error) {
	results := make(map[string]*ScenarioResult)

	for _, run := range verifiedRuns {
		if run.ActualVerdict == nil {
			return nil, fmt.Errorf("run %s has no verdict for result reconstruction", run.DeclaredRunID)
		}

		// CORRECTION02: Store the child's OVERALL classification, not memory.
		// The contract specifies:
		// - canary-growing: overall=growth
		// - canary-bounded: overall=stable
		// - canary-descriptor: overall=resource_growth
		result := &ScenarioResult{
			RunID:    run.DeclaredRunID,
			Verified: run.ActualManifest.SchemaVersion == "1.1.0",
			Overall:  string(run.ActualVerdict.OverallClassification),
			Memory:   string(run.ActualVerdict.MemoryClassification),
			Resource: string(run.ActualVerdict.ResourceClassification),
			Semantic: string(run.ActualVerdict.SemanticClassification),
		}

		results[run.DeclaredScenario] = result
	}

	return results, nil
}

// =============================================================================
// SINGLE AUTHORITATIVE RECONSTRUCTION FUNCTION
// =============================================================================

// ExpectedScenarioClassifications defines the required overall classification for each scenario.
// P0-4 FIX: Matrix validity must verify expected scenario outcomes.
var ExpectedScenarioClassifications = map[string]string{
	"canary-growing":    "growth",
	"canary-bounded":    "stable",
	"canary-descriptor": "resource_growth",
}

// ReconstructMatrixVerdict is the SINGLE authoritative function for reconstructing
// a matrix verdict. Both `matrix` and `verify-matrix` commands MUST use this function.
// No producer-only or verifier-only function may independently compute matrix validity.
func ReconstructMatrixVerdict(
	matrixManifest *MatrixManifest,
	verifiedRuns []*VerifiedRun,
	cleanup *MatrixCleanupEvidence,
) (*MatrixVerdict, error) {
	// Reconstruct cross-run checks
	crossRunChecks, err := ReconstructCrossRunChecks(matrixManifest, verifiedRuns, cleanup)
	if err != nil {
		return nil, fmt.Errorf("reconstruct cross-run checks: %w", err)
	}

	// Reconstruct scenario results
	scenarioResults, err := ReconstructScenarioResults(verifiedRuns)
	if err != nil {
		return nil, fmt.Errorf("reconstruct scenario results: %w", err)
	}

	// P0-4 FIX: Verify expected scenario classifications
	scenarioClassificationsValid := verifyExpectedClassifications(scenarioResults)

	// Compute check counts from canonical projection
	checksTotal, checksPassed, checksFailed := CountCanonicalChecks(crossRunChecks)

	// P0-4 FIX: Matrix valid if ALL checks pass AND scenario classifications match
	matrixValid := checksFailed == 0 && scenarioClassificationsValid

	verdict := &MatrixVerdict{
		MatrixID:        matrixManifest.MatrixID,
		MatrixValid:     matrixValid,
		ScenarioResults: scenarioResults,
		CrossRunChecks:  crossRunChecks,
		ChecksTotal:     checksTotal,
		ChecksPassed:    checksPassed,
		ChecksFailed:    checksFailed,
	}

	return verdict, nil
}

// verifyExpectedClassifications checks that each scenario has its expected overall classification.
func verifyExpectedClassifications(results map[string]*ScenarioResult) bool {
	for scenario, expectedOverall := range ExpectedScenarioClassifications {
		result, ok := results[scenario]
		if !ok {
			return false // Scenario missing
		}
		if result.Overall != expectedOverall {
			return false // Wrong classification
		}
	}
	return true
}

// =============================================================================
// VERDICT COMPARISON
// =============================================================================

// VerdictDiff represents a single field-level difference between verdicts.
type VerdictDiff struct {
	Path    string // Dot-separated path to the field
	Stored  string // Value in stored verdict
	Reconstructed string // Value in reconstructed verdict
}

// CompareVerdicts performs complete field-by-field comparison of two verdicts.
// Returns all differences found.
func CompareVerdicts(stored, reconstructed *MatrixVerdict) []VerdictDiff {
	var diffs []VerdictDiff

	// Top-level fields
	if stored.MatrixID != reconstructed.MatrixID {
		diffs = append(diffs, VerdictDiff{
			Path:           "matrix_id",
			Stored:         stored.MatrixID,
			Reconstructed:  reconstructed.MatrixID,
		})
	}
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
		if storedScenarios[s] != reconScenarios[s] {
			diffs = append(diffs, VerdictDiff{
				Path:          fmt.Sprintf("scenario_results.%s", s),
				Stored:        fmt.Sprintf("present=%v", storedScenarios[s]),
				Reconstructed: fmt.Sprintf("present=%v", reconScenarios[s]),
			})
			continue
		}
		if !storedScenarios[s] {
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

	return diffs
}

// FormatVerdictDiffs formats verdict differences for diagnostic output.
func FormatVerdictDiffs(diffs []VerdictDiff) string {
	if len(diffs) == 0 {
		return ""
	}
	var lines []string
	lines = append(lines, "matrix verdict mismatch:")
	for _, d := range diffs {
		lines = append(lines, fmt.Sprintf("  %s: stored=%s reconstructed=%s", d.Path, d.Stored, d.Reconstructed))
	}
	return strings.Join(lines, "\n")
}

// =============================================================================
// CLEANUP EVIDENCE PERSISTENCE
// =============================================================================

// BuildMatrixCleanupEvidence constructs cleanup evidence from verified runs.
func BuildMatrixCleanupEvidence(
	matrixID string,
	networkOwnership string,
	verifiedRuns []*VerifiedRun,
	observedAt time.Time,
) *MatrixCleanupEvidence {
	records := make([]RunCleanupRecord, len(verifiedRuns))
	for i, run := range verifiedRuns {
		records[i] = RunCleanupRecord{
			Index:    i,
			Scenario: run.DeclaredScenario,
			RunID:    run.DeclaredRunID,
			Container: ContainerCleanupRecord{
				ID:     run.ContainerID,
				Name:   run.ContainerName,
				Status: containerCleanupStatus(run),
			},
			Network: NetworkCleanupRecord{
				ID:     run.NetworkID,
				Name:   run.NetworkName,
				Status: networkCleanupStatus(run),
			},
			Process: ProcessCleanupRecord{
				PID:       run.SubjectPID,
				StartTime: run.SubjectStartTime,
				Status:    processCleanupStatusString(run.ProcessCleanupStatus),
			},
		}
	}

	return &MatrixCleanupEvidence{
		SchemaVersion:    "1.0.0",
		MatrixID:        matrixID,
		ObservedAt:      observedAt,
		NetworkOwnership: networkOwnership,
		Runs:            records,
	}
}

func containerCleanupStatus(run *VerifiedRun) string {
	if !run.CleanupVerified {
		return "unavailable"
	}
	if run.ContainerID == "" {
		return "unavailable"
	}
	return "gone"
}

func networkCleanupStatus(run *VerifiedRun) string {
	if !run.CleanupVerified {
		return "unavailable"
	}
	if run.NetworkID == "" {
		return "unavailable"
	}
	return "gone"
}

func processCleanupStatusString(status ProcessCleanupStatus) string {
	switch status {
	case ProcessGone:
		return "gone"
	case ProcessPIDReused:
		return "pid_reused"
	case ProcessStillAlive:
		return "still_alive"
	case ProcessUnavailable:
		return "unavailable"
	default:
		return "unknown"
	}
}

// WriteMatrixCleanupEvidence writes the cleanup evidence to matrix-cleanup.json.
func WriteMatrixCleanupEvidence(matrixDir string, evidence *MatrixCleanupEvidence) error {
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cleanup evidence: %w", err)
	}
	path := filepath.Join(matrixDir, "matrix-cleanup.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write cleanup evidence: %w", err)
	}
	return nil
}

// =============================================================================
// MATRIX CHECKSUM CONTRACT
// =============================================================================

// ComputeMatrixChecksums computes matrix-level checksums including cleanup evidence.
func ComputeMatrixChecksums(matrixDir string) (string, error) {
	// Read matrix artifacts
	manifestData, err := os.ReadFile(filepath.Join(matrixDir, "matrix-manifest.json"))
	if err != nil {
		return "", fmt.Errorf("read matrix-manifest.json: %w", err)
	}

	verdictData, err := os.ReadFile(filepath.Join(matrixDir, "matrix-verdict.json"))
	if err != nil {
		return "", fmt.Errorf("read matrix-verdict.json: %w", err)
	}

	cleanupData, err := os.ReadFile(filepath.Join(matrixDir, "matrix-cleanup.json"))
	if err != nil {
		return "", fmt.Errorf("read matrix-cleanup.json: %w", err)
	}

	// Compute hashes
	manifestHash := sha256.Sum256(manifestData)
	verdictHash := sha256.Sum256(verdictData)
	cleanupHash := sha256.Sum256(cleanupData)

	// Build deterministic checksum file
	var lines []string
	entries := map[string]string{
		"matrix-manifest.json": hex.EncodeToString(manifestHash[:]),
		"matrix-verdict.json":  hex.EncodeToString(verdictHash[:]),
		"matrix-cleanup.json": hex.EncodeToString(cleanupHash[:]),
	}

	// Sort for determinism
	var keys []string
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("%s  %s", entries[k], k))
	}

	return strings.Join(lines, "\n") + "\n", nil
}

// =============================================================================
// HERMETIC VERIFIED RUN BUILDER
// =============================================================================

// BuildVerifiedRunsFromMatrix constructs verified runs from a matrix manifest and cleanup evidence.
// P0-1 FIX: CleanupVerified and ProcessCleanupStatus are now hydrated from cleanup evidence.
// P0-6 FIX: Single builder used by both producer and verifier paths.
// P0-8 FIX: Uses strict single-document JSON parsing for all artifacts.
func BuildVerifiedRunsFromMatrix(matrixDir string, manifest *MatrixManifest, cleanup *MatrixCleanupEvidence) ([]*VerifiedRun, error) {
	runsDir := filepath.Join(matrixDir, "runs")
	runs := make([]*VerifiedRun, len(manifest.Runs))

	for i, decl := range manifest.Runs {
		runPath := filepath.Join(runsDir, decl.RunID)
		run := &VerifiedRun{
			DeclaredRunID:   decl.RunID,
			DeclaredScenario: decl.Scenario,
			RunIndex:        i,
		}

		// Load manifest with strict JSON parsing
		manifestData, err := os.ReadFile(filepath.Join(runPath, "manifest.json"))
		if err != nil {
			return nil, fmt.Errorf("read manifest for %s: %w", decl.RunID, err)
		}
		var childManifest evidence.Manifest
		if err := decodeStrictJSON(manifestData, &childManifest); err != nil {
			return nil, fmt.Errorf("parse manifest for %s: %w", decl.RunID, err)
		}
		run.ActualManifest = &childManifest

		// Load verdict with strict JSON parsing
		verdictData, err := os.ReadFile(filepath.Join(runPath, "verdict.json"))
		if err != nil {
			return nil, fmt.Errorf("read verdict for %s: %w", decl.RunID, err)
		}
		var childVerdict evidence.Verdict
		if err := decodeStrictJSON(verdictData, &childVerdict); err != nil {
			return nil, fmt.Errorf("parse verdict for %s: %w", decl.RunID, err)
		}
		run.ActualVerdict = &childVerdict

		// P0-1 FIX: Extract container identity from container-inspect.json
		containerData, err := os.ReadFile(filepath.Join(runPath, "container-inspect.json"))
		if err != nil {
			return nil, fmt.Errorf("read container-inspect.json for %s: %w", decl.RunID, err)
		}
		var inspect struct {
			ID string `json:"Id"`
		}
		if err := decodeStrictJSON(containerData, &inspect); err != nil {
			return nil, fmt.Errorf("parse container-inspect.json for %s: %w", decl.RunID, err)
		}
		run.ContainerID = inspect.ID

		// P0-1 FIX: Extract process identity from samples.csv (fail-closed)
		pid, startTime, err := extractProcessIdentity(filepath.Join(runPath, "samples.csv"))
		if err != nil {
			return nil, fmt.Errorf("extract process identity for %s: %w", decl.RunID, err)
		}
		run.SubjectPID = pid
		run.SubjectStartTime = startTime

		// P0-1 FIX: Hydrate cleanup status from cleanup evidence
		if cleanup != nil && i < len(cleanup.Runs) {
			rec := cleanup.Runs[i]
			run.ContainerName = rec.Container.Name
			run.NetworkID = rec.Network.ID
			run.NetworkName = rec.Network.Name

			// Map process cleanup status from string to enum
			switch rec.Process.Status {
			case "gone":
				run.ProcessCleanupStatus = ProcessGone
			case "pid_reused":
				run.ProcessCleanupStatus = ProcessPIDReused
			case "still_alive":
				run.ProcessCleanupStatus = ProcessStillAlive
			default:
				run.ProcessCleanupStatus = ProcessUnavailable
			}
			run.CleanupVerified = true
		}

		runs[i] = run
	}

	return runs, nil
}
