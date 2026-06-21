// Package polling provides typed polling logic for UVB-76 capture netns lab.
package polling

import (
	"context"
	"testing"
	"time"
)

// --- Probe Polling Tests ---

func TestPollProbeSamples_ImmediateSuccess(t *testing.T) {
	mockClient := &MockHTTPClient{
		Responses:   [][]byte{latencySeriesJSON(5, 10, 0)},
		StatusCodes: []int{200},
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

	cfg := PollConfig{Interval: 1 * time.Second, Timeout: 30 * time.Second, RequireCount: 2}
	ctx := context.Background()

	result := poller.PollProbeSamples(ctx, "lab-tovarisch", "http", 120, cfg)

	if !result.OK {
		t.Errorf("PollProbeSamples() OK = false, want true")
	}
	if result.SampleCount != 5 {
		t.Errorf("PollProbeSamples() SampleCount = %d, want 5", result.SampleCount)
	}
	if mockClient.CallCount != 1 {
		t.Errorf("PollProbeSamples() called API %d times, want 1", mockClient.CallCount)
	}
}

func TestPollProbeSamples_PendingThenSuccess(t *testing.T) {
	mockClient := &MockHTTPClient{
		Responses:   [][]byte{latencySeriesJSON(1, 1, 0), latencySeriesJSON(1, 1, 0), latencySeriesJSON(3, 5, 0)},
		StatusCodes: []int{200, 200, 200},
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

	cfg := PollConfig{Interval: 1 * time.Second, Timeout: 30 * time.Second, RequireCount: 2}
	ctx := context.Background()

	result := poller.PollProbeSamples(ctx, "lab-tovarisch", "http", 120, cfg)

	if !result.OK {
		t.Errorf("PollProbeSamples() OK = false, want true")
	}
	if result.SampleCount != 3 {
		t.Errorf("PollProbeSamples() SampleCount = %d, want 3", result.SampleCount)
	}
	if mockClient.CallCount != 3 {
		t.Errorf("PollProbeSamples() called API %d times, want 3", mockClient.CallCount)
	}
}

func TestPollProbeSamples_Timeout(t *testing.T) {
	mockClient := &MockHTTPClient{
		Responses:   [][]byte{latencySeriesJSON(1, 1, 0)},
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

	cfg := PollConfig{Interval: 1 * time.Second, Timeout: 3 * time.Second, RequireCount: 2}
	ctx := context.Background()

	result := poller.PollProbeSamples(ctx, "lab-tovarisch", "http", 120, cfg)

	if result.OK {
		t.Errorf("PollProbeSamples() OK = true, want false (timeout)")
	}
	if !result.Timeout {
		t.Errorf("PollProbeSamples() Timeout = false, want true")
	}
}

func TestPollProbeSamples_FallbackToDeprecatedField(t *testing.T) {
	mockClient := &MockHTTPClient{
		Responses:   [][]byte{latencySeriesJSON(0, 0, 5)},
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

	cfg := PollConfig{Interval: 1 * time.Second, Timeout: 30 * time.Second, RequireCount: 2}
	ctx := context.Background()

	result := poller.PollProbeSamples(ctx, "lab-tovarisch", "http", 120, cfg)

	if !result.OK {
		t.Errorf("PollProbeSamples() OK = false, want true (fallback to sample_count)")
	}
	if result.SampleCount != 5 {
		t.Errorf("PollProbeSamples() SampleCount = %d, want 5 (from deprecated field)", result.SampleCount)
	}
}

func TestPollProbeSamples_ContextCancelled(t *testing.T) {
	mockClient := &MockHTTPClient{
		Responses:   [][]byte{latencySeriesJSON(1, 1, 0)},
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

	cfg := PollConfig{Interval: 100 * time.Millisecond, Timeout: 30 * time.Second, RequireCount: 2}
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	result := poller.PollProbeSamples(ctx, "lab-tovarisch", "http", 120, cfg)

	if result.Error == nil {
		t.Errorf("PollProbeSamples() Error = nil, want context.Canceled")
	}
}
