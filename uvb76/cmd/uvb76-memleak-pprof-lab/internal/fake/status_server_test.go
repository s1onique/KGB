// Package fake provides tests for the fake tovarisch status server.
package fake

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStatusServer_Lifecycle(t *testing.T) {
	// Create temp log file
	tmpDir, err := os.MkdirTemp("", "fake-server-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logFile := filepath.Join(tmpDir, "tovarisch.log")

	// Create server
	server := &StatusServer{
		Port:    "16061",
		LogFile: logFile,
	}

	// Start server
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Shutdown(context.Background())

	// Wait a moment for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Make HTTP GET request
	resp, err := http.Get("http://localhost:16061/status")
	if err != nil {
		t.Fatalf("Failed to GET /status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Read response body
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Error("Expected non-empty response body")
	}

	// Verify log file was written
	logData, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if len(logData) == 0 {
		t.Error("Expected log file to have content")
	}
}

func TestStatusServer_Shutdown(t *testing.T) {
	// Create temp log file
	tmpDir, err := os.MkdirTemp("", "fake-server-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logFile := filepath.Join(tmpDir, "tovarisch.log")

	// Create server
	server := &StatusServer{
		Port:    "16062",
		LogFile: logFile,
	}

	// Start server
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Shutdown gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown error: %v", err)
	}

	// Wait should return after shutdown
	done := make(chan struct{})
	go func() {
		server.Wait()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Error("Wait() did not return after shutdown")
	}
}

func TestStatusServer_MethodNotAllowed(t *testing.T) {
	// Create temp log file
	tmpDir, err := os.MkdirTemp("", "fake-server-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logFile := filepath.Join(tmpDir, "tovarisch.log")

	// Create server
	server := &StatusServer{
		Port:    "16063",
		LogFile: logFile,
	}

	// Start server
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Shutdown(context.Background())

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Make HTTP POST request (should be rejected)
	resp, err := http.Post("http://localhost:16063/status", "application/json", nil)
	if err != nil {
		t.Fatalf("Failed to POST /status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", resp.StatusCode)
	}
}

func TestStatusServer_PortConflictFails(t *testing.T) {
	// Create temp log file
	tmpDir, err := os.MkdirTemp("", "fake-server-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logFile := filepath.Join(tmpDir, "tovarisch.log")

	// Create first server
	server1 := &StatusServer{
		Port:    "16064",
		LogFile: logFile,
	}

	if err := server1.Start(); err != nil {
		t.Fatalf("Failed to start first server: %v", err)
	}
	defer server1.Shutdown(context.Background())

	// Give first server time to bind
	time.Sleep(100 * time.Millisecond)

	// Create second server on same port - should fail
	server2 := &StatusServer{
		Port:    "16064",
		LogFile: filepath.Join(tmpDir, "tovarisch2.log"),
	}

	if err := server2.Start(); err == nil {
		server2.Shutdown(context.Background())
		t.Error("Expected error when binding to occupied port, got nil")
	}

	// Verify first server still works
	resp, err := http.Get("http://localhost:16064/status")
	if err != nil {
		t.Fatalf("First server should still be reachable: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("First server should return 200, got %d", resp.StatusCode)
	}
}
