// Package polling provides typed polling logic for UVB-76 capture netns lab.
package polling

import (
	"encoding/json"
	"testing"
	"time"
)

// --- Status Normalization Tests ---

func TestNormalizeStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected CapturedStatus
	}{
		{"ok", StatusCaptured},
		{"captured", StatusCaptured},
		{"timeout", StatusFailed},
		{"error", StatusFailed},
		{"failed", StatusFailed},
		{"skipped_cooldown", StatusSkippedCooldown},
		{"disabled", StatusDisabled},
		{"not_configured", StatusNotConfigured},
		{"not_attempted", StatusNotAttempted},
		{"in_progress", StatusPending},
		{"pending", StatusPending},
		{"", StatusPending},
		{"unknown_api_status", StatusUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := NormalizeStatus(tc.input)
			if got != tc.expected {
				t.Errorf("NormalizeStatus(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestCaptureExtractLifecycleStatus(t *testing.T) {
	tests := []struct {
		name           string
		captureStatus  string
		status         string
		expectedStatus string
	}{
		{"capture_status wins", "captured", "ok", "captured"},
		{"capture_status empty falls back to status", "", "timeout", "timeout"},
		{"capture_status non-empty wins", "skipped_cooldown", "ok", "skipped_cooldown"},
		{"both empty", "", "", "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &Capture{
				CaptureStatus: tc.captureStatus,
				Status:        tc.status,
			}
			got := c.ExtractLifecycleStatus()
			if got != tc.expectedStatus {
				t.Errorf("ExtractLifecycleStatus() = %q, want %q", got, tc.expectedStatus)
			}
		})
	}
}

func TestCaptureExtractNormalizedStatus(t *testing.T) {
	c := &Capture{CaptureStatus: "ok", Status: "timeout"}
	if got := c.ExtractNormalizedStatus(); got != StatusCaptured {
		t.Errorf("ExtractNormalizedStatus() = %q, want %q", got, StatusCaptured)
	}
}

// --- IsAfterCursor Tests ---

func TestIsAfterCursor(t *testing.T) {
	now := "2024-01-01T01:00:00Z"
	before := "2024-01-01T00:30:00Z"
	after := "2024-01-01T01:30:00Z"

	tests := []struct {
		name      string
		timestamp string
		cursor    string
		expected  bool
	}{
		{"empty cursor always true", "2024-01-01T00:00:00Z", "", true},
		{"empty timestamp false", "", "2024-01-01T00:00:00Z", false},
		{"after cursor true", after, now, true},
		{"before cursor false", before, now, false},
		{"same timestamp false", now, now, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsAfterCursor(tc.timestamp, tc.cursor)
			if got != tc.expected {
				t.Errorf("IsAfterCursor(%q, %q) = %v, want %v", tc.timestamp, tc.cursor, got, tc.expected)
			}
		})
	}
}

// --- MatchReasons Tests ---

func TestMatchReasons(t *testing.T) {
	tests := []struct {
		name      string
		reasons   []string
		pattern   string
		expected  bool
	}{
		{"empty pattern matches all", []string{"foo"}, "", true},
		{"match first reason", []string{"http_probe_timeout", "error"}, `http_probe_timeout`, true},
		{"match second reason", []string{"foo", "http_probe_failure"}, `http_probe_timeout`, false},
		{"no match", []string{"ok"}, `http_probe`, false},
		{"empty reasons", []string{}, `http_probe`, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchReasons(tc.reasons, tc.pattern)
			if got != tc.expected {
				t.Errorf("MatchReasons(%v, %q) = %v, want %v", tc.reasons, tc.pattern, got, tc.expected)
			}
		})
	}
}

// --- ReasonsJoin Tests ---

func TestReasonsJoin(t *testing.T) {
	tests := []struct {
		input    []string
		expected string
	}{
		{[]string{}, ""},
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a|b"},
		{[]string{"http_probe_timeout", "http_probe_failure"}, "http_probe_timeout|http_probe_failure"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			got := ReasonsJoin(tc.input)
			if got != tc.expected {
				t.Errorf("ReasonsJoin(%v) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

// --- Default Config Tests ---

func TestDefaultPollConfig(t *testing.T) {
	cfg := DefaultPollConfig()
	if cfg.Interval != PollInterval {
		t.Errorf("DefaultPollConfig().Interval = %v, want PollInterval (%v)", cfg.Interval, PollInterval)
	}
	if cfg.Timeout != DefaultTimeout {
		t.Errorf("DefaultPollConfig().Timeout = %v, want %v", cfg.Timeout, DefaultTimeout)
	}
	if cfg.RequireCount != 2 {
		t.Errorf("DefaultPollConfig().RequireCount = %d, want 2", cfg.RequireCount)
	}
}

func TestDefaultSpikeEventConfig(t *testing.T) {
	cfg := DefaultSpikeEventConfig()
	if cfg.Interval != 2*time.Second {
		t.Errorf("DefaultSpikeEventConfig().Interval = %v, want 2s", cfg.Interval)
	}
	if cfg.Timeout != DefaultTimeout {
		t.Errorf("DefaultSpikeEventConfig().Timeout = %v, want %v", cfg.Timeout, DefaultTimeout)
	}
	if cfg.ReasonRegex == "" {
		t.Errorf("DefaultSpikeEventConfig().ReasonRegex = empty, want non-empty")
	}
}

func TestDefaultCapturePollConfig(t *testing.T) {
	cfg := DefaultCapturePollConfig()
	if cfg.Interval != 2*time.Second {
		t.Errorf("DefaultCapturePollConfig().Interval = %v, want 2s", cfg.Interval)
	}
	if cfg.Timeout != DefaultTimeout {
		t.Errorf("DefaultCapturePollConfig().Timeout = %v, want %v", cfg.Timeout, DefaultTimeout)
	}
	if !cfg.RequireCapture {
		t.Errorf("DefaultCapturePollConfig().RequireCapture = false, want true")
	}
}

// --- SpikeEventConfig CompileReasonRegex ---

func TestSpikeEventConfig_CompileReasonRegex(t *testing.T) {
	cfg := SpikeEventConfig{ReasonRegex: `http_probe_timeout|http_probe_failure`}
	re, err := cfg.CompileReasonRegex()
	if err != nil {
		t.Fatalf("CompileReasonRegex() error = %v", err)
	}
	if !re.MatchString("http_probe_timeout") {
		t.Errorf("Compiled regex should match http_probe_timeout")
	}
	if re.MatchString("foo") {
		t.Errorf("Compiled regex should not match foo")
	}
}

// --- JSON Parsing Edge Cases ---

func TestParseSpikesResponse_MissingCaptures(t *testing.T) {
	data := []byte(`{"count":1,"spikes":[{"event_id":"evt1","sample_ts":"2024-01-01T00:00:00Z","reasons":["foo"]}]}`)
	var resp SpikesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}
	if len(resp.Spikes) != 1 {
		t.Fatalf("Expected 1 spike, got %d", len(resp.Spikes))
	}
	if resp.Spikes[0].Captures != nil {
		t.Errorf("Expected nil captures, got %v", resp.Spikes[0].Captures)
	}
}

func TestParseSpikesResponse_EmptySpikes(t *testing.T) {
	data := []byte(`{"count":0,"spikes":[]}`)
	var resp SpikesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}
	if resp.Count != 0 {
		t.Errorf("Expected Count=0, got %d", resp.Count)
	}
	if len(resp.Spikes) != 0 {
		t.Errorf("Expected empty spikes, got %d", len(resp.Spikes))
	}
}
