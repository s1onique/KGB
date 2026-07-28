// production_finalize.go — Hermetic production finalization seam.
//
// This module provides a testable seam for the production qualified
// evidence finalization path. It enables hermetic tests without
// requiring Docker.
//
// Reference: ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION51-CORRECTION03
package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/canarycontrol"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
)

// ManifestSchemaVersion is the expected schema version for manifests.
const ManifestSchemaVersion = "1.1.0"

// Production evidence error types for fail-closed behavior.
var (
	// ErrProductionEvidenceMissing is returned when the production
	// qualified evidence file is not present at the expected path.
	ErrProductionEvidenceMissing = errors.New("production qualified evidence missing")

	// ErrProductionEvidenceMismatch is returned when the persisted
	// evidence does not match the returned in-memory evidence.
	ErrProductionEvidenceMismatch = errors.New("production qualified evidence mismatch")

	// ErrProductionEvidenceNotInventoried is returned when the
	// evidence file is not included in the final artifact inventory.
	ErrProductionEvidenceNotInventoried = errors.New("production qualified evidence not inventoried")

	// ErrNilDependency is returned when a required dependency is nil.
	ErrNilDependency = errors.New("nil dependency")

	// ErrMalformedManifest is returned when the manifest cannot be parsed strictly.
	ErrMalformedManifest = errors.New("malformed manifest")

	// ErrMalformedChecksums is returned when checksums cannot be parsed.
	ErrMalformedChecksums = errors.New("malformed checksums")

	// ErrChecksumMismatch is returned when the checksum does not match.
	ErrChecksumMismatch = errors.New("checksum mismatch")

	// ErrDuplicateInventoryEntry is returned when inventory contains duplicates.
	ErrDuplicateInventoryEntry = errors.New("duplicate inventory entry")
)

// WriteManifestFunc is the typed manifest writer function.
// Path-authoritative: caller provides the exact destination path.
type WriteManifestFunc func(path string, manifest *Manifest) error

// WriteChecksumsFunc is the typed checksum writer function.
// Path-authoritative: caller provides the exact destination path and artifact root.
type WriteChecksumsFunc func(path string, artifactRoot string, inventory []string) error

// ExecuteQualifiedLifecycleFunc is the typed lifecycle executor function.
type ExecuteQualifiedLifecycleFunc func(
	context.Context,
	dockerlab.LifecycleOptions,
	string, // producerVersion
) (*dockerlab.QualifiedLifecycleOutcome, error)

// PersistFinalEvidenceFunc is the typed evidence persistence function.
type PersistFinalEvidenceFunc func(
	context.Context,
	*dockerlab.QualifiedLifecycleOutcome,
	ControllerProvenance,
	string, // artifactDir
) (*QualifiedExecutionEvidence, error)

// CollectProvenanceFunc is the typed provenance collector function.
type CollectProvenanceFunc func(ProvenanceOptions) (ControllerProvenance, error)

// ProductionQualifiedRunDependencies contains the injectable dependencies
// for hermetic production finalization tests.
type ProductionQualifiedRunDependencies struct {
	// ExecuteLifecycle is the lifecycle executor. Tests may inject a
	// recording mock or a deterministic fixture.
	ExecuteLifecycle ExecuteQualifiedLifecycleFunc

	// CollectProvenance collects controller provenance. Tests may inject
	// a recording mock or a deterministic fixture.
	CollectProvenance CollectProvenanceFunc

	// PersistFinalEvidence produces and persists qualified evidence.
	// Tests may inject a recording mock or the real producer.
	PersistFinalEvidence PersistFinalEvidenceFunc

	// VerifyEvidenceBytes verifies persisted evidence bytes.
	// Tests may inject the real verifier or a recording mock.
	VerifyEvidenceBytes func([]byte) (VerifyQualifiedExecutionResult, error)

	// WriteManifest writes the final manifest to disk.
	// Tests may inject a recording mock or the real writer.
	// Path-authoritative: receives the exact manifest path.
	WriteManifest WriteManifestFunc

	// WriteChecksums writes the checksums to disk.
	// Tests may inject a recording mock or the real writer.
	// Path-authoritative: receives the exact checksum path and artifact root.
	WriteChecksums WriteChecksumsFunc
}

