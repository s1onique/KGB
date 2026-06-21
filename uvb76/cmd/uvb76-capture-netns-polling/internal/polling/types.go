// Package polling provides typed polling logic for UVB-76 capture netns lab.
//
// This package owns:
//   - API polling loop with context.Context deadline
//   - timeout/deadline handling
//   - capture status extraction
//   - terminal condition decisions
//   - JSON artifact reading/writing for polling results
//   - deterministic failure messages
//
// The shell script should not parse JSON or decide polling state after migration.
package polling

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// CapturedStatus represents canonical capture lifecycle status.
type CapturedStatus string

const (
	StatusCaptured        CapturedStatus = "captured"
	StatusFailed          CapturedStatus = "failed"
	StatusSkippedCooldown CapturedStatus = "skipped_cooldown"
	StatusDisabled        CapturedStatus = "disabled"
	StatusNotConfigured   CapturedStatus = "not_configured"
	StatusNotAttempted    CapturedStatus = "not_attempted"
	StatusPending         CapturedStatus = "pending"
	StatusUnknown         CapturedStatus = "unknown"
)

// StatusNormalization maps API status strings to canonical CapturedStatus.
var StatusNormalization = map[string]CapturedStatus{
	"ok":                StatusCaptured,
	"captured":          StatusCaptured,
	"timeout":           StatusFailed,
	"error":             StatusFailed,
	"failed":            StatusFailed,
	"skipped_cooldown":  StatusSkippedCooldown,
	"disabled":          StatusDisabled,
	"not_configured":    StatusNotConfigured,
	"not_attempted":     StatusNotAttempted,
	"in_progress":       StatusPending,
	"pending":           StatusPending,
}

// NormalizeStatus normalizes API capture status to contract canonical status.
func NormalizeStatus(apiStatus string) CapturedStatus {
	if apiStatus == "" {
		return StatusPending
	}
	if normalized, ok := StatusNormalization[apiStatus]; ok {
		return normalized
	}
	return StatusUnknown
}

// LatencySeries represents the /api/v1/latency/series endpoint response.
type LatencySeries struct {
	RetainedSampleCount  int `json:"retained_sample_count"`
	ReturnedPointCount   int `json:"returned_point_count"`
	SampleCount          int `json:"sample_count"` // deprecated but still supported
}

// SpikesResponse represents the /api/v1/latency/spikes endpoint response.
type SpikesResponse struct {
	Count  int     `json:"count"`
	Spikes []Spike `json:"spikes"`
}

// Spike represents a latency spike event.
type Spike struct {
	EventID  string     `json:"event_id"`
	SampleTS string     `json:"sample_ts"`
	Reasons  []string   `json:"reasons"`
	Captures []Capture  `json:"captures"`
}

// Capture represents a diagnostic capture associated with a spike.
type Capture struct {
	CaptureStatus    string `json:"capture_status"`
	Status           string `json:"status"`
	CaptureStartedAt string `json:"capture_started_at"`
}

// ExtractLifecycleStatus extracts lifecycle status from capture using the rule:
// - Use .capture_status if present and non-empty
// - Fall back to .status
func (c *Capture) ExtractLifecycleStatus() string {
	if c.CaptureStatus != "" {
		return c.CaptureStatus
	}
	if c.Status != "" {
		return c.Status
	}
	return "unknown"
}

// ExtractNormalizedStatus returns the canonical CapturedStatus for a capture.
func (c *Capture) ExtractNormalizedStatus() CapturedStatus {
	return NormalizeStatus(c.ExtractLifecycleStatus())
}

// ProbePollResult represents the result of probe sample polling.
type ProbePollResult struct {
	OK              bool
	SampleCount     int
	PointCount      int
	Timeout         bool
	LastResponse    *LatencySeries
	Error           error
}

// SpikeEventResult represents the result of spike event polling.
type SpikeEventResult struct {
	OK           bool
	EventID      string
	Reasons      []string
	Timeout      bool
	LastResponse *SpikesResponse
	Error        error
}

// CaptureResult represents the result of capture polling for a specific event.
type CaptureResult struct {
	OK              bool
	CaptureStatus   CapturedStatus
	RawStatus       string
	DiagStatus      string
	CaptureStarted  string
	Timeout         bool
	LastResponse    *SpikesResponse
	FailureReason   string
	Error           error
}

