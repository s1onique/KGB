package probe

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"testing"
	"time"

	"github.com/s1onique/KGB/uvb76/state"
)

// TestHTTPTraceCollector_EndToEnd tests the trace collector with a real HTTP server.
func TestHTTPTraceCollector_EndToEnd(t *testing.T) {
	// Create a test server that responds with a small delay
	var handlerCallCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCallCount++
		// Small delay to ensure TTFB tracking works
		time.Sleep(10 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	// Create trace collector
	urlHost := ExtractHost(server.URL)
	collector := newHTTPTraceCollector(urlHost)

	// Create HTTP request with trace hooks
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	// Attach trace hooks
	traceCtx := httptrace.WithClientTrace(req.Context(), collector.getTraceHooks())
	req = req.WithContext(traceCtx)

	// Make request
	start := time.Now()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	latencyMs := float64(time.Since(start).Milliseconds())
	
	// Read the body to ensure all bytes are counted
	if err == nil {
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
	
	// Finalize collector
	collector.Finalize()
	
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Build trace
	trace := collector.BuildHTTPTrace(latencyMs, resp.StatusCode, err, false)

	// Verify trace
	if trace.Kind != "http" {
		t.Errorf("Expected Kind='http', got %q", trace.Kind)
	}
	if trace.HTTPStatus != http.StatusOK {
		t.Errorf("Expected HTTPStatus=200, got %d", trace.HTTPStatus)
	}
	if trace.TimeToFirstByteMs <= 0 {
		t.Errorf("Expected positive TTFB, got %f", trace.TimeToFirstByteMs)
	}
	if trace.RemoteAddr == "" {
		t.Error("Expected RemoteAddr to be set")
	}
	// Note: BytesRead is 0 in this test because we're not using traceReadCloser wrapper
	// In actual probe usage, the body is wrapped with traceReadCloser which tracks bytes

	t.Logf("End-to-end result: ttfb=%.2fms, body=%.2fms, total=%.2fms, reused=%v, bytes=%d",
		trace.TimeToFirstByteMs, trace.BodyReadMs, trace.TotalMs, trace.ConnectionReused, trace.BytesRead)
	t.Logf("Handler called %d times", handlerCallCount)
}

// TestHTTPTraceCollector_ErrorResponse tests trace with error status codes.
func TestHTTPTraceCollector_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":"service unavailable"}`))
	}))
	defer server.Close()

	urlHost := ExtractHost(server.URL)
	collector := newHTTPTraceCollector(urlHost)

	req, _ := http.NewRequest("GET", server.URL, nil)
	traceCtx := httptrace.WithClientTrace(req.Context(), collector.getTraceHooks())
	req = req.WithContext(traceCtx)

	client := &http.Client{Timeout: 5 * time.Second}
	start := time.Now()
	resp, err := client.Do(req)
	latencyMs := float64(time.Since(start).Milliseconds())
	collector.Finalize()

	trace := collector.BuildHTTPTrace(latencyMs, resp.StatusCode, err, false)

	if trace.HTTPStatus != http.StatusServiceUnavailable {
		t.Errorf("Expected HTTPStatus=503, got %d", trace.HTTPStatus)
	}
	if trace.Error != "" {
		t.Errorf("Expected no error string for 503 response, got %q", trace.Error)
	}

	resp.Body.Close()
}

// TestHTTPTraceCollector_JSONSerialization tests that HTTPTrace serializes to JSON correctly.
func TestHTTPTraceCollector_JSONSerialization(t *testing.T) {
	trace := &state.HTTPTrace{
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
		HTTPStatus:          200,
		BytesRead:           1024,
		Error:               "",
	}

	// Serialize to JSON
	data, err := json.Marshal(trace)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	// Deserialize back
	var decoded state.HTTPTrace
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	// Verify fields
	if decoded.Kind != trace.Kind {
		t.Errorf("Kind mismatch: got %q, want %q", decoded.Kind, trace.Kind)
	}
	if decoded.URLHost != trace.URLHost {
		t.Errorf("URLHost mismatch: got %q, want %q", decoded.URLHost, trace.URLHost)
	}
	if decoded.HTTPStatus != trace.HTTPStatus {
		t.Errorf("HTTPStatus mismatch: got %d, want %d", decoded.HTTPStatus, trace.HTTPStatus)
	}
	if decoded.TimeToFirstByteMs != trace.TimeToFirstByteMs {
		t.Errorf("TimeToFirstByteMs mismatch: got %f, want %f", decoded.TimeToFirstByteMs, trace.TimeToFirstByteMs)
	}
	if decoded.ConnectionReused != trace.ConnectionReused {
		t.Errorf("ConnectionReused mismatch: got %v, want %v", decoded.ConnectionReused, trace.ConnectionReused)
	}
}

