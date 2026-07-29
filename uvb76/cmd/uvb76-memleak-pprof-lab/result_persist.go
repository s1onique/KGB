// Package main provides the UVB-76 pprof memory leak lab.
//
// # Result Persistence
//
// This file implements P0-7: Persist result.json with atomic publication.
// Atomically publishes result.json after cleanup and final classification.
// Strictly rereads and decodes the physical file before returning success.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Sentinel errors for result file operations.
// P0-2: Result-removal-specific sentinels, distinct from collector lifecycle sentinels.

// ErrNilResultFileDependency is returned when result file ops receive a nil dependency.
var ErrNilResultFileDependency = errors.New("nil result file dependency")

// ErrResultStillPresent is returned when a result file cannot be removed.
var ErrResultStillPresent = errors.New("result file still present after removal attempt")

// ErrResultAbsenceUnproven is returned when physical absence cannot be proven.
var ErrResultAbsenceUnproven = errors.New("result file absence unproven")

// Sentinel errors for result construction failures (P0-7).
// Note: Common validation errors (ErrNilIdentity, ErrEmptyRunID, etc.) are defined
// in lifecycle_failure_finalize.go to avoid duplication.
var (
	// ErrMissingTovarischPID is returned when tovarisch PID is missing.
	ErrMissingTovarischPID = errors.New("missing tovarisch PID")

	// ErrMissingUVB76PID is returned when uvb76 PID is missing.
	ErrMissingUVB76PID = errors.New("missing uvb76 PID")

	// ErrMissingTovarischStartTime is returned when tovarisch start time is missing.
	ErrMissingTovarischStartTime = errors.New("missing tovarisch start time")

	// ErrMissingUVB76StartTime is returned when uvb76 start time is missing.
	ErrMissingUVB76StartTime = errors.New("missing uvb76 start time")

	// ErrMissingTovarischArgv is returned when tovarisch argv is missing.
	ErrMissingTovarischArgv = errors.New("missing tovarisch argv")

	// ErrMissingUVB76Argv is returned when uvb76 argv is missing.
	ErrMissingUVB76Argv = errors.New("missing uvb76 argv")

	// ErrMissingCollectionStart is returned when collection start time is missing.
	ErrMissingCollectionStart = errors.New("missing collection start time")
)

// resultFileOps defines the operations needed for result file removal.
// P0-1: Provides a seam for deterministic testing without host permissions.
type resultFileOps struct {
	Remove func(string) error
	Lstat  func(string) (os.FileInfo, error)
}

// productionResultFileOps returns the production operations for result file removal.
// P0-1: Single authority for result file removal.
func productionResultFileOps() resultFileOps {
	return resultFileOps{
		Remove: os.Remove,
		Lstat:  os.Lstat,
	}
}

// removeResultFile attempts to remove a result file and returns an error
// only if the file still exists after the removal attempt.
// P0-1: Thin production wrapper - delegates to removeResultFileWithOps.
func removeResultFile(path string) error {
	return removeResultFileWithOps(path, productionResultFileOps())
}

// removeResultFileWithOps removes a result file using the provided operations.
// P0-1: Uses Lstat to avoid following symlinks; preserves all causes.
// P0-2: Validates dependencies before side effects.
func removeResultFileWithOps(path string, ops resultFileOps) error {
	// P0-2: Validate dependencies before any side effects
	if ops.Remove == nil {
		return fmt.Errorf("%w: Remove", ErrNilResultFileDependency)
	}
	if ops.Lstat == nil {
		return fmt.Errorf("%w: Lstat", ErrNilResultFileDependency)
	}

	// P0-3: Physical removal matrix - call Remove first, then Lstat to verify
	removeErr := ops.Remove(path)
	_, statErr := ops.Lstat(path)

	// P0-3: Decision based on physical state after removal attempt
	switch {
	case errors.Is(statErr, os.ErrNotExist):
		// Case 1 & 2: Lstat returns ErrNotExist → proven absent
		// Return nil regardless of removeErr - physical absence outranks operation error
		return nil
	case statErr == nil:
		// Case 3 & 4: File still exists after removal attempt
		if removeErr != nil {
			return errors.Join(ErrResultStillPresent, removeErr)
		}
		return errors.Join(ErrResultStillPresent, errors.New("file still exists after removal"))
	default:
		// Case 5 & 6: Cannot prove absence - path resolution, I/O, or permission error
		// P0-6: Preserve both ErrResultAbsenceUnproven and exact statErr through errors.Join
		if removeErr != nil {
			return errors.Join(ErrResultAbsenceUnproven, statErr, removeErr)
		}
		return errors.Join(ErrResultAbsenceUnproven, statErr)
	}
}

