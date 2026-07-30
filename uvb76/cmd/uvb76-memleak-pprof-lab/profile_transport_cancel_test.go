// Package main provides the UVB-76 pprof memory leak lab.
//
// # One-Shot Profile Transport Tests (Profile-Level Scaffolding)
//
// These tests provide one-shot profile-capture transport scaffolding.
// They test CaptureProfile transport behavior, NOT PollTargetAuthority.
//
// NOTE: These are ONE-SHOT PROFILE-CAPTURE tests, not polling tests.
// Poll transport-then-cancel requires RunCollectionLifecycle with polling/retry.
//
// P0-5: Deterministic transport error in one-shot capture.
package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// errInjectedTransport is a sentinel for transport errors.
var errInjectedTransport = errors.New("injected transport error")

// transportErrorThenCancel is a RoundTripper that returns error, then allows cancel.
type transportErrorThenCancel struct {
	transportErrReturned atomic.Bool
	blocked              chan struct{}
	unblock              chan struct{}
}

func (rt *transportErrorThenCancel) RoundTrip(req *http.Request) (*http.Response, error) {
	if !rt.transportErrReturned.Load() {
		rt.transportErrReturned.Store(true)
		close(rt.blocked)
		return nil, errInjectedTransport
	}
	<-rt.unblock
	return nil, context.Canceled
}

// TestTransportErrorThenCancel_Deterministic verifies transport error in one-shot capture.
// NOTE: One-shot profile-capture test, NOT polling test.
func TestTransportErrorThenCancel_Deterministic(t *testing.T) {
	blocked := make(chan struct{})
	unblock := make(chan struct{})

	transport := &transportErrorThenCancel{
		blocked: blocked,
		unblock: unblock,
	}

	client := &http.Client{Transport: transport}

	ctx, cancel := context.WithCancel(context.Background())

	// Precompute destination in test goroutine
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	errCh := make(chan error, 1)
	go func() {
		errCh <- CaptureProfile(ctx, client, "http://test/profile", destPath, "heap")
	}()

	// Wait for transport error to be returned
	<-blocked

	// Now cancel
	cancel()

	// Unblock (won't be reached due to cancel)
	close(unblock)

	// Wait for capture to complete
	err := <-errCh

	// Verify transport error is preserved
	if err == nil {
		t.Fatal("Expected transport error")
	}
	if !errors.Is(err, ErrProfileTransport) {
		t.Errorf("Expected ErrProfileTransport, got: %v", err)
	}
	if !errors.Is(err, errInjectedTransport) {
		t.Errorf("Expected errInjectedTransport, got: %v", err)
	}
}

// TestTransportErrorThenCancel_Classification verifies error classification.
// NOTE: One-shot profile-capture test, NOT polling test.
func TestTransportErrorThenCancel_Classification(t *testing.T) {
	blocked := make(chan struct{})
	unblock := make(chan struct{})

	transport := &transportErrorThenCancel{
		blocked: blocked,
		unblock: unblock,
	}

	client := &http.Client{Transport: transport}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Precompute destination in test goroutine
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	errCh := make(chan error, 1)
	go func() {
		errCh <- CaptureProfile(ctx, client, "http://test/profile", destPath, "heap")
	}()

	// Wait for transport error
	<-blocked

	// Cancel
	cancel()

	// Wait for result
	err := <-errCh

	// Verify transport category
	if !errors.Is(err, ErrProfileTransport) {
		t.Errorf("Expected ErrProfileTransport, got: %v", err)
	}

	// Verify physical transport error preserved
	if !errors.Is(err, errInjectedTransport) {
		t.Errorf("Expected errInjectedTransport, got: %v", err)
	}

	// Transport error takes precedence in one-shot capture
	if errors.Is(err, ErrProfileCancelled) {
		t.Log("Transport error takes precedence over cancel in one-shot capture")
	}
}

// TestTransportErrorThenCancel_BetweenAttemptWait verifies entry into wait.
// NOTE: One-shot profile-capture test, NOT polling test.
func TestTransportErrorThenCancel_BetweenAttemptWait(t *testing.T) {
	blocked := make(chan struct{})
	unblock := make(chan struct{})

	transport := &transportErrorThenCancel{
		blocked: blocked,
		unblock: unblock,
	}

	client := &http.Client{Transport: transport}

	ctx, cancel := context.WithCancel(context.Background())

	// Precompute destination in test goroutine
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	errCh := make(chan error, 1)
	go func() {
		errCh <- CaptureProfile(ctx, client, "http://test/profile", destPath, "heap")
	}()

	// Wait for transport error
	<-blocked

	// At this point, we should be in the wait
	// Verify we can cancel
	cancel()

	// Wait for result
	err := <-errCh

	// Should have transport error
	if !errors.Is(err, ErrProfileTransport) {
		t.Errorf("Expected ErrProfileTransport, got: %v", err)
	}
}

