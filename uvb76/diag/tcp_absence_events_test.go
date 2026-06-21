package diag

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/config"
	"github.com/s1onique/KGB/uvb76/state"
)

// =============================================================================
// Unit tests for parseEventFields - typed JSON decoding
// =============================================================================

func TestParseEventFields_BasicFields(t *testing.T) {
	fields := `{"reason": "no_matching_socket"}`
	event := state.DiagEventData{
		Source:  "underlay_tcp",
		Message: "no socket found",
		Fields:  &fields,
	}

	result := parseEventFields(event, nil)

	if result.ReasonCode != "no_matching_socket" {
		t.Errorf("expected reason_code 'no_matching_socket', got '%s'", result.ReasonCode)
	}
	if result.Source != "underlay_tcp" {
		t.Errorf("expected source 'underlay_tcp', got '%s'", result.Source)
	}
}

func TestParseEventFields_FieldsWithSpaces(t *testing.T) {
	fields := `{"reason": "no_matching_socket", "detail": "socket not found in namespace"}`
	event := state.DiagEventData{
		Source:  "underlay_tcp",
		Message: "",
		Fields:  &fields,
	}

	result := parseEventFields(event, nil)

	if result.ReasonCode != "no_matching_socket" {
		t.Errorf("expected reason_code 'no_matching_socket', got '%s'", result.ReasonCode)
	}
	if result.Detail != "socket not found in namespace" {
		t.Errorf("expected detail 'socket not found in namespace', got '%s'", result.Detail)
	}
}

func TestParseEventFields_DetailWithEscapedQuotes(t *testing.T) {
	fields := `{"reason": "command_failed", "detail": "failed: \"permission denied\""}`
	event := state.DiagEventData{
		Source:  "underlay_tcp",
		Message: "",
		Fields:  &fields,
	}

	result := parseEventFields(event, nil)

	if result.ReasonCode != "command_failed" {
		t.Errorf("expected reason_code 'command_failed', got '%s'", result.ReasonCode)
	}
	expectedDetail := `failed: "permission denied"`
	if result.Detail != expectedDetail {
		t.Errorf("expected detail '%s', got '%s'", expectedDetail, result.Detail)
	}
}

func TestParseEventFields_DetailWithBackslashes(t *testing.T) {
	fields := `{"reason": "parse_failed", "detail": "path\\to\\file"}`
	event := state.DiagEventData{
		Source:  "underlay_tcp",
		Message: "",
		Fields:  &fields,
	}

	result := parseEventFields(event, nil)

	if result.ReasonCode != "parse_failed" {
		t.Errorf("expected reason_code 'parse_failed', got '%s'", result.ReasonCode)
	}
	if result.Detail != `path\to\file` {
		t.Errorf("expected detail 'path\\to\\file', got '%s'", result.Detail)
	}
}

func TestParseEventFields_FieldOrderChanged(t *testing.T) {
	fields := `{"detail": "some detail", "reason": "permission_denied", "namespace": "netns0"}`
	event := state.DiagEventData{
		Source:  "underlay_tcp",
		Message: "",
		Fields:  &fields,
	}

	result := parseEventFields(event, nil)

	if result.ReasonCode != "permission_denied" {
		t.Errorf("expected reason_code 'permission_denied', got '%s'", result.ReasonCode)
	}
	if result.Detail != "some detail" {
		t.Errorf("expected detail 'some detail', got '%s'", result.Detail)
	}
	if result.Namespace != "netns0" {
		t.Errorf("expected namespace 'netns0', got '%s'", result.Namespace)
	}
}

func TestParseEventFields_NumericFields(t *testing.T) {
	port := 51820
	rawCount := 5
	fields := `{"reason": "no_matching_socket", "expected_peer": "wg0", "expected_port": 51820, "raw_match_count": 5, "probe_kind": "http"}`
	event := state.DiagEventData{
		Source:  "underlay_tcp",
		Message: "",
		Fields:  &fields,
	}

	result := parseEventFields(event, nil)

	if result.ReasonCode != "no_matching_socket" {
		t.Errorf("expected reason_code 'no_matching_socket', got '%s'", result.ReasonCode)
	}
	if result.ExpectedPeer != "wg0" {
		t.Errorf("expected expected_peer 'wg0', got '%s'", result.ExpectedPeer)
	}
	if result.ExpectedPort == nil || *result.ExpectedPort != port {
		t.Errorf("expected expected_port %d, got %v", port, result.ExpectedPort)
	}
	if result.RawMatchCount == nil || *result.RawMatchCount != rawCount {
		t.Errorf("expected raw_match_count %d, got %v", rawCount, result.RawMatchCount)
	}
	if result.ProbeKind != "http" {
		t.Errorf("expected probe_kind 'http', got '%s'", result.ProbeKind)
	}
}

func TestParseEventFields_MalformedJSON(t *testing.T) {
	fields := `{invalid json`
	event := state.DiagEventData{
		Source:  "underlay_tcp",
		Message: "original message",
		Fields:  &fields,
	}

	result := parseEventFields(event, nil)

	if result.ReasonCode != "parse_failed" {
		t.Errorf("expected reason_code 'parse_failed' for malformed JSON, got '%s'", result.ReasonCode)
	}
	// Original message should be preserved
	if result.Detail != "original message" {
		t.Errorf("expected detail 'original message', got '%s'", result.Detail)
	}
}

