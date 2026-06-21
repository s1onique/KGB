// Package diag implements diagnostic capture for UVB-76.
// This file provides route lookup parsing tests.
package diag

import (
	"strings"
	"testing"
)

func TestRouteLookupParser_ParseRouteGetOutput_UnicastViaGateway(t *testing.T) {
	parser := NewRouteLookupParser()
	input := "10.77.0.2 via 192.0.2.1 dev eth0 src 10.0.0.1 uid 0"

	result, err := parser.ParseRouteGetOutput("10.77.0.2", input, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.RouteType != "unicast" {
		t.Errorf("expected route type 'unicast', got '%s'", result.RouteType)
	}
	if result.Interface != "eth0" {
		t.Errorf("expected interface 'eth0', got '%s'", result.Interface)
	}
	if result.SourceIP != "redacted" {
		t.Errorf("expected source 'redacted', got '%s'", result.SourceIP)
	}
	if result.Gateway != "redacted" {
		t.Errorf("expected gateway 'redacted', got '%s'", result.Gateway)
	}
}

func TestRouteLookupParser_ParseRouteGetOutput_DirectRoute(t *testing.T) {
	parser := NewRouteLookupParser()
	input := "10.77.0.2 dev wg-kgb0 src 10.77.0.1 uid 0"

	result, err := parser.ParseRouteGetOutput("10.77.0.2", input, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.RouteType != "unicast" {
		t.Errorf("expected route type 'unicast', got '%s'", result.RouteType)
	}
	if result.Interface != "wg-kgb0" {
		t.Errorf("expected interface 'wg-kgb0', got '%s'", result.Interface)
	}
	if result.SourceIP != "redacted" {
		t.Errorf("expected source 'redacted', got '%s'", result.SourceIP)
	}
	if result.Gateway != "" {
		t.Errorf("expected no gateway for direct route, got '%s'", result.Gateway)
	}
}

func TestRouteLookupParser_ParseRouteGetOutput_LocalRoute(t *testing.T) {
	parser := NewRouteLookupParser()
	input := "local 127.0.0.1 dev lo src 127.0.0.1 uid 0"

	result, err := parser.ParseRouteGetOutput("127.0.0.1", input, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.RouteType != "local" {
		t.Errorf("expected route type 'local', got '%s'", result.RouteType)
	}
	if result.Interface != "lo" {
		t.Errorf("expected interface 'lo', got '%s'", result.Interface)
	}
}

func TestRouteLookupParser_ParseRouteGetOutput_UnreachableRoute(t *testing.T) {
	parser := NewRouteLookupParser()
	input := "unreachable 192.0.2.1 dev eth0 src 10.0.0.1"

	result, err := parser.ParseRouteGetOutput("192.0.2.1", input, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.RouteType != "unreachable" {
		t.Errorf("expected route type 'unreachable', got '%s'", result.RouteType)
	}
}

func TestRouteLookupParser_ParseRouteGetOutput_IPv6Route(t *testing.T) {
	parser := NewRouteLookupParser()
	input := "2001:db8::1 via fe80::1 dev eth0 src 2001:db8::2"

	result, err := parser.ParseRouteGetOutput("2001:db8::1", input, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.RouteType != "unicast" {
		t.Errorf("expected route type 'unicast', got '%s'", result.RouteType)
	}
	if result.Interface != "eth0" {
		t.Errorf("expected interface 'eth0', got '%s'", result.Interface)
	}
}

func TestRouteLookupParser_ParseRouteGetOutput_WithMTU(t *testing.T) {
	parser := NewRouteLookupParser()
	input := "10.77.0.2 dev wg-kgb0 src 10.77.0.1 mtu 1420"

	result, err := parser.ParseRouteGetOutput("10.77.0.2", input, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.MTU == nil {
		t.Error("expected MTU to be set")
	} else if *result.MTU != 1420 {
		t.Errorf("expected MTU 1420, got %d", *result.MTU)
	}
}

func TestRouteLookupParser_ParseRouteGetOutput_WithTable(t *testing.T) {
	parser := NewRouteLookupParser()
	input := "10.77.0.2 dev eth0 src 10.0.0.1 table main"

	result, err := parser.ParseRouteGetOutput("10.77.0.2", input, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Table != "main" {
		t.Errorf("expected table 'main', got '%s'", result.Table)
	}
}

func TestRouteLookupParser_ParseRouteGetOutput_EmptyOutput(t *testing.T) {
	parser := NewRouteLookupParser()
	_, err := parser.ParseRouteGetOutput("10.0.0.1", "", true)
	if err != ErrRouteNoData {
		t.Errorf("expected ErrRouteNoData, got %v", err)
	}
}

func TestRouteLookupParser_ParseRouteGetOutput_UnknownFirstToken(t *testing.T) {
	parser := NewRouteLookupParser()
	// Unknown first token is treated as unicast (graceful degradation)
	input := "unknownkeyword 10.0.0.1 dev eth0"
	result, err := parser.ParseRouteGetOutput("10.0.0.1", input, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Parser defaults to unicast for unknown tokens
	if result.RouteType != "unicast" {
		t.Errorf("expected route type 'unicast', got '%s'", result.RouteType)
	}
}

func TestRouteLookupParser_ParseRouteGetOutput_WithoutRedaction(t *testing.T) {
	parser := NewRouteLookupParser()
	input := "10.77.0.2 via 192.0.2.1 dev eth0 src 10.0.0.1"

	result, err := parser.ParseRouteGetOutput("10.77.0.2", input, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.SourceIP == "redacted" {
		t.Error("expected source to be preserved without redaction")
	}
	if result.Gateway == "redacted" {
		t.Error("expected gateway to be preserved without redaction")
	}
}

func TestRouteLookupParser_ParseRouteGetOutput_BlackholeRoute(t *testing.T) {
	parser := NewRouteLookupParser()
	input := "blackhole 10.77.0.2 dev lo"

	result, err := parser.ParseRouteGetOutput("10.77.0.2", input, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.RouteType != "blackhole" {
		t.Errorf("expected route type 'blackhole', got '%s'", result.RouteType)
	}
}

func TestIsValidRouteTarget(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"10.77.0.2", true},
		{"192.168.1.1", true},
		{"example.com", true},
		{"", false},
		{"; rm -rf /", false},
		{"$(whoami)", false},
		{strings.Repeat("a", 254), false},
	}

	for _, tt := range tests {
		result := isValidRouteTarget(tt.input)
		if result != tt.expected {
			t.Errorf("isValidRouteTarget(%q): expected %v, got %v", tt.input, tt.expected, result)
		}
	}
}

func TestExtractInterfaceField(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"dev eth0", "eth0"},
		{"via 192.0.2.1 dev wg0", "wg0"},
		{"dev", ""},
		{"", ""},
	}

	for _, tt := range tests {
		result := extractInterfaceField(tt.input)
		if result != tt.expected {
			t.Errorf("extractInterfaceField(%q): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestExtractTableField(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"table main", "main"},
		{"table local", "local"},
		{"table 254", "254"},
		{"table", ""},
		{"", ""},
	}

	for _, tt := range tests {
		result := extractTableField(tt.input)
		if result != tt.expected {
			t.Errorf("extractTableField(%q): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestExtractMTUField(t *testing.T) {
	tests := []struct {
		input    string
		expected *int
	}{
		{"mtu 1420", ptrInt(1420)},
		{"mtu 1500", ptrInt(1500)},
		{"mtu", nil},
		{"", nil},
	}

	for _, tt := range tests {
		result := extractMTUField(tt.input)
		if (tt.expected == nil && result != nil) || (tt.expected != nil && result == nil) {
			t.Errorf("extractMTUField(%q): expected %v, got %v", tt.input, tt.expected, result)
		} else if tt.expected != nil && result != nil && *tt.expected != *result {
			t.Errorf("extractMTUField(%q): expected %d, got %d", tt.input, *tt.expected, *result)
		}
	}
}

func ptrInt(v int) *int {
	return &v
}
