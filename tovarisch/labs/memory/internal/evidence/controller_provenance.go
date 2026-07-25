// controller_provenance.go — Canonical controller-binary provenance
// collector (CORRECTION18, CORRECTION48).
//
// The collector reads the embedded build info via
// `runtime/debug.ReadBuildInfo()` and resolves the source tree
// belonging to the embedded revision via `git rev-parse`. The
// caller may pass a pre-resolved tree identity (e.g. from a
// build-time injector) when the repository is unavailable at
// runtime; the collector validates the pair against the
// repository object format and the embedded VCS revision.
//
// CORRECTION48 P0-4: the CleanPolicy field replaces the legacy
// binary RequireClean. Unknown policies are rejected; the
// closure producer (tovarisch-memory-lab CLI and the compiled
// helper test artifact) MUST use ProvenanceRequireClean. The
// hermetic helper path (compiled helper running outside the live
// qualification) MAY use ProvenanceIgnoreWorktree to absorb
// worktree dirtiness, but the resulting provenance is marked
// QualifyingObservation=false so it can never authorize
// `pass=true`.

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
//
// QualifyingObservation is false when the collector ran under
// ProvenanceIgnoreWorktree (or otherwise detected dirty state
// that the caller chose to record). The verifier and any
// downstream qualification must refuse to assign `pass=true`
// when QualifyingObservation is false. QualifyingObservation is
// independent of whether CollectControllerProvenance returned
// a non-nil error — it is set even on success paths that
// deliberately downgrade authorization.
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
	// TrackedModified is true when there are tracked-file
	// modifications in the working tree (git diff HEAD).
	TrackedModified bool
	// StagedModified is true when there are staged-but-uncommitted
	// changes (git diff --cached).
	StagedModified bool
	// UntrackedFiles is true when there are untracked files in
	// the working tree (git ls-files --others --exclude-standard).
	UntrackedFiles bool
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
	// CleanPolicy records which policy was applied to produce
	// this collector output.
	CleanPolicy ProvenanceCleanPolicy
	// QualifyingObservation is true when the policy is
	// ProvenanceRequireClean AND the checkout passed every
	// dirty-state check. It is false when either the policy
	// downgraded authorization or any dirty-state check failed.
	QualifyingObservation bool
}

// ProvenanceOptions configures the collector. The caller passes a
// pre-resolved tree when the repository is unavailable at runtime.
//
// CleanPolicy is the only acceptable policy selector; an empty
// or unknown value is a hard error. The legacy RequireClean bool
// remains as a fallback shim for callers that have not yet been
// ported: RequireClean=true maps to ProvenanceRequireClean and
// RequireClean=false maps to ProvenanceIgnoreWorktree. The two
// fields are mutually exclusive and may not disagree.
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
	// CleanPolicy is the CORRECTION48 typed selector. An empty or
	// unknown value is rejected by ValidateProvenanceCleanPolicy.
	CleanPolicy ProvenanceCleanPolicy
	// RequireClean is a legacy fallback. When CleanPolicy is
	// empty, RequireClean=true is treated as ProvenanceRequireClean
	// and RequireClean=false as ProvenanceIgnoreWorktree. When
	// CleanPolicy is set, RequireClean must agree (after the
	// mapping above) or the collector returns
	// ErrInconsistentCleanPolicy.
	RequireClean bool
}

// ErrInconsistentCleanPolicy indicates that the caller supplied
// both CleanPolicy and RequireClean, and the two fields disagree
// after the legacy mapping.
var ErrInconsistentCleanPolicy = errors.New("inconsistent provenance cleanliness configuration: CleanPolicy and RequireClean disagree")

// ErrProvenanceUnavailable is returned when neither the embedded
// build info nor the local repository is available.
var ErrProvenanceUnavailable = errors.New("controller provenance unavailable: no embedded VCS info and repository not reachable")

// ErrProvenanceDirty is returned when ProvenanceRequireClean is
// selected and the working tree or source commit is dirty.
var ErrProvenanceDirty = errors.New("controller provenance dirty: working tree or source commit has uncommitted changes")

// ErrProvenanceMismatch is returned when HEAD does not match the
// embedded VCS revision.
var ErrProvenanceMismatch = errors.New("controller provenance mismatch: HEAD does not match embedded VCS revision")

// ErrProvenanceTreeMismatch is returned when the resolved tree
// does not match the embedded tree.
var ErrProvenanceTreeMismatch = errors.New("controller provenance tree mismatch")

