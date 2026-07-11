package scriptdoctrine

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestVerifierR11E2EMutations runs each R11.0-required
// end-to-end mutation through the same Verifier path used by
// the gate. Each mutation touches an actual file in the
// repository and the test asserts the resulting diagnostic count.
//
// The repository's bootstrap baseline is the gate's frozen
// metric, so any mutation-driven count change MUST surface as a
// baseline-python-invocation-changed diagnostic. Mutations that
// introduce a hard classification error MUST surface as an
// internal-error diagnostic. Mutations that introduce nothing new
// (e.g. a comment that already matches the policy) MUST NOT alter
// the count.

const (
	makefilePath    = "../../../Makefile"
	baselineRelPath = "../../../docs/tooling/script-doctrine-bootstrap-baseline.csv"
)

// baselineLoad loads the repository's bootstrap baseline from
// the standard location. The test cwd is the package directory
// (internal/tooling/scriptdoctrine), so 3 levels up reach the
// repo root.
func baselineLoad(t *testing.T) map[string]*BaselineEntry {
	t.Helper()
	b, err := LoadBaseline(baselineRelPath)
	if err != nil {
		t.Fatalf("load baseline: %v", err)
	}
	return b
}

// withModifiedFile runs fn after writing content to path. The
// original content is restored when the test (or sub-test)
// finishes.
func withModifiedFile(t *testing.T, path string, content []byte, fn func()) {
	t.Helper()
	orig, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	t.Cleanup(func() { _ = os.WriteFile(path, orig, 0644) })
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	fn()
}

