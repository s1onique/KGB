// bounded_fixture_test.go — Hermetic fixture copy + positive baseline.
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-BOUNDED-QUALIFICATION01-CORRECTION01
// §5.2: "Committed fixture authority. End-to-end tests must use the
// committed accepted fixture, not `.factory` scratch state."
// §5.3: "Valid positive baseline. Before applying any mutation: copy
// the committed fixture into t.TempDir(); preserve its original run
// ID and directory name; invoke the freshly built verifier; require
// exit code 0."
// §5.4: "Checksum-aware mutation harness. Do not rewrite
// `manifest.run_id` merely to create a temporary fixture. Copy the
// fixture as: <t.TempDir()>/<original-run-id>/ and continue passing
// the original run ID."

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence"
)

// boundedFixtureDir is the committed test data fixture used by every
// bounded negative test. Missing this directory is a hard test
// failure (not a skip) per ACT §5.2.
const boundedFixtureDir = "testdata/bounded-valid"

// boundedFixtureRunID is the run_id recorded inside the committed
// fixture's manifest.json. All tests copy the fixture into a temp
// dir with this exact name and pass this exact run_id to the verifier.
const boundedFixtureRunID = "lab-canary-bounded-1784624046"

// requireBoundedFixture fails the test if the committed fixture is
// absent. Per ACT §5.2 a missing fixture must be a test failure.
func requireBoundedFixture(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(boundedFixtureDir)
	if err != nil {
		t.Fatalf("absolute bounded fixture path: %v", err)
	}
	manifest := filepath.Join(abs, "manifest.json")
	if _, err := os.Stat(manifest); err != nil {
		t.Fatalf("committed bounded fixture missing at %s: %v "+
			"(ACT requires a committed fixture; no .factory fallback)",
			abs, err)
	}
	return abs
}

// rebindFixture rewrites the controller_executable_sha256,
// git_commit, and git_tree in the fixture's manifest.json so the
// live-runtime binding verifies, and recomputes checksums.txt so the
// inventory stays consistent. The rebind is in place — the original
// fixture on disk is left untouched for reproducibility.
//
// boundDir must be a copy of the bounded fixture, created by
// copyBoundedFixture. Returns the absolute path of the rebind
// manifest.
func rebindFixture(t *testing.T, boundDir string) string {
	t.Helper()
	manifestPath := filepath.Join(boundDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest for rebind: %v", err)
	}
	var manifest evidence.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse manifest for rebind: %v", err)
	}

	// Determine HEAD commit and tree from the same checkout.
	headCommit, err := runGitForTest("rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	headTree, err := runGitForTest("rev-parse", "HEAD^{tree}")
	if err != nil {
		t.Fatalf("git rev-parse HEAD^{tree}: %v", err)
	}

	manifest.SubjectIdentity.GitCommit = headCommit
	manifest.SubjectIdentity.GitTree = headTree
	manifest.SubjectIdentity.GitObjectFormat = "sha1"
	manifest.SubjectIdentity.ControllerExecutableSHA256 = verifierSHA()
	manifest.SubjectIdentity.ControllerExecutablePath = verifierPath()

	// Re-marshal with LF endings (json.Encoder appends a trailing
	// newline automatically).
	rewritten, err := json.MarshalIndent(&manifest, "", "  ")
	if err != nil {
		t.Fatalf("remarshal manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, append(rewritten, '\n'), 0644); err != nil {
		t.Fatalf("rewrite manifest: %v", err)
	}

	// Recompute checksums.txt so the inventory still matches.
	inventory := manifest.ArtifactInventory
	if err := writeChecksumsForInventory(boundDir, inventory); err != nil {
		t.Fatalf("rewrite checksums: %v", err)
	}
	return manifestPath
}

// copyBoundedFixture copies the committed fixture into dst/<run_id>/
// preserving the run_id. The copy is a plain cp: every file in the
// source fixture is written byte-for-byte to the destination. The
// caller is responsible for rebinding controller_executable_sha256
// and the manifest's git identity via rebindFixture.
//
// Per ACT §5.4 the run_id is preserved (not rewritten to a temp
// name) so that the inventory and checksums remain coherent.
func copyBoundedFixture(t *testing.T, dst string) string {
	t.Helper()
	src := requireBoundedFixture(t)
	dstRun := filepath.Join(dst, boundedFixtureRunID)
	if err := os.MkdirAll(dstRun, 0755); err != nil {
		t.Fatalf("mkdir copy dst: %v", err)
	}
	if err := copyDirContents(src, dstRun); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return dstRun
}

// copyDirContents is a non-recursive plain-file copy used by
// copyBoundedFixture. The bounded fixture is flat (no subdirs).
func copyDirContents(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			return fmt.Errorf("subdir %s in bounded fixture: flat layout required", e.Name())
		}
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), data, 0644); err != nil {
			return err
		}
	}
	return nil
}

// writeChecksumsForInventory recomputes checksums.txt from the
// declared inventory, matching the production writer's behaviour.
func writeChecksumsForInventory(boundDir string, inventory []string) error {
	// Build the deterministic sorted list, skipping checksums.txt
	// itself (per the production convention).
	type entry struct {
		path   string
		sha256 string
	}
	entries := make([]entry, 0, len(inventory))
	for _, name := range inventory {
		if name == "checksums.txt" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(boundDir, name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		sum := sha256.Sum256(data)
		entries = append(entries, entry{name, hex.EncodeToString(sum[:])})
	}
	// Sort by path for determinism.
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j-1].path > entries[j].path; j-- {
			entries[j-1], entries[j] = entries[j], entries[j-1]
		}
	}
	var buf bytes.Buffer
	for _, e := range entries {
		fmt.Fprintf(&buf, "%s  %s\n", e.sha256, e.path)
	}
	return os.WriteFile(filepath.Join(boundDir, "checksums.txt"), buf.Bytes(), 0644)
}

