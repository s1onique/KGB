// Package polling provides typed polling logic for UVB-76 capture netns lab.
package polling

import (
	"bytes"
	"io"
	"net/http"
	"time"
)

// --- Mock HTTP Client ---

// MockHTTPClient implements HTTPClient for testing.
type MockHTTPClient struct {
	Responses   [][]byte
	StatusCodes []int
	Err         error
	CallCount   int
}

// Do implements HTTPClient.
func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.CallCount >= len(m.Responses) {
		m.CallCount++
		return &http.Response{StatusCode: 500, Body: http.NoBody}, nil
	}
	body := m.Responses[m.CallCount]
	status := 200
	if m.CallCount < len(m.StatusCodes) {
		status = m.StatusCodes[m.CallCount]
	}
	m.CallCount++
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, nil
}

// --- Mock Clock ---

// MockClock implements Clock for deterministic testing.
type MockClock struct {
	NowValue   time.Time
	SleepCh    chan time.Duration
	SleepCalls []time.Duration
}

// NewMockClock creates a mock clock starting at t0.
func NewMockClock() *MockClock {
	return &MockClock{
		NowValue:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		SleepCh:    make(chan time.Duration, 100),
		SleepCalls: make([]time.Duration, 0),
	}
}

// Now returns the current mock time.
func (m *MockClock) Now() time.Time { return m.NowValue }

// Sleep records the sleep duration and advances time.
func (m *MockClock) Sleep(d time.Duration) {
	m.SleepCalls = append(m.SleepCalls, d)
	m.NowValue = m.NowValue.Add(d)
	select {
	case m.SleepCh <- d:
	default:
	}
}

// Advance advances the mock clock by duration.
func (m *MockClock) Advance(d time.Duration) {
	m.NowValue = m.NowValue.Add(d)
}

// --- Mock Artifact Writer ---

// MockArtifactWriter implements ArtifactWriter for testing.
type MockArtifactWriter struct {
	ProbeReadyCalled   bool
	SpikeEventCalled   bool
	CaptureCalled      bool
	ProbeReadySeries   *LatencySeries
	SpikeEventResponse *SpikesResponse
	CaptureResponse    *SpikesResponse
}

// WriteProbeReadyArtifact implements ArtifactWriter.
func (m *MockArtifactWriter) WriteProbeReadyArtifact(series *LatencySeries) error {
	m.ProbeReadyCalled = true
	m.ProbeReadySeries = series
	return nil
}

// WriteSpikeEventArtifact implements ArtifactWriter.
func (m *MockArtifactWriter) WriteSpikeEventArtifact(response *SpikesResponse) error {
	m.SpikeEventCalled = true
	m.SpikeEventResponse = response
	return nil
}

// WriteCaptureArtifact implements ArtifactWriter.
func (m *MockArtifactWriter) WriteCaptureArtifact(response *SpikesResponse) error {
	m.CaptureCalled = true
	m.CaptureResponse = response
	return nil
}
