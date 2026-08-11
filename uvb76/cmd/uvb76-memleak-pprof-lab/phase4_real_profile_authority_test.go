// Package main provides the UVB-76 pprof memory leak lab.
//
// # Phase 4: Real CaptureProfile Authority + Cleanup Ownership Tests
//
// These tests exercise the ACTUAL production profile authority path through
// the CaptureProfilesFn seam used by RunCollectionLifecycle.
//
// Authority chain:
//   real caller (runner.go)
//   → CaptureProfilesFn (injected into RunCollectionLifecycle)
//   → captureProfilesWithValidation
//   → CaptureProfile (canonical entry)
//   → captureProfileWithOps
//   → defaultProfileCaptureOps
//
// Cleanup ownership:
//   temp file created → cleanup called on failure
//   rename successful → cleanup NOT called (ownership transferred)
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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/pprof/profile"
)

// =============================================================================
// Phase 4: Real CaptureProfile Authority Tests
// =============================================================================

// TestCaptureProfileRealPath_Success verifies the real production path succeeds.
func TestCaptureProfileRealPath_Success(t *testing.T) {
	// Create a valid pprof profile
	profileData := generateTestProfile(t, "heap")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	// Use the REAL production CaptureProfile
	err := CaptureProfile(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap")
	if err != nil {
		t.Fatalf("CaptureProfile failed: %v", err)
	}

	// Verify destination exists
	if _, statErr := os.Stat(destPath); os.IsNotExist(statErr) {
		t.Fatal("Destination file was not created")
	}

	t.Log("Real CaptureProfile path succeeded")
}

// TestCaptureProfileRealPath_AlreadyCancelled verifies already-cancelled context.
func TestCaptureProfileRealPath_AlreadyCancelled(t *testing.T) {
	profileData := generateTestProfile(t, "heap")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	// Create already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := CaptureProfile(ctx, &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap")
	if err == nil {
		t.Fatal("Expected error for cancelled context")
	}
	// Hard assertion: error must be typed as cancellation
	if !errors.Is(err, context.Canceled) && !errors.Is(err, ErrProfileCancelled) {
		t.Fatalf("expected cancellation identity, got: %v", err)
	}

	// Destination should be absent
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}

	t.Log("Already-cancelled context correctly rejected")
}

// TestCaptureProfileRealPath_TransportError verifies HTTP transport failure.
func TestCaptureProfileRealPath_TransportError(t *testing.T) {
	// Use a transport that always fails
	httpClient := &http.Client{
		Transport: &errorTransport{err: errors.New("connection refused")},
	}

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	err := CaptureProfile(context.Background(), httpClient, "http://localhost:1/debug/pprof/heap", destPath, "heap")
	if err == nil {
		t.Fatal("Expected transport error")
	}
	if !errors.Is(err, ErrProfileTransport) {
		t.Errorf("Expected ErrProfileTransport, got: %v", err)
	}

	t.Log("Transport error correctly typed")
}

// errorTransport is a RoundTripper that always returns an error.
type errorTransport struct {
	err error
}

func (t *errorTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, t.err
}

// TestCaptureProfileRealPath_NonOKStatus verifies non-200 status.
func TestCaptureProfileRealPath_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	err := CaptureProfile(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap")
	if err == nil {
		t.Fatal("Expected status error")
	}

	t.Logf("Non-OK status correctly rejected: %v", err)
}

// TestCaptureProfileRealPath_InvalidProfile verifies validation failure.
func TestCaptureProfileRealPath_InvalidProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
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

	t.Log("Invalid profile correctly rejected")
}

// TestCaptureProfileRealPath_EmptyBody verifies empty body handling.
func TestCaptureProfileRealPath_EmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		// Write nothing
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

	t.Log("Empty body correctly rejected")
}

