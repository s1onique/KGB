package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/server"
)

// TestCheckAPIReadinessBasicSuccess tests basic successful readiness.
func TestCheckAPIReadinessBasicSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/status" {
			t.Errorf("expected /api/v1/status path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(server.ServerStatus{
			StartedAt: "2024-01-01T00:00:00Z",
		})
	}))
	defer server.Close()

	result := CheckAPIReadiness(context.Background(), APIReadinessInput{
		URL:      server.URL,
		Deadline: 5 * time.Second,
	})

	if !result.Ready {
		t.Errorf("expected ready, got terminal error: %v", result.TerminalError)
	}
	if result.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", result.Attempts)
	}
}

// TestCheckAPIReadinessTransientFailureThenSuccess tests transient failures followed by success.
func TestCheckAPIReadinessTransientFailureThenSuccess(t *testing.T) {
	attempt := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt < 3 {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(server.ServerStatus{
			StartedAt: "2024-01-01T00:00:00Z",
		})
	}))
	defer server.Close()

	result := CheckAPIReadiness(context.Background(), APIReadinessInput{
		URL:          server.URL,
		PollInterval: 10 * time.Millisecond,
		Deadline:     5 * time.Second,
	})

	if !result.Ready {
		t.Errorf("expected ready after retries, got terminal error: %v", result.TerminalError)
	}
	if result.Attempts < 3 {
		t.Errorf("expected at least 3 attempts, got %d", result.Attempts)
	}
	if len(result.RecoveredErrors) < 2 {
		t.Errorf("expected at least 2 recovered errors, got %d", len(result.RecoveredErrors))
	}
}

// TestCheckAPIReadinessDeadlineExceeded tests deadline classification.
func TestCheckAPIReadinessDeadlineExceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		http.Error(w, "slow", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	result := CheckAPIReadiness(context.Background(), APIReadinessInput{
		URL:           server.URL,
		PollInterval:  10 * time.Millisecond,
		RequestLimit:  5 * time.Millisecond,
		Deadline:      50 * time.Millisecond,
	})

	if result.Ready {
		t.Error("expected not ready after deadline")
	}
	if !errors.Is(result.TerminalError, ErrAPIReadinessDeadline) {
		t.Errorf("expected deadline error, got: %v", result.TerminalError)
	}
	// P0-3I: Verify cause is preserved for errors.Is
	if !errors.Is(result.TerminalError, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got: %v", result.TerminalError)
	}
}

// P0-3T: Test explicit context cancellation (not deadline).
func TestReadinessExplicitCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	result := CheckAPIReadiness(ctx, APIReadinessInput{
		URL:          server.URL,
		PollInterval: 100 * time.Millisecond,
		Deadline:     5 * time.Second, // Large deadline so cancellation is the trigger
	})

	if result.Ready {
		t.Error("expected not ready after explicit cancellation")
	}

	// P0-3T: Cancellation must include ErrAPIReadinessCancelled sentinel
	if !errors.Is(result.TerminalError, ErrAPIReadinessCancelled) {
		t.Fatalf("expected ErrAPIReadinessCancelled: %v", result.TerminalError)
	}
	// P0-3T: Cancellation must include context.Canceled
	if !errors.Is(result.TerminalError, context.Canceled) {
		t.Fatalf("expected context.Canceled: %v", result.TerminalError)
	}
	// P0-3T: Cancellation must NOT include deadline sentinel
	if errors.Is(result.TerminalError, ErrAPIReadinessDeadline) {
		t.Fatalf("explicit cancellation exposed deadline: %v", result.TerminalError)
	}
}