// ResultSchemaVersion is the current schema version for result.json
const ResultSchemaVersion = 1

// Result represents the durable lab result with full schema.
type Result struct {
	SchemaVersion int    `json:"schema_version"`
	RunID         string `json:"run_id"`
	RowID         string `json:"row_id,omitempty"`
	SourceCommit  string `json:"source_commit"`
	RunStartedAt  string `json:"run_started_at"`

	// Process identities
	TovarischIdentity *ProcessIdentity `json:"tovarisch_identity,omitempty"`
	UVB76Identity     *ProcessIdentity `json:"uvb76_identity,omitempty"`

	// Selected ports
	TovarischPort string `json:"tovarisch_port"`
	UVB76Port     string `json:"uvb76_port"`
	PProfPort     string `json:"pprof_port"`

	// Readiness timestamps
	TovarischStartTime  *time.Time `json:"tovarisch_start_time,omitempty"`
	TovarischReadyTime  *time.Time `json:"tovarisch_ready_time,omitempty"`
	UVB76StartTime      *time.Time `json:"uvb76_start_time,omitempty"`
	UVB76PProfReadyTime *time.Time `json:"uvb76_pprof_ready_time,omitempty"`
	CollectionStartTime *time.Time `json:"collection_start_time,omitempty"`
	CollectionEndTime   *time.Time `json:"collection_end_time,omitempty"`

	// Scrape authority observations
	ScrapeAuthority *ScrapeAuthorityObservation `json:"scrape_authority,omitempty"`

	// Process series file names
	TovarischSeriesFile string `json:"tovarisch_series_file,omitempty"`
	UVB76SeriesFile     string `json:"uvb76_series_file,omitempty"`

	// Sample counts
	TovarischSampleCount int `json:"tovarisch_sample_count"`
	UVB76SampleCount     int `json:"uvb76_sample_count"`

	// Profile file names and sizes
	Profiles []ProfileInfo `json:"profiles,omitempty"`

	// Cleanup outcomes
	CleanupSuccess   bool     `json:"cleanup_success"`
	CleanupErrors    []string `json:"cleanup_errors,omitempty"`
	UVB76Removed     bool     `json:"uvb76_removed"`
	TovarischRemoved bool     `json:"tovarisch_removed"`
	PortsReleased    bool     `json:"ports_released"`

	// Classification
	Classification string `json:"classification"`
	OK             bool   `json:"ok"`

	// Errors
	Errors []string `json:"errors,omitempty"`
}

// ScrapeAuthorityObservation records the authoritative scrape observations.
type ScrapeAuthorityObservation struct {
	TargetID            string     `json:"target_id"`
	AttemptObserved     bool       `json:"attempt_observed"`
	AttemptTimestamp    *time.Time `json:"attempt_timestamp,omitempty"`
	CompletionObserved  bool       `json:"completion_observed"`
	CompletionTimestamp *time.Time `json:"completion_timestamp,omitempty"`
	Reachable           bool       `json:"reachable"`
	Status              string     `json:"status,omitempty"`
	Error               string     `json:"error,omitempty"`
}

