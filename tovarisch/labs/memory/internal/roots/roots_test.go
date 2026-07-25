// roots_test.go — Hermetic table-driven tests for project root resolution.
//
// Tests use temporary fixtures and do not depend on the developer environment.
// Integration tests for the real repository are marked with Integration prefix.

package roots

import (
	"os"
	"path/filepath"
	"testing"
)

const expectedModule = "github.com/s1onique/KGB/tovarisch/labs/memory"

// tempFixture creates a temporary repository structure for testing.
type tempFixture struct {
	repoRoot   string
	moduleRoot string
	cleanupFn  func()
}

func newTempFixture(t *testing.T) *tempFixture {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "roots-test-*")
	if err != nil {
		t.Fatal(err)
	}
	f := &tempFixture{
		repoRoot:   filepath.Join(tmpDir, "repo"),
		moduleRoot: filepath.Join(tmpDir, "repo", "tovarisch", "labs", "memory"),
		cleanupFn:  func() { os.RemoveAll(tmpDir) },
	}
	// Create directory structure
	if err := os.MkdirAll(f.moduleRoot, 0755); err != nil {
		f.cleanupFn()
		t.Fatal(err)
	}
	// Create .git directory
	if err := os.MkdirAll(filepath.Join(f.repoRoot, ".git"), 0755); err != nil {
		f.cleanupFn()
		t.Fatal(err)
	}
	// Create go.mod with expected module declaration
	goModContent := "module " + expectedModule + "\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(f.moduleRoot, "go.mod"), []byte(goModContent), 0644); err != nil {
		f.cleanupFn()
		t.Fatal(err)
	}
	// Create package directory for search tests
	if err := os.MkdirAll(filepath.Join(f.moduleRoot, "cmd", "tovarisch-memory-lab"), 0755); err != nil {
		f.cleanupFn()
		t.Fatal(err)
	}
	return f
}

func TestResolveProjectRoots_Hermetic(t *testing.T) {
	fixture := newTempFixture(t)
	defer fixture.cleanupFn()

	tests := []struct {
		name               string
		explicitRepoRoot   string
		explicitModuleRoot string
		startDir           string
		wantRepo           string
		wantModule         string
		wantErr            bool
	}{
		{
			name:               "both explicit roots valid",
			explicitRepoRoot:   fixture.repoRoot,
			explicitModuleRoot: fixture.moduleRoot,
			wantRepo:           fixture.repoRoot,
			wantModule:         fixture.moduleRoot,
			wantErr:            false,
		},
		{
			name:             "only repo root provided",
			explicitRepoRoot: fixture.repoRoot,
			wantRepo:         fixture.repoRoot,
			wantModule:       fixture.moduleRoot,
			wantErr:          false,
		},
		{
			name:               "only module root provided",
			explicitModuleRoot: fixture.moduleRoot,
			wantRepo:           fixture.repoRoot,
			wantModule:         fixture.moduleRoot,
			wantErr:            false,
		},
		{
			name:               "mismatching explicit roots",
			explicitRepoRoot:   "/tmp/nonexistent",
			explicitModuleRoot: fixture.moduleRoot,
			wantErr:            true,
		},
		{
			name:             "repo root missing .git",
			explicitRepoRoot: "/tmp",
			wantErr:          true,
		},
		{
			name:               "module root missing go.mod",
			explicitModuleRoot: "/tmp",
			wantErr:            true,
		},
		{
			name:       "search from module root",
			startDir:   fixture.moduleRoot,
			wantRepo:   fixture.repoRoot,
			wantModule: fixture.moduleRoot,
			wantErr:    false,
		},
		{
			name:       "search from package directory",
			startDir:   filepath.Join(fixture.moduleRoot, "cmd", "tovarisch-memory-lab"),
			wantRepo:   fixture.repoRoot,
			wantModule: fixture.moduleRoot,
			wantErr:    false,
		},
		{
			name:       "search from repo root",
			startDir:   fixture.repoRoot,
			wantRepo:   fixture.repoRoot,
			wantModule: fixture.moduleRoot,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveProjectRoots(tt.explicitRepoRoot, tt.explicitModuleRoot, tt.startDir)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Repository != tt.wantRepo {
				t.Errorf("Repository = %q, want %q", got.Repository, tt.wantRepo)
			}
			if got.Module != tt.wantModule {
				t.Errorf("Module = %q, want %q", got.Module, tt.wantModule)
			}
		})
	}
}

