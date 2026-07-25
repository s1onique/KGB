// controller_provenance.go — canonical controller-binary provenance.
package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
)

// ControllerProvenance is bound to the running controller binary and the
// repository state from which it was built.
type ControllerProvenance struct {
	VCSRevision           string
	VCSTree               string
	VCSModified           bool
	WorkingTreeDirty      bool
	TrackedModified       bool
	StagedModified        bool
	UntrackedFiles        bool
	SourceCommitDirty     bool
	GitObjectFormat       string
	ExecutableSHA256      string
	ProducerVersion       string
	DockerServerVersion   string
	CleanPolicy           ProvenanceCleanPolicy
	QualifyingObservation bool
}

// ProvenanceOptions is intentionally typed and explicit. An empty policy is a
// hard error; there is no legacy Boolean mapping that can silently authorize a
// qualifying observation.
type ProvenanceOptions struct {
	RepoDir              string
	EmbeddedTreeOverride string
	ExecutablePath       string
	ProducerVersion      string
	DockerServerVersion  string
	CleanPolicy          ProvenanceCleanPolicy
}

var (
	ErrProvenanceUnavailable  = fmt.Errorf("controller provenance unavailable: no complete embedded VCS info or repository")
	ErrProvenanceDirty        = fmt.Errorf("controller provenance dirty: working tree or source commit has uncommitted changes")
	ErrProvenanceMismatch     = fmt.Errorf("controller provenance mismatch: HEAD does not match embedded VCS revision")
	ErrProvenanceTreeMismatch = fmt.Errorf("controller provenance tree mismatch")
)

// CollectControllerProvenance requires physical vcs, vcs.revision, vcs.time,
// and vcs.modified settings from runtime/debug. Missing/empty settings are not
// interpreted as clean.
func CollectControllerProvenance(opts ProvenanceOptions) (ControllerProvenance, error) {
	if err := ValidateProvenanceCleanPolicy(opts.CleanPolicy); err != nil {
		return ControllerProvenance{}, err
	}
	build, ok := debug.ReadBuildInfo()
	if !ok {
		return ControllerProvenance{}, ErrProvenanceUnavailable
	}
	settings := make(map[string]string, len(build.Settings))
	for _, setting := range build.Settings {
		settings[setting.Key] = setting.Value
	}
	for _, key := range []string{"vcs", "vcs.revision", "vcs.time", "vcs.modified"} {
		value, present := settings[key]
		if !present || value == "" {
			return ControllerProvenance{}, fmt.Errorf("%w: missing or empty %s", ErrProvenanceUnavailable, key)
		}
	}
	if settings["vcs"] != "git" {
		return ControllerProvenance{}, fmt.Errorf("%w: vcs=%q", ErrProvenanceUnavailable, settings["vcs"])
	}
	vcsRevision := settings["vcs.revision"]
	if !sha1Hex40.MatchString(vcsRevision) && !sha256Hex64.MatchString(vcsRevision) {
		return ControllerProvenance{}, fmt.Errorf("embedded vcs.revision %q is not 40 or 64 lowercase hex chars", vcsRevision)
	}
	if settings["vcs.modified"] != "true" && settings["vcs.modified"] != "false" {
		return ControllerProvenance{}, fmt.Errorf("%w: malformed vcs.modified=%q", ErrProvenanceUnavailable, settings["vcs.modified"])
	}
	vcsModified := settings["vcs.modified"] == "true"

	gitFormat, head, tree := "", "", ""
	workingDirty, trackedMod, stagedMod, untracked, headMismatch := false, false, false, false, false
	repositoryAuthorityComplete := false
	if opts.RepoDir != "" {
		if exists, statErr := dirExists(opts.RepoDir); statErr == nil && exists {
			var formatOK, headOK, treeOK, worktreeOK, trackedOK, stagedOK, untrackedOK bool
			gitFormat, formatOK = runGit(opts.RepoDir, "rev-parse", "--show-object-format")
			head, headOK = runGit(opts.RepoDir, "rev-parse", "HEAD")
			tree, treeOK = runGit(opts.RepoDir, "rev-parse", "--verify", vcsRevision+"^{tree}")
			workingDirty, worktreeOK = gitWorkingTreeDirty(opts.RepoDir)
			trackedMod, trackedOK = gitTrackedModified(opts.RepoDir)
			stagedMod, stagedOK = gitStagedModified(opts.RepoDir)
			untracked, untrackedOK = gitUntrackedFiles(opts.RepoDir)
			repositoryAuthorityComplete = formatOK && headOK && treeOK && worktreeOK && trackedOK && stagedOK && untrackedOK
			if headOK && head != vcsRevision {
				headMismatch = true
			}
		}
	}
	if opts.CleanPolicy == ProvenanceRequireClean && !repositoryAuthorityComplete {
		return ControllerProvenance{}, fmt.Errorf("%w: repository authority is incomplete", ErrProvenanceUnavailable)
	}
	if opts.EmbeddedTreeOverride != "" {
		if !sha1Hex40.MatchString(opts.EmbeddedTreeOverride) && !sha256Hex64.MatchString(opts.EmbeddedTreeOverride) {
			return ControllerProvenance{}, fmt.Errorf("%w: override=%q", ErrProvenanceTreeMismatch, opts.EmbeddedTreeOverride)
		}
		if tree != "" && tree != opts.EmbeddedTreeOverride {
			return ControllerProvenance{}, fmt.Errorf("%w: repository=%q override=%q", ErrProvenanceTreeMismatch, tree, opts.EmbeddedTreeOverride)
		}
		if tree == "" {
			tree = opts.EmbeddedTreeOverride
		}
	}
	if tree == "" {
		return ControllerProvenance{}, ErrProvenanceUnavailable
	}
	if gitFormat == gitObjectFormatSHA1 {
		if err := ValidateSHA1Hex(tree); err != nil {
			return ControllerProvenance{}, fmt.Errorf("%w: %v", ErrProvenanceTreeMismatch, err)
		}
	} else if gitFormat == gitObjectFormatSHA256 {
		if err := ValidateSHA256Hex(tree); err != nil {
			return ControllerProvenance{}, fmt.Errorf("%w: %v", ErrProvenanceTreeMismatch, err)
		}
	} else if gitFormat != "" {
		return ControllerProvenance{}, fmt.Errorf("unknown git object format %q", gitFormat)
	}

	executablePath := opts.ExecutablePath
	if executablePath == "" {
		executablePath, _ = os.Executable()
	}
	execSHA256 := ""
	if executablePath != "" {
		if resolved, err := filepath.EvalSymlinks(executablePath); err == nil {
			if data, err := os.ReadFile(resolved); err == nil {
				sum := sha256.Sum256(data)
				execSHA256 = hex.EncodeToString(sum[:])
			}
		}
	}
	cp := ControllerProvenance{
		VCSRevision: vcsRevision, VCSTree: tree, VCSModified: vcsModified,
		WorkingTreeDirty: workingDirty, TrackedModified: trackedMod,
		StagedModified: stagedMod, UntrackedFiles: untracked,
		SourceCommitDirty: headMismatch, GitObjectFormat: gitFormat,
		ExecutableSHA256: execSHA256, ProducerVersion: opts.ProducerVersion,
		DockerServerVersion: opts.DockerServerVersion, CleanPolicy: opts.CleanPolicy,
	}
	if err := authorizeProvenance(&cp, head); err != nil {
		return cp, err
	}
	return cp, nil
}

