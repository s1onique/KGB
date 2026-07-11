package scriptdoctrine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// SortDiagnostics tests
// =============================================================================

func TestSortDiagnostics(t *testing.T) {
	diags := []Diagnostic{
		{Check: "python-file", Path: "b.py", Msg: "b"},
		{Check: "shell-loc", Path: "a.sh", Msg: "a"},
		{Check: "python-file", Path: "a.py", Msg: "a"},
		{Check: "shell-loc", Path: "b.sh", Msg: "b"},
	}

	SortDiagnostics(diags)

	expected := []string{
		"python-file:a.py",
		"python-file:b.py",
		"shell-loc:a.sh",
		"shell-loc:b.sh",
	}

	for i, d := range diags {
		got := d.Check + ":" + d.Path
		if got != expected[i] {
			t.Errorf("diags[%d] = %s, want %s", i, got, expected[i])
		}
	}
}

// =============================================================================
// Verifier.Verify tests (count-based end-to-end fixtures)
// =============================================================================

func TestVerifierBootstrapBaselinePassesAtFrozenValues(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := "scripts/example.sh"
	scriptFull := filepath.Join(tmpDir, scriptPath)
	if err := os.MkdirAll(filepath.Dir(scriptFull), 0755); err != nil {
		t.Fatal(err)
	}
	scriptContent := "#!/bin/bash\nset -euo pipefail\npython3 tool.py\necho done\n"
	if err := os.WriteFile(scriptFull, []byte(scriptContent), 0644); err != nil {
		t.Fatal(err)
	}

	verifier := NewVerifier(tmpDir, true)
	verifier.SetInventory(map[string]*InventoryEntry{
		scriptPath: {
			ID: "S001", Path: scriptPath, Language: "shell",
			Role: "verifier", Status: "migration-required",
		},
	})
	verifier.SetBaseline(map[string]*BaselineEntry{
		scriptPath: {
			Path:                  scriptPath,
			BaselineLOC:           0,
			PythonInvocationCount: 1,
		},
	})

	diags := verifier.Verify()
	for _, d := range diags {
		if !strings.HasPrefix(d.Check, "internal-error") {
			t.Errorf("unexpected non-internal diagnostic: %+v", d)
		}
	}
}

