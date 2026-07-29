// Package main provides the UVB-76 pprof memory leak lab.
//
// # Target Polling with Typed Results
//
// This file implements P0-8: Typed TargetPollResult and P0-9: Preserve polling failures.
// All polling failures are typed and propagated. No log-and-continue behavior.
//
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TargetPollInput contains all inputs required for target polling.
// P0-8: All dependencies are explicitly injected.
type TargetPollInput struct {
	// HTTP client for requests
	Client *http.Client

	// Canonical UVB-76 API base URL (from generated config)
	UVB76APIBaseURL string

	// Canonical target binding (from generated config)
	Target TargetConfigBinding

	// Explicit authentication (from generated config)
	Auth TargetStateAuthInput

	// Polling interval
	PollInterval time.Duration

	// Request timeout
	RequestTimeout time.Duration

	// Overall poll deadline
	Deadline time.Duration
}

// TargetPollResult contains all polling outcomes.
// P0-8: Typed result replaces pointer-only poll result.
type TargetPollResult struct {
	// BestAuthority is the best identity-valid snapshot observed.
	BestAuthority *TargetStateAuthority

	// Attempts is the number of poll attempts made.
	Attempts int

	// RecoveredErrors contains transient errors that were recovered.
	RecoveredErrors []error

	// TerminalError is non-nil when polling terminated with a terminal failure.
	TerminalError error

	// Completed is true if a completed identity-valid snapshot was observed.
	Completed bool
}

// PollTargetAuthority polls UVB-76 for target state with typed results.
// P0-8: Returns typed result instead of pointer.
// P0-9: All failures are typed and preserved.
func PollTargetAuthority(ctx context.Context, input TargetPollInput) TargetPollResult {
	result := TargetPollResult{
		RecoveredErrors: make([]error, 0),
	}

	// Validate input
	if input.Client == nil {
		result.TerminalError = fmt.Errorf("nil HTTP client: %w", ErrTargetPollTransport)
		return result
	}
	if input.UVB76APIBaseURL == "" {
		result.TerminalError = fmt.Errorf("empty UVB-76 API base URL: %w", ErrTargetPollTransport)
		return result
	}
	if input.Target.TargetID == "" {
		result.TerminalError = fmt.Errorf("empty target ID: %w", ErrTargetIdentityMismatch)
		return result
	}
	if input.Auth.SessionCookie == "" {
		result.TerminalError = fmt.Errorf("missing auth input: %w", ErrAuthInputMissing)
		return result
	}

	// Build snapshot URL
	snapshotURL := BuildSnapshotURL(input.UVB76APIBaseURL, input.Target.TargetID)

	// Create polling context with deadline
	pollCtx, pollCancel := context.WithTimeout(ctx, input.Deadline)
	defer pollCancel()

	ticker := time.NewTicker(input.PollInterval)
	defer ticker.Stop()

	// Make timeout configurable
	if input.RequestTimeout == 0 {
		input.RequestTimeout = 5 * time.Second
	}

	for {
		select {
		case <-ticker.C:
			result.Attempts++
			auth, pollErr := fetchTargetSnapshotWithAuth(pollCtx, input.Client, snapshotURL, input.Auth.SessionCookie, input.RequestTimeout, input.Target)

			if pollErr != nil {
				// Classify error
				if isTerminalPollError(pollErr) {
					result.TerminalError = pollErr
					return result
				}
				// Recoverable error - retain and continue
				result.RecoveredErrors = append(result.RecoveredErrors, pollErr)
				continue
			}

			if auth != nil {
				// P0-10: Validate identity before accepting
				if validateSnapshotIdentity(auth, input.Target) != nil {
					// Identity mismatch is terminal
					result.TerminalError = fmt.Errorf("%w: snapshot identity validation failed", ErrTargetIdentityMismatch)
					return result
				}

				result.BestAuthority = auth
				if auth.IsScrapeCompleted() {
					result.Completed = true
					return result
				}
			}

		case <-pollCtx.Done():
			// Deadline reached
			if result.BestAuthority == nil {
				// No observation at all
				if len(result.RecoveredErrors) > 0 {
					result.TerminalError = fmt.Errorf("%w: %w", ErrTargetPollNoObservation, errors.Join(result.RecoveredErrors...))
				} else {
					result.TerminalError = fmt.Errorf("%w: deadline reached with no observation", ErrTargetPollDeadline)
				}
			} else if !result.Completed {
				// Observation but no completion
				var cause error
				if result.BestAuthority.IsScrapeAttempted() {
					cause = fmt.Errorf("%w: deadline reached after attempt but before completion", ErrTargetPollNoCompletion)
				} else {
					cause = fmt.Errorf("%w: deadline reached, no completion observed", ErrTargetPollNoCompletion)
				}
				if len(result.RecoveredErrors) > 0 {
					cause = fmt.Errorf("%v: recovered: %w", cause, errors.Join(result.RecoveredErrors...))
				}
				result.TerminalError = fmt.Errorf("%w: %w", ErrTargetPollDeadline, cause)
			}
			return result
		}
	}
}

