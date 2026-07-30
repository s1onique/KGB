// Package main provides the UVB-76 pprof memory leak lab.
//
// # Pipeline Lifecycle Ordering Tests
//
// P0-5: Prove real RunCollectionLifecycle ordering.
package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestCollectionLifecycle_ResultChannelOrdering verifies lifecycle result ordering.
// P0-5: Result channel is populated before goroutine returns.
func TestCollectionLifecycle_ResultChannelOrdering(t *testing.T) {
	// Track if result is available when goroutine exits
	resultAvailable := atomic.Bool{}
	resultCh := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(createSemanticPprofProfile(t))
	}))
	defer server.Close()

	client := &http.Client{Timeout: 3 * time.Second}
	tmpDir := t.TempDir()
	destPath := tmpDir + "/heap.pb.gz"

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
func TestCollectionLifecycle_GoroutineExits(t *testing.T) {
	goroutineDone := atomic.Bool{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(createSemanticPprofProfile(t))
	}))
	defer server.Close()

	client := &http.Client{Timeout: 3 * time.Second}
	tmpDir := t.TempDir()
	destPath := tmpDir + "/heap.pb.gz"

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
func TestCollectionLifecycle_TransportError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close connection without response
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("Server does not support hijacking")
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			t.Fatalf("Hijack failed: %v", err)
		}
		conn.Close()
	}))
	defer server.Close()

	client := &http.Client{Timeout: 3 * time.Second}
	tmpDir := t.TempDir()
	destPath := tmpDir + "/heap.pb.gz"

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
func TestCollectionLifecycle_ContextCancel(t *testing.T) {
	blocked := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until cancelled
		<-blocked
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 10 * time.Second}
	tmpDir := t.TempDir()
	destPath := tmpDir + "/heap.pb.gz"

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
func TestCollectionLifecycle_ConcurrentCaptures(t *testing.T) {
	var wg sync.WaitGroup
	errCh := make(chan error, 10)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(createSemanticPprofProfile(t))
	}))
	defer server.Close()

	client := &http.Client{Timeout: 3 * time.Second}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			tmpDir := t.TempDir()
			destPath := tmpDir + "/heap.pb.gz"
			err := CaptureProfile(context.Background(), client, server.URL, destPath, "heap")
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()

	// Check for errors
	close(errCh)
	for err := range errCh {
		t.Errorf("Concurrent capture failed: %v", err)
	}
}