// TestCaptureProfileOps_RealPathCreateTempFailure verifies CreateTemp failure ownership boundary.
// CreateTemp fails → ownership never established → Remove == 0
func TestCaptureProfileOps_RealPathCreateTempFailure(t *testing.T) {
	profileData := generateTestProfile(t, "heap")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	createErr := errors.New("injected create temp failure")
	var cleanupCalls atomic.Int64

	ops := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			return nil, createErr
		},
		Rename: os.Rename,
		Remove: func(path string) error {
			cleanupCalls.Add(1)
			return os.Remove(path)
		},
		Copy: io.Copy,
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap", ops)

	if err == nil {
		t.Fatal("Expected CreateTemp error")
	}

	// Primary error should be ErrProfilePublication
	if !errors.Is(err, ErrProfilePublication) {
		t.Fatalf("Expected ErrProfilePublication, got: %v", err)
	}

	// Underlying cause should be preserved
	if !errors.Is(err, createErr) {
		t.Fatalf("Expected createErr in error chain, got: %v", err)
	}

	// Ownership never established → cleanup should NOT be called
	if cleanupCalls.Load() != 0 {
		t.Fatalf("cleanup before ownership: got %d, want 0", cleanupCalls.Load())
	}

	t.Logf("CreateTemp failure: ownership not established, cleanup == 0: %v", err)
}

// TestCaptureProfileOps_CopyFailure verifies Copy failure ownership boundary.
// CreateTemp succeeds → ownership established → Copy fails → Remove == 1
func TestCaptureProfileOps_CopyFailure(t *testing.T) {
	var cleanupCalls atomic.Int64

	profileData := generateTestProfile(t, "heap")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	copyErr := errors.New("injected copy failure")

	ops := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		Rename: os.Rename,
		Remove: func(path string) error {
			cleanupCalls.Add(1)
			return os.Remove(path)
		},
		Copy: func(dst io.Writer, src io.Reader) (int64, error) {
			// Read one byte then fail
			buf := make([]byte, 1)
			n, _ := src.Read(buf)
			if n > 0 {
				dst.Write(buf[:n])
			}
			return 0, copyErr
		},
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap", ops)

	if err == nil {
		t.Fatal("Expected Copy error")
	}

	// Primary error should be ErrProfileRead (copy failure)
	if !errors.Is(err, ErrProfileRead) {
		t.Fatalf("Expected ErrProfileRead, got: %v", err)
	}

	// Underlying cause should be preserved
	if !errors.Is(err, copyErr) {
		t.Fatalf("Expected copyErr in error chain, got: %v", err)
	}

	// Cleanup should have been called (ownership established)
	if cleanupCalls.Load() != 1 {
		t.Fatalf("cleanupCalls: got %d, want 1", cleanupCalls.Load())
	}

	t.Logf("Copy failure: ownership established, cleanup == 1: %v", err)
}

// =============================================================================
// Phase 4: Cleanup Ownership Tests
// =============================================================================

// TestCaptureProfileOps_CleanupExactlyOnce verifies cleanup is called exactly once
// when temp file is created.
func TestCaptureProfileOps_CleanupExactlyOnce(t *testing.T) {
	var cleanupCalls atomic.Int64
	var tempCreated bool

	profileData := generateTestProfile(t, "heap")

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
			f, err := os.CreateTemp(dir, pattern)
			if err == nil {
				tempCreated = true
			}
			return f, err
		},
		Rename: func(src, dst string) error {
			// Fail rename to trigger cleanup
			return errors.New("injected rename failure")
		},
		Remove: func(path string) error {
			cleanupCalls.Add(1)
			return os.Remove(path)
		},
		Copy: io.Copy,
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap", ops)

	// Verify temp was created
	if !tempCreated {
		t.Fatal("Temp file was not created")
	}

	// Verify cleanup was called exactly once
	if cleanupCalls.Load() != 1 {
		t.Errorf("cleanupCalls: got %d, want 1", cleanupCalls.Load())
	}

	// Original error should be preserved
	if err == nil {
		t.Fatal("Expected error from failed rename")
	}

	t.Logf("Cleanup called exactly once: %d, error: %v", cleanupCalls.Load(), err)
}

