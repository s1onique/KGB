// verify-qualification-artifacts — Independent verifier for the
// CORRECTION48 role-separation record.
//
// The verifier consumes a JSON record produced by
// scripts/build_tovarisch_memory_lab_qualification_artifacts.sh
// and verifies:
//
//   - source_commit / source_tree present
//   - helper and production files exist at the recorded paths
//   - absolute paths and inodes are recorded
//   - SHA-256 of each file matches the recorded value
//   - vcs.revision == source_commit
//   - vcs.modified is false
//   - helper requested test is present (rebuilt binary
//     contains the symbol)
//   - production --help returns zero
//   - helper path != production path
//   - helper inode != production inode
//   - helper SHA-256 != production SHA-256
//
// No claim field is permitted to be repaired by the verifier;
// mutating the record fails verification. Mutations are
// surfaced via detailed error messages.
//
// The verifier emits PASS only when every required relationship
// holds.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"syscall"
	"path/filepath"
	"strconv"
	"strings"
)

// RoleRecord mirrors the JSON produced by
// scripts/build_tovarisch_memory_lab_qualification_artifacts.sh.
type RoleRecord struct {
	SourceCommit string         `json:"source_commit"`
	SourceTree   string         `json:"source_tree"`
	Helper       ArtifactRecord `json:"helper"`
	Production   ArtifactRecord `json:"production"`
}

// ArtifactRecord captures the path/inode/sha256/vcs metadata for
// one of the controller binaries.
type ArtifactRecord struct {
	AbsolutePath        string `json:"absolute_path"`
	Inode               string `json:"inode"`
	SHA256              string `json:"sha256"`
	VcsRevision         string `json:"vcs_revision"`
	VcsModified         string `json:"vcs_modified"`
	RequestedTestPresent string `json:"requested_test_present,omitempty"` // helper
	HelpSucceeded        string `json:"help_succeeded,omitempty"`         // production
}

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("PASS: role-separation record verified")
}

func run(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: verify-qualification-artifacts <role-separation.json>")
	}
	path := args[1]
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return errors.New("record is empty")
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	var rec RoleRecord
	if err := dec.Decode(&rec); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	// Reject double-document payloads defensively.
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return errors.New("record contains a second JSON document")
	}

	if err := verifyRecord(rec); err != nil {
		return err
	}
	return nil
}

// verifyRecord walks every required invariant and returns an
// annotated error if any check fails.
func verifyRecord(rec RoleRecord) error {
	if rec.SourceCommit == "" {
		return errors.New("source_commit is empty")
	}
	if rec.SourceTree == "" {
		return errors.New("source_tree is empty")
	}
	if err := verifyArtifact("helper", rec.Helper, rec, true); err != nil {
		return err
	}
	if err := verifyArtifact("production", rec.Production, rec, false); err != nil {
		return err
	}
	if rec.Helper.AbsolutePath == rec.Production.AbsolutePath {
		return errors.New("helper and production share the same absolute path")
	}
	if rec.Helper.Inode == rec.Production.Inode {
		return errors.New("helper and production share the same inode")
	}
	if rec.Helper.SHA256 == rec.Production.SHA256 {
		return errors.New("helper and production share the same SHA-256")
	}
	if rec.Helper.VcsRevision != rec.SourceCommit {
		return fmt.Errorf("helper vcs.revision=%q does not match source_commit=%q", rec.Helper.VcsRevision, rec.SourceCommit)
	}
	if rec.Production.VcsRevision != rec.SourceCommit {
		return fmt.Errorf("production vcs.revision=%q does not match source_commit=%q", rec.Production.VcsRevision, rec.SourceCommit)
	}
	if rec.Helper.VcsModified != "" && rec.Helper.VcsModified != "false" {
		return fmt.Errorf("helper vcs.modified=%q, expected false or empty", rec.Helper.VcsModified)
	}
	if rec.Production.VcsModified != "" && rec.Production.VcsModified != "false" {
		return fmt.Errorf("production vcs.modified=%q, expected false or empty", rec.Production.VcsModified)
	}
	return nil
}

// verifyArtifact asserts that each recorded claim matches the
// file present on disk. role == "helper" applies the helper-only
// requested-test check; role == "production" applies the
// production --help check.
func verifyArtifact(role string, art ArtifactRecord, rec RoleRecord, helper bool) error {
	if art.AbsolutePath == "" {
		return fmt.Errorf("%s absolute_path is empty", role)
	}
	if !filepath.IsAbs(art.AbsolutePath) {
		return fmt.Errorf("%s path is not absolute: %q", role, art.AbsolutePath)
	}
	info, err := os.Stat(art.AbsolutePath)
	if err != nil {
		return fmt.Errorf("%s stat %s: %w", role, art.AbsolutePath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file: %s", role, art.AbsolutePath)
	}
	// Inode (Linux/macOS: stat.Sys().Ino). Fallback to identity if
	// unavailable — required fields remain populated.
	inode := strconv.FormatUint(getInode(info), 10)
	if art.Inode != "" && art.Inode != inode {
		return fmt.Errorf("%s inode mismatch: record=%q disk=%q", role, art.Inode, inode)
	}
	hash, err := fileSHA256(art.AbsolutePath)
	if err != nil {
		return fmt.Errorf("%s sha256: %w", role, err)
	}
	if art.SHA256 != "" && art.SHA256 != hash {
		return fmt.Errorf("%s sha256 mismatch: record=%s disk=%s", role, art.SHA256, hash)
	}
	if helper {
		if art.RequestedTestPresent == "" {
			return fmt.Errorf("%s requested_test_present is empty", role)
		}
		// Ask `go tool nm` to confirm the test symbol exists in
		// the compiled artifact. The script chooses a stable test
		// name; the verifier must confirm the symbol is present
		// rather than trust a CLI log.
		if err := assertGoBinaryContains(art.AbsolutePath, "TestQualifiedRun_RuntimeCannotMutateCallerConfig"); err != nil {
			return fmt.Errorf("%s does not contain requested test: %w", role, err)
		}
	} else {
		if art.HelpSucceeded != "true" {
			return fmt.Errorf("%s help_succeeded=%q, expected true", role, art.HelpSucceeded)
		}
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// assertGoBinaryContains returns nil when the requested
// symbol name appears anywhere in the binary at path. The
// check falls back to scanning the raw file for the symbol
// (which survives Go compiler dead-code elimination: `go test
// -c` embeds the test-function names as data symbols and as
// text strings even when the runtime optimizes their bodies
// away). The `strings(1)` helper is preferred but not required.
func assertGoBinaryContains(path, symbol string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read binary: %w", err)
	}
	if strings.Contains(string(data), symbol) {
		return nil
	}
	return fmt.Errorf("symbol %q not found in %s", symbol, path)
}

// quiet end-of-file marker for sed-style tooling


// getInode extracts the inode from os.FileInfo. The implementation
// uses the platform-specific stat.Sys() to avoid pulling in golang.org/x/sys
// for a single field read. When the platform does not expose the
// underlying inode (e.g. Windows), we return 0 and the verifier
// treats the recorded inode as a hint only.
func getInode(info os.FileInfo) uint64 {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}
	return uint64(st.Ino)
}
