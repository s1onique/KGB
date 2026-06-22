package probe

import (
	"errors"
	"testing"

	"github.com/s1onique/KGB/uvb76/config"
)

// TestNativeICMPStatsSnapshot verifies stats snapshot contains all fields.
func TestNativeICMPStatsSnapshot(t *testing.T) {
	stats := &nativeICMPStatsInternal{}
	stats.sent.Store(100)
	stats.received.Store(95)
	stats.timeouts.Store(5)
	stats.socketOpenErrors.Store(0)
	stats.permissionErrors.Store(0)
	stats.parseErrors.Store(0)
	stats.unmatchedReplies.Store(2)
	stats.lastRTTMillis.Store(12)
	stats.lastErrorClass.Store("timeout")

	snap := stats.Snapshot()

	if snap.Backend != "native" {
		t.Errorf("expected backend 'native', got %q", snap.Backend)
	}
	if snap.Sent != 100 {
		t.Errorf("expected sent=100, got %d", snap.Sent)
	}
	if snap.Received != 95 {
		t.Errorf("expected received=95, got %d", snap.Received)
	}
	if snap.Timeouts != 5 {
		t.Errorf("expected timeouts=5, got %d", snap.Timeouts)
	}
	if snap.UnmatchedReplies != 2 {
		t.Errorf("expected unmatched_replies=2, got %d", snap.UnmatchedReplies)
	}
	if snap.LastRTTMillis != 12 {
		t.Errorf("expected last_rtt_ms=12, got %d", snap.LastRTTMillis)
	}
	if snap.LastErrorClass != "timeout" {
		t.Errorf("expected last_error_class='timeout', got %q", snap.LastErrorClass)
	}
}

// TestNativeICMPErrorMethods tests error wrapping and unwrapping.
func TestNativeICMPErrorMethods(t *testing.T) {
	innerErr := errors.New("inner error")
	icmpErr := NewNativeICMPError(ErrClassPermission, innerErr, "actionable message")

	// Test Error()
	if icmpErr.Error() != "inner error" {
		t.Errorf("Error() = %q, want %q", icmpErr.Error(), "inner error")
	}

	// Test Unwrap()
	if !errors.Is(icmpErr, innerErr) {
		t.Error("Unwrap should return inner error")
	}

	// Test ErrorClass
	if icmpErr.ErrorClass != ErrClassPermission {
		t.Errorf("ErrorClass = %q, want %q", icmpErr.ErrorClass, ErrClassPermission)
	}

	// Test UserMessage
	if icmpErr.UserMessage != "actionable message" {
		t.Errorf("UserMessage = %q, want %q", icmpErr.UserMessage, "actionable message")
	}
}

// TestIsPermissionError tests permission error detection.
func TestIsPermissionError(t *testing.T) {
	tests := []struct {
		errStr string
		want   bool
	}{
		{"permission denied", true},
		{"operation not permitted", true},
		{"EPERM", true},
		{"EACCES", true},
		{"timeout", false},
		{"connection refused", false},
	}

	for _, tt := range tests {
		t.Run(tt.errStr, func(t *testing.T) {
			err := errors.New(tt.errStr)
			got := isPermissionError(err)
			if got != tt.want {
				t.Errorf("isPermissionError(%q) = %v, want %v", tt.errStr, got, tt.want)
			}
		})
	}
}

// TestContainsAny tests the containsAny helper.
func TestContainsAny(t *testing.T) {
	tests := []struct {
		s      string
		substr []string
		want   bool
	}{
		{"hello world", []string{"hello"}, true},
		{"hello world", []string{"world"}, true},
		{"hello world", []string{"foo"}, false},
		{"HELLO WORLD", []string{"hello"}, true},
		{"hello world", []string{"foo", "bar", "world"}, true},
		{"hello world", []string{"foo", "bar"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := containsAny(tt.s, tt.substr...)
			if got != tt.want {
				t.Errorf("containsAny(%q, %v) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}

// boolPtr is a helper to get a pointer to a bool.
func boolPtr(b bool) *bool {
	return &b
}

func TestICMPBackendSelectionNative(t *testing.T) {
	cfg := &config.ICMPProbeConfig{Enabled: boolPtr(true), Backend: config.ICMPBackendNative}
	if cfg.BackendType() != config.ICMPBackendNative {
		t.Errorf("expected backend type %q, got %q", config.ICMPBackendNative, cfg.BackendType())
	}
}

func TestICMPBackendSelectionOSPingExplicit(t *testing.T) {
	cfg := &config.ICMPProbeConfig{Enabled: boolPtr(true), Backend: config.ICMPBackendOSPing}
	if cfg.BackendType() != config.ICMPBackendOSPing {
		t.Errorf("expected backend type %q, got %q", config.ICMPBackendOSPing, cfg.BackendType())
	}
}
