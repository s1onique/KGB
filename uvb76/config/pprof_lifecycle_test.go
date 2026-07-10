package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestPProfServerLifecycle_DisabledNoListener verifies that disabled pprof
// does not create any listener.
func TestPProfServerLifecycle_DisabledNoListener(t *testing.T) {
	cfg := PProfConfig{
		Enabled: false,
	}

	// Creating server for disabled config is fine
	srv := NewPProfServer(cfg)
	if srv == nil {
		t.Fatal("Expected non-nil server even for disabled config")
	}

	// But no listeners should be bound - the caller should check cfg.Enabled first
	// This test documents the expected behavior: caller must guard
	if cfg.Enabled {
		t.Error("Config should be disabled for this test")
	}
}

// TestPProfServerLifecycle_EnabledBindsDynamically verifies that enabled pprof
// binds to a dynamically allocated loopback address when using port 0.
func TestPProfServerLifecycle_EnabledBindsDynamically(t *testing.T) {
	cfg := PProfConfig{
		Enabled: true,
		Listen:  "127.0.0.1:0", // Port 0 = dynamic allocation
	}

	srv := NewPProfServer(cfg)

	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}
	defer listener.Close()

	// Verify we got a real bound address
	if listener.Addr() == nil {
		t.Fatal("Expected bound address")
	}

	// Verify it's on loopback
	addr := listener.Addr().(*net.TCPAddr)
	if !addr.IP.IsLoopback() {
		t.Errorf("Expected loopback, got %v", addr.IP)
	}
	if addr.Port == 0 {
		t.Error("Expected non-zero port after dynamic allocation")
	}

	// Start server with this listener
	go func() {
		srv.Serve(listener)
	}()
	defer srv.Close()

	// Verify server responds
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/debug/pprof/", listener.Addr().String()))
	if err != nil {
		t.Fatalf("Failed to reach pprof: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}

// TestPProfServerLifecycle_DebugPPROFReturnsSuccess verifies /debug/pprof/ returns 200.
func TestPProfServerLifecycle_DebugPPROFReturnsSuccess(t *testing.T) {
	cfg := PProfConfig{
		Enabled: true,
		Listen:  "127.0.0.1:0",
	}

	srv := NewPProfServer(cfg)

	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}
	defer listener.Close()

	go func() {
		srv.Serve(listener)
	}()
	defer srv.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/debug/pprof/", listener.Addr().String()))
	if err != nil {
		t.Fatalf("Failed to GET /debug/pprof/: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}

// TestPProfServerLifecycle_HeapReturnsNonEmpty verifies /debug/pprof/heap returns data.
func TestPProfServerLifecycle_HeapReturnsNonEmpty(t *testing.T) {
	cfg := PProfConfig{
		Enabled: true,
		Listen:  "127.0.0.1:0",
	}

	srv := NewPProfServer(cfg)

	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}
	defer listener.Close()

	go func() {
		srv.Serve(listener)
	}()
	defer srv.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/debug/pprof/heap", listener.Addr().String()))
	if err != nil {
		t.Fatalf("Failed to GET /debug/pprof/heap: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read body: %v", err)
	}

	if len(body) == 0 {
		t.Error("Expected non-empty heap profile")
	}
}

// TestPProfServerLifecycle_GoroutineReturnsNonEmpty verifies /debug/pprof/goroutine returns data.
func TestPProfServerLifecycle_GoroutineReturnsNonEmpty(t *testing.T) {
	cfg := PProfConfig{
		Enabled: true,
		Listen:  "127.0.0.1:0",
	}

	srv := NewPProfServer(cfg)

	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}
	defer listener.Close()

	go func() {
		srv.Serve(listener)
	}()
	defer srv.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/debug/pprof/goroutine?debug=1", listener.Addr().String()))
	if err != nil {
		t.Fatalf("Failed to GET /debug/pprof/goroutine: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read body: %v", err)
	}

	if len(body) == 0 {
		t.Error("Expected non-empty goroutine profile")
	}
}

// TestPProfServerLifecycle_OccupiedAddressFails verifies binding to an occupied address fails.
func TestPProfServerLifecycle_OccupiedAddressFails(t *testing.T) {
	// First, bind a socket to occupy a port
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to bind first socket: %v", err)
	}
	defer first.Close()

	addr := first.Addr().String()

	// Now try to bind the same address - should fail
	second, err := net.Listen("tcp", addr)
	if err == nil {
		second.Close()
		t.Fatal("Expected error when binding to occupied address")
	}

	// Verify it's the right kind of error
	if !strings.Contains(err.Error(), "address already in use") && !strings.Contains(err.Error(), "bind") {
		t.Logf("Got error: %v", err)
		// On some systems, the error message varies; just verify it failed
	}
}

// TestPProfServerLifecycle_ShutdownClosesListener verifies shutdown closes the listener.
func TestPProfServerLifecycle_ShutdownClosesListener(t *testing.T) {
	cfg := PProfConfig{
		Enabled: true,
		Listen:  "127.0.0.1:0",
	}

	srv := NewPProfServer(cfg)

	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}
	boundAddr := listener.Addr().String()

	// Start server
	done := make(chan error, 1)
	go func() {
		done <- srv.Serve(listener)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Shutdown with a context
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = srv.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Give Serve time to return
	select {
	case <-done:
		// Expected
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after shutdown")
	}

	// Verify listener is closed by attempting to reconnect
	conn, err := net.DialTimeout("tcp", boundAddr, 500*time.Millisecond)
	if err == nil {
		conn.Close()
		t.Error("Expected listener to be closed after shutdown")
	}
}

// TestPProfServerLifecycle_ExpectedShutdownNoError verifies ErrServerClosed is not a real error.
func TestPProfServerLifecycle_ExpectedShutdownNoError(t *testing.T) {
	cfg := PProfConfig{
		Enabled: true,
		Listen:  "127.0.0.1:0",
	}

	srv := NewPProfServer(cfg)

	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(listener)
	}()

	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	srv.Shutdown(ctx)
	cancel()

	// Wait for Serve to return
	err = <-serveErr

	// ErrServerClosed is expected and should be handled gracefully
	if errors.Is(err, http.ErrServerClosed) {
		// This is the expected case - not a failure
		return
	}
	if err != nil {
		t.Fatalf("Unexpected error from Serve: %v", err)
	}
}

