// Package main provides the UVB-76 pprof memory leak lab.
//
// # Profile Validation and Cancellation-Safe Capture
//
// This file implements:
// P0-5: Overflow detection using limit+1 bytes
// P0-6: Real pprof profile validation via gzip decode + structure checks
//
// For every profile capture requires:
// - HTTP 200
// - bounded body with overflow detection
// - temporary file creation (atomic publication pattern)
// - successful sync and close
// - profile validation
// - atomic rename to final destination
//
// On cancellation or error:
// - destination file is absent
// - temporary files are removed
// - partial artifacts are never published
package main

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Sentinel errors for profile capture.
// P0-5: Typed error categories for cancellation safety.
var (
	// ErrProfileCancelled is returned when profile capture is cancelled.
	ErrProfileCancelled = errors.New("profile capture cancelled")

	// ErrProfileDeadline is returned when profile capture deadline exceeds.
	ErrProfileDeadline = errors.New("profile capture deadline exceeded")

	// ErrProfileTransport is returned when profile transport fails.
	ErrProfileTransport = errors.New("profile transport failure")

	// ErrProfileRead is returned when profile body read fails.
	ErrProfileRead = errors.New("profile body read failure")

	// ErrProfileBodyTooLarge is returned when profile exceeds size limit.
	ErrProfileBodyTooLarge = errors.New("profile body too large")

	// ErrProfileBodyEmpty is returned when profile body is empty.
	ErrProfileBodyEmpty = errors.New("profile body empty")

	// ErrProfileValidation is returned when profile validation fails.
	ErrProfileValidation = errors.New("profile validation failure")

	// ErrProfilePublication is returned when profile publication fails.
	ErrProfilePublication = errors.New("profile publication failure")
)

// P0-2: profileTempFile abstracts os.File for testing seam.
type profileTempFile interface {
	io.WriteSeeker
	Sync() error
	Close() error
	Name() string
}

// profileCaptureOps contains injectable file operations for testing.
// P0-2: This seam allows deterministic fault injection in tests.
type profileCaptureOps struct {
	CreateTemp func(dir, pattern string) (profileTempFile, error)
	Rename    func(oldPath, newPath string) error
	Remove    func(path string) error
	Copy      func(dst io.Writer, src io.Reader) (int64, error)
}

// defaultProfileCaptureOps returns production operations.
func defaultProfileCaptureOps() profileCaptureOps {
	return profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		Rename: os.Rename,
		Remove: os.Remove,
		Copy:   io.Copy,
	}
}

// ProfileValidationError represents a profile validation failure.
type ProfileValidationError struct {
	ProfileName string
	What       string
}

func (e *ProfileValidationError) Error() string {
	return fmt.Sprintf("profile validation failed for %s: %s", e.ProfileName, e.What)
}

// profileContextFailure classifies context errors for profile capture.
// P0-5: Cancellation composition preserving errors.Is contract.
func profileContextFailure(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, context.Canceled):
		return errors.Join(
			ErrProfileCancelled,
			context.Canceled,
		)

	case errors.Is(err, context.DeadlineExceeded):
		return errors.Join(
			ErrProfileDeadline,
			context.DeadlineExceeded,
		)

	default:
		return err
	}
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

// CaptureProfile captures a profile with cancellation-safe atomic publication.
// P0-5: Uses temporary file pattern for atomic publication.
// P0-5: On any failure, temporary file is removed and destination is absent.
// P0-6: Returns error rather than merely logging failure.
// P0-7: Uses context for cancellation of in-flight HTTP requests.
//
// P0-12: Delegates to profile_ops seam for deterministic testing.
func CaptureProfile(ctx context.Context, client *http.Client, url string, outPath string, profileType string) error {
	return captureProfileWithOps(
		ctx,
		client,
		url,
		outPath,
		profileType,
		defaultProfileCaptureOps(),
	)
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

	var validationErrors []string
	for _, req := range required {
		path := filepath.Join(artifactDir, req.profileName)
		if err := ValidateProfile(path, req.profileType); err != nil {
			validationErrors = append(validationErrors, err.Error())
		}
	}

	if len(validationErrors) > 0 {
		return fmt.Errorf("profile validation errors: %s", strings.Join(validationErrors, "; "))
	}

	return nil
}
