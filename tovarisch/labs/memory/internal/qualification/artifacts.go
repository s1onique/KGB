// Package qualification owns external artifact paths and role-separation
// authority for the memory-lab qualification harness.
package qualification

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	HelperBinaryName     = "tovarisch-memory-lab-live-helper.test"
	ProductionBinaryName = "tovarisch-memory-lab"
	MetadataName         = "canary-image-build.json"
)

type QualificationArtifactPaths struct {
	Root               string
	Metadata           string
	HelperBinary       string
	ProductionBinary   string
	HelperEvidence     string
	ProductionEvidence string
	Record             string
}

var (
	ErrArtifactRootInvalid    = errors.New("qualification artifact root is not an absolute external directory")
	ErrPathCollidesWithHelper = errors.New("qualification artifact paths collide")
)

// NewQualificationArtifactPaths validates an external root against the Git
// checkout containing the current process. It uses Git's repository authority,
// not assumptions about the shape of a worktree's administrative files.
func NewQualificationArtifactPaths(root string) (QualificationArtifactPaths, error) {
	return newQualificationArtifactPaths(root, "")
}

// NewQualificationArtifactPathsForSource is the detached-worktree-safe form
// used by the builder. sourceRoot is the checkout whose artifacts must remain
// external.
func NewQualificationArtifactPathsForSource(root, sourceRoot string) (QualificationArtifactPaths, error) {
	return newQualificationArtifactPaths(root, sourceRoot)
}

func newQualificationArtifactPaths(root, sourceRoot string) (QualificationArtifactPaths, error) {
	if root == "" || !filepath.IsAbs(root) {
		return QualificationArtifactPaths{}, fmt.Errorf("%w: root must be absolute", ErrArtifactRootInvalid)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return QualificationArtifactPaths{}, fmt.Errorf("%w: resolve root: %v", ErrArtifactRootInvalid, err)
	}
	info, err := os.Stat(resolvedRoot)
	if err != nil || !info.IsDir() {
		return QualificationArtifactPaths{}, fmt.Errorf("%w: root is not an existing directory", ErrArtifactRootInvalid)
	}
	if sourceRoot != "" {
		canonicalSource, err := filepath.EvalSymlinks(sourceRoot)
		if err != nil {
			return QualificationArtifactPaths{}, fmt.Errorf("%w: source root: %v", ErrArtifactRootInvalid, err)
		}
		if pathIsWithin(canonicalSource, resolvedRoot) || pathIsWithin(resolvedRoot, canonicalSource) {
			return QualificationArtifactPaths{}, fmt.Errorf("%w: root intersects source checkout", ErrArtifactRootInvalid)
		}
	}
	// If Git recognizes the candidate as part of any checkout, reject it.
	// This works for both ordinary and linked worktrees.
	if checkout, ok := gitShowTopLevel(resolvedRoot); ok && pathIsWithin(checkout, resolvedRoot) {
		return QualificationArtifactPaths{}, fmt.Errorf("%w: root is inside checkout %q", ErrArtifactRootInvalid, checkout)
	}
	metadata := filepath.Join(resolvedRoot, "metadata", MetadataName)
	helper := filepath.Join(resolvedRoot, "bin", HelperBinaryName)
	production := filepath.Join(resolvedRoot, "bin", ProductionBinaryName)
	helperEvidence := filepath.Join(resolvedRoot, "evidence", "helper-evidence.json")
	productionEvidence := filepath.Join(resolvedRoot, "evidence", "production-evidence.json")
	record := filepath.Join(resolvedRoot, "role-separation.json")
	if helper == production || helperEvidence == productionEvidence {
		return QualificationArtifactPaths{}, ErrPathCollidesWithHelper
	}
	return QualificationArtifactPaths{Root: resolvedRoot, Metadata: metadata, HelperBinary: helper, ProductionBinary: production, HelperEvidence: helperEvidence, ProductionEvidence: productionEvidence, Record: record}, nil
}

func pathIsWithin(parent, candidate string) bool {
	rel, err := filepath.Rel(parent, candidate)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func gitShowTopLevel(path string) (string, bool) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	root, err := filepath.EvalSymlinks(strings.TrimSpace(string(out)))
	if err != nil {
		return "", false
	}
	return root, true
}

// GitPathAuthorities exposes the canonical Git path authorities needed by
// repository tooling. Callers must not synthesize paths from a .git entry.
func GitPathAuthorities(root string) (gitDir, commonDir, hooks string, err error) {
	gitDir, err = gitRevParse(root, "--git-dir")
	if err != nil {
		return "", "", "", err
	}
	commonDir, err = gitRevParse(root, "--git-common-dir")
	if err != nil {
		return "", "", "", err
	}
	hooks, err = gitRevParse(root, "--git-path", "hooks")
	if err != nil {
		return "", "", "", err
	}
	return resolveGitPath(root, gitDir), resolveGitPath(root, commonDir), resolveGitPath(root, hooks), nil
}

func gitRevParse(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root, "rev-parse"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	value := strings.TrimSpace(string(out))
	if value == "" {
		return "", errors.New("git returned an empty path")
	}
	return value, nil
}

func resolveGitPath(root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(root, path))
}
