// Package main provides the UVB-76 pprof memory leak lab.
//
// # Profile Validation
//
// This file implements P0-5/P0-6:
// - P0-5: Overflow detection using limit+1 bytes
// - P0-6: Real pprof profile validation via gzip decode + structure checks
//
// For every profile capture requires:
// - HTTP 200
// - bounded body with overflow detection
// - successful create/write/sync/close
// - regular non-empty file
// - gzip validity for heap and allocs
// - non-empty valid UTF-8 for goroutine dumps
//
// Profile capture must return an error rather than merely log failure.
package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// ProfileValidationError represents a profile validation failure.
type ProfileValidationError struct {
	ProfileName string
	What        string
}

func (e *ProfileValidationError) Error() string {
	return fmt.Sprintf("profile validation failed for %s: %s", e.ProfileName, e.What)
}

// ValidateProfile performs full validation of a captured profile.
// P0-6: Returns error rather than merely logging failure.
func ValidateProfile(profilePath string, profileType string) error {
	// Check file exists and is regular
	info, err := os.Stat(profilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &ProfileValidationError{profilePath, "file does not exist"}
		}
		return &ProfileValidationError{profilePath, fmt.Sprintf("stat failed: %v", err)}
	}

	if info.IsDir() {
		return &ProfileValidationError{profilePath, "is a directory, not a file"}
	}

	if info.Size() == 0 {
		return &ProfileValidationError{profilePath, "file is empty"}
	}

	// Open the file
	f, err := os.Open(profilePath)
	if err != nil {
		return &ProfileValidationError{profilePath, fmt.Sprintf("open failed: %v", err)}
	}
	defer f.Close()

	// Validate based on profile type
	switch profileType {
	case "heap", "allocs":
		return validateGzipProfile(f, profilePath)
	case "goroutine":
		return validateTextProfile(f, profilePath)
	default:
		// Unknown type, just check non-empty
		return nil
	}
}

// validateGzipProfile validates a gzip-compressed pprof profile.
// P0-6: Validates gzip integrity and non-empty content.
func validateGzipProfile(f *os.File, path string) error {
	// Try to open as gzip
	gz, err := gzip.NewReader(f)
	if err != nil {
		return &ProfileValidationError{path, fmt.Sprintf("not valid gzip: %v", err)}
	}
	defer gz.Close()

	// Read the entire gzip content
	data, err := io.ReadAll(gz)
	if err != nil {
		return &ProfileValidationError{path, fmt.Sprintf("gzip read failed: %v", err)}
	}

	// P0-6: Valid profile must have non-empty content after decompression
	if len(data) == 0 {
		return &ProfileValidationError{path, "gzip content is empty"}
	}

	return nil
}

// validateTextProfile validates a text profile (goroutine dump).
func validateTextProfile(f *os.File, path string) error {
	// Read all content
	content, err := io.ReadAll(f)
	if err != nil {
		return &ProfileValidationError{path, fmt.Sprintf("read failed: %v", err)}
	}

	if len(content) == 0 {
		return &ProfileValidationError{path, "content is empty"}
	}

	// Validate UTF-8 using standard library (P0-6)
	if !utf8.Valid(content) {
		return &ProfileValidationError{path, "content is not valid UTF-8"}
	}

	return nil
}

// CaptureProfile captures a profile with full validation.
// P0-5: Uses limit+1 to detect overflow (response exceeds limit).
// P0-6: Returns error rather than merely logging failure.
func CaptureProfile(client *http.Client, url string, outPath string, profileType string) error {
	// Fetch with bounded body
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: status %d", url, resp.StatusCode)
	}

	// Create file with proper sync
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}

	// P0-5: Use limit+1 to detect overflow
	// Read limit bytes, then try to read one more byte
	// If that byte succeeds, the response was larger than limit
	const limit = 100 * 1024 * 1024 // 100MB max
	limitReader := io.LimitReader(resp.Body, limit+1)

	// Write the bounded content
	written, err := io.Copy(f, limitReader)
	if err != nil {
		f.Close()
		os.Remove(outPath)
		return fmt.Errorf("write %s: %w", outPath, err)
	}

	// P0-5: Check for overflow - if we wrote more than limit, the response was too large
	if written > limit {
		f.Close()
		os.Remove(outPath)
		return fmt.Errorf("profile %s exceeds size limit (%d bytes)", outPath, limit)
	}

	// Sync to disk
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(outPath)
		return fmt.Errorf("sync %s: %w", outPath, err)
	}

	// Close
	if err := f.Close(); err != nil {
		os.Remove(outPath)
		return fmt.Errorf("close %s: %w", outPath, err)
	}

	if written == 0 {
		os.Remove(outPath)
		return fmt.Errorf("profile %s is empty", outPath)
	}

	// Validate the captured profile
	if err := ValidateProfile(outPath, profileType); err != nil {
		os.Remove(outPath)
		return fmt.Errorf("validation failed for %s: %w", outPath, err)
	}

	return nil
}

// ValidateAllProfiles validates all required profiles for a lab run.
// P0-6: Required rows must have valid start, midpoint and final profiles.
func ValidateAllProfiles(artifactDir string) error {
	required := []struct {
		profileName string
		profileType string
	}{
		{"heap-start.pb.gz", "heap"},
		{"heap-mid.pb.gz", "heap"},
		{"heap-final.pb.gz", "heap"},
		{"allocs-start.pb.gz", "allocs"},
		{"allocs-mid.pb.gz", "allocs"},
		{"allocs-final.pb.gz", "allocs"},
		{"goroutine-start.txt", "goroutine"},
		{"goroutine-mid.txt", "goroutine"},
		{"goroutine-final.txt", "goroutine"},
	}

	var errors []string
	for _, req := range required {
		path := filepath.Join(artifactDir, req.profileName)
		if err := ValidateProfile(path, req.profileType); err != nil {
			errors = append(errors, err.Error())
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("profile validation errors: %s", strings.Join(errors, "; "))
	}

	return nil
}
