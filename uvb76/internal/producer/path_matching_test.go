package producer

import "testing"

func TestPathMatching_Exact(t *testing.T) {
	if !PathMatchesPattern("uvb76/foo.json", "uvb76/foo.json") {
		t.Error("exact path should match")
	}
	if PathMatchesPattern("uvb76/foo.json.backup", "uvb76/foo.json") {
		t.Error("substring 'uvb76/foo.json' must not match 'uvb76/foo.json.backup'")
	}
}

// TestPathMatching_Glob verifies single-segment glob.
func TestPathMatching_Glob(t *testing.T) {
	if !PathMatchesPattern("uvb76/cmd/abc/main.go", "uvb76/cmd/*/main.go") {
		t.Error("single-level glob should match")
	}
	if PathMatchesPattern("uvb76/cmd/abc/sub/main.go", "uvb76/cmd/*/main.go") {
		t.Error("single-level glob must not span slashes")
	}
}

// TestPathMatching_Recursive verifies recursive glob.
func TestPathMatching_Recursive(t *testing.T) {
	if !PathMatchesPattern("uvb76/cmd/abc/sub/main.go", "uvb76/cmd/**/main.go") {
		t.Error("recursive glob should match nested")
	}
	if !PathMatchesPattern("uvb76/cmd/abc/main.go", "uvb76/cmd/**/*") {
		t.Error("recursive **/* should match direct")
	}
}

// TestPathMatching_RejectsAbsolute rejects absolute paths.
func TestPathMatching_RejectsAbsolute(t *testing.T) {
	if _, err := CompileInventoryPattern("/abs/path"); err == nil {
		t.Error("absolute path should be rejected")
	}
}

// TestPathMatching_RejectsEscape rejects parent-directory escape.
func TestPathMatching_RejectsEscape(t *testing.T) {
	if _, err := CompileInventoryPattern("uvb76/../../etc/passwd"); err == nil {
		t.Error("parent-directory escape should be rejected")
	}
}

// TestCompile_RoundTripCheck verifies the compiled kind.
func TestCompile_RoundTripCheck(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"uvb76/foo.json", "exact"},
		{"uvb76/cmd/*/main.go", "glob"},
		{"uvb76/cmd/**/*.json", "recursive"},
	}
	for _, tc := range cases {
		c, err := CompileInventoryPattern(tc.in)
		if err != nil {
			t.Errorf("compile %s: %v", tc.in, err)
			continue
		}
		if c.Kind != tc.want {
			t.Errorf("kind for %s: got %s, want %s", tc.in, c.Kind, tc.want)
		}
	}
}

// TestBypassDetector_AllowlistIsRespected checks allowlist semantics.