// TestCheckAPIReadinessProcessExited tests process exit detection.
func TestCheckAPIReadinessProcessExited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(server.ServerStatus{
			StartedAt: "2024-01-01T00:00:00Z",
		})
	}))
	defer server.Close()

	processExitedCalled := false
	result := CheckAPIReadiness(context.Background(), APIReadinessInput{
		URL:      server.URL,
		Deadline: 5 * time.Second,
		ProcessExited: func() bool {
			processExitedCalled = true
			return true // Simulate process exited
		},
	})

	if result.Ready {
		t.Error("expected not ready when process exited")
	}
	if !errors.Is(result.TerminalError, ErrAPIReadinessProcessExited) {
		t.Errorf("expected process exited error, got: %v", result.TerminalError)
	}
	// P0-3L: Process exit should NOT include deadline sentinel
	if errors.Is(result.TerminalError, ErrAPIReadinessDeadline) {
		t.Error("process exit should not include deadline sentinel")
	}
	if !processExitedCalled {
		t.Error("expected ProcessExited to be called")
	}
}

// P0-3V: Test process exit with earlier causes preserved.
func TestCheckAPIReadinessProcessExitedWithCauses(t *testing.T) {
	attempt := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt == 1 {
			// First attempt returns 503 - establishes status cause
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		// Subsequent attempts are never reached due to ProcessExited
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(server.ServerStatus{
			StartedAt: "2024-01-01T00:00:00Z",
		})
	}))
	defer server.Close()

	processExited := false
	result := CheckAPIReadiness(context.Background(), APIReadinessInput{
		URL:          server.URL,
		PollInterval: 10 * time.Millisecond,
		Deadline:     500 * time.Millisecond,
		ProcessExited: func() bool {
			if attempt >= 1 {
				processExited = true
				return true
			}
			return false
		},
	})

	if result.Ready {
		t.Error("expected not ready when process exited")
	}
	if !errors.Is(result.TerminalError, ErrAPIReadinessProcessExited) {
		t.Errorf("expected ErrAPIReadinessProcessExited: %v", result.TerminalError)
	}
	// P0-3V: Earlier causes must be preserved
	if !errors.Is(result.TerminalError, ErrAPIReadinessStatus) {
		t.Errorf("expected earlier ErrAPIReadinessStatus preserved: %v", result.TerminalError)
	}
	// P0-3V: Must not include deadline sentinel
	if errors.Is(result.TerminalError, ErrAPIReadinessDeadline) {
		t.Error("process exit with earlier causes should not include deadline sentinel")
	}
	if !processExited {
		t.Error("expected process exited to be called")
	}
}

// TestCheckAPIReadinessWrongService tests wrong service rejection.
func TestCheckAPIReadinessWrongService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return wrong service response (missing started_at)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	result := CheckAPIReadiness(context.Background(), APIReadinessInput{
		URL:          server.URL,
		PollInterval: 10 * time.Millisecond,
		Deadline:     100 * time.Millisecond,
	})

	if result.Ready {
		t.Error("expected not ready for wrong service")
	}
	// Should have recovered errors for wrong service status
	if len(result.RecoveredErrors) == 0 {
		t.Error("expected recovered errors for wrong service")
	}
}

// TestCheckAPIReadinessUnknownJSONFields tests strict JSON decoding rejects unknown fields.
func TestCheckAPIReadinessUnknownJSONFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return JSON with extra unknown field
		json.NewEncoder(w).Encode(map[string]interface{}{
			"started_at": "2024-01-01T00:00:00Z",
			"extra":      "value", // Unknown field - should be rejected
		})
	}))
	defer server.Close()

	result := CheckAPIReadiness(context.Background(), APIReadinessInput{
		URL:          server.URL,
		PollInterval: 10 * time.Millisecond,
		Deadline:     100 * time.Millisecond,
	})

	if result.Ready {
		t.Error("expected not ready for unknown JSON fields")
	}
}

// TestCheckAPIReadinessMultipleJSONDocuments tests multiple JSON documents rejection.
func TestCheckAPIReadinessMultipleJSONDocuments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return multiple JSON documents
		w.Write([]byte(`{"started_at":"2024-01-01T00:00:00Z"}{"started_at":"2024-01-01T00:00:00Z"}`))
	}))
	defer server.Close()

	result := CheckAPIReadiness(context.Background(), APIReadinessInput{
		URL:          server.URL,
		PollInterval: 10 * time.Millisecond,
		Deadline:     100 * time.Millisecond,
	})

	if result.Ready {
		t.Error("expected not ready for multiple JSON documents")
	}
}