func TestResolveProjectRoots_TempDirectory(t *testing.T) {
	// Test that search from unrelated temp directory fails
	tmpDir, err := os.MkdirTemp("", "roots-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	_, err = ResolveProjectRoots("", "", tmpDir)
	if err == nil {
		t.Error("expected error when searching from unrelated temp directory")
	}
}

func TestProjectRoots_PackagePath(t *testing.T) {
	roots := ProjectRoots{
		Repository: "/repo",
		Module:     "/repo/tovarisch/labs/memory",
	}

	tests := []struct {
		relPath string
		want    string
	}{
		{"cmd/tovarisch-memory-lab", "/repo/tovarisch/labs/memory/cmd/tovarisch-memory-lab"},
		{"internal/evidence", "/repo/tovarisch/labs/memory/internal/evidence"},
	}

	for _, tt := range tests {
		got := roots.PackagePath(tt.relPath)
		if got != tt.want {
			t.Errorf("PackagePath(%q) = %q, want %q", tt.relPath, got, tt.want)
		}
	}
}

func TestCanonicalExistingPath(t *testing.T) {
	// Test with temp fixture
	tmpDir, err := os.MkdirTemp("", "canonical-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Test absolute path
	got, err := canonicalExistingPath(tmpDir)
	if err != nil {
		t.Fatalf("canonicalExistingPath(%q) error: %v", tmpDir, err)
	}
	if got != tmpDir {
		t.Errorf("canonicalExistingPath(%q) = %q, want %q", tmpDir, got, tmpDir)
	}

	// Test nonexistent path
	_, err = canonicalExistingPath(filepath.Join(tmpDir, "nonexistent"))
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestValidateModuleRoot_GoModParsing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gomod-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Test valid go.mod with correct module
	validGoMod := "module " + expectedModule + "\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(validGoMod), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateModuleRoot(tmpDir); err != nil {
		t.Errorf("expected no error for valid go.mod, got: %v", err)
	}

	// Test wrong module path
	wrongGoMod := "module github.com/wrong/module\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(wrongGoMod), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateModuleRoot(tmpDir); err == nil {
		t.Error("expected error for wrong module path")
	}

	// Test module path in comment only
	commentOnlyGoMod := "// module " + expectedModule + "\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(commentOnlyGoMod), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateModuleRoot(tmpDir); err == nil {
		t.Error("expected error when module only in comment")
	}

	// Test missing module directive
	noModuleGoMod := "go 1.21\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(noModuleGoMod), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateModuleRoot(tmpDir); err == nil {
		t.Error("expected error when module directive missing")
	}
}

func TestValidateRepoRoot_Gitfile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gitfile-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Test valid .git directory
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := validateRepoRoot(tmpDir); err != nil {
		t.Errorf("expected no error for valid .git directory, got: %v", err)
	}

	// Remove .git directory and create gitfile
	if err := os.RemoveAll(gitDir); err != nil {
		t.Fatal(err)
	}
	gitfileContent := "gitdir: /some/path/to/actual/.git\n"
	if err := os.WriteFile(gitDir, []byte(gitfileContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateRepoRoot(tmpDir); err != nil {
		t.Errorf("expected no error for valid gitfile, got: %v", err)
	}
}

func TestSymlinkResolution(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "symlink-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create actual directory
	realDir := filepath.Join(tmpDir, "real")
	if err := os.MkdirAll(filepath.Join(realDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create symlink
	symlinkDir := filepath.Join(tmpDir, "link")
	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Fatal(err)
	}

	// Resolve through symlink should give canonical path
	canonicalReal, err := canonicalExistingPath(realDir)
	if err != nil {
		t.Fatal(err)
	}
	canonicalSymlink, err := canonicalExistingPath(symlinkDir)
	if err != nil {
		t.Fatal(err)
	}
	if canonicalReal != canonicalSymlink {
		t.Errorf("symlink canonical path %q != real canonical path %q", canonicalSymlink, canonicalReal)
	}

	// Broken symlink
	brokenLink := filepath.Join(tmpDir, "broken")
	if err := os.Symlink("/nonexistent/path", brokenLink); err != nil {
		t.Fatal(err)
	}
	_, err = canonicalExistingPath(brokenLink)
	if err == nil {
		t.Error("expected error for broken symlink")
	}
}

func TestFindModuleRoot_DirectGoMod(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "direct-gomod-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create go.mod directly in tmpDir
	goModContent := "module " + expectedModule + "\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goModContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Should find module at tmpDir
	got, err := findModuleRoot(tmpDir)
	if err != nil {
		t.Fatalf("findModuleRoot(%q) error: %v", tmpDir, err)
	}
	if got != tmpDir {
		t.Errorf("findModuleRoot(%q) = %q, want %q", tmpDir, got, tmpDir)
	}
}

// TestIntegration_RealRepository tests against the real repository.
// This test is skipped if not run from within a valid KGB repository.
func TestIntegration_RealRepository(t *testing.T) {
	if os.Getenv("TOVARISCH_INTEGRATION_TEST") != "1" {
		t.Skip("skipping integration test (not in integration mode)")
	}

	// Test with actual repository paths
	repoRoot := os.Getenv("TOVARISCH_REPO_ROOT")
	moduleRoot := os.Getenv("TOVARISCH_MEMORY_MODULE_ROOT")

	if repoRoot == "" || moduleRoot == "" {
		t.Skip("TOVARISCH_REPO_ROOT or TOVARISCH_MEMORY_MODULE_ROOT not set")
	}

	got, err := ResolveProjectRoots(repoRoot, moduleRoot, "")
	if err != nil {
		t.Fatalf("ResolveProjectRoots error: %v", err)
	}
	if got.Repository != repoRoot {
		t.Errorf("Repository = %q, want %q", got.Repository, repoRoot)
	}
	if got.Module != moduleRoot {
		t.Errorf("Module = %q, want %q", got.Module, moduleRoot)
	}
}
