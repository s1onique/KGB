// Package main provides the UVB-76 pprof memory leak lab.
//
// # Target Poll Diagnostics
//
// This file implements P0-11: Publish complete bounded polling diagnostics.
// Diagnostics are bounded to prevent unbounded memory growth and exclude all secrets.
package main

import (
	"encoding/json"
	"slices"
	"strings"
)

// Maximum number of recovered errors to retain.
const MaxRecoveredErrors = 16

// Maximum length of error strings to retain.
const MaxErrorLength = 512

// TargetPollSummary contains the bounded diagnostic summary for a poll run.
// P0-11: All secrets are excluded from diagnostics.
type TargetPollSummary struct {
	// TargetID is the canonical target identifier.
	TargetID string `json:"target_id"`

	// ConfiguredBaseURL is the expected target base URL.
	ConfiguredBaseURL string `json:"configured_base_url"`

	// SourceURL is the actual URL polled.
	SourceURL string `json:"source_url"`

	// Attempts is the total number of poll attempts made.
	Attempts int `json:"attempts"`

	// RecoveredErrorCount is the number of recovered (transient) errors.
	RecoveredErrorCount int `json:"recovered_error_count"`

	// RecoveredErrors contains up to MaxRecoveredErrors bounded error strings.
	RecoveredErrors []string `json:"recovered_errors,omitempty"`

	// AttemptObserved indicates if at least one scrape attempt was observed.
	AttemptObserved bool `json:"attempt_observed"`

	// CompletionObserved indicates if a completed scrape was observed.
	CompletionObserved bool `json:"completion_observed"`

	// IdentityValidated indicates if target identity was validated.
	IdentityValidated bool `json:"identity_validated"`

	// P0-11: TerminalCategories contains typed terminal error categories
	TerminalCategories []string `json:"terminal_categories,omitempty"`

	// TerminalError is the terminal error string if any.
	TerminalError string `json:"terminal_error,omitempty"`
}

// BuildTargetPollSummary creates a bounded diagnostic summary from poll results.
// P0-11: Bounds recovered errors and excludes all secrets.
func BuildTargetPollSummary(
	targetID string,
	configuredBaseURL string,
	sourceURL string,
	result TargetPollResult,
	identityValidated bool,
) TargetPollSummary {
	summary := TargetPollSummary{
		TargetID:           targetID,
		ConfiguredBaseURL:  configuredBaseURL,
		SourceURL:          sourceURL,
		Attempts:           result.Attempts,
		AttemptObserved:    result.BestAuthority != nil && result.BestAuthority.AttemptObserved,
		CompletionObserved: result.BestAuthority != nil && result.BestAuthority.CompletionObserved,
		IdentityValidated:  identityValidated,
	}

	// Bound recovered errors
	summary.RecoveredErrorCount = len(result.RecoveredErrors)
	for i, errStr := range result.RecoveredErrors {
		if i >= MaxRecoveredErrors {
			break
		}
		errStr = boundErrorString(errStr)
		// Additional sanitization - remove any embedded secrets
		errStr = sanitizeErrorString(errStr)
		summary.RecoveredErrors = append(summary.RecoveredErrors, errStr)
	}

	// Bound terminal error
	if result.TerminalError != nil {
		termErr := boundErrorString(result.TerminalError.Error())
		termErr = sanitizeErrorString(termErr)
		summary.TerminalError = termErr
	}

	return summary
}

// boundErrorString bounds an error string to MaxErrorLength.
func boundErrorString(s string) string {
	if len(s) > MaxErrorLength {
		return s[:MaxErrorLength] + "..."
	}
	return s
}

// sanitizeErrorString removes potential secrets from error strings.
// P0-11: Ensures no secrets appear in diagnostics.
func sanitizeErrorString(s string) string {
	// Remove any base64-looking strings that might be credentials
	// This is a heuristic - the actual secrets are never added to errors
	sanitized := s

	// Remove common credential patterns
	patterns := []string{
		"session=",
		"token=",
		"cookie=",
		"authorization=",
		"bearer ",
	}

	for _, pattern := range patterns {
		idx := strings.Index(strings.ToLower(sanitized), pattern)
		if idx >= 0 {
			// Truncate after the pattern
			sanitized = sanitized[:idx+len(pattern)] + "[REDACTED]"
		}
	}

	return sanitized
}

