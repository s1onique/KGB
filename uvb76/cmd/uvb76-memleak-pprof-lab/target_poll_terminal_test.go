// Package main provides the UVB-76 pprof memory leak lab.
//
// # Target Poll Terminal Error Matrix Tests
//
// P0-6: Complete errors.Is matrix for all semantic poll terminal outcomes.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestPollTarget_TerminalErrorMatrix tests the complete errors.Is matrix for all terminal
// scenarios. Each scenario must expose the required identities and must NOT expose
// the forbidden identities.
func TestPollTarget_TerminalErrorMatrix(t *testing.T) {
	tests := []struct {
		name        string
		scenario    string
		setupServer func() *httptest.Server
		pollInput   TargetPollInput
		ctx         (func() (context.Context, context.CancelFunc))
		required    []error // Must be errors.Is true
		forbidden   []error // Must be errors.Is false
	}{
		{
			name:     "explicit_cancel_before_first_attempt",
			scenario: "cancel_context_immediately",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(10 * time.Second)
					w.WriteHeader(http.StatusOK)
					fmt.Fprint(w, "{}")
				}))
			},
			pollInput: func() TargetPollInput {
				return TargetPollInput{
					Client:          &http.Client{Timeout: 10 * time.Second},
					UVB76APIBaseURL: "",
					Target:          TargetConfigBinding{TargetID: "test"},
					Auth:            TargetStateAuthInput{CookieName: "s", CookieValue: "v"},
					PollInterval:    100 * time.Millisecond,
					RequestTimeout:  1 * time.Second,
					Deadline:        10 * time.Second,
				}
			}(),
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			},
			required:  []error{ErrTargetPollCancelled, context.Canceled},
			forbidden: []error{ErrTargetPollDeadline, context.DeadlineExceeded},
		},
		{
			name:     "explicit_cancel_after_recoverable_error",
			scenario: "cancel_between_attempts_after_transport",
			setupServer: func() *httptest.Server {
				attempt := 0
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					attempt++
					if attempt == 1 {
						// First attempt: recoverable
						w.WriteHeader(http.StatusServiceUnavailable)
						fmt.Fprint(w, `{"error": "temp"}`)
					} else {
						time.Sleep(10 * time.Second)
						w.WriteHeader(http.StatusOK)
					}
				}))
			},
			pollInput: func() TargetPollInput {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
				srv.Close()
				return TargetPollInput{
					Client:          &http.Client{Timeout: 1 * time.Second},
					UVB76APIBaseURL: srv.URL,
					Target:          TargetConfigBinding{TargetID: "test"},
					Auth:            TargetStateAuthInput{CookieName: "s", CookieValue: "v"},
					PollInterval:    200 * time.Millisecond,
					RequestTimeout:  500 * time.Millisecond,
					Deadline:        10 * time.Second,
				}
			}(),
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			},
			required:  []error{ErrTargetPollCancelled, context.Canceled},
			forbidden: []error{ErrTargetPollDeadline, context.DeadlineExceeded},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "explicit_cancel_before_first_attempt" {
				// Skip setupServer for this case - no server needed
				ctx, cancel := tt.ctx()
				defer cancel()

				// Cancel immediately
				time.Sleep(50 * time.Millisecond)
				cancel()

				// Create input with non-routable URL
				input := TargetPollInput{
					Client:          &http.Client{Timeout: 5 * time.Second},
					UVB76APIBaseURL: "http://127.0.0.1:1",
					Target:          TargetConfigBinding{TargetID: "test"},
					Auth:            TargetStateAuthInput{CookieName: "s", CookieValue: "v"},
					PollInterval:    100 * time.Millisecond,
					RequestTimeout:  500 * time.Millisecond,
					Deadline:        2 * time.Second,
				}

				result := PollTargetAuthority(ctx, input)

				// Verify required identities
				for _, req := range tt.required {
					if !errors.Is(result.TerminalError, req) {
						t.Errorf("Required identity %v not found in %v", req, result.TerminalError)
					}
				}

				// Verify forbidden identities
				for _, forbid := range tt.forbidden {
					if errors.Is(result.TerminalError, forbid) {
						t.Errorf("Forbidden identity %v found in %v", forbid, result.TerminalError)
					}
				}
			}

			if tt.name == "explicit_cancel_after_recoverable_error" {
				// Create server
				server := tt.setupServer()
				defer server.Close()

				input := TargetPollInput{
					Client:          &http.Client{Timeout: 1 * time.Second},
					UVB76APIBaseURL: server.URL,
					Target:          TargetConfigBinding{TargetID: "test"},
					Auth:            TargetStateAuthInput{CookieName: "s", CookieValue: "v"},
					PollInterval:    200 * time.Millisecond,
					RequestTimeout:  500 * time.Millisecond,
					Deadline:        5 * time.Second,
				}

				// P0-4: Use WithCancel (not WithTimeout) for explicit cancel
				// P0-4: WithTimeout fires as DeadlineExceeded
				ctx, cancel := context.WithCancel(context.Background())

				// Run poll in goroutine so we can cancel between attempts
				resultCh := make(chan TargetPollResult, 1)
				go func() {
					resultCh <- PollTargetAuthority(ctx, input)
				}()

				// Wait for first attempt to complete
				time.Sleep(300 * time.Millisecond)

				// Cancel explicitly
				cancel()

				result := <-resultCh

				// Verify required identities
				for _, req := range tt.required {
					if !errors.Is(result.TerminalError, req) {
						t.Errorf("Required identity %v not found in %v", req, result.TerminalError)
					}
				}

				// Verify forbidden identities
				for _, forbid := range tt.forbidden {
					if errors.Is(result.TerminalError, forbid) {
						t.Errorf("Forbidden identity %v found in %v", forbid, result.TerminalError)
					}
				}
			}
		})
	}
}