// ProfileInfo records metadata about a captured profile.
type ProfileInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// persistResult atomically publishes result.json after cleanup.
// P0-7: Strictly reread and decode the physical file before returning success.
// P0-8: If verification fails, remove the invalid final file and verify removal.
// Requires exact-one JSON document (no trailing data).
func persistResult(result *Result, artifactDir string) error {
	resultFile := filepath.Join(artifactDir, "result.json")

	// Marshal to JSON
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}

	// Write to temp file first (atomic publish)
	tmpFile := resultFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("write temp result: %w", err)
	}

	// Sync to disk
	f, err := os.Open(tmpFile)
	if err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("open temp result for sync: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpFile)
		return fmt.Errorf("sync temp result: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("close temp result: %w", err)
	}

	// Rename to final location (atomic)
	if err := os.Rename(tmpFile, resultFile); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("rename result: %w", err)
	}

	// P0-7: Reread and decode to verify exact-one JSON
	reread, err := os.Open(resultFile)
	if err != nil {
		// P0-8: Reread open failure - try to remove anyway and report both
		if rmErr := removeResultFile(resultFile); rmErr != nil {
			return errors.Join(fmt.Errorf("reread failed: %w", err), rmErr)
		}
		return fmt.Errorf("reread result: %w; file removed", err)
	}
	defer reread.Close()

	var verified Result
	decoder := json.NewDecoder(reread)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&verified); err != nil {
		// P0-8: Remove invalid final file on decode failure
		if rmErr := removeResultFile(resultFile); rmErr != nil {
			return errors.Join(fmt.Errorf("decode failed: %w", err), rmErr)
		}
		return fmt.Errorf("decode verified result: %w; file removed", err)
	}

	// P0-7: Require exactly one JSON document - must read EOF after first document
	var trailingCheck map[string]interface{}
	if err := decoder.Decode(&trailingCheck); err != io.EOF {
		// P0-8: Remove invalid final file on trailing data
		if rmErr := removeResultFile(resultFile); rmErr != nil {
			return errors.Join(fmt.Errorf("trailing data: %w", err), rmErr)
		}
		return fmt.Errorf("expected EOF after result: %w; file removed", err)
	}

	// P0-7: Verify complete result equality
	if err := verifyResultEquality(&verified, result); err != nil {
		// P0-8: Remove invalid final file on equality failure
		if rmErr := removeResultFile(resultFile); rmErr != nil {
			return errors.Join(fmt.Errorf("equality failed: %w", err), rmErr)
		}
		return fmt.Errorf("result equality check failed: %w; file removed", err)
	}

	return nil
}

