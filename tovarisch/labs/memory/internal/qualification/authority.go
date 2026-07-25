package qualification

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// BinaryAuthority is the physical Go build-information authority embedded in
// a binary. A successful value always has all four required VCS settings.
type BinaryAuthority struct {
	VCS         string
	VCSRevision string
	VCSTime     string
	VCSModified bool
}

// ReadEmbeddedBinaryAuthority reads build info directly from the named binary.
// It intentionally does not accept a source-commit fallback, an environment
// variable, an ldflag, or a caller-supplied claim.
func ReadEmbeddedBinaryAuthority(path string) (BinaryAuthority, error) {
	settings, err := readEmbeddedBinarySettings(path)
	if err != nil {
		return BinaryAuthority{}, err
	}
	return validateEmbeddedBinarySettings(settings)
}

func readEmbeddedBinarySettings(path string) (map[string]string, error) {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrBuildInfoRead, path, err)
	}
	settings := make(map[string]string, len(info.Settings))
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	return settings, nil
}

func validateEmbeddedBinarySettings(settings map[string]string) (BinaryAuthority, error) {
	vcs, ok := settings["vcs"]
	if !ok || vcs == "" {
		return BinaryAuthority{}, ErrMissingEmbeddedVCS
	}
	revision, ok := settings["vcs.revision"]
	if !ok || revision == "" {
		return BinaryAuthority{}, ErrMissingEmbeddedRevision
	}
	stamp, ok := settings["vcs.time"]
	if !ok || stamp == "" {
		return BinaryAuthority{}, ErrMissingEmbeddedTime
	}
	if _, err := time.Parse(time.RFC3339, stamp); err != nil {
		return BinaryAuthority{}, fmt.Errorf("%w: malformed vcs.time %q", ErrBuildInfoRead, stamp)
	}
	modified, ok := settings["vcs.modified"]
	if !ok {
		return BinaryAuthority{}, ErrMissingEmbeddedModified
	}
	if modified == "" {
		return BinaryAuthority{}, ErrEmptyEmbeddedModified
	}
	if modified != "false" {
		return BinaryAuthority{}, fmt.Errorf("%w: vcs.modified=%q", ErrModifiedBinary, modified)
	}
	if vcs != "git" {
		return BinaryAuthority{}, fmt.Errorf("%w: vcs=%q (expected git)", ErrBuildInfoRead, vcs)
	}
	if !ValidateObjectID(revision) {
		return BinaryAuthority{}, fmt.Errorf("%w: %q", ErrMalformedEmbeddedRevision, revision)
	}
	return BinaryAuthority{VCS: vcs, VCSRevision: revision, VCSTime: stamp, VCSModified: false}, nil
}

// ReadEmbeddedBinaryAuthorityForCommit is the strict expected-commit adapter
// used by the builder and verifier.
func ReadEmbeddedBinaryAuthorityForCommit(path, expectedCommit string) (BinaryAuthority, error) {
	authority, err := ReadEmbeddedBinaryAuthority(path)
	if err != nil {
		return BinaryAuthority{}, err
	}
	if !ValidateObjectID(expectedCommit) {
		return BinaryAuthority{}, fmt.Errorf("%w: expected source commit %q", ErrEmbeddedRevisionMismatch, expectedCommit)
	}
	if authority.VCSRevision != expectedCommit {
		return BinaryAuthority{}, fmt.Errorf("%w: got %q want %q", ErrEmbeddedRevisionMismatch, authority.VCSRevision, expectedCommit)
	}
	return authority, nil
}

// BinaryRecordFromFile reconstructs all binary and filesystem facts for a
// record. It resolves the path so symlink aliases cannot create fake identity.
func BinaryRecordFromFile(path string, role BinaryRole) (BinaryRecord, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return BinaryRecord{}, fmt.Errorf("absolute binary path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return BinaryRecord{}, fmt.Errorf("resolve binary path %s: %w", abs, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return BinaryRecord{}, fmt.Errorf("stat binary %s: %w", resolved, err)
	}
	if !info.Mode().IsRegular() {
		return BinaryRecord{}, fmt.Errorf("binary %s is not a regular file", resolved)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return BinaryRecord{}, fmt.Errorf("read binary %s: %w", resolved, err)
	}
	sum := sha256.Sum256(data)
	authority, err := ReadEmbeddedBinaryAuthority(resolved)
	if err != nil {
		return BinaryRecord{}, err
	}
	device, inode := statIdentity(info)
	return BinaryRecord{
		AbsolutePath: resolved,
		Device:       device,
		Inode:        inode,
		Size:         info.Size(),
		SHA256:       hex.EncodeToString(sum[:]),
		VCS:          authority.VCS,
		VCSRevision:  authority.VCSRevision,
		VCSTime:      authority.VCSTime,
		VCSModified:  authority.VCSModified,
		Role:         role,
	}, nil
}

func statIdentity(info os.FileInfo) (device, inode uint64) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0
	}
	return uint64(st.Dev), uint64(st.Ino)
}

// FileSHA256 is intentionally exported for evidence producers that need the
// same hash grammar as the verifier.
func FileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// TestListResult captures the exact executable-role probe output.
type TestListResult struct {
	Lines []string
	Raw   []byte
}

// RunExactHelperTestList invokes the Go test binary's own listing protocol.
// It never scans executable bytes for a test-name substring.
func RunExactHelperTestList(path, expected string) (TestListResult, error) {
	cmd := exec.Command(path, "-test.list", "^"+regexpQuote(expected)+"$")
	raw, err := cmd.CombinedOutput()
	result := TestListResult{Lines: nonEmptyLines(string(raw)), Raw: raw}
	if err != nil {
		return result, fmt.Errorf("%w: %v", ErrHelperTestMissing, err)
	}
	count := 0
	for _, line := range result.Lines {
		if line == expected {
			count++
		}
	}
	if count != 1 || len(result.Lines) != 1 {
		return result, fmt.Errorf("%w: expected exactly one line %q, got %v", ErrHelperTestMissing, expected, result.Lines)
	}
	return result, nil
}

func regexpQuote(s string) string {
	// The expected helper name is a Go identifier. Keep this local helper
	// dependency-free while still making the command's regex exact.
	return strings.NewReplacer("\\", "\\\\", ".", "\\.", "^", "\\^", "$", "\\$", "[", "\\[", "]", "\\]").Replace(s)
}

func nonEmptyLines(raw string) []string {
	var lines []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// RunProductionHelp executes the production role probe and returns its exact
// process exit code. A nonzero exit is never converted into success.
func RunProductionHelp(path string) (int, []byte, error) {
	cmd := exec.Command(path, "--help")
	raw, err := cmd.CombinedOutput()
	if err == nil {
		return 0, raw, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), raw, fmt.Errorf("%w: exit code %d", ErrProductionHelpFailure, exitErr.ExitCode())
	}
	return -1, raw, fmt.Errorf("%w: %v", ErrProductionHelpFailure, err)
}

func formatExitCode(code int) string { return strconv.Itoa(code) }