// ProductionQualifiedRunOptions contains the options for a production run.
type ProductionQualifiedRunOptions struct {
	// RepositoryRoot is the Git repository root directory for provenance.
	RepositoryRoot string

	// ArtifactRoot is the root artifacts directory.
	ArtifactRoot string

	// RunID is the unique run identifier.
	RunID string

	// Scenario is the scenario name.
	Scenario string

	// ProducerVersion is the producer version string.
	ProducerVersion string

	// DockerVersion is the Docker server version string.
	DockerVersion string

	// LifecycleOptions are the options passed to the lifecycle executor.
	LifecycleOptions dockerlab.LifecycleOptions

	// ExpectedInventory is the complete list of expected artifact paths.
	// The evidence file MUST be included in this list.
	ExpectedInventory []string
}

// ProductionFinalizationResult contains the result of production finalization.
type ProductionFinalizationResult struct {
	Evidence         *QualifiedExecutionEvidence
	EvidencePath     string
	EvidenceBytes    []byte
	ManifestPath     string
	ChecksumsPath    string
	Inventory        []string
	ProducerCalled   bool
	ManifestWritten  bool
	ChecksumsWritten bool
}

// FinalizeProductionQualifiedRun is the canonical production finalization seam.
// It orchestrates the exact phase order required by the production contract:
//
//   - lifecycle returns
//   - validate finalized outcome
//   - collect running-production provenance (from RepositoryRoot)
//   - call canonical producer exactly once
//   - verify returned evidence semantic content
//   - read and verify persisted evidence bytes
//   - bind returned and persisted evidence content
//   - write final manifest (includes evidence)
//   - write checksums (includes evidence)
//   - return success
func FinalizeProductionQualifiedRun(
	ctx context.Context,
	dependencies ProductionQualifiedRunDependencies,
	options ProductionQualifiedRunOptions,
) (*ProductionFinalizationResult, error) {
	if ctx == nil {
		return nil, errors.New("context is nil")
	}
	if options.RepositoryRoot == "" {
		return nil, errors.New("repository root is empty")
	}
	if options.ArtifactRoot == "" {
		return nil, errors.New("artifact root is empty")
	}
	if options.RunID == "" {
		return nil, errors.New("run ID is empty")
	}
	if options.ProducerVersion == "" {
		return nil, errors.New("producer version is empty")
	}

	// Validate dependencies before execution (fail-closed)
	if dependencies.ExecuteLifecycle == nil {
		return nil, fmt.Errorf("%w: ExecuteLifecycle", ErrNilDependency)
	}
	if dependencies.CollectProvenance == nil {
		return nil, fmt.Errorf("%w: CollectProvenance", ErrNilDependency)
	}
	if dependencies.PersistFinalEvidence == nil {
		return nil, fmt.Errorf("%w: PersistFinalEvidence", ErrNilDependency)
	}
	if dependencies.VerifyEvidenceBytes == nil {
		return nil, fmt.Errorf("%w: VerifyEvidenceBytes", ErrNilDependency)
	}
	if dependencies.WriteManifest == nil {
		return nil, fmt.Errorf("%w: WriteManifest", ErrNilDependency)
	}
	if dependencies.WriteChecksums == nil {
		return nil, fmt.Errorf("%w: WriteChecksums", ErrNilDependency)
	}

	artifactPath := filepath.Join(options.ArtifactRoot, options.RunID)

	// Phase 1: Execute lifecycle
	outcome, err := dependencies.ExecuteLifecycle(ctx, options.LifecycleOptions, options.ProducerVersion)
	if err != nil {
		return nil, fmt.Errorf("lifecycle: %w", err)
	}

	// Phase 2: Validate finalized outcome
	if outcome == nil {
		return nil, errors.New("lifecycle returned nil outcome")
	}
	if outcome.Observations == nil {
		return nil, errors.New("lifecycle outcome observations is nil")
	}
	if !outcome.Terminal {
		return nil, ErrFinalQualifiedOutcomeIncomplete
	}

	// Phase 3: Collect provenance using RepositoryRoot (NOT ArtifactRoot)
	cp, err := dependencies.CollectProvenance(ProvenanceOptions{
		RepoDir:             options.RepositoryRoot,
		ProducerVersion:     options.ProducerVersion,
		DockerServerVersion: options.DockerVersion,
		CleanPolicy:         ProvenanceRequireClean,
	})
	if err != nil {
		return nil, fmt.Errorf("collect provenance: %w", err)
	}

	// Phase 4: Call canonical producer exactly once
	evidence, err := dependencies.PersistFinalEvidence(ctx, outcome, cp, artifactPath)
	if err != nil {
		return nil, fmt.Errorf("persist evidence: %w", err)
	}
	if evidence == nil {
		return nil, errors.New("producer returned nil evidence")
	}

	// Phase 5: Verify returned evidence
	if !evidence.Pass {
		return nil, errors.New("producer returned pass=false")
	}
	if !evidence.ImageExactIDMatch || !evidence.NetworkExactIDMatch || !evidence.CleanupComplete {
		return nil, errors.New("producer returned evidence with false claims")
	}

	// Phase 6: Confirm physical artifact exists
	evidencePath := filepath.Join(artifactPath, "qualified-execution-evidence.json")
	evidenceBytes, err := os.ReadFile(evidencePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProductionEvidenceMissing, err)
	}

	// Phase 7: Verify persisted bytes with strict decoder
	verified, err := dependencies.VerifyEvidenceBytes(evidenceBytes)
	if err != nil {
		return nil, fmt.Errorf("verify evidence bytes: %w", err)
	}
	if !verified.Pass {
		return nil, fmt.Errorf("persisted evidence verification failed: %v", verified.Errors)
	}

	// Phase 8: Bind returned and persisted evidence content (complete comparison)
	if err := bindReturnedAndPersistedEvidence(evidence, evidenceBytes); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProductionEvidenceMismatch, err)
	}

	// Phase 9: Verify evidence is in expected inventory
	if !isInInventory("qualified-execution-evidence.json", options.ExpectedInventory) {
		return nil, ErrProductionEvidenceNotInventoried
	}

	// Phase 10: Build final manifest with evidence entry
	manifestPath := filepath.Join(artifactPath, "manifest.json")
	finalInventory := make([]string, 0, len(options.ExpectedInventory))
	hasEvidence := false
	for _, item := range options.ExpectedInventory {
		finalInventory = append(finalInventory, item)
		if item == "qualified-execution-evidence.json" {
			hasEvidence = true
		}
	}
	// Ensure evidence is in inventory
	if !hasEvidence {
		finalInventory = append(finalInventory, "qualified-execution-evidence.json")
	}

	// Phase 11: Write final manifest (includes evidence in inventory)
	manifest := &Manifest{
		SchemaVersion:     "1.1.0",
		RunID:             options.RunID,
		Scenario:          options.Scenario,
		ArtifactInventory: finalInventory,
	}
	// Calculate path-authoritative manifest path
	if err := dependencies.WriteManifest(manifestPath, manifest); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}

	// Phase 12: Write checksums (includes evidence)
	checksumsPath := filepath.Join(artifactPath, "checksums.txt")
	// P0-2: Inventory entries are relative to the run directory (artifactPath),
	// not the parent artifacts directory. Writer must resolve paths against artifactPath.
	if err := dependencies.WriteChecksums(checksumsPath, artifactPath, finalInventory); err != nil {
		return nil, fmt.Errorf("write checksums: %w", err)
	}

	// Phase 13: Strictly verify physical manifest
	if err := verifyPhysicalManifest(manifestPath, options.RunID, options.Scenario, finalInventory); err != nil {
		return nil, fmt.Errorf("verify physical manifest: %w", err)
	}

	// Phase 14: Verify physical checksum digest
	if err := verifyPhysicalChecksums(checksumsPath, evidencePath, finalInventory); err != nil {
		return nil, fmt.Errorf("verify physical checksums: %w", err)
	}

	return &ProductionFinalizationResult{
		Evidence:         evidence,
		EvidencePath:     evidencePath,
		EvidenceBytes:    evidenceBytes,
		ManifestPath:     manifestPath,
		ChecksumsPath:    checksumsPath,
		Inventory:        finalInventory,
		ProducerCalled:   true,
		ManifestWritten:  true,
		ChecksumsWritten: true,
	}, nil
}

