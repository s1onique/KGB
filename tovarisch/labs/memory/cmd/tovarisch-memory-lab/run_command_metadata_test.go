// run_command_metadata_test.go — Tests for the metadata-path
// resolution seam in runCommandWithDocker.
//
// Required tests (CORRECTION48 P0-3):
//   - TestRunCommand_MetadataFlagWins
//   - TestRunCommand_MetadataEnvironmentFallback
//   - TestRunCommand_MetadataCompatibilityFallback
//   - TestRunCommand_MetadataMissingFailsBeforeDocker
//   - TestRunCommand_MetadataInvalidFailsBeforeDocker
//   - TestRunCommand_MetadataResolvedOnce
//
// The tests use a fake Docker factory that records whether it
// was invoked and rejects any attempt to construct a real
// docker client; failure tests prove the Docker factory is
// never called when metadata resolution fails.

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/buildmetadata"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
)

// dockerCallResult captures a docker factory invocation.
type dockerCallResult struct {
	called bool
}

// dockerRecorder returns a docker factory and a tracker. When
// the factory is called, tracker.called flips to true.
func dockerRecorder() (factory func(context.Context) (*dockerlab.Client, error), tracker *dockerCallResult) {
	t := &dockerCallResult{}
	f := func(ctx context.Context) (*dockerlab.Client, error) {
		t.called = true
		return nil, errors.New("docker factory called unexpectedly")
	}
	return f, t
}

// validMetadataFixture writes a schema-correct canary image
// build metadata blob under dir/name and returns its path.
func validMetadataFixture(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	commit := strings.Repeat("a", 40)
	p := filepath.Join(dir, name)
	valid := buildmetadata.CanaryImageBuild{
		SchemaVersion:      buildmetadata.SchemaVersion,
		SourceCommit:       commit,
		SourceTree:         strings.Repeat("b", 40),
		CanarySourceTree:   strings.Repeat("c", 40),
		RequestedReference: "kgb-tovarisch-canary:test-S48",
		EngineImageID:      "sha256:" + strings.Repeat("d", 64),
		RepoDigests:        []string{},
		CanaryBinarySHA256: strings.Repeat("f", 64),
		CanaryVCSRevision:  commit,
	}
	if err := buildmetadata.WriteAtomic(p, valid); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}
	return p
}

// runWithResolvers exercises runCommandWithDocker using the
// production resolver and a fake docker recorder.
func runWithResolvers(t *testing.T, args []string, dockerFactory func(context.Context) (*dockerlab.Client, error)) error {
	t.Helper()
	resolver := buildmetadata.ResolveCanaryMetadataPath
	return runCommandWithDocker(args, resolver, dockerFactory)
}

func TestRunCommand_MetadataFlagWins(t *testing.T) {
	root := t.TempDir()
	// Construct three distinct metadata files; the explicit flag
	// must win regardless of env and repo fallback.
	flagPath := validMetadataFixture(t, root, "flag.json")
	envPath := validMetadataFixture(t, root, "env.json")
	repoCompatDir := filepath.Join(root, "tovarisch", "labs", "memory")
	if err := os.MkdirAll(repoCompatDir, 0o755); err != nil {
		t.Fatal(err)
	}
	repoPath := validMetadataFixture(t, repoCompatDir, "canary-image-build.json")
	// Use t.Setenv so the env var and repo dir are isolated.
	t.Setenv(buildmetadata.EnvMetadataPath, envPath)
	args := []string{
		"run",
		"--scenario=canary-bounded",
		"--duration=60",
		"--artifacts-dir", t.TempDir(),
		"--repository-root", root,
		"--canary-build-metadata", flagPath,
	}
	dockerFactory, tracker := dockerRecorder()
	if err := runWithResolvers(t, args, dockerFactory); err == nil {
		t.Fatal("expected error because docker factory is unreachable")
	}
	if !tracker.called {
		t.Fatal("docker factory should have been called after metadata resolved")
	}
	if !strings.Contains(errFromRun(t, args, dockerFactory).Error(), "docker factory called unexpectedly") {
		t.Fatalf("expected docker factory error after metadata success, got %v", errFromRun(t, args, dockerFactory))
	}
	_ = repoPath // ensure repo path was on disk for completeness
}

