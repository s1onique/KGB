// Package polling provides typed polling logic for UVB-76 capture netns lab.
package polling

import (
	"context"
	"testing"
)

// --- API Client Tests ---

func TestAPIClient_FetchLatencySeries_MalformedJSON(t *testing.T) {
	mockClient := &MockHTTPClient{
		Responses:   [][]byte{[]byte(`{invalid`)},
		StatusCodes: []int{200},
	}

	client := &APIClient{
		BaseURL:    "http://localhost:9999",
		Username:   "admin",
		Password:   "test",
		HTTPClient: mockClient,
	}

	ctx := context.Background()
	_, err := client.FetchLatencySeries(ctx, "lab-tovarisch", "http", 120)

	if err == nil {
		t.Errorf("FetchLatencySeries() error = nil, want error for malformed JSON")
	}
}

func TestAPIClient_FetchSpikes_EmptyResponse(t *testing.T) {
	mockClient := &MockHTTPClient{
		Responses:   [][]byte{[]byte(`{"count":0,"spikes":[]}`)},
		StatusCodes: []int{200},
	}

	client := &APIClient{
		BaseURL:    "http://localhost:9999",
		Username:   "admin",
		Password:   "test",
		HTTPClient: mockClient,
	}

	ctx := context.Background()
	result, err := client.FetchSpikes(ctx, "lab-tovarisch", true, 20)

	if err != nil {
		t.Errorf("FetchSpikes() error = %v, want nil", err)
	}
	if result.Count != 0 {
		t.Errorf("FetchSpikes() Count = %d, want 0", result.Count)
	}
}

func TestAPIClient_FetchSpikes_APIError(t *testing.T) {
	mockClient := &MockHTTPClient{
		Responses:   [][]byte{[]byte(`{"error":"internal error"}`)},
		StatusCodes: []int{500},
	}

	client := &APIClient{
		BaseURL:    "http://localhost:9999",
		Username:   "admin",
		Password:   "test",
		HTTPClient: mockClient,
	}

	ctx := context.Background()
	_, err := client.FetchSpikes(ctx, "lab-tovarisch", true, 20)

	if err == nil {
		t.Errorf("FetchSpikes() error = nil, want error for HTTP 500")
	}
}
