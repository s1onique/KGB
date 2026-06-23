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

	snap, err := parseSmapsRollup(f)
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

	snap, err := parseSmapsRollup(f)
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

	snap, err := parseStatusRSS(f)
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

func TestMemorySnapshot(t *testing.T) {
	snap := MemorySnapshot{RSSKiB: 1000, PSSKiB: 900}
	if snap.RSSKiB != 1000 {
		t.Errorf("RSS = %d, want 1000", snap.RSSKiB)
	}
	if snap.PSSKiB != 900 {
		t.Errorf("PSS = %d, want 900", snap.PSSKiB)
	}
}
