package state

import (
	"strings"
	"testing"
	"time"
)

// TestSpikeDetector_HTTP503ProbeFailure tests that HTTP 503 responses with explicit
// http_probe_503 reason are correctly recognized.
func TestSpikeDetector_HTTP503ProbeFailure(t *testing.T) {
	config := SpikeConfig{
		HTTPWarningMs:        1000,
		HTTPCriticalMs:       5000,
		RelativeMultiplier:   10.0,
		MinSamplesForMedian:  5,
		MaxPreviousSamples:   30,
		MaxEventsPerTracker:  100,
	}
	detector := NewSpikeDetectorWithConfig(config)

	now := time.Now().UTC()
	var prevSamples []LatencySample
	for i := 0; i < 25; i++ {
		prevSamples = append(prevSamples, LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		})
	}

	// Error string includes the explicit http_probe_503 reason (as produced by probe.go)
	errStr := "http_probe_503: HTTP 503"
	httpStatus := 503

	event := detector.DetectAndRecord(
		"test-target", "http",
		100.0, // normal latency - 503 response was fast
		now,
		false, // reachable = false
		nil, &httpStatus, &errStr,
		prevSamples,
	)

	if event == nil {
		t.Fatal("Expected spike event for HTTP 503 probe failure")
	}

	// Should have http_probe_503 reason
	found503Reason := false
	for _, r := range event.Reasons {
		if r == "http_probe_503" {
			found503Reason = true
		}
	}
	if !found503Reason {
		t.Errorf("Expected 'http_probe_503' reason, got %v", event.Reasons)
	}

	// Should be critical severity
	if event.Severity != "critical" {
		t.Errorf("Expected severity 'critical', got '%s'", event.Severity)
	}

	// HTTP status should be recorded
	if event.HTTPStatus == nil || *event.HTTPStatus != 503 {
		t.Errorf("Expected HTTPStatus=503, got %v", event.HTTPStatus)
	}
}

// TestSpikeDetector_HTTP5xxProbeFailure tests that various 5xx responses with explicit
// http_probe_* reasons are correctly recognized.
func TestSpikeDetector_HTTP5xxProbeFailure(t *testing.T) {
	testCases := []struct {
		statusCode    int
		errStr        string
		expectedReason string
	}{
		{502, "http_probe_502: HTTP 502", "http_probe_502"},
		{503, "http_probe_503: HTTP 503", "http_probe_503"},
		{504, "http_probe_504: HTTP 504", "http_probe_504"},
		{500, "http_probe_5xx: HTTP 500", "http_probe_5xx"},
	}

	for _, tc := range testCases {
		t.Run(tc.errStr, func(t *testing.T) {
			config := SpikeConfig{
				HTTPWarningMs:        1000,
				HTTPCriticalMs:       5000,
				RelativeMultiplier:   10.0,
				MinSamplesForMedian:  5,
				MaxPreviousSamples:   30,
				MaxEventsPerTracker:  100,
			}
			detector := NewSpikeDetectorWithConfig(config)

			now := time.Now().UTC()
			var prevSamples []LatencySample
			for i := 0; i < 25; i++ {
				prevSamples = append(prevSamples, LatencySample{
					Timestamp: now.Add(-time.Duration(25-i) * time.Second),
					LatencyMs: 50.0,
					Reachable: true,
				})
			}

			httpStatus := tc.statusCode
			event := detector.DetectAndRecord(
				"test-target", "http",
				100.0,
				now,
				false, // reachable = false
				nil, &httpStatus, &tc.errStr,
				prevSamples,
			)

			if event == nil {
				t.Fatalf("Expected spike event for HTTP %d", tc.statusCode)
			}

			foundReason := false
			for _, r := range event.Reasons {
				if r == tc.expectedReason {
					foundReason = true
				}
			}
			if !foundReason {
				t.Errorf("Expected '%s' reason, got %v", tc.expectedReason, event.Reasons)
			}
		})
	}
}