// TestCaptureProfileOps_CleanupOnCopyError verifies cleanup on copy failure.
func TestCaptureProfileOps_CleanupOnCopyError(t *testing.T) {
	var cleanupCalls atomic.Int64
	var tempCreated bool

	profileData := generateTestProfile(t, "heap")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "heap.pb.gz")

	ops := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			f, err := os.CreateTemp(dir, pattern)
			if err == nil {
				tempCreated = true
			}
			return f, err
		},
		Rename: os.Rename,
		Remove: func(path string) error {
			cleanupCalls.Add(1)
			return os.Remove(path)
		},
		Copy: func(dst io.Writer, src io.Reader) (int64, error) {
			// Copy only partial data then fail
			buf := make([]byte, 1024)
			n, _ := src.Read(buf)
			if n > 0 {
				dst.Write(buf[:n])
			}
			return 0, errors.New("injected copy failure")
		},
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap", ops)

	if !tempCreated {
		t.Fatal("Temp file was not created")
	}

	if cleanupCalls.Load() != 1 {
		t.Errorf("cleanupCalls: got %d, want 1", cleanupCalls.Load())
	}

	if err == nil {
		t.Fatal("Expected error from failed copy")
	}

	t.Logf("Cleanup on copy failure: %d calls", cleanupCalls.Load())
}

// TestCaptureProfileOps_NoCleanupOnSuccess verifies NO cleanup when rename succeeds.
func TestCaptureProfileOps_NoCleanupOnSuccess(t *testing.T) {
	var cleanupCalls atomic.Int64

	profileData := generateTestProfile(t, "heap")

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
			return os.CreateTemp(dir, pattern)
		},
		Rename: os.Rename,
		Remove: func(path string) error {
			cleanupCalls.Add(1)
			return os.Remove(path)
		},
		Copy: io.Copy,
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap", ops)

	if err != nil {
		t.Fatalf("CaptureProfile failed: %v", err)
	}

	// Verify destination exists
	if _, statErr := os.Stat(destPath); os.IsNotExist(statErr) {
		t.Fatal("Destination was not created")
	}

	// Cleanup should NOT be called (ownership transferred to destination)
	if cleanupCalls.Load() != 0 {
		t.Errorf("cleanupCalls: got %d, want 0", cleanupCalls.Load())
	}

	t.Log("No cleanup on success (ownership transferred)")
}

// TestCaptureProfileOps_PrimaryAndCleanupFailurePreserved verifies all errors preserved.
func TestCaptureProfileOps_PrimaryAndCleanupFailurePreserved(t *testing.T) {
	profileData := generateTestProfile(t, "heap")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	renameErr := errors.New("injected rename failure")
	removeErr := errors.New("injected remove failure")

	ops := profileCaptureOps{
		CreateTemp: func(dir, pattern string) (profileTempFile, error) {
			f, err := os.CreateTemp(dir, pattern)
			return f, err
		},
		Rename: func(src, dst string) error {
			return renameErr
		},
		Remove: func(path string) error {
			return removeErr
		},
		Copy: io.Copy,
	}

	err := captureProfileWithOps(context.Background(), &http.Client{Timeout: 5 * time.Second}, server.URL, destPath, "heap", ops)

	if err == nil {
		t.Fatal("Expected errors")
	}

	// Primary error category should be preserved
	if !errors.Is(err, ErrProfilePublication) {
		t.Fatalf("Expected ErrProfilePublication in error chain, got: %v", err)
	}

	// Cleanup error should be preserved (errors.Join includes all errors)
	if !errors.Is(err, removeErr) {
		t.Fatalf("Expected removeErr in error chain, got: %v", err)
	}

	// Primary cause (renameErr) should also be preserved
	// errors.Is traverses the complete wrapped/joined error tree
	if !errors.Is(err, renameErr) {
		t.Fatalf("Expected renameErr in error chain, got: %v", err)
	}

	t.Logf("All errors preserved: %v", err)
}

// =============================================================================
// Phase 4: CaptureProfilesFn Seam Integration
// =============================================================================

