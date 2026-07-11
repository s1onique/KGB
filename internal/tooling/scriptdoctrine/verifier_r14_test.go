package scriptdoctrine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// R14 closure tests.
//
// Each test exercises one of the four R14 review items:
//
//   - Typed-error propagation through Verifier.Verify() (Item 2).
//   - Shared bash value-option parsing for the wrapped (sudo/env/exec)
//     dispatcher (Item 3).
//   - Makefile recipe-context `$(shell ...)` extraction (Item 4).
//   - End-to-end through the real Verifier, not just helper functions.
//
// The tests are intentionally placed in a fresh file so the
// existing R11/R12/R13 test files stay below the LLM-friendly
// 450-line hard limit.

// writeR14Fixture writes path under root with content and returns
// the relative path used for the script inventory.
func writeR14Fixture(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// hasInternalDiagWithLine returns true when any internal-error
// diagnostic for path carries a non-zero Line and Column. The
// R14 closure expects the verifier to surface line/column from
// the typed *ClassificationError via errors.As.
func hasInternalDiagWithLine(diags []Diagnostic, path string) (Diagnostic, bool) {
	for _, d := range diags {
		if d.Check == "internal-error" && d.Path == path && d.Line > 0 && d.Column > 0 {
			return d, true
		}
	}
	return Diagnostic{}, false
}

// ----------------------------------------------------------------------------
// R14 Item 2: typed-error propagation through Verifier.Verify().
// ----------------------------------------------------------------------------

// TestR14Mutation_DynamicBashCDiagnosticLineColumn pins the
// R14.2 closure that a `bash -c "$DYNAMIC"` payload must reach
// Verifier.Verify() with a non-zero Line and Column on the
// surfaced internal-error diagnostic. Before the R14 closure the
// verification path collapsed the typed *ClassificationError into
// a generic integer (-1) diagnostic, so the Line/Column never
// reached the user.
func TestR14Mutation_DynamicBashCDiagnosticLineColumn(t *testing.T) {
	tmpDir := t.TempDir()
	rel := "scripts/dyn_bash_c.sh"
	full := filepath.Join(tmpDir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	// 5-line script; the failing call lives on line 3.
	const content = "#!/bin/bash\n" +
		"set -euo pipefail\n" +
		"bash -c \"$DYNAMIC\"\n" +
		"echo ok\n"
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	v := NewVerifier(tmpDir, false)
	diags := v.Verify()

	got, ok := hasInternalDiagWithLine(diags, rel)
	if !ok {
		t.Fatalf("expected internal-error diag with line/column for %s, got: %+v", rel, diags)
	}
	if got.Line != 3 {
		t.Errorf("internal-error Line = %d, want 3 (the bash -c call line)", got.Line)
	}
	if got.Column < 1 {
		t.Errorf("internal-error Column = %d, want >= 1", got.Column)
	}
	if !strings.Contains(got.Msg, "dynamic bash -c") {
		t.Errorf("internal-error Msg = %q, want reason mentioning dynamic bash -c", got.Msg)
	}
}

// TestR14Mutation_WrappedDynamicBashCDiagnosticLineColumn covers
// the same fail-closed contract via the wrapped dispatcher
// (sudo/env/exec). The verifier must surface the original
// line/column of the OUTER call rather than zero.
func TestR14Mutation_WrappedDynamicBashCDiagnosticLineColumn(t *testing.T) {
	tmpDir := t.TempDir()
	rel := "scripts/dyn_wrapped.sh"
	full := filepath.Join(tmpDir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	const content = "#!/bin/bash\n" +
		"set -euo pipefail\n" +
		"sudo bash -c \"$DYNAMIC\"\n" +
		"echo ok\n"
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	v := NewVerifier(tmpDir, false)
	diags := v.Verify()

	got, ok := hasInternalDiagWithLine(diags, rel)
	if !ok {
		t.Fatalf("expected internal-error diag with line/column for %s, got: %+v", rel, diags)
	}
	if got.Line != 3 {
		t.Errorf("wrapped internal-error Line = %d, want 3", got.Line)
	}
	if got.Column < 1 {
		t.Errorf("wrapped internal-error Column = %d, want >= 1", got.Column)
	}
}

// TestR14Mutation_DynamicMakeCommandDiagnosticLineColumn pins
// the R14.2 closure that an unresolved Make-variable reference
// inside `$(shell ...)` reaches Verify() with a non-zero Line
// and Column. Before R14 the typed *ClassificationError was
// collapsed to -1 via the int-returning
// CountPythonInvocationsInMakefile, so this surface silently
// produced `count=0 err=nil` and never reached the diagnostic
// pipeline.
//
// The surface uses $(shell $(UNKNOWN) x.py) which the Make
// extractor recognises as an unresolved command-position
// reference (R12 fail-closed gate).
func TestR14Mutation_DynamicMakeCommandDiagnosticLineColumn(t *testing.T) {
	tmpDir := t.TempDir()
	mkfile := filepath.Join(tmpDir, "Makefile")
	const content = "all:\n" +
		"\techo ok\n" +
		"RESULT := $(shell $(UNKNOWN) x.py)\n"
	if err := os.WriteFile(mkfile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	v := NewVerifier(tmpDir, false)
	diags := v.Verify()

	got, ok := hasInternalDiagWithLine(diags, "Makefile")
	if !ok {
		t.Fatalf("expected internal-error diag with line/column for Makefile, got: %+v", diags)
	}
	if got.Line != 3 {
		t.Errorf("Makefile internal-error Line = %d, want 3", got.Line)
	}
	if got.Column < 1 {
		t.Errorf("Makefile internal-error Column = %d, want >= 1", got.Column)
	}
}

// TestR14Mutation_DynamicWorkflowShellDiagnosticLineColumn pins
// the R14.2 closure for GitHub Actions workflows whose `shell:`
// field is dynamic (`shell: ${{ matrix.shell }}`). The verifier
// must surface a typed diagnostic with the step's real Line and
// Column (R11.5 YAML AST path) instead of an integer -1.
func TestR14Mutation_DynamicWorkflowShellDiagnosticLineColumn(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".github", "workflows"), 0755); err != nil {
		t.Fatal(err)
	}
	workflowPath := filepath.Join(tmpDir, ".github", "workflows", "r14-dyn-shell.yml")
	// The shell field lives on line 6; the run field on line 5.
	const content = "name: R14 dynamic shell\n" +
		"on: workflow_dispatch\n" +
		"jobs:\n" +
		"  build:\n" +
		"    runs-on: ubuntu-latest\n" +
		"    steps:\n" +
		"      - run: python3 hidden.py\n" +
		"        shell: ${{ matrix.shell }}\n"
	if err := os.WriteFile(workflowPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	v := NewVerifier(tmpDir, false)
	diags := v.Verify()

	rel := ".github/workflows/r14-dyn-shell.yml"
	got, ok := hasInternalDiagWithLine(diags, rel)
	if !ok {
		t.Fatalf("expected internal-error diag with line/column for %s, got: %+v", rel, diags)
	}
	if got.Line != 7 {
		t.Errorf("workflow internal-error Line = %d, want 7 (shell field)", got.Line)
	}
	if got.Column < 1 {
		t.Errorf("workflow internal-error Column = %d, want >= 1", got.Column)
	}
}

// ----------------------------------------------------------------------------
// R14 Item 3: bash value options on the wrapped (sudo/env/exec) path.
// ----------------------------------------------------------------------------

// TestR14BashWrappedValueOptions pins the R14.3 closure that
// value-taking bash options (-O, +O, --rcfile, --init-file)
// apply symmetrically on the wrapped dispatcher. Before R14
// only the direct `bash -c` path consumed the option's value,
// so `sudo bash -O extglob -c 'python3 x.py'` returned zero
// (the value was misread as the script path).
func TestR14BashWrappedValueOptions(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		{"sudo -O extglob -c", `sudo bash -O extglob -c 'python3 x.py'`, 1},
		{"env --rcfile -c", `env X=1 bash --rcfile myfile -c 'python3 x.py'`, 1},
		{"exec +O -c", `exec bash +O extglob -c 'python3 x.py'`, 1},
		{"sudo -O dynamic -c (hard error)", `sudo bash -O extglob -c "$DYNAMIC"`, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocations([]byte(tc.data))
			if got != tc.want {
				t.Errorf("CountPythonInvocations(%q) = %d, want %d", tc.data, got, tc.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// R14 Item 4: Makefile recipe-context `$(shell ...)` extraction.
// ----------------------------------------------------------------------------

// TestR14MakeRecipeContextShell pins the R14.4 closure that a
// TAB-prefixed `# $(shell ...)` (recipe context) counts as one
// python invocation, while a top-level `# $(shell ...)` Make
// comment does not. GNU Make treats `#` at the start of a
// logical line (preceded by a newline) as a comment, but a
// TAB-anchored `#` is part of a recipe and is still expanded
// at Make-time.
func TestR14MakeRecipeContextShell(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		{
			name: "top-level comment containing $(shell ...) is ignored",
			data: "# $(shell python3 x.py)\nall:\n\techo ok\n",
			want: 0,
		},
		{
			name: "recipe-line comment containing $(shell ...) is expanded",
			data: "all:\n\t# $(shell python3 x.py)\n",
			want: 1,
		},
		{
			name: "recipe line with embedded comment expands $(shell ...)",
			data: "all:\n\techo \"# $(shell python3 x.py)\"\n",
			want: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocationsInMakefile([]byte(tc.data))
			if got != tc.want {
				t.Errorf("CountPythonInvocationsInMakefile(%q) = %d, want %d",
					tc.data, got, tc.want)
			}
		})
	}
}

// TestR14Mutation_BashWrappedValueOptionVerifier pins the full
// Verifier.Verify() path for the wrapped value-option matrix
// (Item 3). The test files exist in a synthetic repo so the
// verifier can read them through the standard walk. We assert
// the diagnostic count for each row matches the closure target.
func TestR14Mutation_BashWrappedValueOptionVerifier(t *testing.T) {
	cases := []struct {
		name        string
		content     string
		wantCount   int
		skipOnError bool
	}{
		{
			name:      "sudo bash -O extglob -c python3",
			content:   "#!/bin/bash\nset -euo pipefail\nsudo bash -O extglob -c 'python3 x.py'\necho ok\n",
			wantCount: 1,
		},
		{
			name:      "env --rcfile -c python3",
			content:   "#!/bin/bash\nset -euo pipefail\nenv X=1 bash --rcfile f -c 'python3 x.py'\necho ok\n",
			wantCount: 1,
		},
		{
			name:      "exec +O -c python3",
			content:   "#!/bin/bash\nset -euo pipefail\nexec bash +O extglob -c 'python3 x.py'\necho ok\n",
			wantCount: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			rel := "scripts/r14_bash.sh"
			full := filepath.Join(tmpDir, rel)
			if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}
			v := NewVerifier(tmpDir, false)
			diags := v.Verify()
			total := 0
			for _, d := range diags {
				if d.Check == "python-invocation" && d.Path == rel {
					total += parseSiteCount(d.Msg)
				}
			}
			if total != tc.wantCount {
				t.Errorf("python-invocation total = %d, want %d (diags=%+v)",
					total, tc.wantCount, diags)
			}
		})
	}
}