// TestSpikeDetector_HTTP4xxProbeFailure tests that 4xx responses are also treated as failures.
func TestSpikeDetector_HTTP4xxProbeFailure(t *testing.T) {
	config := SpikeConfig{
		HTTPWarningMs:        1000,
		HTTPCriticalMs:       5000,
		RelativeMultiplier:   10.0,
		MinSamplesForMedian:  5,
		MaxPreviousSamples:   30,
		MaxEventsPerTracker:  100,
	}
	detector := NewSpikeDetectorWithConfig(config)

	now := time.Now().UTC()
	var prevSamples []LatencySample
	for i := 0; i < 25; i++ {
		prevSamples = append(prevSamples, LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		})
	}

	// Error string includes the explicit http_probe_404 reason
	errStr := "http_probe_404: HTTP 404"
	httpStatus := 404

	event := detector.DetectAndRecord(
		"test-target", "http",
		50.0, // normal fast response
		now,
		false, // reachable = false
		nil, &httpStatus, &errStr,
		prevSamples,
	)

	if event == nil {
		t.Fatal("Expected spike event for HTTP 404 probe failure")
	}

	// Should have http_probe_404 reason
	found404Reason := false
	for _, r := range event.Reasons {
		if r == "http_probe_404" {
			found404Reason = true
		}
	}
	if !found404Reason {
		t.Errorf("Expected 'http_probe_404' reason, got %v", event.Reasons)
	}
}

// TestSpikeDetector_TransportFailureVsHTTP5xx tests that transport failures
// and HTTP 5xx failures produce distinct spike reasons.
func TestSpikeDetector_TransportFailureVsHTTP5xx(t *testing.T) {
	config := SpikeConfig{
		HTTPWarningMs:        1000,
		HTTPCriticalMs:       5000,
		RelativeMultiplier:   10.0,
		MinSamplesForMedian:  5,
		MaxPreviousSamples:   30,
		MaxEventsPerTracker:  100,
	}
	detector := NewSpikeDetectorWithConfig(config)

	now := time.Now().UTC()
	var prevSamples []LatencySample
	for i := 0; i < 25; i++ {
		prevSamples = append(prevSamples, LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		})
	}

	// Test HTTP 503 failure
	httpStatus := 503
	http503Err := "http_probe_503: HTTP 503"
	httpEvent := detector.DetectAndRecord(
		"test-target", "http",
		50.0,
		now,
		false,
		nil, &httpStatus, &http503Err,
		prevSamples,
	)

	if httpEvent == nil {
		t.Fatal("Expected spike event for HTTP 503")
	}

	// HTTP 503 should NOT have connection_refused reason
	for _, r := range httpEvent.Reasons {
		if strings.Contains(r, "connection_refused") {
			t.Errorf("HTTP 503 should not have connection_refused reason, got %v", httpEvent.Reasons)
		}
	}

	// Test transport failure
	transportErr := "request failed: connection refused"
	var transportHttpStatus *int
	transportEvent := detector.DetectAndRecord(
		"test-target", "http",
		5.0, // very fast failure
		now,
		false,
		nil, transportHttpStatus, &transportErr,
		prevSamples,
	)

	if transportEvent == nil {
		t.Fatal("Expected spike event for transport failure")
	}

	// Transport failure should have connection_refused reason
	foundConnRefused := false
	for _, r := range transportEvent.Reasons {
		if r == "http_probe_connection_refused" {
			foundConnRefused = true
		}
	}
	if !foundConnRefused {
		t.Errorf("Expected 'http_probe_connection_refused' for transport failure, got %v", transportEvent.Reasons)
	}

	// Transport failure should NOT have http_probe_5xx reason
	for _, r := range transportEvent.Reasons {
		if strings.Contains(r, "http_probe_5") || strings.Contains(r, "http_probe_503") {
			t.Errorf("Transport failure should not have 5xx reason, got %v", transportEvent.Reasons)
		}
	}
}