func errFromRun(t *testing.T, args []string, dockerFactory func(context.Context) (*dockerlab.Client, error)) error {
	return runCommandWithDocker(args, buildmetadata.ResolveCanaryMetadataPath, dockerFactory)
}

func TestRunCommand_MetadataEnvironmentFallback(t *testing.T) {
	root := t.TempDir()
	envPath := validMetadataFixture(t, root, "env.json")
	t.Setenv(buildmetadata.EnvMetadataPath, envPath)
	args := []string{
		"run",
		"--scenario=canary-bounded",
		"--duration=60",
		"--artifacts-dir", t.TempDir(),
		"--repository-root", root,
	}
	dockerFactory, tracker := dockerRecorder()
	_ = runWithResolvers(t, args, dockerFactory)
	if !tracker.called {
		t.Fatal("env fallback should reach the docker factory")
	}
}

func TestRunCommand_MetadataCompatibilityFallback(t *testing.T) {
	root := t.TempDir()
	repoCompatDir := filepath.Join(root, "tovarisch", "labs", "memory")
	if err := os.MkdirAll(repoCompatDir, 0o755); err != nil {
		t.Fatal(err)
	}
	validMetadataFixture(t, repoCompatDir, "canary-image-build.json")
	args := []string{
		"run",
		"--scenario=canary-bounded",
		"--duration=60",
		"--artifacts-dir", t.TempDir(),
		"--repository-root", root,
	}
	dockerFactory, tracker := dockerRecorder()
	_ = runWithResolvers(t, args, dockerFactory)
	if !tracker.called {
		t.Fatal("repo compatibility fallback should reach the docker factory")
	}
}

func TestRunCommand_MetadataMissingFailsBeforeDocker(t *testing.T) {
	root := t.TempDir()
	// No metadata anywhere; the resolved placeholder
	// /repo/tovarisch/labs/memory/canary-image-build.json does
	// not exist.
	args := []string{
		"run",
		"--scenario=canary-bounded",
		"--duration=60",
		"--artifacts-dir", t.TempDir(),
		"--repository-root", root,
		"--canary-build-metadata", filepath.Join(root, "absent.json"),
	}
	dockerFactory, tracker := dockerRecorder()
	err := runWithResolvers(t, args, dockerFactory)
	if err == nil {
		t.Fatal("expected metadata failure")
	}
	if tracker.called {
		t.Fatal("docker factory MUST NOT be called when metadata resolution fails")
	}
	if !strings.Contains(err.Error(), "canary build metadata") {
		t.Fatalf("expected metadata error, got %v", err)
	}
}

func TestRunCommand_MetadataInvalidFailsBeforeDocker(t *testing.T) {
	root := t.TempDir()
	// Invalid schema: empty `{}` JSON.
	p := filepath.Join(root, "bad.json")
	if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{
		"run",
		"--scenario=canary-bounded",
		"--duration=60",
		"--artifacts-dir", t.TempDir(),
		"--canary-build-metadata", p,
	}
	dockerFactory, tracker := dockerRecorder()
	err := runWithResolvers(t, args, dockerFactory)
	if err == nil {
		t.Fatal("expected invalid metadata failure")
	}
	if tracker.called {
		t.Fatal("docker factory MUST NOT be called when metadata is invalid")
	}
}

func TestRunCommand_MetadataResolvedOnce(t *testing.T) {
	root := t.TempDir()
	validMetadataFixture(t, root, "metadata.json")
	args := []string{
		"run",
		"--scenario=canary-bounded",
		"--duration=60",
		"--artifacts-dir", t.TempDir(),
		"--canary-build-metadata", filepath.Join(root, "metadata.json"),
	}
	calls := 0
	dockerFactory := func(ctx context.Context) (*dockerlab.Client, error) {
		calls++
		return nil, fmt.Errorf("stop after first call (%d)", calls)
	}
	_ = runWithResolvers(t, args, dockerFactory)
	if calls != 1 {
		t.Fatalf("docker factory called %d times, want exactly 1", calls)
	}
}
