package scriptdoctrine

import (
	"fmt"
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

func TestVerifierFailsClosedOnReadError(t *testing.T) {
	tmpDir := t.TempDir()
	badPath := "scripts/bad.sh"
	badFull := filepath.Join(tmpDir, badPath)
	if err := os.MkdirAll(filepath.Dir(badFull), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badFull, []byte("echo hi\n"), 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(badFull, 0644) })

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
		anyDiag := false
		for _, d := range diags {
			if d.Path == badPath {
				anyDiag = true
			}
		}
		if !anyDiag {
			t.Errorf("expected a diagnostic for unreadable baseline entry, got none (all: %v)", diags)
		}
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
// Mutation test: one added invocation = exactly one violation
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
// Smoke helper
// =============================================================================

func TestPrintAllDiagnostics_SkipUnlessRequested(t *testing.T) {
	if os.Getenv("VERBOSE_DOCTRINE") == "" {
		t.Skip("set VERBOSE_DOCTRINE=1 to print diagnostics")
	}
	tmpDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	verifier := NewVerifier(tmpDir, true)
	for _, d := range verifier.Verify() {
		fmt.Printf("[%s] %s: %s\n", d.Check, d.Path, d.Msg)
	}
}
