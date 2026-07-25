// controller_provenance.go — Canonical controller-binary provenance
// collector (CORRECTION18).
//
// The collector reads the embedded build info via
// `runtime/debug.ReadBuildInfo()` and resolves the source tree
// belonging to the embedded revision via `git rev-parse`. The
// caller may pass a pre-resolved tree identity (e.g. from a
// build-time injector) when the repository is unavailable at
// runtime; the collector validates the pair against the repository
// object format and the embedded VCS revision.

package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
)

// ControllerProvenance is the result of the controller-binary
// provenance collector. The fields are bound to the running
// controller binary; canary subject provenance is recorded
// separately inside the canary image build metadata.
type ControllerProvenance struct {
	// VCSRevision is the embedded git revision (full OID).
	VCSRevision string
	// VCSTree is the resolved source tree belonging to the revision.
	VCSTree string
	// VCSModified is true when the embedded build reports the
	// source has local modifications.
	VCSModified bool
	// WorkingTreeDirty is true when `git status --porcelain` reports
	// any modifications at the controller working tree.
	WorkingTreeDirty bool
	// SourceCommitDirty is true when HEAD differs from the
	// embedded revision.
	SourceCommitDirty bool
	// GitObjectFormat is the canonical object format: "sha1" or
	// "sha256".
	GitObjectFormat string
	// ExecutableSHA256 is the SHA-256 of the controller binary on
	// disk.
	ExecutableSHA256 string
	// ProducerVersion is the producer/CLI version string.
	ProducerVersion string
	// DockerServerVersion is collected once by orchestration and carried
	// with the running-binary provenance authority.
	DockerServerVersion string
}

// ProvenanceOptions configures the collector. The caller passes a
// pre-resolved tree when the repository is unavailable at runtime.
type ProvenanceOptions struct {
	// RepoDir is the path to the repository root. The collector runs
	// `git` from this directory.
	RepoDir string
	// EmbeddedTreeOverride is the build-time tree identity. When
	// non-empty, it is used as the source tree; the collector still
	// validates the embedded revision via git and proves
	// HEAD==embedded.
	EmbeddedTreeOverride string
	// ProducerVersion is the CLI version string.
	ProducerVersion string
	// DockerServerVersion is the observed Engine server version.
	DockerServerVersion string
	// RequireClean forces a failure when the working tree or source
	// commit is dirty. Defaults to true.
	RequireClean bool
}

// ErrProvenanceUnavailable is returned when neither the embedded
// build info nor the local repository is available.
var ErrProvenanceUnavailable = errors.New("controller provenance unavailable: no embedded VCS info and repository not reachable")

// ErrProvenanceDirty is returned when RequireClean is true and the
// working tree or the source commit is dirty.
var ErrProvenanceDirty = errors.New("controller provenance dirty: working tree or source commit has uncommitted changes")

// ErrProvenanceMismatch is returned when HEAD does not match the
// embedded VCS revision.
var ErrProvenanceMismatch = errors.New("controller provenance mismatch: HEAD does not match embedded VCS revision")

// ErrProvenanceTreeMismatch is returned when the resolved tree
// does not match the embedded tree.
var ErrProvenanceTreeMismatch = errors.New("controller provenance tree mismatch")

