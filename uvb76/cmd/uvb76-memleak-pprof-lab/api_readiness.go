// Package main provides the UVB-76 pprof memory leak lab.
//
// # Typed API Readiness
//
// This file implements P0-3: Typed UVB-76 API readiness authority.
// Readiness proves the expected UVB-76 service, not just any HTTP response.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/s1onique/KGB/uvb76/server"
)

// P0-3O: Canonical UVB-76 status route consumed by runtime.
// P0-3R: This constant is mechanically bound - lab runtime uses its parts.
const UVB76StatusRoute = server.StatusRoute

// Sentinel errors for API readiness.
var (
	// ErrAPIReadinessInput is returned when API readiness input is invalid.
	ErrAPIReadinessInput = errors.New("invalid API readiness input")

	// ErrAPIReadinessURL is returned when the API readiness URL is invalid.
	ErrAPIReadinessURL = errors.New("invalid API readiness URL")

	// ErrAPIReadinessTransport is returned when transport failure occurs.
	ErrAPIReadinessTransport = errors.New("API readiness transport failure")

	// ErrAPIReadinessStatus is returned when unexpected status code is received.
	ErrAPIReadinessStatus = errors.New("unexpected API readiness status")

	// ErrAPIReadinessDecode is returned when response decode fails.
	ErrAPIReadinessDecode = errors.New("invalid API readiness response")

	// ErrAPIReadinessWrongService is returned when response is not UVB-76.
	ErrAPIReadinessWrongService = errors.New("readiness response is not UVB-76")

	// ErrAPIReadinessDeadline is returned when deadline is exceeded.
	ErrAPIReadinessDeadline = errors.New("API readiness deadline exceeded")

	// ErrAPIReadinessCancelled is returned when context is cancelled.
	ErrAPIReadinessCancelled = errors.New("API readiness context cancelled")

	// ErrAPIReadinessProcessExited is returned when UVB-76 exits before readiness.
	ErrAPIReadinessProcessExited = errors.New("UVB-76 exited before API readiness")

	// ErrAPIReadinessRedirect is returned when redirect is detected.
	ErrAPIReadinessRedirect = errors.New("API readiness redirect detected")

	// P0-3E: Recovered error bounds
	maxAPIReadinessRecoveredErrors = 16
	maxAPIReadinessErrorBytes     = 512
	maxStatusBodyBytes            = 4096
)

// APIReadinessInput contains all inputs required for API readiness checking.
type APIReadinessInput struct {
	// URL is the canonical UVB-76 API base URL (must be absolute).
	URL string

	// PollInterval is the interval between readiness checks.
	PollInterval time.Duration

	// RequestLimit is the timeout for each request.
	RequestLimit time.Duration

	// Deadline is the overall deadline for readiness.
	Deadline time.Duration

	// ProcessExited is a function that checks if the process has exited.
	// If nil, process exit detection is skipped.
	ProcessExited func() bool
}

// APIReadinessResult contains the result of API readiness checking.
type APIReadinessResult struct {
	// URL is the final URL that was checked.
	URL string

	// Attempts is the number of HTTP attempts made.
	Attempts int

	// Ready indicates if the API became ready within the deadline.
	Ready bool

	// ReadyAt is the time when readiness was first achieved.
	ReadyAt *time.Time

	// LastStatusCode is the last HTTP status code received.
	LastStatusCode int

	// RecoveredErrors contains bounded error texts for diagnostics.
	// P0-3E: Count is bounded to maxAPIReadinessRecoveredErrors.
	RecoveredErrors []string

	// RecoveredErrorCount is the total number of recovered errors.
	RecoveredErrorCount int

	// TerminalError is non-nil when readiness failed terminally.
	// P0-3F: TerminalError preserves underlying causes via errors.Join.
	TerminalError error
}

// P0-3Q: Bounded cause set - at most one instance of each category.
type causeSet struct {
	transport    bool
	redirect     bool
	status       bool
	decode       bool
	wrongService bool
}

func (s *causeSet) Error() error {
	var causes []error
	if s.transport {
		causes = append(causes, ErrAPIReadinessTransport)
	}
	if s.redirect {
		causes = append(causes, ErrAPIReadinessRedirect)
	}
	if s.status {
		causes = append(causes, ErrAPIReadinessStatus)
	}
	if s.decode {
		causes = append(causes, ErrAPIReadinessDecode)
	}
	if s.wrongService {
		causes = append(causes, ErrAPIReadinessWrongService)
	}
	return errors.Join(causes...)
}

