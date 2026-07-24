// roots.go — Canonical project root resolver for CORRECTION25/CORRECTION26.
//
// Defines distinct contracts for repository root and memory-module root.
// Used by TestMain (buildOnce) and live-smoke provenance setup.
//
// Key improvements over CORRECTION24:
// - Hermetic tests using temporary fixtures
// - Proper symlink resolution via filepath.EvalSymlinks
// - Git worktree support (accepts .git as gitfile)
// - Proper go.mod parsing via modfile.ModulePath
// - Improved module discovery in multiple locations

package roots

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

// ProjectRoots holds the resolved repository and module paths.
type ProjectRoots struct {
	Repository string // absolute path to repository root (.git parent)
	Module    string // absolute path to memory module root (go.mod parent)
}

// ResolveProjectRoots resolves both roots from explicit environment
// variables or by searching upward from a start directory.
//
// Resolution order:
//  1. Validate both explicit roots when both present
//  2. Derive module root from explicit repository root
//  3. Derive repository root from explicit module root
//  4. Search upward from start directory
//  5. Fail closed on invariant violations
func ResolveProjectRoots(
	explicitRepoRoot, explicitModuleRoot, startDir string,
) (ProjectRoots, error) {
	var repoRoot, moduleRoot string

	// Case: both explicitly provided
	if explicitRepoRoot != "" && explicitModuleRoot != "" {
		repoRoot, err := canonicalExistingPath(explicitRepoRoot)
		if err != nil {
			return ProjectRoots{}, fmt.Errorf("repo root canonical: %w", err)
		}
		moduleRoot, err := canonicalExistingPath(explicitModuleRoot)
		if err != nil {
			return ProjectRoots{}, fmt.Errorf("module root canonical: %w", err)
		}
		if err := validateRoots(repoRoot, moduleRoot); err != nil {
			return ProjectRoots{}, fmt.Errorf("explicit roots mismatch: %w", err)
		}
		return ProjectRoots{Repository: repoRoot, Module: moduleRoot}, nil
	}

	// Case: only repository root provided
	if explicitRepoRoot != "" {
		var err error
		repoRoot, err = canonicalExistingPath(explicitRepoRoot)
		if err != nil {
			return ProjectRoots{}, fmt.Errorf("repo root canonical: %w", err)
		}
		if err := validateRepoRoot(repoRoot); err != nil {
			return ProjectRoots{}, fmt.Errorf("invalid repo root: %w", err)
		}
		moduleRoot = filepath.Join(repoRoot, "tovarisch", "labs", "memory")
		moduleRoot, err = canonicalExistingPath(moduleRoot)
		if err != nil {
			return ProjectRoots{}, fmt.Errorf("derived module root canonical: %w", err)
		}
		if err := validateModuleRoot(moduleRoot); err != nil {
			return ProjectRoots{}, fmt.Errorf("derived module root invalid: %w", err)
		}
		return ProjectRoots{Repository: repoRoot, Module: moduleRoot}, nil
	}

	// Case: only module root provided
	if explicitModuleRoot != "" {
		var err error
		moduleRoot, err = canonicalExistingPath(explicitModuleRoot)
		if err != nil {
			return ProjectRoots{}, fmt.Errorf("module root canonical: %w", err)
		}
		if err := validateModuleRoot(moduleRoot); err != nil {
			return ProjectRoots{}, fmt.Errorf("invalid module root: %w", err)
		}
		// Search upward from module root to find .git
		repoRoot, err = findRepoRoot(moduleRoot)
		if err != nil {
			return ProjectRoots{}, fmt.Errorf("repo root not found: %w", err)
		}
		return ProjectRoots{Repository: repoRoot, Module: moduleRoot}, nil
	}

	// Case: search upward from start directory
	if startDir != "" {
		startDir, err := canonicalExistingPath(startDir)
		if err != nil {
			return ProjectRoots{}, fmt.Errorf("start dir canonical: %w", err)
		}
		moduleRoot, err = findModuleRoot(startDir)
		if err != nil {
			return ProjectRoots{}, fmt.Errorf("module root not found: %w", err)
		}
		repoRoot, err = findRepoRoot(moduleRoot)
		if err != nil {
			return ProjectRoots{}, fmt.Errorf("repo root not found: %w", err)
		}
		return ProjectRoots{Repository: repoRoot, Module: moduleRoot}, nil
	}

	return ProjectRoots{}, fmt.Errorf("no root information provided")
}

// canonicalExistingPath returns the canonical absolute path, resolving symlinks.
func canonicalExistingPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("filepath.Abs: %w", err)
	}
	canon, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("path does not exist: %s", abs)
		}
		return "", fmt.Errorf("filepath.EvalSymlinks: %w", err)
	}
	return filepath.Clean(canon), nil
}

