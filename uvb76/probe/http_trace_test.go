package probe

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TestHTTPTraceCollector_FreshConnection tests trace collection for a fresh connection with all phases.
func TestHTTPTraceCollector_FreshConnection(t *testing.T) {
	collector := newHTTPTraceCollector("example.com:8080")
	
	// Simulate all phases happening
	collector.mu.Lock()
	collector.dnsStart = collector.startTime.Add(1 * time.Millisecond)
	collector.dnsDone = collector.startTime.Add(5 * time.Millisecond)
	collector.connectStart = collector.startTime.Add(6 * time.Millisecond)
	collector.connectDone = collector.startTime.Add(20 * time.Millisecond)
	collector.tlsStart = collector.startTime.Add(21 * time.Millisecond)
	collector.tlsDone = collector.startTime.Add(50 * time.Millisecond)
	collector.gotConnTime = collector.startTime.Add(51 * time.Millisecond)
	collector.wroteRequestTime = collector.startTime.Add(52 * time.Millisecond)
	collector.firstByteTime = collector.startTime.Add(100 * time.Millisecond)
	collector.bodyReadDone = collector.startTime.Add(110 * time.Millisecond)
	collector.connectionReused = false
	collector.wasIdle = false
	collector.remoteAddr = "192.0.2.1:8080"
	collector.bytesRead = 1024
	collector.mu.Unlock()

	// Build the trace
	trace := collector.BuildHTTPTrace(110.0, 200, nil, false)

	// Verify trace fields
	if trace.Kind != "http" {
		t.Errorf("Kind = %q, want %q", trace.Kind, "http")
	}
	if trace.URLHost != "example.com:8080" {
		t.Errorf("URLHost = %q, want %q", trace.URLHost, "example.com:8080")
	}
	if trace.RemoteAddr != "192.0.2.1" {
		t.Errorf("RemoteAddr = %q, want %q (should be redacted to host only)", trace.RemoteAddr, "192.0.2.1")
	}
	if trace.ConnectionReused {
		t.Error("ConnectionReused = true, want false")
	}
	if trace.WasIdle {
		t.Error("WasIdle = true, want false")
	}

	// Verify phase timings (in ms)
	// DNS: 5-1 = 4ms
	if trace.DNSMs < 3.9 || trace.DNSMs > 4.1 {
		t.Errorf("DNSMs = %f, want ~4ms", trace.DNSMs)
	}
	// TCP: 20-6 = 14ms
	if trace.TCPConnectMs < 13.9 || trace.TCPConnectMs > 14.1 {
		t.Errorf("TCPConnectMs = %f, want ~14ms", trace.TCPConnectMs)
	}
	// TLS: 50-21 = 29ms
	if trace.TLSHandshakeMs < 28.9 || trace.TLSHandshakeMs > 29.1 {
		t.Errorf("TLSHandshakeMs = %f, want ~29ms", trace.TLSHandshakeMs)
	}
	// GotConn: 51ms from start
	if trace.GotConnMs < 50.9 || trace.GotConnMs > 51.1 {
		t.Errorf("GotConnMs = %f, want ~51ms", trace.GotConnMs)
	}
	// TTFB: 100-52 = 48ms
	if trace.TimeToFirstByteMs < 47.9 || trace.TimeToFirstByteMs > 48.1 {
		t.Errorf("TimeToFirstByteMs = %f, want ~48ms", trace.TimeToFirstByteMs)
	}
	// Body: 110-100 = 10ms
	if trace.BodyReadMs < 9.9 || trace.BodyReadMs > 10.1 {
		t.Errorf("BodyReadMs = %f, want ~10ms", trace.BodyReadMs)
	}
	if trace.HTTPStatus != 200 {
		t.Errorf("HTTPStatus = %d, want 200", trace.HTTPStatus)
	}
	if trace.BytesRead != 1024 {
		t.Errorf("BytesRead = %d, want 1024", trace.BytesRead)
	}
	if trace.Error != "" {
		t.Errorf("Error = %q, want empty", trace.Error)
	}
}