func TestParseEventFields_ExitCodeDerivesCommandTool(t *testing.T) {
	fields := `{"reason": "command_failed", "exit_code": 1}`
	event := state.DiagEventData{
		Source:  "underlay_tcp",
		Message: "",
		Fields:  &fields,
	}

	result := parseEventFields(event, nil)

	if result.ReasonCode != "command_failed" {
		t.Errorf("expected reason_code 'command_failed', got '%s'", result.ReasonCode)
	}
	if result.CommandTool != "ss (exit=1)" {
		t.Errorf("expected command_tool 'ss (exit=1)', got '%s'", result.CommandTool)
	}
}

func TestParseEventFields_ExplicitCommandToolTakesPrecedence(t *testing.T) {
	fields := `{"reason": "command_failed", "command_tool": "tcpdiag", "exit_code": 1}`
	event := state.DiagEventData{
		Source:  "underlay_tcp",
		Message: "",
		Fields:  &fields,
	}

	result := parseEventFields(event, nil)

	if result.CommandTool != "tcpdiag" {
		t.Errorf("expected command_tool 'tcpdiag', got '%s'", result.CommandTool)
	}
}

func TestParseEventFields_NilFields(t *testing.T) {
	event := state.DiagEventData{
		Source:  "underlay_tcp",
		Message: "no socket found",
		Fields:  nil,
	}

	result := parseEventFields(event, nil)

	// Should use default reason when no fields
	if result.ReasonCode != "no_matching_socket" {
		t.Errorf("expected default reason_code 'no_matching_socket', got '%s'", result.ReasonCode)
	}
	if result.Detail != "no socket found" {
		t.Errorf("expected detail 'no socket found', got '%s'", result.Detail)
	}
}

func TestParseEventFields_EmptyFields(t *testing.T) {
	fields := ""
	event := state.DiagEventData{
		Source:  "underlay_tcp",
		Message: "empty fields",
		Fields:  &fields,
	}

	result := parseEventFields(event, nil)

	if result.ReasonCode != "no_matching_socket" {
		t.Errorf("expected default reason_code 'no_matching_socket', got '%s'", result.ReasonCode)
	}
	if result.Detail != "empty fields" {
		t.Errorf("expected detail 'empty fields', got '%s'", result.Detail)
	}
}

func TestParseEventFields_PeerContextFallback(t *testing.T) {
	fields := `{"reason": "no_matching_socket"}`
	event := state.DiagEventData{
		Source:  "underlay_tcp",
		Message: "",
		Fields:  &fields,
	}

	peer := &config.DiagPeerConfig{Name: "wg-peer-1"}
	result := parseEventFields(event, peer)

	if result.ExpectedPeer != "wg-peer-1" {
		t.Errorf("expected expected_peer 'wg-peer-1' from peer context, got '%s'", result.ExpectedPeer)
	}
}

func TestParseEventFields_PeerContextDoesNotOverrideExplicit(t *testing.T) {
	fields := `{"reason": "no_matching_socket", "expected_peer": "explicit-peer"}`
	event := state.DiagEventData{
		Source:  "underlay_tcp",
		Message: "",
		Fields:  &fields,
	}

	peer := &config.DiagPeerConfig{Name: "wg-peer-1"}
	result := parseEventFields(event, peer)

	// Explicit expected_peer should take precedence
	if result.ExpectedPeer != "explicit-peer" {
		t.Errorf("expected expected_peer 'explicit-peer', got '%s'", result.ExpectedPeer)
	}
}

// =============================================================================
// Unit tests for buildTcpAbsenceEvents
// =============================================================================

func TestBuildTcpAbsenceEvents_IgnoresNonUnderlayTCP(t *testing.T) {
	events := []state.DiagEventData{
		{
			Source:  "wireguard",
			Message: "wireguard event",
		},
		{
			Source:  "interface",
			Message: "interface event",
		},
	}

	result := buildTcpAbsenceEvents(events, nil)

	if len(result) != 0 {
		t.Errorf("expected no events for non-underlay_tcp sources, got %d", len(result))
	}
}

func TestBuildTcpAbsenceEvents_EmptyUnderlayTCP(t *testing.T) {
	fields := `{"reason": "no_matching_socket"}`
	events := []state.DiagEventData{
		{
			Source:  "underlay_tcp",
			Message: "",
			Fields:  &fields,
		},
	}

	result := buildTcpAbsenceEvents(events, nil)

	if len(result) != 1 {
		t.Errorf("expected 1 absence event, got %d", len(result))
	}
	if result[0].ReasonCode != "no_matching_socket" {
		t.Errorf("expected reason_code 'no_matching_socket', got '%s'", result[0].ReasonCode)
	}
}
func TestBuildTcpAbsenceEvents_MixedEvents(t *testing.T) {
	wgFields := `{"reason": "ok"}`
	tcpFields := `{"reason": "no_matching_socket"}`
	events := []state.DiagEventData{
		{
			Source:  "wireguard",
			Message: "wg event",
			Fields:  &wgFields,
		},
		{
			Source:  "underlay_tcp",
			Message: "",
			Fields:  &tcpFields,
		},
	}

	result := buildTcpAbsenceEvents(events, nil)

	// Only underlay_tcp events should be included
	if len(result) != 1 {
		t.Errorf("expected 1 absence event, got %d", len(result))
	}
	if result[0].Source != "underlay_tcp" {
		t.Errorf("expected source 'underlay_tcp', got '%s'", result[0].Source)
	}
}
