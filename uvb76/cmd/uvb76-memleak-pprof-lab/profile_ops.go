// Package main provides the UVB-76 pprof memory leak lab.
//
// # Profile Operation Seam
//
// P0-2: Profile capture operations are injected via profileCaptureOps.
// This enables deterministic failure injection in tests while defaulting
// to production operations (os.CreateTemp, os.Rename, os.Remove, io.Copy).
//
// The seam ensures:
// - Every operation can be replaced for testing
// - Production callers cannot select test behavior
// - Nil operation functions cause immediate failure (fail-closed)
package main

import (
	"errors"
	"io"
	"os"
)

// P0-2: Sentinel errors for deterministic injection.
var (
	// errInjectedRead is returned when body copy read fails.
	errInjectedRead = errors.New("injected profile body read failure")

	// errCreateTemp is returned when temp file creation fails.
	errCreateTemp = errors.New("injected create temp failure")

	// errSync is returned when file sync fails.
	errSync = errors.New("injected sync failure")

	// errClose is returned when file close fails.
	errClose = errors.New("injected close failure")

	// errRename is returned when rename fails.
	errRename = errors.New("injected rename failure")

	// errRemove is returned when temp file removal fails.
	errRemove = errors.New("injected remove failure")
)

// P0-2: profileTempFile is the interface for temporary profile files.
type profileTempFile interface {
	io.Writer
	Name() string
	Sync() error
	Close() error
}

// P0-2: profileCaptureOps contains injectable operations for profile capture.
type profileCaptureOps struct {
	// CreateTemp creates a temporary file.
	CreateTemp func(dir, pattern string) (profileTempFile, error)

	// Rename renames a file.
	Rename func(oldPath, newPath string) error

	// Remove removes a file.
	Remove func(path string) error

	// Copy copies data from reader to writer.
	Copy func(dst io.Writer, src io.Reader) (int64, error)
}

// P0-2: defaultProfileCaptureOps returns production operations.
func defaultProfileCaptureOps() profileCaptureOps {
	return profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			f, err := os.CreateTemp(dir, pattern)
			if err != nil {
				return nil, err
			}
			return f, nil
		},
		Rename: os.Rename,
		Remove: os.Remove,
		Copy:   io.Copy,
	}
}

// P0-2: validateProfileOps checks that all operations are non-nil.
func validateProfileOps(ops profileCaptureOps) error {
	if ops.CreateTemp == nil {
		return errors.New("CreateTemp is required")
	}
	if ops.Rename == nil {
		return errors.New("Rename is required")
	}
	if ops.Remove == nil {
		return errors.New("Remove is required")
	}
	if ops.Copy == nil {
		return errors.New("Copy is required")
	}
	return nil
}

// P0-2: cleanupProfileTemp is the centralized cleanup authority.
// It closes the file (if still open) and removes the temp path.
// P0-5: All cleanup failures are joined and returned.
func cleanupProfileTemp(ops profileCaptureOps, file profileTempFile, path string) error {
	var errs []error

	// Close file if non-nil
	if file != nil {
		if closeErr := file.Close(); closeErr != nil {
			errs = append(errs, closeErr)
		}
	}

	// Remove temp file
	if removeErr := ops.Remove(path); removeErr != nil {
		errs = append(errs, removeErr)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// P0-2: fakeTempFile is a test fake implementing profileTempFile.
type fakeTempFile struct {
	name     string
	content  []byte
	closed   bool
	syncErr  error
	closeErr error
}

// Name returns the temp file name.
func (f *fakeTempFile) Name() string {
	return f.name
}

// Write writes data to the temp file.
func (f *fakeTempFile) Write(p []byte) (int, error) {
	f.content = append(f.content, p...)
	return len(p), nil
}

// Sync syncs the temp file.
func (f *fakeTempFile) Sync() error {
	return f.syncErr
}

// Close closes the temp file.
func (f *fakeTempFile) Close() error {
	f.closed = true
	return f.closeErr
}

// P0-2: fakeProfileOps returns an ops set for testing with injected failures.
func fakeProfileOps() profileCaptureOps {
	return profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			return nil, errCreateTemp
		},
		Rename: func(oldPath, newPath string) error {
			return errRename
		},
		Remove: func(path string) error {
			return errRemove
		},
		Copy: func(dst io.Writer, src io.Reader) (int64, error) {
			return 0, errInjectedRead
		},
	}
}