// TestCheckAPIReadinessMalformedJSON tests malformed JSON rejection.
func TestCheckAPIReadinessMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return malformed JSON
		w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	result := CheckAPIReadiness(context.Background(), APIReadinessInput{
		URL:          server.URL,
		PollInterval: 10 * time.Millisecond,
		Deadline:     100 * time.Millisecond,
	})

	if result.Ready {
		t.Error("expected not ready for malformed JSON")
	}
}

// TestCheckAPIReadinessEmptyURL tests empty URL rejection.
func TestCheckAPIReadinessEmptyURL(t *testing.T) {
	result := CheckAPIReadiness(context.Background(), APIReadinessInput{
		URL:     "",
		Deadline: 5 * time.Second,
	})

	if result.Ready {
		t.Error("expected not ready for empty URL")
	}
	if !errors.Is(result.TerminalError, ErrAPIReadinessURL) {
		t.Errorf("expected URL error, got: %v", result.TerminalError)
	}
}

// TestCheckAPIReadinessRedirectBlocked tests that redirects are blocked.
func TestCheckAPIReadinessRedirectBlocked(t *testing.T) {
	var redirectSourceCalls atomic.Int64
	var redirectDestCalls atomic.Int64

	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectDestCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(server.ServerStatus{
			StartedAt: "2024-01-01T00:00:00Z",
		})
	}))
	defer finalServer.Close()

	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectSourceCalls.Add(1)
		http.Redirect(w, r, finalServer.URL+"/api/v1/status", http.StatusMovedPermanently)
	}))
	defer redirectServer.Close()

	result := CheckAPIReadiness(context.Background(), APIReadinessInput{
		URL:          redirectServer.URL,
		PollInterval: 10 * time.Millisecond,
		Deadline:     100 * time.Millisecond,
	})

	if result.Ready {
		t.Error("expected not ready when redirect is blocked")
	}

	// Redirect source was contacted
	if redirectSourceCalls.Load() == 0 {
		t.Error("redirect source should have been called")
	}

	// Redirect destination was NOT contacted
	if redirectDestCalls.Load() > 0 {
		t.Error("redirect destination should not have been called (redirect was blocked)")
	}
}

// TestCheckAPIReadinessRecoveredErrorBounds tests recovered error bounds.
func TestCheckAPIReadinessRecoveredErrorBounds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		http.Error(w, "error", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	result := CheckAPIReadiness(context.Background(), APIReadinessInput{
		URL:           server.URL,
		PollInterval:  10 * time.Millisecond,
		RequestLimit:  5 * time.Millisecond,
		Deadline:      200 * time.Millisecond,
	})

	// Should have multiple recovered errors
	if len(result.RecoveredErrors) < 2 {
		t.Errorf("expected multiple recovered errors, got %d", len(result.RecoveredErrors))
	}

	// P0-3I: Error texts should be bounded
	for _, err := range result.RecoveredErrors {
		if len(err) > maxAPIReadinessErrorBytes {
			t.Errorf("recovered error string too long: %d bytes (max %d)", len(err), maxAPIReadinessErrorBytes)
		}
	}

	// Verify RecoveredErrorCount tracks total
	if result.RecoveredErrorCount < 2 {
		t.Errorf("expected RecoveredErrorCount >= 2, got %d", result.RecoveredErrorCount)
	}
}