func TestVerifierBootstrapBaselineDetectsCountChange(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := "scripts/example.sh"
	scriptFull := filepath.Join(tmpDir, scriptPath)
	if err := os.MkdirAll(filepath.Dir(scriptFull), 0755); err != nil {
		t.Fatal(err)
	}
	scriptContent := "#!/bin/bash\nset -euo pipefail\npython3 tool.py\npython3 another.py\necho done\n"
	if err := os.WriteFile(scriptFull, []byte(scriptContent), 0644); err != nil {
		t.Fatal(err)
	}

	verifier := NewVerifier(tmpDir, true)
	verifier.SetInventory(map[string]*InventoryEntry{
		scriptPath: {
			ID: "S001", Path: scriptPath, Language: "shell",
			Role: "verifier", Status: "migration-required",
		},
	})
	verifier.SetBaseline(map[string]*BaselineEntry{
		scriptPath: {
			Path:                  scriptPath,
			BaselineLOC:           0,
			PythonInvocationCount: 1,
		},
	})

	diags := verifier.Verify()
	count := 0
	for _, d := range diags {
		if d.Check == "baseline-python-invocation-changed" && d.Path == scriptPath {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 baseline-python-invocation-changed diagnostic, got %d (all: %v)", count, diags)
	}
}

func TestVerifierDetectsStaleBaselinePath(t *testing.T) {
	tmpDir := t.TempDir()
	verifier := NewVerifier(tmpDir, true)
	verifier.SetBaseline(map[string]*BaselineEntry{
		"scripts/ghost.sh": {
			Path:                  "scripts/ghost.sh",
			BaselineLOC:           10,
			PythonInvocationCount: 0,
		},
	})

	diags := verifier.Verify()
	found := false
	for _, d := range diags {
		if d.Check == "stale-bootstrap-baseline" && d.Path == "scripts/ghost.sh" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected stale-bootstrap-baseline diagnostic, got: %v", diags)
	}
}

// TestVerifierFailsClosedOnReadError uses a directory at the baseline
// path so that stat succeeds but reading fails with a non-IsNotExist
// error. The verifier must emit exactly one internal-error diagnostic
// for that path - not stale-bootstrap-baseline (which is for IsNotExist)
// and not "no diagnostic".
func TestVerifierFailsClosedOnReadError(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a directory at the path the baseline references. Stat
	// will report it as a directory (mode.IsRegular() == false), so the
	// verifier emits an internal-error - not a stale-baseline.
	badPath := "scripts/bad.sh"
	badFull := filepath.Join(tmpDir, badPath)
	if err := os.MkdirAll(badFull, 0755); err != nil {
		t.Fatal(err)
	}

	verifier := NewVerifier(tmpDir, true)
	verifier.SetBaseline(map[string]*BaselineEntry{
		badPath: {
			Path:                  badPath,
			BaselineLOC:           1,
			PythonInvocationCount: 0,
		},
	})

	diags := verifier.Verify()
	sawInternal := false
	for _, d := range diags {
		if d.Check == "internal-error" && d.Path == badPath {
			sawInternal = true
		}
	}
	if !sawInternal {
		t.Errorf("expected exactly one internal-error diagnostic for %s, got: %v", badPath, diags)
	}
}

func TestVerifierDetectsExtensionlessPythonShebang(t *testing.T) {
	tmpDir := t.TempDir()
	shebangPath := "scripts/legacy_tool"
	shebangFull := filepath.Join(tmpDir, shebangPath)
	if err := os.MkdirAll(filepath.Dir(shebangFull), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shebangFull, []byte("#!/usr/bin/env python3\nprint('legacy tool')\n"), 0644); err != nil {
		t.Fatal(err)
	}

	verifier := NewVerifier(tmpDir, false)

	diags := verifier.checkPythonShebangs()
	found := false
	for _, d := range diags {
		if d.Check == "python-shebang" && d.Path == shebangPath {
			found = true
		}
	}
	if !found {
		t.Errorf("expected python-shebang diagnostic for non-executable file, got: %v", diags)
	}
}

// =============================================================================
// R6 mutation tests: one added invocation = exactly one violation
// =============================================================================

func TestMutationExactlyOneAddedInvocation(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := "scripts/mut.sh"
	scriptFull := filepath.Join(tmpDir, scriptPath)
	if err := os.MkdirAll(filepath.Dir(scriptFull), 0755); err != nil {
		t.Fatal(err)
	}

	initial := "#!/bin/bash\nset -euo pipefail\npython3 tool.py\necho done\n"
	if err := os.WriteFile(scriptFull, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	verifier := NewVerifier(tmpDir, true)
	verifier.SetInventory(map[string]*InventoryEntry{
		scriptPath: {
			ID: "S001", Path: scriptPath, Language: "shell",
			Role: "verifier", Status: "migration-required",
		},
	})
	verifier.SetBaseline(map[string]*BaselineEntry{
		scriptPath: {
			Path:                  scriptPath,
			BaselineLOC:           0,
			PythonInvocationCount: 1,
		},
	})

	mutated := "#!/bin/bash\nset -euo pipefail\npython3 tool.py\npython3 another.py\necho done\n"
	if err := os.WriteFile(scriptFull, []byte(mutated), 0644); err != nil {
		t.Fatal(err)
	}

	diags := verifier.checkBaselineEnforcement()
	count := 0
	for _, d := range diags {
		if d.Check == "baseline-python-invocation-changed" && d.Path == scriptPath {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one baseline-python-invocation-changed diagnostic after mutation, got %d (all: %v)", count, diags)
	}

	for _, d := range diags {
		if d.Check == "baseline-python-invocation-changed" && d.Path == scriptPath {
			if !strings.Contains(d.Msg, "baseline=1") || !strings.Contains(d.Msg, "current=2") {
				t.Errorf("diagnostic msg lacks expected numbers: %q", d.Msg)
			}
		}
	}
}

// TestMutationSemicolonAddedInvocation is the R6 regression case: a
// second invocation joined with `;` must register as a second site.
func TestMutationSemicolonAddedInvocation(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := "scripts/mut_semi.sh"
	scriptFull := filepath.Join(tmpDir, scriptPath)
	if err := os.MkdirAll(filepath.Dir(scriptFull), 0755); err != nil {
		t.Fatal(err)
	}

	initial := "#!/bin/bash\nset -euo pipefail\npython3 first.py\necho done\n"
	if err := os.WriteFile(scriptFull, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	verifier := NewVerifier(tmpDir, true)
	verifier.SetInventory(map[string]*InventoryEntry{
		scriptPath: {
			ID: "S001", Path: scriptPath, Language: "shell",
			Role: "verifier", Status: "migration-required",
		},
	})
	verifier.SetBaseline(map[string]*BaselineEntry{
		scriptPath: {
			Path:                  scriptPath,
			BaselineLOC:           0,
			PythonInvocationCount: 1,
		},
	})

	mutated := "#!/bin/bash\nset -euo pipefail\npython3 first.py; python3 second.py\necho done\n"
	if err := os.WriteFile(scriptFull, []byte(mutated), 0644); err != nil {
		t.Fatal(err)
	}

	diags := verifier.checkBaselineEnforcement()
	count := 0
	for _, d := range diags {
		if d.Check == "baseline-python-invocation-changed" && d.Path == scriptPath {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one baseline-python-invocation-changed after ; mutation, got %d (all: %v)", count, diags)
	}
	for _, d := range diags {
		if d.Check == "baseline-python-invocation-changed" && d.Path == scriptPath {
			if !strings.Contains(d.Msg, "baseline=1") || !strings.Contains(d.Msg, "current=2") {
				t.Errorf("diagnostic msg lacks expected numbers: %q", d.Msg)
			}
		}
	}
}

// TestMutationAfterOutputCommand is the R6 regression case for `echo ok;
// python3 added.py` - the output command must not suppress the python
// site that follows.
func TestMutationAfterOutputCommand(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := "scripts/mut_echo.sh"
	scriptFull := filepath.Join(tmpDir, scriptPath)
	if err := os.MkdirAll(filepath.Dir(scriptFull), 0755); err != nil {
		t.Fatal(err)
	}

	initial := "#!/bin/bash\nset -euo pipefail\necho hello\necho done\n"
	if err := os.WriteFile(scriptFull, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	verifier := NewVerifier(tmpDir, true)
	verifier.SetInventory(map[string]*InventoryEntry{
		scriptPath: {
			ID: "S001", Path: scriptPath, Language: "shell",
			Role: "verifier", Status: "migration-required",
		},
	})
	verifier.SetBaseline(map[string]*BaselineEntry{
		scriptPath: {
			Path:                  scriptPath,
			BaselineLOC:           0,
			PythonInvocationCount: 0,
		},
	})

	mutated := "#!/bin/bash\nset -euo pipefail\necho hello\npython3 added.py\necho done\n"
	if err := os.WriteFile(scriptFull, []byte(mutated), 0644); err != nil {
		t.Fatal(err)
	}

	diags := verifier.checkBaselineEnforcement()
	count := 0
	for _, d := range diags {
		if d.Check == "baseline-python-invocation-changed" && d.Path == scriptPath {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one baseline-python-invocation-changed after python added to echo file, got %d (all: %v)", count, diags)
	}
}

func TestMutationCommentsDoNotCount(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := "scripts/mut_comments.sh"
	scriptFull := filepath.Join(tmpDir, scriptPath)
	if err := os.MkdirAll(filepath.Dir(scriptFull), 0755); err != nil {
		t.Fatal(err)
	}

	initial := "#!/bin/bash\nset -euo pipefail\npython3 tool.py\necho done\n"
	if err := os.WriteFile(scriptFull, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	verifier := NewVerifier(tmpDir, true)
	verifier.SetInventory(map[string]*InventoryEntry{
		scriptPath: {
			ID: "S001", Path: scriptPath, Language: "shell",
			Role: "verifier", Status: "migration-required",
		},
	})
	verifier.SetBaseline(map[string]*BaselineEntry{
		scriptPath: {
			Path:                  scriptPath,
			BaselineLOC:           0,
			PythonInvocationCount: 1,
		},
	})

	mutated := "#!/bin/bash\n# python3 mentioned in a comment\nset -euo pipefail\npython3 tool.py\necho \"use python3 for setup\"\necho done\n"
	if err := os.WriteFile(scriptFull, []byte(mutated), 0644); err != nil {
		t.Fatal(err)
	}

	diags := verifier.checkBaselineEnforcement()
	for _, d := range diags {
		if d.Check == "baseline-python-invocation-changed" && d.Path == scriptPath {
			t.Errorf("comments should not register as invocations, got diagnostic: %v", d)
		}
	}
}

// =============================================================================
// R7: walk-error capture tests (P1 fix)
// =============================================================================

// TestWalkMakefilesCapturesTopLevelWalkError pins the R7 P1 fix:
// the error returned by filepath.Walk in walkMakefiles must be
// surfaced as an internal-error diagnostic, not silently dropped.
// Before the fix, a permission-denied or missing-root walk would
// return no diagnostics at all and the verifier would falsely
// green-light the repo.
//
// The closure-level diagnostic uses the phrase "walking for
// Makefiles" (in-progress); the top-level catch uses "walk for
// Makefiles" (finished). Either is acceptable evidence that the
// walk error is no longer swallowed.
func TestWalkMakefilesCapturesTopLevelWalkError(t *testing.T) {
	// A non-existent root forces filepath.Walk to return the lstat
	// error wrapping into its own return value (the closure gets
	// called once with the error, then walk itself returns the same
	// error after the closure finishes).
	missingRoot := filepath.Join(t.TempDir(), "does", "not", "exist")
	verifier := NewVerifier(missingRoot, true)
	var diags []Diagnostic
	verifier.walkMakefiles(missingRoot, &diags)

	found := false
	for _, d := range diags {
		if d.Check != "internal-error" {
			continue
		}
		if strings.Contains(d.Msg, "walk for Makefiles") ||
			strings.Contains(d.Msg, "walking for Makefiles") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected internal-error diagnostic mentioning 'walk for Makefiles', got: %v", diags)
	}
}