// P0-3S: Centralized context termination - deterministic projection.
func finalizeReadinessContext(ctx context.Context, causes *causeSet) error {
	err := ctx.Err()
	var sentinel error
	if errors.Is(err, context.Canceled) {
		sentinel = ErrAPIReadinessCancelled
	} else if errors.Is(err, context.DeadlineExceeded) {
		sentinel = ErrAPIReadinessDeadline
	} else {
		sentinel = ErrAPIReadinessInput
	}
	return errors.Join(sentinel, err, causes.Error())
}

// P0-3W: Probe result captures body within request context lifetime.
type apiReadinessProbe struct {
	StatusCode int
	Body       []byte
	Err        error
}

// P0-3W: probeAPIReadinessOnce reads the body within the request context scope.
// This ensures the context cancellation doesn't abort body reading.
func probeAPIReadinessOnce(
	ctx context.Context,
	client *http.Client,
	statusURL string,
	requestLimit time.Duration,
) apiReadinessProbe {
	var probe apiReadinessProbe

	// P0-3P: Derive request context from parent - parent's cancellation propagates
	reqCtx, cancel := context.WithTimeout(ctx, requestLimit)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, statusURL, nil)
	if err != nil {
		probe.Err = fmt.Errorf("create request: %w", err)
		return probe
	}

	resp, err := client.Do(req)
	if err != nil {
		probe.Err = err
		return probe
	}
	defer resp.Body.Close()

	// P0-3W: Read body within the request context scope
	// This ensures context cancellation doesn't abort body reading
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxStatusBodyBytes)+1))
	if err != nil {
		probe.Err = err
		return probe
	}

	probe.StatusCode = resp.StatusCode
	probe.Body = body
	return probe
}

// truncateError truncates an error message to exactly maxBytes bytes.
// P0-3I: Preserves sentinel identity separately.
// P0-3J: Final result is bounded to exactly maxBytes.
func truncateError(sentinel, err error, maxBytes int) (string, error) {
	if maxBytes <= 0 {
		return "", nil
	}
	msg := err.Error()
	if len(msg) <= maxBytes {
		return msg, nil
	}
	// Need room for "..."
	ellipsis := "..."
	if maxBytes <= len(ellipsis) {
		return strings.Repeat(".", maxBytes), nil
	}
	return msg[:maxBytes-len(ellipsis)] + ellipsis, nil
}

