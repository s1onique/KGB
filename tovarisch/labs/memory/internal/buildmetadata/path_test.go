// path_test.go — Tests for ResolveCanaryMetadataPath.
//
// Every fixture uses a temporary directory under t.TempDir so the
// tests do not rely on repository layout. The valid-metadata
// helper writes a schema-correct CanaryImageBuild via
// buildmetadata.WriteAtomic so the resolver's verifyValid step
// has real content to parse, not a hand-crafted JSON blob.

package buildmetadata

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func writeValidMetadata(t *testing.T, dir, name string) string {
	t.Helper()
	commit := strings.Repeat("a", 40)
	tree := strings.Repeat("b", 40)
	canaryTree := strings.Repeat("c", 40)
	valid := CanaryImageBuild{
		SchemaVersion:      SchemaVersion,
		SourceCommit:       commit,
		SourceTree:         tree,
		CanarySourceTree:   canaryTree,
		RequestedReference: "kgb-tovarisch-canary:correction48",
		EngineImageID:      "sha256:" + strings.Repeat("d", 64),
		RepoDigests:        []string{},
		CanaryBinarySHA256: strings.Repeat("f", 64),
		CanaryVCSRevision:  commit,
	}
	p := filepath.Join(dir, name)
	if err := WriteAtomic(p, valid); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}
	return p
}

func absPath(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatalf("abs(%q): %v", p, err)
	}
	return abs
}

func TestCanaryMetadataPath_ExplicitWins(t *testing.T) {
	root := t.TempDir()
	explicit := writeValidMetadata(t, root, "explicit.json")
	env := writeValidMetadata(t, root, "env.json")
	repo := writeValidMetadata(t, root, "repo.json")
	got, err := ResolveCanaryMetadataPath(MetadataPathOptions{
		ExplicitPath: explicit,
		Environment:  env,
		Repository:   filepath.Dir(repo),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := absPath(t, explicit)
	if got != want {
		t.Fatalf("explicit path not honored: got %q want %q", got, want)
	}
}

func TestCanaryMetadataPath_EnvironmentFallback(t *testing.T) {
	root := t.TempDir()
	env := writeValidMetadata(t, root, "env.json")
	got, err := ResolveCanaryMetadataPath(MetadataPathOptions{
		Environment: env,
		Repository:  root,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := absPath(t, env)
	if got != want {
		t.Fatalf("environment path not honored: got %q want %q", got, want)
	}
}

func TestCanaryMetadataPath_RepositoryCompatibilityFallback(t *testing.T) {
	repoRoot := t.TempDir()
	repoCompatDir := filepath.Join(repoRoot, "tovarisch", "labs", "memory")
	if err := os.MkdirAll(repoCompatDir, 0o755); err != nil {
		t.Fatal(err)
	}
	compat := writeValidMetadata(t, repoCompatDir, "canary-image-build.json")
	got, err := ResolveCanaryMetadataPath(MetadataPathOptions{Repository: repoRoot})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := absPath(t, compat)
	if got != want {
		t.Fatalf("repository compatibility path not honored: got %q want %q", got, want)
	}
}

func TestCanaryMetadataPath_ExplicitMissingFails(t *testing.T) {
	root := t.TempDir()
	env := writeValidMetadata(t, root, "env.json")
	repo := writeValidMetadata(t, root, "repo.json")
	missing := filepath.Join(root, "not-on-disk.json")
	_, err := ResolveCanaryMetadataPath(MetadataPathOptions{
		ExplicitPath: missing,
		Environment:  env,
		Repository:   filepath.Dir(repo),
	})
	if err == nil {
		t.Fatal("expected error for missing explicit path")
	}
	if errors.Is(err, ErrMetadataUnresolved) {
		t.Fatalf("explicit-missing must NOT degrade to unresolved sentinel: %v", err)
	}
}

func TestCanaryMetadataPath_EnvironmentMissingFails(t *testing.T) {
	root := t.TempDir()
	repo := writeValidMetadata(t, root, "repo.json")
	_, err := ResolveCanaryMetadataPath(MetadataPathOptions{
		Environment: filepath.Join(root, "env-missing.json"),
		Repository:  filepath.Dir(repo),
	})
	if err == nil {
		t.Fatal("expected error for missing environment path")
	}
	if errors.Is(err, ErrMetadataUnresolved) {
		t.Fatalf("env-missing must NOT degrade to unresolved sentinel: %v", err)
	}
}

func TestCanaryMetadataPath_DirectoryRejected(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "not-a-file")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveCanaryMetadataPath(MetadataPathOptions{ExplicitPath: dir})
	if err == nil {
		t.Fatal("directory was accepted as metadata path")
	}
	if !errors.Is(err, ErrMetadataNotRegular) {
		t.Fatalf("expected ErrMetadataNotRegular, got %v", err)
	}
}

func TestCanaryMetadataPath_SymlinkResolved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks unsupported on windows CI")
	}
	root := t.TempDir()
	target := writeValidMetadata(t, root, "target.json")
	link := filepath.Join(root, "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveCanaryMetadataPath(MetadataPathOptions{ExplicitPath: link})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := absPath(t, target)
	if got != want {
		t.Fatalf("symlink not resolved: got %q want %q", got, want)
	}
}

func TestCanaryMetadataPath_BrokenSymlinkRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks unsupported on windows CI")
	}
	root := t.TempDir()
	link := filepath.Join(root, "broken.json")
	target := filepath.Join(root, "no-such-target.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveCanaryMetadataPath(MetadataPathOptions{ExplicitPath: link})
	if err == nil {
		t.Fatal("broken symlink was accepted as metadata source")
	}
	if !errors.Is(err, ErrMetadataBrokenSymlink) {
		t.Fatalf("expected ErrMetadataBrokenSymlink, got %v", err)
	}
}

func TestCanaryMetadataPath_NonRegularFileRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mkfifo unsupported on windows CI")
	}
	root := t.TempDir()
	fifo := filepath.Join(root, "fifo.json")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	_, err := ResolveCanaryMetadataPath(MetadataPathOptions{ExplicitPath: fifo})
	if err == nil {
		t.Fatal("FIFO was accepted as metadata source")
	}
	if !errors.Is(err, ErrMetadataNotRegular) {
		t.Fatalf("expected ErrMetadataNotRegular, got %v", err)
	}
}

