package scriptdoctrine

import (
	"os"
	"path/filepath"
	"testing"
)

// =============================================================================
// CountLogicalLOC tests
// =============================================================================

func TestCountLogicalLOC(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int
	}{
		{
			name:     "empty file",
			content:  "",
			expected: 0,
		},
		{
			name: "only shebang and comments",
			content: `#!/bin/bash
# This is a comment
# Another comment
`,
			expected: 0,
		},
		{
			name: "shebang and code",
			content: `#!/bin/bash
echo "hello"
x=1
`,
			expected: 2,
		},
		{
			name: "with blank lines",
			content: `#!/bin/bash

echo "hello"

x=1

`,
			expected: 2,
		},
		{
			name: "comments interspersed",
			content: `#!/bin/bash
# comment
echo "hello"
# another comment
x=1
`,
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "test.sh")
			if err := os.WriteFile(tmpFile, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write temp file: %v", err)
			}

			got := CountLogicalLOC(tmpFile)
			if got != tt.expected {
				t.Errorf("CountLogicalLOC() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestCountLogicalLOCReadError(t *testing.T) {
	got := CountLogicalLOC(filepath.Join(t.TempDir(), "does-not-exist.sh"))
	if got != -1 {
		t.Errorf("CountLogicalLOC on missing file = %d, want -1", got)
	}
}

func TestLOCBoundary(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("50 LOC passes", func(t *testing.T) {
		content := "#!/bin/bash\n"
		for i := 0; i < 50; i++ {
			content += "echo $((1))\n"
		}
		tmpFile := filepath.Join(tmpDir, "pass50.sh")
		if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		got := CountLogicalLOC(tmpFile)
		if got != 50 {
			t.Errorf("CountLogicalLOC() = %d, want 50", got)
		}
		if got > MaxShellLOC {
			t.Errorf("50 LOC script should not exceed max %d", MaxShellLOC)
		}
	})

	t.Run("51 LOC fails", func(t *testing.T) {
		content := "#!/bin/bash\n"
		for i := 0; i < 51; i++ {
			content += "echo $((1))\n"
		}
		tmpFile := filepath.Join(tmpDir, "fail51.sh")
		if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		got := CountLogicalLOC(tmpFile)
		if got != 51 {
			t.Errorf("CountLogicalLOC() = %d, want 51", got)
		}
		if got <= MaxShellLOC {
			t.Errorf("51 LOC script should exceed max %d", MaxShellLOC)
		}
	})
}

// =============================================================================
// HasPythonShebang tests (now error-returning)
// =============================================================================

func TestHasPythonShebang(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{name: "bash shebang", content: "#!/bin/bash\necho hello\n", expected: false},
		{name: "python3 shebang", content: "#!/usr/bin/env python3\nprint('hello')\n", expected: true},
		{name: "python shebang", content: "#!/usr/bin/python\nprint('hello')\n", expected: true},
		{name: "no shebang", content: "echo hello\n", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := filepath.Join(tmpDir, "test.sh")
			if err := os.WriteFile(tmpFile, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write temp file: %v", err)
			}

			got, err := HasPythonShebang(tmpFile)
			if err != nil {
				t.Fatalf("HasPythonShebang error = %v", err)
			}
			if got != tt.expected {
				t.Errorf("HasPythonShebang() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestHasPythonShebangUnreadable enforces fail-closed: a read failure must
// surface as an error, never as "false".
func TestHasPythonShebangUnreadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-file.sh")
	_, err := HasPythonShebang(path)
	if err == nil {
		t.Errorf("HasPythonShebang on missing file should return error, got nil")
	}
}

// =============================================================================
// CountPythonInvocations tests (R6: executable command sites)
// =============================================================================

// TestCountPythonInvocationsCommandSites covers the R6 metric:
// executable command sites, not lines.
func TestCountPythonInvocationsCommandSites(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		// R6 mandated cases.
		{"two commands on one line via ;", "python3 a.py; python3 b.py", 2},
		{"and-or chain with two python sites", "true && python3 a.py || python3 b.py", 2},
		{"echo then python", "echo ok; python3 a.py", 1},
		{"printf with substitution", `printf '%s' "$(python3 a.py)"`, 1},
		{"grep then python", "grep x file && python3 a.py", 1},
		{"echo python as argument", `echo "python3 a.py"`, 0},
		{"command -v lookup", "command -v python3", 0},
		{"command --version lookup", "command --version python3", 0},

		// Basic invocations (must still work after parser rewrite).
		{"direct python3", "python3 script.py", 1},
		{"direct python", "python script.py", 1},
		{"direct pip", "pip install foo", 1},
		{"direct pytest", "pytest tests/", 1},
		{"tab indented recipe", "\tpython3 script.py", 1},
		{"sudo prefix", "sudo python3 script.py", 1},
		{"env prefix", "env python3 script.py", 1},
		{"exec prefix", "exec python3 script.py", 1},
		{"/usr/bin/env prefix", "/usr/bin/env python3 script.py", 1},
		{"absolute path", "/usr/bin/python3 script.py", 1},
		{"backtick substitution", "X=`python3 -c 'print(1)'`", 1},

		// Comments and non-executing references.
		{"comment only line", "# python3 script.py", 0},
		{"echo python arg no separator", `echo "use python3"`, 0},
		{"pure variable assignment", "PY=python3", 0},
		{"quoted value", `MY_VAR="use python3"`, 0},

		// Shebang.
		{"python shebang only", "#!/usr/bin/env python3", 1},

		// Multi-line.
		{"three distinct lines", "python3 a.py\npython3 b.py\npython3 c.py\n", 3},
		{"comments and invocations", "# lead\npython3 a.py\n# mid\npython3 b.py\n", 2},

		// Edge cases.
		{"empty", "", 0},
		{"blank lines", "\n\n\n", 0},
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

// TestCountPythonInvocationsDeduplicatesOverlappingPatterns pins the
// invariant that the metric counts command sites, not regex hits.
func TestCountPythonInvocationsDeduplicatesOverlappingPatterns(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    int
	}{
		{"sudo python3 single command", "sudo python3 script.py", 1},
		{"/usr/bin/env python3 single command", "/usr/bin/env python3 script.py", 1},
		{"shebang counted once", "#!/usr/bin/env python3", 1},
		{"two distinct lines", "python3 a.py\npython3 b.py", 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocations([]byte(tc.content))
			if got != tc.want {
				t.Errorf("CountPythonInvocations = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestCountPythonInvocationsIgnoresCommentsAndOutputCommands pins that
// comments, documentation, and argument references never count.
func TestCountPythonInvocationsIgnoresCommentsAndOutputCommands(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    int
	}{
		{"comment line", "# python3 script.py", 0},
		{"echo with python as arg", `echo "python3 script.py"`, 0},
		{"printf with python as arg", `printf 'use python3 script.py\n'`, 0},
		{"inline trailing comment", "echo hello # python3 ignored", 0},
		{"documentation string", `MY_VAR="Use python3 for setup"`, 0},
		{"variable assignment", "PY=python3", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountPythonInvocations([]byte(tc.content))
			if got != tc.want {
				t.Errorf("CountPythonInvocations = %d, want %d for %q", got, tc.want, tc.content)
			}
		})
	}
}

// =============================================================================
// Parser unit tests
// =============================================================================

func TestSplitShellList(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"single command", "echo hi", []string{"echo hi"}},
		{"two commands via ;", "echo a; echo b", []string{"echo a", "echo b"}},
		{"two commands via &&", "true && python3 a.py", []string{"true", "python3 a.py"}},
		{"two commands via ||", "false || python3 a.py", []string{"false", "python3 a.py"}},
		{"pipe", "echo data | python3 -", []string{"echo data", "python3 -"}},
		{"quoted semicolon", `echo "a;b"`, []string{`echo "a;b"`}},
		// Note: the paren grouping test does not invoke the substitution
		// extraction; it only verifies that the splitter treats the
		// entire parenthesised region as one operand. The substitution
		// content is counted by countInCommandList via
		// extractAndCountSubstitutions, separately.
		{"paren grouping not split", "echo $(python3 -c 'print(1)')", []string{"echo $(python3 -c 'print(1)')"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitShellList(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("splitShellList(%q) = %v (len %d), want %v (len %d)", tc.in, got, len(got), tc.want, len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("splitShellList[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
