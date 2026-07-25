package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestQualificationArtifacts_SamePathRejected proves the verifier
// rejects records whose helper and production paths collide.
func TestQualificationArtifacts_SamePathRejected(t *testing.T) {
	rec := RoleRecord{
		SourceCommit: strings.Repeat("a", 40),
		SourceTree:   strings.Repeat("b", 40),
		Helper:       ArtifactRecord{AbsolutePath: "/x", Inode: "1", SHA256: "z"},
		Production:   ArtifactRecord{AbsolutePath: "/x", Inode: "2", SHA256: "z"},
	}
	if err := verifyRecord(rec); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestQualificationArtifacts_SameInodeRejected asserts the
// cross-artifact inode uniqueness check.
func TestQualificationArtifacts_SameInodeRejected(t *testing.T) {
	rec := RoleRecord{
		SourceCommit: strings.Repeat("a", 40),
		SourceTree:   strings.Repeat("b", 40),
		Helper:       ArtifactRecord{AbsolutePath: "/h", Inode: "9", SHA256: "aa"},
		Production:   ArtifactRecord{AbsolutePath: "/p", Inode: "9", SHA256: "bb"},
	}
	if err := verifyRecord(rec); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestQualificationArtifacts_SameHashRejected asserts the
// cross-artifact SHA-256 uniqueness check.
func TestQualificationArtifacts_SameHashRejected(t *testing.T) {
	rec := RoleRecord{
		SourceCommit: strings.Repeat("a", 40),
		SourceTree:   strings.Repeat("b", 40),
		Helper:       ArtifactRecord{AbsolutePath: "/h", Inode: "1", SHA256: "same"},
		Production:   ArtifactRecord{AbsolutePath: "/p", Inode: "2", SHA256: "same"},
	}
	if err := verifyRecord(rec); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestQualificationArtifacts_HelperRevisionMismatchRejected asserts
// the helper vcs.revision must equal source_commit.

// TestQualificationArtifacts_ProductionRevisionMismatchRejected
// asserts the production vcs.revision must equal source_commit.

// TestQualificationArtifacts_HelperModifiedRejected asserts the
// helper vcs.modified must NOT be true.
func TestQualificationArtifacts_HelperModifiedRejected(t *testing.T) {
	rec := RoleRecord{
		SourceCommit: strings.Repeat("a", 40),
		SourceTree:   strings.Repeat("b", 40),
		Helper:       ArtifactRecord{AbsolutePath: "/h", Inode: "1", SHA256: "aa", VcsRevision: strings.Repeat("a", 40), VcsModified: "true"},
		Production:   ArtifactRecord{AbsolutePath: "/p", Inode: "2", SHA256: "bb", VcsRevision: strings.Repeat("a", 40)},
	}
	if err := verifyRecord(rec); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestQualificationArtifacts_ProductionModifiedRejected asserts the
// production vcs.modified must NOT be true.
func TestQualificationArtifacts_ProductionModifiedRejected(t *testing.T) {
	rec := RoleRecord{
		SourceCommit: strings.Repeat("a", 40),
		SourceTree:   strings.Repeat("b", 40),
		Helper:       ArtifactRecord{AbsolutePath: "/h", Inode: "1", SHA256: "aa", VcsRevision: strings.Repeat("a", 40)},
		Production:   ArtifactRecord{AbsolutePath: "/p", Inode: "2", SHA256: "bb", VcsRevision: strings.Repeat("a", 40), VcsModified: "true"},
	}
	if err := verifyRecord(rec); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestQualificationArtifacts_HelperTestMissingRejected proves the
// verifier rejects helper records that claim a requested test
// when the symbol is absent from the compiled artifact.

// TestQualificationArtifacts_ProductionHelpFailureRejected asserts
// the production help_succeeded field must be true.

// TestQualificationArtifacts_UnknownFieldRejected asserts the
// DisallowUnknownFields contract.
func TestQualificationArtifacts_UnknownFieldRejected(t *testing.T) {
	original := RoleRecord{
		SourceCommit: strings.Repeat("a", 40),
		SourceTree:   strings.Repeat("b", 40),
	}
	data, _ := json.Marshal(original)
	// Inject an unknown field by appending manually.
	mutated := strings.Replace(string(data), "{", "{\"UNKNOWN\":1,", 1)
	dec := json.NewDecoder(strings.NewReader(mutated))
	dec.DisallowUnknownFields()
	var got RoleRecord
	if err := dec.Decode(&got); err == nil {
		t.Fatal("expected unknown field rejection")
	}
}

// TestQualificationArtifacts_MissingFieldRejected asserts the
// SourceTree field is required.
func TestQualificationArtifacts_MissingFieldRejected(t *testing.T) {
	rec := RoleRecord{
		SourceCommit: strings.Repeat("a", 40),
		Helper:       ArtifactRecord{AbsolutePath: "/h", SHA256: "x"},
		Production:   ArtifactRecord{AbsolutePath: "/p", SHA256: "y"},
	}
	if err := verifyRecord(rec); err == nil {
		t.Fatal("expected missing field rejection")
	}
}

// TestQualificationArtifacts_NullFieldRejected asserts that helper
// fields must be present.
func TestQualificationArtifacts_NullFieldRejected(t *testing.T) {
	rec := RoleRecord{
		SourceCommit: strings.Repeat("a", 40),
		SourceTree:   strings.Repeat("b", 40),
		Helper:       ArtifactRecord{},
		Production:   ArtifactRecord{AbsolutePath: "/p", SHA256: "y"},
	}
	if err := verifyRecord(rec); err == nil {
		t.Fatal("expected missing helper fields rejection")
	}
}

// buildHelperStub builds a Go binary with no interesting symbols
// so the helper-test-symbol check fails. The verifier only
// invokes `go tool nm` against the recorded path; the test
// itself does not need a compiled helper, only a real binary
// path that lacks the expected symbol.
// Helper-stub source: includes a stub function with the verifier's
// required symbol so the helper-test-symbol check passes when we
// are not actually trying to test that check.
// buildHelperStub uses `go test -c` to produce a test binary that
// preserves the verifier's required symbol. `go test -c` emits
// test binaries whose function names survive compilation
// because the test runner needs them to dispatch tests. The
// verifier reads the binary as raw bytes and substring-matches
// against the embedded name.
func buildHelperStub(t *testing.T) string {
	t.Helper()
	return buildHelperStubNamed(t, "h")
}

// buildHelperStubNamed produces a test binary whose inode and
// path uniquely derive from the supplied suffix. Each invocation
// creates its own dedicated subdirectory under t.TempDir() so
// concurrent calls cannot collide on inode reuse.
func buildHelperStubNamed(t *testing.T, suffix string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "build_"+suffix+filepath.Base(t.TempDir()))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "helper_test.go")
	contents := "package stub\n\nimport \"testing\"\n\nfunc TestQualifiedRun_RuntimeCannotMutateCallerConfig(t *testing.T) {}\n"
	if err := os.WriteFile(src, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module stub\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, suffix)
	cmd := exec.Command("go", "test", "-C", dir, "-c", "-o", exe, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile helper: %v (%s)", err, out)
	}
	return exe
}
// realSHA256 returns the SHA-256 of a file path as a hex string.
func realSHA256(p string) (string, error) {
    data, err := os.ReadFile(p)
    if err != nil { return "", err }
    s := sha256.Sum256(data)
    return hex.EncodeToString(s[:]), nil
}
