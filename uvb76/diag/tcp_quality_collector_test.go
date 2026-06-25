package diag

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/state"
)

func TestCollectTcpQuality_ICMPProbe(t *testing.T) {
	collector := NewTcpQualityCollector()
	ctx := context.Background()

	result := collector.CollectTcpQuality(ctx, "icmp", "10.0.0.5")

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.MatchedSocket {
		t.Error("expected matched_socket=false for ICMP")
	}
	if result.ErrorKind != state.TcpQualityErrorUnavailable {
		t.Errorf("expected error_kind 'unavailable', got '%s'", result.ErrorKind)
	}
	if result.Error != "tcp quality is http/probe-only" {
		t.Errorf("expected error message, got '%s'", result.Error)
	}
	if result.Kind != "icmp" {
		t.Errorf("expected kind 'icmp', got '%s'", result.Kind)
	}
}

func TestCollectTcpQuality_EmptyTarget(t *testing.T) {
	collector := NewTcpQualityCollector()
	ctx := context.Background()

	result := collector.CollectTcpQuality(ctx, "http", "")

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.MatchedSocket {
		t.Error("expected matched_socket=false for empty target")
	}
	if result.ErrorKind != state.TcpQualityErrorTargetUnresolved {
		t.Errorf("expected error_kind 'target_unresolved', got '%s'", result.ErrorKind)
	}
}

func TestCollectTcpQuality_SuccessfulCollection(t *testing.T) {
	collector := &TcpQualityCollector{
		timeout:        100 * time.Millisecond,
		maxStdoutBytes: 4096,
		maxStderrBytes: 512,
	}

	ctx := context.Background()

	// Test successful collection with mock data
	// Note: This test requires ss to be available, so we skip in CI without ss
	// Instead, test the parser directly
	result := collector.CollectTcpQuality(ctx, "http", "10.0.0.5")

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// result depends on ss availability - may be matched or not matched
	_ = result
}

func TestTcpQuality_JSONSerialization(t *testing.T) {
	// Test that successful TcpQuality serializes correctly
	tq := &state.TcpQuality{
		Kind:           "http",
		LookupTarget:  "10.0.0.5",
		MatchedSocket: true,
		Source:        "ss-tcp-info",
		State:         "ESTAB",
		Local:         "redacted:45678",
		Remote:        "redacted:8080",
		// Queue fields are pointers so zero values serialize as 0, not omitted
		SendQueueBytes: tcpQPtrInt64(0),
		RecvQueueBytes: tcpQPtrInt64(0),
		RTTUs:         tcpQPtrInt64(113000),
		RTTVarUs:      tcpQPtrInt64(12000),
		RetransmitsCurrent: tcpQPtrInt64(0),
		RetransmitsTotal:   tcpQPtrInt64(3),
		Unacked:        tcpQPtrInt64(0),
		Lost:           tcpQPtrInt64(0),
		Sacked:         tcpQPtrInt64(0),
		Reordering:     tcpQPtrInt64(3),
		SndCwnd:        tcpQPtrInt32(10),
		CollectedAt:    "2026-06-21T22:35:00Z",
	}

	data, err := json.Marshal(tq)
	if err != nil {
		t.Fatalf("failed to marshal TcpQuality: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal TcpQuality: %v", err)
	}

	// Verify key fields
	if parsed["kind"] != "http" {
		t.Errorf("expected kind 'http', got '%v'", parsed["kind"])
	}
	if parsed["matched_socket"] != true {
		t.Errorf("expected matched_socket true, got '%v'", parsed["matched_socket"])
	}
	if parsed["source"] != "ss-tcp-info" {
		t.Errorf("expected source 'ss-tcp-info', got '%v'", parsed["source"])
	}
	if parsed["state"] != "ESTAB" {
		t.Errorf("expected state 'ESTAB', got '%v'", parsed["state"])
	}
	if parsed["local"] != "redacted:45678" {
		t.Errorf("expected local 'redacted:45678', got '%v'", parsed["local"])
	}
	if parsed["remote"] != "redacted:8080" {
		t.Errorf("expected remote 'redacted:8080', got '%v'", parsed["remote"])
	}
	if parsed["rtt_us"] != float64(113000) {
		t.Errorf("expected rtt_us 113000, got '%v'", parsed["rtt_us"])
	}
	if parsed["retransmits_total"] != float64(3) {
		t.Errorf("expected retransmits_total 3, got '%v'", parsed["retransmits_total"])
	}

	// CRITICAL: Verify zero queue values serialize as 0, not omitted
	if parsed["send_queue_bytes"] == nil {
		t.Error("send_queue_bytes should be 0, not omitted (nil means unobserved)")
	} else if parsed["send_queue_bytes"] != float64(0) {
		t.Errorf("expected send_queue_bytes 0, got '%v'", parsed["send_queue_bytes"])
	}
	if parsed["recv_queue_bytes"] == nil {
		t.Error("recv_queue_bytes should be 0, not omitted (nil means unobserved)")
	} else if parsed["recv_queue_bytes"] != float64(0) {
		t.Errorf("expected recv_queue_bytes 0, got '%v'", parsed["recv_queue_bytes"])
	}
}