// authorizeProvenance is the single policy gate. Ignore-worktree is useful
// for targeted non-qualifying tests only and can never set authorization true.
func authorizeProvenance(cp *ControllerProvenance, repositoryHead string) error {
	switch cp.CleanPolicy {
	case ProvenanceRequireClean:
		if cp.VCSModified {
			return fmt.Errorf("%w: embedded vcs.modified=true", ErrProvenanceDirty)
		}
		if cp.TrackedModified {
			return fmt.Errorf("%w: tracked files modified", ErrProvenanceDirty)
		}
		if cp.StagedModified {
			return fmt.Errorf("%w: staged modifications", ErrProvenanceDirty)
		}
		if cp.UntrackedFiles {
			return fmt.Errorf("%w: untracked files", ErrProvenanceDirty)
		}
		if cp.WorkingTreeDirty || cp.SourceCommitDirty {
			return ErrProvenanceDirty
		}
		if repositoryHead != "" && repositoryHead != cp.VCSRevision {
			return ErrProvenanceMismatch
		}
		cp.QualifyingObservation = true
	case ProvenanceIgnoreWorktree:
		cp.QualifyingObservation = false
	default:
		return ValidateProvenanceCleanPolicy(cp.CleanPolicy)
	}
	return nil
}

func runGit(dir string, args ...string) (string, bool) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}
func dirExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}
func gitWorkingTreeDirty(dir string) (bool, bool) {
	out, ok := runGit(dir, "status", "--porcelain", "--untracked-files=all")
	return strings.TrimSpace(out) != "", ok
}
func gitTrackedModified(dir string) (bool, bool) {
	out, ok := runGit(dir, "diff", "--name-only", "HEAD")
	return strings.TrimSpace(out) != "", ok
}
func gitStagedModified(dir string) (bool, bool) {
	out, ok := runGit(dir, "diff", "--cached", "--name-only")
	return strings.TrimSpace(out) != "", ok
}
func gitUntrackedFiles(dir string) (bool, bool) {
	out, ok := runGit(dir, "ls-files", "--others", "--exclude-standard")
	return strings.TrimSpace(out) != "", ok
}
