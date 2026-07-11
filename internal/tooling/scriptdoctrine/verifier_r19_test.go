package scriptdoctrine

import (
	"os"
	"path/filepath"
	"testing"
)

// R19 closure tests: assignment-operator disambiguation.
//
// The R18 target detector incorrectly treated `::=` and `:::=` as
// rule separators because it only skipped `:=`. GNU Make officially
// supports these assignment operators:
//   ::=   (simply expanded variable)
//   :::=  (immediately expanded variable)
//
// The fix teaches isMakeTargetLine to skip `::=` and `:::=` before
// classifying `:` as a rule separator.

// Simply expanded variable assignment (::=).
const r19EveSimplyExpanded = "" +
	".RECIPEPREFIX = .\n" +
	"VAR ::= value\n" +
	".RECIPEPREFIX = >\n" +
	"\n" +
	"second:\n" +
	">$(shell python3 hidden.py)\n"

// Immediately expanded variable assignment (:::=).
const r19EveImmediatelyExpanded = "" +
	".RECIPEPREFIX = .\n" +
	"VAR :::= value\n" +
	".RECIPEPREFIX = >\n" +
	"\n" +
	"second:\n" +
	">$(shell python3 hidden.py)\n"

// Target-specific simply-expanded variable assignment.
//
// The Make rule has a genuine `:` separator before `::=`, so the
// line must be classified as a target AND its recipe must be
// recognised. The recipe uses a TAB prefix (the default Make
// recipe prefix) and is NOT inside `$(shell ...)`, so the count
// proves the recipe context is preserved when the assignment uses
// `::=`.
const r19EveTargetSpecificSimplyExpanded = "" +
	"target: LOCAL ::= value\n" +
	"\tpython3 x.py\n"

// Target-specific immediately-expanded variable assignment.
//
// Same shape as the `::=` case but with `:::=`. The recipe must
// still be recognised as a python invocation.
const r19EveTargetSpecificImmediatelyExpanded = "" +
	"target: LOCAL :::= value\n" +
	"\tpython3 x.py\n"

// Double-colon rule with dependency.
const r19EveDoubleColonRule = "" +
	"target:: dependency\n" +
	"\tpython3 hidden.py\n"

// TestR19AssignmentOperatorDisambiguation pins each R19 required
// case from the expert review: ::=, :::=, target-specific ::=,
// target-specific :::=, and genuine double-colon rules.
func TestR19AssignmentOperatorDisambiguation(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		{
			// Simply expanded variable: ::= ends rule context, allows
			// prefix transition, then recipe executes python.
			name: "VAR ::= value + prefix transition + Python recipe",
			data: r19EveSimplyExpanded,
			want: 1,
		},
		{
			// Immediately expanded variable: :::= works the same way.
			name: "VAR :::= value + prefix transition + Python recipe",
			data: r19EveImmediatelyExpanded,
			want: 1,
		},
		{
			// Target-specific ::=. The first `:` is a genuine rule
			// separator; the `::=` after it is an assignment to LOCAL.
			// The recipe (TAB-prefixed, no $(shell)) must be counted.
			name: "target: LOCAL ::= value + TAB python recipe",
			data: r19EveTargetSpecificSimplyExpanded,
			want: 1,
		},
		{
			// Target-specific :::=. Same shape, with the immediately
			// expanded variant. The recipe must still be counted.
			name: "target: LOCAL :::= value + TAB python recipe",
			data: r19EveTargetSpecificImmediatelyExpanded,
			want: 1,
		},
		{
			// Double-colon rule: `::` without trailing `=` is a rule.
			name: "target:: dependency + python recipe",
			data: r19EveDoubleColonRule,
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

// TestR19VerifierSimplyExpandedBypass covers the end-to-end
// `Verifier.Verify()` path for the primary ::= attack vector.
// A Makefile with `VAR ::= value` followed by a prefix transition
// and a python recipe must produce one `python-invocation` diagnostic.
func TestR19VerifierSimplyExpandedBypass(t *testing.T) {
	tmpDir := t.TempDir()
	makefilePath := filepath.Join(tmpDir, "Makefile")
	if err := os.WriteFile(makefilePath, []byte(r19EveSimplyExpanded), 0644); err != nil {
		t.Fatal(err)
	}
	v := NewVerifier(tmpDir, false)
	diags := v.Verify()
	found := false
	for _, d := range diags {
		if d.Check == "python-invocation" && filepath.Base(d.Path) == "Makefile" {
			found = true
			if total := parseSiteCount(d.Msg); total != 1 {
				t.Errorf("expected 1 python invocation for ::= bypass, got %d (diags: %+v)", total, diags)
			}
		}
	}
	if !found {
		t.Fatalf("expected python-invocation diag for Makefile, got: %+v", diags)
	}
}

// TestR19VerifierImmediatelyExpandedBypass covers the end-to-end
// `Verifier.Verify()` path for the `:::=` attack vector.
func TestR19VerifierImmediatelyExpandedBypass(t *testing.T) {
	tmpDir := t.TempDir()
	makefilePath := filepath.Join(tmpDir, "Makefile")
	if err := os.WriteFile(makefilePath, []byte(r19EveImmediatelyExpanded), 0644); err != nil {
		t.Fatal(err)
	}
	v := NewVerifier(tmpDir, false)
	diags := v.Verify()
	found := false
	for _, d := range diags {
		if d.Check == "python-invocation" && filepath.Base(d.Path) == "Makefile" {
			found = true
			if total := parseSiteCount(d.Msg); total != 1 {
				t.Errorf("expected 1 python invocation for :::: bypass, got %d (diags: %+v)", total, diags)
			}
		}
	}
	if !found {
		t.Fatalf("expected python-invocation diag for Makefile, got: %+v", diags)
	}
}

// TestR19VerifierTargetSpecificSimplyExpanded covers the
// end-to-end `Verifier.Verify()` path for the target-specific
// `::=` case. A Makefile with `target: LOCAL ::= value` plus a
// TAB-prefixed python recipe must produce one `python-invocation`
// diagnostic.
func TestR19VerifierTargetSpecificSimplyExpanded(t *testing.T) {
	tmpDir := t.TempDir()
	makefilePath := filepath.Join(tmpDir, "Makefile")
	if err := os.WriteFile(makefilePath, []byte(r19EveTargetSpecificSimplyExpanded), 0644); err != nil {
		t.Fatal(err)
	}
	v := NewVerifier(tmpDir, false)
	diags := v.Verify()
	found := false
	for _, d := range diags {
		if d.Check == "python-invocation" && filepath.Base(d.Path) == "Makefile" {
			found = true
			if total := parseSiteCount(d.Msg); total != 1 {
				t.Errorf("expected 1 python invocation for target-specific ::=, got %d (diags: %+v)", total, diags)
			}
		}
	}
	if !found {
		t.Fatalf("expected python-invocation diag for Makefile, got: %+v", diags)
	}
}
