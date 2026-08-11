// Package main provides the UVB-76 pprof memory leak lab.
//
// # Profile Body Cancellation Tests
//
// P0-5: Cancellation during actual response-body reading.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"
)

// blockingProfileBody implements io.ReadCloser for deterministic body-read cancellation.
// P0-5: First Read succeeds, second Read blocks until context cancellation.
type blockingProfileBody struct {
	ctx           context.Context
	firstReadDone chan struct{}
	secondRead    chan struct{}
	first         bool
}

func (b *blockingProfileBody) Read(p []byte) (int, error) {
	if !b.first {
		b.first = true
		p[0] = 0x1f // Valid pprof gzip magic byte
		close(b.firstReadDone)
		return 1, nil
	}

	close(b.secondRead)
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

func (b *blockingProfileBody) Close() error {
	return nil
}

// cancellableRoundTripper returns a response with blockingProfileBody.
type cancellableRoundTripper struct {
	body *blockingProfileBody
}

func (rt *cancellableRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 200,
		Body:       rt.body,
		Header:     http.Header{"Content-Type": []string{"application/gzip"}},
	}, nil
}

// TestProfileBodyRead_CancelDuringActualRead verifies cancellation during body read.
// P0-5: First Read completes, second Read blocks until cancellation.
func TestProfileBodyRead_CancelDuringActualRead(t *testing.T) {
	firstReadDone := make(chan struct{})
	secondRead := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())

	body := &blockingProfileBody{
		ctx:           ctx,
		firstReadDone: firstReadDone,
		secondRead:    secondRead,
	}

	transport := &cancellableRoundTripper{body: body}

	client := &http.Client{Transport: transport}

	// Start capture in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- CaptureProfile(ctx, client, "http://test/profile", t.TempDir()+"/heap.pb.gz", "heap")
	}()

	// Wait for first read to complete
	<-firstReadDone

	// Cancel during second read
	cancel()

	// Wait for second read to be blocked and then cancelled
	<-secondRead

	// Wait for capture to complete
	err := <-errCh

	// Verify explicit cancel result
	if err == nil {
		t.Fatal("Expected cancel error")
	}
	if !errors.Is(err, ErrProfileCancelled) {
		t.Errorf("Expected ErrProfileCancelled, got: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled, got: %v", err)
	}

	// Must NOT have deadline
	if errors.Is(err, ErrProfileDeadline) {
		t.Error("Should not have ErrProfileDeadline for explicit cancel")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Error("Should not have context.DeadlineExceeded for explicit cancel")
	}
}

// TestProfileBodyRead_DeadlineDuringActualRead verifies deadline during body read.
func TestProfileBodyRead_DeadlineDuringActualRead(t *testing.T) {
	firstReadDone := make(chan struct{})
	secondRead := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	body := &blockingProfileBody{
		ctx:           ctx,
		firstReadDone: firstReadDone,
		secondRead:    secondRead,
	}

	transport := &cancellableRoundTripper{body: body}

	client := &http.Client{Transport: transport}

	// Start capture in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- CaptureProfile(ctx, client, "http://test/profile", t.TempDir()+"/heap.pb.gz", "heap")
	}()

	// Wait for first read to complete
	<-firstReadDone

	// Wait for second read to be blocked
	<-secondRead

	// Wait for capture to complete
	err := <-errCh

	// Verify deadline result
	if err == nil {
		t.Fatal("Expected deadline error")
	}
	if !errors.Is(err, ErrProfileDeadline) {
		t.Errorf("Expected ErrProfileDeadline, got: %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected context.DeadlineExceeded, got: %v", err)
	}

	// Must NOT have cancel
	if errors.Is(err, ErrProfileCancelled) {
		t.Error("Should not have ErrProfileCancelled for deadline")
	}
	if errors.Is(err, context.Canceled) {
		t.Error("Should not have context.Canceled for deadline")
	}
}

// TestProfileBodyRead_ParentCancelDuringBodyRead verifies parent context cancellation.
func TestProfileBodyRead_ParentCancelDuringBodyRead(t *testing.T) {
	firstReadDone := make(chan struct{})
	secondRead := make(chan struct{})

	parentCtx, parentCancel := context.WithCancel(context.Background())

	// Create a derived deadline context
	ctx, cancel := context.WithTimeout(parentCtx, 10*time.Second)
	defer cancel()

	body := &blockingProfileBody{
		ctx:           ctx,
		firstReadDone: firstReadDone,
		secondRead:    secondRead,
	}

	transport := &cancellableRoundTripper{body: body}

	client := &http.Client{Transport: transport}

	// Start capture in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- CaptureProfile(ctx, client, "http://test/profile", t.TempDir()+"/heap.pb.gz", "heap")
	}()

	// Wait for first read to complete
	<-firstReadDone

	// Cancel parent context
	parentCancel()

	// Wait for second read to be blocked and then cancelled
	<-secondRead

	// Wait for capture to complete
	err := <-errCh

	// Verify explicit cancel result from parent
	if err == nil {
		t.Fatal("Expected cancel error")
	}
	if !errors.Is(err, ErrProfileCancelled) {
		t.Errorf("Expected ErrProfileCancelled, got: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled, got: %v", err)
	}
}

// TestProfileBodyRead_DestinationAbsent verifies destination is absent on cancellation.
func TestProfileBodyRead_DestinationAbsent(t *testing.T) {
	firstReadDone := make(chan struct{})
	secondRead := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())

	body := &blockingProfileBody{
		ctx:           ctx,
		firstReadDone: firstReadDone,
		secondRead:    secondRead,
	}

	transport := &cancellableRoundTripper{body: body}
	client := &http.Client{Transport: transport}

	tmpDir := t.TempDir()
	destPath := tmpDir + "/heap.pb.gz"

	// Start capture in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- CaptureProfile(ctx, client, "http://test/profile", destPath, "heap")
	}()

	// Wait for first read to complete
	<-firstReadDone

	// Cancel during second read
	cancel()

	// Wait for second read to be blocked and then cancelled
	<-secondRead

	// Wait for capture to complete
	<-errCh

	// Verify destination is absent
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}
}
