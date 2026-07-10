// Package main provides tests for the lab harness lifecycle management.
package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestProcessState_Running(t *testing.T) {
	ps := &ProcessState{}

	if ps.Running() {
		t.Error("new ProcessState should not be running")
	}
	if ps.Exited() {
		t.Error("new ProcessState should not be exited")
	}

	ps.mu.Lock()
	ps.running = true
	ps.mu.Unlock()

	if !ps.Running() {
		t.Error("ProcessState should be running after setting running=true")
	}
}

func TestProcessState_ExitInfo(t *testing.T) {
	ps := &ProcessState{}

	code, sig := ps.ExitInfo()
	if code != 0 {
		t.Errorf("expected exitCode 0, got %d", code)
	}
	if sig != 0 {
		t.Errorf("expected signal 0, got %v", sig)
	}

	ps.SetExited(42, syscall.SIGSEGV)

	code, sig = ps.ExitInfo()
	if code != 42 {
		t.Errorf("expected exitCode 42, got %d", code)
	}
	if sig != syscall.SIGSEGV {
		t.Errorf("expected SIGSEGV, got %v", sig)
	}
}

func TestProcessState_Exited(t *testing.T) {
	ps := &ProcessState{}

	if ps.Exited() {
		t.Error("new ProcessState should not be exited")
	}

	ps.SetExited(1, 0)

	if !ps.Exited() {
		t.Error("ProcessState should be exited after SetExited")
	}
	if ps.Running() {
		t.Error("ProcessState should not be running after SetExited")
	}
}

func TestLifecycleState_String(t *testing.T) {
	tests := []struct {
		state    LifecycleState
		expected string
	}{
		{StateSetup, "SETUP"},
		{StateLaunching, "LAUNCHING"},
		{StateRunning, "RUNNING"},
		{StateReady, "READY"},
		{StateCollecting, "COLLECTING"},
		{StateCollected, "COLLECTED"},
		{StateVerified, "VERIFIED"},
		{StateShutdown, "SHUTDOWN"},
		{StateFailedStartup, "FAILED_STARTUP"},
		{StateFailedReadiness, "FAILED_READINESS"},
		{StateFailedCollection, "FAILED_COLLECTION"},
		{StateFailedVerification, "FAILED_VERIFICATION"},
		{LifecycleState(999), "UNKNOWN"},
	}

	for _, tc := range tests {
		if got := tc.state.String(); got != tc.expected {
			t.Errorf("LifecycleState(%d).String() = %q, want %q", tc.state, got, tc.expected)
		}
	}
}

func TestBuildCrashEvidence(t *testing.T) {
	evidence := StartupEvidence{
		PID:              1234,
		ExecutablePath:   "/bin/uvb76",
		Args:             []string{"-dev", "-config", "/tmp/config"},
		ChosenPorts: PortsChoice{
			UVB76:     "18444",
			PPROF:     "16060",
			Tovarisch: "18317",
		},
	}

	crash := buildCrashEvidence(evidence, false, false, StateFailedReadiness)

	if crash.PID != 1234 {
		t.Errorf("expected PID 1234, got %d", crash.PID)
	}
	if crash.PPROFReady != false {
		t.Error("expected PPROFReady=false")
	}
	if crash.CollectorStarted != false {
		t.Error("expected CollectorStarted=false")
	}
	if crash.State != "FAILED_READINESS" {
		t.Errorf("expected state FAILED_READINESS, got %s", crash.State)
	}
}

func TestRemoveStaleArtifacts(t *testing.T) {
	// Create temp dir
	tmpDir, err := os.MkdirTemp("", "stale-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create stale files
	staleFiles := []string{"exit.json", "startup_evidence.json"}
	for _, f := range staleFiles {
		path := filepath.Join(tmpDir, f)
		if err := os.WriteFile(path, []byte("stale"), 0644); err != nil {
			t.Fatalf("failed to create stale file %s: %v", f, err)
		}
	}

	// Remove stale artifacts
	if err := removeStaleArtifacts(tmpDir); err != nil {
		t.Fatalf("removeStaleArtifacts failed: %v", err)
	}

	// Verify files are gone
	for _, f := range staleFiles {
		path := filepath.Join(tmpDir, f)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed, but it exists", f)
		}
	}
}

func TestWaitForHTTPReady_Timeout(t *testing.T) {
	// Test with invalid URL - should timeout
	ok := waitForHTTPReady("http://localhost:99999/nonexistent", 100*time.Millisecond)
	if ok {
		t.Error("expected waitForHTTPReady to return false for unreachable URL")
	}
}

func TestStartupEvidence_JSON(t *testing.T) {
	evidence := StartupEvidence{
		LaunchTimestamp:     "2024-01-15T10:30:00Z",
		PID:                12345,
		ExecutablePath:     "/usr/bin/uvb76",
		Args:               []string{"-dev", "-config", "/tmp/lab.json"},
		ConfigPath:         "/tmp/lab.json",
		ChosenPorts: PortsChoice{
			UVB76:     "18444",
			PPROF:     "16060",
			Tovarisch: "18317",
		},
		StartupDurationMs:    150,
		ReadinessDurationMs: 2000,
	}

	// Verify fields are accessible
	if evidence.PID != 12345 {
		t.Errorf("expected PID 12345, got %d", evidence.PID)
	}
	if evidence.StartupDurationMs != 150 {
		t.Errorf("expected StartupDurationMs 150, got %d", evidence.StartupDurationMs)
	}
	if evidence.ReadinessDurationMs != 2000 {
		t.Errorf("expected ReadinessDurationMs 2000, got %d", evidence.ReadinessDurationMs)
	}
	if len(evidence.Args) != 3 {
		t.Errorf("expected 3 args, got %d", len(evidence.Args))
	}
}

func TestCrashEvidence_JSON(t *testing.T) {
	crash := CrashEvidence{
		PID:              12345,
		ExitCode:         1,
		ExitSignal:       11,
		RuntimeMs:        742,
		PPROFReady:       false,
		CollectorStarted: false,
		State:            "FAILED_READINESS",
		StderrExcerpt:    "listen tcp :16060: bind: address already in use",
	}

	// Verify fields
	if crash.PID != 12345 {
		t.Errorf("expected PID 12345, got %d", crash.PID)
	}
	if crash.ExitCode != 1 {
		t.Errorf("expected ExitCode 1, got %d", crash.ExitCode)
	}
	if crash.ExitSignal != 11 {
		t.Errorf("expected ExitSignal 11, got %d", crash.ExitSignal)
	}
	if crash.RuntimeMs != 742 {
		t.Errorf("expected RuntimeMs 742, got %d", crash.RuntimeMs)
	}
	if crash.PPROFReady != false {
		t.Error("expected PPROFReady=false")
	}
	if crash.CollectorStarted != false {
		t.Error("expected CollectorStarted=false")
	}
	if crash.State != "FAILED_READINESS" {
		t.Errorf("expected State FAILED_READINESS, got %s", crash.State)
	}
}
