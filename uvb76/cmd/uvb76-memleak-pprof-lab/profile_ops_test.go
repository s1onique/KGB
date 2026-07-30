// Package main provides the UVB-76 pprof memory leak lab.
//
// # Profile Operations Tests
//
// P0-3, P0-4, P0-5, P0-7: Deterministic fault injection tests.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/pprof/profile"
)

// Injected operation errors for deterministic testing.
var (
	errInjectedRead = errors.New("injected read error")
	errCreateTemp   = errors.New("injected CreateTemp error")
	errSync         = errors.New("injected sync error")
	errClose        = errors.New("injected close error")
	errRename       = errors.New("injected rename error")
	errRemove       = errors.New("injected remove error")
	errCopy         = errors.New("injected copy error")
)

// faultingTempFile implements profileTempFile for deterministic fault injection.
// P0-4: Uses real temporary file with injected failures.
// Each injected failure fires once, then delegates to the real file.
type faultingTempFile struct {
	file         *os.File
	syncErrOnce  error // injected sync error, fires once then nil
	closeErrOnce error // injected close error, fires once then nil
	syncCalls    int
	closeCalls   int
}

func (f *faultingTempFile) Name() string { return f.file.Name() }

func (f *faultingTempFile) Write(p []byte) (int, error) {
	return f.file.Write(p)
}

func (f *faultingTempFile) Seek(offset int64, whence int) (int64, error) {
	return f.file.Seek(offset, whence)
}

func (f *faultingTempFile) Sync() error {
	f.syncCalls++
	if f.syncErrOnce != nil {
		err := f.syncErrOnce
		f.syncErrOnce = nil // fire once
		return err
	}
	return f.file.Sync()
}

func (f *faultingTempFile) Close() error {
	f.closeCalls++
	if f.closeErrOnce != nil {
		err := f.closeErrOnce
		f.closeErrOnce = nil // fire once
		// Still close the real file to release descriptor
		_ = f.file.Close()
		return err
	}
	return f.file.Close()
}

// fakeTempFile implements profileTempFile for testing.
type fakeTempFile struct {
	name     string
	data     []byte
	pos      int
	syncErr  error
	closeErr error
	closed   bool
}

func (f *fakeTempFile) Name() string { return f.name }

func (f *fakeTempFile) Write(p []byte) (int, error) {
	if f.closed {
		return 0, errors.New("write on closed file")
	}
	f.data = append(f.data, p...)
	return len(p), nil
}

func (f *fakeTempFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		f.pos = int(offset)
	case io.SeekCurrent:
		f.pos += int(offset)
	case io.SeekEnd:
		f.pos = len(f.data) + int(offset)
	}
	return int64(f.pos), nil
}

func (f *fakeTempFile) Sync() error { return f.syncErr }

func (f *fakeTempFile) Close() error {
	f.closed = true
	return f.closeErr
}

// lifecycleFakeTempFile wraps fakeTempFile with lifecycle callbacks.
type lifecycleFakeTempFile struct {
	*fakeTempFile
	onClose func()
}

func (f *lifecycleFakeTempFile) Close() error {
	if f.onClose != nil {
		f.onClose()
	}
	return f.fakeTempFile.Close()
}

// fakeProfileOps implements profileCaptureOps for deterministic testing.
type fakeProfileOps struct {
	CreateTemp func(dir, pattern string) (profileTempFile, error)
	Rename     func(oldPath, newPath string) error
	Remove     func(path string) error
	Copy       func(dst io.Writer, src io.Reader) (int64, error)
}

func (f fakeProfileOps) Validate() error {
	if f.CreateTemp == nil {
		return errors.New("CreateTemp is nil")
	}
	if f.Rename == nil {
		return errors.New("Rename is nil")
	}
	if f.Remove == nil {
		return errors.New("Remove is nil")
	}
	if f.Copy == nil {
		return errors.New("Copy is nil")
	}
	return nil
}

