package scriptdoctrine

import (
	"os"
	"path/filepath"
	"testing"
)

// R16 closure tests — Make lexical state machine with `.RECIPEPREFIX`
// reassignment, recipe-prefix reset, and logical-line continuation
// support. The matrix below extends the R14/R15 categories with the
// expert review blocker surfaces called out in the R16 verdict.

const r16EveRecipePrefixChange = "" +
	".RECIPEPREFIX = >\n" +
	"first:\n" +
	"># no Python\n" +
	"\n" +
	".RECIPEPREFIX = |\n" +
	"second:\n" +
	"|# $(shell python3 hidden.py)\n"

const r16EveRecipePrefixReset = "" +
	".RECIPEPREFIX = >\n" +
	"first:\n" +
	">echo ok\n" +
	"\n" +
	".RECIPEPREFIX =\n" +
	"second:\n" +
	"\t# $(shell python3 hidden.py)\n"

const r16EveContinuationRecipe = "" +
	"all:\n" +
	"\tpython3 \\\n" +
	"\t    step.py arg\n"

const r16EveContinuationComment = "" +
	"all:\n" +
	"VALUE := foo \\\n" +
	"     # $(shell python3 hidden.py)\n" +
	"\techo ok\n"

// TestR16MakeLexStateMatrix pins the state-machine close. Each row is
// the R16 verdict's required case (plus a couple of R12/R14/R15 rows
// kept here for regression coverage).
func TestR16MakeLexStateMatrix(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		// R12 row — column-one comment.
		{
			name: "toplevel comment suppressed (R12 preserved)",
			data: "# $(shell python3 x.py)\nall:\n\techo ok\n",
			want: 0,
		},
		// R15 row — recipe prefix `>` declared via `.RECIPEPREFIX`.
		{
			name: "recipe prefix > still counts (R15 preserved)",
			data: ".RECIPEPREFIX = >\nall:\n># $(shell python3 x.py)\n",
			want: 1,
		},
		// R15 row — escaped hash counts as code.
		{
			name: "escaped hash literal (R15 preserved)",
			data: "VALUE := \\# $(shell python3 x.py)\nall:\n\techo ok\n",
			want: 1,
		},
		// R16 NEW — recipe prefix changes > to | across epochs.
		{
			name: "recipe prefix changes > to | (second epoch counts)",
			data: r16EveRecipePrefixChange,
			want: 1,
		},
		// R16 NEW — resetting `.RECIPEPREFIX =` (no value) restores TAB.
		{
			name: "recipe prefix reset to TAB (tab recipe counts)",
			data: r16EveRecipePrefixReset,
			want: 1,
		},
		// R16 NEW — logical-line continuation: recipe spans a `\<newline>`.
		{
			name: "continued recipe physical line (recipe retained)",
			data: r16EveContinuationRecipe,
			want: 1,
		},
		// R16 NEW — continuation that is a comment in non-recipe
		// context is masked (a top-level continuation, not a recipe).
		{
			name: "continued non-recipe comment (masked)",
			data: r16EveContinuationComment,
			want: 0,
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

// TestR16VerifierPrefixTransitionBypass covers the review-bypass
// attack class: the second epoch's prefix re-declaration is not seen
// by the previous global-prefix masker, so the verifier fails to
// find the Python invocation. This test pins the end-to-end
// Verifier.Verify() path, not just the helper count.
func TestR16VerifierPrefixTransitionBypass(t *testing.T) {
	tmpDir := t.TempDir()
	makefilePath := filepath.Join(tmpDir, "Makefile")
	const content = r16EveRecipePrefixChange
	if err := os.WriteFile(makefilePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	v := NewVerifier(tmpDir, false)
	diags := v.Verify()
	pythonInvocationDiag := false
	for _, d := range diags {
		if d.Check == "python-invocation" && filepath.Base(d.Path) == "Makefile" {
			pythonInvocationDiag = true
			total := parseSiteCount(d.Msg)
			if total != 1 {
				t.Errorf("expected 1 python invocation for prefix-transition bypass, got %d (full diags: %+v)", total, diags)
			}
			return
		}
	}
	if !pythonInvocationDiag {
		t.Fatalf("expected python-invocation diag for Makefile, got: %+v", diags)
	}
}

// TestR16DirectivePreservesPrefixCommentIgnored ensures that a
// `.RECIPEPREFIX = X` directive inside a `#`-comment does NOT
// leak into the active prefix. The next line still uses the
// previous prefix (TAB), and the `#`-prefix text does not start
// a recipe line.
func TestR16DirectivePreservesPrefixCommentIgnored(t *testing.T) {
	// Case A: comment line at column 1 whose body mentions
	// `.RECIPEPREFIX = i`. The leading `#` makes this a
	// comment-only line; the directive embedded inside the
	// comment must NOT change the active recipe prefix. The
	// next line is therefore NOT a recipe (TAB is the default
	// prefix but `>echo i-prefix` would still not be a recipe
	// under default TAB). We assert the corollary: a TAB-prefix
	// recipe line continues to count as 1 because the prefix did
	// not change.
	const contentA = "" +
		"# .RECIPEPREFIX = i\n" +
		"all:\n" +
		"\tpython3 tool.py\n"
	if got := CountPythonInvocationsInMakefile([]byte(contentA)); got != 1 {
		t.Errorf("directive-in-comment must NOT change prefix: count=%d, want 1", got)
	}
	// Case B: the same `.RECIPEPREFIX = i` directive IN force
	// (not inside a comment) IS observed. The recipe line
	// starts with `i`, not TAB, so it counts as 1.
	const contentB = "" +
		".RECIPEPREFIX = i\n" +
		"all:\n" +
		"ipython3 tool.py\n"
	if got := CountPythonInvocationsInMakefile([]byte(contentB)); got != 1 {
		t.Errorf("directive in force must drive the active prefix: count=%d, want 1", got)
	}
}
