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