// validateRoots checks consistency of both explicit roots.
func validateRoots(repoRoot, moduleRoot string) error {
	if err := validateRepoRoot(repoRoot); err != nil {
		return err
	}
	if err := validateModuleRoot(moduleRoot); err != nil {
		return err
	}
	// Module must be under repository
	if !strings.HasPrefix(moduleRoot, repoRoot) {
		return fmt.Errorf("module root %q is not under repo root %q", moduleRoot, repoRoot)
	}
	expectedModule := filepath.Join(repoRoot, "tovarisch", "labs", "memory")
	if filepath.Clean(moduleRoot) != filepath.Clean(expectedModule) {
		return fmt.Errorf("module root %q is not at expected path %q", moduleRoot, expectedModule)
	}
	return nil
}

// validateRepoRoot checks that the path contains .git (directory or gitfile).
func validateRepoRoot(repoRoot string) error {
	if repoRoot == "" || repoRoot == "/" {
		return fmt.Errorf("repo root is empty or root")
	}
	gitPath := filepath.Join(repoRoot, ".git")
	st, err := os.Stat(gitPath)
	if err == nil && st.IsDir() {
		return nil // .git is a directory
	}
	// Check if .git is a gitfile (worktree case)
	data, err := os.ReadFile(gitPath)
	if err == nil {
		line := strings.TrimSpace(string(data))
		if strings.HasPrefix(line, "gitdir: ") {
			// It's a gitfile pointing to the actual git directory
			return nil
		}
	}
	return fmt.Errorf("repo root %q does not contain valid .git", repoRoot)
}

// validateModuleRoot checks that the path contains go.mod
// with the expected module declaration.
func validateModuleRoot(moduleRoot string) error {
	if moduleRoot == "" || moduleRoot == "/" {
		return fmt.Errorf("module root is empty or root")
	}
	goModPath := filepath.Join(moduleRoot, "go.mod")
	st, err := os.Stat(goModPath)
	if err != nil {
		return fmt.Errorf("module root %q does not contain go.mod: %w", moduleRoot, err)
	}
	if st.IsDir() {
		return fmt.Errorf("module root %q/go.mod is a directory", moduleRoot)
	}
	// Parse go.mod and verify module declaration
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return fmt.Errorf("cannot read go.mod: %w", err)
	}
	modulePath := modfile.ModulePath(data)
	expectedModule := "github.com/s1onique/KGB/tovarisch/labs/memory"
	if modulePath != expectedModule {
		return fmt.Errorf("go.mod declares %q, expected %q", modulePath, expectedModule)
	}
	return nil
}

// findRepoRoot searches upward from start for .git.
func findRepoRoot(start string) (string, error) {
	dir := start
	for {
		if dir == "/" || dir == "." || dir == "" {
			break
		}
		gitPath := filepath.Join(dir, ".git")
		st, err := os.Stat(gitPath)
		if err == nil && st.IsDir() {
			return dir, nil
		}
		// Also check for gitfile
		if err == nil || os.IsNotExist(err) {
			data, err := os.ReadFile(gitPath)
			if err == nil {
				line := strings.TrimSpace(string(data))
				if strings.HasPrefix(line, "gitdir: ") {
					return dir, nil
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no .git found from %q", start)
}

// findModuleRoot searches upward from start for the memory module.
// Checks multiple candidate locations.
func findModuleRoot(start string) (string, error) {
	dir := start
	for {
		if dir == "/" || dir == "." || dir == "" {
			break
		}
		// Check direct go.mod
		goModPath := filepath.Join(dir, "go.mod")
		if st, err := os.Stat(goModPath); err == nil && !st.IsDir() {
			// Verify it's the right module
			if data, err := os.ReadFile(goModPath); err == nil {
				modulePath := modfile.ModulePath(data)
				if modulePath == "github.com/s1onique/KGB/tovarisch/labs/memory" {
					return dir, nil
				}
			}
		}
		// Check tovarisch/labs/memory/go.mod
		candidatePath := filepath.Join(dir, "tovarisch", "labs", "memory", "go.mod")
		if st, err := os.Stat(candidatePath); err == nil && !st.IsDir() {
			// Verify it's the right module
			if data, err := os.ReadFile(candidatePath); err == nil {
				modulePath := modfile.ModulePath(data)
				if modulePath == "github.com/s1onique/KGB/tovarisch/labs/memory" {
					return filepath.Join(dir, "tovarisch", "labs", "memory"), nil
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("memory module not found from %q", start)
}

// PackagePath returns the absolute path to a package within the module.
func (r ProjectRoots) PackagePath(relativePath string) string {
	return filepath.Join(r.Module, relativePath)
}

// Helper to parse go.mod from bytes (for testing)
func parseGoModPath(data []byte) (string, error) {
	// Handle if data has trailing newline
	trimmed := bytes.TrimSuffix(data, []byte("\n"))
	modulePath := modfile.ModulePath(trimmed)
	return modulePath, nil
}
