// roots_test.go — Table-driven tests for project root resolution.

package roots

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProjectRoots(t *testing.T) {
	// Get actual paths for validation
	repoRoot := "/home/kgb/Projects/KGB"
	moduleRoot := "/home/kgb/Projects/KGB/tovarisch/labs/memory"

	tests := []struct {
		name                  string
		explicitRepoRoot      string
		explicitModuleRoot    string
		startDir              string
		wantRepo              string
		wantModule            string
		wantErr               bool
	}{
		{
			name:               "both explicit roots valid",
			explicitRepoRoot:   repoRoot,
			explicitModuleRoot: moduleRoot,
			wantRepo:           repoRoot,
			wantModule:         moduleRoot,
			wantErr:            false,
		},
		{
			name:             "only repo root provided",
			explicitRepoRoot: repoRoot,
			wantRepo:         repoRoot,
			wantModule:       moduleRoot,
			wantErr:          false,
		},
		{
			name:               "only module root provided",
			explicitModuleRoot: moduleRoot,
			wantRepo:           repoRoot,
			wantModule:         moduleRoot,
			wantErr:            false,
		},
		{
			name:       "mismatching explicit roots",
			explicitRepoRoot: "/tmp",
			explicitModuleRoot: moduleRoot,
			wantErr:    true,
		},
		{
			name:               "repo root missing .git",
			explicitRepoRoot:   "/tmp",
			wantErr:            true,
		},
		{
			name:               "module root missing go.mod",
			explicitModuleRoot: "/tmp",
			wantErr:            true,
		},
		{
			name:       "search from module root",
			startDir:   moduleRoot,
			wantRepo:   repoRoot,
			wantModule: moduleRoot,
			wantErr:    false,
		},
		{
			name:       "search from package directory",
			startDir:   filepath.Join(moduleRoot, "cmd", "tovarisch-memory-lab"),
			wantRepo:   repoRoot,
			wantModule: moduleRoot,
			wantErr:    false,
		},
		{
			name:       "search from repo root",
			startDir:   repoRoot,
			wantRepo:   repoRoot,
			wantModule: moduleRoot,
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