// DecodeQualifiedExecutionEvidenceExactlyOne decodes evidence using strict JSON decoding.
// It rejects unknown fields, multiple documents, and trailing data.
func DecodeQualifiedExecutionEvidenceExactlyOne(data []byte) (*QualifiedExecutionEvidence, error) {
	if len(data) == 0 {
		return nil, errors.New("empty input")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var evidence QualifiedExecutionEvidence
	if err := decoder.Decode(&evidence); err != nil {
		return nil, fmt.Errorf("decode evidence: %w", err)
	}

	// Require EOF after first document
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple JSON documents not allowed")
		}
		return nil, fmt.Errorf("unexpected token after document: %w", err)
	}

	return &evidence, nil
}

// bindReturnedAndPersistedEvidence verifies that the returned evidence
// semantically matches the persisted evidence bytes using complete field comparison.
func bindReturnedAndPersistedEvidence(returned *QualifiedExecutionEvidence, persistedBytes []byte) error {
	// Step 1: Strictly decode persisted evidence
	persisted, err := DecodeQualifiedExecutionEvidenceExactlyOne(persistedBytes)
	if err != nil {
		return fmt.Errorf("strictly decode persisted evidence: %w", err)
	}

	// Step 2: Verify returned evidence fields
	if !returned.Pass {
		return errors.New("returned evidence has pass=false")
	}
	if !returned.ImageExactIDMatch {
		return errors.New("returned evidence has image_exact_id_match=false")
	}
	if !returned.NetworkExactIDMatch {
		return errors.New("returned evidence has network_exact_id_match=false")
	}
	if !returned.CleanupComplete {
		return errors.New("returned evidence has cleanup_complete=false")
	}

	// Step 3: Complete semantic comparison of all fields
	if returned.SchemaVersion != persisted.SchemaVersion {
		return fmt.Errorf("SchemaVersion mismatch: returned=%q persisted=%q",
			returned.SchemaVersion, persisted.SchemaVersion)
	}

	if returned.ImageExactIDMatch != persisted.ImageExactIDMatch {
		return fmt.Errorf("ImageExactIDMatch mismatch: returned=%v persisted=%v",
			returned.ImageExactIDMatch, persisted.ImageExactIDMatch)
	}

	if returned.NetworkExactIDMatch != persisted.NetworkExactIDMatch {
		return fmt.Errorf("NetworkExactIDMatch mismatch: returned=%v persisted=%v",
			returned.NetworkExactIDMatch, persisted.NetworkExactIDMatch)
	}

	if returned.CleanupComplete != persisted.CleanupComplete {
		return fmt.Errorf("CleanupComplete mismatch: returned=%v persisted=%v",
			returned.CleanupComplete, persisted.CleanupComplete)
	}

	if returned.Pass != persisted.Pass {
		return fmt.Errorf("Pass mismatch: returned=%v persisted=%v",
			returned.Pass, persisted.Pass)
	}

	// Provenance comparison
	if returned.Provenance.SourceCommit != persisted.Provenance.SourceCommit {
		return fmt.Errorf("SourceCommit mismatch: returned=%q persisted=%q",
			returned.Provenance.SourceCommit, persisted.Provenance.SourceCommit)
	}

	if returned.Provenance.SourceTree != persisted.Provenance.SourceTree {
		return fmt.Errorf("SourceTree mismatch: returned=%q persisted=%q",
			returned.Provenance.SourceTree, persisted.Provenance.SourceTree)
	}

	if returned.Provenance.ExecutableSHA256 != persisted.Provenance.ExecutableSHA256 {
		return fmt.Errorf("ExecutableSHA256 mismatch: returned=%q persisted=%q",
			returned.Provenance.ExecutableSHA256, persisted.Provenance.ExecutableSHA256)
	}

	if returned.Provenance.DockerServerVersion != persisted.Provenance.DockerServerVersion {
		return fmt.Errorf("DockerServerVersion mismatch: returned=%q persisted=%q",
			returned.Provenance.DockerServerVersion, persisted.Provenance.DockerServerVersion)
	}

	if returned.Provenance.ProducerVersion != persisted.Provenance.ProducerVersion {
		return fmt.Errorf("ProducerVersion mismatch: returned=%q persisted=%q",
			returned.Provenance.ProducerVersion, persisted.Provenance.ProducerVersion)
	}

	// Reachability comparison
	if returned.Reachability.Success != persisted.Reachability.Success {
		return fmt.Errorf("Reachability.Success mismatch: returned=%v persisted=%v",
			returned.Reachability.Success, persisted.Reachability.Success)
	}

	// Pull observations comparison
	if returned.Pull.AttemptCount != persisted.Pull.AttemptCount {
		return fmt.Errorf("Pull.AttemptCount mismatch: returned=%d persisted=%d",
			returned.Pull.AttemptCount, persisted.Pull.AttemptCount)
	}

	return nil
}

