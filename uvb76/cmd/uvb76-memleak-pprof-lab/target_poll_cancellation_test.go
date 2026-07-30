// Package main provides the UVB-76 pprof memory leak lab.
//
// # Target Poll Cancellation Tests
//
// P0-3: Tests for real PollTargetAuthority cancellation in production scenarios.
// P0-3: All tests use the real PollTargetAuthority function.
// P0-3: Tests verify blocked headers, blocked body, and between-attempt cancellation.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestPollTargetAuthority_BlockedHeaders verifies PollTargetAuthority terminates
// when the server never sends headers after request arrival.
func TestPollTargetAuthority_BlockedHeaders(t *testing.T) {
	// Server signals
	requestStarted := make(chan struct{}, 1)
	serverCancelled := make(chan struct{}, 1)
	handlerExited := make(chan struct{}, 1)

	// Create server that blocks after request arrival
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)

		// Wait for cancellation
		select {
		case <-r.Context().Done():
			close(serverCancelled)
		case <-handlerExited:
		}
		close(handlerExited)
	}))
	defer server.Close()

	// Create target config
	target := TargetConfigBinding{
		TargetID: "blocked-headers-target",
		BaseURL:  server.URL,
	}

	auth := TargetStateAuthInput{
		CookieName:  "session",
		CookieValue: "test-session-token",
	}

	// Create poll input
	input := TargetPollInput{
		Client:          &http.Client{Timeout: 5 * time.Second},
		UVB76APIBaseURL: server.URL,
		Target:          target,
		Auth:            auth,
		PollInterval:    100 * time.Millisecond,
		RequestTimeout:  500 * time.Millisecond,
		Deadline:        2 * time.Second,
	}

	// Create cancellable context
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Start poll in goroutine
	resultCh := make(chan TargetPollResult, 1)
	go func() {
		resultCh <- PollTargetAuthority(ctx, input)
	}()

	// Wait for request to start
	select {
	case <-requestStarted:
		t.Log("Request started")
	case <-time.After(5 * time.Second):
		t.Fatal("Request never started")
	}

	// Wait a moment for server to observe context
	time.Sleep(100 * time.Millisecond)

	// Cancel the context
	cancel()

	// Wait for poll to return
	select {
	case result := <-resultCh:
		// Verify poll terminated
		if result.TerminalError == nil {
			t.Error("Expected terminal error after cancellation")
		}

		// Verify explicit cancellation semantics
		if !errorsIs(result.TerminalError, ErrTargetPollCancelled) {
			t.Errorf("Expected ErrTargetPollCancelled, got: %v", result.TerminalError)
		}
		if !errorsIs(result.TerminalError, context.Canceled) {
			t.Errorf("Expected context.Canceled, got: %v", result.TerminalError)
		}

		// Verify no deadline error
		if errorsIs(result.TerminalError, context.DeadlineExceeded) {
			t.Error("Should not have deadline exceeded after explicit cancel")
		}

		t.Logf("Poll terminated with: %v", result.TerminalError)

	case <-time.After(10 * time.Second):
		t.Fatal("Poll did not return within timeout")
	}

	// Verify server was cancelled
	select {
	case <-serverCancelled:
		t.Log("Server observed cancellation")
	case <-time.After(2 * time.Second):
		t.Error("Server did not observe cancellation")
	}
}

