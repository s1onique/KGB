// Package polling provides typed polling logic for UVB-76 capture netns lab.
package polling

import (
	"context"
	"testing"
	"time"
)

// --- Spike Event Polling Tests ---

func TestPollSpikeEvent_ImmediateMatch(t *testing.T) {
	spikes := spikesResponseJSON(1, `[`+spikeJSON("evt123", "2024-01-01T00:01:00Z", []string{"http_probe_timeout", "foo"}, `[]`)+`]`)
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

	cfg := SpikeEventConfig{Interval: 1 * time.Second, Timeout: 30 * time.Second, ReasonRegex: `http_probe_timeout`}
	ctx := context.Background()

	result := poller.PollSpikeEvent(ctx, "lab-tovarisch", "", `http_probe_timeout`, cfg)

	if !result.OK {
		t.Errorf("PollSpikeEvent() OK = false, want true")
	}
	if result.EventID != "evt123" {
		t.Errorf("PollSpikeEvent() EventID = %q, want %q", result.EventID, "evt123")
	}
}

func TestPollSpikeEvent_CursorFilter(t *testing.T) {
	// Spike before cursor should not match
	spikes := spikesResponseJSON(2, `[`+
		spikeJSON("evt-old", "2024-01-01T00:00:00Z", []string{"http_probe_timeout"}, `[]`)+`,`+
		spikeJSON("evt-new", "2024-01-01T01:00:00Z", []string{"http_probe_timeout"}, `[]`)+`]`)
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

	cfg := SpikeEventConfig{Interval: 1 * time.Second, Timeout: 30 * time.Second, ReasonRegex: `http_probe_timeout`}
	ctx := context.Background()

	// Cursor is after evt-old but before evt-new
	result := poller.PollSpikeEvent(ctx, "lab-tovarisch", "2024-01-01T00:30:00Z", `http_probe_timeout`, cfg)

	if !result.OK {
		t.Errorf("PollSpikeEvent() OK = false, want true")
	}
	if result.EventID != "evt-new" {
		t.Errorf("PollSpikeEvent() EventID = %q, want %q (after cursor)", result.EventID, "evt-new")
	}
}

func TestPollSpikeEvent_Timeout(t *testing.T) {
	spikes := spikesResponseJSON(1, `[`+spikeJSON("evt123", "2024-01-01T00:01:00Z", []string{"foo"}, `[]`)+`]`)
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

	cfg := SpikeEventConfig{Interval: 1 * time.Second, Timeout: 3 * time.Second, ReasonRegex: `http_probe_timeout`}
	ctx := context.Background()

	result := poller.PollSpikeEvent(ctx, "lab-tovarisch", "", `http_probe_timeout`, cfg)

	if result.OK {
		t.Errorf("PollSpikeEvent() OK = true, want false (timeout)")
	}
	if !result.Timeout {
		t.Errorf("PollSpikeEvent() Timeout = false, want true")
	}
}

func TestPollSpikeEvent_InvalidRegex(t *testing.T) {
	client := &APIClient{
		BaseURL:  "http://localhost:9999",
		Username: "admin",
		Password: "test",
	}

	poller := &Poller{
		APIClient: client,
		Clock:     NewMockClock(),
	}

	cfg := SpikeEventConfig{Interval: 1 * time.Second, Timeout: 30 * time.Second, ReasonRegex: `[invalid`}
	ctx := context.Background()

	result := poller.PollSpikeEvent(ctx, "lab-tovarisch", "", `[invalid`, cfg)

	if result.Error == nil {
		t.Errorf("PollSpikeEvent() Error = nil, want error for invalid regex")
	}
}