// TestHTTPTraceCollector_ReusedConnection tests trace collection for a reused connection.
func TestHTTPTraceCollector_ReusedConnection(t *testing.T) {
	collector := newHTTPTraceCollector("example.com")
	
	// Simulate reused connection - no DNS, no TCP, no TLS
	collector.mu.Lock()
	collector.gotConnTime = collector.startTime.Add(1 * time.Millisecond)
	collector.wroteRequestTime = collector.startTime.Add(2 * time.Millisecond)
	collector.firstByteTime = collector.startTime.Add(10 * time.Millisecond)
	collector.bodyReadDone = collector.startTime.Add(15 * time.Millisecond)
	collector.connectionReused = true
	collector.wasIdle = true
	collector.remoteAddr = "192.0.2.1:443"
	collector.bytesRead = 512
	collector.mu.Unlock()

	trace := collector.BuildHTTPTrace(15.0, 200, nil, false)

	// Verify no DNS/TCP/TLS
	if trace.DNSMs != 0 {
		t.Errorf("DNSMs = %f, want 0 for reused connection", trace.DNSMs)
	}
	if trace.TCPConnectMs != 0 {
		t.Errorf("TCPConnectMs = %f, want 0 for reused connection", trace.TCPConnectMs)
	}
	if trace.TLSHandshakeMs != 0 {
		t.Errorf("TLSHandshakeMs = %f, want 0 for reused connection", trace.TLSHandshakeMs)
	}
	if !trace.ConnectionReused {
		t.Error("ConnectionReused = false, want true")
	}
	if !trace.WasIdle {
		t.Error("WasIdle = false, want true for idle connection")
	}
	// GotConn should be short
	if trace.GotConnMs > 2 {
		t.Errorf("GotConnMs = %f, want < 2ms for reused connection", trace.GotConnMs)
	}
}

// TestHTTPTraceCollector_FailedDNS tests trace collection when DNS fails.
func TestHTTPTraceCollector_FailedDNS(t *testing.T) {
	collector := newHTTPTraceCollector("nonexistent.example")
	
	collector.mu.Lock()
	collector.dnsStart = collector.startTime.Add(1 * time.Millisecond)
	collector.dnsDone = collector.startTime.Add(5000 * time.Millisecond) // 5s timeout
	collector.dnsError = errors.New("server misbehaving")
	collector.mu.Unlock()

	trace := collector.BuildHTTPTrace(5000.0, 0, errors.New("request failed: Get \"https://nonexistent.example/\":"), false)

	if trace.Error == "" {
		t.Error("Error should be set for failed DNS")
	}
	if trace.HTTPStatus != 0 {
		t.Errorf("HTTPStatus = %d, want 0 for failed request", trace.HTTPStatus)
	}
	if trace.DNSMs < 4999 || trace.DNSMs > 5001 {
		t.Errorf("DNSMs = %f, want ~5000ms", trace.DNSMs)
	}
}

// TestHTTPTraceCollector_FailedConnect tests trace collection when TCP connect fails.
func TestHTTPTraceCollector_FailedConnect(t *testing.T) {
	collector := newHTTPTraceCollector("example.com")
	
	collector.mu.Lock()
	collector.dnsStart = collector.startTime.Add(1 * time.Millisecond)
	collector.dnsDone = collector.startTime.Add(5 * time.Millisecond)
	collector.connectStart = collector.startTime.Add(6 * time.Millisecond)
	collector.connectDone = collector.startTime.Add(5000 * time.Millisecond)
	collector.connectError = errors.New("connection refused")
	collector.mu.Unlock()

	trace := collector.BuildHTTPTrace(5000.0, 0, errors.New("request failed: connection refused"), false)

	if trace.Error == "" {
		t.Error("Error should be set for failed connect")
	}
	if !contains(trace.Error, "connect") {
		t.Errorf("Error = %q, should contain 'connect'", trace.Error)
	}
	if trace.TCPConnectMs < 4993 || trace.TCPConnectMs > 5001 {
		t.Errorf("TCPConnectMs = %f, want ~5000ms", trace.TCPConnectMs)
	}
}

