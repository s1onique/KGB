// Package main provides the UVB-76 pprof memory leak lab.
//
// # Target Polling with Typed Results and Complete Cancellation
//
// This file implements:
// P0-3: PollTargetAuthority cancellation-complete
// P0-7: Bounded recovered cause set
// P0-8: Centralized semantic poll termination
// P0-9: All failures typed and preserved with errors.Join
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

// Sentinel errors for target poll.
var (
	// ErrTargetPollCancelled is returned when poll is explicitly cancelled.
	// P0-8: Explicit cancellation must not expose deadline identities.
	ErrTargetPollCancelled = errors.New("target poll cancelled")

	// ErrTargetPollDeadline is returned when polling deadline expires.
	ErrTargetPollDeadline = errors.New("target poll deadline")

	// ErrTargetPollTransport is returned on transport errors during polling.
	ErrTargetPollTransport = errors.New("target poll transport error")

	// ErrTargetPollUnauthorized is returned on 401 during polling.
	ErrTargetPollUnauthorized = errors.New("target poll unauthorized")

	// ErrTargetPollForbidden is returned on 403 during polling.
	ErrTargetPollForbidden = errors.New("target poll forbidden")

	// ErrTargetPollUnexpectedStatus is returned on unexpected HTTP status.
	ErrTargetPollUnexpectedStatus = errors.New("target poll unexpected status")

	// ErrTargetPollDecode is returned when snapshot decode fails.
	ErrTargetPollDecode = errors.New("target poll decode failed")

	// ErrTargetPollTrailingContent is returned when response has trailing content.
	ErrTargetPollTrailingContent = errors.New("target poll trailing content")

	// ErrTargetPollTargetNotFound is returned when target doesn't exist.
	ErrTargetPollTargetNotFound = errors.New("target poll target not found")

	// ErrTargetPollNoObservation is returned when no observation before deadline.
	ErrTargetPollNoObservation = errors.New("target poll no observation")

	// ErrTargetPollNoCompletion is returned when no completion before deadline.
	ErrTargetPollNoCompletion = errors.New("target poll no completion")
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

	// RecoveredErrorCount is the number of transient errors that were recovered.
	RecoveredErrorCount int

	// RecoveredErrors contains truncated diagnostic strings for recovered errors.
	// P0-7: Bounded to maximum 16 entries, each truncated to 512 bytes.
	RecoveredErrors []string

	// TerminalError is non-nil when polling terminated with a terminal failure.
	TerminalError error

	// Completed is true if a completed identity-valid snapshot was observed.
	Completed bool
}

// targetPollProgress tracks semantic progress during polling.
// P0-8: Used to classify deadline termination correctly.
type targetPollProgress struct {
	// ObservationSeen is true if at least one observation was made.
	ObservationSeen bool

	// AttemptSeen is true if at least one attempt was made.
	AttemptSeen bool

	// CompletionSeen is true if a completed snapshot was observed.
	CompletionSeen bool
}

// targetPollCauseSet tracks bounded machine causes for recovered errors.
// P0-7: Bounded set prevents unbounded error accumulation.
type targetPollCauseSet struct {
	transport        bool
	unexpectedStatus bool
	targetNotFound   bool
}

// Error implements error for targetPollCauseSet.
func (s targetPollCauseSet) Error() error {
	var causes []error

	if s.transport {
		causes = append(causes, ErrTargetPollTransport)
	}
	if s.unexpectedStatus {
		causes = append(causes, ErrTargetPollUnexpectedStatus)
	}
	if s.targetNotFound {
		causes = append(causes, ErrTargetPollTargetNotFound)
	}

	return errors.Join(causes...)
}

// MaxRecoveredDiagnostics is the maximum number of diagnostic entries.
const MaxRecoveredDiagnostics = 16

// MaxRecoveredErrorLen is the maximum length of each diagnostic string.
const MaxRecoveredErrorLen = 512

