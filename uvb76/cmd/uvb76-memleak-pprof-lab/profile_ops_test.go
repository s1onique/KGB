// Package main provides the UVB-76 pprof memory leak lab.
//
// # Profile Operation Tests
//
// P0-3: Deterministic body-Copy error injection tests.
// P0-4: Deterministic filesystem failure matrix tests.
// P0-5: Centralized cleanup tests.
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// captureProfileWithOps captures a profile using the provided operations.
// P0-2: This is the internal entry point that accepts injected operations.
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

	// Create request with context for cancellation support
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return profileContextFailure(err)
	}

	// Request can be cancelled by context
	resp, err := client.Do(req)
	if err != nil {
		return errors.Join(
			ErrProfileTransport,
			profileContextFailure(err),
		)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("fetch " + url + ": status " + string(rune(resp.StatusCode)))
	}

	// Create temporary file in same directory as final destination
	tmpDir := filepath.Dir(outPath)
	tmp, err := ops.CreateTemp(tmpDir, filepath.Base(outPath)+".tmp.*")
	if err != nil {
		return errors.Join(
			ErrProfilePublication,
			err,
		)
	}
	tmpPath := tmp.Name()

	// P0-5: Use centralized cleanup on any failure
	cleanup := func() {
		cleanupProfileTemp(ops, tmp, tmpPath)
	}

	// Use limit+1 to detect overflow
	const limit = 100 * 1024 * 1024 // 100MB max
	limitReader := io.LimitReader(resp.Body, limit+1)

	// Copy with injected operation
	written, err := ops.Copy(tmp, limitReader)
	if err != nil {
		cleanup()
		return errors.Join(
			ErrProfileRead,
			profileContextFailure(err),
		)
	}

	// Check for overflow
	if written > limit {
		cleanup()
		return errors.Join(ErrProfileBodyTooLarge, errors.New("profile exceeds size limit"))
	}

	// Sync to disk
	if err := tmp.Sync(); err != nil {
		cleanup()
		return errors.Join(
			ErrProfilePublication,
			err,
		)
	}

	// Close temp file
	if err := tmp.Close(); err != nil {
		cleanup()
		return errors.Join(
			ErrProfilePublication,
			err,
		)
	}

	if written == 0 {
		cleanup()
		return errors.Join(ErrProfileBodyEmpty, errors.New("profile is empty"))
	}

	// Validate the captured profile
	if err := ValidateProfile(tmpPath, profileType); err != nil {
		cleanup()
		return errors.Join(ErrProfileValidation, err)
	}

	// Atomic rename
	if err := ops.Rename(tmpPath, outPath); err != nil {
		cleanup()
		return errors.Join(ErrProfilePublication, err)
	}

	return nil
}

// TestProfileOps_NilOpsFailClosed verifies that nil operations cause immediate failure.
func TestProfileOps_NilOpsFailClosed(t *testing.T) {
	ops := profileCaptureOps{
		CreateTemp: nil,
		Rename:     nil,
		Remove:     nil,
		Copy:       nil,
	}

	err := validateProfileOps(ops)
	if err == nil {
		t.Error("Expected error for nil ops")
	}

	// Test partial nil
	ops2 := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) { return nil, nil },
		Rename:     nil,
		Remove:     nil,
		Copy:       nil,
	}

	err = validateProfileOps(ops2)
	if err == nil {
		t.Error("Expected error for partial nil ops")
	}
}

// TestProfileOps_DefaultOpsValid verifies that default ops are valid.
func TestProfileOps_DefaultOpsValid(t *testing.T) {
	ops := defaultProfileCaptureOps()
	err := validateProfileOps(ops)
	if err != nil {
		t.Errorf("Default ops should be valid: %v", err)
	}
}