// DecodeManifestExactlyOne decodes a manifest using strict JSON decoding.
// It rejects unknown fields, multiple documents, and trailing data.
func DecodeManifestExactlyOne(data []byte) (*Manifest, error) {
	if len(data) == 0 {
		return nil, errors.New("empty input")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}

	// Require EOF after first document
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple JSON documents not allowed")
		}
		return nil, fmt.Errorf("unexpected token after document: %w", err)
	}

	return &manifest, nil
}

// verifyPhysicalManifest strictly verifies the physical manifest file.
// It performs exact ordered equality for the inventory, rejecting substitutions,
// reordering, duplicates, missing/extra entries, unknown JSON members, and
// second JSON values.
func verifyPhysicalManifest(manifestPath, expectedRunID, expectedScenario string, expectedInventory []string) error {
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}

	// Strictly decode the manifest
	manifest, err := DecodeManifestExactlyOne(manifestBytes)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedManifest, err)
	}

	// Verify schema version
	if manifest.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("%w: schema version mismatch: got %q, want %q",
			ErrMalformedManifest, manifest.SchemaVersion, ManifestSchemaVersion)
	}

	// Verify run ID
	if manifest.RunID != expectedRunID {
		return fmt.Errorf("run ID mismatch: got %q, want %q", manifest.RunID, expectedRunID)
	}

	// Verify scenario
	if manifest.Scenario != expectedScenario {
		return fmt.Errorf("scenario mismatch: got %q, want %q", manifest.Scenario, expectedScenario)
	}

	// P0-3: EXACT ORDERED EQUALITY for inventory.
	// Reject: substitutions, reordering, duplicates, missing entries, extra entries.

	// Check for duplicate inventory entries
	seen := make(map[string]bool)
	for i, item := range manifest.ArtifactInventory {
		if seen[item] {
			return fmt.Errorf("%w: %q appears more than once at positions %d and earlier",
				ErrDuplicateInventoryEntry, item, i)
		}
		seen[item] = true
	}

	// Count evidence in inventory
	evidenceCount := 0
	for _, item := range manifest.ArtifactInventory {
		if item == "qualified-execution-evidence.json" {
			evidenceCount++
		}
	}

	if evidenceCount != 1 {
		return fmt.Errorf("evidence appears %d times in manifest, want exactly 1", evidenceCount)
	}

	// EXACT ORDERED EQUALITY: length must match
	if len(manifest.ArtifactInventory) != len(expectedInventory) {
		return fmt.Errorf("inventory length mismatch: got %d, want %d",
			len(manifest.ArtifactInventory), len(expectedInventory))
	}

	// EXACT ORDERED EQUALITY: each position must match
	for i := range manifest.ArtifactInventory {
		if manifest.ArtifactInventory[i] != expectedInventory[i] {
			return fmt.Errorf("inventory position %d mismatch: got %q, want %q",
				i, manifest.ArtifactInventory[i], expectedInventory[i])
		}
	}

	return nil
}