// TestCheckAPIReadinessHTTPErrorStatusCodes tests various HTTP error status codes.
func TestCheckAPIReadinessHTTPErrorStatusCodes(t *testing.T) {
	testCases := []struct {
		statusCode int
		name       string
	}{
		{401, "Unauthorized"},
		{403, "Forbidden"},
		{404, "Not Found"},
		{500, "Internal Server Error"},
		{502, "Bad Gateway"},
		{503, "Service Unavailable"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, tc.name, tc.statusCode)
			}))
			defer server.Close()

			result := CheckAPIReadiness(context.Background(), APIReadinessInput{
				URL:          server.URL,
				PollInterval: 10 * time.Millisecond,
				Deadline:     50 * time.Millisecond,
			})

			if result.Ready {
				t.Errorf("%s: expected not ready", tc.name)
			}
			if result.LastStatusCode != tc.statusCode {
				t.Errorf("expected status %d, got %d", tc.statusCode, result.LastStatusCode)
			}
		})
	}
}

// TestProbeAPIReadinessOnceContextLifecycle tests the iteration-scoped context lifecycle.
func TestProbeAPIReadinessOnceContextLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(server.ServerStatus{
			StartedAt: "2024-01-01T00:00:00Z",
		})
	}))
	defer server.Close()

	client := &http.Client{}

	// Make multiple probes - each should have its own context
	for i := 0; i < 3; i++ {
		probe := probeAPIReadinessOnce(context.Background(), client, server.URL+"/api/v1/status", 5*time.Second)
		if probe.Err != nil {
			t.Fatalf("probe %d failed: %v", i, probe.Err)
		}

		if probe.StatusCode != http.StatusOK {
			t.Errorf("probe %d: expected 200, got %d", i, probe.StatusCode)
		}
	}
}

// TestDecodeStrictJSON tests the strict JSON decoder.
func TestDecodeStrictJSON(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		wantErr  bool
	}{
		{"valid", `{"started_at":"2024-01-01T00:00:00Z"}`, false},
		{"unknown field", `{"started_at":"2024-01-01T00:00:00Z","extra":"value"}`, true},
		{"multiple docs", `{"started_at":"2024-01-01T00:00:00Z"}{"started_at":"2024-01-01T00:00:00Z"}`, true},
		{"malformed", `{invalid`, true},
		{"empty", ``, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var resp server.ServerStatus
			err := decodeStrictJSON([]byte(tc.input), &resp)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestAPIReadinessInputValidation tests input validation.
func TestAPIReadinessInputValidation(t *testing.T) {
	testCases := []struct {
		name    string
		input   APIReadinessInput
		wantErr error
	}{
		{
			name:    "empty URL",
			input:   APIReadinessInput{URL: ""},
			wantErr: ErrAPIReadinessURL,
		},
		{
			name:    "relative URL",
			input:   APIReadinessInput{URL: "/api/status"},
			wantErr: ErrAPIReadinessURL,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := CheckAPIReadiness(context.Background(), tc.input)
			if result.TerminalError == nil {
				t.Error("expected error, got nil")
				return
			}
			if !errors.Is(result.TerminalError, tc.wantErr) {
				t.Errorf("expected %v, got %v", tc.wantErr, result.TerminalError)
			}
		})
	}
}

// TestCheckAPIReadinessCanonicalRoute tests that canonical route is used.
func TestCheckAPIReadinessCanonicalRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify canonical path using production constant
		if r.URL.Path != server.StatusRoute {
			t.Errorf("expected %q, got %q", server.StatusRoute, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(server.ServerStatus{
			StartedAt: "2024-01-01T00:00:00Z",
		})
	}))
	defer server.Close()

	result := CheckAPIReadiness(context.Background(), APIReadinessInput{
		URL:      server.URL,
		Deadline: 5 * time.Second,
	})

	if !result.Ready {
		t.Errorf("expected ready, got: %v", result.TerminalError)
	}

	// Verify result URL contains canonical route
	if !strings.Contains(result.URL, "/api/v1/status") {
		t.Errorf("expected URL to contain /api/v1/status, got: %s", result.URL)
	}
}

