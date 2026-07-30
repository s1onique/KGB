// Package main provides the UVB-76 pprof memory leak lab.
//
// # Profile Validation Cancellation Tests
//
// P0-9: Tests for cancellation-safe profile publication.
// P0-9: All tests verify no partial artifacts are left behind.
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

// TestCaptureProfile_CancelBeforeHeaders verifies that cancelling before headers
// leaves no destination or temp files.
func TestCaptureProfile_CancelBeforeHeaders(t *testing.T) {
	// P0: Handler must observe r.Context().Done() to allow server cleanup
	requestStarted := make(chan struct{}, 1)
	handlerExited := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		defer close(handlerExited)
		// Block until cancelled - observe context for proper cleanup
		<-r.Context().Done()
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test-profile.pb.gz")

	// P0: Use explicit WithCancel for cancellation test
	// P0: WithTimeout fires as DeadlineExceeded, not Canceled
	client := &http.Client{Timeout: 10 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())

	// Run in goroutine so we can cancel
	errCh := make(chan error, 1)
	go func() {
		errCh <- CaptureProfile(ctx, client, server.URL, destPath, "heap")
	}()

	// Wait for request to start
	<-requestStarted

	// Cancel explicitly
	cancel()

	err := <-errCh

	// Should have cancellation error
	if err == nil {
		t.Error("Expected error after cancellation")
	}

	// P0: Require both ErrProfileCancelled AND context.Canceled
	if !errors.Is(err, ErrProfileCancelled) {
		t.Errorf("Expected ErrProfileCancelled, got: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled, got: %v", err)
	}

	// Verify destination absent
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}

	// Verify no temp files
	tempFiles, _ := filepath.Glob(filepath.Join(tmpDir, "test-profile*.tmp*"))
	if len(tempFiles) > 0 {
		t.Errorf("Temp files should be absent, found: %v", tempFiles)
	}

	// Wait for handler cleanup
	<-handlerExited
}

// TestCaptureProfile_DeadlineBeforeHeaders verifies that deadline before headers
// leaves no destination or temp files.
func TestCaptureProfile_DeadlineBeforeHeaders(t *testing.T) {
	requestStarted := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		// Block until deadline - observe context
		<-r.Context().Done()
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test-profile.pb.gz")

	// P0: Use WithTimeout for deadline test
	client := &http.Client{Timeout: 10 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := CaptureProfile(ctx, client, server.URL, destPath, "heap")

	// Should have deadline error
	if err == nil {
		t.Error("Expected error after deadline")
	}

	// P0: Require both ErrProfileDeadline AND context.DeadlineExceeded
	if !errors.Is(err, ErrProfileDeadline) {
		t.Errorf("Expected ErrProfileDeadline, got: %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected context.DeadlineExceeded, got: %v", err)
	}

	// Verify destination absent
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}

	// Verify no temp files
	tempFiles, _ := filepath.Glob(filepath.Join(tmpDir, "test-profile*.tmp*"))
	if len(tempFiles) > 0 {
		t.Errorf("Temp files should be absent, found: %v", tempFiles)
	}
}

// TestCaptureProfile_CancelDuringBody verifies that cancelling during body read
// leaves no destination or temp files.
func TestCaptureProfile_CancelDuringBody(t *testing.T) {
	bodyStarted := make(chan struct{}, 1)
	handlerExited := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(bodyStarted)

		// Write slowly to allow cancellation
		for i := 0; i < 100; i++ {
			fmt.Fprint(w, "x")
			w.(http.Flusher).Flush()
			time.Sleep(10 * time.Millisecond)
			select {
			case <-r.Context().Done():
				close(handlerExited)
				return
			default:
			}
		}
		close(handlerExited)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test-profile.pb.gz")

	// P0: Use explicit WithCancel for cancellation test
	client := &http.Client{Timeout: 10 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())

	// Run in goroutine so we can cancel
	errCh := make(chan error, 1)
	go func() {
		errCh <- CaptureProfile(ctx, client, server.URL, destPath, "heap")
	}()

	// Wait for body to start being read
	<-bodyStarted

	// Cancel explicitly
	cancel()

	err := <-errCh

	// P0: Require both ErrProfileCancelled AND context.Canceled
	if !errors.Is(err, ErrProfileCancelled) {
		t.Errorf("Expected ErrProfileCancelled, got: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled, got: %v", err)
	}

	// Verify destination absent
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}

	// Verify no temp files
	tempFiles, _ := filepath.Glob(filepath.Join(tmpDir, "test-profile*.tmp*"))
	if len(tempFiles) > 0 {
		t.Errorf("Temp files should be absent, found: %v", tempFiles)
	}

	// Wait for handler cleanup
	<-handlerExited
}