// verifyPhysicalChecksums is the authoritative checksum verifier for the production finalizer.
// It consumes the canonical ParseChecksumsCanonical and ResolveRegularArtifactPath helpers
// to ensure a single authoritative implementation.
//
// For every ChecksummedInventory entry, it:
//   - parses checksums using the canonical parser;
//   - validates each path using the canonical resolver;
//   - reads physical bytes;
//   - recomputes SHA-256;
//   - requires the exact lowercase digest.
func verifyPhysicalChecksums(checksumsPath, evidencePath string, inventory []string) error {
	checksumBytes, err := os.ReadFile(checksumsPath)
	if err != nil {
		return fmt.Errorf("read checksums: %w", err)
	}

	// Use the canonical checksum parser
	entries, err := ParseChecksumsCanonical(checksumBytes)
	if err != nil {
		return fmt.Errorf("%w: parse error: %v", ErrMalformedChecksums, err)
	}

	// Build a set of parsed paths for validation
	seen := make(map[string]bool)
	for _, entry := range entries {
		seen[entry.Path] = true
	}

	// Verify evidence is in checksums
	if !seen["qualified-execution-evidence.json"] {
		return fmt.Errorf("%w: evidence missing from checksums", ErrMalformedChecksums)
	}

	// Verify inventory consistency: every item in inventory must be in checksums
	for _, item := range inventory {
		if !seen[item] {
			return fmt.Errorf("%w: %q from inventory not in checksums", ErrMalformedChecksums, item)
		}
	}

	// Verify checksums are inventory-authoritative: every path in checksums must be in inventory
	for path := range seen {
		if !isInInventory(path, inventory) {
			return fmt.Errorf("%w: %q in checksums but not in inventory", ErrMalformedChecksums, path)
		}
	}

	// Derive the run root from the evidence path
	runRoot := filepath.Dir(evidencePath)

	// For each checksum entry, physically verify the file using the canonical resolver
	for _, entry := range entries {
		// Use the canonical physical resolver
		resolvedPath, err := ResolveRegularArtifactPath(runRoot, entry.Path)
		if err != nil {
			return fmt.Errorf("%w: resolve %q: %v", ErrChecksumMismatch, entry.Path, err)
		}

		// Read physical bytes and recompute SHA-256
		fileBytes, err := os.ReadFile(resolvedPath)
		if err != nil {
			return fmt.Errorf("%w: cannot read %q: %v", ErrChecksumMismatch, entry.Path, err)
		}

		actualDigest := sha256.Sum256(fileBytes)
		actualHex := hex.EncodeToString(actualDigest[:])

		if actualHex != entry.Digest {
			return fmt.Errorf("%w: %q: declared=%s actual=%s",
				ErrChecksumMismatch, entry.Path, entry.Digest, actualHex)
		}
	}

	return nil
}