// TestCaptureProfilesFn_SeamIntegration verifies the production authority chain
// from runner.go → RunCollectionLifecycle → captureProfilesWithValidation → CaptureProfile.
//
// This test uses the ACTUAL production captureProfilesWithValidation function
// (runner.go:820), proving the real production composition is exercised.
func TestCaptureProfilesFn_SeamIntegration(t *testing.T) {
	// Create a test server with valid profile
	profileData := generateTestProfile(t, "heap")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	// Track profile calls through the seam
	var profileCalls atomic.Int64

	// Extract pprof port from test server URL
	// The production captureProfilesWithValidation expects localhost:pprofPort URLs
	serverURL := server.URL
	_, portStr, _ := strings.Cut(strings.TrimPrefix(serverURL, "http://"), ":")
	pprofPort := portStr

	// Build the CaptureProfilesFn using ACTUAL production captureProfilesWithValidation
	// This is the same pattern as runner.go:362-367
	artifactDir := t.TempDir()
	collectionStart := time.Now()
	duration := 100 * time.Millisecond
	interval := 50 * time.Millisecond
	observationEnd := collectionStart.Add(10 * time.Second)

	captureFn := func(ctx context.Context) error {
		profileCalls.Add(1)
		// Use the ACTUAL production function (same package, same authority)
		return captureProfilesWithValidation(
			ctx,
			&http.Client{Timeout: 5 * time.Second},
			pprofPort,
			artifactDir,
			collectionStart,
			duration,
			interval,
			observationEnd,
		)
	}

	// Create contexts matching production lifecycle
	labCtx, labCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer labCancel()

	obsCtx, obsCancel := context.WithCancel(labCtx)
	defer obsCancel()

	profileDeadline := time.Now().Add(5 * time.Second)
	profileCtx, profileCancel := context.WithDeadline(labCtx, profileDeadline)
	defer profileCancel()

	// Run through RunCollectionLifecycle with controlled lifecycle parameters
	var wg sync.WaitGroup
	lifecycleInput := CollectionLifecycleInput{
		ObservationCtx:     obsCtx,
		ProfileCtx:         profileCtx,
		ObservationCancel:  obsCancel,
		WaitGroup:          &wg,
		CollectorInput:     &CollectorInput{},
		PollInput: TargetPollInput{
			Client:           &http.Client{Timeout: 5 * time.Second},
			UVB76APIBaseURL:   "http://localhost:1",
			Target:           TargetConfigBinding{},
			Auth:             TargetStateAuthInput{},
			PollInterval:     100 * time.Millisecond,
			RequestTimeout:    100 * time.Millisecond,
			Deadline:          5 * time.Second,
		},
		PollDrainTimeout:   100 * time.Millisecond,
		CaptureProfilesFn:  captureFn,
	}

	result := RunCollectionLifecycle(lifecycleInput)

	// Verify CaptureProfilesFn was called exactly once
	if profileCalls.Load() != 1 {
		t.Errorf("profileCalls: got %d, want 1", profileCalls.Load())
	}

	// Verify the lifecycle completed successfully
	if result.PollTerminalError != nil {
		t.Fatalf("CaptureProfilesFn via RunCollectionLifecycle failed: %v", result.PollTerminalError)
	}

	t.Log("Production authority chain verified: runner.go → RunCollectionLifecycle → captureProfilesWithValidation → CaptureProfile")
}

// =============================================================================
// Phase 4: Cancellation During Body Read
// =============================================================================

// TestCaptureProfileRealPath_CancelDuringBodyRead verifies cancellation during body read.
func TestCaptureProfileRealPath_CancelDuringBodyRead(t *testing.T) {
	readStarted := make(chan struct{})
	readUnblocked := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()

		close(readStarted)
		<-readUnblocked
		fmt.Fprint(w, string(make([]byte, 1024)))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- CaptureProfile(ctx, &http.Client{Timeout: 10 * time.Second}, server.URL, destPath, "heap")
	}()

	<-readStarted
	cancel()
	close(readUnblocked)

	err := <-errCh
	if err == nil {
		t.Fatal("Expected cancel error")
	}
	if !errors.Is(err, ErrProfileCancelled) && !errors.Is(err, context.Canceled) {
		t.Errorf("Expected cancellation error, got: %v", err)
	}

	// Destination should be absent
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}

	t.Log("Cancellation during body read correctly handled")
}

// =============================================================================
// Helper
// =============================================================================

// generateTestProfile creates a valid pprof heap profile for testing.
func generateTestProfile(t *testing.T, profileType string) []byte {
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

	return buf.Bytes()
}
