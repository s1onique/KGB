package scriptdoctrine

import "testing"

// TestR9Coverage pins the R9 review's missing-signal fixes. Each
// case exercises a real-world false-green pattern the previous
// visitor silently returned zero for, or a real-world false-
// positive that must not regress.
//
// The extraction API differs by source: Makefiles use the
// Makefile extractor, workflows use the YAML AST walker, and
// shell scripts use the direct shell walker. Mixing the APIs
// would produce parse errors unrelated to the underlying
// classification logic.
//
// Workflow test snippets below are wrapped in a minimal
// `jobs.<name>.steps` scaffold so the R11 YAML AST walker can
// locate the run step.
func TestR9Coverage_Makefile(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		// `$$` literal-dollar escape.
		{"$$ literal-dollar escape", "all:\n\techo $$(python3 hidden.py)", 1},
		// Variable-indirected: `PYTHON := python3` + `$(PYTHON) ...`.
		{"PYTHON := python3 + $(PYTHON) hidden.py", "PYTHON := python3\nall:\n\t$(PYTHON) hidden.py", 1},
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

func TestR9Coverage_Shell(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		// bash -c / sh -c shell indirection.
		{"bash -c 'python3 x.py'", "bash -c 'python3 x.py'", 1},
		{"sh -c \"python3 x.py\"", "sh -c \"python3 x.py\"", 1},
		{"bash -c 'echo hi' (no python)", "bash -c 'echo hi'", 0},
		// env wrappers.
		{"env -i python3 x.py", "env -i python3 x.py", 1},
		{"env FOO=bar python3 x.py", "env FOO=bar python3 x.py", 1},
		{"env MODE=test python3 x.py", "env MODE=test python3 x.py", 1},
		{"env python3 x.py", "env python3 x.py", 1},
		// sudo wrappers.
		{"sudo -E python3 x.py", "sudo -E python3 x.py", 1},
		{"sudo python3 x.py", "sudo python3 x.py", 1},
		// command -v / -V / --help are lookups.
		{"command -v python3 (lookup)", "command -v python3", 0},
		{"command -V python3 (lookup)", "command -V python3", 0},
		{"command --help python3 (lookup)", "command --help python3", 0},
		{"command python3 x.py", "command python3 x.py", 1},
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

func TestR9Coverage_YAMLWorkflow(t *testing.T) {
	// Each snippet is wrapped in a minimal workflow scaffold so
	// the R11 YAML AST walker can locate the run step. The run
	// body content is what the line-based parser saw in R9.
	wrap := func(step string) string {
		return "jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - " + step + "\n"
	}
	cases := []struct {
		name     string
		stepYAML string
		want     int
	}{
		// Quoted inline scalar.
		{"run: \"python3 x.py\"", wrap("run: \"python3 x.py\""), 1},
		{"run: 'python3 x.py'", wrap("run: 'python3 x.py'"), 1},
		// Plain inline scalar.
		{"run: python3 x.py", wrap("run: python3 x.py"), 1},
		// Block-scalar run body.
		{"- run: |\n    python3 x.py", wrap("run: |\n          python3 x.py"), 1},
		// shell: python3 -> 1 invocation.
		{"- run: echo hi\n  shell: python3", wrap("run: echo hi\n        shell: python3"), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocationsInYAMLRunBlocks([]byte(tc.stepYAML))
			if got != tc.want {
				t.Errorf("CountPythonInvocationsInYAMLRunBlocks(%q) = %d, want %d", tc.stepYAML, got, tc.want)
			}
		})
	}
}