// fetchTargetSnapshotWithAuth fetches a target snapshot with explicit auth.
func fetchTargetSnapshotWithAuth(
	ctx context.Context,
	client *http.Client,
	url string,
	sessionCookie string,
	timeout time.Duration,
	expectedTarget TargetConfigBinding,
) (*TargetStateAuthority, error) {
	// Create request with timeout
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: create request: %v", ErrTargetPollTransport, err)
	}

	// Add session auth
	req.AddCookie(&http.Cookie{Name: "session", Value: sessionCookie})

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTargetPollTransport, err)
	}
	defer resp.Body.Close()

	// Handle status codes
	switch resp.StatusCode {
	case http.StatusOK:
		// Continue to body parsing
	case http.StatusNotFound:
		return nil, fmt.Errorf("%w: target not found", ErrTargetPollTargetNotFound)
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("%w: session required", ErrTargetPollUnauthorized)
	case http.StatusForbidden:
		return nil, fmt.Errorf("%w: access forbidden", ErrTargetPollForbidden)
	default:
		return nil, fmt.Errorf("%w: status %d", ErrTargetPollUnexpectedStatus, resp.StatusCode)
	}

	// Decode with strict field matching
	decoder := json.NewDecoder(resp.Body)
	decoder.DisallowUnknownFields()

	var snap productionTargetSnapshot
	if err := decoder.Decode(&snap); err != nil {
		return nil, fmt.Errorf("%w: decode snapshot: %v", ErrTargetPollDecode, err)
	}

	// P0-9: Require exactly one JSON document
	var trailingCheck struct{}
	if err := decoder.Decode(&trailingCheck); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("%w: multiple JSON documents", ErrTargetPollTrailingContent)
		}
		return nil, fmt.Errorf("%w: expected EOF after snapshot: %v", ErrTargetPollTrailingContent, err)
	}

	// Also verify no trailing content
	body, _ := io.ReadAll(resp.Body)
	if len(body) > 0 {
		trimmed := strings.TrimSpace(string(body))
		if trimmed != "" {
			return nil, fmt.Errorf("%w: unexpected trailing content %q", ErrTargetPollTrailingContent, trimmed)
		}
	}

	// Build authority
	auth := &TargetStateAuthority{
		TargetID:          snap.TargetID,
		ConfiguredBaseURL: snap.BaseURL,
		SourceURL:         url,
	}

	// Attempt authority
	if snap.ScrapedAt != "" {
		auth.AttemptObserved = true
		if t, err := time.Parse(time.RFC3339, snap.ScrapedAt); err == nil {
			auth.AttemptTimestamp = t
			auth.AttemptTimestampValid = true
		}
	}

	// Completion authority
	auth.Reachable = snap.Reachable
	auth.Status = snap.Status
	auth.Error = snap.Error

	if snap.Reachable && snap.Error == "" && snap.RawResponse == "" {
		if snap.Status != "" {
			auth.CompletionObserved = true
			auth.CompletionTimestamp = auth.AttemptTimestamp
			auth.CompletionTimestampValid = auth.AttemptTimestampValid
		}
	} else if snap.Reachable && snap.Error == "" {
		auth.CompletionObserved = true
		auth.CompletionTimestamp = auth.AttemptTimestamp
		auth.CompletionTimestampValid = auth.AttemptTimestampValid
	}

	return auth, nil
}

// isTerminalPollError classifies whether a polling error is terminal.
func isTerminalPollError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()

	// Terminal error types
	terminalPrefixes := []string{
		"unauthorized",
		"forbidden",
		"identity mismatch",
		"trailing content",
		"multiple JSON documents",
		"decode failed",
		"schema-invalid",
	}

	for _, prefix := range terminalPrefixes {
		if strings.Contains(errStr, prefix) {
			return true
		}
	}

	return false
}

// validateSnapshotIdentity validates a snapshot against the expected target binding.
// P0-10: Must be called before accepting any snapshot as BestAuthority.
func validateSnapshotIdentity(auth *TargetStateAuthority, expected TargetConfigBinding) error {
	if auth == nil {
		return fmt.Errorf("nil authority")
	}

	// Target ID must match
	if auth.TargetID != expected.TargetID {
		return fmt.Errorf("%w: target ID %q != expected %q", ErrTargetIdentityMismatch, auth.TargetID, expected.TargetID)
	}

	// Configured base URL must match
	if auth.ConfiguredBaseURL != "" && auth.ConfiguredBaseURL != expected.BaseURL {
		return fmt.Errorf("%w: configured base URL %q != expected %q", ErrTargetIdentityMismatch, auth.ConfiguredBaseURL, expected.BaseURL)
	}

	return nil
}

// PollTargetAuthoritySimple is a simple wrapper for the existing poll function.
// P0-8: Maintains backward compatibility while using typed result internally.
func PollTargetAuthoritySimple(ctx context.Context, authority *GeneratedLabAuthority, pollInterval, deadline time.Duration) TargetPollResult {
	if authority == nil {
		return TargetPollResult{
			TerminalError: fmt.Errorf("nil authority: %w", ErrGeneratedConfigNil),
		}
	}

	input := TargetPollInput{
		Client:          &http.Client{Timeout: 5 * time.Second},
		UVB76APIBaseURL: authority.UVB76APIBaseURL,
		Target:          authority.Target,
		Auth:            authority.TargetStateAuth,
		PollInterval:    pollInterval,
		RequestTimeout:  5 * time.Second,
		Deadline:        deadline,
	}

	return PollTargetAuthority(ctx, input)
}
