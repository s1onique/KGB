package state

import (
	"strings"
	"testing"
	"time"
)

// TestDetectAndRecordWithTcpQuality_NativeTCPQualityCarriedToSpike proves that
// native TCP quality collected from the actual HTTP probe socket survives into
// the SpikeEvent as NativeTcpQuality with source=native_tcp_info and matched_socket=true.
//
// This is a platform-neutral regression test that exercises the state-level carrier
// without depending on any OS-specific TCP_INFO collection.
func TestDetectAndRecordWithTcpQuality_NativeTCPQualityCarriedToSpike(t *testing.T) {
	config := SpikeConfig{
		HTTPWarningMs:        100,
		HTTPCriticalMs:       500,
		RelativeMultiplier:    10.0,
		MinSamplesForMedian:  5,
		MaxPreviousSamples:   30,
		MaxEventsPerTracker:  100,
	}
	detector := NewSpikeDetectorWithConfig(config)

	now := time.Now().UTC()

	// Pre-populate with normal samples to establish baseline
	var prevSamples []LatencySample
	for i := 0; i < 25; i++ {
		prevSamples = append(prevSamples, LatencySample{
			Timestamp: now.Add(-time.Duration(25-i) * time.Second),
			LatencyMs: 50.0,
			Reachable: true,
		})
	}

	// Create native TCP quality evidence from actual probe socket
	rttUs := int64(113000)
	rttVarUs := int64(12000)
	sndCwnd := int32(10)
	retransTotal := int64(3)

	nativeTcpQuality := &TcpQuality{
		Kind:               "http",
		LookupTarget:       "10.0.0.5",
		MatchedSocket:      true, // From actual probe socket, not synthetic
		Source:             "native_tcp_info",
		State:              "ESTAB",
		Local:              "redacted:45678",
		Remote:             "redacted:8080",
		RTTUs:              &rttUs,
		RTTVarUs:           &rttVarUs,
		SndCwnd:            &sndCwnd,
		RetransmitsTotal:   &retransTotal,
		CollectedAt:        time.Now().UTC().Format(time.RFC3339),
	}

	// Trigger a warning-level latency spike with native TCP quality
	event := detector.DetectAndRecordWithTcpQuality(
		"test-target", "http",
		150.0, // Above warning threshold (100ms)
		now,
		true, // reachable
		nil,  // scheduler delay
		intPtr(200), // http status
		nil,  // no error
		prevSamples,
		nil,  // httpTrace
		nativeTcpQuality,
	)

	// Verify spike event was created
	if event == nil {
		t.Fatal("Expected spike event for latency above warning threshold")
	}

	// Verify NativeTcpQuality is present
	if event.NativeTcpQuality == nil {
		t.Fatal("Expected NativeTcpQuality to be populated on spike event")
	}

	// Verify source is native_tcp_info
	if event.NativeTcpQuality.Source != "native_tcp_info" {
		t.Errorf("Expected source 'native_tcp_info', got '%s'", event.NativeTcpQuality.Source)
	}

	// Verify matched_socket is true (actual probe socket)
	if !event.NativeTcpQuality.MatchedSocket {
		t.Error("Expected matched_socket=true for native TCP_INFO from actual probe socket")
	}

	// Verify kind is preserved
	if event.NativeTcpQuality.Kind != "http" {
		t.Errorf("Expected kind 'http', got '%s'", event.NativeTcpQuality.Kind)
	}

	// Verify lookup target is preserved
	if event.NativeTcpQuality.LookupTarget != "10.0.0.5" {
		t.Errorf("Expected lookup_target '10.0.0.5', got '%s'", event.NativeTcpQuality.LookupTarget)
	}

	// Verify RTT data is preserved
	if event.NativeTcpQuality.RTTUs == nil || *event.NativeTcpQuality.RTTUs != 113000 {
		t.Errorf("Expected RTT 113000, got %v", event.NativeTcpQuality.RTTUs)
	}

	// Verify state is preserved
	if event.NativeTcpQuality.State != "ESTAB" {
		t.Errorf("Expected state 'ESTAB', got '%s'", event.NativeTcpQuality.State)
	}

	// Verify reason includes latency threshold
	foundLatencyReason := false
	for _, reason := range event.Reasons {
		if strings.Contains(reason, "warning") || strings.Contains(reason, "threshold") {
			foundLatencyReason = true
		}
	}
	if !foundLatencyReason {
		t.Errorf("Expected latency-related reason, got %v", event.Reasons)
	}
}

