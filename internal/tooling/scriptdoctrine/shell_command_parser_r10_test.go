package scriptdoctrine

import "testing"

// TestR10Coverage pins the R10 review's additional Makefile / shell
// / workflow cases. Each entry exercises a real-world false-green
// pattern the R9 implementation still returned zero for, or a
// false-positive that must not regress.
//
// The extraction API differs by source: Makefiles use the
// Makefile extractor, workflows use the YAML run-block extractor,
// and shell scripts use the direct shell walker. Mixing the APIs
// would produce parse errors unrelated to the underlying
// classification logic.
func TestR10Coverage_Makefile(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		// .RECIPEPREFIX = > (single-char value, no leading
		// space). Make's official example: the recipe line is
		// `> python3 ...`.
		{".RECIPEPREFIX = > with > prefix", ".RECIPEPREFIX = >\n>python3 hidden.py", 1},
		// Same-line recipe (`target: ; cmd`). The semicolon is
		// required; without it the line contains prerequisites,
		// not a recipe.
		{"same-line recipe (with semicolon)", "build: ; python3 tool.py", 1},
		// Same-line recipe with prerequisites, NOT a recipe.
		{"same-line with prereq (no semicolon)", "build: python3.py", 0},
		// (shell ...) in a variable assignment: this is
		// make-time execution, fail-closed.
		{"$(shell ...) in assignment",
			"RESULT := $(shell python3 hidden.py)\nall:\n\techo $$RESULT", 0},
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

func TestR10Coverage_Shell(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		// `command --` end-of-options: drop both `command` and
		// `--` so the remainder is treated as a real command.
		{"command -- python3 x.py", "command -- python3 x.py", 1},
		// sudo value flag -u + value: skip the value as well.
		{"sudo -u root python3 x.py", "sudo -u root python3 x.py", 1},
		// env value flag -u + value: skip the value.
		{"env -u NAME python3 x.py", "env -u NAME python3 x.py", 1},
		// env -i (no value): just the flag.
		{"env -i python3 x.py", "env -i python3 x.py", 1},
		// sudo with -E (no value): just the flag.
		{"sudo -E python3 x.py", "sudo -E python3 x.py", 1},
		// bash -ec (combined option): the script is still
		// `python3 x.py`.
		{"bash -ec 'python3 x.py'", "bash -ec 'python3 x.py'", 1},
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

func TestR10Coverage_YAMLWorkflow(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		// Custom shell template with {0} placeholder: the
		// command part is `python`, so 1 invocation.
		{"shell: python {0}", "run: echo hi\nshell: python {0}", 1},
		{"shell: /usr/bin/python3 {0}",
			"run: echo hi\nshell: /usr/bin/python3 {0}", 1},
		// Quoted shell template - still need to extract the
		// command part after stripping the YAML quotes.
		{"shell: \"python\"", "run: echo hi\nshell: \"python\"", 1},
		// shell: bash -> no python count.
		{"shell: bash", "run: echo hi\nshell: bash", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocationsInYAMLRunBlocks([]byte(tc.data))
			if got != tc.want {
				t.Errorf("CountPythonInvocationsInYAMLRunBlocks(%q) = %d, want %d", tc.data, got, tc.want)
			}
		})
	}
}