// TestPollTarget_ImmediateTerminalErrors tests that immediate terminal errors do not
// gain false deadline or cancellation categories.
func TestPollTarget_ImmediateTerminalErrors(t *testing.T) {
	tests := []struct {
		name      string
		handler   http.HandlerFunc
		forbidden []error // Must NOT be errors.Is true
	}{
		{
			name: "unauthorized",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			forbidden: []error{
				ErrTargetPollDeadline,
				ErrTargetPollCancelled,
				context.DeadlineExceeded,
				context.Canceled,
			},
		},
		{
			name: "forbidden",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			forbidden: []error{
				ErrTargetPollDeadline,
				ErrTargetPollCancelled,
				context.DeadlineExceeded,
				context.Canceled,
			},
		},
		{
			name: "decode_failure",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, `{invalid json}`)
			},
			forbidden: []error{
				ErrTargetPollDeadline,
				ErrTargetPollCancelled,
				context.DeadlineExceeded,
				context.Canceled,
			},
		},
		{
			name: "trailing_content",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, `{} extra content here`)
			},
			forbidden: []error{
				ErrTargetPollDeadline,
				ErrTargetPollCancelled,
				context.DeadlineExceeded,
				context.Canceled,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			input := TargetPollInput{
				Client:          &http.Client{Timeout: 5 * time.Second},
				UVB76APIBaseURL: server.URL,
				Target:          TargetConfigBinding{TargetID: "test"},
				Auth:            TargetStateAuthInput{CookieName: "s", CookieValue: "v"},
				PollInterval:    100 * time.Millisecond,
				RequestTimeout:  5 * time.Second,
				Deadline:        10 * time.Second,
			}

			ctx := context.Background()
			result := PollTargetAuthority(ctx, input)

			if result.TerminalError == nil {
				t.Fatal("Expected terminal error")
			}

			// Verify no forbidden identities
			for _, forbid := range tt.forbidden {
				if errors.Is(result.TerminalError, forbid) {
					t.Errorf("Forbidden identity %v found in %v", forbid, result.TerminalError)
				}
			}

			t.Logf("Terminal error for %s: %v", tt.name, result.TerminalError)
		})
	}
}

// TestPollTarget_CompletionNoTerminalError tests that a completed snapshot has no terminal error.
func TestPollTarget_CompletionNoTerminalError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"target_id": "test",
			"reachable":   true,
			"status":      "healthy", // Non-empty status required for IsScrapeCompleted
			"error":       "",
			"scraped_at":  time.Now().Format(time.RFC3339), // Required for AttemptObserved
		})
	}))
	defer server.Close()

	input := TargetPollInput{
		Client:          &http.Client{Timeout: 5 * time.Second},
		UVB76APIBaseURL: server.URL,
		Target:          TargetConfigBinding{TargetID: "test"},
		Auth:            TargetStateAuthInput{CookieName: "s", CookieValue: "v"},
		PollInterval:    100 * time.Millisecond,
		RequestTimeout:  5 * time.Second,
		Deadline:        10 * time.Second,
	}

	ctx := context.Background()
	result := PollTargetAuthority(ctx, input)

	if result.TerminalError != nil {
		t.Errorf("Completed poll should not have terminal error, got: %v", result.TerminalError)
	}

	if !result.Completed {
		t.Error("Expected poll to be completed")
	}
}

