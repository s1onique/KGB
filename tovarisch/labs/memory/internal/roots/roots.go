// roots.go — Canonical project root resolver for CORRECTION25.
//
// Defines distinct contracts for repository root and memory-module root.
// Used by TestMain (buildOnce) and live-smoke provenance setup.

package roots

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
//  1. Validate both explicit roots when both are present
//  2. Derive module root from explicit repository root
//  3. Derive repository root from explicit module root
//  4. Search upward from start directory
//  5. Fail closed when invariants are violated
func ResolveProjectRoots(
	explicitRepoRoot, explicitModuleRoot, startDir string,
) (ProjectRoots, error) {
	var repoRoot, moduleRoot string

	// Case: both explicitly provided
	if explicitRepoRoot != "" && explicitModuleRoot != "" {
		repoRoot = mustAbs(explicitRepoRoot)
		moduleRoot = mustAbs(explicitModuleRoot)
		if err := validateRoots(repoRoot, moduleRoot); err != nil {
			return ProjectRoots{}, fmt.Errorf("explicit roots mismatch: %w", err)
		}
		return ProjectRoots{Repository: repoRoot, Module: moduleRoot}, nil
	}

	// Case: only repository root provided
	if explicitRepoRoot != "" {
		repoRoot = mustAbs(explicitRepoRoot)
		if err := validateRepoRoot(repoRoot); err != nil {
			return ProjectRoots{}, fmt.Errorf("invalid repo root: %w", err)
		}
		moduleRoot = filepath.Join(repoRoot, "tovarisch", "labs", "memory")
		if err := validateModuleRoot(moduleRoot); err != nil {
			return ProjectRoots{}, fmt.Errorf("derived module root invalid: %w", err)
		}
		return ProjectRoots{Repository: repoRoot, Module: moduleRoot}, nil
	}

	// Case: only module root provided
	if explicitModuleRoot != "" {
		moduleRoot = mustAbs(explicitModuleRoot)
		if err := validateModuleRoot(moduleRoot); err != nil {
			return ProjectRoots{}, fmt.Errorf("invalid module root: %w", err)
		}
		// Search upward from module root to find .git
		repoRoot, err := findRepoRoot(moduleRoot)
		if err != nil {
			return ProjectRoots{}, fmt.Errorf("repo root not found: %w", err)
		}
		return ProjectRoots{Repository: repoRoot, Module: moduleRoot}, nil
	}

	// Case: search upward from start directory
	if startDir != "" {
		startDir = mustAbs(startDir)
		moduleRoot, err := findModuleRoot(startDir)
		if err != nil {
			return ProjectRoots{}, fmt.Errorf("module root not found: %w", err)
		}
		repoRoot, err := findRepoRoot(moduleRoot)
		if err != nil {
			return ProjectRoots{}, fmt.Errorf("repo root not found: %w", err)
		}
		return ProjectRoots{Repository: repoRoot, Module: moduleRoot}, nil
	}

	return ProjectRoots{}, fmt.Errorf("no root information provided")
}

// mustAbs returns the absolute path, resolving symlinks.
func mustAbs(p string) string {
	abs, _ := filepath.Abs(p)
	return abs
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

// validateRepoRoot checks that the path contains .git.
func validateRepoRoot(repoRoot string) error {
	if repoRoot == "" || repoRoot == "/" {
		return fmt.Errorf("repo root is empty or root")
	}
	gitPath := filepath.Join(repoRoot, ".git")
	st, err := os.Stat(gitPath)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("repo root %q does not contain .git", repoRoot)
	}
	return nil
}

// validateModuleRoot checks that the path contains go.mod
// with the expected module declaration.
func validateModuleRoot(moduleRoot string) error {
	if moduleRoot == "" || moduleRoot == "/" {
		return fmt.Errorf("module root is empty or root")
	}
	goModPath := filepath.Join(moduleRoot, "go.mod")
	st, err := os.Stat(goModPath)
	if err != nil || st.IsDir() {
		return fmt.Errorf("module root %q does not contain go.mod", moduleRoot)
	}
	// Verify module declaration
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return fmt.Errorf("cannot read go.mod: %w", err)
	}
	if !strings.Contains(string(data), "github.com/s1onique/KGB/tovarisch/labs/memory") {
		return fmt.Errorf("go.mod does not declare expected module")
	}
	return nil
}

// findRepoRoot searches upward from start for .git.
func findRepoRoot(start string) (string, error) {
	dir := start
	for {
		if dir == "/" || dir == "" {
			break
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
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
func findModuleRoot(start string) (string, error) {
	dir := start
	for {
		if dir == "/" || dir == "" {
			break
		}
		goModPath := filepath.Join(dir, "tovarisch", "labs", "memory", "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return filepath.Join(dir, "tovarisch", "labs", "memory"), nil
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
