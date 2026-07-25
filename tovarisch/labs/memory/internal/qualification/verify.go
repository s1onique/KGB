package qualification

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// VerifyOptions describes the two independent inputs to verification.
type VerifyOptions struct {
	SourceRoot string
	RecordPath string
}

// VerifyQualificationArtifacts reconstructs source, filesystem, embedded
// build-info, and executable-role facts. The record is used only as a set of
// values to compare against independently reconstructed facts.
func VerifyQualificationArtifacts(opts VerifyOptions) error {
	if opts.SourceRoot == "" || opts.RecordPath == "" {
		return fmt.Errorf("%w: source-root and record are required", ErrRecordInvalid)
	}
	sourceRoot, err := canonicalPath(opts.SourceRoot)
	if err != nil {
		return fmt.Errorf("resolve source root: %w", err)
	}
	raw, err := readRecordFile(opts.RecordPath)
	if err != nil {
		return err
	}
	record, err := DecodeQualificationRecord(raw)
	if err != nil {
		return err
	}
	if record.SourceRoot != sourceRoot {
		return fmt.Errorf("%w: record=%q actual=%q", ErrSourceRootMismatch, record.SourceRoot, sourceRoot)
	}
	head, err := gitValue(sourceRoot, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("resolve source HEAD: %w", err)
	}
	tree, err := gitValue(sourceRoot, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return fmt.Errorf("resolve source tree: %w", err)
	}
	if record.SourceCommit != head {
		return fmt.Errorf("%w: record=%q actual=%q", ErrSourceCommitMismatch, record.SourceCommit, head)
	}
	if record.SourceTree != tree {
		return fmt.Errorf("%w: record=%q actual=%q", ErrSourceTreeMismatch, record.SourceTree, tree)
	}

	helper, err := verifyBinaryRecord("helper", record.Helper, BinaryRoleLiveHelper, head)
	if err != nil {
		return err
	}
	production, err := verifyBinaryRecord("production", record.Production, BinaryRoleProductionCLI, head)
	if err != nil {
		return err
	}
	if helper.resolvedPath == production.resolvedPath {
		return ErrRelationshipSamePath
	}
	if helper.record.Device == production.record.Device && helper.record.Inode == production.record.Inode {
		return ErrRelationshipSameDeviceInode
	}
	if helper.record.SHA256 == production.record.SHA256 {
		return ErrRelationshipSameHash
	}

	if record.HelperLiveTest != LiveHelperTest {
		return fmt.Errorf("%w: record=%q", ErrHelperTestMissing, record.HelperLiveTest)
	}
	if _, err := RunExactHelperTestList(helper.resolvedPath, LiveHelperTest); err != nil {
		return err
	}
	exitCode, _, err := RunProductionHelp(production.resolvedPath)
	if err != nil {
		return err
	}
	if exitCode != 0 || record.ProductionHelpExitCode != exitCode {
		return fmt.Errorf("%w: record=%d actual=%d", ErrProductionHelpFailure, record.ProductionHelpExitCode, exitCode)
	}
	return nil
}

type verifiedBinary struct {
	record       BinaryRecord
	resolvedPath string
}

func verifyBinaryRecord(role string, recorded BinaryRecord, expectedRole BinaryRole, expectedCommit string) (verifiedBinary, error) {
	if recorded.Role != expectedRole {
		return verifiedBinary{}, fmt.Errorf("%w: %s role=%q", ErrRecordInvalid, role, recorded.Role)
	}
	resolved, err := canonicalPath(recorded.AbsolutePath)
	if err != nil {
		return verifiedBinary{}, fmt.Errorf("%s path: %w", role, err)
	}
	actual, err := BinaryRecordFromFile(resolved, expectedRole)
	if err != nil {
		if errors.Is(err, ErrModifiedBinary) || errors.Is(err, ErrEmptyEmbeddedModified) || errors.Is(err, ErrMissingEmbeddedModified) {
			return verifiedBinary{}, roleModifiedError(role)
		}
		if errors.Is(err, ErrMalformedEmbeddedRevision) || errors.Is(err, ErrMissingEmbeddedRevision) || errors.Is(err, ErrEmbeddedRevisionMismatch) {
			return verifiedBinary{}, roleRevisionError(role, err)
		}
		return verifiedBinary{}, fmt.Errorf("%s embedded authority: %w", role, err)
	}
	if actual.VCSRevision != expectedCommit {
		return verifiedBinary{}, roleRevisionError(role, fmt.Errorf("actual=%q expected=%q", actual.VCSRevision, expectedCommit))
	}
	if actual.VCSModified {
		return verifiedBinary{}, roleModifiedError(role)
	}
	if recorded.AbsolutePath != resolved {
		return verifiedBinary{}, fmt.Errorf("%w: %s path record=%q actual=%q", ErrRecordInvalid, role, recorded.AbsolutePath, resolved)
	}
	if recorded.Device != actual.Device || recorded.Inode != actual.Inode {
		return verifiedBinary{}, fmt.Errorf("%w: %s device/inode", ErrRecordInvalid, role)
	}
	if recorded.Size != actual.Size {
		return verifiedBinary{}, fmt.Errorf("%w: %s size", ErrRecordInvalid, role)
	}
	if recorded.SHA256 != actual.SHA256 {
		return verifiedBinary{}, fmt.Errorf("%w: %s sha256", ErrRecordInvalid, role)
	}
	if recorded.VCS != actual.VCS || recorded.VCSRevision != actual.VCSRevision || recorded.VCSTime != actual.VCSTime || recorded.VCSModified != actual.VCSModified {
		if recorded.VCSRevision != actual.VCSRevision {
			return verifiedBinary{}, roleRevisionError(role, fmt.Errorf("record=%q actual=%q", recorded.VCSRevision, actual.VCSRevision))
		}
		if recorded.VCSModified != actual.VCSModified {
			return verifiedBinary{}, roleModifiedError(role)
		}
		return verifiedBinary{}, fmt.Errorf("%w: %s embedded build info", ErrRecordInvalid, role)
	}
	return verifiedBinary{record: actual, resolvedPath: resolved}, nil
}

func roleRevisionError(role string, cause error) error {
	if role == "helper" {
		return fmt.Errorf("%w: %v", ErrHelperRevisionMismatch, cause)
	}
	return fmt.Errorf("%w: %v", ErrProductionRevisionMismatch, cause)
}

func roleModifiedError(role string) error {
	if role == "helper" {
		return ErrHelperModified
	}
	return ErrProductionModified
}

func readRecordFile(path string) ([]byte, error) {
	data, err := osReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read record %s: %w", path, err)
	}
	return data, nil
}

// These small variables keep file effects injectable in focused tests without
// weakening the production verifier's real-file path.
var osReadFile = func(path string) ([]byte, error) { return os.ReadFile(path) }

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func gitValue(root string, args ...string) (string, error) {
	full := append([]string{"-C", root}, args...)
	cmd := exec.Command("git", full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	value := strings.TrimSpace(string(out))
	if value == "" {
		return "", fmt.Errorf("git %s returned empty output", strings.Join(args, " "))
	}
	return value, nil
}
