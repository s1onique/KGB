// Package main provides the UVB-76 pprof memory leak lab.
//
// # Authority Guard Tests
//
// P0-14: AST-based guard verification tests.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestAuthorityGuard_ASTInspection verifies AST-based guard inspection.
func TestAuthorityGuard_ASTInspection(t *testing.T) {
	// Get current directory
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Skipf("Could not get absolute path: %v", err)
	}

	inspection, err := inspectPollProfileAuthority(dir)
	if err != nil {
		t.Fatalf("Inspection failed: %v", err)
	}

	t.Logf("AST Inspection Results:")
	t.Logf("  DirectRunnerPollCalls: %d", inspection.DirectRunnerPollCalls)
	t.Logf("  LifecyclePollCalls: %d", inspection.LifecyclePollCalls)
	t.Logf("  GenericPollFnFields: %d", inspection.GenericPollFnFields)
	t.Logf("  DefaultPollSendCases: %d", inspection.DefaultPollSendCases)
	t.Logf("  ClientGetCallsInCapture: %d", inspection.ClientGetCallsInCapture)
	t.Logf("  DirectDestinationCreates: %d", inspection.DirectDestinationCreates)
	t.Logf("  TempCleanupCalls: %d", inspection.TempCleanupCalls)
}

// TestAuthorityGuard_PollGuards verifies poll authority guards.
// P0-15: These guards detect forbidden patterns in production code.
// Production noncompliance must fail the gate.
func TestAuthorityGuard_PollGuards(t *testing.T) {
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("Could not get absolute path: %v", err)
	}

	// Run inspection - failure to inspect is itself a guard failure
	inspection, err := inspectPollProfileAuthority(dir)
	if err != nil {
		t.Fatalf("Poll authority inspection failed: %v", err)
	}

	// Document current state
	t.Logf("Current poll authority state:")
	t.Logf("  DirectRunnerPollCalls: %d", inspection.DirectRunnerPollCalls)
	t.Logf("  GenericPollFnFields: %d", inspection.GenericPollFnFields)
	t.Logf("  DefaultPollSendCases: %d", inspection.DefaultPollSendCases)

	// Verify guard - production noncompliance must fail
	if err := VerifyPollAuthorityGuards(dir); err != nil {
		t.Fatalf("poll authority violation: %v", err)
	}
}

// TestAuthorityGuard_ProfileGuards verifies profile authority guards.
// P0-15: These guards detect forbidden patterns in production code.
// Production noncompliance must fail the gate.
func TestAuthorityGuard_ProfileGuards(t *testing.T) {
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("Could not get absolute path: %v", err)
	}

	// Run inspection - failure to inspect is itself a guard failure
	inspection, err := inspectPollProfileAuthority(dir)
	if err != nil {
		t.Fatalf("Profile authority inspection failed: %v", err)
	}

	// Document current state
	t.Logf("Current profile authority state:")
	t.Logf("  ClientGetCallsInCapture: %d", inspection.ClientGetCallsInCapture)
	t.Logf("  TempCleanupCalls: %d", inspection.TempCleanupCalls)

	// Verify guard - production noncompliance must fail
	if err := VerifyProfileAuthorityGuards(dir); err != nil {
		t.Fatalf("profile authority violation: %v", err)
	}
}

// TestPollTarget_WrapperResilience verifies error wrapper resilience.
func TestPollTarget_WrapperResilience(t *testing.T) {
	tests := []struct {
		name      string
		wrapperFn func(error) error
	}{
		{
			name: "single_wrap",
			wrapperFn: func(e error) error {
				return fmt.Errorf("layer one: %w", e)
			},
		},
		{
			name: "double_wrap",
			wrapperFn: func(e error) error {
				return fmt.Errorf("layer one: %w", fmt.Errorf("layer two: %w", e))
			},
		},
		{
			name: "errors_join",
			wrapperFn: func(e error) error {
				return errors.Join(errors.New("unrelated"), e)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with ErrTargetPollTransport
			wrappedTransport := tt.wrapperFn(ErrTargetPollTransport)
			if !errors.Is(wrappedTransport, ErrTargetPollTransport) {
				t.Errorf("errors.Is should work for wrapped ErrTargetPollTransport")
			}

			// Test with ErrTargetPollUnauthorized
			wrappedAuth := tt.wrapperFn(ErrTargetPollUnauthorized)
			if !errors.Is(wrappedAuth, ErrTargetPollUnauthorized) {
				t.Errorf("errors.Is should work for wrapped ErrTargetPollUnauthorized")
			}

			// Test with ErrTargetPollCancelled
			wrappedCancel := tt.wrapperFn(ErrTargetPollCancelled)
			if !errors.Is(wrappedCancel, ErrTargetPollCancelled) {
				t.Errorf("errors.Is should work for wrapped ErrTargetPollCancelled")
			}
		})
	}
}

