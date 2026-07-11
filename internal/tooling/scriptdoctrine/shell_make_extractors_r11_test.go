package scriptdoctrine

import "testing"

// TestR11MakeShellFunction pins the R11.4 `$(shell ...)` matrix.
//
// GNU Make evaluates `$(shell ...)` at Make-expansion time, NOT
// recipe time, so a Python invocation inside `$(shell ...)` runs
// even when no recipe ever fires. R10 silently undercounted; R11
// scans the entire Makefile for these sites and aggregates their
// classification.

func TestR11MakeShellFunction(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		// Plain literal payload.
		{"plain literal",
			"RESULT := $(shell python3 x.py)\nall:\n\techo $$RESULT", 1},
		// != shell assignment.
		{"!= assignment",
			"RESULT != python3 x.py\nall:\n\techo ok", 1},
		// Use from a recipe.
		{"echo in recipe",
			"all:\n\techo $(shell python3 x.py)", 1},
		// Variable indirection followed through $(shell).
		{"PYTHON := python3 + $(shell ...)",
			"PYTHON := python3\nRESULT := $(shell $(PYTHON) x.py)\nall:\n\techo $$RESULT", 1},
		// Empty inner body - silently 0.
		{"empty $(shell)",
			"RESULT := $(shell)\nall:\n\techo $$RESULT", 0},
		// Non-Python literal $(shell) - 0.
		{"echo only",
			"OUT := $(shell echo python3)\nall:\n\techo $$OUT", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocationsInMakefile([]byte(tc.data))
			if got != tc.want {
				t.Errorf("CountPythonInvocationsInMakefile(%q) = %d, want %d", tc.data, got, tc.want)
			}
		})
	}
}

// TestR11MakeShellAssignmentDynamic documents the fail-closed
// contract for `!=` shell assignments whose RHS is dynamic: the
// verifier surfaces an internal error rather than a count.
func TestR11MakeShellFunctionDynamic(t *testing.T) {
	cases := []string{
		"RESULT := $(shell bash -c \"$DYNAMIC\")\nall:\n\techo $$RESULT",
		"unbalanced $(shell python3 x.py",
	}
	for _, data := range cases {
		t.Run(data, func(t *testing.T) {
			got := CountPythonInvocationsInMakefile([]byte(data))
			if got != -1 {
				t.Errorf("CountPythonInvocationsInMakefile(%q) = %d, want -1 (fail-closed)", data, got)
			}
		})
	}
}

// TestR11MakeCountingInvariant guarantees that a single
// $(shell python3 x.py) site is counted ONCE even when both the
// expansion-time classifier and the recipe parser look at the
// same Makefile. The recipe parser receives a masked copy of the
// Makefile in which the `$(shell ...)` body is whitespace.
func TestR11MakeCountingInvariant(t *testing.T) {
	data := "RESULT := $(shell python3 x.py)\nall:\n\techo \"x is $$RESULT\"\n"
	got := CountPythonInvocationsInMakefile([]byte(data))
	if got != 1 {
		t.Errorf("CountPythonInvocationsInMakefile(%q) = %d, want 1", data, got)
	}
}

// TestR12MakeCommentExclusion pins the R12 closure that Make
// comment text never contributes to a python invocation count.
// The scanner must skip lines whose first non-whitespace byte is
// `#` (GNU Make's comment prefix) so comments like `# RESULT
// := $(shell python3 x.py)` do not surface as a python site.
func TestR12MakeCommentExclusion(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		{"assignment in comment is ignored",
			"# RESULT := $(shell python3 hidden.py)\nall:\n\techo ok\n", 0},
		{"!= in comment is ignored",
			"# RESULT != python3 hidden.py\nall:\n\techo ok\n", 0},
		{"recipe line is not a comment",
			"all:\n\tpython3 tool.py\n", 1},
		{"real assignment still counts",
			"RESULT := $(shell python3 x.py)\nall:\n\techo ok\n", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocationsInMakefile([]byte(tc.data))
			if got != tc.want {
				t.Errorf("CountPythonInvocationsInMakefile(%q) = %d, want %d", tc.data, got, tc.want)
			}
		})
	}
}

// TestR12MakeUnresolvedReference pins the R12 fail-closed gate
// for `$(shell X)` where X contains an unresolved `$(VAR)` in
// command position. The Make `$(PYTHON)` indirection works only
// when the variable is statically resolvable; otherwise the
// verifier surfaces a hard error.
func TestR12MakeUnresolvedReference(t *testing.T) {
	cases := []string{
		"RESULT := $(shell $(UNKNOWN_COMMAND) x.py)\nall:\n\techo ok\n",
		"RESULT != $(UNKNOWN_COMMAND) x.py\nall:\n\techo ok\n",
		"RESULT := $(shell $(call choose-python) x.py)\nall:\n\techo ok\n",
	}
	for _, data := range cases {
		t.Run(data, func(t *testing.T) {
			got := CountPythonInvocationsInMakefile([]byte(data))
			if got != -1 {
				t.Errorf("CountPythonInvocationsInMakefile(%q) = %d, want -1 (fail-closed)", data, got)
			}
		})
	}
}

// TestR12MakeResolvedReference is the positive counterpart: a
// `$(PYTHON)` reference that the resolver already knows about
// counts as one python invocation per $(shell X) site.
func TestR12MakeResolvedReference(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		{"$(PYTHON) := python3 then $(shell $(PYTHON) x.py)",
			"PYTHON := python3\nRESULT := $(shell $(PYTHON) x.py)\nall:\n\techo ok\n", 1},
		{"$(PYTHON) := python3 then $(PYTHON) hidden.py",
			"PYTHON := python3\nRESULT != $(PYTHON) hidden.py\nall:\n\techo ok\n", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocationsInMakefile([]byte(tc.data))
			if got != tc.want {
				t.Errorf("CountPythonInvocationsInMakefile(%q) = %d, want %d", tc.data, got, tc.want)
			}
		})
	}
}