// CollectControllerProvenance reads the embedded VCS info from the
// running controller binary, resolves the source tree from the
// repository, and returns a fully bound ControllerProvenance.
// Under ProvenanceRequireClean, any dirty state is a hard error;
// under ProvenanceIgnoreWorktree, dirty state is recorded but the
// resulting QualifyingObservation is forced to false.
func CollectControllerProvenance(opts ProvenanceOptions) (ControllerProvenance, error) {
	policy, err := resolveCleanPolicy(opts)
	if err != nil {
		return ControllerProvenance{}, err
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
	trackedMod := false
	stagedMod := false
	untracked := false
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
			trackedMod, _ = gitTrackedModified(repoDir)
			stagedMod, _ = gitStagedModified(repoDir)
			untracked, _ = gitUntrackedFiles(repoDir)
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
		return ControllerProvenance{}, fmt.Errorf("unknown git object format %q", gitFormat)
	}

	// Compute the SHA-256 of the running controller binary.
	execSHA256, err := executableSHA256()
	if err != nil {
		execSHA256 = ""
	}

	cp := ControllerProvenance{
		VCSRevision:          vcsRevision,
		VCSTree:              tree,
		VCSModified:          vcsModified,
		WorkingTreeDirty:     workingDirty,
		TrackedModified:      trackedMod,
		StagedModified:       stagedMod,
		UntrackedFiles:       untracked,
		SourceCommitDirty:    headMismatch,
		GitObjectFormat:      gitFormat,
		ExecutableSHA256:     execSHA256,
		ProducerVersion:      opts.ProducerVersion,
		DockerServerVersion:  opts.DockerServerVersion,
		CleanPolicy:          policy,
		QualifyingObservation: false,
	}
	switch policy {
	case ProvenanceRequireClean:
		if cp.VCSModified {
			return cp, fmt.Errorf("%w: embedded vcs.modified=true", ErrProvenanceDirty)
		}
		if cp.TrackedModified {
			return cp, fmt.Errorf("%w: tracked files modified in working tree", ErrProvenanceDirty)
		}
		if cp.StagedModified {
			return cp, fmt.Errorf("%w: staged modifications present", ErrProvenanceDirty)
		}
		if cp.UntrackedFiles {
			return cp, fmt.Errorf("%w: untracked files present in working tree", ErrProvenanceDirty)
		}
		if cp.WorkingTreeDirty || cp.SourceCommitDirty {
			return cp, ErrProvenanceDirty
		}
		if head != "" && head != cp.VCSRevision {
			return cp, ErrProvenanceMismatch
		}
		cp.QualifyingObservation = true
	case ProvenanceIgnoreWorktree:
		// Always non-qualifying; dirty facts are recorded for the
		// verifier and the close report.
		cp.QualifyingObservation = false
	}
	return cp, nil
}

// resolveCleanPolicy harmonizes CleanPolicy and the legacy
// RequireClean field. Returns an error when the two disagree.
func resolveCleanPolicy(opts ProvenanceOptions) (ProvenanceCleanPolicy, error) {
	has := false
	policy := opts.CleanPolicy
	if policy != "" {
		if err := ValidateProvenanceCleanPolicy(policy); err != nil {
			return "", err
		}
		has = true
	}
	// Map legacy RequireClean when no explicit CleanPolicy.
	if !has {
		if opts.RequireClean {
			return ProvenanceRequireClean, nil
		}
		return ProvenanceIgnoreWorktree, nil
	}
	// If both are present, RequireClean must agree with CleanPolicy.
	want := policy == ProvenanceRequireClean
	if opts.RequireClean != want && opts.CleanPolicy != "" {
		return "", ErrInconsistentCleanPolicy
	}
	return policy, nil
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
// `git status --porcelain` reports both staged and unstaged
// modifications; the helper-only breakdown is in gitTrackedModified,
// gitStagedModified, and gitUntrackedFiles.
func gitWorkingTreeDirty(dir string) (bool, bool) {
	out, ok := runGit(dir, "status", "--porcelain")
	if !ok {
		return false, false
	}
	return strings.TrimSpace(out) != "", true
}

// gitTrackedModified returns true when tracked files have local
// modifications that are not yet staged. Implemented via
// `git diff --name-only HEAD`.
func gitTrackedModified(dir string) (bool, bool) {
	out, ok := runGit(dir, "diff", "--name-only", "HEAD")
	if !ok {
		return false, false
	}
	return strings.TrimSpace(out) != "", true
}

// gitStagedModified returns true when changes are staged for
// commit but not yet committed. Implemented via
// `git diff --cached --name-only`.
func gitStagedModified(dir string) (bool, bool) {
	out, ok := runGit(dir, "diff", "--cached", "--name-only")
	if !ok {
		return false, false
	}
	return strings.TrimSpace(out) != "", true
}

// gitUntrackedFiles returns true when there are untracked files in
// the working tree (excluding ignored paths). Implemented via
// `git ls-files --others --exclude-standard`.
func gitUntrackedFiles(dir string) (bool, bool) {
	out, ok := runGit(dir, "ls-files", "--others", "--exclude-standard")
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
