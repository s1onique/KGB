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
	// Nonexistent file -> -1 (fail-closed).
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
// HasPythonShebang tests
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
		{name: "empty file", content: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := filepath.Join(tmpDir, "test.sh")
			if err := os.WriteFile(tmpFile, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write temp file: %v", err)
			}

			got := HasPythonShebang(tmpFile)
			if got != tt.expected {
				t.Errorf("HasPythonShebang() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// =============================================================================
// CountPythonInvocations tests
// =============================================================================

func TestCountPythonInvocationsExactCounts(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{"direct python3", "python3 script.py\n", 1},
		{"direct python", "python script.py\n", 1},
		{"direct pip", "pip install foo\n", 1},
		{"direct pytest", "pytest tests/\n", 1},
		{"tab indented recipe", "\tpython3 script.py\n", 1},
		{"sudo prefix", "sudo python3 script.py\n", 1},
		{"env prefix", "env python3 script.py\n", 1},
		{"exec prefix", "exec python3 script.py\n", 1},
		{"command prefix", "command python3 script.py\n", 1},
		{"/usr/bin/env prefix", "/usr/bin/env python3 script.py\n", 1},
		{"absolute path", "/usr/bin/python3 script.py\n", 1},
		{"var assign with cmd substitution", "X=$(python3 -c 'print(1)')\n", 1},
		{"var assign with backtick", "X=`python3 -c 'print(1)'`\n", 1},
		{"comment only", "# python3 script.py\n", 0},
		{"leading inline comment", "echo hello # python3 ignored\n", 0},
		{"echo argument", "echo \"python3 script.py\"\n", 0},
		{"printf argument", "printf 'use python3 script.py\\n'\n", 0},
		{"pure variable assignment", "PY=python3\n", 0},
		{"quoted value", "MY_VAR=\"use python3\"\n", 0},
		{"multiple invocations same line", "python3 a.py; python3 b.py\n", 1},
		{"python shebang only", "#!/usr/bin/env python3\n", 1},
		{"python shebang with body", "#!/usr/bin/env python3\nprint('hi')\n", 1},
		{"three distinct lines", "python3 a.py\npython3 b.py\npython3 c.py\n", 3},
		{"comments and invocations", "# lead\npython3 a.py\n# mid\npython3 b.py\n", 2},
		{"empty", "", 0},
		{"comment only file", "# comment\n# another\n", 0},
		{"blank lines", "\n\n\n", 0},
		{"and-or chain", "true && python3 a.py || python3 b.py\n", 1},
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

func TestCountPythonInvocationsDeduplicatesOverlappingPatterns(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    int
	}{
		{"sudo python3 was counted twice before", "sudo python3 script.py\n", 1},
		{"/usr/bin/env python3 was miscounted", "/usr/bin/env python3 script.py\n", 1},
		{"shebang counted once", "#!/usr/bin/env python3\n", 1},
		{"two distinct lines", "python3 a.py\npython3 b.py\n", 2},
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

func TestCountPythonInvocationsIgnoresCommentsAndOutputCommands(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    int
	}{
		{"comment line", "# python3 script.py\n", 0},
		{"echo with python as arg", "echo \"python3 script.py\"\n", 0},
		{"printf with python as arg", "printf 'use python3 script.py\\n'\n", 0},
		{"inline trailing comment", "echo hello # python3 ignored\n", 0},
		{"documentation string", "MY_VAR=\"Use python3 for setup\"\n", 0},
		{"variable assignment", "PY=python3\n", 0},
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