// PollConfig holds configuration for polling operations.
type PollConfig struct {
	Interval    time.Duration
	Timeout     time.Duration
	RequireCount int // minimum sample/capture count to consider success
}

// Default timeout duration.
const DefaultTimeout = 30 * time.Second

// Default poll interval duration.
const PollInterval = 2 * time.Second

// DefaultPollConfig returns sensible defaults for lab polling.
func DefaultPollConfig() PollConfig {
	return PollConfig{
		Interval:     PollInterval,
		Timeout:      DefaultTimeout,
		RequireCount: 2,
	}
}

// SpikeEventConfig holds configuration for spike event polling.
type SpikeEventConfig struct {
	Interval    time.Duration
	Timeout     time.Duration
	ReasonRegex string // regex pattern to match spike reasons
}

// DefaultSpikeEventConfig returns sensible defaults for spike event polling.
func DefaultSpikeEventConfig() SpikeEventConfig {
	return SpikeEventConfig{
		Interval:    2 * time.Second,
		Timeout:     30 * time.Second,
		ReasonRegex: `http_probe_timeout|http_probe_failure|http_probe_connection_refused`,
	}
}

// CompileReasonRegex compiles the reason regex pattern.
func (c SpikeEventConfig) CompileReasonRegex() (*regexp.Regexp, error) {
	return regexp.Compile(c.ReasonRegex)
}

// CapturePollConfig holds configuration for capture polling.
type CapturePollConfig struct {
	Interval       time.Duration
	Timeout        time.Duration
	RequireCapture bool // if true, require capture_status=captured
}

// DefaultCapturePollConfig returns sensible defaults for capture polling.
func DefaultCapturePollConfig() CapturePollConfig {
	return CapturePollConfig{
		Interval:       2 * time.Second,
		Timeout:        30 * time.Second,
		RequireCapture: true,
	}
}

// ArtifactWriter defines the interface for writing polling artifacts.
type ArtifactWriter interface {
	WriteProbeReadyArtifact(series *LatencySeries) error
	WriteSpikeEventArtifact(response *SpikesResponse) error
	WriteCaptureArtifact(response *SpikesResponse) error
}

// FileArtifactWriter writes artifacts to files.
type FileArtifactWriter struct {
	ProbeReadyPath   string
	SpikeEventPath   string
	CapturePath      string
}

// WriteProbeReadyArtifact writes latency series to probe ready artifact file.
// Writes raw LatencySeries JSON for backward compatibility with existing verifiers.
func (w *FileArtifactWriter) WriteProbeReadyArtifact(series *LatencySeries) error {
	if w.ProbeReadyPath == "" || series == nil {
		return nil
	}
	data, err := json.MarshalIndent(series, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal probe ready: %w", err)
	}
	return writeFile(w.ProbeReadyPath, data)
}

// WriteSpikeEventArtifact writes spikes response to spike event artifact file.
func (w *FileArtifactWriter) WriteSpikeEventArtifact(response *SpikesResponse) error {
	if w.SpikeEventPath == "" || response == nil {
		return nil
	}
	return writeJSONFile(w.SpikeEventPath, response)
}

// WriteCaptureArtifact writes spikes response to capture artifact file.
func (w *FileArtifactWriter) WriteCaptureArtifact(response *SpikesResponse) error {
	if w.CapturePath == "" || response == nil {
		return nil
	}
	return writeJSONFile(w.CapturePath, response)
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

func writeJSONFile(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// IsAfterCursor returns true if timestamp is after the cursor.
func IsAfterCursor(timestamp, cursor string) bool {
	if cursor == "" {
		return true
	}
	if timestamp == "" {
		return false
	}
	// Parse timestamps and compare
	ts, err1 := time.Parse(time.RFC3339, timestamp)
	cs, err2 := time.Parse(time.RFC3339, cursor)
	if err1 != nil || err2 != nil {
		// Fall back to string comparison if parsing fails
		return timestamp > cursor
	}
	return ts.After(cs)
}

// MatchReasons checks if any spike reason matches the given regex pattern.
func MatchReasons(reasons []string, pattern string) bool {
	if pattern == "" {
		return true
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	for _, r := range reasons {
		if re.MatchString(r) {
			return true
		}
	}
	return false
}

// ReasonsJoin joins spike reasons with pipe separator.
func ReasonsJoin(reasons []string) string {
	return strings.Join(reasons, "|")
}
