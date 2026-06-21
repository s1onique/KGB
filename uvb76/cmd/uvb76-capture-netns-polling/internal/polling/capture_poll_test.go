// Package polling provides typed polling logic for UVB-76 capture netns lab.
package polling

import (
	"context"
	"testing"
	"time"
)

// --- Capture Polling Tests ---

func TestPollCaptureForEvent_ImmediateCaptured(t *testing.T) {
	spikes := spikesResponseJSON(1, `[`+spikeJSON("evt123", "2024-01-01T00:01:00Z", []string{"http_probe_timeout"}, captureJSON("ok", "captured"))+`]`)
	mockClient := &MockHTTPClient{
		Responses:   [][]byte{spikes},
		StatusCodes: []int{200},
	}

	client := &APIClient{
		BaseURL:    "http://localhost:9999",
		Username:   "admin",
		Password:   "test",
		HTTPClient: mockClient,
	}

	poller := &Poller{
		APIClient: client,
		Clock:     NewMockClock(),
	}

	cfg := CapturePollConfig{Interval: 1 * time.Second, Timeout: 30 * time.Second, RequireCapture: true}
	ctx := context.Background()

	result := poller.PollCaptureForEvent(ctx, "lab-tovarisch", "evt123", cfg)

	if !result.OK {
		t.Errorf("PollCaptureForEvent() OK = false, want true")
	}
	if result.CaptureStatus != StatusCaptured {
		t.Errorf("PollCaptureForEvent() CaptureStatus = %q, want %q", result.CaptureStatus, StatusCaptured)
	}
}

func TestPollCaptureForEvent_FailedStatus(t *testing.T) {
	spikes := spikesResponseJSON(1, `[`+spikeJSON("evt123", "2024-01-01T00:01:00Z", []string{"http_probe_timeout"}, captureJSON("timeout", ""))+`]`)
	mockClient := &MockHTTPClient{
		Responses:   [][]byte{spikes},
		StatusCodes: []int{200},
	}

	client := &APIClient{
		BaseURL:    "http://localhost:9999",
		Username:   "admin",
		Password:   "test",
		HTTPClient: mockClient,
	}

	poller := &Poller{
		APIClient: client,
		Clock:     NewMockClock(),
	}

	cfg := CapturePollConfig{Interval: 1 * time.Second, Timeout: 30 * time.Second, RequireCapture: true}
	ctx := context.Background()

	result := poller.PollCaptureForEvent(ctx, "lab-tovarisch", "evt123", cfg)

	if result.OK {
		t.Errorf("PollCaptureForEvent() OK = true, want false (failed status)")
	}
	if result.CaptureStatus != StatusFailed {
		t.Errorf("PollCaptureForEvent() CaptureStatus = %q, want %q", result.CaptureStatus, StatusFailed)
	}
	if result.FailureReason == "" {
		t.Errorf("PollCaptureForEvent() FailureReason = empty, want non-empty")
	}
}

func TestPollCaptureForEvent_SkippedCooldown(t *testing.T) {
	spikes := spikesResponseJSON(1, `[`+spikeJSON("evt123", "2024-01-01T00:01:00Z", []string{"http_probe_timeout"}, captureJSON("ok", "skipped_cooldown"))+`]`)
	mockClient := &MockHTTPClient{
		Responses:   [][]byte{spikes},
		StatusCodes: []int{200},
	}

	client := &APIClient{
		BaseURL:    "http://localhost:9999",
		Username:   "admin",
		Password:   "test",
		HTTPClient: mockClient,
	}

	poller := &Poller{
		APIClient: client,
		Clock:     NewMockClock(),
	}

	cfg := CapturePollConfig{Interval: 1 * time.Second, Timeout: 30 * time.Second, RequireCapture: true}
	ctx := context.Background()

	result := poller.PollCaptureForEvent(ctx, "lab-tovarisch", "evt123", cfg)

	if result.OK {
		t.Errorf("PollCaptureForEvent() OK = true, want false (skipped_cooldown)")
	}
	if result.CaptureStatus != StatusSkippedCooldown {
		t.Errorf("PollCaptureForEvent() CaptureStatus = %q, want %q", result.CaptureStatus, StatusSkippedCooldown)
	}
}

func TestPollCaptureForEvent_NoCaptures(t *testing.T) {
	spikes := spikesResponseJSON(1, `[`+spikeJSON("evt123", "2024-01-01T00:01:00Z", []string{"http_probe_timeout"}, `[]`)+`]`)
	mockClient := &MockHTTPClient{
		Responses:   [][]byte{spikes},
		StatusCodes: []int{200},
	}

	client := &APIClient{
		BaseURL:    "http://localhost:9999",
		Username:   "admin",
		Password:   "test",
		HTTPClient: mockClient,
	}

	mockClock := NewMockClock()
	poller := &Poller{
		APIClient: client,
		Clock:     mockClock,
	}

	cfg := CapturePollConfig{Interval: 1 * time.Second, Timeout: 3 * time.Second, RequireCapture: true}
	ctx := context.Background()

	result := poller.PollCaptureForEvent(ctx, "lab-tovarisch", "evt123", cfg)

	if result.OK {
		t.Errorf("PollCaptureForEvent() OK = true, want false (no captures)")
	}
	if !result.Timeout {
		t.Errorf("PollCaptureForEvent() Timeout = false, want true")
	}
}