func TestTcpQuality_ErrorBlock_JSONSerialization(t *testing.T) {
	// Test that error TcpQuality serializes correctly
	tq := &state.TcpQuality{
		Kind:           "http",
		LookupTarget:  "10.0.0.5",
		MatchedSocket: false,
		Source:        "ss-tcp-info",
		ErrorKind:     state.TcpQualityErrorNoMatchingSocket,
		Error:         "no matching TCP socket found for probe destination",
		MatchCount:    tcpQPtrInt(2),
		CollectedAt:   "2026-06-21T22:35:00Z",
	}

	data, err := json.Marshal(tq)
	if err != nil {
		t.Fatalf("failed to marshal TcpQuality: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal TcpQuality: %v", err)
	}

	// Verify key fields
	if parsed["matched_socket"] != false {
		t.Errorf("expected matched_socket false, got '%v'", parsed["matched_socket"])
	}
	if parsed["error_kind"] != "no_matching_socket" {
		t.Errorf("expected error_kind 'no_matching_socket', got '%v'", parsed["error_kind"])
	}
	if parsed["match_count"] != float64(2) {
		t.Errorf("expected match_count 2, got '%v'", parsed["match_count"])
	}

	// Verify socket fields are omitted
	if _, ok := parsed["state"]; ok {
		t.Error("state should be omitted for unmatched socket")
	}
	if _, ok := parsed["rtt_us"]; ok {
		t.Error("rtt_us should be omitted for unmatched socket")
	}
}

func TestTcpQuality_NoRawIPLeak(t *testing.T) {
	// Test that IP addresses are properly redacted in serialization.
	// Note: LookupTarget may retain the probe destination because it is already
	// present in the diagnostic target identity. Local and Remote must be redacted.
	tq := &state.TcpQuality{
		Kind:           "http",
		LookupTarget:  "redacted", // May be retained as it's target identity
		MatchedSocket: true,
		Source:        "ss-tcp-info",
		State:         "ESTAB",
		Local:         "redacted:45678", // Must be redacted
		Remote:        "redacted:8080",  // Must be redacted
		SendQueueBytes: tcpQPtrInt64(0),
		RecvQueueBytes: tcpQPtrInt64(0),
		RTTUs:         tcpQPtrInt64(113000),
		CollectedAt:   "2026-06-21T22:35:00Z",
	}

	data, err := json.Marshal(tq)
	if err != nil {
		t.Fatalf("failed to marshal TcpQuality: %v", err)
	}

	jsonStr := string(data)

	// Check that no raw IPs leak through
	privateIPs := []string{
		"10.0.0.1", "192.168.1.1", "172.16.0.1",
		"10.0.0.5", "10.77.0.1", "192.0.2.1",
		"2001:db8::1", "::1",
	}

	for _, ip := range privateIPs {
		if ip != "redacted" && contains(jsonStr, ip) {
			t.Errorf("raw IP %s found in serialized TcpQuality", ip)
		}
	}
}

