package scriptdoctrine

import (
	"strings"
	"testing"
)

// =============================================================================
// R7/R8: AST visitor behaviour tests
// =============================================================================
//
// These tests pin down the mvdan.cc/sh-based walker. They probe the
// classification helpers (isPythonCommandWord, isCommandPrefixWord,
// countPythonInvocationsInLine) and run end-to-end through
// CountPythonInvocations so we exercise the same surface the verifier
// uses. The byte-level tests in shell_analysis_test.go cover the
// file-level helpers (LOC, shebang, FromFile). Keeping them separate
// avoids growing a single test file past the 450-line hard limit the
// LLM-friendliness gate enforces.

// TestCountPythonInvocationsCompoundCommands covers R7 P2: every
// compound shell form (for/while/if/case/subshell/block) is now
// walked by the mvdan.cc/sh AST visitor instead of being silently
// skipped by the handwritten splitter. Each example below would
// have undercounted (often to 0) on the R6 parser, because the
// hand-rolled splitShellList ignored compound structures entirely.
func TestCountPythonInvocationsCompoundCommands(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		// for-loop body.
		{"for loop one body site", "for f in *.py; do python3 \"$f\"; done", 1},
		{"for loop two body sites", "for f in *.py; do python3 a.py; python3 b.py; done", 2},

		// while-loop condition and body.
		{"while loop body only",
			"while true; do python3 tick.py; done", 1},
		{"while loop condition python",
			"while python3 -c 'exit(0)'; do echo ok; done", 1},
		{"until loop body",
			"until false; do python3 poll.py; done", 1},

		// if/elif/else.
		{"if then site",
			"if true; then python3 ok.py; fi", 1},
		{"if else site",
			"if true; then echo no; else python3 fallback.py; fi", 1},
		{"if/elif/else only python in branches",
			"if false; then x; elif true; then python3 ok.py; else python3 err.py; fi", 2},
		{"if condition python is the trigger",
			"if python3 check.py; then echo ok; fi", 1},

		// subshell.
		{"subshell with python",
			"(python3 helper.py)", 1},
		{"subshell piped",
			"(python3 helper.py) | tee log", 1},

		// brace group.
		{"brace group two sites",
			"{ python3 a.py; python3 b.py; }", 2},

		// case-clause.
		{"case arms each site",
			"case $x in a) python3 one.py ;; b) python3 two.py ;; esac", 2},

		// pipeline: each segment is its own command site.
		{"pipeline three stages",
			"python3 a.py | python3 b.py | python3 c.py", 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocations([]byte(tc.content))
			if got != tc.want {
				t.Errorf("CountPythonInvocations(%q) = %d, want %d", tc.content, got, tc.want)
			}
		})
	}
}

// TestCountPythonInvocationsAssignmentAndRedirectionPrefixes covers
// R7 P2: shell prefixes like FOO=bar before the command and
// redirections whose word embeds a $(...) substitution are now
// walked. The handwritten parser stripped FOO= only at line start
// and never recursed into redirection words.
func TestCountPythonInvocationsAssignmentAndRedirectionPrefixes(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		// Single assignment prefix.
		{"single assignment prefix",
			"PYTHONPATH=/opt python3 tool.py", 1},
		{"multiple assignment prefixes",
			"A=1 B=2 python3 tool.py", 1},
		{"assignment prefix does NOT count itself",
			"PYTHON=python3", 0},
		{"assignment prefix then semicolon-joined sites",
			"A=1 python3 a.py; B=2 python3 b.py", 2},

		// Redirections with command substitution - the python lives
		// INSIDE the substitution inside the redirection word.
		{"redirect to substitution",
			"echo data >$(python3 -c 'print(\"/tmp/out\")')", 1},
		{"stdin from substitution",
			"tool < <(python3 gen.py)", 1},
		{"redirect with python in argument",
			"tool > /tmp/$(python3 -c 'print(1)')", 1},

		// Heredoc bodies: with body substitutions the embedded
		// $(...) still counts because the AST keeps the body as a
		// Word that contains a CmdSubst part.
		{"heredoc literal body ignored",
			"cat <<'PY'\nprint('hello world')\nPY\n", 0},
		{"heredoc with expansion",
			"cat <<EOF\nvalue: $(python3 tag.py)\nEOF\n", 1},

		// Assignment + redirect on the same statement.
		{"assignment prefix and redirect substitution",
			"A=1 tee out >$(python3 pick.py)", 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocations([]byte(tc.content))
			if got != tc.want {
				t.Errorf("CountPythonInvocations(%q) = %d, want %d", tc.content, got, tc.want)
			}
		})
	}
}

