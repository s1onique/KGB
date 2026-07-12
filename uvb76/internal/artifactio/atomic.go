package artifactio

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// PublicationResult captures the outcome of an atomic publication step.
type PublicationResult struct {
	// FinalPath is the published final artifact path.
	FinalPath string
	// TempPath is the temporary file used during publication. Empty after rename.
	TempPath string
	// RolledBack indicates whether the final published file was rolled back.
	RolledBack bool
}

// publishResult is the legacy lowercase alias.
type publishResult = PublicationResult

// publish writes sanitized bytes to a same-directory temporary file and
// renames it to the destination path only after its final mode is verified.
// Every pre-rename failure removes the temporary file and leaves an existing
// destination untouched.
func publish(parentCtx *writeContext, sanitized []byte) (*PublicationResult, error) {
	return publishWithOps(parentCtx, sanitized, defaultPublishOps())
}

type publishOps struct {
	chmod      func(string, os.FileMode) error
	verifyMode func(string, os.FileMode) error
	rename     func(string, string) error
	remove     func(string) error
}

func defaultPublishOps() publishOps {
	return publishOps{
		chmod:      os.Chmod,
		verifyMode: verifyFinalMode,
		rename:     os.Rename,
		remove:     os.Remove,
	}
}

// publishWithOps exposes deterministic stage seams to package tests without
// replacing process-global filesystem functions.
func publishWithOps(parentCtx *writeContext, sanitized []byte, ops publishOps) (*PublicationResult, error) {
	if err := parentCtx.Policy.validate(); err != nil {
		return nil, newError(parentCtx, "policy_invalid", err)
	}
	if len(sanitized) > parentCtx.Policy.MaxOutputBytes {
		return nil, newError(parentCtx, "output_too_large",
			fmt.Errorf("sanitized output size %d exceeds MaxOutputBytes %d",
				len(sanitized), parentCtx.Policy.MaxOutputBytes))
	}
	if len(sanitized) == 0 {
		return nil, newError(parentCtx, "output_too_large",
			fmt.Errorf("sanitized output is empty"))
	}
	dest := parentCtx.Destination
	destDir := filepath.Dir(dest)
	if destDir == "" {
		return nil, newError(parentCtx, "io",
			fmt.Errorf("destination has empty directory"))
	}
	if err := ensureDir(destDir); err != nil {
		return nil, newError(parentCtx, "io", err)
	}

	base := filepath.Base(dest)
	tmpPath, tmpFile, err := createTempFile(destDir, base)
	if err != nil {
		return nil, newError(parentCtx, "io", err)
	}

	// cleanup removes the temp file and closes if still open.
	cleanup := func() {
		if tmpFile != nil {
			_ = tmpFile.Close()
			tmpFile = nil
		}
		if tmpPath != "" {
			_ = ops.remove(tmpPath)
			tmpPath = ""
		}
	}

	if _, err := tmpFile.Write(sanitized); err != nil {
		cleanup()
		return nil, newError(parentCtx, "io", err)
	}
	if err := tmpFile.Sync(); err != nil {
		cleanup()
		return nil, newError(parentCtx, "io", err)
	}
	if err := tmpFile.Close(); err != nil {
		cleanup()
		return nil, newError(parentCtx, "io", err)
	}
	tmpFile = nil

	if err := ops.chmod(tmpPath, parentCtx.Policy.FileMode); err != nil {
		cleanup()
		return nil, newError(parentCtx, "permission", err)
	}
	if err := ops.verifyMode(tmpPath, parentCtx.Policy.FileMode); err != nil {
		cleanup()
		return nil, newError(parentCtx, "permission",
			fmt.Errorf("pre-rename mode verification failed: %w", err))
	}
	if err := ops.rename(tmpPath, dest); err != nil {
		cleanup()
		return nil, newError(parentCtx, "atomic_publish", err)
	}
	tmpPath = "" // success, no cleanup needed
	return &PublicationResult{FinalPath: dest, RolledBack: false}, nil
}

// writeContext carries the minimum context the atomic-publish helper needs.
type writeContext struct {
	SurfaceID   string
	Destination string
	Sanitizer   string
	Policy      WritePolicy
}

func newError(ctx *writeContext, category string, wrapped error) *Error {
	return &Error{
		SurfaceID:       ctx.SurfaceID,
		Destination:     ctx.Destination,
		Sanitizer:       ctx.Sanitizer,
		FailureCategory: category,
		Wrapped:         wrapped,
	}
}

func newErrorWithRule(ctx *writeContext, category, ruleID, fieldPath string, wrapped error) *Error {
	e := newError(ctx, category, wrapped)
	e.RuleID = ruleID
	e.FieldPath = fieldPath
	return e
}

// ensureDir ensures the directory exists with restrictive permissions.
func ensureDir(dir string) error {
	if _, err := os.Stat(dir); err == nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return nil
}

// createTempFile creates a new temporary file in dir using os.CreateTemp so
// the directory listing never leaks the chosen name prefix.
//
// The base name carries a leading-dot plus suffix so the final rename is to a
// regular artifact name. CreateTemp returns a path matching the chosen pattern
// plus six random bytes.
func createTempFile(dir, base string) (string, *os.File, error) {
	if dir == "" {
		return "", nil, errors.New("empty destination directory")
	}
	if base == "" {
		base = "artifact"
	}
	pattern := ".tmp." + base + ".*"
	tmpFile, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", nil, err
	}
	// Apply restrictive permissions immediately. CreateTemp respects umask,
	// so we explicitly set 0o600 to avoid accidental world-readability.
	if err := os.Chmod(tmpFile.Name(), 0o600); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return "", nil, err
	}
	return tmpFile.Name(), tmpFile, nil
}

// copyAndRemove was previously used by the test fallback path. The R4R1
// harden removed it from the production publication path; it remains here
// only as a private helper for tests that opt out of atomic publication.
func copyAndRemove(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}

// verifyFinalMode asserts the final file mode matches the policy. Returns
// an error if a mode mismatch is detected on Unix.
func verifyFinalMode(path string, want os.FileMode) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	got := info.Mode().Perm()
	if got != want.Perm() {
		return fmt.Errorf("artifactio: expected mode %s on %s, got %s",
			modeStr(want.Perm()), path, modeStr(got))
	}
	return nil
}

// modeStr formats a Unix mode for diagnostics.
func modeStr(m os.FileMode) string {
	const oct = "01234567"
	if m == 0 {
		return "0"
	}
	var digits [3]byte
	v := uint64(m)
	for i := 2; i >= 0; i-- {
		digits[i] = oct[v&7]
		v >>= 3
	}
	return "0" + string(digits[:])
}