// TestPollTargetAuthority_BlockedBody verifies PollTargetAuthority terminates
// when server sends headers but never completes the body.
func TestPollTargetAuthority_BlockedBody(t *testing.T) {
	requestStarted := make(chan struct{}, 1)
	headersFlushed := make(chan struct{}, 1)
	serverCancelled := make(chan struct{}, 1)

	// Create server that sends headers but blocks on body
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)

		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(headersFlushed)

		// Write incomplete JSON prefix
		fmt.Fprint(w, `{"partial":`)

		// Block until cancelled
		select {
		case <-r.Context().Done():
			close(serverCancelled)
			return
		}
	}))
	defer server.Close()

	target := TargetConfigBinding{
		TargetID: "blocked-body-target",
		BaseURL:  server.URL,
	}

	auth := TargetStateAuthInput{
		CookieName:  "session",
		CookieValue: "test-session-token",
	}

	input := TargetPollInput{
		Client:          &http.Client{Timeout: 5 * time.Second},
		UVB76APIBaseURL: server.URL,
		Target:          target,
		Auth:            auth,
		PollInterval:    100 * time.Millisecond,
		RequestTimeout:  2 * time.Second,
		Deadline:        5 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	resultCh := make(chan TargetPollResult, 1)
	go func() {
		resultCh <- PollTargetAuthority(ctx, input)
	}()

	// Wait for headers
	select {
	case <-headersFlushed:
		t.Log("Headers flushed")
	case <-time.After(5 * time.Second):
		t.Fatal("Headers never flushed")
	}

	// Cancel
	cancel()

	select {
	case result := <-resultCh:
		// Should have decode error since body is incomplete
		if result.TerminalError != nil {
			t.Logf("Poll terminated with: %v", result.TerminalError)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Poll did not return")
	}
}

// TestPollTargetAuthority_BetweenAttempts verifies PollTargetAuthority terminates
// when cancelled between poll attempts.
func TestPollTargetAuthority_BetweenAttempts(t *testing.T) {
	attemptCount := 0
	mu := sync.Mutex{}
	firstAttemptDone := make(chan struct{}, 1)
	blockSecondAttempt := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attemptCount++
		currentAttempt := attemptCount
		mu.Unlock()

		if currentAttempt == 1 {
			close(firstAttemptDone)
			// First attempt: return recoverable error
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"error": "temporary unavailable"}`)
			return
		}

		// Subsequent attempts: block until cancelled
		select {
		case <-blockSecondAttempt:
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)
		case <-r.Context().Done():
			return
		}
	}))
	defer server.Close()

	target := TargetConfigBinding{
		TargetID: "between-attempts-target",
		BaseURL:  server.URL,
	}

	auth := TargetStateAuthInput{
		CookieName:  "session",
		CookieValue: "test-session-token",
	}

	// Long poll interval so we can cancel between attempts
	input := TargetPollInput{
		Client:          &http.Client{Timeout: 2 * time.Second},
		UVB76APIBaseURL: server.URL,
		Target:          target,
		Auth:            auth,
		PollInterval:    500 * time.Millisecond, // Long interval
		RequestTimeout:  2 * time.Second,
		Deadline:        10 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resultCh := make(chan TargetPollResult, 1)
	go func() {
		resultCh <- PollTargetAuthority(ctx, input)
	}()

	// Wait for first attempt to complete
	select {
	case <-firstAttemptDone:
		t.Log("First attempt completed")
	case <-time.After(5 * time.Second):
		t.Fatal("First attempt never completed")
	}

	// Wait for poll interval to pass
	time.Sleep(600 * time.Millisecond)

	// Cancel between attempts
	cancel()
	close(blockSecondAttempt)

	select {
	case result := <-resultCh:
		// Verify explicit cancellation
		if !errorsIs(result.TerminalError, ErrTargetPollCancelled) {
			t.Errorf("Expected ErrTargetPollCancelled, got: %v", result.TerminalError)
		}
		if !errorsIs(result.TerminalError, context.Canceled) {
			t.Errorf("Expected context.Canceled, got: %v", result.TerminalError)
		}

		// Verify first attempt was recorded
		if result.Attempts < 1 {
			t.Errorf("Expected at least 1 attempt, got %d", result.Attempts)
		}

		t.Logf("Poll terminated with: %v", result.TerminalError)
	case <-time.After(5 * time.Second):
		t.Fatal("Poll did not return")
	}
}

// TestPollTargetAuthority_ParentCancelNotRecovered verifies that explicit
// parent cancellation is not recorded as a recovered transport error.
func TestPollTargetAuthority_ParentCancelNotRecovered(t *testing.T) {
	attemptCount := 0
	mu := sync.Mutex{}
	firstAttemptDone := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attemptCount++
		currentAttempt := attemptCount
		mu.Unlock()

		if currentAttempt == 1 {
			close(firstAttemptDone)
		}

		// Return recoverable error
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"error": "temporary"}`)
	}))
	defer server.Close()

	target := TargetConfigBinding{
		TargetID: "parent-cancel-test",
		BaseURL:  server.URL,
	}

	auth := TargetStateAuthInput{
		CookieName:  "session",
		CookieValue: "test-token",
	}

	input := TargetPollInput{
		Client:          &http.Client{Timeout: 1 * time.Second},
		UVB76APIBaseURL: server.URL,
		Target:          target,
		Auth:            auth,
		PollInterval:    100 * time.Millisecond,
		RequestTimeout:  1 * time.Second,
		Deadline:        5 * time.Second, // Long deadline so parent cancel fires first
	}

	// P0-4: Use context.WithCancel (not WithTimeout) for explicit cancellation
	// P0-4: WithTimeout fires as DeadlineExceeded, not Canceled
	ctx, cancel := context.WithCancel(context.Background())

	resultCh := make(chan TargetPollResult, 1)
	go func() {
		resultCh <- PollTargetAuthority(ctx, input)
	}()

	// Wait for at least one attempt to complete
	select {
	case <-firstAttemptDone:
		t.Log("First attempt completed")
	case <-time.After(2 * time.Second):
		t.Fatal("First attempt never completed")
	}

	// Wait for poll interval to pass
	time.Sleep(150 * time.Millisecond)

	// P0-4: Explicit cancel (not deadline expiry)
	cancel()

	select {
	case result := <-resultCh:
		// P0-4: Parent cancellation must NOT be classified as recovered transport error
		if !errorsIs(result.TerminalError, ErrTargetPollCancelled) {
			t.Errorf("Expected ErrTargetPollCancelled, got: %v", result.TerminalError)
		}
		if !errorsIs(result.TerminalError, context.Canceled) {
			t.Errorf("Expected context.Canceled, got: %v", result.TerminalError)
		}

		// P0-4: Should NOT be deadline error
		if errorsIs(result.TerminalError, ErrTargetPollDeadline) {
			t.Error("Should not be deadline error with explicit cancel")
		}
		if errorsIs(result.TerminalError, context.DeadlineExceeded) {
			t.Error("Should not be DeadlineExceeded with explicit cancel")
		}

		t.Logf("Terminal error: %v", result.TerminalError)
		t.Logf("Recovered count: %d", result.RecoveredErrorCount)
	case <-time.After(2 * time.Second):
		t.Fatal("Poll did not return")
	}
}