// createSemanticPprofProfile creates a semantically valid pprof profile.
// P0-6: Uses the official pprof profile package to create a valid profile.
func createSemanticPprofProfile(t *testing.T) []byte {
	t.Helper()

	// Create a valid pprof profile using the official package
	p := &profile.Profile{
		SampleType: []*profile.ValueType{
			{Type: "alloc_objects", Unit: "count"},
			{Type: "alloc_space", Unit: "bytes"},
		},
		Sample: []*profile.Sample{
			{
				Location: []*profile.Location{},
				Value:    []int64{100, 200},
			},
		},
		PeriodType: &profile.ValueType{
			Type: "time",
			Unit: "nanoseconds",
		},
		Period: 1,
	}

	var buf bytes.Buffer
	if err := p.Write(&buf); err != nil {
		t.Fatalf("Failed to write profile: %v", err)
	}

	// Verify the profile can be parsed back
	if _, err := profile.ParseData(buf.Bytes()); err != nil {
		t.Fatalf("Profile is not valid pprof data: %v", err)
	}

	return buf.Bytes()
}

// createValidGzipProfile creates a valid gzip-compressed pprof profile for testing.
// Deprecated: Use createSemanticPprofProfile instead.
func createValidGzipProfile(t *testing.T) []byte {
	t.Helper()
	return createSemanticPprofProfile(t)
}

// TestCaptureProfileOps_Success verifies successful profile capture.
func TestCaptureProfileOps_Success(t *testing.T) {
	profileData := createValidGzipProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	err := CaptureProfile(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap")

	if err != nil {
		t.Fatalf("CaptureProfile failed: %v", err)
	}

	// Verify destination exists
	if _, statErr := os.Stat(destPath); os.IsNotExist(statErr) {
		t.Fatalf("Destination file not created: %v", statErr)
	}
}

// TestCaptureProfileOps_CreateTempFailure verifies CreateTemp failure handling.
func TestCaptureProfileOps_CreateTempFailure(t *testing.T) {
	profileData := createValidGzipProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	ops := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			return nil, errCreateTemp
		},
		Rename: os.Rename,
		Remove: os.Remove,
		Copy:   io.Copy,
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap", ops)

	if err == nil {
		t.Fatal("Expected CreateTemp error")
	}
	if !errors.Is(err, ErrProfilePublication) {
		t.Errorf("Expected ErrProfilePublication, got: %v", err)
	}
}

// Note: TestCaptureProfileOps_SyncFailure, TestCaptureProfileOps_CloseFailure,
// TestCaptureProfileOps_CleanupFailure, and TestCaptureProfileOps_RenameFailure
// are removed due to complex temp file lifecycle interactions with cleanup logic.
// The public API tests (TestCaptureProfile*) cover the important scenarios.

// TestCaptureProfileOps_BodyCopyError verifies body copy error handling.
func TestCaptureProfileOps_BodyCopyError(t *testing.T) {
	profileData := createValidGzipProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	ops := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			realFile, err := os.CreateTemp(dir, "test.tmp.*")
			if err != nil {
				return nil, err
			}
			name := realFile.Name()
			realFile.Close()
			os.Remove(name)
			return &fakeTempFile{name: name}, nil
		},
		Rename: os.Rename,
		Remove: os.Remove,
		Copy: func(dst io.Writer, src io.Reader) (int64, error) {
			return 0, errInjectedRead
		},
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap", ops)

	if err == nil {
		t.Fatal("Expected Copy error")
	}
	if !errors.Is(err, ErrProfileRead) {
		t.Errorf("Expected ErrProfileRead, got: %v", err)
	}
	if !errors.Is(err, errInjectedRead) {
		t.Errorf("Expected errInjectedRead, got: %v", err)
	}
}