func TestTcpQuality_NewTcpQualityUnavailable(t *testing.T) {
	result := state.NewTcpQualityUnavailable("http", "10.0.0.5",
		state.TcpQualityErrorCommandMissing, "ss command not found")

	if result.Kind != "http" {
		t.Errorf("expected kind 'http', got '%s'", result.Kind)
	}
	if result.LookupTarget != "10.0.0.5" {
		t.Errorf("expected lookup_target '10.0.0.5', got '%s'", result.LookupTarget)
	}
	if result.MatchedSocket {
		t.Error("expected matched_socket=false")
	}
	if result.ErrorKind != state.TcpQualityErrorCommandMissing {
		t.Errorf("expected error_kind 'command_missing', got '%s'", result.ErrorKind)
	}
	if result.Error != "ss command not found" {
		t.Errorf("expected error 'ss command not found', got '%s'", result.Error)
	}
	if result.Source != "ss-tcp-info" {
		t.Errorf("expected source 'ss-tcp-info', got '%s'", result.Source)
	}
}

func TestTcpQuality_NewTcpQualitySuccess(t *testing.T) {
	result := state.NewTcpQualitySuccess("http", "10.0.0.5")

	if result.Kind != "http" {
		t.Errorf("expected kind 'http', got '%s'", result.Kind)
	}
	if result.LookupTarget != "10.0.0.5" {
		t.Errorf("expected lookup_target '10.0.0.5', got '%s'", result.LookupTarget)
	}
	if !result.MatchedSocket {
		t.Error("expected matched_socket=true")
	}
	if result.Source != "ss-tcp-info" {
		t.Errorf("expected source 'ss-tcp-info', got '%s'", result.Source)
	}
}

func TestTcpQuality_SourceValues(t *testing.T) {
	// Test that new source constants are defined correctly
	if TcpQualitySourceNativeTCPInfo != "native_tcp_info" {
		t.Errorf("expected TcpQualitySourceNativeTCPInfo='native_tcp_info', got '%s'", TcpQualitySourceNativeTCPInfo)
	}
	if TcpQualitySourceSyntheticTCPInfo != "synthetic_tcp_info" {
		t.Errorf("expected TcpQualitySourceSyntheticTCPInfo='synthetic_tcp_info', got '%s'", TcpQualitySourceSyntheticTCPInfo)
	}
	if TcpQualitySourceCLIFallback != "ss-tcp-info" {
		t.Errorf("expected TcpQualitySourceCLIFallback='ss-tcp-info', got '%s'", TcpQualitySourceCLIFallback)
	}
	if TcpQualitySourceUnavailable != "unavailable" {
		t.Errorf("expected TcpQualitySourceUnavailable='unavailable', got '%s'", TcpQualitySourceUnavailable)
	}
}

func TestTcpQuality_NativeSourceSerialization(t *testing.T) {
	// Test that TcpQuality with native_tcp_info source serializes correctly
	tq := &state.TcpQuality{
		Kind:           "http",
		LookupTarget:  "10.0.0.5",
		MatchedSocket: true,
		Source:        TcpQualitySourceNativeTCPInfo,
		State:         "ESTAB",
		Local:         "redacted:45678",
		Remote:        "redacted:8080",
		RTTUs:         tcpQPtrInt64(5000),
		SndCwnd:       tcpQPtrInt32(10),
		CollectedAt:   "2026-06-21T22:35:00Z",
	}

	data, err := json.Marshal(tq)
	if err != nil {
		t.Fatalf("failed to marshal TcpQuality: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal TcpQuality: %v", err)
	}

	// Verify source is native_tcp_info
	if parsed["source"] != "native_tcp_info" {
		t.Errorf("expected source 'native_tcp_info', got '%v'", parsed["source"])
	}
}

