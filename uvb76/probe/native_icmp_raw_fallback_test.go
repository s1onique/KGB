package probe

import (
	"errors"
	"testing"
)

// ============================================================================
// Raw ICMP Fallback Tests
// ============================================================================

// multiSocketFakeOpener attempts dgram socket first, then raw socket.
type multiSocketFakeOpener struct {
	dgramErr error                  // error to return for dgram socket attempt
	rawConn  NativeICMPPacketConn // successful raw socket connection
	rawErr   error                  // error to return for raw socket attempt (if rawConn is nil)
}

func (f *multiSocketFakeOpener) OpenSocket() (*SocketOpenResult, error) {
	result := f.tryOpenSocket()

	// Store result for status reporting (like real opener does)
	lastSocketOpenResult.Store(result)

	if result.Conn != nil {
		return result, nil
	}
	if result.DgramError != "" && result.RawError != "" {
		return result, errors.New("dgram failed: " + result.DgramError + "; raw failed: " + result.RawError)
	}
	if result.DgramError != "" {
		return result, errors.New(result.DgramError)
	}
	if result.RawError != "" {
		return result, errors.New(result.RawError)
	}
	return result, errors.New("unknown error")
}

func (f *multiSocketFakeOpener) tryOpenSocket() *SocketOpenResult {
	result := &SocketOpenResult{}

	// Simulate dgram socket attempt
	if f.dgramErr != nil {
		result.DgramError = extractSocketError(f.dgramErr)
		// Only try raw if dgram failed with permission error
		if !isPermissionError(f.dgramErr) {
			return result
		}
		// Try raw socket
		if f.rawConn != nil {
			result.Conn = f.rawConn
			result.SocketMode = SocketModeRawICMP
			return result
		}
		if f.rawErr != nil {
			result.RawError = extractSocketError(f.rawErr)
			return result
		}
		return result
	}

	// Dgram succeeded
	result.Conn = &fakeICMPPacketConn{}
	result.SocketMode = SocketModeDgramICMP
	return result
}

func TestRawICMPFallbackSelectsRawSocket(t *testing.T) {
	// Simulate RT-AX88U: dgram fails with EACCES, raw succeeds
	fakeOpener := &multiSocketFakeOpener{
		dgramErr: errors.New("permission denied"),
		rawConn:  &fakeICMPPacketConn{},
	}

	backend, err := NewNativeICMPBackendWithOpener(fakeOpener)
	if err != nil {
		t.Fatalf("expected backend to succeed with raw fallback, got error: %v", err)
	}
	defer backend.Close()

	// Verify socket mode is raw
	result := GetLastSocketOpenResult()
	if result == nil {
		t.Fatal("expected socket result to be recorded")
	}
	if result.SocketMode != SocketModeRawICMP {
		t.Errorf("expected socket mode raw_icmp, got %v", result.SocketMode)
	}
	if result.DgramError == "" {
		t.Error("expected dgram error to be recorded")
	}
}

func TestDgramICMPSucceedsWhenAvailable(t *testing.T) {
	fakeOpener := &multiSocketFakeOpener{
		dgramErr: nil, // success
	}

	backend, err := NewNativeICMPBackendWithOpener(fakeOpener)
	if err != nil {
		t.Fatalf("expected backend to succeed with dgram, got error: %v", err)
	}
	defer backend.Close()

	// Verify socket mode is dgram
	result := GetLastSocketOpenResult()
	if result == nil {
		t.Fatal("expected socket result to be recorded")
	}
	if result.SocketMode != SocketModeDgramICMP {
		t.Errorf("expected socket mode dgram_icmp, got %v", result.SocketMode)
	}
	if result.DgramError != "" {
		t.Errorf("expected no dgram error, got %v", result.DgramError)
	}
}

func TestBothSocketsFailReturnsError(t *testing.T) {
	fakeOpener := &multiSocketFakeOpener{
		dgramErr: errors.New("permission denied"),
		rawErr:   errors.New("operation not permitted"),
	}

	backend, err := NewNativeICMPBackendWithOpener(fakeOpener)
	if err == nil {
		if backend != nil {
			backend.Close()
		}
		t.Fatal("expected error when both sockets fail")
	}

	// Verify both errors are captured
	result := GetLastSocketOpenResult()
	if result == nil {
		t.Fatal("expected socket result to be recorded")
	}
	if result.DgramError == "" {
		t.Error("expected dgram error to be recorded")
	}
	if result.RawError == "" {
		t.Error("expected raw error to be recorded")
	}
}

func TestNonPermissionErrorDoesNotRetryRaw(t *testing.T) {
	// Non-permission error (e.g., address in use) should not retry raw socket
	fakeOpener := &multiSocketFakeOpener{
		dgramErr: errors.New("address already in use"),
		rawConn:  &fakeICMPPacketConn{}, // This should NOT be used
	}

	backend, err := NewNativeICMPBackendWithOpener(fakeOpener)
	if err == nil {
		if backend != nil {
			backend.Close()
		}
		t.Fatal("expected error for non-permission dgram failure")
	}

	// Verify raw socket was NOT tried (no raw error recorded)
	result := GetLastSocketOpenResult()
	if result == nil {
		t.Fatal("expected socket result to be recorded")
	}
	if result.RawError != "" {
		t.Error("expected raw error to be empty (raw socket should not be tried)")
	}
}

func TestExtractSocketError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"nil error", nil, ""},
		{"EACCES explicit", errors.New("dgram connect: EACCES"), "EACCES (permission denied)"},
		{"EPERM", errors.New("operation not permitted"), "EPERM (operation not permitted)"},
		{"generic", errors.New("some other error"), "some other error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSocketError(tt.err)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestSocketOpenResultStatusReporting(t *testing.T) {
	// Test that SocketOpenResult captures all necessary fields for status reporting
	fakeOpener := &multiSocketFakeOpener{
		dgramErr: errors.New("permission denied"),
		rawConn:  &fakeICMPPacketConn{},
	}

	_, err := NewNativeICMPBackendWithOpener(fakeOpener)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}

	// Get socket result directly (like status handler does)
	result := GetLastSocketOpenResult()
	if result == nil {
		t.Fatal("expected socket result to be recorded")
	}

	// Verify socket mode diagnostics are present
	if result.SocketMode != SocketModeRawICMP {
		t.Errorf("expected socket_mode raw_icmp, got %v", result.SocketMode)
	}
	if result.DgramError == "" {
		t.Error("expected dgram_error to be populated")
	}
	if result.Conn == nil {
		t.Error("expected connection to be set")
	}
}