// recomputeChecksumsFor is used by mutation tests that change the
// content of a single artifact (e.g. workload, final-state) and need
// checksums.txt to stay consistent so the verifier's inventory check
// passes and the targeted mutation assertion can fire. It calls
// writeChecksumsForInventory with the canonical inventory read from
// the fixture's manifest.
func recomputeChecksumsFor(t *testing.T, boundDir string) {
	t.Helper()
	manifest, err := readManifest(t, boundDir)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if err := writeChecksumsForInventory(boundDir, manifest.ArtifactInventory); err != nil {
		t.Fatalf("recompute checksums: %v", err)
	}
}

// readManifest parses the manifest.json of a bound fixture copy.
func readManifest(t *testing.T, boundDir string) (*evidence.Manifest, error) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(boundDir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m evidence.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// runVerifier invokes the freshly built verifier as a subprocess
// against the bound fixture copy. It returns the combined output and
// exit error; tests assert the specific error path or exit code.
func runVerifier(t *testing.T, artifactsDir string) (string, error) {
	t.Helper()
	cmd := exec.Command(verifierPath(), "verify",
		"--artifacts-dir", artifactsDir,
		"--run-id", boundedFixtureRunID)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	// Combine stdout+stderr so diagnostic assertions can match
	// either stream; the production verifier writes diagnostics to
	// stdout via fmt.Printf.
	return stdout.String() + stderr.String(), err
}

// runGitForTest returns trimmed output of a `git` invocation
// rooted at the repository (production code uses the same helper
// from the parent package; this is a local copy so the test
// infrastructure does not depend on the production path).
func runGitForTest(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRootForTest()
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// repoRootForTest returns the absolute path of the repo root by
// walking upward looking for AGENTS.md (the existing repoRoot uses
// a similar heuristic).
func repoRootForTest() string {
	cwd, err := os.Getwd()
	if err != nil {
		return cwd
	}
	dir := cwd
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return cwd
}

// fixtureMustExist asserts the committed fixture directory contains
// the 10 canonical artifacts. Used by the positive baseline test.
func fixtureMustExist(t *testing.T, dir string) {
	t.Helper()
	canonical := []string{
		"manifest.json",
		"verdict.json",
		"samples.csv",
		"events.jsonl",
		"container-inspect.json",
		"container-logs.txt",
		"initial-canary-state.json",
		"final-canary-state.json",
		"workload-result.json",
		"checksums.txt",
	}
	for _, name := range canonical {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("canonical artifact %s missing from committed fixture: %v", name, err)
		}
	}
}

// TestBoundedPositiveBaseline_CopiedFixtureVerifies copies the
// committed fixture into t.TempDir(), rebinds the live-inode fields
// to the freshly built verifier, and requires exit code 0. This is
// the positive control required by ACT §5.3.
func TestBoundedPositiveBaseline_CopiedFixtureVerifies(t *testing.T) {
	// Sanity: the committed fixture must exist (this is a hard
	// failure, not a skip).
	src := requireBoundedFixture(t)
	fixtureMustExist(t, src)

	dst := t.TempDir()
	boundDir := copyBoundedFixture(t, dst)
	rebindFixture(t, boundDir)

	out, err := runVerifier(t, dst)
	if err != nil {
		t.Fatalf("positive baseline: verifier rejected copied fixture:\n%s", out)
	}
	// Sanity: the verifier output reports stable classification.
	if !strings.Contains(out, "Overall: stable") {
		t.Errorf("positive baseline: expected Overall: stable in verifier output, got:\n%s", out)
	}
	if !strings.Contains(out, "ScenarioValid: true") {
		t.Errorf("positive baseline: expected ScenarioValid: true, got:\n%s", out)
	}
	if !strings.Contains(out, "CanariesValid: true") {
		t.Errorf("positive baseline: expected CanariesValid: true, got:\n%s", out)
	}
}

// TestBoundedPositiveBaseline_InventoryVerifies asserts that the
// committed fixture's checksums.txt matches the actual SHA-256 of
// each canonical artifact. This is the inventory-check control
// required by the "harness controls" section of the mandatory test
// matrix.
func TestBoundedPositiveBaseline_InventoryVerifies(t *testing.T) {
	src := requireBoundedFixture(t)
	fixtureMustExist(t, src)

	// The committed fixture's checksums.txt was produced from a
	// different verifier build (Go build embeds timestamp), so the
	// controller_executable_sha256 will not match the live verifier.
	// We only verify that every other artifact's checksum in the
	// committed file is correct. The rebind path in
	// TestBoundedPositiveBaseline_CopiedFixtureVerifies covers the
	// exe-SHA binding.
	checksumPath := filepath.Join(src, "checksums.txt")
	raw, err := os.ReadFile(checksumPath)
	if err != nil {
		t.Fatalf("read checksums.txt: %v", err)
	}
	lines := strings.Split(string(raw), "\n")
	checked := 0
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			t.Fatalf("malformed checksums.txt line: %q", line)
		}
		stored := strings.TrimSpace(parts[0])
		path := strings.TrimSpace(parts[1])
		if path == "checksums.txt" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		sum := sha256.Sum256(data)
		got := hex.EncodeToString(sum[:])
		if got != stored {
			t.Errorf("checksum mismatch for %s: stored=%s got=%s", path, stored, got)
		}
		checked++
	}
	if checked < 9 {
		t.Errorf("expected at least 9 artifact checksums (excluding checksums.txt), found %d", checked)
	}
	// Reference fs to keep the import set tight; the package only
	// uses fs in this file's test helpers via copyDirContents.
}