// verifyResultEquality checks that the physical result matches the supplied result exactly.
// P0-7: Compares all fields exhaustively, including timestamps, profiles, and slice contents.
func verifyResultEquality(verified, original *Result) error {
	// Schema version
	if verified.SchemaVersion != original.SchemaVersion {
		return fmt.Errorf("schema_version mismatch: got %d, want %d", verified.SchemaVersion, original.SchemaVersion)
	}
	// Run ID
	if verified.RunID != original.RunID {
		return fmt.Errorf("run_id mismatch: got %q, want %q", verified.RunID, original.RunID)
	}
	// Classification
	if verified.Classification != original.Classification {
		return fmt.Errorf("classification mismatch: got %q, want %q", verified.Classification, original.Classification)
	}
	// Row ID
	if verified.RowID != original.RowID {
		return fmt.Errorf("row_id mismatch: got %q, want %q", verified.RowID, original.RowID)
	}
	// Source commit
	if verified.SourceCommit != original.SourceCommit {
		return fmt.Errorf("source_commit mismatch: got %q, want %q", verified.SourceCommit, original.SourceCommit)
	}
	// RunStartedAt
	if verified.RunStartedAt != original.RunStartedAt {
		return fmt.Errorf("run_started_at mismatch: got %q, want %q", verified.RunStartedAt, original.RunStartedAt)
	}
	// OK flag
	if verified.OK != original.OK {
		return fmt.Errorf("ok mismatch: got %v, want %v", verified.OK, original.OK)
	}
	// Ports
	if verified.TovarischPort != original.TovarischPort {
		return fmt.Errorf("tovarisch_port mismatch: got %q, want %q", verified.TovarischPort, original.TovarischPort)
	}
	if verified.UVB76Port != original.UVB76Port {
		return fmt.Errorf("uvb76_port mismatch: got %q, want %q", verified.UVB76Port, original.UVB76Port)
	}
	if verified.PProfPort != original.PProfPort {
		return fmt.Errorf("pprof_port mismatch: got %q, want %q", verified.PProfPort, original.PProfPort)
	}

	// Timestamps - compare all timestamps exhaustively
	if !compareTimePtr(verified.TovarischStartTime, original.TovarischStartTime) {
		return fmt.Errorf("tovarisch_start_time mismatch")
	}
	if !compareTimePtr(verified.TovarischReadyTime, original.TovarischReadyTime) {
		return fmt.Errorf("tovarisch_ready_time mismatch")
	}
	if !compareTimePtr(verified.UVB76StartTime, original.UVB76StartTime) {
		return fmt.Errorf("uvb76_start_time mismatch")
	}
	if !compareTimePtr(verified.UVB76PProfReadyTime, original.UVB76PProfReadyTime) {
		return fmt.Errorf("uvb76_pprof_ready_time mismatch")
	}
	if !compareTimePtr(verified.CollectionStartTime, original.CollectionStartTime) {
		return fmt.Errorf("collection_start_time mismatch")
	}
	if !compareTimePtr(verified.CollectionEndTime, original.CollectionEndTime) {
		return fmt.Errorf("collection_end_time mismatch")
	}

	// Process identities
	if !compareProcessIdentity(verified.TovarischIdentity, original.TovarischIdentity) {
		return fmt.Errorf("tovarisch_identity mismatch")
	}
	if !compareProcessIdentity(verified.UVB76Identity, original.UVB76Identity) {
		return fmt.Errorf("uvb76_identity mismatch")
	}

	// Cleanup outcomes
	if verified.CleanupSuccess != original.CleanupSuccess {
		return fmt.Errorf("cleanup_success mismatch: got %v, want %v", verified.CleanupSuccess, original.CleanupSuccess)
	}
	if verified.UVB76Removed != original.UVB76Removed {
		return fmt.Errorf("uvb76_removed mismatch: got %v, want %v", verified.UVB76Removed, original.UVB76Removed)
	}
	if verified.TovarischRemoved != original.TovarischRemoved {
		return fmt.Errorf("tovarisch_removed mismatch: got %v, want %v", verified.TovarischRemoved, original.TovarischRemoved)
	}
	if verified.PortsReleased != original.PortsReleased {
		return fmt.Errorf("ports_released mismatch: got %v, want %v", verified.PortsReleased, original.PortsReleased)
	}

	// P0-7: Compare error slice contents (not just lengths)
	if !compareStringSlices(verified.CleanupErrors, original.CleanupErrors) {
		return fmt.Errorf("cleanup_errors content mismatch")
	}
	if !compareStringSlices(verified.Errors, original.Errors) {
		return fmt.Errorf("errors content mismatch")
	}

	// Scrape authority (including timestamps)
	if !compareScrapeAuthority(verified.ScrapeAuthority, original.ScrapeAuthority) {
		return fmt.Errorf("scrape_authority mismatch")
	}

	// Series files
	if verified.TovarischSeriesFile != original.TovarischSeriesFile {
		return fmt.Errorf("tovarisch_series_file mismatch: got %q, want %q", verified.TovarischSeriesFile, original.TovarischSeriesFile)
	}
	if verified.UVB76SeriesFile != original.UVB76SeriesFile {
		return fmt.Errorf("uvb76_series_file mismatch: got %q, want %q", verified.UVB76SeriesFile, original.UVB76SeriesFile)
	}

	// Sample counts
	if verified.TovarischSampleCount != original.TovarischSampleCount {
		return fmt.Errorf("tovarisch_sample_count mismatch: got %d, want %d", verified.TovarischSampleCount, original.TovarischSampleCount)
	}
	if verified.UVB76SampleCount != original.UVB76SampleCount {
		return fmt.Errorf("uvb76_sample_count mismatch: got %d, want %d", verified.UVB76SampleCount, original.UVB76SampleCount)
	}

	// Profile inventory
	if !compareProfiles(verified.Profiles, original.Profiles) {
		return fmt.Errorf("profiles mismatch")
	}

	return nil
}

