// shared_fixture_helpers_test.go — Scenario-agnostic fixture helpers.
//
// These helpers are shared by the bounded and descriptor ACT
// hermetic-test suites. The bounded suite continues to call its
// own wrapper functions for backward compatibility; the descriptor
// suite uses these generalized helpers directly.
//
// Required behavior:
//   - Missing fixture fails the test (never skips).
//   - Fixture is copied into dst/<runID>/ with the original run_id
//     preserved (no rewrite).
//   - The verifier is the freshly built per-test binary; .factory
//     state is never consulted.
//   - Mutations to the copy recompute checksums so the targeted
//     assertion (not the checksum validator) fires.

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireScenarioFixture asserts the committed fixture directory
// contains a manifest.json. A missing fixture is a hard test
// failure, never a skip, per ACT §5.2 (generalized to all
// scenarios).
func requireScenarioFixture(t *testing.T, srcDir string) string {
	t.Helper()
	abs, err := filepath.Abs(srcDir)
	if err != nil {
		t.Fatalf("absolute fixture path %s: %v", srcDir, err)
	}
	manifest := filepath.Join(abs, "manifest.json")
	if _, err := os.Stat(manifest); err != nil {
		t.Fatalf("committed fixture missing at %s: %v "+
			"(ACT requires a committed fixture; no .factory fallback)",
			abs, err)
	}
	return abs
}

// copyFixture copies the committed fixture at srcDir into
// dst/<runID>/, preserving the run_id byte-for-byte. The caller is
// responsible for rebinding controller_executable_sha256 and the
// manifest's git identity via rebindFixture.
func copyFixture(t *testing.T, dst, srcDir, runID string) string {
	t.Helper()
	src := requireScenarioFixture(t, srcDir)
	dstRun := filepath.Join(dst, runID)
	if err := os.MkdirAll(dstRun, 0755); err != nil {
		t.Fatalf("mkdir copy dst: %v", err)
	}
	if err := copyDirContents(src, dstRun); err != nil {
		t.Fatalf("copy fixture %s: %v", src, err)
	}
	return dstRun
}

// runVerifierForRunID invokes the freshly built verifier against
// the scenario-agnostic fixture copy. It returns the combined
// stdout+stderr output and the exit error; tests assert the
// specific error path or exit code.
func runVerifierForRunID(t *testing.T, artifactsDir, runID string) (string, error) {
	t.Helper()
	cmd := exec.Command(verifierPath(), "verify",
		"--artifacts-dir", artifactsDir,
		"--run-id", runID)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String() + stderr.String(), err
}

// mutateAndVerifyForFixture binds a fresh fixture copy, applies the
// mutation, recomputes checksums, runs the verifier, and asserts
// the verifier exits non-zero with the expected diagnostic substring
// in its output. The substring is the canonical production error
// path for the targeted mutation class.
func mutateAndVerifyForFixture(
	t *testing.T,
	srcDir, runID string,
	mutate func(boundDir string),
	expectDiagnostic string,
) {
	t.Helper()
	dst := t.TempDir()
	boundDir := copyFixture(t, dst, srcDir, runID)
	rebindFixture(t, boundDir)
	mutate(boundDir)
	recomputeChecksumsFor(t, boundDir)

	out, err := runVerifierForRunID(t, dst, runID)
	if err == nil {
		t.Fatalf("mutation did not cause verifier failure; output:\n%s", out)
	}
	if !strings.Contains(out, expectDiagnostic) {
		t.Errorf("mutation produced wrong diagnostic.\n"+
			"expected substring: %q\nfull output:\n%s", expectDiagnostic, out)
	}
}

// scenarioFixtureFilesExist asserts the committed fixture
// directory contains the 10 canonical artifacts. Used by positive
// baseline tests across all scenarios.
func scenarioFixtureFilesExist(t *testing.T, dir string) {
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
			t.Fatalf("canonical artifact %q missing from committed fixture %s: %v", name, dir, err)
		}
	}
}

// formatDiag is a tiny helper for building expected-diagnostic
// strings in a consistent style.
func formatDiag(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}