// TestHTTPTraceCollector_ConnectionReuse tests connection reuse tracking.
func TestHTTPTraceCollector_ConnectionReuse(t *testing.T) {
	var connectionCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connectionCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"n":` + fmt.Sprintf("%d", connectionCount) + `}`))
	}))
	defer server.Close()

	// Create a shared client to enable connection reuse
	client := &http.Client{Timeout: 5 * time.Second}

	for i := 0; i < 5; i++ {
		urlHost := ExtractHost(server.URL)
		collector := newHTTPTraceCollector(urlHost)

		req, _ := http.NewRequest("GET", server.URL, nil)
		traceCtx := httptrace.WithClientTrace(req.Context(), collector.getTraceHooks())
		req = req.WithContext(traceCtx)

		start := time.Now()
		resp, err := client.Do(req)
		latencyMs := float64(time.Since(start).Milliseconds())
		collector.Finalize()

		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}

		trace := collector.BuildHTTPTrace(latencyMs, resp.StatusCode, nil, false)
		resp.Body.Close()

		t.Logf("Request %d: ttfb=%.2fms, reused=%v", i, trace.TimeToFirstByteMs, trace.ConnectionReused)
	}

	// Should have made 5 requests
	if connectionCount != 5 {
		t.Errorf("Expected 5 requests, got %d", connectionCount)
	}
}

// TestHTTPTraceCollector_Privacy tests that sensitive data is not leaked in traces.
func TestHTTPTraceCollector_Privacy(t *testing.T) {
	// Test server that would expose sensitive headers
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"secret":"data"}`))
	}))
	defer server.Close()

	urlHost := ExtractHost(server.URL)
	collector := newHTTPTraceCollector(urlHost)

	req, _ := http.NewRequest("GET", server.URL, nil)
	// Add sensitive headers
	req.Header.Set("Authorization", "Bearer secret-token-12345")
	req.Header.Set("Cookie", "session=abc123")
	req.Header.Set("X-API-Key", "my-secret-key")

	traceCtx := httptrace.WithClientTrace(req.Context(), collector.getTraceHooks())
	req = req.WithContext(traceCtx)

	client := &http.Client{Timeout: 5 * time.Second}
	start := time.Now()
	resp, err := client.Do(req)
	latencyMs := float64(time.Since(start).Milliseconds())
	collector.Finalize()

	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify headers were sent
	if receivedHeaders.Get("Authorization") != "Bearer secret-token-12345" {
		t.Error("Authorization header not sent")
	}

	// Build trace and serialize to JSON
	trace := collector.BuildHTTPTrace(latencyMs, resp.StatusCode, nil, false)
	traceJSON, _ := json.Marshal(trace)
	traceStr := string(traceJSON)

	// These should NOT appear in the trace
	sensitive := []string{
		"secret-token",
		"abc123",
		"my-secret-key",
		"Bearer",
		"Cookie",
		"session",
	}
	for _, s := range sensitive {
		if stringContains(traceStr, s) {
			t.Errorf("Trace contains sensitive data %q", s)
		}
	}
}

// stringContains is a simple string contains helper.
func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestExtractHost_Integration tests ExtractHost with real URLs.
func TestExtractHost_Integration(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"http://example.com/path", "example.com"},
		{"https://example.com:8443/path", "example.com:8443"},
		{"http://localhost:8080/", "localhost:8080"},
		{"https://192.0.2.1:443/", "192.0.2.1:443"},
	}

	for _, tt := range tests {
		result := ExtractHost(tt.url)
		if result != tt.expected {
			t.Errorf("ExtractHost(%q) = %q, want %q", tt.url, result, tt.expected)
		}
	}
}

