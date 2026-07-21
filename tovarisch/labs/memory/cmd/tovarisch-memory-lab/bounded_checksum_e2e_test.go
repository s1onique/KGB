// bounded_checksum_e2e_test.go — End-to-end checksum parser/verifier
// tests for CORRECTION03.
//
// These tests cover the four mandatory end-to-end verifier
// mutations from ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-
// QUALIFICATION01-CORRECTION03 §7:
//   - genuine traversal mutation (preserves all canonical
//     checksums, appends a traversal entry);
//   - unexpected local checksum (extra.json);
//   - non-hex 64-character checksum (modifies one canonical
//     entry to exactly 64 non-hex characters);
//   - self-checksum entry (checksums.txt);
//
// plus the positive baseline control.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeAdditionalChecksumLines appends lines to the existing
// checksums.txt without disturbing any canonical entry.
func writeAdditionalChecksumLines(t *testing.T, boundDir string, lines ...string) {
	t.Helper()
	checksumPath := filepath.Join(boundDir, "checksums.txt")
	f, err := os.OpenFile(checksumPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open append: %v", err)
	}
	defer f.Close()
	for _, line := range lines {
		if _, err := f.WriteString(line + "\n"); err != nil {
			t.Fatalf("append %q: %v", line, err)
		}
	}
}

// replaceChecksumForPath replaces the entire checksum line for path
// in checksums.txt with one that uses exactly 64 z characters as the
// checksum (non-hex). The path is preserved. The function uses
// positional string arithmetic (not regex) so the resulting line is
// exactly 64-char hash + two-space separator + path.
func replaceChecksumForPath(t *testing.T, boundDir, path string) {
	t.Helper()
	checksumPath := filepath.Join(boundDir, "checksums.txt")
	data, err := os.ReadFile(checksumPath)
	if err != nil {
		t.Fatalf("read checksums: %v", err)
	}
	prefix := "  " + path
	idx := strings.Index(string(data), prefix)
	if idx < 0 {
		t.Fatalf("path not found in checksums: %s", path)
	}
	hashStart := idx - 64
	if hashStart < 0 {
		t.Fatalf("no room for 64-char hash before %q", prefix)
	}
	nonHexHash := strings.Repeat("z", 64)
	modified := string(data)[:hashStart] + nonHexHash + string(data)[hashStart+64:]
	if err := os.WriteFile(checksumPath, []byte(modified), 0644); err != nil {
		t.Fatalf("write checksums: %v", err)
	}
}

// TestE2E_GenuinePathTraversal appends a path-traversal entry to
// a valid bound fixture (preserving every canonical checksum) and
// asserts the verifier's traversal diagnostic fires.
func TestE2E_GenuinePathTraversal(t *testing.T) {
	dst := t.TempDir()
	boundDir := copyBoundedFixture(t, dst)
	rebindFixture(t, boundDir)
	bad := strings.Repeat("a", 64) + "  ../escape.json"
	writeAdditionalChecksumLines(t, boundDir, bad)

	out, err := runVerifier(t, dst)
	if err == nil {
		t.Fatalf("traversal: verifier accepted; output:\n%s", out)
	}
	if !strings.Contains(out, "invalid checksum artifact path:") {
		t.Errorf("traversal: wrong diagnostic (want \"invalid checksum artifact path:\"):\n%s", out)
	}
}

// TestE2E_UnexpectedLocalChecksum appends an extra.json entry to
// a valid bound fixture and asserts the verifier's
// "unexpected checksum entry" diagnostic fires.
func TestE2E_UnexpectedLocalChecksum(t *testing.T) {
	dst := t.TempDir()
	boundDir := copyBoundedFixture(t, dst)
	rebindFixture(t, boundDir)
	extra := strings.Repeat("a", 64) + "  extra.json"
	writeAdditionalChecksumLines(t, boundDir, extra)

	out, err := runVerifier(t, dst)
	if err == nil {
		t.Fatalf("unexpected local: verifier accepted; output:\n%s", out)
	}
	if !strings.Contains(out, "unexpected checksum entry: extra.json") {
		t.Errorf("unexpected local: wrong diagnostic (want \"unexpected checksum entry: extra.json\"):\n%s", out)
	}
}

// TestE2E_NonHexChecksum64Char modifies one canonical checksum to
// exactly 64 non-hex characters while preserving its path, and
// asserts the verifier's encoding diagnostic fires.
func TestE2E_NonHexChecksum64Char(t *testing.T) {
	dst := t.TempDir()
	boundDir := copyBoundedFixture(t, dst)
	rebindFixture(t, boundDir)
	replaceChecksumForPath(t, boundDir, "workload-result.json")

	out, err := runVerifier(t, dst)
	if err == nil {
		t.Fatalf("non-hex: verifier accepted; output:\n%s", out)
	}
	if !strings.Contains(out, "invalid checksum hash encoding:") {
		t.Errorf("non-hex: wrong diagnostic (want \"invalid checksum hash encoding:\"):\n%s", out)
	}
}

// TestE2E_SelfChecksumEntry appends a checksum entry for
// checksums.txt (the canonical forbidden path) and asserts the
// verifier's unexpected-checksum-entry diagnostic fires.
func TestE2E_SelfChecksumEntry(t *testing.T) {
	dst := t.TempDir()
	boundDir := copyBoundedFixture(t, dst)
	rebindFixture(t, boundDir)
	selfEntry := strings.Repeat("a", 64) + "  checksums.txt"
	writeAdditionalChecksumLines(t, boundDir, selfEntry)

	out, err := runVerifier(t, dst)
	if err == nil {
		t.Fatalf("self-checksum: verifier accepted; output:\n%s", out)
	}
	if !strings.Contains(out, "unexpected checksum entry: checksums.txt") {
		t.Errorf("self-checksum: wrong diagnostic (want \"unexpected checksum entry: checksums.txt\"):\n%s", out)
	}
}
