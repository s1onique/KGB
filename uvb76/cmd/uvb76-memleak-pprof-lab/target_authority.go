// Package main provides the UVB-76 pprof memory leak lab.
//
// # Target Authority Discovery
//
// This file implements P0-1: Discovery of production UVB-76 target-state authority.
// The canonical route for target-state is GET /api/v1/targets/{id}/snapshot
// which returns TargetSnapshot from the state manager.
//
// Key observations about scrape authority:
// - TargetSnapshot.ScrapedAt serves as attempt_timestamp
// - Reachable=true with Error="" indicates completed scrape
// - No explicit "completed" flag exists; completion inferred from successful parse
// - The target identity (id, base_url) comes from the snapshot
//
// Authentication: All /api/v1/targets/* routes require session auth.
// The lab must handle authentication for target-state queries.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TargetStateAuthority represents the authoritative target-state from UVB-76.
// This is the strict lab projection over the production TargetSnapshot.
type TargetStateAuthority struct {
	// Identity fields (from config, validated against response)
	TargetID          string `json:"target_id"`
	ConfiguredBaseURL string `json:"configured_base_url,omitempty"`

	// Attempt authority
	AttemptObserved       bool      `json:"attempt_observed"`
	AttemptTimestamp      time.Time `json:"attempt_timestamp"`
	AttemptTimestampValid bool      `json:"attempt_timestamp_valid"`

	// Completion authority
	CompletionObserved       bool      `json:"completion_observed"`
	CompletionTimestamp      time.Time `json:"completion_timestamp"`
	CompletionTimestampValid bool      `json:"completion_timestamp_valid"`

	// Result authority
	Reachable bool   `json:"reachable"`
	Status    string `json:"status,omitempty"`
	Error     string `json:"error,omitempty"`

	// Source URL for debugging (from where we fetched)
	SourceURL string `json:"source_url,omitempty"`
}

// productionTargetSnapshot is the raw response from /api/v1/targets/{id}/snapshot
type productionTargetSnapshot struct {
	TargetID    string `json:"target_id"`
	BaseURL     string `json:"base_url,omitempty"` // P0-1: Capture the configured base URL from snapshot
	ScrapedAt   string `json:"scraped_at"`
	Reachable   bool   `json:"reachable"`
	Status      string `json:"status,omitempty"`
	PeerVersion string `json:"peer_version,omitempty"`
	NodeID      string `json:"node_id,omitempty"`
	Error       string `json:"error,omitempty"`
	RawResponse string `json:"raw_response,omitempty"`
}

// FetchTargetState queries UVB-76 for authoritative target-state.
// Returns nil if the target doesn't exist or can't be reached.
//
// P0-1: Uses canonical route GET /api/v1/targets/{id}/snapshot
// P0-2: Derives attempt/completion from explicit production state
func FetchTargetState(uvb76Port, targetID, sessionCookie string) (*TargetStateAuthority, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("http://localhost:%s/api/v1/targets/%s/snapshot", uvb76Port, targetID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Add session auth if provided
	if sessionCookie != "" {
		req.AddCookie(&http.Cookie{Name: "session", Value: sessionCookie})
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch target state: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Target doesn't exist
		return nil, nil
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized: session required for target-state endpoint")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	// Decode with strict field matching - unknown fields cause decode failure
	decoder := json.NewDecoder(resp.Body)
	decoder.DisallowUnknownFields()

	var snap productionTargetSnapshot
	if err := decoder.Decode(&snap); err != nil {
		return nil, fmt.Errorf("decode target snapshot: %w", err)
	}

	// P0-2: Require exact-one JSON document (same strict check as result_persist.go)
	// A second Decode returns nil, not io.EOF, so this rejects a second valid document
	var trailingCheck struct{}
	if err := decoder.Decode(&trailingCheck); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON documents in response (expected exactly one)")
		}
		return nil, fmt.Errorf("expected EOF after snapshot: %w", err)
	}

	// Also verify no trailing content
	body, _ := io.ReadAll(resp.Body)
	if len(body) > 0 {
		trimmed := strings.TrimSpace(string(body))
		if trimmed != "" {
			return nil, fmt.Errorf("unexpected trailing content after JSON: %q", trimmed)
		}
	}

	// P0-1: Populate ConfiguredBaseURL from snapshot for URL identity binding
	auth := &TargetStateAuthority{
		TargetID:          snap.TargetID,
		ConfiguredBaseURL: snap.BaseURL, // P0-1: URL identity from production snapshot
		SourceURL:         url,
	}

	// Attempt authority: ScrapedAt is set on every scrape attempt
	if snap.ScrapedAt != "" {
		auth.AttemptObserved = true
		if t, err := time.Parse(time.RFC3339, snap.ScrapedAt); err == nil {
			auth.AttemptTimestamp = t
			auth.AttemptTimestampValid = true
		}
	}

	// Completion authority: scrape completed if reachable and no error
	// Note: UVB-76 doesn't have explicit "completed" flag; completion inferred from:
	// - Reachable=true (HTTP request succeeded)
	// - Error="" (no error recorded)
	// - RawResponse present (valid JSON was parsed)
	auth.Reachable = snap.Reachable
	auth.Status = snap.Status
	auth.Error = snap.Error

	if snap.Reachable && snap.Error == "" && snap.RawResponse == "" {
		// If reachable but no raw response, it might not have been fully parsed
		// However, if Status is set, the scrape was complete
		if snap.Status != "" {
			auth.CompletionObserved = true
			auth.CompletionTimestamp = auth.AttemptTimestamp
			auth.CompletionTimestampValid = auth.AttemptTimestampValid
		}
	} else if snap.Reachable && snap.Error == "" {
		// Standard completion: reachable with no error
		auth.CompletionObserved = true
		auth.CompletionTimestamp = auth.AttemptTimestamp
		auth.CompletionTimestampValid = auth.AttemptTimestampValid
	}

	return auth, nil
}

// ValidateTargetIdentity checks that the observed target matches expected configuration.
// P0-2: Target identity must match generated config; fail closed when URL validation skipped.
// This function returns nil ONLY when both ID and (if expected) URL match exactly.
func ValidateTargetIdentity(auth *TargetStateAuthority, expectedID, expectedBaseURL string) error {
	if auth == nil {
		return fmt.Errorf("nil authority")
	}

	if auth.TargetID != expectedID {
		return fmt.Errorf("target ID mismatch: got %q, want %q", auth.TargetID, expectedID)
	}

	// P0-2: Fail closed - if we expect URL validation but it's absent, reject
	if expectedBaseURL != "" {
		if auth.ConfiguredBaseURL == "" {
			// Observed URL is absent - fail closed instead of silently skipping
			return fmt.Errorf("configured base URL expected but not observed in snapshot")
		}
		// Strip trailing slash for comparison
		got := strings.TrimSuffix(auth.ConfiguredBaseURL, "/")
		want := strings.TrimSuffix(expectedBaseURL, "/")
		if got != want {
			return fmt.Errorf("base URL mismatch: got %q, want %q", got, want)
		}
	}

	return nil
}

// IsScrapeCompleted returns true if the authority indicates a completed scrape.
// A scrape is completed when:
// - AttemptObserved is true
// - CompletionObserved is true
// - Reachable is true
// - Error is empty
func (a *TargetStateAuthority) IsScrapeCompleted() bool {
	return a.AttemptObserved && a.CompletionObserved && a.Reachable && a.Error == ""
}

// IsScrapeAttempted returns true if at least one scrape attempt was observed.
func (a *TargetStateAuthority) IsScrapeAttempted() bool {
	return a.AttemptObserved && a.AttemptTimestampValid
}
