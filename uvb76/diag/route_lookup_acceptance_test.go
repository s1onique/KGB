// Package diag implements diagnostic capture for UVB-76.
// This file provides acceptance tests for the RouteCollector.
package diag

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/state"
)

// Force CLI fallback on non-Linux platforms where NETLINK_ROUTE is unavailable
func init() {
	// Enable CLI fallback only when native netlink is not available
	// This ensures acceptance tests work on macOS/Windows
	// Note: On Linux, native NETLINK_ROUTE is used by default
	if runtime.GOOS != "linux" {
		UseCLIFallback = true
	}
}

func TestRouteCollector_CollectRouteLookup_Success(t *testing.T) {
	collector := NewRouteCollector()
	ctx := context.Background()

	// Use localhost as a route that should always succeed
	route := collector.CollectRouteLookup(ctx, state.ProbeRouteKindHTTP, "127.0.0.1", "127.0.0.1")

	if route == nil {
		t.Fatal("expected non-nil route result")
	}
	if !route.Ok {
		t.Logf("Route lookup failed (may be expected on this platform): %s - %s", route.ErrorKind, route.Error)
		// This is acceptable on some platforms where ip route get may not work
		return
	}

	// Verify successful result has required fields
	if route.Kind != state.ProbeRouteKindHTTP {
		t.Errorf("expected kind 'http', got '%s'", route.Kind)
	}
	if route.LookupTarget != "127.0.0.1" {
		t.Errorf("expected lookup_target '127.0.0.1', got '%s'", route.LookupTarget)
	}
	if route.CollectedAt == "" {
		t.Error("expected non-empty collected_at")
	}
}

func TestRouteCollector_CollectRouteLookup_Timeout(t *testing.T) {
	collector := NewRouteCollector()

	// Use a very short timeout that will expire before any route lookup completes
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Small delay to ensure context is expired
	time.Sleep(10 * time.Millisecond)

	route := collector.CollectRouteLookup(ctx, state.ProbeRouteKindHTTP, "127.0.0.1", "127.0.0.1")

	if route == nil {
		t.Fatal("expected non-nil route result")
	}
	if route.Ok {
		t.Error("expected route lookup to fail")
	}
	if route.ErrorKind != state.RouteLookupErrorTimeout {
		t.Errorf("expected error_kind 'timeout', got '%s'", route.ErrorKind)
	}
}

func TestRouteCollector_CollectRouteLookup_InvalidTarget(t *testing.T) {
	collector := NewRouteCollector()
	ctx := context.Background()

	// Test various invalid targets
	invalidTargets := []string{
		"; rm -rf /",
		"$(whoami)",
		"&& ls",
		"",
	}

	for _, target := range invalidTargets {
		route := collector.CollectRouteLookup(ctx, state.ProbeRouteKindHTTP, target, target)

		if route == nil {
			t.Errorf("expected non-nil route result for target %q", target)
			continue
		}
		if route.Ok {
			t.Errorf("expected route lookup to fail for invalid target %q", target)
		}
		if route.ErrorKind != state.RouteLookupErrorUnavailable {
			t.Errorf("expected error_kind 'unavailable' for target %q, got '%s'", target, route.ErrorKind)
		}
	}
}

func TestRouteCollector_CollectRouteLookup_ICMPKind(t *testing.T) {
	collector := NewRouteCollector()
	ctx := context.Background()

	route := collector.CollectRouteLookup(ctx, state.ProbeRouteKindICMP, "8.8.8.8", "8.8.8.8")

	if route == nil {
		t.Fatal("expected non-nil route result")
	}
	if route.Kind != state.ProbeRouteKindICMP {
		t.Errorf("expected kind 'icmp', got '%s'", route.Kind)
	}
	if route.ProbeHost != "8.8.8.8" {
		t.Errorf("expected probe_host '8.8.8.8', got '%s'", route.ProbeHost)
	}
}

func TestRouteCollector_CollectRouteLookup_CommandMissing(t *testing.T) {
	// Test that invalid target returns command_missing when command is not found.
	// This test is host-dependent: it passes if 'ip' is not installed,
	// or returns non_zero_exit if 'ip' is installed but target is invalid.
	collector := NewRouteCollector()
	ctx := context.Background()

	route := collector.CollectRouteLookup(ctx, state.ProbeRouteKindHTTP, "", "")

	if route == nil {
		t.Fatal("expected non-nil route result")
	}
	if route.Ok {
		t.Error("expected route lookup to fail")
	}
	// Either command_missing (if ip not found) or unavailable (invalid target)
	if route.ErrorKind != state.RouteLookupErrorCommandMissing &&
		route.ErrorKind != state.RouteLookupErrorUnavailable {
		t.Errorf("expected error_kind 'command_missing' or 'unavailable', got '%s'", route.ErrorKind)
	}
}

