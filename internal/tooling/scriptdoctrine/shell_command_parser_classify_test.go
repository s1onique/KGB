package scriptdoctrine

import "testing"

// Word classification tests live in this companion file to keep
// shell_command_parser_test.go under the LLM-friendliness hard limit
// (450 lines). They pin the helper vocabulary that the AST visitor
// uses to decide whether a given CallExpr should count as a python
// invocation site.

// TestIsPythonCommandWord pins the accepted command-word vocabulary
// after the parser rewrite. Anything outside this set must not count
// even if it ends in "python" or is part of a path.
func TestIsPythonCommandWord(t *testing.T) {
	tests := []struct {
		word string
		want bool
	}{
		{"python", true},
		{"python3", true},
		{"python3.10", true},
		{"python3.11", true},
		{"pip", true},
		{"pip3", true},
		{"pytest", true},
		{"/usr/bin/python3", true},
		{"/usr/local/bin/python3.12", true},
		{"pythonhelper", false}, // not version-suffixed
		{"pythonical", false},   // not version-suffixed
		{"pythons", false},      // not version-suffixed
		{"py", false},           // not a python interpreter
		{"pip3install", false},  // not a python interpreter
		{"pythonpath", false},   // not a python interpreter
		{"echo", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.word, func(t *testing.T) {
			if got := isPythonCommandWord(tc.word); got != tc.want {
				t.Errorf("isPythonCommandWord(%q) = %v, want %v", tc.word, got, tc.want)
			}
		})
	}
}

// TestIsCommandPrefixWord pins the recognised command-prefix
// vocabulary. Adding a new prefix requires updating both this test
// and the walker in shell_command_parser.go.
func TestIsCommandPrefixWord(t *testing.T) {
	tests := []struct {
		word string
		want bool
	}{
		{"sudo", false},
		{"env", false},
		{"/usr/bin/env", false},
		{"exec", true},
		{"command", true},
		{"echo", false},
		{"PREFIX", false}, // case sensitive
		{"", false},       // expanded -> not a prefix
	}
	for _, tc := range tests {
		t.Run(tc.word, func(t *testing.T) {
			if got := isCommandPrefixWord(tc.word); got != tc.want {
				t.Errorf("isCommandPrefixWord(%q) = %v, want %v", tc.word, got, tc.want)
			}
		})
	}
}