// TestCountPythonInvocationsQuotingCommentsHeredocs covers R7 P2:
// the AST visitor respects shell quoting rules and the parser no
// longer needs the handwritten "strip inline comment" preprocessor
// (mvdan.cc/sh handles # comments natively).
func TestCountPythonInvocationsQuotingCommentsHeredocs(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		// Inline trailing comments must not count as invocations.
		{"trailing comment on plain command", "echo ok # python3 mentioned", 0},
		{"trailing comment after python invocation",
			"python3 tool.py # explanatory", 1},

		// Quote handling - "python3" inside single/double quotes is data.
		{"single quoted python", `echo 'python3 is nice'`, 0},
		{"double quoted python", `echo "python3 is nice"`, 0},
		{"mixed quoting", `echo 'use' "python3" 'always'`, 0},

		// Quoted command substitution - python is INSIDE the
		// substitution, which IS executed, so it counts.
		{"quoted command substitution runs python",
			`printf '%s' "result=$(python3 -c 'print(1)')"`, 1},

		// Nested compound inside a substitution.
		{"substitution with subshell",
			`echo "$( (python3 inner.py) )"`, 1},

		// Heredoc body containing a script invocation elsewhere.
		{"heredoc body site (comment-like header)",
			"cat <<PY\n# this is body, not a shell comment\nPY\necho done\n", 0},

		// Process substitution.
		{"process substitution",
			"diff <(python3 left.py) <(python3 right.py)", 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocations([]byte(tc.content))
			if got != tc.want {
				t.Errorf("CountPythonInvocations(%q) = %d, want %d", tc.content, got, tc.want)
			}
		})
	}
}

// =============================================================================
// R8: missed-node coverage (FuncDecl, TimeClause, ForClause.Loop, ParamExp)
// =============================================================================
//
// The R7 review identified four shell constructs that the manual
// walker missed. Each test below pins the mvdan.cc/sh-based
// `syntax.Walk` visitor's handling of one of those constructs.
// These are the regression cases that fail-open parsers would lose.

func TestCountPythonInvocationsInFuncDecl(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		// Function body with python invocation. The parser must
		// enter the FuncDecl body and find the python CallExpr.
		// The subsequent call to the function name is not python
		// itself - it is just another CallExpr for a non-python
		// command, so the count is 1, not 2.
		{"python inside function body (called once)",
			`run_python() { python3 inside.py; }; run_python`, 1},
		{"only the body site (function never called)",
			`run_python() { python3 inside.py; }`, 1},
		{"python inside function with two body sites",
			`run_python() { python3 a.py; python3 b.py; }; run_python`, 2},
		{"two function bodies each with python",
			`a() { python3 a.py; }; b() { python3 b.py; }`, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocations([]byte(tc.content))
			if got != tc.want {
				t.Errorf("CountPythonInvocations(%q) = %d, want %d", tc.content, got, tc.want)
			}
		})
	}
}

func TestCountPythonInvocationsInTimeClause(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{"time with python target",
			`time python3 timed.py`, 1},
		{"time pipeline with python",
			`time python3 a.py | python3 b.py`, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocations([]byte(tc.content))
			if got != tc.want {
				t.Errorf("CountPythonInvocations(%q) = %d, want %d", tc.content, got, tc.want)
			}
		})
	}
}

func TestCountPythonInvocationsInForLoopIterable(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{"for iterates command substitution result",
			`for x in $(python3 items.py); do echo "$x"; done`, 1},
		{"for body also python (2 sites)",
			`for x in $(python3 items.py); do python3 body.py; done`, 2},
		{"for iterable is a regular word",
			`for x in *.py; do python3 "$x"; done`, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocations([]byte(tc.content))
			if got != tc.want {
				t.Errorf("CountPythonInvocations(%q) = %d, want %d", tc.content, got, tc.want)
			}
		})
	}
}