// TestCheckAPIReadinessTerminalCausesPreserved tests that terminal error preserves causes.
func TestCheckAPIReadinessTerminalCausesPreserved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	result := CheckAPIReadiness(context.Background(), APIReadinessInput{
		URL:          server.URL,
		PollInterval: 10 * time.Millisecond,
		Deadline:     50 * time.Millisecond,
	})

	if result.Ready {
		t.Error("expected not ready")
	}

	// P0-3I: Terminal error should contain deadline
	if !errors.Is(result.TerminalError, ErrAPIReadinessDeadline) {
		t.Errorf("expected deadline in terminal error, got: %v", result.TerminalError)
	}

	// P0-3I: Terminal error should contain recovered causes via errors.Is
	if !errors.Is(result.TerminalError, ErrAPIReadinessStatus) {
		t.Errorf("expected ErrAPIReadinessStatus in terminal error, got: %v", result.TerminalError)
	}

	// Terminal error should also contain recovered errors
	if result.RecoveredErrorCount == 0 {
		t.Error("expected recovered errors to be preserved")
	}
}

// TestCheckAPIReadinessMaxRecoveredErrorsBound tests that recovered errors are bounded.
func TestCheckAPIReadinessMaxRecoveredErrorsBound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "error", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	result := CheckAPIReadiness(context.Background(), APIReadinessInput{
		URL:          server.URL,
		PollInterval: 5 * time.Millisecond,
		Deadline:     200 * time.Millisecond,
	})

	if result.Ready {
		t.Error("expected not ready")
	}

	// RecoveredErrors slice should be bounded
	if len(result.RecoveredErrors) > maxAPIReadinessRecoveredErrors {
		t.Errorf("expected RecoveredErrors <= %d, got %d", maxAPIReadinessRecoveredErrors, len(result.RecoveredErrors))
	}
}

// TestCheckAPIReadinessProductionSchemaRequired tests that production schema is required.
func TestCheckAPIReadinessProductionSchemaRequired(t *testing.T) {
	testCases := []struct {
		name       string
		body       string
		shouldFail bool
	}{
		{
			name:       "valid production schema",
			body:       `{"started_at":"2024-01-01T00:00:00Z"}`,
			shouldFail: false,
		},
		{
			name:       "missing started_at",
			body:       `{}`,
			shouldFail: true,
		},
		{
			name:       "wrong field names",
			body:       `{"status":"ok"}`,
			shouldFail: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(tc.body))
			}))
			defer server.Close()

			result := CheckAPIReadiness(context.Background(), APIReadinessInput{
				URL:          server.URL,
				PollInterval: 100 * time.Millisecond,
				Deadline:     200 * time.Millisecond,
			})

			if tc.shouldFail && result.Ready {
				t.Error("expected not ready for invalid schema")
			}
			if !tc.shouldFail && !result.Ready {
				t.Errorf("expected ready for valid schema, got: %v", result.TerminalError)
			}
		})
	}
}

// P0-3U: Test truncation with exact byte bounds.
func TestTruncateError(t *testing.T) {
	testCases := []struct {
		name     string
		maxBytes int
		wantLen  int // expected final length
	}{
		{"max_0", 0, 0},
		{"max_1", 1, 1},
		{"max_2", 2, 2},
		{"max_3", 3, 3}, // exactly ellipsis length
		{"max_10", 10, 10},
		{"max_512", 512, 512},
	}

	longErr := errors.New(strings.Repeat("x", 1000))

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			truncated, _ := truncateError(ErrAPIReadinessTransport, longErr, tc.maxBytes)
			if got := len(truncated); got != tc.wantLen {
				t.Errorf("maxBytes=%d: got %d chars, want exactly %d: %q", tc.maxBytes, got, tc.wantLen, truncated)
			}
		})
	}

	// Test short message passes through unchanged
	shortErr := errors.New("short")
	msg, _ := truncateError(ErrAPIReadinessTransport, shortErr, 10)
	if msg != "short" {
		t.Errorf("expected 'short', got: %s", msg)
	}
}