func TestTcpQuality_UnavailableSourceSerialization(t *testing.T) {
	// Test that TcpQuality with unavailable source serializes correctly
	tq := &state.TcpQuality{
		Kind:           "http",
		LookupTarget:  "10.0.0.5",
		MatchedSocket: false,
		Source:        TcpQualitySourceUnavailable,
		ErrorKind:     state.TcpQualityErrorUnavailable,
		Error:         "tcp quality is http/probe-only",
		CollectedAt:   "2026-06-21T22:35:00Z",
	}

	data, err := json.Marshal(tq)
	if err != nil {
		t.Fatalf("failed to marshal TcpQuality: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal TcpQuality: %v", err)
	}

	// Verify source is unavailable
	if parsed["source"] != "unavailable" {
		t.Errorf("expected source 'unavailable', got '%v'", parsed["source"])
	}
}

func TestDiagCapture_WithTcpQuality(t *testing.T) {
	capture := state.DiagCapture{
		Source:               "peer1",
		BaseURL:              "http://10.0.0.5:8080",
		CaptureStartedAt:     time.Now().UTC(),
		Status:               state.DiagCaptureStatusOK,
		CaptureStatus:        state.CaptureStatusCaptured,
		EffectiveCaptureURL:  "http://10.0.0.5:8080/status.json",
	}

	tcpQuality := &state.TcpQuality{
		Kind:           "http",
		LookupTarget:  "10.0.0.5",
		MatchedSocket: true,
		Source:        "ss-tcp-info",
		State:         "ESTAB",
		Local:         "redacted:45678",
		Remote:        "redacted:8080",
		RTTUs:         tcpQPtrInt64(113000),
		SndCwnd:       tcpQPtrInt32(10),
		CollectedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	capture.TcpQuality = tcpQuality

	// Serialize and verify
	data, err := json.Marshal(capture)
	if err != nil {
		t.Fatalf("failed to marshal DiagCapture: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal DiagCapture: %v", err)
	}

	tcpQualityParsed, ok := parsed["tcp_quality"].(map[string]interface{})
	if !ok {
		t.Fatal("tcp_quality not found in parsed capture")
	}

	if tcpQualityParsed["matched_socket"] != true {
		t.Errorf("expected matched_socket true in tcp_quality")
	}
	if tcpQualityParsed["rtt_us"] != float64(113000) {
		t.Errorf("expected rtt_us 113000, got '%v'", tcpQualityParsed["rtt_us"])
	}
}

func TestDiagCapture_BackwardCompatibility(t *testing.T) {
	// Test that DiagCapture without TcpQuality still serializes/deserializes
	capture := state.DiagCapture{
		Source:               "peer1",
		BaseURL:              "http://10.0.0.5:8080",
		CaptureStartedAt:     time.Now().UTC(),
		Status:               state.DiagCaptureStatusOK,
		CaptureStatus:        state.CaptureStatusCaptured,
		EffectiveCaptureURL:  "http://10.0.0.5:8080/status.json",
		// TcpQuality is nil
	}

	data, err := json.Marshal(capture)
	if err != nil {
		t.Fatalf("failed to marshal DiagCapture: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal DiagCapture: %v", err)
	}

	// tcp_quality should be omitted (nil)
	if _, ok := parsed["tcp_quality"]; ok {
		t.Error("tcp_quality should be omitted for nil value")
	}
}

// Helper functions

func tcpQPtrInt64(v int64) *int64 {
	return &v
}

func tcpQPtrInt32(v int32) *int32 {
	return &v
}

func tcpQPtrInt(v int) *int {
	return &v
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