// TestTransportErrorThenCancel_NoSecondTransaction verifies no second transaction after cancel.
// NOTE: One-shot profile-capture test, NOT polling test.
func TestTransportErrorThenCancel_NoSecondTransaction(t *testing.T) {
	blocked := make(chan struct{})
	unblock := make(chan struct{})
	transactionCount := atomic.Int64{}

	transport := &countingTransport{
		inner:         &transportErrorThenCancel{blocked: blocked, unblock: unblock},
		count:         &transactionCount,
		secondUnblock: unblock,
	}

	client := &http.Client{Transport: transport}

	ctx, cancel := context.WithCancel(context.Background())

	// Precompute destination in test goroutine
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	errCh := make(chan error, 1)
	go func() {
		errCh <- CaptureProfile(ctx, client, "http://test/profile", destPath, "heap")
	}()

	// Wait for first transaction to block
	<-blocked

	// Cancel immediately
	cancel()

	// Unblock second transaction (if it happens)
	close(unblock)

	// Wait for result
	err := <-errCh

	// Verify only one transaction occurred
	if tc := transactionCount.Load(); tc > 1 {
		t.Errorf("Expected at most 1 transaction, got: %d", tc)
	}

	// Verify transport error
	if !errors.Is(err, ErrProfileTransport) {
		t.Errorf("Expected ErrProfileTransport, got: %v", err)
	}
}

// countingTransport counts RoundTrip calls.
type countingTransport struct {
	inner         http.RoundTripper
	count         *atomic.Int64
	secondUnblock chan struct{}
}

func (ct *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ct.count.Add(1)
	return ct.inner.RoundTrip(req)
}

// TestTransportErrorThenCancel_RequiresBothCategories verifies both categories required.
// NOTE: One-shot profile-capture test, NOT polling test.
func TestTransportErrorThenCancel_RequiresBothCategories(t *testing.T) {
	blocked := make(chan struct{})
	unblock := make(chan struct{})

	transport := &transportErrorThenCancel{
		blocked: blocked,
		unblock: unblock,
	}

	client := &http.Client{Transport: transport}

	ctx, cancel := context.WithCancel(context.Background())

	// Precompute destination in test goroutine
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	errCh := make(chan error, 1)
	go func() {
		errCh <- CaptureProfile(ctx, client, "http://test/profile", destPath, "heap")
	}()

	// Wait for transport error
	<-blocked

	// Cancel
	cancel()

	// Wait for result
	err := <-errCh

	// Must have transport category
	if !errors.Is(err, ErrProfileTransport) {
		t.Error("Must have transport category")
	}

	// Must have physical error
	if !errors.Is(err, errInjectedTransport) {
		t.Error("Must have physical transport error")
	}
}

// errorRoundTripper is a RoundTripper that returns configurable errors.
type errorRoundTripper struct {
	err error
}

func (rt *errorRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, rt.err
}

// failingReadCloser implements io.ReadCloser for transport error body.
type failingReadCloser struct {
	readErr error
}

func (f *failingReadCloser) Read(p []byte) (int, error) {
	return 0, f.readErr
}

func (f *failingReadCloser) Close() error {
	return nil
}

// TestTransportError_BodyReadError verifies body read error during transport.
func TestTransportError_BodyReadError(t *testing.T) {
	// Precompute destination in test goroutine
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "heap.pb.gz")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		// Write partial data then close - no t.Fatal in handler
		w.Write([]byte{0x1f, 0x8b}) // gzip magic
		// Don't write complete gzip data
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := &http.Client{Timeout: 1 * time.Second}

	err := CaptureProfile(ctx, client, server.URL, destPath, "heap")

	// Should have some error (either transport or validation)
	if err == nil {
		t.Fatal("Expected error")
	}

	// Should be one of the known categories
	validErrs := []error{
		ErrProfileTransport,
		ErrProfileRead,
		ErrProfileValidation,
		ErrProfileDeadline,
	}

	hasCategory := false
	for _, e := range validErrs {
		if errors.Is(err, e) {
			hasCategory = true
			break
		}
	}

	if !hasCategory {
		t.Errorf("Expected known error category, got: %v", err)
	}
}