// TestDetectAndRecordWithTcpQuality_CriticalSpikeWithNativeTCPQuality tests that
// critical-level spikes also carry native TCP quality.
func TestDetectAndRecordWithTcpQuality_CriticalSpikeWithNativeTCPQuality(t *testing.T) {
	config := SpikeConfig{
		HTTPWarningMs:        100,
		HTTPCriticalMs:       500,
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

	// Native TCP quality with retransmits (evidence of issues)
	retransCurrent := int64(5)
	retransTotal := int64(15)
	lost := int64(2)

	nativeTcpQuality := &TcpQuality{
		Kind:               "http",
		LookupTarget:       "192.168.1.100",
		MatchedSocket:      true,
		Source:             "native_tcp_info",
		State:              "ESTAB",
		RetransmitsCurrent: &retransCurrent,
		RetransmitsTotal:   &retransTotal,
		Lost:               &lost,
		CollectedAt:        time.Now().UTC().Format(time.RFC3339),
	}

	// Trigger critical-level spike
	event := detector.DetectAndRecordWithTcpQuality(
		"critical-target", "http",
		600.0, // Above critical threshold (500ms)
		now,
		true, // reachable
		nil,
		intPtr(200),
		nil,
		prevSamples,
		nil, // httpTrace
		nativeTcpQuality,
	)

	if event == nil {
		t.Fatal("Expected spike event for critical latency")
	}

	// Verify severity
	if event.Severity != "critical" {
		t.Errorf("Expected severity 'critical', got '%s'", event.Severity)
	}

	// Verify native TCP quality carried through
	if event.NativeTcpQuality == nil {
		t.Fatal("Expected NativeTcpQuality on critical spike")
	}
	if event.NativeTcpQuality.Source != "native_tcp_info" {
		t.Errorf("Expected source 'native_tcp_info', got '%s'", event.NativeTcpQuality.Source)
	}
	if !event.NativeTcpQuality.MatchedSocket {
		t.Error("Expected matched_socket=true")
	}

	// Verify retransmit data is preserved
	if event.NativeTcpQuality.RetransmitsCurrent == nil || *event.NativeTcpQuality.RetransmitsCurrent != 5 {
		t.Errorf("Expected retransmits_current 5, got %v", event.NativeTcpQuality.RetransmitsCurrent)
	}
}

// TestDetectAndRecord_LegacyNilTCPQuality proves that the legacy DetectAndRecord
// method (without native TCP quality) still works and produces events without
// NativeTcpQuality.
func TestDetectAndRecord_LegacyNilTCPQuality(t *testing.T) {
	config := SpikeConfig{
		HTTPWarningMs:        100,
		HTTPCriticalMs:       500,
		RelativeMultiplier:    10.0,
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

	// Use legacy DetectAndRecord (passes nil for nativeTcpQuality internally)
	event := detector.DetectAndRecord(
		"test-target", "http",
		200.0, // Above warning
		now,
		true,
		nil,
		intPtr(200),
		nil,
		prevSamples,
		nil, // httpTrace
	)

	if event == nil {
		t.Fatal("Expected spike event from legacy DetectAndRecord")
	}

	// NativeTcpQuality should be nil (legacy behavior)
	if event.NativeTcpQuality != nil {
		t.Error("Expected NativeTcpQuality to be nil for legacy DetectAndRecord")
	}

	// But other spike fields should still be populated
	if event.Kind != "http" {
		t.Errorf("Expected kind 'http', got '%s'", event.Kind)
	}
	if event.Severity != "warning" {
		t.Errorf("Expected severity 'warning', got '%s'", event.Severity)
	}
}

// TestDetectAndRecordWithTcpQuality_NilTCPQuality proves that passing nil
// NativeTcpQuality to DetectAndRecordWithTcpQuality produces events without
// NativeTcpQuality (same as legacy behavior).
func TestDetectAndRecordWithTcpQuality_NilTCPQuality(t *testing.T) {
	config := SpikeConfig{
		HTTPWarningMs:        100,
		HTTPCriticalMs:       500,
		RelativeMultiplier:    10.0,
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

	// Explicitly pass nil NativeTcpQuality
	event := detector.DetectAndRecordWithTcpQuality(
		"test-target", "http",
		200.0,
		now,
		true,
		nil,
		intPtr(200),
		nil,
		prevSamples,
		nil, // httpTrace
		nil, // nativeTcpQuality = nil
	)

	if event == nil {
		t.Fatal("Expected spike event")
	}

	// NativeTcpQuality should be nil
	if event.NativeTcpQuality != nil {
		t.Error("Expected NativeTcpQuality to be nil when passed nil")
	}
}

// TestDetectAndRecordWithTcpQuality_HTTPFailureWithNativeTCPQuality tests that
// HTTP probe failures also carry native TCP quality when available.
func TestDetectAndRecordWithTcpQuality_HTTPFailureWithNativeTCPQuality(t *testing.T) {
	config := SpikeConfig{
		HTTPWarningMs:        100,
		HTTPCriticalMs:       500,
		RelativeMultiplier:    10.0,
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

	// Native TCP quality even on failure (socket was established)
	rttUs := int64(50000)
	nativeTcpQuality := &TcpQuality{
		Kind:          "http",
		LookupTarget:  "10.0.0.5",
		MatchedSocket: true,
		Source:        "native_tcp_info",
		State:         "ESTAB",
		RTTUs:         &rttUs,
		CollectedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	errStr := "request failed: http_probe_503"
	event := detector.DetectAndRecordWithTcpQuality(
		"test-target", "http",
		3000.0,
		now,
		false, // reachable=false (failure)
		nil,
		intPtr(503),
		&errStr,
		prevSamples,
		nil, // httpTrace
		nativeTcpQuality,
	)

	if event == nil {
		t.Fatal("Expected spike event for HTTP failure")
	}

	// Verify severity is critical for failures
	if event.Severity != "critical" {
		t.Errorf("Expected severity 'critical' for HTTP failure, got '%s'", event.Severity)
	}

	// Verify native TCP quality is present
	if event.NativeTcpQuality == nil {
		t.Fatal("Expected NativeTcpQuality even on HTTP failure")
	}
	if event.NativeTcpQuality.Source != "native_tcp_info" {
		t.Errorf("Expected source 'native_tcp_info', got '%s'", event.NativeTcpQuality.Source)
	}
	if !event.NativeTcpQuality.MatchedSocket {
		t.Error("Expected matched_socket=true")
	}

	// Verify failure reason
	foundFailureReason := false
	for _, reason := range event.Reasons {
		if strings.Contains(reason, "503") || strings.Contains(reason, "failure") {
			foundFailureReason = true
		}
	}
	if !foundFailureReason {
		t.Errorf("Expected failure-related reason, got %v", event.Reasons)
	}
}

// Helper function

func intPtr(v int) *int {
	return &v
}