// TestCaptureProfileOps_InjectedReadError verifies deterministic body read failure.
func TestCaptureProfileOps_InjectedReadError(t *testing.T) {
	// Server returns valid gzip but Copy will fail
	validGzip := createValidGzipProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		// Write data that will be partially read then fail
		w.Write(validGzip[:len(validGzip)/2])
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "profile.pb.gz")

	// Create ops with failing Copy
	ops := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		Rename: os.Rename,
		Remove: os.Remove,
		Copy: func(dst io.Writer, src io.Reader) (int64, error) {
			// Read some bytes, then fail
			buf := make([]byte, 1024)
			n, _ := src.Read(buf)
			if n > 0 {
				dst.Write(buf[:n])
			}
			return int64(n), errInjectedRead
		},
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5}, server.URL, destPath, "heap", ops)

	// P0-3: Require ErrProfileRead and errInjectedRead
	if err == nil {
		t.Fatal("Expected error")
	}
	if !errors.Is(err, ErrProfileRead) {
		t.Errorf("Expected ErrProfileRead, got: %v", err)
	}
	if !errors.Is(err, errInjectedRead) {
		t.Errorf("Expected errInjectedRead, got: %v", err)
	}

	// P0-3: Forbidden ErrProfileTransport
	if errors.Is(err, ErrProfileTransport) {
		t.Error("Should not have ErrProfileTransport for read error")
	}

	// P0-3: Destination absent
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}

	// P0-3: Temp file absent
	tempFiles, _ := filepath.Glob(filepath.Join(tmpDir, "*.tmp*"))
	if len(tempFiles) > 0 {
		t.Errorf("Temp files should be absent, found: %v", tempFiles)
	}
}

// TestCaptureProfileOps_CreateTempFailure verifies CreateTemp failure.
func TestCaptureProfileOps_CreateTempFailure(t *testing.T) {
	validGzip := createValidGzipProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(validGzip)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "profile.pb.gz")

	// Create ops with failing CreateTemp
	ops := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			return nil, errCreateTemp
		},
		Rename: os.Rename,
		Remove: os.Remove,
		Copy:   io.Copy,
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5}, server.URL, destPath, "heap", ops)

	// P0-4: Require ErrProfilePublication and errCreateTemp
	if err == nil {
		t.Fatal("Expected error")
	}
	if !errors.Is(err, ErrProfilePublication) {
		t.Errorf("Expected ErrProfilePublication, got: %v", err)
	}
	if !errors.Is(err, errCreateTemp) {
		t.Errorf("Expected errCreateTemp, got: %v", err)
	}

	// Destination unchanged
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}
}

// TestCaptureProfileOps_SyncFailure verifies Sync failure.
func TestCaptureProfileOps_SyncFailure(t *testing.T) {
	validGzip := createValidGzipProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(validGzip)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "profile.pb.gz")

	// Create ops with fake temp file that fails Sync
	ops := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			realFile, err := os.CreateTemp(dir, pattern)
			if err != nil {
				return nil, err
			}
			return &fakeTempFile{
				name:    realFile.Name(),
				content: []byte{},
				syncErr: errSync,
			}, nil
		},
		Rename: os.Rename,
		Remove: os.Remove,
		Copy:   io.Copy,
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 30 * time.Second}, server.URL, destPath, "heap", ops)

	// P0-4: Require ErrProfilePublication and errSync
	if err == nil {
		t.Fatal("Expected error")
	}
	if !errors.Is(err, ErrProfilePublication) {
		t.Errorf("Expected ErrProfilePublication, got: %v", err)
	}
	if !errors.Is(err, errSync) {
		t.Errorf("Expected errSync, got: %v", err)
	}

	// Destination unchanged
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}
}

// TestCaptureProfileOps_CloseFailure verifies Close failure.
func TestCaptureProfileOps_CloseFailure(t *testing.T) {
	validGzip := createValidGzipProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(validGzip)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "profile.pb.gz")

	// Create ops with fake temp file that fails Close
	ops := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			realFile, err := os.CreateTemp(dir, pattern)
			if err != nil {
				return nil, err
			}
			return &fakeTempFile{
				name:     realFile.Name(),
				content:  []byte{},
				closeErr: errClose,
			}, nil
		},
		Rename: os.Rename,
		Remove: os.Remove,
		Copy:   io.Copy,
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 30 * time.Second}, server.URL, destPath, "heap", ops)

	// P0-4: Require ErrProfilePublication and errClose
	if err == nil {
		t.Fatal("Expected error")
	}
	if !errors.Is(err, ErrProfilePublication) {
		t.Errorf("Expected ErrProfilePublication, got: %v", err)
	}
	if !errors.Is(err, errClose) {
		t.Errorf("Expected errClose, got: %v", err)
	}

	// Destination unchanged
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}
}

// TestCaptureProfileOps_RenameFailure verifies Rename failure.
func TestCaptureProfileOps_RenameFailure(t *testing.T) {
	validGzip := createValidGzipProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(validGzip)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "profile.pb.gz")

	// Create ops with failing Rename
	ops := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		Rename: func(oldPath, newPath string) error {
			return errRename
		},
		Remove: os.Remove,
		Copy:   io.Copy,
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5}, server.URL, destPath, "heap", ops)

	// P0-4: Require ErrProfilePublication and errRename
	if err == nil {
		t.Fatal("Expected error")
	}
	if !errors.Is(err, ErrProfilePublication) {
		t.Errorf("Expected ErrProfilePublication, got: %v", err)
	}
	if !errors.Is(err, errRename) {
		t.Errorf("Expected errRename, got: %v", err)
	}

	// Old destination unchanged (doesn't exist, so still absent)
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}
}