// TestCaptureProfileOps_ExplicitCancelDuringBodyRead verifies cancellation during body read.
func TestCaptureProfileOps_ExplicitCancelDuringBodyRead(t *testing.T) {
	readStarted := make(chan struct{})
	readUnblocked := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()

		// Signal that we're about to write more data
		close(readStarted)

		// Wait for cancel signal before continuing
		<-readUnblocked

		// Write remaining data
		fmt.Fprint(w, string(make([]byte, 1024)))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		ops := profileCaptureOps{
			CreateTemp: func(dir, pattern string) (profileTempFile, error) {
				return os.CreateTemp(dir, pattern)
			},
			Rename: os.Rename,
			Remove: os.Remove,
			Copy:   io.Copy,
		}
		errCh <- captureProfileWithOps(ctx, &http.Client{Timeout: 10 * time.Second}, server.URL, destPath, "heap", ops)
	}()

	// Wait for read to start
	<-readStarted

	// Cancel immediately
	cancel()

	// Unblock the server
	close(readUnblocked)

	// Wait for capture to complete
	err := <-errCh

	if err == nil {
		t.Fatal("Expected cancel error")
	}
	if !errors.Is(err, ErrProfileCancelled) {
		t.Errorf("Expected ErrProfileCancelled, got: %v", err)
	}

	// Destination should be absent
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}
}

// TestCaptureProfileOps_ValidationError verifies validation error handling.
func TestCaptureProfileOps_ValidationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		// Write invalid pprof data
		w.Write([]byte("not a valid pprof profile"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	err := CaptureProfile(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap")

	if err == nil {
		t.Fatal("Expected validation error")
	}
	if !errors.Is(err, ErrProfileValidation) {
		t.Errorf("Expected ErrProfileValidation, got: %v", err)
	}
}

// Note: TestCaptureProfileOps_RenameFailure is removed due to complex temp file
// lifecycle interactions. The public API test TestCaptureProfile_PublicationError
// covers rename failure scenarios.

// TestCaptureProfileOps_EmptyBody verifies empty body handling.
func TestCaptureProfileOps_EmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		// Write empty body
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	err := CaptureProfile(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap")

	if err == nil {
		t.Fatal("Expected empty body error")
	}
	if !errors.Is(err, ErrProfileBodyEmpty) {
		t.Errorf("Expected ErrProfileBodyEmpty, got: %v", err)
	}
}

// TestCaptureProfileOps_TransportError verifies transport error handling.
func TestCaptureProfileOps_TransportError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close connection without response
		hijacked, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("Handler does not support hijacking")
		}
		conn, _, err := hijacked.Hijack()
		if err != nil {
			t.Fatalf("Hijack failed: %v", err)
		}
		conn.Close()
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	err := CaptureProfile(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap")

	if err == nil {
		t.Fatal("Expected transport error")
	}
	if !errors.Is(err, ErrProfileTransport) {
		t.Errorf("Expected ErrProfileTransport, got: %v", err)
	}
}

// TestCaptureProfileOps_ConcurrentAccess verifies concurrent access safety.
func TestCaptureProfileOps_ConcurrentAccess(t *testing.T) {
	profileData := createValidGzipProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			destPath := filepath.Join(tmpDir, fmt.Sprintf("heap-%d.pb.gz", n))
			err := CaptureProfile(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap")
			if err != nil {
				t.Errorf("Concurrent capture %d failed: %v", n, err)
			}
		}(i)
	}

	wg.Wait()
}

// TestProfileOps_ValidateProfileOps verifies operation validation.
func TestProfileOps_ValidateProfileOps(t *testing.T) {
	// Valid ops
	validOps := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) { return nil, nil },
		Rename:     os.Rename,
		Remove:     os.Remove,
		Copy:       io.Copy,
	}
	if err := validateProfileOps(validOps); err != nil {
		t.Errorf("Valid ops should not error: %v", err)
	}

	// Invalid: nil CreateTemp
	invalidOps := profileCaptureOps{
		CreateTemp: nil,
		Rename:     os.Rename,
		Remove:     os.Remove,
		Copy:       io.Copy,
	}
	if err := validateProfileOps(invalidOps); err == nil {
		t.Error("Nil CreateTemp should error")
	}
}

// TestProfileOps_DefaultOps verifies default operations work.
func TestProfileOps_DefaultOps(t *testing.T) {
	ops := defaultProfileCaptureOps()
	if err := validateProfileOps(ops); err != nil {
		t.Errorf("Default ops should be valid: %v", err)
	}
}

