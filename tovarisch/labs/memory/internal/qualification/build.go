package qualification

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BuildOptions controls a hermetic external qualification build.
type BuildOptions struct {
	SourceRoot   string
	ArtifactRoot string
}

// BuildQualificationArtifacts compiles the exact live helper package and the
// production CLI, validates physical build authority immediately, and writes
// the record only after both binaries and role probes pass.
var (
	qualificationRunGo        = runGo
	qualificationRecordBinary = BinaryRecordFromFile
)

func BuildQualificationArtifacts(opts BuildOptions) (recordPath string, err error) {
	if err := rejectDisabledBuildVCS(); err != nil {
		return "", err
	}
	sourceRoot, err := canonicalPath(opts.SourceRoot)
	if err != nil {
		return "", fmt.Errorf("source root: %w", err)
	}
	moduleRoot := filepath.Join(sourceRoot, "tovarisch", "labs", "memory")
	if info, statErr := os.Stat(moduleRoot); statErr != nil || !info.IsDir() {
		return "", fmt.Errorf("memory module is unavailable beneath %s", sourceRoot)
	}
	commit, tree, err := sourceIdentity(sourceRoot)
	if err != nil {
		return "", err
	}
	if dirty, err := sourceDirty(sourceRoot); err != nil {
		return "", err
	} else if dirty {
		return "", fmt.Errorf("%w: %s", ErrDirtySource, sourceRoot)
	}
	if !ValidateObjectID(commit) || !ValidateObjectID(tree) {
		return "", fmt.Errorf("source identity is malformed: commit=%q tree=%q", commit, tree)
	}
	artifactRoot, err := filepath.Abs(opts.ArtifactRoot)
	if err != nil {
		return "", fmt.Errorf("artifact root: %w", err)
	}
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		return "", fmt.Errorf("create artifact root: %w", err)
	}
	paths, err := NewQualificationArtifactPathsForSource(artifactRoot, sourceRoot)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(paths.HelperBinary), 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(paths.Metadata), 0o755); err != nil {
		return "", err
	}

	built := []string{paths.HelperBinary, paths.ProductionBinary, paths.Record}
	succeeded := false
	defer func() {
		if !succeeded {
			for _, path := range built {
				_ = os.Remove(path)
			}
		}
	}()

	if err := qualificationRunGo(moduleRoot, "test", "-buildvcs=true", "-c", "-o", paths.HelperBinary, "./cmd/tovarisch-memory-lab"); err != nil {
		return "", fmt.Errorf("build live helper: %w", err)
	}
	if err := qualificationRunGo(moduleRoot, "build", "-buildvcs=true", "-o", paths.ProductionBinary, "./cmd/tovarisch-memory-lab"); err != nil {
		return "", fmt.Errorf("build production CLI: %w", err)
	}

	helper, err := qualificationRecordBinary(paths.HelperBinary, BinaryRoleLiveHelper)
	if err != nil {
		return "", fmt.Errorf("helper authority: %w", err)
	}
	if helper.VCSRevision != commit {
		return "", fmt.Errorf("%w: helper=%q source=%q", ErrHelperRevisionMismatch, helper.VCSRevision, commit)
	}
	if helper.VCSModified {
		return "", ErrHelperModified
	}
	if _, err := RunExactHelperTestList(paths.HelperBinary, LiveHelperTest); err != nil {
		return "", err
	}
	production, err := qualificationRecordBinary(paths.ProductionBinary, BinaryRoleProductionCLI)
	if err != nil {
		return "", fmt.Errorf("production authority: %w", err)
	}
	if production.VCSRevision != commit {
		return "", fmt.Errorf("%w: production=%q source=%q", ErrProductionRevisionMismatch, production.VCSRevision, commit)
	}
	if production.VCSModified {
		return "", ErrProductionModified
	}
	exitCode, _, err := RunProductionHelp(paths.ProductionBinary)
	if err != nil || exitCode != 0 {
		if err != nil {
			return "", err
		}
		return "", ErrProductionHelpFailure
	}
	if helper.AbsolutePath == production.AbsolutePath {
		return "", ErrRelationshipSamePath
	}
	if helper.Device == production.Device && helper.Inode == production.Inode {
		return "", ErrRelationshipSameDeviceInode
	}
	if helper.SHA256 == production.SHA256 {
		return "", ErrRelationshipSameHash
	}

	record := QualificationRecord{
		SchemaVersion:          RecordSchemaVersion,
		SourceRoot:             sourceRoot,
		SourceCommit:           commit,
		SourceTree:             tree,
		Helper:                 helper,
		Production:             production,
		HelperLiveTest:         LiveHelperTest,
		ProductionHelpExitCode: exitCode,
	}
	data, err := MarshalQualificationRecord(record)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(paths.Record, data, 0o644); err != nil {
		return "", fmt.Errorf("write qualification record: %w", err)
	}
	succeeded = true
	return paths.Record, nil
}

func rejectDisabledBuildVCS() error {
	flags := os.Getenv("GOFLAGS")
	for _, token := range strings.Fields(flags) {
		if strings.Contains(token, "-buildvcs=false") {
			return fmt.Errorf("%w: %q", ErrBuildVCSDisabled, token)
		}
	}
	return nil
}

func sourceIdentity(root string) (string, string, error) {
	commit, err := gitValue(root, "rev-parse", "HEAD")
	if err != nil {
		return "", "", err
	}
	tree, err := gitValue(root, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return "", "", err
	}
	return commit, tree, nil
}

func sourceDirty(root string) (bool, error) {
	out, err := gitValue(root, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		// A clean checkout produces no output; gitValue deliberately rejects
		// empty output, so call the command directly for this predicate.
		cmd := exec.Command("git", "-C", root, "status", "--porcelain", "--untracked-files=all")
		bytes, runErr := cmd.CombinedOutput()
		if runErr != nil {
			return false, fmt.Errorf("git status: %w (%s)", runErr, strings.TrimSpace(string(bytes)))
		}
		return strings.TrimSpace(string(bytes)) != "", nil
	}
	return strings.TrimSpace(out) != "", nil
}

func runGo(moduleRoot string, args ...string) error {
	cmd := exec.Command("go", args...)
	cmd.Dir = moduleRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
