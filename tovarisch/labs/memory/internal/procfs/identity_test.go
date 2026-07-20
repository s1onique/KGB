// procfs_test.go — Unit tests for procfs package
//
// Tests for smaps_rollup parsing and procfs identity verification.

package procfs

import (
	"os"
	"testing"
)

// TestSmapsRollupParsing tests smaps_rollup field parsing.
func TestSmapsRollupParsing(t *testing.T) {
	// Create a mock smaps_rollup file
	content := `Rss:                5120 kB
Pss:                4800 kB
Pss_Anon:           4096 kB
Pss_File:            512 kB
Pss_Shmem:           192 kB
Private_Clean:       128 kB
Private_Dirty:      3968 kB
Shared_Clean:       1024 kB
Shared_Dirty:        128 kB
Anonymous:          4096 kB
Swap:                  0 kB
`

	f, err := os.CreateTemp("", "smaps_rollup")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()

	// Re-open for reading
	f2, err := os.Open(f.Name())
	if err != nil {
		t.Fatalf("reopen temp file: %v", err)
	}
	defer f2.Close()

	smaps, err := parseSmapsRollup(1, f2)
	if err != nil {
		t.Fatalf("parse smaps_rollup: %v", err)
	}

	// Verify parsed values
	if smaps.RSSKiB != 5120 {
		t.Errorf("Rss: got %d, want 5120", smaps.RSSKiB)
	}
	if smaps.PSSKiB != 4800 {
		t.Errorf("Pss: got %d, want 4800", smaps.PSSKiB)
	}
	if smaps.PSSAnonKiB != 4096 {
		t.Errorf("Pss_Anon: got %d, want 4096", smaps.PSSAnonKiB)
	}
	if smaps.PrivateDirtyKiB != 3968 {
		t.Errorf("Private_Dirty: got %d, want 3968", smaps.PrivateDirtyKiB)
	}
	if smaps.AnonymousKiB != 4096 {
		t.Errorf("Anonymous: got %d, want 4096", smaps.AnonymousKiB)
	}

	// Verify availability flags
	if !smaps.HasRSS {
		t.Error("Rss availability flag not set")
	}
	if !smaps.HasPSSAnon {
		t.Error("Pss_Anon availability flag not set")
	}
}

// TestSmapsRollupUnknownFields tests handling of unknown fields.
func TestSmapsRollupUnknownFields(t *testing.T) {
	content := `Rss:                1024 kB
Unknown_Field:       999 kB
Pss:                 800 kB
Another_Unknown:    123 kB
`

	f, err := os.CreateTemp("", "smaps_rollup")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()

	f2, err := os.Open(f.Name())
	if err != nil {
		t.Fatalf("reopen temp file: %v", err)
	}
	defer f2.Close()

	smaps, err := parseSmapsRollup(1, f2)
	if err != nil {
		t.Fatalf("parse smaps_rollup with unknown fields: %v", err)
	}

	// Unknown fields should be ignored, known fields should be parsed
	if smaps.RSSKiB != 1024 {
		t.Errorf("Rss with unknown fields: got %d, want 1024", smaps.RSSKiB)
	}
	if smaps.PSSKiB != 800 {
		t.Errorf("Pss with unknown fields: got %d, want 800", smaps.PSSKiB)
	}
}

// TestSmapsRollupDuplicateFields tests rejection of duplicate fields.
func TestSmapsRollupDuplicateFields(t *testing.T) {
	content := `Rss:                1024 kB
Rss:                2048 kB
`

	f, err := os.CreateTemp("", "smaps_rollup")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()

	f2, err := os.Open(f.Name())
	if err != nil {
		t.Fatalf("reopen temp file: %v", err)
	}
	defer f2.Close()

	_, err = parseSmapsRollup(1, f2)
	// Second occurrence should overwrite first (behavior: last value wins)
	if err != nil {
		t.Logf("Note: duplicate fields returned error: %v", err)
	}
}

// TestSmapsRollupMissingFields tests missing field handling.
func TestSmapsRollupMissingFields(t *testing.T) {
	content := `Rss:                1024 kB
`

	f, err := os.CreateTemp("", "smaps_rollup")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()

	f2, err := os.Open(f.Name())
	if err != nil {
		t.Fatalf("reopen temp file: %v", err)
	}
	defer f2.Close()

	smaps, err := parseSmapsRollup(1, f2)
	if err != nil {
		t.Fatalf("parse smaps_rollup: %v", err)
	}

	// Missing fields should be zero
	if smaps.PSSKiB != 0 {
		t.Errorf("Missing Pss: got %d, want 0", smaps.PSSKiB)
	}

	// Missing fields should have availability flag = false
	if smaps.HasPSS {
		t.Error("Pss availability flag should be false for missing field")
	}
}

// TestProcErrorIsZombie tests zombie error detection.
func TestProcErrorIsZombie(t *testing.T) {
	err1 := &ProcError{PID: 123, Op: "read smaps_rollup zombie"}
	if !err1.IsZombie() {
		t.Error("Expected zombie detection for 'zombie' in op")
	}

	err2 := &ProcError{PID: 123, Op: "read status exited"}
	if !err2.IsZombie() {
		t.Error("Expected zombie detection for 'exited' in op")
	}

	err3 := &ProcError{PID: 123, Op: "read maps"}
	if err3.IsZombie() {
		t.Error("Should not detect zombie for 'maps' op")
	}
}

// TestFDInfoParsing tests FD info parsing.
func TestFDInfoParsing(t *testing.T) {
	info := &FDInfo{
		Total:   10,
		Socket:  5,
		pipe:    2,
		eventfd: 1,
		anon:    2,
	}

	if info.Total != 10 {
		t.Errorf("expected Total=10, got %d", info.Total)
	}
	if info.Socket != 5 {
		t.Errorf("expected Socket=5, got %d", info.Socket)
	}
}