// CheckAPIReadiness checks if the UVB-76 API is ready.
// P0-3: Uses redirect-disabled transport to prevent proving wrong service.
// P0-3B: Validates exact service response schema using production ServerStatus type.
// P0-3H: Creates overall deadline context first, making deadline physically authoritative.
func CheckAPIReadiness(ctx context.Context, input APIReadinessInput) APIReadinessResult {
	result := APIReadinessResult{}

	// P0-3M: Validate input
	if input.URL == "" {
		result.TerminalError = ErrAPIReadinessURL
		return result
	}
	if input.PollInterval < 0 {
		result.TerminalError = fmt.Errorf("%w: negative PollInterval", ErrAPIReadinessInput)
		return result
	}
	if input.RequestLimit < 0 {
		result.TerminalError = fmt.Errorf("%w: negative RequestLimit", ErrAPIReadinessInput)
		return result
	}
	if input.Deadline < 0 {
		result.TerminalError = fmt.Errorf("%w: negative Deadline", ErrAPIReadinessInput)
		return result
	}
	if ctx == nil {
		result.TerminalError = fmt.Errorf("%w: nil context", ErrAPIReadinessInput)
		return result
	}

	// P0-3A: Build canonical status URL using net/url JoinPath
	baseURL, err := urlParse(input.URL)
	if err != nil {
		result.TerminalError = fmt.Errorf("%w: parse URL: %v", ErrAPIReadinessURL, err)
		return result
	}
	if !baseURL.IsAbs() {
		result.TerminalError = fmt.Errorf("%w: URL must be absolute", ErrAPIReadinessURL)
		return result
	}

	// P0-3R: Use canonical route constant from production server package
	// Split the constant to mechanically bind the runtime to the authoritative source
	routeParts := strings.Split(strings.TrimPrefix(UVB76StatusRoute, "/"), "/")
	statusURL := baseURL.JoinPath(routeParts...).String()
	result.URL = statusURL

	// Set defaults
	if input.PollInterval == 0 {
		input.PollInterval = 250 * time.Millisecond
	}
	if input.RequestLimit == 0 {
		input.RequestLimit = 2 * time.Second
	}
	if input.Deadline == 0 {
		input.Deadline = 30 * time.Second
	}

	// P0-3H: Create overall deadline context first
	readinessCtx, readinessCancel := context.WithTimeout(ctx, input.Deadline)
	defer readinessCancel()

	// P0-3G: Single redirect policy via CheckRedirect only (no transport wrapper)
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	ticker := time.NewTicker(input.PollInterval)
	defer ticker.Stop()

	// P0-3Q: Bounded cause set
	causes := &causeSet{}

	for {
		select {
		case <-ticker.C:
			// P0-3S: Check parent context first for deterministic precedence
			// When both ticker and context are ready, context always wins.
			if readinessCtx.Err() != nil {
				result.TerminalError = finalizeReadinessContext(readinessCtx, causes)
				return result
			}

			result.Attempts++

			// Check process exit after context check
			if input.ProcessExited != nil && input.ProcessExited() {
				// P0-3L: Process exit does not include deadline sentinel
				// P0-3V: Include earlier causes
				result.TerminalError = errors.Join(
					ErrAPIReadinessProcessExited,
					causes.Error(),
				)
				return result
			}

			// P0-3W: Per-request timeout via derived context - body read within scope
			probe := probeAPIReadinessOnce(readinessCtx, client, statusURL, input.RequestLimit)

			if probe.Err != nil {
				// P0-3S: Check if parent context terminated first
				// This makes parent termination deterministic over recoverable errors
				if readinessCtx.Err() != nil {
					result.TerminalError = finalizeReadinessContext(readinessCtx, causes)
					return result
				}

				// Transport error - recoverable
				result.RecoveredErrorCount++
				causes.transport = true
				if len(result.RecoveredErrors) < maxAPIReadinessRecoveredErrors {
					truncated, _ := truncateError(ErrAPIReadinessTransport, probe.Err, maxAPIReadinessErrorBytes)
					result.RecoveredErrors = append(result.RecoveredErrors, truncated)
				}
				continue
			}

			result.LastStatusCode = probe.StatusCode

			// P0-3N: Check for oversized body before processing
			if len(probe.Body) > maxStatusBodyBytes {
				result.RecoveredErrorCount++
				causes.decode = true
				if len(result.RecoveredErrors) < maxAPIReadinessRecoveredErrors {
					result.RecoveredErrors = append(result.RecoveredErrors,
						fmt.Sprintf("body exceeds %d bytes", maxStatusBodyBytes))
				}
				continue
			}

			// P0-3G: Check for redirect
			if probe.StatusCode >= 300 && probe.StatusCode < 400 {
				result.RecoveredErrorCount++
				causes.redirect = true
				if len(result.RecoveredErrors) < maxAPIReadinessRecoveredErrors {
					result.RecoveredErrors = append(result.RecoveredErrors,
						fmt.Sprintf("redirect status=%d", probe.StatusCode))
				}
				continue
			}

			// Validate status code
			if probe.StatusCode != http.StatusOK {
				result.RecoveredErrorCount++
				causes.status = true
				if len(result.RecoveredErrors) < maxAPIReadinessRecoveredErrors {
					result.RecoveredErrors = append(result.RecoveredErrors,
						fmt.Sprintf("HTTP status=%d", probe.StatusCode))
				}
				continue
			}

			// P0-3B: Validate exact service response schema
			var statusResp server.ServerStatus
			if err := decodeStrictJSON(probe.Body, &statusResp); err != nil {
				result.RecoveredErrorCount++
				causes.decode = true
				if len(result.RecoveredErrors) < maxAPIReadinessRecoveredErrors {
					truncated, _ := truncateError(ErrAPIReadinessDecode, err, maxAPIReadinessErrorBytes)
					result.RecoveredErrors = append(result.RecoveredErrors, truncated)
				}
				continue
			}

			// P0-3B: Production ServerStatus requires StartedAt field
			if statusResp.StartedAt == "" {
				result.RecoveredErrorCount++
				causes.wrongService = true
				if len(result.RecoveredErrors) < maxAPIReadinessRecoveredErrors {
					result.RecoveredErrors = append(result.RecoveredErrors, "missing started_at")
				}
				continue
			}

			// Success - API is ready
			result.Ready = true
			now := time.Now()
			result.ReadyAt = &now
			return result

		case <-readinessCtx.Done():
			// P0-3S: Centralized termination - deterministic
			result.TerminalError = finalizeReadinessContext(readinessCtx, causes)
			return result
		}
	}
}

// decodeStrictJSON decodes JSON with strict field checking.
func decodeStrictJSON(data []byte, v interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(v); err != nil {
		return err
	}

	// Ensure no trailing data
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON documents not allowed")
		}
		return err
	}

	return nil
}

// urlParse wraps url.Parse for testability.
var urlParse = func(rawURL string) (*url.URL, error) {
	return url.Parse(rawURL)
}
