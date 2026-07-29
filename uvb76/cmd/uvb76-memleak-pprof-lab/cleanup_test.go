// Package main provides tests for the memory lab.
package main

import (
	"testing"
	"time"
)

// TestVerifyPPROFEndpoints_InvalidPort tests endpoint verification with unreachable port.
func TestVerifyPPROFEndpoints_InvalidPort(t *testing.T) {
	// Use a port that's unlikely to be in use
	result := verifyPPROFEndpoints("59999")
	if result {
		t.Error("expected false for unreachable port")
	}
}

// TestWaitForPPROFReady_AlreadyExited tests readiness check when process already exited.
func TestWaitForPPROFReady_AlreadyExited(t *testing.T) {
	ps := &ProcessState{}
	ps.done = make(chan struct{})

	// Simulate already exited
	ps.mu.Lock()
	ps.running = false
	ps.exited = true
	ps.exitCode = 1
	ps.mu.Unlock()
	close(ps.done)

	ready, err := waitForPPROFReady("59999", 5*time.Second, ps)
	if ready {
		t.Error("expected ready=false for exited process")
	}
	if err == nil {
		t.Error("expected error for exited process")
	}
}