// P0-3M: Test negative duration validation
func TestCheckAPIReadinessNegativeDurations(t *testing.T) {
	testCases := []struct {
		name    string
		input   APIReadinessInput
		wantErr string
	}{
		{
			name:    "negative PollInterval",
			input:   APIReadinessInput{URL: "http://localhost:8080", PollInterval: -1 * time.Second},
			wantErr: "PollInterval",
		},
		{
			name:    "negative RequestLimit",
			input:   APIReadinessInput{URL: "http://localhost:8080", RequestLimit: -1 * time.Second},
			wantErr: "RequestLimit",
		},
		{
			name:    "negative Deadline",
			input:   APIReadinessInput{URL: "http://localhost:8080", Deadline: -1 * time.Second},
			wantErr: "Deadline",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := CheckAPIReadiness(context.Background(), tc.input)
			if result.TerminalError == nil {
				t.Error("expected error, got nil")
				return
			}
			if !strings.Contains(result.TerminalError.Error(), tc.wantErr) {
				t.Errorf("expected %q in error, got: %v", tc.wantErr, result.TerminalError)
			}
		})
	}
}

// TestCheckAPIReadinessNilContext tests nil context rejection.
func TestCheckAPIReadinessNilContext(t *testing.T) {
	result := CheckAPIReadiness(nil, APIReadinessInput{
		URL:     "http://localhost:8080",
		Deadline: 5 * time.Second,
	})

	if result.TerminalError == nil {
		t.Error("expected error for nil context")
	}
	if !strings.Contains(result.TerminalError.Error(), "nil context") {
		t.Errorf("expected 'nil context' in error, got: %v", result.TerminalError)
	}
}

// P0-3N: Test oversized body detection
func TestCheckAPIReadinessOversizedBody(t *testing.T) {
	// Create a response body larger than maxStatusBodyBytes
	largeBody := make([]byte, maxStatusBodyBytes+1000)
	for i := range largeBody {
		largeBody[i] = 'a'
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(largeBody)
	}))
	defer server.Close()

	result := CheckAPIReadiness(context.Background(), APIReadinessInput{
		URL:          server.URL,
		PollInterval: 10 * time.Millisecond,
		Deadline:     100 * time.Millisecond,
	})

	if result.Ready {
		t.Error("expected not ready for oversized body")
	}
}

// P0-3H+P0-3P: Test that per-request timeout works with multiple attempts.
func TestCheckAPIReadinessPerRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Each request takes 50ms
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(server.ServerStatus{
			StartedAt: "2024-01-01T00:00:00Z",
		})
	}))
	defer server.Close()

	result := CheckAPIReadiness(context.Background(), APIReadinessInput{
		URL:          server.URL,
		PollInterval: 10 * time.Millisecond,
		RequestLimit: 20 * time.Millisecond, // Each request times out after 20ms
		Deadline:     200 * time.Millisecond, // Overall deadline 200ms
	})

	// Should fail due to repeated request timeouts
	if result.Ready {
		t.Error("expected not ready due to request timeouts")
	}

	// Should have made multiple attempts (request timeouts)
	if result.Attempts < 5 {
		t.Errorf("expected multiple attempts due to request timeouts, got %d", result.Attempts)
	}

	// Should be deadline error
	if !errors.Is(result.TerminalError, ErrAPIReadinessDeadline) {
		t.Errorf("expected deadline error, got: %v", result.TerminalError)
	}
}

// P0-3W: Test that request context covers body reading with delayed response.
func TestReadinessRequestContextCoversBodyRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()

		// Delay body write by 25ms after headers
		time.Sleep(25 * time.Millisecond)

		_ = json.NewEncoder(w).Encode(server.ServerStatus{
			StartedAt: "2026-07-30T00:00:00Z",
		})
	}))
	defer srv.Close()

	result := CheckAPIReadiness(
		context.Background(),
		APIReadinessInput{
			URL:          srv.URL,
			PollInterval: time.Millisecond,
			RequestLimit: 100 * time.Millisecond,
			Deadline:     time.Second,
		},
	)

	if !result.Ready {
		t.Fatalf("expected ready (body read within request context), got %v", result.TerminalError)
	}
}