// TestCaptureProfileOps_CleanupFailure verifies cleanup failure handling.
func TestCaptureProfileOps_CleanupFailure(t *testing.T) {
	validGzip := createValidGzipProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(validGzip)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "profile.pb.gz")

	// Create ops with failing Remove (simulates cleanup failure)
	ops := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		Rename: func(oldPath, newPath string) error {
			return errRename // Simulate rename failure
		},
		Remove: func(path string) error {
			return errRemove // Cleanup will fail
		},
		Copy: io.Copy,
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5}, server.URL, destPath, "heap", ops)

	// P0-4: Initiating error and cleanup error both preserved
	if err == nil {
		t.Fatal("Expected error")
	}
	// errRename should be in the error
	if !errors.Is(err, errRename) {
		t.Errorf("Expected errRename, got: %v", err)
	}
	// errRemove (cleanup) should also be in the error via errors.Join
	if !errors.Is(err, errRemove) {
		t.Errorf("Expected errRemove (cleanup), got: %v", err)
	}

	// Destination absent
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}
}

// TestCaptureProfileOps_SuccessfulCapture verifies successful atomic replacement.
func TestCaptureProfileOps_SuccessfulCapture(t *testing.T) {
	validGzip := createValidGzipProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(validGzip)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "profile.pb.gz")

	// Create existing destination
	oldContent := []byte("old content")
	if err := os.WriteFile(destPath, oldContent, 0644); err != nil {
		t.Fatalf("Failed to create old profile: %v", err)
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5}, server.URL, destPath, "heap", defaultProfileCaptureOps())

	if err != nil {
		t.Fatalf("Expected success, got: %v", err)
	}

	// Verify exact new bytes
	newContent, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal("Destination should exist")
	}

	if !bytes.Equal(newContent, validGzip) {
		t.Fatalf("Destination not replaced with new content: got %d bytes, expected %d", len(newContent), len(validGzip))
	}

	// Verify old content gone
	if bytes.Equal(newContent, oldContent) {
		t.Fatal("Old content should be replaced")
	}

	// No temp files
	tempFiles, _ := filepath.Glob(filepath.Join(tmpDir, "*.tmp*"))
	if len(tempFiles) > 0 {
		t.Errorf("Temp files should be absent, found: %v", tempFiles)
	}
}

// TestCaptureProfileOps_ExplicitCancelDuringBodyRead verifies cancellation during body read.
func TestCaptureProfileOps_ExplicitCancelDuringBodyRead(t *testing.T) {
	// Server sends partial data then hangs
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()

		// Write first byte
		fmt.Fprint(w, "x")
		w.(http.Flusher).Flush()

		// Block until cancelled - but don't hold connection forever
		select {
		case <-r.Context().Done():
			return
		case <-time.After(30 * time.Second): // Safety timeout
			return
		}
	}))
	// Set longer close timeout to avoid server close panic
	server.EnableHTTP2 = false
	server.Start()
	defer server.CloseClientConnections() // Force close connections
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "profile.pb.gz")

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- captureProfileWithOps(ctx, &http.Client{Timeout: 10}, server.URL, destPath, "heap", defaultProfileCaptureOps())
	}()

	// Wait a bit for server to start
	time.Sleep(100 * time.Millisecond)

	// Cancel explicitly
	cancel()

	select {
	case err := <-errCh:
		// P0-7: Require ErrProfileCancelled and context.Canceled
		if err == nil {
			t.Fatal("Expected error")
		}
		if !errors.Is(err, ErrProfileCancelled) {
			t.Errorf("Expected ErrProfileCancelled, got: %v", err)
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context.Canceled, got: %v", err)
		}

		// Destination absent
		if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
			t.Errorf("Destination should be absent, got: %v", statErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Test timed out waiting for operation")
	}
}

// createValidGzipProfile creates a valid gzip profile accepted by the production validator.
func createValidGzipProfile(t *testing.T) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write([]byte("heap profile v1\n"))
	gz.Close()
	return buf.Bytes()
}

// Verify imports
var _, _ = fmt.Print, time.After
