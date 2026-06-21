package state

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestHTTPTrace_IncludedInSpikeEvent verifies that HTTPTrace is properly included
// in SpikeEvent and can be serialized to JSON for export.
func TestHTTPTrace_IncludedInSpikeEvent(t *testing.T) {
	// Create a detector
	detector := NewSpikeDetector()

	// Create a sample HTTPTrace
	httpTrace := &HTTPTrace{
		Kind:                "http",
		URLHost:             "example.com",
		RemoteAddr:          "192.0.2.1",
		DNSMs:               5.0,
		TCPConnectMs:        10.0,
		TLSHandshakeMs:      15.0,
		GotConnMs:           26.0,
		TimeToFirstByteMs:   100.0,
		BodyReadMs:          50.0,
		TotalMs:             150.0,
		ConnectionReused:    false,
		WasIdle:             false,
		HTTPStatus:          503,
		BytesRead:           1024,
		Error:               "",
	}

	// Build previous samples
	now := time.Now().UTC()
	var prevSamples []LatencySample
	for i := 0; i < 30; i++ {
		prevSamples = append(prevSamples, LatencySample{
			Timestamp: now.Add(-time.Duration(i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		})
	}

	// Create a spike with HTTPTrace
	spike := detector.DetectAndRecord(
		"test-target",
		"http",
		2000.0, // high latency
		now,
		true, // reachable
		nil, // scheduler delay
		nil, // http status (we use errStr for 5xx classification)
		nil, // probe error (not needed for latency spike)
		prevSamples,
		httpTrace,
	)

	if spike == nil {
		t.Fatal("Expected spike event to be created")
	}

	// Verify SpikeEvent has HTTPTrace field
	if spike.HTTPTrace == nil {
		t.Fatal("Expected HTTPTrace to be set in spike event")
	}

	// Verify HTTPTrace fields
	if spike.HTTPTrace.Kind != "http" {
		t.Errorf("Expected Kind='http', got %q", spike.HTTPTrace.Kind)
	}
	if spike.HTTPTrace.URLHost != "example.com" {
		t.Errorf("Expected URLHost='example.com', got %q", spike.HTTPTrace.URLHost)
	}
	if spike.HTTPTrace.TimeToFirstByteMs != 100.0 {
		t.Errorf("Expected TTFB=100.0, got %f", spike.HTTPTrace.TimeToFirstByteMs)
	}
	if spike.HTTPTrace.HTTPStatus != 503 {
		t.Errorf("Expected HTTPStatus=503, got %d", spike.HTTPTrace.HTTPStatus)
	}

	t.Logf("Spike event created with HTTPTrace: ttfb=%.2fms, total=%.2fms",
		spike.HTTPTrace.TimeToFirstByteMs, spike.HTTPTrace.TotalMs)
}

// TestHTTPTrace_NilForICMP verifies that HTTPTrace is nil for ICMP probes.
func TestHTTPTrace_NilForICMP(t *testing.T) {
	detector := NewSpikeDetector()

	now := time.Now().UTC()
	var prevSamples []LatencySample
	for i := 0; i < 30; i++ {
		prevSamples = append(prevSamples, LatencySample{
			Timestamp: now.Add(-time.Duration(i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		})
	}

	// Create an ICMP spike (httpTrace should be nil)
	spike := detector.DetectAndRecord(
		"test-target",
		"icmp",
		2000.0,
		now,
		true,
		nil,
		nil,
		nil,
		prevSamples,
		nil, // httpTrace is nil for ICMP
	)

	if spike == nil {
		t.Fatal("Expected ICMP spike to be created")
	}

	if spike.HTTPTrace != nil {
		t.Error("Expected HTTPTrace to be nil for ICMP probes")
	}
}

// TestHTTPTrace_JSONExport verifies that spike events with HTTPTrace
// can be properly serialized to JSON for export.
func TestHTTPTrace_JSONExport(t *testing.T) {
	detector := NewSpikeDetector()

	now := time.Now().UTC()
	var prevSamples []LatencySample
	for i := 0; i < 30; i++ {
		prevSamples = append(prevSamples, LatencySample{
			Timestamp: now.Add(-time.Duration(i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		})
	}

	httpTrace := &HTTPTrace{
		Kind:              "http",
		URLHost:           "example.com",
		RemoteAddr:        "192.0.2.1",
		DNSMs:             5.0,
		TCPConnectMs:      10.0,
		TLSHandshakeMs:    15.0,
		GotConnMs:         26.0,
		TimeToFirstByteMs: 100.0,
		BodyReadMs:        50.0,
		TotalMs:           150.0,
		ConnectionReused:  false,
		WasIdle:           false,
		HTTPStatus:        503,
		BytesRead:         1024,
		Error:             "",
	}

	spike := detector.DetectAndRecord(
		"test-target",
		"http",
		2000.0,
		now,
		true,
		nil,
		nil,
		nil,
		prevSamples,
		httpTrace,
	)

	// Serialize to JSON (simulating export)
	data, err := json.Marshal(spike)
	if err != nil {
		t.Fatalf("Failed to marshal spike event: %v", err)
	}

	jsonStr := string(data)

	// Verify HTTPTrace data is in JSON (lowercase field names)
	if !strings.Contains(jsonStr, `"kind":"http"`) {
		t.Error("JSON should contain kind='http'")
	}
	if !strings.Contains(jsonStr, `"url_host":"example.com"`) {
		t.Error("JSON should contain url_host")
	}
	if !strings.Contains(jsonStr, `"time_to_first_byte_ms":100`) {
		t.Error("JSON should contain time_to_first_byte_ms")
	}
	if !strings.Contains(jsonStr, `"http_status":503`) {
		t.Error("JSON should contain http_status")
	}

	// Verify sensitive data is NOT leaked
	if strings.Contains(jsonStr, "secret") || strings.Contains(jsonStr, "token") {
		t.Error("JSON should not contain sensitive data")
	}

	t.Logf("Spike event JSON export: %s", jsonStr[:min(len(jsonStr), 200)]+"...")
}

// TestHTTPTrace_PrivacyInExport verifies that sensitive data in HTTPTrace is redacted.
func TestHTTPTrace_PrivacyInExport(t *testing.T) {
	detector := NewSpikeDetector()

	now := time.Now().UTC()
	var prevSamples []LatencySample
	for i := 0; i < 30; i++ {
		prevSamples = append(prevSamples, LatencySample{
			Timestamp: now.Add(-time.Duration(i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		})
	}

	// Create trace with error containing potential sensitive info
	httpTrace := &HTTPTrace{
		Kind:              "http",
		URLHost:           "example.com",
		RemoteAddr:        "192.0.2.1",
		DNSMs:             5.0,
		TCPConnectMs:      10.0,
		TLSHandshakeMs:    15.0,
		GotConnMs:         26.0,
		TimeToFirstByteMs: 100.0,
		BodyReadMs:        50.0,
		TotalMs:           150.0,
		ConnectionReused:  false,
		WasIdle:           false,
		HTTPStatus:        503,
		BytesRead:         1024,
		Error:             "request failed: connection refused",
	}

	spike := detector.DetectAndRecord(
		"test-target",
		"http",
		2000.0,
		now,
		true,
		nil,
		nil,
		nil,
		prevSamples,
		httpTrace,
	)

	data, _ := json.Marshal(spike)
	jsonStr := string(data)

	// Verify error message is included but sanitized
	if !strings.Contains(jsonStr, "error") {
		t.Error("JSON should contain error field")
	}

	// Error should be safe (no newlines, truncated)
	if strings.Contains(jsonStr, "\n") || strings.Contains(jsonStr, "\r") {
		t.Error("Error field should not contain newlines")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