func TestCanaryMetadataPath_UnknownSchemaRejected(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "unknown-schema.json")
	if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveCanaryMetadataPath(MetadataPathOptions{ExplicitPath: p})
	if err == nil {
		t.Fatal("unknown schema was accepted")
	}
	if !errors.Is(err, ErrMetadataInvalid) {
		t.Fatalf("expected ErrMetadataInvalid, got %v", err)
	}
}

func TestCanaryMetadataPath_InvalidMetadataRejected(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "bad-identity.json")
	// Valid schema_version but malformed OID identities so
	// Validate() rejects the doc after Read() succeeds.
	blob := `{"schema_version":"canary-image-build/v2","source_commit":"not-hex","source_tree":"` +
		strings.Repeat("b", 40) + `","canary_source_tree":"` +
		strings.Repeat("c", 40) + `","requested_reference":"foo","engine_image_id":"sha256:` +
		strings.Repeat("d", 64) + `","repo_digests":[],"buildkit_manifest_digest":"","buildkit_index_digest":"","canary_binary_sha256":"` +
		strings.Repeat("f", 64) + `","canary_vcs_revision":"not-hex"}`
	if err := os.WriteFile(p, []byte(blob), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveCanaryMetadataPath(MetadataPathOptions{ExplicitPath: p})
	if err == nil {
		t.Fatal("malformed identity was accepted")
	}
	if !errors.Is(err, ErrMetadataInvalid) {
		t.Fatalf("expected ErrMetadataInvalid, got %v", err)
	}
}