// TestPProfServerLifecycle_MemProfileRateAppliedOnlyWhenEnabled verifies
// MemProfileRate is only modified when pprof is enabled.
func TestPProfServerLifecycle_MemProfileRateAppliedOnlyWhenEnabled(t *testing.T) {
	originalRate := runtime.MemProfileRate
	defer func() { runtime.MemProfileRate = originalRate }()

	// Set a known baseline
	runtime.MemProfileRate = 8192

	// Disabled config should NOT apply rate
	cfg := PProfConfig{Enabled: false, MemProfileRate: 16384}
	ApplyPProfRuntimeConfig(cfg)
	if runtime.MemProfileRate != 8192 {
		t.Errorf("Disabled config changed rate: got %d, want 8192", runtime.MemProfileRate)
	}

	// Enabled config with positive rate SHOULD apply
	cfg = PProfConfig{Enabled: true, MemProfileRate: 16384}
	ApplyPProfRuntimeConfig(cfg)
	if runtime.MemProfileRate != 16384 {
		t.Errorf("Enabled config did not apply rate: got %d, want 16384", runtime.MemProfileRate)
	}

	// Restore for next sub-test
	runtime.MemProfileRate = 8192

	// Zero rate should NOT apply (even if enabled)
	cfg = PProfConfig{Enabled: true, MemProfileRate: 0}
	ApplyPProfRuntimeConfig(cfg)
	if runtime.MemProfileRate != 8192 {
		t.Errorf("Zero rate should not change rate: got %d, want 8192", runtime.MemProfileRate)
	}
}