// isInInventory checks if the given path is in the inventory.
func isInInventory(path string, inventory []string) bool {
	for _, item := range inventory {
		if item == path {
			return true
		}
	}
	return false
}

// ErrInvalidArtifactPath is returned when an artifact path fails validation.
var ErrInvalidArtifactPath = errors.New("invalid artifact path")

// ValidateArtifactPath validates a canonical artifact path using lexical rules
// and physical component walking. It rejects:
// - empty paths
// - "." path
// - absolute paths
// - backslash paths
// - empty, "." or ".." components
// - noncanonical cleaned representations
// - intermediate and final symlinks
// - non-regular files
//
// It proves containment using filepath.Rel.
func ValidateArtifactPath(path string, runRoot string) error {
	// Reject empty
	if path == "" {
		return fmt.Errorf("%w: empty path", ErrInvalidArtifactPath)
	}

	// Reject "."
	if path == "." {
		return fmt.Errorf("%w: path is .", ErrInvalidArtifactPath)
	}

	// Reject absolute
	if filepath.IsAbs(path) {
		return fmt.Errorf("%w: absolute path not allowed", ErrInvalidArtifactPath)
	}

	// Reject backslash
	if strings.ContainsRune(path, '\\') {
		return fmt.Errorf("%w: backslash not allowed", ErrInvalidArtifactPath)
	}

	// Reject noncanonical cleaned representations
	cleaned := filepath.ToSlash(filepath.Clean(path))
	if cleaned != path {
		return fmt.Errorf("%w: noncanonical path %q (cleaned: %q)", ErrInvalidArtifactPath, path, cleaned)
	}

	// Check each component
	components := strings.Split(path, "/")
	for _, part := range components {
		if part == "" {
			return fmt.Errorf("%w: empty component in %q", ErrInvalidArtifactPath, path)
		}
		if part == "." {
			return fmt.Errorf("%w: dot component in %q", ErrInvalidArtifactPath, path)
		}
		if part == ".." {
			return fmt.Errorf("%w: traversal component in %q", ErrInvalidArtifactPath, path)
		}
	}

	// Walk every physical component and reject any symlink
	resolvedPath := filepath.Join(runRoot, path)
	dir := runRoot
	for _, part := range components {
		next := filepath.Join(dir, part)
		info, err := os.Lstat(next)
		if err != nil {
			return fmt.Errorf("%w: cannot stat %q: %v", ErrInvalidArtifactPath, path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlink in path %q (component: %q)", ErrInvalidArtifactPath, path, part)
		}
		dir = next
	}

	// Prove containment using filepath.Rel
	rel, err := filepath.Rel(runRoot, resolvedPath)
	if err != nil {
		return fmt.Errorf("%w: cannot derive relative path: %v", ErrInvalidArtifactPath, err)
	}
	// Must be the same as the original path (no escape)
	if rel != path {
		return fmt.Errorf("%w: path %q escapes run root", ErrInvalidArtifactPath, path)
	}

	// Require regular file
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return fmt.Errorf("%w: cannot stat %q: %v", ErrInvalidArtifactPath, path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %q is not a regular file", ErrInvalidArtifactPath, path)
	}

	return nil
}

// ChecksumEntry represents a parsed checksum line.
type ChecksumEntry struct {
	Path    string
	Digest  string
	LineNum int
}

// ErrMalformedChecksumLine is returned for invalid checksum line format.
var ErrMalformedChecksumLine = errors.New("malformed checksum line")

// ParseChecksumsCanonical parses checksums in canonical format:
// <64 lowercase hex><two ASCII spaces><canonical path><LF>
//
// It rejects:
// - one or three spaces
// - tabs
// - CRLF (requires LF only)
// - missing final LF
// - blank lines
// - comments
// - surrounding whitespace
// - duplicate paths
// - malformed paths
func ParseChecksumsCanonical(data []byte) ([]ChecksumEntry, error) {
	if len(data) == 0 {
		return nil, errors.New("empty input")
	}

	// Check for CRLF - reject if present
	for i := 0; i < len(data); i++ {
		if data[i] == '\r' {
			return nil, fmt.Errorf("%w: CRLF not allowed (line ending must be LF only)", ErrMalformedChecksumLine)
		}
	}

	// Require final LF
	if data[len(data)-1] != '\n' {
		return nil, fmt.Errorf("%w: missing final LF", ErrMalformedChecksumLine)
	}

	lines := strings.Split(string(data), "\n")
	// Remove the empty string after the final LF
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var entries []ChecksumEntry
	seen := make(map[string]int) // path -> first line number for duplicate detection

	for lineNum, line := range lines {
		// Reject blank lines
		if strings.TrimSpace(line) == "" {
			return nil, fmt.Errorf("%w: line %d: blank line", ErrMalformedChecksumLine, lineNum+1)
		}

		// Reject comment lines (starting with #)
		if strings.HasPrefix(line, "#") {
			return nil, fmt.Errorf("%w: line %d: comment not allowed", ErrMalformedChecksumLine, lineNum+1)
		}

		// Require exactly two ASCII spaces as separator
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("%w: line %d: expected 'digest  path' format (exactly two spaces)", ErrMalformedChecksumLine, lineNum+1)
		}

		// No leading/trailing whitespace allowed
		digestHex := parts[0]
		path := parts[1]
		if digestHex != strings.TrimSpace(digestHex) {
			return nil, fmt.Errorf("%w: line %d: no leading/trailing whitespace on digest", ErrMalformedChecksumLine, lineNum+1)
		}
		if path != strings.TrimSpace(path) {
			return nil, fmt.Errorf("%w: line %d: no leading/trailing whitespace on path", ErrMalformedChecksumLine, lineNum+1)
		}

		// Validate digest is exactly 64 lowercase hex characters
		if len(digestHex) != 64 {
			return nil, fmt.Errorf("%w: line %d: digest length %d, want 64", ErrMalformedChecksumLine, lineNum+1, len(digestHex))
		}

		// Validate digest is lowercase hex
		for _, c := range digestHex {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				return nil, fmt.Errorf("%w: line %d: digest must be lowercase hex", ErrMalformedChecksumLine, lineNum+1)
			}
		}

		// Validate path is non-empty
		if path == "" {
			return nil, fmt.Errorf("%w: line %d: empty path", ErrMalformedChecksumLine, lineNum+1)
		}

		// Validate path is safe relative
		if filepath.IsAbs(path) {
			return nil, fmt.Errorf("%w: line %d: absolute path %q not allowed", ErrMalformedChecksumLine, lineNum+1, path)
		}
		if strings.ContainsRune(path, '\\') {
			return nil, fmt.Errorf("%w: line %d: backslash in path %q not allowed", ErrMalformedChecksumLine, lineNum+1, path)
		}
		// Check each path component
		for _, part := range strings.Split(path, "/") {
			if part == ".." {
				return nil, fmt.Errorf("%w: line %d: traversal component '..' not allowed", ErrMalformedChecksumLine, lineNum+1)
			}
			if part == "." {
				return nil, fmt.Errorf("%w: line %d: dot component '.' not allowed", ErrMalformedChecksumLine, lineNum+1)
			}
		}

		// Check for duplicate paths
		if firstLine, exists := seen[path]; exists {
			return nil, fmt.Errorf("%w: %q appears at lines %d and %d", ErrMalformedChecksumLine, path, firstLine, lineNum+1)
		}
		seen[path] = lineNum + 1

		entries = append(entries, ChecksumEntry{
			Path:    path,
			Digest:  digestHex,
			LineNum: lineNum + 1,
		})
	}

	return entries, nil
}