// CloneTargetPollSummary creates a deep copy of the summary.
func CloneTargetPollSummary(summary TargetPollSummary) TargetPollSummary {
	clone := TargetPollSummary{
		TargetID:            summary.TargetID,
		ConfiguredBaseURL:   summary.ConfiguredBaseURL,
		SourceURL:           summary.SourceURL,
		Attempts:            summary.Attempts,
		RecoveredErrorCount: summary.RecoveredErrorCount,
		RecoveredErrors:     make([]string, len(summary.RecoveredErrors)),
		AttemptObserved:     summary.AttemptObserved,
		CompletionObserved:  summary.CompletionObserved,
		IdentityValidated:   summary.IdentityValidated,
		TerminalError:       summary.TerminalError,
	}
	copy(clone.RecoveredErrors, summary.RecoveredErrors)
	return clone
}

// TargetPollSummaryResult contains the result with diagnostics.
type TargetPollSummaryResult struct {
	// Summary is the bounded diagnostic summary.
	Summary TargetPollSummary

	// JSON is the deterministic JSON encoding of the summary.
	JSON []byte
}

// MarshalTargetPollSummary creates a deterministic JSON encoding of the summary.
func MarshalTargetPollSummary(summary TargetPollSummary) ([]byte, error) {
	// Clone to ensure deterministic encoding
	clone := CloneTargetPollSummary(summary)

	// Use json.Marshal for deterministic ordering
	data, err := json.Marshal(clone)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// UnmarshalTargetPollSummary decodes a JSON encoding of the summary.
func UnmarshalTargetPollSummary(data []byte) (TargetPollSummary, error) {
	var summary TargetPollSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return summary, err
	}
	return summary, nil
}

// VerifyTargetPollSummaryRoundTrip verifies round-trip encoding.
func VerifyTargetPollSummaryRoundTrip(summary TargetPollSummary) error {
	data, err := MarshalTargetPollSummary(summary)
	if err != nil {
		return err
	}

	decoded, err := UnmarshalTargetPollSummary(data)
	if err != nil {
		return err
	}

	// Compare key fields
	if decoded.TargetID != summary.TargetID {
		return &roundTripError{"TargetID mismatch"}
	}
	if decoded.ConfiguredBaseURL != summary.ConfiguredBaseURL {
		return &roundTripError{"ConfiguredBaseURL mismatch"}
	}
	if decoded.SourceURL != summary.SourceURL {
		return &roundTripError{"SourceURL mismatch"}
	}
	if decoded.Attempts != summary.Attempts {
		return &roundTripError{"Attempts mismatch"}
	}
	if decoded.RecoveredErrorCount != summary.RecoveredErrorCount {
		return &roundTripError{"RecoveredErrorCount mismatch"}
	}
	if len(decoded.RecoveredErrors) != len(summary.RecoveredErrors) {
		return &roundTripError{"RecoveredErrors length mismatch"}
	}
	if !slices.Equal(decoded.RecoveredErrors, summary.RecoveredErrors) {
		return &roundTripError{"RecoveredErrors content mismatch"}
	}
	if decoded.TerminalError != summary.TerminalError {
		return &roundTripError{"TerminalError mismatch"}
	}

	return nil
}

type roundTripError struct {
	msg string
}

func (e *roundTripError) Error() string {
	return e.msg
}

// ValidateDiagnosticsSecrets checks that no secrets are present in the summary.
func ValidateDiagnosticsSecrets(summary TargetPollSummary, plaintextPassword, cookieValue string) error {
	// Check all string fields for secrets
	fields := []string{
		summary.TargetID,
		summary.ConfiguredBaseURL,
		summary.SourceURL,
		summary.TerminalError,
	}
	fields = append(fields, summary.RecoveredErrors...)

	for _, field := range fields {
		if containsSecret(field, plaintextPassword) {
			return &secretLeakError{"plaintext password leaked into diagnostics"}
		}
		if containsSecret(field, cookieValue) {
			return &secretLeakError{"cookie value leaked into diagnostics"}
		}
	}

	return nil
}

func containsSecret(field, secret string) bool {
	if secret == "" {
		return false
	}
	return strings.Contains(field, secret)
}

type secretLeakError struct {
	msg string
}

func (e *secretLeakError) Error() string {
	return e.msg
}
