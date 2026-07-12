package artifactio

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAtomicPublication_ChmodFailure_PreservesPrior(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode semantics")
	}
	ctx := seededAtomicTestContext(t)
	injected := errors.New("injected chmod failure")
	ops := defaultPublishOps()
	verifyCalled := false
	renameCalled := false
	ops.chmod = func(string, os.FileMode) error { return injected }
	ops.verifyMode = func(string, os.FileMode) error {
		verifyCalled = true
		return nil
	}
	ops.rename = func(string, string) error {
		renameCalled = true
		return nil
	}

	_, err := publishWithOps(ctx, []byte(`{"name":"new"}`), ops)
	assertAtomicFailure(t, ctx, err, "permission", injected)
	if verifyCalled {
		t.Error("mode verification ran after injected chmod failure")
	}
	if renameCalled {
		t.Error("rename ran after injected chmod failure")
	}
	assertPriorAndNoTemps(t, ctx.Destination)
}

func TestAtomicPublication_PreRenameModeVerifyFailure_PreservesPrior(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode semantics")
	}
	ctx := seededAtomicTestContext(t)
	injected := errors.New("injected mode verification failure")
	ops := defaultPublishOps()
	verifiedPath := ""
	renameCalled := false
	ops.verifyMode = func(path string, want os.FileMode) error {
		verifiedPath = path
		return injected
	}
	ops.rename = func(string, string) error {
		renameCalled = true
		return nil
	}

	_, err := publishWithOps(ctx, []byte(`{"name":"new"}`), ops)
	assertAtomicFailure(t, ctx, err, "permission", injected)
	if verifiedPath == "" {
		t.Fatal("injected mode verifier was not called")
	}
	if verifiedPath == ctx.Destination {
		t.Errorf("mode verifier received destination %q; want temporary path", verifiedPath)
	}
	if filepath.Dir(verifiedPath) != filepath.Dir(ctx.Destination) {
		t.Errorf("mode verifier path %q is not in destination directory", verifiedPath)
	}
	if renameCalled {
		t.Error("rename ran after injected pre-rename verification failure")
	}
	assertPriorAndNoTemps(t, ctx.Destination)
}

func TestAtomicPublication_RenameFailure_PreservesPrior(t *testing.T) {
	ctx := seededAtomicTestContext(t)
	injected := errors.New("injected rename failure")
	ops := defaultPublishOps()
	var stages []string
	realVerify := ops.verifyMode
	ops.verifyMode = func(path string, want os.FileMode) error {
		stages = append(stages, "verify")
		return realVerify(path, want)
	}
	ops.rename = func(oldPath, newPath string) error {
		stages = append(stages, "rename")
		if oldPath == ctx.Destination {
			t.Errorf("rename source = destination %q; want temporary path", oldPath)
		}
		if newPath != ctx.Destination {
			t.Errorf("rename destination = %q, want %q", newPath, ctx.Destination)
		}
		return injected
	}

	_, err := publishWithOps(ctx, []byte(`{"name":"new"}`), ops)
	assertAtomicFailure(t, ctx, err, "atomic_publish", injected)
	if got := strings.Join(stages, ","); got != "verify,rename" {
		t.Errorf("publication stages = %q, want verify,rename", got)
	}
	assertPriorAndNoTemps(t, ctx.Destination)
}

func seededAtomicTestContext(t *testing.T) *writeContext {
	t.Helper()
	artifactDir := filepath.Join(t.TempDir(), "artifacts")
	if err := os.MkdirAll(artifactDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dest := filepath.Join(artifactDir, "result.json")
	if err := os.WriteFile(dest, []byte("PRIOR"), 0o600); err != nil {
		t.Fatalf("seed prior artifact: %v", err)
	}
	return &writeContext{
		SurfaceID:   "test-surface",
		Destination: dest,
		Sanitizer:   "redact_structured_json",
		Policy:      DefaultRuntimePolicy(),
	}
}

func assertAtomicFailure(t *testing.T, ctx *writeContext, err error, category string, injected error) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected injected %s failure", category)
	}
	var artifactErr *Error
	if !errors.As(err, &artifactErr) {
		t.Fatalf("error type = %T, want *artifactio.Error", err)
	}
	if artifactErr.FailureCategory != category {
		t.Errorf("failure category = %q, want %q", artifactErr.FailureCategory, category)
	}
	if !errors.Is(err, injected) {
		t.Errorf("error %v does not wrap injected failure %v", err, injected)
	}
	if strings.Contains(err.Error(), `{"name":"new"}`) {
		t.Error("error message leaks payload")
	}
	if artifactErr.Destination != ctx.Destination {
		t.Errorf("error destination = %q, want %q", artifactErr.Destination, ctx.Destination)
	}
}

func assertPriorAndNoTemps(t *testing.T, dest string) {
	t.Helper()
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read prior artifact: %v", err)
	}
	if string(got) != "PRIOR" {
		t.Errorf("prior artifact = %q, want PRIOR", got)
	}
	entries, err := os.ReadDir(filepath.Dir(dest))
	if err != nil {
		t.Fatalf("read artifact directory: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tmp.") {
			t.Errorf("leftover temporary file %q", entry.Name())
		}
	}
}