// ResolveRegularArtifactPath resolves an artifact path to its physical location.
// It validates the path and ensures it resolves to a regular file within runRoot.
func ResolveRegularArtifactPath(runRoot string, artifactPath string) (string, error) {
	// Validate the artifact path first
	if err := ValidateArtifactPath(artifactPath, runRoot); err != nil {
		return "", err
	}

	// Resolve and validate runRoot
	if runRoot == "" {
		return "", fmt.Errorf("%w: runRoot is empty", ErrInvalidArtifactPath)
	}
	absRunRoot, err := filepath.Abs(runRoot)
	if err != nil {
		return "", fmt.Errorf("%w: cannot resolve runRoot: %v", ErrInvalidArtifactPath, err)
	}

	// Resolve the full path
	resolvedPath := filepath.Join(absRunRoot, artifactPath)

	// Prove containment using filepath.Rel
	rel, err := filepath.Rel(absRunRoot, resolvedPath)
	if err != nil {
		return "", fmt.Errorf("%w: cannot derive relative path: %v", ErrInvalidArtifactPath, err)
	}
	// Must be the same as the original path (no escape)
	if rel != artifactPath {
		return "", fmt.Errorf("%w: path escapes run root", ErrInvalidArtifactPath)
	}

	// Require regular file
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("%w: cannot stat %q: %v", ErrInvalidArtifactPath, artifactPath, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: %q is not a regular file", ErrInvalidArtifactPath, artifactPath)
	}

	return resolvedPath, nil
}

