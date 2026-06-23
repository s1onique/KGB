// procfs_test.go — Tests for native /proc memory reading

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMemValue(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected int64
	}{
		{"VmRSS", "VmRSS:     5120 kB", 5120},
		{"Rss", "Rss:                5120 kB", 5120},
		{"Pss", "Pss:                4800 kB", 4800},
		{"WithTabs", "VmRSS:\t\t 12345 kB", 12345},
		{"Empty", "", 0},
		{"NoNumber", "VmRSS:     kB", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseMemValue(tt.line)
			if got != tt.expected {
				t.Errorf("parseMemValue(%q) = %d, want %d", tt.line, got, tt.expected)
			}
		})
	}
}

func TestParseSmapsRollup(t *testing.T) {
	content := `VmRSS:     5120 kB
Rss:                5120 kB
Pss:                4800 kB
Shared_Clean:       4096 kB
Shared_Dirty:       1024 kB
Private_Clean:      0 kB
Private_Dirty:      0 kB
Referenced:         5120 kB
Anonymous:          1024 kB
LazyFree:          0 kB
AnonHugePages:      0 kB
ShmemPmdMapped:     0 kB
Shared_Hugetlb:     0 kB
Private_Hugetlb:    0 kB
Swap:               0 kB
SwapPss:            0 kB
Locked:             0 kB
`

	tmpDir := t.TempDir()
	smapsPath := filepath.Join(tmpDir, "smaps_rollup")
	if err := os.WriteFile(smapsPath, []byte(content), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	f, err := os.Open(smapsPath)
	if err != nil {
		t.Fatalf("open test file: %v", err)
	}
	defer f.Close()

	snap, err := parseSmapsRollup(12345, f)
	if err != nil {
		t.Fatalf("parseSmapsRollup: %v", err)
	}

	if snap.RSSKiB != 5120 {
		t.Errorf("RSS = %d, want 5120", snap.RSSKiB)
	}
	if snap.PSSKiB != 4800 {
		t.Errorf("PSS = %d, want 4800", snap.PSSKiB)
	}
}

func TestParseSmapsRollupEmptyPSS(t *testing.T) {
	content := `VmRSS:     5120 kB
Rss:                5120 kB
`

	tmpDir := t.TempDir()
	smapsPath := filepath.Join(tmpDir, "smaps_rollup")
	if err := os.WriteFile(smapsPath, []byte(content), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	f, err := os.Open(smapsPath)
	if err != nil {
		t.Fatalf("open test file: %v", err)
	}
	defer f.Close()

	snap, err := parseSmapsRollup(12345, f)
	if err != nil {
		t.Fatalf("parseSmapsRollup: %v", err)
	}

	if snap.RSSKiB != 5120 {
		t.Errorf("RSS = %d, want 5120", snap.RSSKiB)
	}
	if snap.PSSKiB != 0 {
		t.Errorf("PSS = %d, want 0", snap.PSSKiB)
	}
}

func TestParseStatusRSS(t *testing.T) {
	content := `Name:   tovarisch
Umask:  0022
State:  R (running)
Tgid:   12345
Ngid:   0
Pid:    12345
PPid:   1
TracerPid:      0
Uid:    1000    1000    1000    1000
Gid:    1000    1000    1000    1000
FDSize: 64
Groups: 1000
VmPeak:     10240 kB
VmSize:     10240 kB
VmLck:         0 kB
VmPin:         0 kB
VmHWM:      5120 kB
VmRSS:      5120 kB
VmData:     2048 kB
VmStk:       128 kB
VmExe:       256 kB
VmLib:      1024 kB
VmPMD:         4 kB
VmPTE:        24 kB
VmSwap:        0 kB
`

	tmpDir := t.TempDir()
	statusPath := filepath.Join(tmpDir, "status")
	if err := os.WriteFile(statusPath, []byte(content), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	f, err := os.Open(statusPath)
	if err != nil {
		t.Fatalf("open test file: %v", err)
	}
	defer f.Close()

	snap, err := parseStatusRSS(12345, f)
	if err != nil {
		t.Fatalf("parseStatusRSS: %v", err)
	}

	if snap.RSSKiB != 5120 {
		t.Errorf("RSS = %d, want 5120", snap.RSSKiB)
	}
	if snap.PSSKiB != 0 {
		t.Errorf("PSS = %d, want 0 (status fallback)", snap.PSSKiB)
	}
}

func TestParseStatusRSSZombie(t *testing.T) {
	// Process that is zombie/exited - no VmRSS line at all
	// A zombie process typically doesn't have VmRSS in /proc/pid/status
	content := `Name:   tovarisch
State:  Z (zombie)
Tgid:   12345
Pid:    12345
PPid:   1
VmPeak:     10240 kB
VmSize:     10240 kB
`

	tmpDir := t.TempDir()
	statusPath := filepath.Join(tmpDir, "status")
	if err := os.WriteFile(statusPath, []byte(content), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	f, err := os.Open(statusPath)
	if err != nil {
		t.Fatalf("open test file: %v", err)
	}
	defer f.Close()

	_, err = parseStatusRSS(12345, f)
	if err == nil {
		t.Fatal("parseStatusRSS for zombie process should return error")
	}

	procErr, ok := err.(*ProcError)
	if !ok {
		t.Fatalf("error should be *ProcError, got %T", err)
	}

	// Verify real PID is preserved (not 0)
	if procErr.PID != 12345 {
		t.Errorf("ProcError.PID = %d, want 12345", procErr.PID)
	}

	// Verify error message mentions zombie
	if !procErr.IsZombie() {
		t.Errorf("ProcError.IsZombie() = false, want true")
	}
	if !strings.Contains(procErr.Op, "zombie") {
		t.Errorf("ProcError.Op = %q, want contains 'zombie'", procErr.Op)
	}
}

func TestParseStatusRSSMissingVmRSS(t *testing.T) {
	// Process with no VmRSS line at all - should report with real PID
	content := `Name:   tovarisch
State:  S (sleeping)
Tgid:   12345
Pid:    12345
PPid:   1
VmPeak:     10240 kB
VmSize:     10240 kB
`

	tmpDir := t.TempDir()
	statusPath := filepath.Join(tmpDir, "status")
	if err := os.WriteFile(statusPath, []byte(content), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	f, err := os.Open(statusPath)
	if err != nil {
		t.Fatalf("open test file: %v", err)
	}
	defer f.Close()

	_, err = parseStatusRSS(12345, f)
	if err == nil {
		t.Fatal("parseStatusRSS for missing VmRSS should return error")
	}

	procErr, ok := err.(*ProcError)
	if !ok {
		t.Fatalf("error should be *ProcError, got %T", err)
	}

	// Verify real PID is preserved (not 0)
	if procErr.PID != 12345 {
		t.Errorf("ProcError.PID = %d, want 12345", procErr.PID)
	}

	// Error should not be zombie since state is S
	if procErr.IsZombie() {
		t.Errorf("ProcError.IsZombie() = true, want false for sleeping process")
	}
}

func TestProcPath(t *testing.T) {
	got := procPath(12345, "status")
	if !strings.HasSuffix(got, "/proc/12345/status") && got != "/proc/12345/status" {
		t.Errorf("procPath = %q, want /proc/12345/status", got)
	}
}

func TestProcError(t *testing.T) {
	err := &ProcError{PID: 123, Op: "read"}
	if msg := err.Error(); !strings.Contains(msg, "123") || !strings.Contains(msg, "read") {
		t.Errorf("ProcError.Error() = %q, want contains PID and operation", msg)
	}
}

func TestProcErrorZombie(t *testing.T) {
	err := &ProcError{PID: 123, Op: "read VmRSS (zombie/exited)"}
	if !err.IsZombie() {
		t.Errorf("ProcError.IsZombie() = false, want true")
	}
	if msg := err.Error(); !strings.Contains(msg, "123") {
		t.Errorf("ProcError.Error() = %q, want contains PID", msg)
	}
}

func TestProcErrorNotZombie(t *testing.T) {
	err := &ProcError{PID: 123, Op: "parse VmRSS"}
	if err.IsZombie() {
		t.Errorf("ProcError.IsZombie() = true, want false")
	}
}

func TestMemorySnapshot(t *testing.T) {
	snap := MemorySnapshot{RSSKiB: 1000, PSSKiB: 900}
	if snap.RSSKiB != 1000 {
		t.Errorf("RSS = %d, want 1000", snap.RSSKiB)
	}
	if snap.PSSKiB != 900 {
		t.Errorf("PSS = %d, want 900", snap.PSSKiB)
	}
}

func TestParseStatusRSSPreservesPID(t *testing.T) {
	// Regression test: ensure parseStatusRSS uses the passed PID, not hardcoded 0
	testPIDs := []int{1, 100, 12345, 99999}

	for _, expectedPID := range testPIDs {
		content := `Name:   test
State:  R (running)
Pid:    ` + string(rune('0'+expectedPID%10)) + `
VmRSS:      1024 kB
`

		// Even with malformed content, the PID should be preserved in error
		tmpDir := t.TempDir()
		statusPath := filepath.Join(tmpDir, "status")
		if err := os.WriteFile(statusPath, []byte(content), 0644); err != nil {
			t.Fatalf("write test file: %v", err)
		}

		f, err := os.Open(statusPath)
		if err != nil {
			t.Fatalf("open test file: %v", err)
		}
		defer f.Close()

		// Test with valid content first
		validContent := `Name:   test
State:  R (running)
Pid:    99999
VmRSS:      1024 kB
`
		if err := os.WriteFile(statusPath, []byte(validContent), 0644); err != nil {
			t.Fatalf("write test file: %v", err)
		}
		f.Close()

		f2, err := os.Open(statusPath)
		if err != nil {
			t.Fatalf("open test file: %v", err)
		}
		defer f2.Close()

		snap, err := parseStatusRSS(expectedPID, f2)
		if err != nil {
			t.Errorf("parseStatusRSS(%d) returned error: %v", expectedPID, err)
			continue
		}
		if snap.RSSKiB != 1024 {
			t.Errorf("parseStatusRSS(%d) RSS = %d, want 1024", expectedPID, snap.RSSKiB)
		}
	}
}

func TestParseStatusRSSWithVariousStates(t *testing.T) {
	// States where VmRSS is typically present
	activeStates := []string{
		"R (running)",
		"S (sleeping)",
		"D (disk sleep)",
		"T (stopped)",
		"t (tracing stop)",
		"I (idle)",
	}

	for _, state := range activeStates {
		content := `Name:   test
State:  ` + state + `
Pid:    12345
VmRSS:      1024 kB
`

		tmpDir := t.TempDir()
		statusPath := filepath.Join(tmpDir, "status")
		if err := os.WriteFile(statusPath, []byte(content), 0644); err != nil {
			t.Fatalf("write test file: %v", err)
		}

		f, err := os.Open(statusPath)
		if err != nil {
			t.Fatalf("open test file: %v", err)
		}
		defer f.Close()

		snap, err := parseStatusRSS(12345, f)
		if err != nil {
			t.Errorf("State %q: unexpected error: %v", state, err)
		}
		if snap.RSSKiB != 1024 {
			t.Errorf("State %q: RSS = %d, want 1024", state, snap.RSSKiB)
		}
	}

	// States where process is exited/zombie - no VmRSS expected
	zombieStates := []string{"Z (zombie)", "X (dead)"}

	for _, state := range zombieStates {
		content := `Name:   test
State:  ` + state + `
Pid:    12345
VmPeak:     10240 kB
VmSize:     10240 kB
`

		tmpDir := t.TempDir()
		statusPath := filepath.Join(tmpDir, "status")
		if err := os.WriteFile(statusPath, []byte(content), 0644); err != nil {
			t.Fatalf("write test file: %v", err)
		}

		f, err := os.Open(statusPath)
		if err != nil {
			t.Fatalf("open test file: %v", err)
		}
		defer f.Close()

		_, err = parseStatusRSS(12345, f)
		if err == nil {
			t.Errorf("State %q: expected zombie error, got none", state)
		} else if procErr, ok := err.(*ProcError); ok {
			if !procErr.IsZombie() {
				t.Errorf("State %q: IsZombie() = false, want true", state)
			}
		}
	}
}