// TestHTTPTraceCollector_SlowFirstByte tests trace when TTFB is the slow component.
func TestHTTPTraceCollector_SlowFirstByte(t *testing.T) {
	collector := newHTTPTraceCollector("example.com")
	
	collector.mu.Lock()
	collector.dnsStart = collector.startTime.Add(1 * time.Millisecond)
	collector.dnsDone = collector.startTime.Add(2 * time.Millisecond)
	collector.connectStart = collector.startTime.Add(3 * time.Millisecond)
	collector.connectDone = collector.startTime.Add(10 * time.Millisecond)
	collector.gotConnTime = collector.startTime.Add(11 * time.Millisecond)
	collector.wroteRequestTime = collector.startTime.Add(12 * time.Millisecond)
	// Server is slow to respond
	collector.firstByteTime = collector.startTime.Add(5000 * time.Millisecond) // 5s server delay
	collector.bodyReadDone = collector.startTime.Add(5005 * time.Millisecond)
	collector.connectionReused = false
	collector.remoteAddr = "192.0.2.1:443"
	collector.bytesRead = 100
	collector.mu.Unlock()

	trace := collector.BuildHTTPTrace(5005.0, 200, nil, false)

	// TTFB should be ~4993ms (5000 - 12 + small delta)
	if trace.TimeToFirstByteMs < 4988 || trace.TimeToFirstByteMs > 5000 {
		t.Errorf("TimeToFirstByteMs = %f, want ~4993ms (dominant phase)", trace.TimeToFirstByteMs)
	}
	// Body read should be minimal
	if trace.BodyReadMs > 10 {
		t.Errorf("BodyReadMs = %f, want < 10ms", trace.BodyReadMs)
	}
}

// TestHTTPTraceCollector_SlowBodyRead tests trace when body read is the slow component.
func TestHTTPTraceCollector_SlowBodyRead(t *testing.T) {
	collector := newHTTPTraceCollector("example.com")
	
	collector.mu.Lock()
	collector.dnsStart = collector.startTime.Add(1 * time.Millisecond)
	collector.dnsDone = collector.startTime.Add(2 * time.Millisecond)
	collector.connectStart = collector.startTime.Add(3 * time.Millisecond)
	collector.connectDone = collector.startTime.Add(10 * time.Millisecond)
	collector.gotConnTime = collector.startTime.Add(11 * time.Millisecond)
	collector.wroteRequestTime = collector.startTime.Add(12 * time.Millisecond)
	collector.firstByteTime = collector.startTime.Add(50 * time.Millisecond) // fast TTFB
	// Slow body read (e.g., slow connection)
	collector.bodyReadDone = collector.startTime.Add(5000 * time.Millisecond) // 5s body read
	collector.connectionReused = false
	collector.remoteAddr = "192.0.2.1:443"
	collector.bytesRead = 1024 * 1024 // 1MB
	collector.mu.Unlock()

	trace := collector.BuildHTTPTrace(5000.0, 200, nil, false)

	// TTFB should be ~38ms
	if trace.TimeToFirstByteMs < 37 || trace.TimeToFirstByteMs > 40 {
		t.Errorf("TimeToFirstByteMs = %f, want ~38ms", trace.TimeToFirstByteMs)
	}
	// Body read should dominate
	if trace.BodyReadMs < 4945 || trace.BodyReadMs > 5000 {
		t.Errorf("BodyReadMs = %f, want ~4950ms (dominant phase)", trace.BodyReadMs)
	}
}