func TestCountPythonInvocationsInParamExpDefault(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{"param-exp default to command substitution",
			`echo "${value:-$(python3 fallback.py)}"`, 1},
		{"param-exp default to literal word (no python)",
			`echo "${value:-literal}"`, 0},
		{"arith-exp and command substitution",
			`echo "$(($(python3 size.py) + 1))"`, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocations([]byte(tc.content))
			if got != tc.want {
				t.Errorf("CountPythonInvocations(%q) = %d, want %d", tc.content, got, tc.want)
			}
		})
	}
}

// TestCountPythonInvocationsInCoprocClause pins the coproc coverage:
// `coproc NAME { cmd; }` and `coproc NAME cmd` shells both carry
// runnable bodies. mvdan.cc/sh's `syntax.Walk` visits them; the
// previous manual walker did not.
func TestCountPythonInvocationsInCoprocClause(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{"coproc with python body",
			`coproc python3 worker.py`, 1},
		{"coproc named block with python",
			`coproc pyjob { python3 worker.py; }`, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocations([]byte(tc.content))
			if got != tc.want {
				t.Errorf("CountPythonInvocations(%q) = %d, want %d", tc.content, got, tc.want)
			}
		})
	}
}

// =============================================================================
// R7 fail-closed + scanner.Err regression tests
// =============================================================================

// TestCountPythonInvocationsMalformedSyntaxFailsClosed covers the
// R7 P2 fail-closed contract: malformed shell syntax is reported
// as -1 by both the line-level helper and the byte-level wrapper.
// Returning 0 would falsely green-light the script; the caller
// (verifier) must surface an internal-error diagnostic instead.
func TestCountPythonInvocationsMalformedSyntaxFailsClosed(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"unclosed double quote", `echo "hello`},
		{"unclosed single quote", `echo 'hello`},
		{"missing then", `if true then fi`},
		{"empty pipe right side", `echo foo |`},
		{"unclosed $(...)", `echo "$(python3 tool.py`},
		{"unclosed $((...))", `echo $((1+`},
		// R8 regression: a malformed line wedged between good
		// lines must NOT be silently swallowed. The whole-file
		// parser must return -1, not 1.
		{"mixed good and malformed",
			"python3 baseline.py\nif true then python3 hidden.py fi\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Probe the line-level helper directly so we know the
			// failure comes from parsing, not from the reader scan.
			got := countPythonInvocationsInLine(tc.input)
			if got != -1 {
				t.Errorf("countPythonInvocationsInLine(%q) = %d, want -1 (fail-closed)", tc.input, got)
			}
			// And the byte-level wrapper must also surface -1.
			wrapper := CountPythonInvocations([]byte(tc.input))
			if wrapper != -1 {
				t.Errorf("CountPythonInvocations(%q) = %d, want -1 (fail-closed)", tc.input, wrapper)
			}
		})
	}
}

// TestCountPythonInvocationsLargeInputSucceeds pins the AST path's
// behaviour on large inputs: the per-line `bufio.Scanner`
// overflow that motivated the R7 P1 fix is gone, so a multi-KiB
// single line (which R7-era `bufio.Scanner` would reject as
// ErrTooLong) is now parsed as one shell command tree. The count
// should reflect the embedded invocations exactly.
func TestCountPythonInvocationsLargeInputSucceeds(t *testing.T) {
	// Build a "line" longer than the 64KiB scanner buffer that
	// the old per-line scanner used to enforce.
	overlong := strings.Repeat("python3 a.py; ", 5000)
	if len(overlong) < 64*1024 {
		t.Fatalf("test setup: oversize line must be > 64KiB, got %d bytes", len(overlong))
	}
	got := CountPythonInvocations([]byte(overlong))
	if got != 5000 {
		t.Errorf("CountPythonInvocations on %d-byte line = %d, want 5000", len(overlong), got)
	}
}

// TestCountPythonInvocationsNilReturnsMinusOne enforces the
// existing contract that nil input is treated as an error, not
// zero.
func TestCountPythonInvocationsNilReturnsMinusOne(t *testing.T) {
	got := CountPythonInvocations(nil)
	if got != -1 {
		t.Errorf("CountPythonInvocations(nil) = %d, want -1", got)
	}
}