// appendToFile writes original | suffix to path for the duration
// of the test.
func appendToFile(t *testing.T, path, suffix string) {
	t.Helper()
	orig, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	t.Cleanup(func() { _ = os.WriteFile(path, orig, 0644) })
	updated := append(orig, []byte(suffix)...)
	if err := os.WriteFile(path, updated, 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// baselineDiagnosticCount runs the Verifier against the repo and
// returns the number of baseline-python-invocation-changed
// diagnostics whose Path equals the given base file name.
func baselineDiagnosticCount(t *testing.T, fileName string) int {
	t.Helper()
	v := NewVerifier(repoRoot(t), true)
	v.SetBaseline(baselineLoad(t))
	diags := v.Verify()
	n := 0
	for _, d := range diags {
		if d.Check == "baseline-python-invocation-changed" && d.Path == fileName {
			n++
		}
	}
	return n
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("could not locate repo root from %s", dir)
	return ""
}

// TestR11Mutation1_MakeShellFunction: add `RESULT := $(shell python3 x.py)`
// to Makefile and check the count delta is exactly +1.
func TestR11Mutation1_MakeShellFunction(t *testing.T) {
	before := baselineDiagnosticCount(t, "Makefile")
	appendToFile(t, makefilePath, "\nRESULT := $(shell python3 x.py)\n")
	after := baselineDiagnosticCount(t, "Makefile")
	delta := after - before
	if delta != 1 {
		t.Errorf("Makefile $(shell) mutation produced %d baseline-python-invocation-changed diagnostics, want exactly 1", delta)
	}
}

// TestR11Mutation2_MakeShellAssignment: add `RESULT != python3 x.py`
// to Makefile and check the count delta is exactly +1.
func TestR11Mutation2_MakeShellAssignment(t *testing.T) {
	before := baselineDiagnosticCount(t, "Makefile")
	appendToFile(t, makefilePath, "\nRESULT != python3 x.py\n")
	after := baselineDiagnosticCount(t, "Makefile")
	delta := after - before
	if delta != 1 {
		t.Errorf("Makefile != assignment mutation produced %d baseline-python-invocation-changed diagnostics, want exactly 1", delta)
	}
}

// TestR11Mutation3_BashDashCDynamic: change a literal `bash -c`
// payload to a dynamic `"$COMMAND"` payload, and check that the
// surface produces an internal-error rather than a count change.
func TestR11Mutation3_BashDashCDynamic(t *testing.T) {
	tmpDir := t.TempDir()
	tmpScript := filepath.Join(tmpDir, "mutation3.sh")
	contentOrig := []byte("#!/bin/bash\nset -euo pipefail\nbash -c 'python3 x.py'\necho ok\n")
	if err := os.WriteFile(tmpScript, contentOrig, 0644); err != nil {
		t.Fatalf("write tmp script: %v", err)
	}
	v := NewVerifier(tmpDir, false)
	before := pythonInvocationCount(t, v.Verify(), "mutation3.sh")
	if before != 1 {
		t.Errorf("literal bash -c mutation produced %d python sites, want 1", before)
	}
	// Now mutate to dynamic payload.
	mutated := []byte("#!/bin/bash\nset -euo pipefail\nbash -c \"$COMMAND\"\necho ok\n")
	if err := os.WriteFile(tmpScript, mutated, 0644); err != nil {
		t.Fatalf("mutate: %v", err)
	}
	v = NewVerifier(tmpDir, false)
	internalDiags := 0
	for _, d := range v.Verify() {
		if d.Check == "internal-error" && d.Path == "mutation3.sh" {
			internalDiags++
		}
	}
	if internalDiags != 1 {
		t.Errorf("dynamic bash -c mutation produced %d internal-error diagnostics, want 1", internalDiags)
	}
}

// pythonInvocationCount sums the integer site counts in
// python-invocation diagnostics for paths containing want.
func pythonInvocationCount(t *testing.T, diags []Diagnostic, want string) int {
	t.Helper()
	total := 0
	for _, d := range diags {
		if d.Check != "python-invocation" {
			continue
		}
		if !strings.Contains(d.Path, want) {
			continue
		}
		total += parseSiteCount(d.Msg)
	}
	return total
}

// parseSiteCount extracts the integer count from a "(N site(s))"
// suffix produced by check_python.go's diagnostic writer. The
// substring "site(s)" contains a nested parenthesis, so the
// parser scans forward from the outer "(" and skips the leading
// digit run until whitespace.
var siteCountRx = regexp.MustCompile(`\((\d+) site[(]s[)]?\)`)

func parseSiteCount(msg string) int {
	m := siteCountRx.FindStringSubmatch(msg)
	if len(m) < 2 {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

// TestR11Mutation4_WorkflowDefaultPython writes a workflow with
// `defaults.run.shell: python` and two run steps. Check the
// invocation count delta is exactly +2 over the no-shell version.
func TestR11Mutation4_WorkflowDefaultPython(t *testing.T) {
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, ".github", "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatal(err)
	}
	workflowPath := filepath.Join(workflowsDir, "r11-mutation.yml")
	baseline := []byte("name: R11 Mutation\non: workflow_dispatch\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n")
	if err := os.WriteFile(workflowPath, baseline, 0644); err != nil {
		t.Fatal(err)
	}
	v := NewVerifier(tmpDir, false)
	before := 0
	for _, d := range v.Verify() {
		if d.Check == "python-invocation" {
			before += parseSiteCount(d.Msg)
		}
	}
	mutated := []byte("defaults:\n  run:\n    shell: python\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: print(\"one\")\n      - run: print(\"two\")\n")
	if err := os.WriteFile(workflowPath, mutated, 0644); err != nil {
		t.Fatal(err)
	}
	v = NewVerifier(tmpDir, false)
	after := 0
	for _, d := range v.Verify() {
		if d.Check == "python-invocation" {
			after += parseSiteCount(d.Msg)
		}
	}
	if after-before != 2 {
		t.Errorf("workflow default python mutation produced invocation delta %d, want 2 (before=%d after=%d)", after-before, before, after)
	}
}

// TestR11Mutation5_StepShellPython adds a step-level
// `shell: python -u {0}` to a workflow with a single run step.
// The delta must be exactly +1.
func TestR11Mutation5_StepShellPython(t *testing.T) {
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, ".github", "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatal(err)
	}
	workflowPath := filepath.Join(workflowsDir, "r11-mutation5.yml")
	baseline := []byte("jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n")
	if err := os.WriteFile(workflowPath, baseline, 0644); err != nil {
		t.Fatal(err)
	}
	v := NewVerifier(tmpDir, false)
	before := 0
	for _, d := range v.Verify() {
		if d.Check == "python-invocation" {
			before += parseSiteCount(d.Msg)
		}
	}
	mutated := []byte("jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: print(\"python\")\n        shell: python -u {0}\n")
	if err := os.WriteFile(workflowPath, mutated, 0644); err != nil {
		t.Fatal(err)
	}
	v = NewVerifier(tmpDir, false)
	after := 0
	for _, d := range v.Verify() {
		if d.Check == "python-invocation" {
			after += parseSiteCount(d.Msg)
		}
	}
	if after-before != 1 {
		t.Errorf("step shell python mutation produced invocation delta %d, want 1 (before=%d after=%d)", after-before, before, after)
	}
}

// TestR11Mutation6_CommentNoCountChange ensures that adding a
// comment containing each of the known patterns does not change
// the count.
func TestR11Mutation6_CommentNoCountChange(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "r11-comment.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\nset -euo pipefail\necho hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	v := NewVerifier(tmpDir, false)
	before := 0
	for _, d := range v.Verify() {
		if d.Check == "python-invocation" {
			before += parseSiteCount(d.Msg)
		}
	}
	mutated := []byte("#!/bin/bash\nset -euo pipefail\necho hello\n# Examples:\n# python3 x.py\n# $(shell python3 x.py)\n# bash -c 'python3 x.py'\n# shell: python\n# RESULT := $(shell python3 x.py)\n")
	if err := os.WriteFile(scriptPath, mutated, 0644); err != nil {
		t.Fatal(err)
	}
	v = NewVerifier(tmpDir, false)
	after := 0
	for _, d := range v.Verify() {
		if d.Check == "python-invocation" {
			after += parseSiteCount(d.Msg)
		}
	}
	if before != after {
		t.Errorf("comment-only mutation changed count: before=%d after=%d", before, after)
	}
}