// compareTimePtr compares two time pointers.
func compareTimePtr(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

// compareStringSlices compares two string slices for exact equality.
func compareStringSlices(a, b []string) bool {
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

// compareProfiles compares profile inventories.
func compareProfiles(a, b []ProfileInfo) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || a[i].Path != b[i].Path || a[i].Size != b[i].Size {
			return false
		}
	}
	return true
}

// compareProcessIdentity compares two process identities.
// P0-6: Compares ALL fields including StartTime for physical verification.
func compareProcessIdentity(a, b *ProcessIdentity) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// P0-6: Compare all fields including StartTime
	if a.ExecutablePath != b.ExecutablePath ||
		a.PID != b.PID ||
		a.Port != b.Port ||
		!a.StartTime.Equal(b.StartTime) ||
		!slices.Equal(a.Argv, b.Argv) {
		return false
	}
	return true
}

// compareScrapeAuthority compares two scrape authority observations.
func compareScrapeAuthority(a, b *ScrapeAuthorityObservation) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.TargetID != b.TargetID ||
		a.AttemptObserved != b.AttemptObserved ||
		a.CompletionObserved != b.CompletionObserved ||
		a.Reachable != b.Reachable ||
		a.Status != b.Status ||
		a.Error != b.Error {
		return false
	}
	// Compare timestamps
	if !compareTimePtr(a.AttemptTimestamp, b.AttemptTimestamp) {
		return false
	}
	if !compareTimePtr(a.CompletionTimestamp, b.CompletionTimestamp) {
		return false
	}
	return true
}