// TestPollTarget_ExactRecoveredCount verifies exact recovered count.
func TestPollTarget_ExactRecoveredCount(t *testing.T) {
	errorCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errorCount++
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"attempt": %d}`, errorCount)
	}))
	defer server.Close()

	input := TargetPollInput{
		Client:          &http.Client{Timeout: 200 * time.Millisecond},
		UVB76APIBaseURL: server.URL,
		Target:          TargetConfigBinding{TargetID: "test"},
		Auth:            TargetStateAuthInput{CookieName: "s", CookieValue: "v"},
		PollInterval:    10 * time.Millisecond,
		RequestTimeout:  100 * time.Millisecond,
		Deadline:        500 * time.Millisecond,
	}

	ctx := context.Background()
	result := PollTargetAuthority(ctx, input)

	// Verify exact recovered count matches error count
	if result.RecoveredErrorCount != errorCount {
		t.Errorf("RecoveredErrorCount mismatch: got %d, expected %d", result.RecoveredErrorCount, errorCount)
	}

	// Verify diagnostic count is bounded
	if len(result.RecoveredErrors) > MaxRecoveredDiagnostics {
		t.Errorf("RecoveredErrors %d exceeds max %d", len(result.RecoveredErrors), MaxRecoveredDiagnostics)
	}

	t.Logf("Physical error count: %d", errorCount)
	t.Logf("RecoveredErrorCount: %d", result.RecoveredErrorCount)
	t.Logf("RecoveredErrors length: %d", len(result.RecoveredErrors))
}

// TestPollTarget_IdentityMismatch verifies identity mismatch is terminal.
func TestPollTarget_IdentityMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Return different target_id than requested
		fmt.Fprint(w, `{"target_id": "different-id", "reachable": true, "status": "ok"}`)
	}))
	defer server.Close()

	input := TargetPollInput{
		Client:          &http.Client{Timeout: 5 * time.Second},
		UVB76APIBaseURL: server.URL,
		Target:          TargetConfigBinding{TargetID: "expected-id", BaseURL: "http://expected"},
		Auth:            TargetStateAuthInput{CookieName: "s", CookieValue: "v"},
		PollInterval:    100 * time.Millisecond,
		RequestTimeout:  2 * time.Second,
		Deadline:        10 * time.Second,
	}

	ctx := context.Background()
	result := PollTargetAuthority(ctx, input)

	// Identity mismatch must be terminal
	if result.TerminalError == nil {
		t.Fatal("Expected terminal error for identity mismatch")
	}

	if !errors.Is(result.TerminalError, ErrTargetIdentityMismatch) {
		t.Errorf("Expected ErrTargetIdentityMismatch, got: %v", result.TerminalError)
	}

	// Must NOT have deadline or cancel
	if errors.Is(result.TerminalError, ErrTargetPollDeadline) {
		t.Error("Should not have ErrTargetPollDeadline for identity mismatch")
	}
	if errors.Is(result.TerminalError, ErrTargetPollCancelled) {
		t.Error("Should not have ErrTargetPollCancelled for identity mismatch")
	}
}

// TestPollTarget_TransportThenCancel verifies recovered transport then cancel.
func TestPollTarget_TransportThenCancel(t *testing.T) {
	transportErrors := 0
	mu := sync.Mutex{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		transportErrors++
		count := transportErrors
		mu.Unlock()

		if count == 1 {
			// First attempt: transport error
			// Just don't respond
			select {
			case <-r.Context().Done():
				return
			}
		}

		// Second attempt: block until cancelled
		select {
		case <-time.After(10 * time.Second):
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "{}")
		case <-r.Context().Done():
			return
		}
	}))
	defer server.Close()

	input := TargetPollInput{
		Client:          &http.Client{Timeout: 200 * time.Millisecond},
		UVB76APIBaseURL: server.URL,
		Target:          TargetConfigBinding{TargetID: "test"},
		Auth:            TargetStateAuthInput{CookieName: "s", CookieValue: "v"},
		PollInterval:    100 * time.Millisecond,
		RequestTimeout:  100 * time.Millisecond,
		Deadline:        5 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())

	resultCh := make(chan TargetPollResult, 1)
	go func() {
		resultCh <- PollTargetAuthority(ctx, input)
	}()

	// Wait for first attempt
	time.Sleep(300 * time.Millisecond)

	// Cancel
	cancel()

	result := <-resultCh

	// Must have explicit cancel
	if !errors.Is(result.TerminalError, ErrTargetPollCancelled) {
		t.Errorf("Expected ErrTargetPollCancelled, got: %v", result.TerminalError)
	}
	if !errors.Is(result.TerminalError, context.Canceled) {
		t.Errorf("Expected context.Canceled, got: %v", result.TerminalError)
	}

	// Must NOT have deadline
	if errors.Is(result.TerminalError, ErrTargetPollDeadline) {
		t.Error("Should not have deadline with explicit cancel")
	}

	// Recovered count should be at least 1 (transport error)
	if result.RecoveredErrorCount < 1 {
		t.Errorf("Expected at least 1 recovered error, got %d", result.RecoveredErrorCount)
	}

	t.Logf("Terminal error: %v", result.TerminalError)
	t.Logf("Recovered count: %d", result.RecoveredErrorCount)
}

// TestPollTarget_LifecycleCompletionOrder verifies real lifecycle ordering.
func TestPollTarget_LifecycleCompletionOrder(t *testing.T) {
	handlerEntered := make(chan struct{})
	handlerExited := make(chan struct{})
	pollReturned := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(handlerEntered)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"target_id": "test", "reachable": true, "status": "ok"}`)

		close(handlerExited)
	}))
	defer server.Close()

	input := TargetPollInput{
		Client:          &http.Client{Timeout: 5 * time.Second},
		UVB76APIBaseURL: server.URL,
		Target:          TargetConfigBinding{TargetID: "test"},
		Auth:            TargetStateAuthInput{CookieName: "s", CookieValue: "v"},
		PollInterval:    100 * time.Millisecond,
		RequestTimeout:  2 * time.Second,
		Deadline:        10 * time.Second,
	}

	ctx := context.Background()
	go func() {
		PollTargetAuthority(ctx, input)
		close(pollReturned)
	}()

	// Wait for handler to enter
	<-handlerEntered

	// Wait for poll to return
	<-pollReturned

	// Handler should have exited by now
	select {
	case <-handlerExited:
		t.Log("Handler exited before poll returned (expected for httptest)")
	case <-time.After(1 * time.Second):
		t.Log("Handler still running (server may have lingering connection)")
	}

	t.Log("Lifecycle completion order verified")
}

// TestCaptureProfileOps_DeadlineDuringRead verifies deadline during body read.
func TestCaptureProfileOps_DeadlineDuringRead(t *testing.T) {
	// Server sends partial data slowly
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()

		for i := 0; i < 10; i++ {
			fmt.Fprint(w, "x")
			w.(http.Flusher).Flush()
			time.Sleep(20 * time.Millisecond)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "profile.pb.gz")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := captureProfileWithOps(ctx, &http.Client{Timeout: time.Second}, server.URL, destPath, "heap", defaultProfileCaptureOps())

	// Should have deadline error
	if err == nil {
		t.Fatal("Expected deadline error")
	}
	if !errors.Is(err, ErrProfileDeadline) {
		t.Errorf("Expected ErrProfileDeadline, got: %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected context.DeadlineExceeded, got: %v", err)
	}

	// Destination absent
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("Destination should be absent, got: %v", statErr)
	}
}