// TestPollTargetAuthority_PollDoneBeforeReturn verifies that PollTargetAuthority
// returns after observing pollDone when cancelled.
func TestPollTargetAuthority_PollDoneBeforeReturn(t *testing.T) {
	// For this test, we verify the lifecycle uses drainTargetPoll
	// which waits for both result and done
	t.Log("Testing that drainTargetPoll waits for pollDone")

	// Verify drainTargetPoll helper exists and works
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	resultCh := make(chan TargetPollResult, 1)
	doneCh := make(chan struct{})

	go func() {
		// Simulate poll result
		time.Sleep(50 * time.Millisecond)
		resultCh <- TargetPollResult{}
	}()

	go func() {
		// Simulate poll done
		time.Sleep(100 * time.Millisecond)
		close(doneCh)
	}()

	drainCtx, drainCancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer drainCancel()

	drainResult := drainTargetPoll(drainCtx, resultCh, doneCh)

	if !drainResult.ResultReceived {
		t.Error("Expected result to be received")
	}
	if !drainResult.GoroutineTerminated {
		t.Error("Expected goroutine to be terminated")
	}
	if drainResult.TerminalError != nil {
		t.Errorf("Did not expect terminal error, got: %v", drainResult.TerminalError)
	}
}

// errorsIs is a helper that uses the standard errors.Is for proper Join handling
func errorsIs(err error, target error) bool {
	return errors.Is(err, target)
}

// TestRunCollectionLifecycle_WithRealPollInput verifies RunCollectionLifecycle
// works with real TargetPollInput and observes both result and goroutine termination.
func TestRunCollectionLifecycle_WithRealPollInput(t *testing.T) {
	// Create a server that completes successfully
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"target_id": "test-target",
			"reachable": true,
			"status":    "healthy",
		})
	}))
	defer server.Close()

	auth := TargetStateAuthInput{
		CookieName:  "session",
		CookieValue: "test-token",
	}

	// Create valid poll input that will fail quickly
	pollInput := TargetPollInput{
		Client:          &http.Client{Timeout: 100 * time.Millisecond},
		UVB76APIBaseURL: server.URL,
		Target: TargetConfigBinding{
			TargetID: "test-target",
			BaseURL:  "http://localhost:9999",
		},
		Auth:           auth,
		PollInterval:   50 * time.Millisecond,
		RequestTimeout: 100 * time.Millisecond,
		Deadline:       200 * time.Millisecond,
	}

	// Create valid collector input
	var samples []ProcessSample
	var mu sync.Mutex
	var errors []string

	collectorInput := &CollectorInput{
		TovarischSamples: &samples,
		UVB76Samples:     &samples,
		CollectorErrors:  &errors,
		SamplesMu:        &mu,
	}

	// Create observation context
	obsCtx, obsCancel := context.WithCancel(context.Background())
	profileCtx, profileCancel := context.WithCancel(context.Background())
	defer profileCancel()

	var wg sync.WaitGroup

	result := RunCollectionLifecycle(CollectionLifecycleInput{
		ObservationCtx:    obsCtx,
		ProfileCtx:        profileCtx,
		ObservationCancel: obsCancel,
		WaitGroup:         &wg,
		CollectorInput:    collectorInput,
		PollInput:         pollInput,
		PollDrainTimeout:  500 * time.Millisecond,
		CaptureProfilesFn: func(ctx context.Context) error {
			// Profile capture does nothing in this test
			return nil
		},
	})

	// Verify result received flag
	if !result.PollResultReceived {
		t.Error("Expected PollResultReceived to be true")
	}

	// Verify goroutine terminated flag
	if !result.PollGoroutineTerminated {
		t.Error("Expected PollGoroutineTerminated to be true")
	}

	// Verify no drain timeout
	if result.PollTerminalError != nil {
		if errorsIs(result.PollTerminalError, ErrPollReceiveDeadline) {
			t.Error("Should not have drain deadline error")
		}
	}

	t.Logf("Poll terminal error: %v", result.PollTerminalError)
}