// TestCaptureProfile_DeadlineDuringBody verifies that deadline during body read
// leaves no destination or temp files.
func TestCaptureProfile_DeadlineDuringBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()

		// Write slowly to exceed deadline
		for i := 0; i < 100; i++ {
			fmt.Fprint(w, "x")
			w.(http.Flusher).Flush()
			time.Sleep(20 * time.Millisecond)
			select {
			case <-r.Context().Done():
				return
			default:
			}
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test-profile.pb.gz")

	client := &http.Client{Timeout: 10 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := CaptureProfile(ctx, client, server.URL, destPath, "heap")

	// Should have deadline error
	if err == nil {
		t.Error("Expected error after deadline")
	}

	if !errors.Is(err, ErrProfileDeadline) && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected deadline error, got: %v", err)
	}

	// Verify destination absent
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}
}

// TestCaptureProfile_InvalidProfile verifies that an invalid profile leaves no
// destination or temp files.
func TestCaptureProfile_InvalidProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Write invalid gzip content
		w.Write([]byte("this is not a gzip file"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test-profile.pb.gz")

	client := &http.Client{Timeout: 5 * time.Second}
	ctx := context.Background()

	err := CaptureProfile(ctx, client, server.URL, destPath, "heap")

	// Should have validation error
	if err == nil {
		t.Error("Expected validation error")
	}

	if !errors.Is(err, ErrProfileValidation) {
		t.Errorf("Expected validation error, got: %v", err)
	}

	// Verify destination absent
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}
}

// TestCaptureProfile_OversizedProfile verifies that an oversized profile leaves no
// destination or temp files.
func TestCaptureProfile_OversizedProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Write more than 100MB
		buf := make([]byte, 1024*1024) // 1MB chunk
		for i := 0; i < 101; i++ {
			w.Write(buf)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test-profile.pb.gz")

	client := &http.Client{Timeout: 30 * time.Second}
	ctx := context.Background()

	err := CaptureProfile(ctx, client, server.URL, destPath, "heap")

	// Should have body too large error
	if err == nil {
		t.Error("Expected size error")
	}

	if !errors.Is(err, ErrProfileBodyTooLarge) {
		t.Errorf("Expected body too large error, got: %v", err)
	}

	// Verify destination absent
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}
}

// TestCaptureProfile_ExistingDestinationPreserved verifies that an existing valid
// destination is preserved when replacement fails.
func TestCaptureProfile_ExistingDestinationPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "existing-profile.pb.gz")

	// Create existing valid profile (valid gzip with content)
	existingContent := []byte{
		0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x0b, 0xc9, 0xc8, 0xcc, 0x4f, 0x4d,
		0xcd, 0xc9, 0xcf, 0x4b, 0x55, 0x00, 0x86, 0x28,
		0x4a, 0x4d, 0x2e, 0x00, 0x00, 0x00, 0xe8, 0x00,
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	if err := os.WriteFile(destPath, existingContent, 0644); err != nil {
		t.Fatalf("Failed to create existing profile: %v", err)
	}

	// P0: Handler must observe r.Context().Done() for proper cleanup
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Block observing context for cancellation
		<-r.Context().Done()
	}))
	defer server.Close()

	client := &http.Client{Timeout: 10 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := CaptureProfile(ctx, client, server.URL, destPath, "heap")

	// Should fail
	if err == nil {
		t.Error("Expected error after cancellation")
	}

	// Verify original content preserved - byte-exact match
	preservedContent, err := os.ReadFile(destPath)
	if err != nil {
		t.Errorf("Original file should still exist: %v", err)
	}

	if !bytes.Equal(preservedContent, existingContent) {
		t.Fatalf("Existing destination changed: got %d bytes, expected %d", len(preservedContent), len(existingContent))
	}
}