// TestPollTarget_DeadlineNoObservation tests deadline without any observation.
func TestPollTarget_DeadlineNoObservation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "{}")
	}))
	defer server.Close()

	input := TargetPollInput{
		Client:          &http.Client{Timeout: 10 * time.Second},
		UVB76APIBaseURL: server.URL,
		Target:          TargetConfigBinding{TargetID: "test"},
		Auth:            TargetStateAuthInput{CookieName: "s", CookieValue: "v"},
		PollInterval:    100 * time.Millisecond,
		RequestTimeout:  500 * time.Millisecond,
		Deadline:        200 * time.Millisecond, // Very short deadline
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	result := PollTargetAuthority(ctx, input)

	// Must have deadline identity
	if !errors.Is(result.TerminalError, ErrTargetPollDeadline) {
		t.Errorf("Expected ErrTargetPollDeadline, got: %v", result.TerminalError)
	}

	// Must have no-observation identity
	if !errors.Is(result.TerminalError, ErrTargetPollNoObservation) {
		t.Errorf("Expected ErrTargetPollNoObservation, got: %v", result.TerminalError)
	}

	// Must have DeadlineExceeded
	if !errors.Is(result.TerminalError, context.DeadlineExceeded) {
		t.Errorf("Expected context.DeadlineExceeded, got: %v", result.TerminalError)
	}

	// Must NOT have cancelled
	if errors.Is(result.TerminalError, context.Canceled) {
		t.Errorf("Should not have context.Canceled")
	}
}

// TestPollTarget_DeadlineAfterObservationNoCompletion tests deadline after observation but no completion.
func TestPollTarget_DeadlineAfterObservationNoCompletion(t *testing.T) {
	attempt := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		w.Header().Set("Content-Type", "application/json")
		if attempt == 1 {
			// First attempt: return partial observation (no completion)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"target_id": "test",
				"reachable": false,
				"error":     "connection refused",
			})
		} else {
			time.Sleep(10 * time.Second)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	input := TargetPollInput{
		Client:          &http.Client{Timeout: 10 * time.Second},
		UVB76APIBaseURL: server.URL,
		Target:          TargetConfigBinding{TargetID: "test"},
		Auth:            TargetStateAuthInput{CookieName: "s", CookieValue: "v"},
		PollInterval:    100 * time.Millisecond,
		RequestTimeout:  2 * time.Second,
		Deadline:        1 * time.Second, // Deadline after first observation but before completion
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := PollTargetAuthority(ctx, input)

	// Must have deadline identity
	if !errors.Is(result.TerminalError, ErrTargetPollDeadline) {
		t.Errorf("Expected ErrTargetPollDeadline, got: %v", result.TerminalError)
	}

	// Must have no-completion identity (not no-observation)
	if !errors.Is(result.TerminalError, ErrTargetPollNoCompletion) {
		t.Errorf("Expected ErrTargetPollNoCompletion, got: %v", result.TerminalError)
	}

	// Must have DeadlineExceeded
	if !errors.Is(result.TerminalError, context.DeadlineExceeded) {
		t.Errorf("Expected context.DeadlineExceeded, got: %v", result.TerminalError)
	}

	// Must NOT have cancelled
	if errors.Is(result.TerminalError, context.Canceled) {
		t.Errorf("Should not have context.Canceled")
	}

	// Must have had observation
	if result.BestAuthority == nil {
		t.Error("Expected observation to be recorded")
	}

	t.Logf("Terminal error: %v", result.TerminalError)
}

// TestPollTarget_RecoveredCauseBound tests that recovered cause counts are bounded.
func TestPollTarget_RecoveredCauseBound(t *testing.T) {
	errorCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errorCount++
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"attempt": %d}`, errorCount)
	}))
	defer server.Close()

	input := TargetPollInput{
		Client:          &http.Client{Timeout: 500 * time.Millisecond},
		UVB76APIBaseURL: server.URL,
		Target:          TargetConfigBinding{TargetID: "test"},
		Auth:            TargetStateAuthInput{CookieName: "s", CookieValue: "v"},
		PollInterval:    10 * time.Millisecond, // Very short interval
		RequestTimeout:  200 * time.Millisecond,
		Deadline:        1 * time.Second,
	}

	ctx := context.Background()
	result := PollTargetAuthority(ctx, input)

	// Verify recovered error count
	if result.RecoveredErrorCount == 0 {
		t.Error("Expected some recovered errors")
	}

	// Verify bounded diagnostics
	if len(result.RecoveredErrors) > MaxRecoveredDiagnostics {
		t.Errorf("Recovered errors %d exceeds max %d", len(result.RecoveredErrors), MaxRecoveredDiagnostics)
	}

	// Verify each error is bounded
	for i, errStr := range result.RecoveredErrors {
		if len(errStr) > MaxRecoveredErrorLen {
			t.Errorf("Recovered error %d length %d exceeds max %d", i, len(errStr), MaxRecoveredErrorLen)
		}
	}

	t.Logf("Recovered error count: %d, diagnostic count: %d", result.RecoveredErrorCount, len(result.RecoveredErrors))
}
