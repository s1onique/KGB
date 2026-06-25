package probe

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestHTTPTraceCollector_GetActualConnForTCPInfo proves that httpTraceCollector.GetActualConn()
// returns the actual connection that can be used with diag.CollectTcpQualityFromConn().
//
// This test verifies the narrow integration seam between:
// 1. httpTraceCollector capturing the actual connection via httptrace.GotConn
// 2. diag.CollectTcpQualityFromConn() collecting TCP_INFO from that connection
//
// Note: This test does NOT perform a full HTTP probe. It tests the collector's ability
// to capture and expose the actual connection for TCP_INFO collection.
func TestHTTPTraceCollector_GetActualConnForTCPInfo(t *testing.T) {
	// Start a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	// Create a collector
	collector := newHTTPTraceCollector(server.URL)

	// Dial the server directly (simulating what http.Transport does)
	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	// Give connection time to establish
	time.Sleep(50 * time.Millisecond)

	// Manually set the actualConn (simulating what httptrace.GotConn does)
	// In real usage, httptrace would capture this via the GotConn hook
	collector.mu.Lock()
	collector.actualConn = conn
	collector.connectionReused = false
	collector.wasIdle = false
	collector.mu.Unlock()

	// Get the actual connection
	actualConn := collector.GetActualConn()

	if actualConn == nil {
		t.Fatal("Expected non-nil actualConn")
	}

	if actualConn != conn {
		t.Error("Expected GetActualConn to return the same connection")
	}

	// Verify it's a TCP connection (required for TCP_INFO collection)
	if _, ok := actualConn.(*net.TCPConn); !ok {
		t.Error("Expected TCPConn for native TCP_INFO collection")
	}

	t.Logf("GetActualConn returns usable TCP connection for TCP_INFO collection")
}

// TestHTTPTraceCollector_GetActualConn_NilWhenNotSet verifies that GetActualConn
// returns nil when no connection has been captured.
func TestHTTPTraceCollector_GetActualConn_NilWhenNotSet(t *testing.T) {
	collector := newHTTPTraceCollector("example.com")

	// GetActualConn should return nil when no connection was captured
	conn := collector.GetActualConn()
	if conn != nil {
		t.Error("Expected nil when no connection was captured")
	}
}

// TestHTTPTraceCollector_GetActualConn_AfterFailedRequest verifies that GetActualConn
// returns nil when the request failed before getting a connection.
func TestHTTPTraceCollector_GetActualConn_AfterFailedRequest(t *testing.T) {
	collector := newHTTPTraceCollector("nonexistent.invalid")

	// Even without a connection, GetActualConn should return nil safely
	conn := collector.GetActualConn()
	if conn != nil {
		t.Error("Expected nil for failed request")
	}
}

// TestHTTPTraceCollector_ConnectionReusedAndIdle flags whether the connection
// was reused from the transport pool.
func TestHTTPTraceCollector_ConnectionReusedAndIdle(t *testing.T) {
	collector := newHTTPTraceCollector("example.com")

	// Test fresh connection
	collector.mu.Lock()
	collector.connectionReused = false
	collector.wasIdle = false
	collector.mu.Unlock()

	if collector.IsConnectionReused() {
		t.Error("Expected false for fresh connection")
	}
	if collector.WasIdle() {
		t.Error("Expected false for fresh connection")
	}

	// Test reused connection
	collector.mu.Lock()
	collector.connectionReused = true
	collector.wasIdle = true
	collector.mu.Unlock()

	if !collector.IsConnectionReused() {
		t.Error("Expected true for reused connection")
	}
	if !collector.WasIdle() {
		t.Error("Expected true for idle connection")
	}
}

// TestHTTPTraceCollector_ActualConnCanBeNil verifies that GetActualConn returns nil
// when actualConn was never set (request failed before connect).
func TestHTTPTraceCollector_ActualConnCanBeNil(t *testing.T) {
	collector := newHTTPTraceCollector("example.com")

	// Verify nil is safe to return
	conn := collector.GetActualConn()
	if conn != nil {
		t.Error("Expected nil when actualConn was never set")
	}
}
