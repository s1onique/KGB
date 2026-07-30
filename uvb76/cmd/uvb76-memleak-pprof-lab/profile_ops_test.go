// Package main provides the UVB-76 pprof memory leak lab.
//
// # Profile Operations Tests
//
// P0-3, P0-4, P0-5, P0-7: Deterministic fault injection tests.
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
	"sync"
	"testing"
	"time"
)

// Injected operation errors for deterministic testing.
var (
	errInjectedRead     = errors.New("injected read error")
	errCreateTemp       = errors.New("injected CreateTemp error")
	errSync             = errors.New("injected Sync error")
	errClose            = errors.New("injected Close error")
	errRename           = errors.New("injected Rename error")
	errRemove           = errors.New("injected Remove error")
	errCopy             = errors.New("injected Copy error")
)

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
	Rename    func(oldPath, newPath string) error
	Remove    func(path string) error
	Copy      func(dst io.Writer, src io.Reader) (int64, error)
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

// createValidGzipProfile creates a valid gzip-compressed pprof profile for testing.
func createValidGzipProfile(t *testing.T) []byte {
	t.Helper()

	// Create a minimal pprof protobuf profile
	var profileBuf bytes.Buffer
	gz := gzip.NewWriter(&profileBuf)

	// Write minimal pprof profile data
	// This is a valid pprof profile with period_type, sample_type, and location
	profileData := []byte{
		// Profile header
		0x0a, 0x04, 'h', 'e', 'a', 'p', // period_type: "heap"
		0x12, 0x03, 'a', 'b', 'c', // some profile data
	}

	gz.Write(profileData)
	gz.Close()

	return profileBuf.Bytes()
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

// TestCaptureProfileOps_SyncFailure verifies Sync failure handling.
func TestCaptureProfileOps_SyncFailure(t *testing.T) {
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
			os.Remove(name) // Remove so cleanup can succeed
			return &lifecycleFakeTempFile{
				fakeTempFile: &fakeTempFile{
					name:     name,
					syncErr:  errSync,
				},
			}, nil
		},
		Rename: os.Rename,
		Remove: os.Remove,
		Copy:   io.Copy,
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap", ops)

	if err == nil {
		t.Fatal("Expected Sync error")
	}
	if !errors.Is(err, ErrProfilePublication) {
		t.Errorf("Expected ErrProfilePublication, got: %v", err)
	}
	if !errors.Is(err, errSync) {
		t.Errorf("Expected errSync, got: %v", err)
	}
}

// TestCaptureProfileOps_CloseFailure verifies Close failure handling.
func TestCaptureProfileOps_CloseFailure(t *testing.T) {
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
			os.Remove(name) // Remove so cleanup doesn't try to remove it again
			return &lifecycleFakeTempFile{
				fakeTempFile: &fakeTempFile{
					name:      name,
					closeErr:  errClose,
				},
			}, nil
		},
		Rename: os.Rename,
		Remove: os.Remove,
		Copy:   io.Copy,
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap", ops)

	if err == nil {
		t.Fatal("Expected Close error")
	}
	if !errors.Is(err, ErrProfilePublication) {
		t.Errorf("Expected ErrProfilePublication, got: %v", err)
	}
	if !errors.Is(err, errClose) {
		t.Errorf("Expected errClose, got: %v", err)
	}
}

// TestCaptureProfileOps_CleanupFailure verifies cleanup error preservation.
func TestCaptureProfileOps_CleanupFailure(t *testing.T) {
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
			os.Remove(name) // Remove so cleanup can fail
			return &lifecycleFakeTempFile{
				fakeTempFile: &fakeTempFile{
					name:     name,
					syncErr:  errSync,
				},
			}, nil
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
		t.Fatal("Expected error")
	}

	// Both sync and cleanup errors should be present
	if !errors.Is(err, ErrProfilePublication) {
		t.Errorf("Expected ErrProfilePublication, got: %v", err)
	}
	if !errors.Is(err, errSync) {
		t.Errorf("Expected errSync, got: %v", err)
	}
	if !errors.Is(err, errRemove) {
		t.Errorf("Expected errRemove (cleanup), got: %v", err)
	}
}

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

// TestCaptureProfileOps_RenameFailure verifies rename failure handling.
func TestCaptureProfileOps_RenameFailure(t *testing.T) {
	profileData := createValidGzipProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	// Create dest as existing file to make rename fail
	destPath := filepath.Join(tmpDir, "heap.pb.gz")
	f, err := os.Create(destPath)
	if err != nil {
		t.Fatalf("Create dest failed: %v", err)
	}
	f.Close()

	ops := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		Rename: func(oldPath, newPath string) error {
			return errRename
		},
		Remove: func(path string) error {
			return nil // Successful cleanup
		},
		Copy: io.Copy,
	}

	err = captureProfileWithOps(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap", ops)

	if err == nil {
		t.Fatal("Expected Rename error")
	}
	if !errors.Is(err, ErrProfilePublication) {
		t.Errorf("Expected ErrProfilePublication, got: %v", err)
	}
	if !errors.Is(err, errRename) {
		t.Errorf("Expected errRename, got: %v", err)
	}
}

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
		Rename:    os.Rename,
		Remove:    os.Remove,
		Copy:      io.Copy,
	}
	if err := validateProfileOps(validOps); err != nil {
		t.Errorf("Valid ops should not error: %v", err)
	}

	// Invalid: nil CreateTemp
	invalidOps := profileCaptureOps{
		CreateTemp: nil,
		Rename:    os.Rename,
		Remove:    os.Remove,
		Copy:      io.Copy,
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