// PollTargetAuthority polls UVB-76 for target state with typed results.
// P0-3: Cancellation-complete - all blocking points observe context.
// P0-8: Centralized semantic termination using finalizeTargetPollContext.
// P0-9: All failures typed and preserved.
func PollTargetAuthority(ctx context.Context, input TargetPollInput) TargetPollResult {
	result := TargetPollResult{}

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
	if input.Auth.CookieName == "" || input.Auth.CookieValue == "" {
		result.TerminalError = fmt.Errorf("missing auth input: %w", ErrAuthInputMissing)
		return result
	}

	// Build snapshot URL
	snapshotURL := BuildSnapshotURL(input.UVB76APIBaseURL, input.Target.TargetID)

	// Create polling context with deadline derived from caller context
	pollCtx, pollCancel := context.WithTimeout(ctx, input.Deadline)
	defer pollCancel()

	// P0-7: Bounded recovered causes
	var recoveredCauses targetPollCauseSet

	ticker := time.NewTicker(input.PollInterval)
	defer ticker.Stop()

	// Make timeout configurable
	if input.RequestTimeout == 0 {
		input.RequestTimeout = 5 * time.Second
	}

	for {
		select {
		case <-pollCtx.Done():
			// P0-3: Check parent context for explicit cancellation vs deadline
			// P0-8: Centralized termination using finalizeTargetPollContext
			return finalizeTargetPollContext(ctx, pollCtx, targetPollProgress{
				ObservationSeen: result.BestAuthority != nil,
				AttemptSeen:    result.Attempts > 0,
				CompletionSeen: result.Completed,
			}, recoveredCauses, result)

		case <-ticker.C:
			result.Attempts++
			progress := targetPollProgress{
				ObservationSeen: result.BestAuthority != nil,
				AttemptSeen:     true,
				CompletionSeen:  result.Completed,
			}

			auth, pollErr := fetchTargetSnapshotWithAuth(
				pollCtx, input.Client, snapshotURL,
				input.Auth.CookieName, input.Auth.CookieValue,
				input.RequestTimeout, input.Target,
			)

			if pollErr != nil {
				// P0-3: Check parent context before classifying as recoverable
				// This prevents cancellation from being recorded as transport error
				if pollCtx.Err() != nil {
					return finalizeTargetPollContext(ctx, pollCtx, progress, recoveredCauses, result)
				}

				// Classify error
				if isTerminalPollError(pollErr) {
					result.TerminalError = pollErr
					return result
				}

				// P0-7: Recoverable error - track bounded cause
				recoveredCauses = addRecoveredCause(recoveredCauses, pollErr)
				result.RecoveredErrorCount++

				// P0-7: Add truncated diagnostic string
				if len(result.RecoveredErrors) < MaxRecoveredDiagnostics {
					errStr := truncateErrorString(pollErr.Error(), MaxRecoveredErrorLen)
					result.RecoveredErrors = append(result.RecoveredErrors, errStr)
				}
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
		}
	}
}

// finalizeTargetPollContext classifies and returns the appropriate terminal error.
// P0-8: Centralized semantic termination.
func finalizeTargetPollContext(
	parentCtx context.Context,
	pollCtx context.Context,
	progress targetPollProgress,
	recoveredCauses targetPollCauseSet,
	result TargetPollResult,
) TargetPollResult {
	// P0-8: Explicit cancellation - distinct from deadline
	if errors.Is(parentCtx.Err(), context.Canceled) {
		result.TerminalError = errors.Join(
			ErrTargetPollCancelled,
			context.Canceled,
			recoveredCauses.Error(),
		)
		return result
	}

	// P0-8: Deadline classifications based on progress
	if !progress.ObservationSeen {
		// P0-8: Deadline with no observation
		result.TerminalError = errors.Join(
			ErrTargetPollDeadline,
			ErrTargetPollNoObservation,
			context.DeadlineExceeded,
			recoveredCauses.Error(),
		)
	} else if !progress.CompletionSeen {
		// P0-8: Deadline after observation but without completion
		result.TerminalError = errors.Join(
			ErrTargetPollDeadline,
			ErrTargetPollNoCompletion,
			context.DeadlineExceeded,
			recoveredCauses.Error(),
		)
	} else {
		// Should not reach here if completion was seen, but handle gracefully
		result.TerminalError = errors.Join(
			ErrTargetPollDeadline,
			context.DeadlineExceeded,
			recoveredCauses.Error(),
		)
	}

	return result
}

// addRecoveredCause adds a cause to the bounded set based on error type.
func addRecoveredCause(set targetPollCauseSet, err error) targetPollCauseSet {
	if errors.Is(err, ErrTargetPollTransport) {
		set.transport = true
	} else if errors.Is(err, ErrTargetPollUnexpectedStatus) {
		set.unexpectedStatus = true
	} else if errors.Is(err, ErrTargetPollTargetNotFound) {
		set.targetNotFound = true
	}
	return set
}

// truncateErrorString truncates error string to maximum length.
func truncateErrorString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// fetchTargetSnapshotWithAuth fetches a target snapshot with explicit auth.
// P0-3: Cancellation-complete - all blocking points observe request context.
func fetchTargetSnapshotWithAuth(
	ctx context.Context,
	client *http.Client,
	url string,
	cookieName string,
	cookieValue string,
	timeout time.Duration,
	expectedTarget TargetConfigBinding,
) (*TargetStateAuthority, error) {
	// P0-3: Create request with timeout context for cancellation support
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// P0-3: Request can be cancelled by context
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: create request: %v", ErrTargetPollTransport, err)
	}

	// Add session auth using http.Cookie with separate name/value
	req.AddCookie(&http.Cookie{Name: cookieName, Value: cookieValue})

	// P0-3: HTTP request observes reqCtx for cancellation
	resp, err := client.Do(req)
	if err != nil {
		// P0-3: Request-level cancellation is captured here
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

	// P0-3: Body read observes reqCtx for cancellation
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
	// P0-3: Body read for trailing check observes reqCtx
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
// P0-9: Uses errors.Is for sentinel classification, not string matching.
func isTerminalPollError(err error) bool {
	if err == nil {
		return false
	}

	// P0-9: Classify using sentinel errors via errors.Is
	return errors.Is(err, ErrTargetPollUnauthorized) ||
		errors.Is(err, ErrTargetPollForbidden) ||
		errors.Is(err, ErrTargetPollDecode) ||
		errors.Is(err, ErrTargetPollTrailingContent) ||
		errors.Is(err, ErrTargetIdentityMismatch)
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
