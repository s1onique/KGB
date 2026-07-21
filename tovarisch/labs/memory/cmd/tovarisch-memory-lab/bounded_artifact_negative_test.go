// bounded_artifact_negative_test.go — Bounded artifact geometry and
// inventory mutations.
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01-CORRECTION01
// §5.4: geometry/inventory mutations test the artifact
// geometry/inventory validator; checksum corruption tests the
// checksum validator; undeclared artifact tests exact inventory.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestArtifact_RemoveCanonicalArtifact rejects an evidence bundle
// where one of the 10 canonical artifacts is missing.
func TestArtifact_RemoveCanonicalArtifact(t *testing.T) {
	dst := t.TempDir()
	boundDir := copyBoundedFixture(t, dst)
	rebindFixture(t, boundDir)

	// Remove one canonical artifact without recomputing checksums
	// (the inventory check is the one that fires).
	if err := os.Remove(filepath.Join(boundDir, "workload-result.json")); err != nil {
		t.Fatalf("remove artifact: %v", err)
	}

	out, err := runVerifier(t, dst)
	if err == nil {
		t.Fatalf("remove artifact: verifier accepted; output:\n%s", out)
	}
	if !strings.Contains(out, "missing file from inventory: workload-result.json") {
		t.Errorf("remove artifact: wrong diagnostic:\n%s", out)
	}
}

// TestArtifact_AddUndeclaredArtifact rejects an evidence bundle
// where an extra file has been added beyond the declared inventory.
func TestArtifact_AddUndeclaredArtifact(t *testing.T) {
	dst := t.TempDir()
	boundDir := copyBoundedFixture(t, dst)
	rebindFixture(t, boundDir)

	// Add an undeclared file.
	if err := os.WriteFile(filepath.Join(boundDir, "extra-file.txt"), []byte("sneaky"), 0644); err != nil {
		t.Fatalf("write extra: %v", err)
	}

	out, err := runVerifier(t, dst)
	if err == nil {
		t.Fatalf("add undeclared artifact: verifier accepted; output:\n%s", out)
	}
	if !strings.Contains(out, "unexpected file not in inventory: extra-file.txt") {
		t.Errorf("add undeclared artifact: wrong diagnostic:\n%s", out)
	}
}

// TestArtifact_CorruptChecksum rejects an evidence bundle where
// checksums.txt contains a corrupted hash. The intended rejection
// path is the checksum validator (not the checksum parser).
func TestArtifact_CorruptChecksum(t *testing.T) {
	dst := t.TempDir()
	boundDir := copyBoundedFixture(t, dst)
	rebindFixture(t, boundDir)

	// Corrupt a checksum: flip one hex character in the middle of
	// the hash (preserves length so the parser accepts the line, but
	// the value no longer matches the recomputed SHA-256).
	checksumPath := filepath.Join(boundDir, "checksums.txt")
	data, err := os.ReadFile(checksumPath)
	if err != nil {
		t.Fatalf("read checksums: %v", err)
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Flip one hex character: 0 -> 1 in the middle of the hash.
		idx := strings.Index(line, "  ")
		if idx < 0 {
			continue
		}
		hash := line[:idx]
		// Flip the 32nd hex character (somewhere in the middle).
		if len(hash) > 32 && hash[32] == '0' {
			line = line[:32] + "1" + line[33:]
		} else if len(hash) > 32 {
			line = line[:32] + "0" + line[33:]
		}
		lines[i] = line
		break
	}
	if err := os.WriteFile(checksumPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		t.Fatalf("write checksums: %v", err)
	}

	out, err := runVerifier(t, dst)
	if err == nil {
		t.Fatalf("corrupt checksum: verifier accepted; output:\n%s", out)
	}
	if !strings.Contains(out, "checksum mismatch for") {
		t.Errorf("corrupt checksum: wrong diagnostic:\n%s", out)
	}
}

// TestArtifact_RemoveChecksumEntry rejects an evidence bundle where
// checksums.txt is missing a checksum line for a canonical artifact.
func TestArtifact_RemoveChecksumEntry(t *testing.T) {
	dst := t.TempDir()
	boundDir := copyBoundedFixture(t, dst)
	rebindFixture(t, boundDir)

	// Replace checksums.txt with an empty file (removes all entries).
	if err := os.WriteFile(filepath.Join(boundDir, "checksums.txt"), []byte(""), 0644); err != nil {
		t.Fatalf("write empty checksums: %v", err)
	}

	out, err := runVerifier(t, dst)
	if err == nil {
		t.Fatalf("remove checksum entry: verifier accepted; output:\n%s", out)
	}
	if !strings.Contains(out, "missing checksum for:") {
		t.Errorf("remove checksum entry: wrong diagnostic:\n%s", out)
	}
}