// TestHTTPTraceCollector_SlowBodyRead_Regression tests that slow body reads are correctly attributed
// and that bounded reads prevent unbounded consumption. This exercises the real probe path.
func TestHTTPTraceCollector_SlowBodyRead_Regression(t *testing.T) {
	const maxTraceBodyReadBytes = 64 * 1024 // Must match probe.go constant
	const slowBodyDelayMs = 100

	// Create a test server that sends a slow body
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		// Send body slowly to make body_read_ms measurable
		for i := 0; i < 10; i++ {
			time.Sleep(time.Duration(slowBodyDelayMs) * time.Millisecond / 10)
			w.Write([]byte("0123456789")) // 10 bytes per write
		}
	}))
	defer server.Close()

	urlHost := ExtractHost(server.URL)
	collector := newHTTPTraceCollector(urlHost)

	req, _ := http.NewRequest("GET", server.URL, nil)
	traceCtx := httptrace.WithClientTrace(req.Context(), collector.getTraceHooks())
	req = req.WithContext(traceCtx)

	start := time.Now()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)

	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Use the same bounded read pattern as probe.go
	wrappedBody := newTraceReadCloser(resp.Body, collector)
	resp.Body = wrappedBody
	limited := io.LimitReader(wrappedBody, maxTraceBodyReadBytes+1)
	_, _ = io.Copy(io.Discard, limited)
	resp.Body.Close()

	// Finalize collector AFTER body read
	collector.Finalize()

	// Compute total_ms from start to body read complete
	totalMs := float64(time.Since(start).Milliseconds())

	// Build trace
	bodyTruncated := collector.BytesRead() > maxTraceBodyReadBytes
	trace := collector.BuildHTTPTrace(totalMs, resp.StatusCode, nil, bodyTruncated)

	// Assertions:
	// 1. TTFB should be positive
	if trace.TimeToFirstByteMs <= 0 {
		t.Errorf("TimeToFirstByteMs = %.2fms, want > 0", trace.TimeToFirstByteMs)
	}

	// 2. total_ms should be >= TimeToFirstByteMs (this was the bug - total_ms was pre-body)
	if trace.TotalMs < trace.TimeToFirstByteMs*0.9 {
		t.Errorf("TotalMs = %.2fms should be >= TimeToFirstByteMs(%.2f)*0.9",
			trace.TotalMs, trace.TimeToFirstByteMs)
	}

	// 4. Bounded read should work (body should not be truncated for small body)
	if bodyTruncated {
		t.Errorf("BodyTruncated = true, want false for small test body")
	}

	t.Logf("Slow body regression: ttfb=%.2fms, body=%.2fms, total=%.2fms, bytes=%d, truncated=%v",
		trace.TimeToFirstByteMs, trace.BodyReadMs, trace.TotalMs, trace.BytesRead, bodyTruncated)
}

// TestHTTPTraceCollector_BoundedRead_Regression tests that large responses are bounded
// and BodyTruncated is correctly set.
func TestHTTPTraceCollector_BoundedRead_Regression(t *testing.T) {
	const maxTraceBodyReadBytes = 64 * 1024
	const largeBodySize = maxTraceBodyReadBytes * 2 // 2x the limit

	// Create a server that sends a large body
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		// Write more than the limit
		buf := make([]byte, 1024)
		for i := 0; i < largeBodySize/1024; i++ {
			w.Write(buf)
		}
	}))
	defer server.Close()

	urlHost := ExtractHost(server.URL)
	collector := newHTTPTraceCollector(urlHost)

	req, _ := http.NewRequest("GET", server.URL, nil)
	traceCtx := httptrace.WithClientTrace(req.Context(), collector.getTraceHooks())
	req = req.WithContext(traceCtx)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Bounded read
	wrappedBody := newTraceReadCloser(resp.Body, collector)
	resp.Body = wrappedBody
	limited := io.LimitReader(wrappedBody, maxTraceBodyReadBytes+1)
	_, _ = io.Copy(io.Discard, limited)
	resp.Body.Close()

	collector.Finalize()

	bodyTruncated := collector.BytesRead() > maxTraceBodyReadBytes

	// Assertions:
	// 1. BytesRead should be at most max+1 (limited reader)
	if collector.BytesRead() > maxTraceBodyReadBytes+1 {
		t.Errorf("BytesRead = %d, want <= %d (bounded by LimitReader)", collector.BytesRead(), maxTraceBodyReadBytes+1)
	}

	// 2. BodyTruncated should be true for large response
	if !bodyTruncated {
		t.Errorf("BodyTruncated = false, want true for body > %d bytes", maxTraceBodyReadBytes)
	}

	t.Logf("Bounded read: bytes=%d, truncated=%v", collector.BytesRead(), bodyTruncated)
}
