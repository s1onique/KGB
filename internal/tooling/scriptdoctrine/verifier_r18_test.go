package scriptdoctrine

import (
	"os"
	"path/filepath"
	"testing"
)

// R18 closure tests: explicit rule recognition.
//
// The R17 walker still misclassified ordinary rules whose last
// byte was not `:`. R18 lifts the heuristic: any `:` (or `::`,
// `&:`, `&::`) before an unescaped `=` is a target rule separator.
// The classifier also recognises dot-prefixed targets
// (`.PHONY: all`, `.hidden: dep`) before falling to the
// `.`-directive branch.

const r18EvePrereqRule = "" +
	"all: dependency\n" +
	"\tpython3 hidden.py\n"

const r18EveOrderOnly = "" +
	"all: | order-only\n" +
	"\tpython3 hidden.py\n"

const r18EveDotTarget = "" +
	".RECIPEPREFIX = .\n" +
	".hidden: source\n" +
	".python3 hidden.py\n"

const r18EvePhonyTarget = "" +
	".PHONY: all\n" +
	"all: dependency\n" +
	"\tpython3 hidden.py\n"

const r18EvePatternRule = "" +
	"%.o: %.c\n" +
	"\tpython3 generator.py\n"

const r18EveDoubleColon = "" +
	"target:: dependency\n" +
	"\tpython3 hidden.py\n"

// TestR18ExplicitRuleRecognition pins each R18 required case
// from the expert review.
func TestR18ExplicitRuleRecognition(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		{
			// Prerequisite rule (most common form).
			name: "all: dependency <TAB>python3 x.py",
			data: r18EvePrereqRule,
			want: 1,
		},
		{
			// Order-only prerequisite.
			name: "all: | order-only <TAB>python3 x.py",
			data: r18EveOrderOnly,
			want: 1,
		},
		{
			// Dot-prefixed target.
			name: ".RECIPEPREFIX = . .hidden: dep .python3 x.py",
			data: r18EveDotTarget,
			want: 1,
		},
		{
			// `.PHONY: all` is itself a target rule.
			name: ".PHONY: all all: dep <TAB>python3 x.py",
			data: r18EvePhonyTarget,
			want: 1,
		},
		{
			// Pattern rule.
			name: "%.o: %.c <TAB>python3 generator.py",
			data: r18EvePatternRule,
			want: 1,
		},
		{
			// Double-colon rule.
			name: "target:: dependency <TAB>python3 x.py",
			data: r18EveDoubleColon,
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

// TestR18VerifierPrereqRuleBypass covers the end-to-end
// `Verifier.Verify()` path: a Makefile that uses ordinary
// rule-with-prerequisites syntax must produce one
// `python-invocation` diagnostic for `Makefile`.
func TestR18VerifierPrereqRuleBypass(t *testing.T) {
	tmpDir := t.TempDir()
	makefilePath := filepath.Join(tmpDir, "Makefile")
	if err := os.WriteFile(makefilePath, []byte(r18EvePrereqRule), 0644); err != nil {
		t.Fatal(err)
	}
	v := NewVerifier(tmpDir, false)
	diags := v.Verify()
	found := false
	for _, d := range diags {
		if d.Check == "python-invocation" && filepath.Base(d.Path) == "Makefile" {
			found = true
			if total := parseSiteCount(d.Msg); total != 1 {
				t.Errorf("expected 1 python invocation for prereq rule, got %d (diags: %+v)", total, diags)
			}
		}
	}
	if !found {
		t.Fatalf("expected python-invocation diag for Makefile, got: %+v", diags)
	}
}