// TestArtifact_DuplicateChecksumEntry rejects an evidence bundle
// where checksums.txt contains a duplicate entry. The production
// parser rejects duplicates with a "duplicate entry" error.
func TestArtifact_DuplicateChecksumEntry(t *testing.T) {
	dst := t.TempDir()
	boundDir := copyBoundedFixture(t, dst)
	rebindFixture(t, boundDir)

	// Read the current checksums and append a duplicate.
	checksumPath := filepath.Join(boundDir, "checksums.txt")
	data, err := os.ReadFile(checksumPath)
	if err != nil {
		t.Fatalf("read checksums: %v", err)
	}
	// Append the first non-empty line again to create a duplicate.
	lines := strings.Split(string(data), "\n")
	dupLine := ""
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			dupLine = line
			break
		}
	}
	if dupLine == "" {
		t.Fatalf("no checksum line to duplicate")
	}
	combined := string(data) + dupLine + "\n"
	if err := os.WriteFile(checksumPath, []byte(combined), 0644); err != nil {
		t.Fatalf("write duplicate checksums: %v", err)
	}

	// ParseChecksumsFile returns "duplicate entry for: <path>".
	out, err := runVerifier(t, dst)
	if err == nil {
		t.Fatalf("duplicate checksum: verifier accepted; output:\n%s", out)
	}
	if !strings.Contains(out, "duplicate entry for:") {
		t.Errorf("duplicate checksum: wrong diagnostic (want \"duplicate entry for:\"):\n%s", out)
	}
}

// TestArtifact_ChecksumPathTraversal rejects an evidence bundle
// where checksums.txt contains a path-traversal entry. The
// replacement overwrites every checksum with one entry whose path
// is outside the manifest inventory, so the verifier's
// "missing checksum for:" diagnostic fires for the inventory
// item whose checksum was lost.
func TestArtifact_ChecksumPathTraversal(t *testing.T) {
	dst := t.TempDir()
	boundDir := copyBoundedFixture(t, dst)
	rebindFixture(t, boundDir)

	// Overwrite checksums.txt with a single path-traversal line.
	checksumPath := filepath.Join(boundDir, "checksums.txt")
	bad := strings.Repeat("a", 64) + "  ../escape.json\n"
	if err := os.WriteFile(checksumPath, []byte(bad), 0644); err != nil {
		t.Fatalf("write bad checksums: %v", err)
	}

	out, err := runVerifier(t, dst)
	if err == nil {
		t.Fatalf("path traversal: verifier accepted; output:\n%s", out)
	}
	// The replacement removes every inventory checksum and leaves
	// only the path-traversal entry, so the verifier's
	// "missing checksum for:" diagnostic fires for the first
	// inventory item not in checksums.txt.
	if !strings.Contains(out, "invalid checksum artifact path:") {
		t.Errorf("path traversal: wrong diagnostic (want \"missing checksum for: container-inspect.json\"):\n%s", out)
	}
}

// TestArtifact_MalformedChecksumHash rejects an evidence bundle
// where checksums.txt contains a non-hex hash. ParseChecksumLine
// returns "invalid hash length" because the hash is not 64 hex chars.
func TestArtifact_MalformedChecksumHash(t *testing.T) {
	dst := t.TempDir()
	boundDir := copyBoundedFixture(t, dst)
	rebindFixture(t, boundDir)

	// Replace checksums.txt with a single non-hex line.
	checksumPath := filepath.Join(boundDir, "checksums.txt")
	bad := "not-a-hex-hash  manifest.json\n"
	if err := os.WriteFile(checksumPath, []byte(bad), 0644); err != nil {
		t.Fatalf("write bad checksums: %v", err)
	}

	out, err := runVerifier(t, dst)
	if err == nil {
		t.Fatalf("malformed checksum: verifier accepted; output:\n%s", out)
	}
	if !strings.Contains(out, "invalid checksum hash length:") {
		t.Errorf("malformed checksum: wrong diagnostic (want \"invalid hash length:\"):\n%s", out)
	}
}
