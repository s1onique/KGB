package scriptdoctrine

import "testing"

// TestR11YAMLWorkflowShellPrecedence pins the R11.6 contract:
// for every step containing `run:`, the effective shell is
// computed as `step > job defaults > workflow defaults > bash`.
//
// Each matrix entry is a complete workflow document; the
// verifier counts exactly the python invocations dictated by the
// combined effective-shell rule.

func TestR11YAMLWorkflowShellPrecedence(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		// Step-level shell overrides the workflow default.
		{"step bash overrides workflow python",
			"defaults:\n  run:\n    shell: python\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ok\n        shell: bash\n      - run: echo still-ok\n        shell: bash\n", 0},
		// Step-level python overrides job bash.
		{"step python overrides job bash",
			"jobs:\n  build:\n    runs-on: ubuntu-latest\n    defaults:\n      run:\n        shell: bash\n    steps:\n      - run: print(\"python\")\n        shell: python -u {0}\n", 1},
		// Workflow default python applied to two run steps.
		{"workflow python applies to two steps",
			"defaults:\n  run:\n    shell: python\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: print(\"one\")\n      - run: print(\"two\")\n", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocationsInYAMLRunBlocks([]byte(tc.data))
			if got != tc.want {
				t.Errorf("CountPythonInvocationsInYAMLRunBlocks(%s) = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

// TestR11YAMLWorkflowRunScalarForms verifies that the YAML AST
// walker correctly decodes scalar run bodies in all three
// recognised forms: inline scalar, literal block scalar (`|`),
// folded block scalar (`>`).
func TestR11YAMLWorkflowRunScalarForms(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		{"quoted inline scalar (double-quoted)",
			"jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: \"python3 x.py\"\n", 1},
		{"quoted inline scalar (single-quoted)",
			"jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: 'python3 x.py'\n", 1},
		{"plain inline scalar",
			"jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: python3 x.py\n", 1},
		{"literal block scalar |",
			"jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          python3 x.py\n", 1},
		{"folded block scalar >",
			"jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: >\n          python3 x.py\n", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocationsInYAMLRunBlocks([]byte(tc.data))
			if got != tc.want {
				t.Errorf("CountPythonInvocationsInYAMLRunBlocks(%s) = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

// TestR11YAMLWorkflowShellCustomTemplates pins the R11.6
// custom-shell template contract: the first whitespace-delimited
// word is the executable, and Python-flavoured templates count
// as one invocation per affected step.
func TestR11YAMLWorkflowShellCustomTemplates(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
	}{
		{"python {0}", wrap("run: |\n          pass\n        shell: python {0}\n"), 1},
		{"python -u {0}", wrap("run: |\n          pass\n        shell: python -u {0}\n"), 1},
		{"/usr/bin/python3 {0}", wrap("run: |\n          pass\n        shell: /usr/bin/python3 {0}\n"), 1},
		{"python {0} --flag", wrap("run: |\n          pass\n        shell: python {0} --flag\n"), 1},
		// Custom template with bash as the executable -> 0.
		{"bash with options", wrap("run: |\n          echo ok\n        shell: bash -euxo pipefail {0}\n"), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocationsInYAMLRunBlocks([]byte(tc.data))
			if got != tc.want {
				t.Errorf("CountPythonInvocationsInYAMLRunBlocks(%s) = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

// TestR11YAMLWorkflowIgnoresUses pins the R11.5 / R11.6 contract
// that `uses:` steps are skipped by the verifier (no run body to
// parse).
func TestR11YAMLWorkflowIgnoresUses(t *testing.T) {
	data := "jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n      - run: echo ok\n"
	got := CountPythonInvocationsInYAMLRunBlocks([]byte(data))
	if got != 0 {
		t.Errorf("CountPythonInvocationsInYAMLRunBlocks(uses+run) = %d, want 0", got)
	}
}

// TestR11YAMLWorkflowMalformed documents the fail-closed contract
// for malformed YAML or workflows whose shell field is not a
// scalar.
func TestR11YAMLWorkflowMalformed(t *testing.T) {
	cases := []struct {
		name string
		data string
	}{
		{"malformed YAML (missing colon)",
			"jobs\n  broken\n"},
		{"shell: not a scalar",
			"jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ok\n        shell:\n          - bash\n          - -eux\n"},
		{"run: not a scalar",
			"jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run:\n          - python3\n          - x.py\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocationsInYAMLRunBlocks([]byte(tc.data))
			if got != -1 {
				t.Errorf("CountPythonInvocationsInYAMLRunBlocks(%s) = %d, want -1 (fail-closed)", tc.name, got)
			}
		})
	}
}

// wrap builds a one-job workflow scaffold around a single step
// so the R11.5 AST walker has the structure it requires.
func wrap(stepYAML string) string {
	return "jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - " + stepYAML
}