// TestProfileOps_CleanupPreserving verifies cleanup error preservation.
func TestProfileOps_CleanupPreserving(t *testing.T) {
	// Test with remove error
	ops := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			return &fakeTempFile{name: "/nonexistent/file"}, nil
		},
		Rename: os.Rename,
		Remove: func(path string) error {
			return errRemove
		},
		Copy: io.Copy,
	}

	tmp := &fakeTempFile{name: "/nonexistent/file"}
	err := cleanupProfileTempPreserving(ops, tmp, "/nonexistent/file")

	if !errors.Is(err, errRemove) {
		t.Errorf("Expected errRemove, got: %v", err)
	}
}

// TestProfileOps_ValidateGzipProfile verifies gzip validation by using ValidateProfile.
func TestProfileOps_ValidateGzipProfile(t *testing.T) {
	// Create a temp file with valid gzip data
	valid := createValidGzipProfile(t)

	tmpDir := t.TempDir()
	validPath := filepath.Join(tmpDir, "valid.gz")
	if err := os.WriteFile(validPath, valid, 0644); err != nil {
		t.Fatalf("Failed to write valid gzip: %v", err)
	}

	// Validate should succeed
	if err := ValidateProfile(validPath, "heap"); err != nil {
		t.Errorf("Valid gzip should validate: %v", err)
	}

	// Create a temp file with invalid data
	invalidPath := filepath.Join(tmpDir, "invalid.gz")
	if err := os.WriteFile(invalidPath, []byte("not gzip"), 0644); err != nil {
		t.Fatalf("Failed to write invalid data: %v", err)
	}

	// Validate should fail
	if err := ValidateProfile(invalidPath, "heap"); err == nil {
		t.Error("Invalid gzip should not validate")
	}
}

// TestProfileOps_SyncFailure verifies sync failure handling.
// P0-3: Deterministic sync failure injection.
func TestProfileOps_SyncFailure(t *testing.T) {
	profileData := createSemanticPprofProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	// Track cleanup calls
	removeCalled := false

	ops := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			realFile, err := os.CreateTemp(dir, pattern)
			if err != nil {
				return nil, err
			}
			return &faultingTempFile{
				file:        realFile,
				syncErrOnce: errSync,
			}, nil
		},
		Rename: os.Rename,
		Remove: func(path string) error {
			removeCalled = true
			return os.Remove(path)
		},
		Copy: io.Copy,
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap", ops)

	if err == nil {
		t.Fatal("Expected sync error")
	}

	// Verify both broad and physical errors are preserved
	if !errors.Is(err, ErrProfilePublication) {
		t.Errorf("Expected ErrProfilePublication, got: %v", err)
	}
	if !errors.Is(err, errSync) {
		t.Errorf("Expected errSync, got: %v", err)
	}

	// Verify cleanup was attempted
	if !removeCalled {
		t.Error("Remove should have been called for temp file cleanup")
	}

	// Verify destination is unchanged
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}
}

// TestProfileOps_CloseFailure verifies close failure handling.
// P0-3: Deterministic close failure with cleanup retry.
func TestProfileOps_CloseFailure(t *testing.T) {
	profileData := createSemanticPprofProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	// Track cleanup calls
	var tempFile *faultingTempFile
	closeCalls := 0

	ops := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			realFile, err := os.CreateTemp(dir, pattern)
			if err != nil {
				return nil, err
			}
			tempFile = &faultingTempFile{
				file:         realFile,
				closeErrOnce: errClose,
			}
			return tempFile, nil
		},
		Rename: os.Rename,
		Remove: func(path string) error {
			closeCalls = tempFile.closeCalls
			return os.Remove(path)
		},
		Copy: io.Copy,
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap", ops)

	if err == nil {
		t.Fatal("Expected close error")
	}

	// Verify both broad and physical errors are preserved
	if !errors.Is(err, ErrProfilePublication) {
		t.Errorf("Expected ErrProfilePublication, got: %v", err)
	}
	if !errors.Is(err, errClose) {
		t.Errorf("Expected errClose, got: %v", err)
	}

	// Verify first close error is preserved
	if closeCalls < 1 {
		t.Error("Close should have been called at least once")
	}

	// Verify cleanup was attempted
	if tempFile == nil {
		t.Error("Temp file should have been created")
	}
}

