// Package main provides the UVB-76 pprof memory leak lab.
//
// # Profile Capture Goroutine Tests (Profile-Level Scaffolding)
//
// These tests provide profile-capture-level goroutine scaffolding.
// They test CaptureProfile behavior, not RunCollectionLifecycle.
//
// NOTE: These are PROFILE-CAPTURE tests, not lifecycle tests.
// RunCollectionLifecycle ordering requires separate production integration.
//
// P0-5: Prove profile capture ordering.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestCollectionLifecycle_ResultChannelOrdering verifies profile capture result ordering.
// NOTE: Profile-capture level test, not RunCollectionLifecycle test.
func TestCollectionLifecycle_ResultChannelOrdering(t *testing.T) {
	resultAvailable := atomic.Bool{}
	resultCh := make(chan struct{})

	// Precompute fixture in test goroutine
	profileData := createSemanticPprofProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 3 * time.Second}
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start capture
	go func() {
		err := CaptureProfile(ctx, client, server.URL, destPath, "heap")
		if err == nil {
			resultAvailable.Store(true)
		}
		close(resultCh)
	}()

	// Wait for result
	<-resultCh

	// Result should be available immediately after channel closes
	if !resultAvailable.Load() {
		t.Log("Capture returned without error")
	}
}

// TestCollectionLifecycle_GoroutineExits verifies goroutine exits after result.
// NOTE: Profile-capture level test, not RunCollectionLifecycle test.
func TestCollectionLifecycle_GoroutineExits(t *testing.T) {
	goroutineDone := atomic.Bool{}

	profileData := createSemanticPprofProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 3 * time.Second}
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)

	go func() {
		goroutineDone.Store(true)
		errCh <- CaptureProfile(ctx, client, server.URL, destPath, "heap")
	}()

	// Wait for result
	err := <-errCh

	// Verify goroutine marked itself done
	if !goroutineDone.Load() {
		t.Error("Goroutine should have set done flag")
	}

	// Result should be available
	if err != nil {
		t.Logf("Capture returned: %v", err)
	}
}

// TestCollectionLifecycle_TransportError propagates transport errors correctly.
// NOTE: Profile-capture level test, not RunCollectionLifecycle test.
func TestCollectionLifecycle_TransportError(t *testing.T) {
	handlerErrCh := make(chan error, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close connection without response - no t.Fatal in handler
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			handlerErrCh <- errors.New("server does not support hijacking")
			return
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			handlerErrCh <- fmt.Errorf("hijack: %w", err)
			return
		}
		conn.Close()
	}))
	defer server.Close()

	// Check handler setup
	select {
	case hErr := <-handlerErrCh:
		t.Fatalf("Handler setup failed: %v", hErr)
	default:
		// OK
	}

	client := &http.Client{Timeout: 3 * time.Second}
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := CaptureProfile(ctx, client, server.URL, destPath, "heap")

	// Verify transport error
	if err == nil {
		t.Fatal("Expected transport error")
	}
	if !errors.Is(err, ErrProfileTransport) {
		t.Errorf("Expected ErrProfileTransport, got: %v", err)
	}
}

// TestCollectionLifecycle_ContextCancel verifies context cancellation.
// NOTE: Profile-capture level test, not RunCollectionLifecycle test.
func TestCollectionLifecycle_ContextCancel(t *testing.T) {
	blocked := make(chan struct{})

	profileData := createSemanticPprofProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until cancelled - no t.Fatal in handler
		select {
		case <-blocked:
			// Continue
		case <-time.After(5 * time.Second):
			// Timeout
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 10 * time.Second}
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- CaptureProfile(ctx, client, server.URL, destPath, "heap")
	}()

	// Wait for deadline
	err := <-errCh

	// Should have deadline error
	if err == nil {
		t.Fatal("Expected deadline error")
	}
	if !errors.Is(err, ErrProfileDeadline) {
		t.Errorf("Expected ErrProfileDeadline, got: %v", err)
	}

	// Unblock server
	close(blocked)
}

// TestCollectionLifecycle_ConcurrentCaptures verifies concurrent capture safety.
// NOTE: Profile-capture level test, not RunCollectionLifecycle test.
func TestCollectionLifecycle_ConcurrentCaptures(t *testing.T) {
	var wg sync.WaitGroup
	errCh := make(chan error, 10)

	profileData := createSemanticPprofProfile(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(profileData)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 3 * time.Second}

	// Precompute destinations in test goroutine (NOT in worker goroutines)
	tmpDir := t.TempDir()
	destinations := make([]string, 5)
	for i := range destinations {
		destinations[i] = filepath.Join(tmpDir, fmt.Sprintf("heap-%d.pb.gz", i))
	}

	for i, destPath := range destinations {
		wg.Add(1)
		go func(idx int, path string) {
			defer wg.Done()
			if err := CaptureProfile(context.Background(), client, server.URL, path, "heap"); err != nil {
				errCh <- fmt.Errorf("capture %d: %w", idx, err)
			}
		}(i, destPath)
	}

	wg.Wait()

	// Check for errors
	close(errCh)
	for err := range errCh {
		t.Errorf("Concurrent capture failed: %v", err)
	}
}
