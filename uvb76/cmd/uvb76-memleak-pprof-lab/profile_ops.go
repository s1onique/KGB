// Package main provides the UVB-76 pprof memory leak lab.
//
// # Profile Capture Operations Seam
//
// P0-2: This file provides the operation seam for deterministic testing.
// The production CaptureProfile uses this seam internally.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// validateProfileOps validates that required operations are set.
func validateProfileOps(ops profileCaptureOps) error {
	if ops.CreateTemp == nil {
		return fmt.Errorf("CreateTemp is required")
	}
	if ops.Rename == nil {
		return fmt.Errorf("Rename is required")
	}
	if ops.Remove == nil {
		return fmt.Errorf("Remove is required")
	}
	if ops.Copy == nil {
		return fmt.Errorf("Copy is required")
	}
	return nil
}

// cleanupProfileTempPreserving removes temp file and returns cleanup error.
// P0-5: Cleanup errors are now preserved using errors.Join.
func cleanupProfileTempPreserving(ops profileCaptureOps, tmp profileTempFile, tmpPath string) error {
	var cleanupErrors []error

	if tmp != nil {
		if err := tmp.Close(); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("close temp profile: %w", err))
		}
	}

	if tmpPath != "" {
		if err := ops.Remove(tmpPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove temp profile: %w", err))
		}
	}

	return errors.Join(cleanupErrors...)
}

// captureProfileWithOps captures a profile using the provided operations.
// P0-2: This is the internal entry point that accepts injected operations.
// This allows deterministic fault injection in tests.
func captureProfileWithOps(
	ctx context.Context,
	client *http.Client,
	url string,
	outPath string,
	profileType string,
	ops profileCaptureOps,
) error {
	// P0-2: Fail-closed on nil operations
	if err := validateProfileOps(ops); err != nil {
		return err
	}

	// Track cleanup errors
	var cleanupErr error

	// Create request with context for cancellation support
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return profileContextFailure(fmt.Errorf("create request for %s: %w", url, err))
	}

	// Request can be cancelled by context
	resp, err := client.Do(req)
	if err != nil {
		return errors.Join(
			ErrProfileTransport,
			profileContextFailure(fmt.Errorf("fetch %s: %w", url, err)),
		)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: status %d", url, resp.StatusCode)
	}

	// Create temporary file in same directory as final destination
	tmpDir := filepath.Dir(outPath)
	tmp, err := ops.CreateTemp(tmpDir, filepath.Base(outPath)+".tmp.*")
	if err != nil {
		return errors.Join(
			ErrProfilePublication,
			fmt.Errorf("create temp file: %w", err),
		)
	}
	tmpPath := tmp.Name()

	// Use limit+1 to detect overflow
	const limit = 100 * 1024 * 1024 // 100MB max
	limitReader := io.LimitReader(resp.Body, limit+1)

	// Copy with injected operation
	written, err := ops.Copy(tmp, limitReader)
	if err != nil {
		// Preserve both copy error and cleanup error
		cleanupErr = cleanupProfileTempPreserving(ops, tmp, tmpPath)
		return errors.Join(
			ErrProfileRead,
			profileContextFailure(fmt.Errorf("write %s: %w", tmpPath, err)),
			cleanupErr,
		)
	}

	// Check for overflow
	if written > limit {
		cleanupErr = cleanupProfileTempPreserving(ops, tmp, tmpPath)
		return errors.Join(
			ErrProfileBodyTooLarge,
			fmt.Errorf("profile %s exceeds size limit (%d bytes)", outPath, limit),
			cleanupErr,
		)
	}

	// Sync to disk
	if err := tmp.Sync(); err != nil {
		cleanupErr = cleanupProfileTempPreserving(ops, tmp, tmpPath)
		return errors.Join(
			ErrProfilePublication,
			fmt.Errorf("sync temp file: %w", err),
			cleanupErr,
		)
	}

	// Close temp file
	if err := tmp.Close(); err != nil {
		cleanupErr = cleanupProfileTempPreserving(ops, tmp, tmpPath)
		return errors.Join(
			ErrProfilePublication,
			fmt.Errorf("close temp file: %w", err),
			cleanupErr,
		)
	}

	if written == 0 {
		cleanupErr = cleanupProfileTempPreserving(ops, tmp, tmpPath)
		return errors.Join(
			ErrProfileBodyEmpty,
			fmt.Errorf("profile %s is empty", outPath),
			cleanupErr,
		)
	}

	// Validate the captured profile BEFORE renaming
	if err := ValidateProfile(tmpPath, profileType); err != nil {
		cleanupErr = cleanupProfileTempPreserving(ops, tmp, tmpPath)
		return errors.Join(
			ErrProfileValidation,
			fmt.Errorf("validation failed for %s: %w", tmpPath, err),
			cleanupErr,
		)
	}

	// Atomic rename
	if err := ops.Rename(tmpPath, outPath); err != nil {
		cleanupErr = cleanupProfileTempPreserving(ops, tmp, tmpPath)
		return errors.Join(
			ErrProfilePublication,
			fmt.Errorf("rename %s to %s: %w", tmpPath, outPath, err),
			cleanupErr,
		)
	}

	return nil
}