// TestProfileOps_RenameFailure verifies rename failure handling.
// P0-3: Deterministic rename failure with cleanup.
func TestProfileOps_RenameFailure(t *testing.T) {
	profileData := createSemanticPprofProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	// Track cleanup
	var tempPath string
	removeCalled := false

	ops := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			realFile, err := os.CreateTemp(dir, pattern)
			if err != nil {
				return nil, err
			}
			tempPath = realFile.Name()
			// Don't close the file - let the lifecycle manage it
			return &faultingTempFile{file: realFile}, nil
		},
		Rename: func(oldPath, newPath string) error {
			return errRename
		},
		Remove: func(path string) error {
			removeCalled = true
			return os.Remove(path)
		},
		Copy: io.Copy,
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap", ops)

	if err == nil {
		t.Fatal("Expected rename error")
	}

	// Verify both broad and physical errors are preserved
	if !errors.Is(err, ErrProfilePublication) {
		t.Errorf("Expected ErrProfilePublication, got: %v", err)
	}
	if !errors.Is(err, errRename) {
		t.Errorf("Expected errRename, got: %v", err)
	}

	// Verify temp path is absent after cleanup
	if tempPath != "" {
		if _, statErr := os.Stat(tempPath); !os.IsNotExist(statErr) {
			t.Errorf("Temp path should be absent, got: %v", statErr)
		}
	}

	// Verify destination is absent
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}

	// Verify cleanup was attempted
	if !removeCalled {
		t.Error("Remove should have been called")
	}
}

// TestProfileOps_CombinedFailure verifies combined initiating + cleanup failure.
// P0-3: Multiple error preservation with errors.Join.
func TestProfileOps_CombinedFailure(t *testing.T) {
	profileData := createSemanticPprofProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	ops := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			realFile, err := os.CreateTemp(dir, pattern)
			if err != nil {
				return nil, err
			}
			return &faultingTempFile{file: realFile}, nil
		},
		Rename: func(oldPath, newPath string) error {
			return errRename
		},
		Remove: func(path string) error {
			return errRemove
		},
		Copy: io.Copy,
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap", ops)

	if err == nil {
		t.Fatal("Expected combined error")
	}

	// Verify all errors are preserved
	if !errors.Is(err, ErrProfilePublication) {
		t.Errorf("Expected ErrProfilePublication, got: %v", err)
	}
	if !errors.Is(err, errRename) {
		t.Errorf("Expected errRename, got: %v", err)
	}
	if !errors.Is(err, errRemove) {
		t.Errorf("Expected errRemove, got: %v", err)
	}
}

// TestProfileOps_ClosePlusRemoveFailure verifies close + remove cleanup failure.
// P0-3: Both close and remove errors preserved.
func TestProfileOps_ClosePlusRemoveFailure(t *testing.T) {
	profileData := createSemanticPprofProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	var tempFile *faultingTempFile

	ops := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			realFile, err := os.CreateTemp(dir, pattern)
			if err != nil {
				return nil, err
			}
			tempFile = &faultingTempFile{
				file:         realFile,
				closeErrOnce: errClose,
			}
			return tempFile, nil
		},
		Rename: os.Rename,
		Remove: func(path string) error {
			return errRemove
		},
		Copy: io.Copy,
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap", ops)

	if err == nil {
		t.Fatal("Expected combined close + remove error")
	}

	// Verify both close and remove errors are preserved
	if !errors.Is(err, errClose) {
		t.Errorf("Expected errClose, got: %v", err)
	}
	if !errors.Is(err, errRemove) {
		t.Errorf("Expected errRemove, got: %v", err)
	}
}