// isCleanupError determines if an error is a cleanup-specific error.
func isCleanupError(err string) bool {
	// Cleanup errors are specifically about cleanup operations
	cleanupPrefixes := []string{
		"cleanup ",
		"kill ",
		"remove ",
		"port release",
		"sync after cleanup",
		"process exit after kill",
	}
	for _, prefix := range cleanupPrefixes {
		if len(err) >= len(prefix) && err[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// BuildResultFromLabResult constructs a Result from the internal LabResult.
// P0-7: Returns error for incomplete or invalid observations.
// P0-1: Accepts *runExecutionIdentity for complete identity consumption.
// P0-2: Uses identity fields directly - no time.Now, no flag package globals.
// P0-3: Populates ALL ProcessIdentity fields including Argv and StartTime.
func BuildResultFromLabResult(identity *runExecutionIdentity, lab LabResult) (*Result, error) {
	// P0-7: Validate required inputs
	if identity == nil {
		return nil, ErrNilIdentity
	}
	if identity.RunID == "" {
		return nil, ErrEmptyRunID
	}
	if identity.SourceCommit == "" {
		return nil, ErrEmptySourceCommit
	}
	if identity.TovarischPort == "" {
		return nil, ErrEmptyTovarischPort
	}
	if identity.UVB76Port == "" {
		return nil, ErrEmptyUVB76Port
	}

	// P0-7: Accumulate validation errors
	var validationErrors []error

	// P0-9: Separate cleanup errors from general errors
	var cleanupErrors []string
	var executionErrors []string
	for _, err := range lab.Errors {
		if isCleanupError(err) {
			cleanupErrors = append(cleanupErrors, err)
		} else {
			executionErrors = append(executionErrors, err)
		}
	}

	r := &Result{
		SchemaVersion:       ResultSchemaVersion,
		RunID:               identity.RunID,
		SourceCommit:        identity.SourceCommit,
		RunStartedAt:        identity.RunStartedAt.Format(time.RFC3339),
		TovarischPort:       identity.TovarischPort,
		UVB76Port:           identity.UVB76Port,
		PProfPort:           identity.PProfPort,
		TovarischStartTime:  lab.TovarischStartTime,
		TovarischReadyTime:  lab.TovarischReadyTime,
		UVB76StartTime:      lab.UVB76StartTime,
		UVB76PProfReadyTime: lab.UVB76PProfReadyTime,
		CollectionStartTime: lab.CollectionStartTime,
		CollectionEndTime:   lab.CollectionEndTime,
		// P0-9: Cleanup success is based only on cleanup errors, not all errors
		CleanupSuccess:   len(cleanupErrors) == 0,
		CleanupErrors:    cleanupErrors,
		UVB76Removed:     lab.UVB76Removed,
		TovarischRemoved: lab.TovarischRemoved,
		PortsReleased:    lab.PortsReleased,
		Classification:   lab.Classification,
		OK:               lab.OK,
		// P0-9: Execution/artifact errors are separate from cleanup errors
		Errors: executionErrors,
	}

	// P0-3: Populate complete ProcessIdentity with ALL fields
	if lab.TovarischPID > 0 {
		// P0-7: Validate tovarisch process fields
		if lab.TovarischBinPath == "" {
			validationErrors = append(validationErrors, ErrEmptyTovarischBinPath)
		}
		if lab.TovarischStartTime == nil {
			validationErrors = append(validationErrors, ErrMissingTovarischStartTime)
		}
		if lab.TovarischArgv == nil {
			validationErrors = append(validationErrors, ErrMissingTovarischArgv)
		}

		r.TovarischIdentity = &ProcessIdentity{
			ExecutablePath: lab.TovarischBinPath,
			PID:            lab.TovarischPID,
			Port:           identity.TovarischPort,
		}
		// P0-3: Include StartTime if available
		if lab.TovarischStartTime != nil {
			r.TovarischIdentity.StartTime = *lab.TovarischStartTime
		}
		// P0-3: Clone argv from lab if available
		if lab.TovarischArgv != nil {
			r.TovarischIdentity.Argv = slices.Clone(lab.TovarischArgv)
		}
	} else {
		// P0-7: Real tovarisch should have been started
		validationErrors = append(validationErrors, ErrMissingTovarischPID)
	}

	if lab.UVB76PID > 0 {
		// P0-7: Validate uvb76 process fields
		if lab.UVB76BinPath == "" {
			validationErrors = append(validationErrors, ErrEmptyUVB76BinPath)
		}
		if lab.UVB76StartTime == nil {
			validationErrors = append(validationErrors, ErrMissingUVB76StartTime)
		}
		if lab.UVB76Argv == nil {
			validationErrors = append(validationErrors, ErrMissingUVB76Argv)
		}

		r.UVB76Identity = &ProcessIdentity{
			ExecutablePath: lab.UVB76BinPath,
			PID:            lab.UVB76PID,
			Port:           identity.UVB76Port,
		}
		// P0-3: Include StartTime if available
		if lab.UVB76StartTime != nil {
			r.UVB76Identity.StartTime = *lab.UVB76StartTime
		}
		// P0-3: Clone argv from lab if available
		if lab.UVB76Argv != nil {
			r.UVB76Identity.Argv = slices.Clone(lab.UVB76Argv)
		}
	} else {
		// P0-7: Real uvb76 should have been started
		validationErrors = append(validationErrors, ErrMissingUVB76PID)
	}

	// P0-7: Return all validation errors joined
	if len(validationErrors) > 0 {
		return nil, errors.Join(validationErrors...)
	}

	return r, nil
}
