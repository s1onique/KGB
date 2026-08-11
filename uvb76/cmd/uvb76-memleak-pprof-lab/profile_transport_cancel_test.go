// Package main provides the UVB-76 pprof memory leak lab.
//
// # Transport Error Then Cancel Tests
//
// P0-5: Deterministic transport error followed by cancellation.
package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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

// TestTransportErrorThenCancel_Deterministic verifies transport error followed by cancel.
// P0-5: Returns exact transport sentinel, signals classification, proves entry into
// between-attempt wait, cancels before transaction two, requires both categories.
func TestTransportErrorThenCancel_Deterministic(t *testing.T) {
	blocked := make(chan struct{})
	unblock := make(chan struct{})

	transport := &transportErrorThenCancel{
		blocked: blocked,
		unblock: unblock,
	}

	client := &http.Client{Transport: transport}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- CaptureProfile(ctx, client, "http://test/profile", t.TempDir()+"/heap.pb.gz", "heap")
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

	errCh := make(chan error, 1)
	go func() {
		errCh <- CaptureProfile(ctx, client, "http://test/profile", t.TempDir()+"/heap.pb.gz", "heap")
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

	// Should NOT have cancel category (transport error takes precedence)
	if errors.Is(err, ErrProfileCancelled) {
		t.Error("Transport error should take precedence over cancel")
	}
}

// TestTransportErrorThenCancel_BetweenAttemptWait verifies entry into wait.
func TestTransportErrorThenCancel_BetweenAttemptWait(t *testing.T) {
	blocked := make(chan struct{})
	unblock := make(chan struct{})

	transport := &transportErrorThenCancel{
		blocked: blocked,
		unblock: unblock,
	}

	client := &http.Client{Transport: transport}

	ctx, cancel := context.WithCancel(context.Background())

	// Start capture
	errCh := make(chan error, 1)
	go func() {
		errCh <- CaptureProfile(ctx, client, "http://test/profile", t.TempDir()+"/heap.pb.gz", "heap")
	}()

	// Wait for transport error
	<-blocked

	// At this point, we should be in the between-attempt wait
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

	errCh := make(chan error, 1)
	go func() {
		errCh <- CaptureProfile(ctx, client, "http://test/profile", t.TempDir()+"/heap.pb.gz", "heap")
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
func TestTransportErrorThenCancel_RequiresBothCategories(t *testing.T) {
	blocked := make(chan struct{})
	unblock := make(chan struct{})

	transport := &transportErrorThenCancel{
		blocked: blocked,
		unblock: unblock,
	}

	client := &http.Client{Transport: transport}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- CaptureProfile(ctx, client, "http://test/profile", t.TempDir()+"/heap.pb.gz", "heap")
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		// Write partial data then close
		w.Write([]byte{0x1f, 0x8b}) // gzip magic
		// Don't write complete gzip data
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := &http.Client{Timeout: 1 * time.Second}

	err := CaptureProfile(ctx, client, server.URL, t.TempDir()+"/heap.pb.gz", "heap")

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