func TestPollCaptureForEvent_EventNotFound(t *testing.T) {
	spikes := spikesResponseJSON(1, `[`+spikeJSON("evt-other", "2024-01-01T00:01:00Z", []string{"http_probe_timeout"}, captureJSON("ok", "captured"))+`]`)
	mockClient := &MockHTTPClient{
		Responses:   [][]byte{spikes},
		StatusCodes: []int{200},
	}

	client := &APIClient{
		BaseURL:    "http://localhost:9999",
		Username:   "admin",
		Password:   "test",
		HTTPClient: mockClient,
	}

	mockClock := NewMockClock()
	poller := &Poller{
		APIClient: client,
		Clock:     mockClock,
	}

	cfg := CapturePollConfig{Interval: 1 * time.Second, Timeout: 3 * time.Second, RequireCapture: true}
	ctx := context.Background()

	result := poller.PollCaptureForEvent(ctx, "lab-tovarisch", "evt123", cfg)

	if result.OK {
		t.Errorf("PollCaptureForEvent() OK = true, want false (event not found)")
	}
	if !result.Timeout {
		t.Errorf("PollCaptureForEvent() Timeout = false, want true")
	}
}

func TestPollCaptureForEvent_NonCaptured_WithoutRequireCapture(t *testing.T) {
	spikes := spikesResponseJSON(1, `[`+spikeJSON("evt123", "2024-01-01T00:01:00Z", []string{"http_probe_timeout"}, captureJSON("timeout", ""))+`]`)
	mockClient := &MockHTTPClient{
		Responses:   [][]byte{spikes},
		StatusCodes: []int{200},
	}

	client := &APIClient{
		BaseURL:    "http://localhost:9999",
		Username:   "admin",
		Password:   "test",
		HTTPClient: mockClient,
	}

	poller := &Poller{
		APIClient: client,
		Clock:     NewMockClock(),
	}

	cfg := CapturePollConfig{Interval: 1 * time.Second, Timeout: 30 * time.Second, RequireCapture: false}
	ctx := context.Background()

	result := poller.PollCaptureForEvent(ctx, "lab-tovarisch", "evt123", cfg)

	if !result.OK {
		t.Errorf("PollCaptureForEvent() OK = false, want true (RequireCapture=false)")
	}
	if result.CaptureStatus != StatusFailed {
		t.Errorf("PollCaptureForEvent() CaptureStatus = %q, want %q", result.CaptureStatus, StatusFailed)
	}
}

// --- Artifact Writer Tests ---

func TestPoller_WritesArtifacts(t *testing.T) {
	spikes := spikesResponseJSON(1, `[`+spikeJSON("evt123", "2024-01-01T00:01:00Z", []string{"http_probe_timeout"}, captureJSON("ok", "captured"))+`]`)
	mockClient := &MockHTTPClient{
		Responses:   [][]byte{latencySeriesJSON(5, 10, 0), spikes, spikes},
		StatusCodes: []int{200, 200, 200},
	}

	client := &APIClient{
		BaseURL:    "http://localhost:9999",
		Username:   "admin",
		Password:   "test",
		HTTPClient: mockClient,
	}

	writer := &MockArtifactWriter{}
	poller := &Poller{
		APIClient:      client,
		ArtifactWriter: writer,
		Clock:          NewMockClock(),
	}

	ctx := context.Background()

	// Poll probe samples
	cfg := PollConfig{Interval: 1 * time.Second, Timeout: 60 * time.Second, RequireCount: 2}
	poller.PollProbeSamples(ctx, "lab-tovarisch", "http", 120, cfg)

	if !writer.ProbeReadyCalled {
		t.Errorf("PollProbeSamples() did not write probe ready artifact")
	}

	// Poll spike event
	writer.ProbeReadyCalled = false
	spikeCfg := SpikeEventConfig{Interval: 1 * time.Second, Timeout: 60 * time.Second, ReasonRegex: `http_probe_timeout`}
	poller.PollSpikeEvent(ctx, "lab-tovarisch", "", `http_probe_timeout`, spikeCfg)

	if !writer.SpikeEventCalled {
		t.Errorf("PollSpikeEvent() did not write spike event artifact")
	}

	// Poll capture
	writer.SpikeEventCalled = false
	captureCfg := CapturePollConfig{Interval: 1 * time.Second, Timeout: 60 * time.Second, RequireCapture: true}
	poller.PollCaptureForEvent(ctx, "lab-tovarisch", "evt123", captureCfg)

	if !writer.CaptureCalled {
		t.Errorf("PollCaptureForEvent() did not write capture artifact")
	}
}
