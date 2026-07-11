package scriptdoctrine

import "testing"

// TestR15MakeCommentContextMatrix closes the R15 review blocker
// that ordinary indented Make comments and trailing inline
// comments still counted as Python invocations after R14. The
// R14 byte-level `(i == 0 || data[i-1] == '\n')` check treated
// any non-newline byte (space, TAB) as a separator and never
// suppressed an indented `#`, so surfaces like
// `   # $(shell python3 x.py)` and
// `VALUE := ok # $(shell python3 hidden.py)` falsely inflated
// the count.
//
// R15 replaces that check with a line-aware
// `maskMakeComments` pass that recognises the GNU Make rule:
//
//   - In any non-recipe logical line, an unescaped `#` at any
//     column starts a comment that runs to EOL.
//   - In a recipe line (TAB by default; `.RECIPEPREFIX = X`
//     when declared), the line is passed to the shell and
//     `$(shell ...)` is expanded before parsing.
//   - `\#` is a literal `#` (escape), NOT a comment start.
func TestR15MakeCommentContextMatrix(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		{
			// Column-one comment (R12 row preserved).
			name: "toplevel comment suppressed",
			data: "# $(shell python3 x.py)\nall:\n\techo ok\n",
			want: 0,
		},
		{
			// NEW: indented comment (whitespace prefix before `#`).
			name: "indented comment suppressed",
			data: "all:\n   # $(shell python3 x.py)\n\techo ok\n",
			want: 0,
		},
		{
			// NEW: trailing inline comment after a real assignment.
			name: "trailing inline comment suppressed",
			data: "VALUE := ok # $(shell python3 x.py)\nall:\n\techo ok\n",
			want: 0,
		},
		{
			// NEW: `\#` is a literal `#` and must NOT trigger comment
			// suppression.
			name: "escaped hash still counts",
			data: "VALUE := \\# $(shell python3 x.py)\nall:\n\techo ok\n",
			want: 1,
		},
		{
			// R14 row preserved: TAB-prefixed recipe lines still expand
			// `$(shell ...)`.
			name: "recipe line comment still counts",
			data: "all:\n\t# $(shell python3 x.py)\n",
			want: 1,
		},
		{
			// NEW: `.RECIPEPREFIX = >` declares a non-TAB prefix; a
			// `>`-prefixed recipe line must still count.
			name: "recipe prefix > still counts",
			data: ".RECIPEPREFIX = >\nall:\n># $(shell python3 x.py)\n",
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