// RecordOutcome creates a deterministic fixture outcome for testing.
// All operation types use the canonical canarycontrol.Operation constants.
// Phases are conditional: terminal_observed only if terminal=true,
// container_removed only if containerRemoved=true, etc.
func RecordOutcome(terminal, containerRemoved, networkRemoved bool) *dockerlab.QualifiedLifecycleOutcome {
	obs := &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		Image: dockerlab.ImageObservations{
			RequestedReference:    "kgb-tovarisch-canary:latest",
			InspectedBeforeCreate: "sha256:" + strings.Repeat("a", 64),
			CreateRequestImage:    "sha256:" + strings.Repeat("a", 64),
			ContainerInspectImage: "sha256:" + strings.Repeat("a", 64),
			ContainerConfigImage:  "sha256:" + strings.Repeat("a", 64),
		},
		Network: dockerlab.NetworkObservations{
			RequestedName:       "test-net",
			CreateResponseID:    strings.Repeat("b", 64),
			InspectResponseID:   strings.Repeat("b", 64),
			ContainerEndpointID: strings.Repeat("b", 64),
			Removed:             networkRemoved,
		},
		Pull: dockerlab.PullObservations{
			ObservationAvailable: true,
			Attempted:            false,
			AttemptCount:         0,
		},
		Container: dockerlab.ContainerObservations{
			ID:                    "test-container-id",
			Created:               true,
			Inspected:             true,
			Started:               true,
			TerminalStateObserved: terminal,
			Removed:               containerRemoved,
		},
		Reachability: dockerlab.ReachabilityObservations{
			Method:     dockerlab.ReachabilityMethodDockerExec,
			NetworkID:  strings.Repeat("b", 64),
			TargetHost: "127.0.0.1",
			TargetPort: 8080,
			Health: dockerlab.ReachabilityOperationObservation{
				Operation:         canarycontrol.OpHealth,
				ExecExitCode:      0,
				HTTPStatus:        200,
				ResponseValidated: true,
				Mode:              "growing",
			},
			InitialState: dockerlab.ReachabilityOperationObservation{
				Operation:         canarycontrol.OpState,
				ExecExitCode:      0,
				HTTPStatus:        200,
				ResponseValidated: true,
				Mode:              "growing",
			},
			Operate: dockerlab.ReachabilityOperateObservation{
				Operation:         canarycontrol.OpOperate,
				ExecExitCode:      0,
				HTTPStatus:        200,
				Requested:         5,
				Attempted:         5,
				Completed:         5,
				ResponseValidated: true,
			},
			FinalState: dockerlab.ReachabilityOperationObservation{
				Operation:         canarycontrol.OpState,
				ExecExitCode:      0,
				HTTPStatus:        200,
				ResponseValidated: true,
				Mode:              "growing",
			},
			Success: true,
		},
	}

	// Build phases conditionally: exact order, no duplicates
	var phases []dockerlab.QualifiedLifecyclePhase
	phases = append(phases, dockerlab.PhasePrepared)
	phases = append(phases, dockerlab.PhaseStarted)
	phases = append(phases, dockerlab.PhaseWorkloadEntered)
	phases = append(phases, dockerlab.PhaseWorkloadObserved)
	phases = append(phases, dockerlab.PhaseWorkloadReturned)
	// Only add terminal_observed if terminal=true
	if terminal {
		phases = append(phases, dockerlab.PhaseTerminalObserved)
	}
	// Only add container_removed if containerRemoved=true
	if containerRemoved {
		phases = append(phases, dockerlab.PhaseContainerRemoved)
	}
	// Only add network_removed if networkRemoved=true
	if networkRemoved {
		phases = append(phases, dockerlab.PhaseNetworkRemoved)
	}
	phases = append(phases, dockerlab.PhaseLifecycleReturned)

	return &dockerlab.QualifiedLifecycleOutcome{
		ContainerID:      obs.Container.ID,
		ImageID:          obs.Image.InspectedBeforeCreate,
		NetworkID:        obs.Network.InspectResponseID,
		Started:          true,
		Terminal:         terminal,
		ContainerRemoved: containerRemoved,
		NetworkRemoved:   networkRemoved,
		StartedByRuntime: true,
		Observations:     dockerlab.CloneQualifiedExecutionObservations(obs),
		Phases:           phases,
	}
}