// TestHTTPTraceCollector_PartialHooks tests that missing hooks don't cause panic.
func TestHTTPTraceCollector_PartialHooks(t *testing.T) {
	collector := newHTTPTraceCollector("example.com")
	
	// Only set some hooks - simulate partial collection
	collector.mu.Lock()
	collector.gotConnTime = collector.startTime.Add(5 * time.Millisecond)
	collector.firstByteTime = collector.startTime.Add(100 * time.Millisecond)
	collector.bodyReadDone = collector.startTime.Add(110 * time.Millisecond)
	collector.bytesRead = 256
	collector.mu.Unlock()

	// Should not panic
	trace := collector.BuildHTTPTrace(110.0, 200, nil, false)

	if trace.TotalMs != 110.0 {
		t.Errorf("TotalMs = %f, want 110", trace.TotalMs)
	}
	if trace.HTTPStatus != 200 {
		t.Errorf("HTTPStatus = %d, want 200", trace.HTTPStatus)
	}
}

// TestHTTPTraceCollector_SanitizesRemoteAddr tests that remote addr is redacted.
func TestHTTPTraceCollector_SanitizesRemoteAddr(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"192.0.2.1:8080", "192.0.2.1"},
		{"192.0.2.1:443", "192.0.2.1"},
		{"example.com:8080", "example.com"},
		// IPv6 addresses get the port stripped, resulting in just the host
		{"[2001:db8::1]:443", "2001:db8::1"},
		{"just-hostname", "just-hostname"},
		{"", ""},
	}

	for _, tt := range tests {
		collector := newHTTPTraceCollector(tt.input)
		collector.mu.Lock()
		collector.remoteAddr = tt.input
		collector.mu.Unlock()
		collector.Finalize()

		trace := collector.BuildHTTPTrace(10.0, 200, nil, false)
		if trace.RemoteAddr != tt.expected {
			t.Errorf("RemoteAddr for %q = %q, want %q", tt.input, trace.RemoteAddr, tt.expected)
		}
	}
}

// TestHTTPTraceCollector_SanitizesError tests that error messages are sanitized.
func TestHTTPTraceCollector_SanitizesError(t *testing.T) {
	// Error sanitization: removes newlines, truncates to 200 chars
	sanitized := sanitizeError("error\rwith\rcarriage")
	// Should have spaces (cr/lf removed), but not spaces added
	if !strings.Contains(sanitized, "error") {
		t.Errorf("sanitizeError should preserve 'error'")
	}
	// Verify newline removal works
	newlineTest := sanitizeError("error\nwith\nnewlines")
	if strings.Contains(newlineTest, "\n") {
		t.Errorf("sanitizeError should remove newlines, got %q", newlineTest)
	}
	// Verify truncation works
	longErr := strings.Repeat("x", 300)
	if len(sanitizeError(longErr)) != 200 {
		t.Errorf("sanitizeError should truncate to 200 chars, got %d", len(sanitizeError(longErr)))
	}
}

// TestTraceReadCloser tests body read tracking.
func TestTraceReadCloser(t *testing.T) {
	collector := newHTTPTraceCollector("example.com")
	rc := &mockReadCloser{data: []byte("hello world")}
	wrapped := newTraceReadCloser(rc, collector)

	buf := make([]byte, 100)
	n, err := wrapped.Read(buf)
	if err != nil {
		t.Errorf("Read error: %v", err)
	}
	if n != 11 {
		t.Errorf("Read returned %d bytes, want 11", n)
	}

	collector.mu.RLock()
	if collector.bytesRead != 11 {
		t.Errorf("bytesRead = %d, want 11", collector.bytesRead)
	}
	collector.mu.RUnlock()
}

// mockReadCloser implements io.ReadCloser for testing.
type mockReadCloser struct {
	data     []byte
	readIdx  int
}

func (m *mockReadCloser) Read(p []byte) (int, error) {
	if m.readIdx >= len(m.data) {
		return 0, errors.New("EOF")
	}
	n := copy(p, m.data[m.readIdx:])
	m.readIdx += n
	return n, nil
}

func (m *mockReadCloser) Close() error {
	return nil
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

var strings_test = struct {
	Replicate func(string, int) string
}{
	Replicate: func(s string, count int) string {
		result := ""
		for i := 0; i < count; i++ {
			result += s
		}
		return result
	},
}