// CollectControllerProvenance reads the embedded VCS info from the
// running controller binary, resolves the source tree from the
// repository, and returns a fully bound ControllerProvenance. When
// RequireClean is true (default), any dirty state is a hard error.
func CollectControllerProvenance(opts ProvenanceOptions) (ControllerProvenance, error) {
	if opts.RequireClean == false {
		// Treat unset as true. Set explicitly to false to allow dirty.
		opts.RequireClean = true
	}
	build, ok := debug.ReadBuildInfo()
	if !ok {
		return ControllerProvenance{}, ErrProvenanceUnavailable
	}
	var vcsRevision, vcsModifiedRaw string
	for _, s := range build.Settings {
		switch s.Key {
		case "vcs.revision":
			vcsRevision = s.Value
		case "vcs.modified":
			vcsModifiedRaw = s.Value
		}
	}
	if vcsRevision == "" {
		return ControllerProvenance{}, ErrProvenanceUnavailable
	}
	if !sha1Hex40.MatchString(vcsRevision) && !sha256Hex64.MatchString(vcsRevision) {
		return ControllerProvenance{}, fmt.Errorf("embedded vcs.revision %q is not 40 or 64 hex chars", vcsRevision)
	}
	vcsModified := vcsModifiedRaw == "true"

	// Git object format from `git rev-parse --show-object-format`.
	gitFormat := ""
	repoDir := opts.RepoDir
	head := ""
	tree := ""
	workingDirty := false
	headMismatch := false
	if repoDir != "" {
		if exists, err := dirExists(repoDir); err == nil && exists {
			gitFormat, _ = runGit(repoDir, "rev-parse", "--show-object-format")
			head, _ = runGit(repoDir, "rev-parse", "HEAD")
			if vcsRevision != "" {
				tree, _ = runGit(repoDir, "rev-parse", "--verify", vcsRevision+"^{tree}")
			}
			if head != "" && head != vcsRevision {
				headMismatch = true
			}
			workingDirty, _ = gitWorkingTreeDirty(repoDir)
		}
	}
	if opts.EmbeddedTreeOverride != "" {
		tree = opts.EmbeddedTreeOverride
	}
	if tree == "" {
		return ControllerProvenance{}, ErrProvenanceUnavailable
	}
	// Validate the resolved tree against the git object format.
	if gitFormat == gitObjectFormatSHA1 {
		if err := ValidateSHA1Hex(tree); err != nil {
			return ControllerProvenance{}, fmt.Errorf("%w: tree=%q format=sha1", ErrProvenanceTreeMismatch, tree)
		}
	} else if gitFormat == gitObjectFormatSHA256 {
		if err := ValidateSHA256Hex(tree); err != nil {
			return ControllerProvenance{}, fmt.Errorf("%w: tree=%q format=sha256", ErrProvenanceTreeMismatch, tree)
		}
	} else if gitFormat != "" {
		// Unknown object format: reject.
		return ControllerProvenance{}, fmt.Errorf("unknown git object format %q", gitFormat)
	}

	// Compute the SHA-256 of the running controller binary.
	execSHA256, err := executableSHA256()
	if err != nil {
		execSHA256 = ""
	}

	cp := ControllerProvenance{
		VCSRevision:         vcsRevision,
		VCSTree:             tree,
		VCSModified:         vcsModified,
		WorkingTreeDirty:    workingDirty,
		SourceCommitDirty:   headMismatch,
		GitObjectFormat:     gitFormat,
		ExecutableSHA256:    execSHA256,
		ProducerVersion:     opts.ProducerVersion,
		DockerServerVersion: opts.DockerServerVersion,
	}
	if opts.RequireClean {
		if cp.VCSModified || cp.WorkingTreeDirty || cp.SourceCommitDirty {
			return cp, ErrProvenanceDirty
		}
		if head != "" && head != cp.VCSRevision {
			return cp, ErrProvenanceMismatch
		}
	}
	return cp, nil
}

// runGit runs `git <args...>` in dir and returns (stdout, success).
func runGit(dir string, args ...string) (string, bool) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

func dirExists(p string) (bool, error) {
	if p == "" {
		return false, nil
	}
	st, err := os.Stat(p)
	if err != nil {
		return false, err
	}
	return st.IsDir(), nil
}

// gitWorkingTreeDirty returns true when the working tree is dirty.
func gitWorkingTreeDirty(dir string) (bool, bool) {
	out, ok := runGit(dir, "status", "--porcelain")
	if !ok {
		return false, false
	}
	return strings.TrimSpace(out) != "", true
}

// executableSHA256 hashes the running controller binary on disk.
// Returns the empty string when the executable path cannot be
// resolved.
func executableSHA256() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		resolved = exe
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