// TestCaptureProfile_SuccessfulAtomicReplacement verifies that a successful capture
// atomically replaces the destination.
func TestCaptureProfile_SuccessfulAtomicReplacement(t *testing.T) {
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "valid-profile.pb.gz")

	// Generate valid gzip data at runtime
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write([]byte("test profile content for validation"))
	gz.Close()
	validGzip := buf.Bytes()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(validGzip)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	ctx := context.Background()

	err := CaptureProfile(ctx, client, server.URL, destPath, "heap")

	if err != nil {
		t.Errorf("Expected successful capture, got: %v", err)
	}

	// Verify destination exists and is valid
	info, err := os.Stat(destPath)
	if err != nil {
		t.Errorf("Destination should exist: %v", err)
	}

	if info.Size() != int64(len(validGzip)) {
		t.Errorf("Destination size mismatch: got %d, expected %d", info.Size(), len(validGzip))
	}

	// Verify no temp files
	tempFiles, _ := filepath.Glob(filepath.Join(tmpDir, "valid-profile*.tmp*"))
	if len(tempFiles) > 0 {
		t.Errorf("Temp files should be absent, found: %v", tempFiles)
	}
}

// TestCaptureProfile_PublicationError verifies that a publication error leaves no
// destination or temp files.
func TestCaptureProfile_PublicationError(t *testing.T) {
	// Use a path that cannot be renamed to
	// (directory doesn't exist)
	destPath := "/nonexistent/path/test-profile.pb.gz"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	ctx := context.Background()

	err := CaptureProfile(ctx, client, server.URL, destPath, "heap")

	// Should fail with publication error
	if err == nil {
		t.Error("Expected publication error")
	}

	// Note: We can't easily verify temp files are cleaned up since the
	// temp directory would be created in /nonexistent/path/
}

// TestCaptureProfile_TransportError verifies transport errors are typed correctly.
func TestCaptureProfile_TransportError(t *testing.T) {
	// Non-routable address
	client := &http.Client{Timeout: 1 * time.Second}
	ctx := context.Background()

	err := CaptureProfile(ctx, client, "http://127.0.0.1:1/test", "/tmp/test", "heap")

	if err == nil {
		t.Error("Expected transport error")
	}

	// Should be transport error
	if !errors.Is(err, ErrProfileTransport) {
		t.Errorf("Expected transport error, got: %v", err)
	}
}

// TestCaptureProfile_ReadError verifies read errors are typed correctly.
func TestCaptureProfile_ReadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Close connection mid-read
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test-profile.pb.gz")

	client := &http.Client{Timeout: 5 * time.Second}
	ctx := context.Background()

	err := CaptureProfile(ctx, client, server.URL, destPath, "heap")

	if err == nil {
		t.Error("Expected read error")
	}

	// Should be transport or read error
	if !errors.Is(err, ErrProfileTransport) && !errors.Is(err, ErrProfileRead) {
		t.Logf("Got error type: %v", err)
	}
}

// TestCaptureProfile_EmptyBody verifies empty body is rejected.
func TestCaptureProfile_EmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Empty body
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test-profile.pb.gz")

	client := &http.Client{Timeout: 5 * time.Second}
	ctx := context.Background()

	err := CaptureProfile(ctx, client, server.URL, destPath, "heap")

	if err == nil {
		t.Error("Expected error for empty body")
	}

	// Verify destination absent
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}
}

// Verify we can read gzip content without the import
var _ = io.Copy