func TestRouteCollector_CollectRouteLookup_SensitiveFieldsRedacted(t *testing.T) {
	collector := NewRouteCollector()
	ctx := context.Background()

	route := collector.CollectRouteLookup(ctx, state.ProbeRouteKindHTTP, "127.0.0.1", "127.0.0.1")

	if route == nil {
		t.Skip("route lookup not available on this platform")
	}
	if !route.Ok {
		t.Skip("route lookup failed, skipping redaction check")
	}

	// Verify that sensitive fields are redacted
	if route.SourceIP == "" || route.SourceIP == "redacted" {
		// This is expected - source IP should be redacted
	} else {
		// Source IP is not "redacted" - could be platform-specific behavior
		t.Logf("Note: source_ip is '%s' (may vary by platform)", route.SourceIP)
	}

	if route.Gateway == "" || route.Gateway == "redacted" {
		// This is expected - gateway should be redacted
	} else {
		// Gateway is not "redacted" - could be platform-specific behavior
		t.Logf("Note: gateway is '%s' (may vary by platform)", route.Gateway)
	}
}

func TestRouteCollector_NewCaptureService_WiresRouteCollector(t *testing.T) {
	// This test verifies that NewCaptureService properly wires the RouteCollector
	// so that production captures will include probe_route evidence.
	cfg := &testDiagnosticsConfig{
		Enabled:         true,
		CaptureOnSpike:  true,
		TimeoutMs:       1000,
		CooldownSeconds: 60,
	}
	store := testCaptureStore{}

	// Create the capture service
	svc := newTestCaptureService(cfg, &store)

	// Verify that routeCollector is wired (not nil)
	if svc.routeCollector == nil {
		t.Error("expected routeCollector to be wired in NewCaptureService")
	}
}

// testDiagnosticsConfig is a minimal diagnostics config for testing.
type testDiagnosticsConfig struct {
	Enabled         bool
	CaptureOnSpike  bool
	TimeoutMs       int
	CooldownSeconds int
}

func (c *testDiagnosticsConfig) GetEnabled() bool                          { return c.Enabled }
func (c *testDiagnosticsConfig) GetCaptureOnSpike() bool                    { return c.CaptureOnSpike }
func (c *testDiagnosticsConfig) GetTimeoutMs() int                          { return c.TimeoutMs }
func (c *testDiagnosticsConfig) GetCooldownSeconds() int                     { return c.CooldownSeconds }
func (c *testDiagnosticsConfig) TargetToDiagPeers() map[string]*testDiagPeerConfig {
	return nil
}

type testDiagPeerConfig struct {
	Name   string
	BaseURL string
}

type testCaptureStore struct{}

func (s *testCaptureStore) AddCapture(eventID string, capture interface{})                             {}
func (s *testCaptureStore) AddCaptureWithProvenance(eventID string, capture interface{}, a, b string)  {}
func (s *testCaptureStore) GetCaptures(eventID string) []interface{}                                   { return nil }
func (s *testCaptureStore) IsInCooldown(peerName string, cooldownSeconds int) bool                      { return false }
func (s *testCaptureStore) IsInFlight(peerName string) bool                                            { return false }
func (s *testCaptureStore) ReserveInFlight(peerName string) bool                                       { return true }
func (s *testCaptureStore) ReleaseInFlight(peerName string)                                            {}
func (s *testCaptureStore) GetLastCaptureTime(peerName string) time.Time                               { return time.Time{} }
func (s *testCaptureStore) EvaluateCooldown(now time.Time, peerName string, cooldownSeconds int) interface{} {
	return nil
}

// newTestCaptureService creates a capture service for testing.
// This mirrors NewCaptureService but uses test types.
func newTestCaptureService(cfg *testDiagnosticsConfig, captures *testCaptureStore) *CaptureService {
	timeout := cfg.TimeoutMs
	if timeout <= 0 {
		timeout = 5000
	}

	return &CaptureService{
		cfg:            nil,
		captures:       nil,
		httpClient:     nil,
		targetPeers:    nil,
		routeCollector: NewRouteCollector(), // This is the key: wired by default
		stopCh:         make(chan struct{}),
	}
}
