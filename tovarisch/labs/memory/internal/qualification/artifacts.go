// Package qualification owns the harness-side contracts for
// building and verifying the two controller artifacts that
// participate in a canary-matrix live qualification:
//
//   1. the helper Go-test executable compiled via `go test -c`,
//      and
//   2. the production tovarisch-memory-lab CLI compiled via
//      `go build`.
//
// The package deliberately knows nothing about Docker or
// canary-image build steps; it only owns the path layout and
// the role-separation record consumed by the
// verify-qualification-artifacts verifier.
//
// Reference: kgb://factory/workflow
package qualification

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// HelperBinaryName is the canonical filename of the compiled
// helper test executable. The Go `go test -c` default is to
// drop the file alongside the test source. CORRECTION48
// requires the build script to redirect that to the external
// artifact root and rename it to a stable, role-bearing name.
const HelperBinaryName = "tovarisch-memory-lab-helper.test"

// ProductionBinaryName is the canonical filename of the
// compiled production CLI artifact.
const ProductionBinaryName = "tovarisch-memory-lab"

// HelperEvidenceName is the canonical filename of the helper
// live-execution evidence (placeholder; the live run lives in
// CORRECTION49, but the role-separation record names the path
// so downstream tooling can read it).
const HelperEvidenceName = "helper-evidence.json"

// ProductionEvidenceName is the canonical filename of the
// production live-execution evidence (placeholder; the live run
// lives in CORRECTION49).
const ProductionEvidenceName = "production-evidence.json"

// MetadataName is the canonical filename of the canary image
// build metadata that lives beneath the external artifact root.
const MetadataName = "canary-image-build.json"

// QualificationArtifactPaths describes the canonical path
// layout beneath the external artifact root.
//
// All four binary/evidence paths are absolute after
// construction. They are written beneath the artifact root only
// if the root rejects any path that would otherwise resolve
// beneath the source checkout.
type QualificationArtifactPaths struct {
	Root               string
	Metadata           string
	HelperBinary       string
	ProductionBinary   string
	HelperEvidence     string
	ProductionEvidence string
}

// ErrArtifactRootInvalid indicates the supplied root is not a
// suitable external artifact directory (see NewQualificationArtifactPaths).
var ErrArtifactRootInvalid = errors.New("qualification artifact root is not an absolute external directory")

// ErrPathCollidesWithHelper indicates two of the canonical
// paths resolved to the same location.
var ErrPathCollidesWithHelper = errors.New("qualification artifact paths collide: helper and production resolve to the same location")

// moduleRootCandidates returns the list of directories that are
// considered part of the source checkout. Returning to the test
// build the helper lives in `cmd/tovarisch-memory-lab/`-shaped
// test packages inside `tovarisch/labs/memory/`. We treat the
// entire `tovarisch/labs/memory/` tree plus the parent
// repository as off-limits.
func moduleRootsPresent() []string {
	if cwd, err := os.Getwd(); err == nil {
		return []string{cwd}
	}
	return nil
}

// NewQualificationArtifactPaths validates the supplied root and
// constructs the canonical path layout beneath it. The function
// refuses to construct paths that resolve beneath the working
// directory (which inside `tovarisch/labs/memory/` IS the
// repository).
//
// Required root invariants:
//   - filepath.Abs(root) succeeds and equals root;
//   - root is an existing directory;
//   - root is NOT beneath the source checkout.
//
// Returns ErrArtifactRootInvalid when any invariant fails.
func NewQualificationArtifactPaths(root string) (QualificationArtifactPaths, error) {
	if root == "" {
		return QualificationArtifactPaths{}, fmt.Errorf("%w: empty root", ErrArtifactRootInvalid)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return QualificationArtifactPaths{}, fmt.Errorf("%w: abs failed: %v", ErrArtifactRootInvalid, err)
	}
	if abs != root {
		// The caller did not pass an absolute path. We refuse to
		// silently rebuild relative paths because the S48
		// contract demands explicit absolute roots.
		return QualificationArtifactPaths{}, fmt.Errorf("%w: relative root %q (resolved=%q)", ErrArtifactRootInvalid, root, abs)
	}
	// Resolve symlinks BEFORE the repository check. A symlink
	// whose target is the source checkout (or any path beneath
	// one) must be detected as a repository root.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	info, err := os.Stat(abs)
	if err != nil {
		return QualificationArtifactPaths{}, fmt.Errorf("%w: stat failed: %v", ErrArtifactRootInvalid, err)
	}
	if !info.IsDir() {
		return QualificationArtifactPaths{}, fmt.Errorf("%w: %q is not a directory", ErrArtifactRootInvalid, abs)
	}
	cwd, _ := os.Getwd()
	rel, err := filepath.Rel(cwd, abs)
	if err == nil && !strings.HasPrefix(rel, "..") && rel != ".." {
		// abs is beneath cwd (or equal to it). Reject.
		return QualificationArtifactPaths{}, fmt.Errorf("%w: %q is inside the working tree (%q)", ErrArtifactRootInvalid, abs, cwd)
	}
	// Belt and braces: walk up looking for the KGB `.git`; if
	// found anywhere along the parent chain, treat as "inside
	// the repository checkout". This catches callers that pass
	// a sibling directory under the same repo.
	if pathHasAncestorWithGit(abs) {
		return QualificationArtifactPaths{}, fmt.Errorf("%w: %q resolves beneath a .git ancestor", ErrArtifactRootInvalid, abs)
	}
	metadata := filepath.Join(abs, "metadata", MetadataName)
	helper := filepath.Join(abs, "bin", HelperBinaryName)
	production := filepath.Join(abs, "bin", ProductionBinaryName)
	helperEvidence := filepath.Join(abs, "evidence", HelperEvidenceName)
	productionEvidence := filepath.Join(abs, "evidence", ProductionEvidenceName)
	if helper == production {
		return QualificationArtifactPaths{}, ErrPathCollidesWithHelper
	}
	return QualificationArtifactPaths{
		Root:               abs,
		Metadata:           metadata,
		HelperBinary:       helper,
		ProductionBinary:   production,
		HelperEvidence:     helperEvidence,
		ProductionEvidence: productionEvidence,
	}, nil
}

// pathHasAncestorWithGit walks the directory upward looking for
// a `.git` entry. Returns true if one is encountered. The check
// stops at the filesystem root.
func pathHasAncestorWithGit(p string) bool {
	cur := p
	for {
		if _, err := os.Stat(filepath.Join(cur, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return false
		}
		cur = parent
	}
}

// moduleRootsPresent is documented in moduleRootsPresent
// declaration; the symbol is kept for compatibility with the
// inventory layer.
func init() {
	_ = moduleRootsPresent
}
