// artifacts_test.go — Tests for the external qualification
// artifact-root path contract. Each test creates a t.TempDir()
// outside the source checkout and verifies the path layout.

package qualification

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempOutsideRepo(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func TestQualificationArtifactPaths_ExternalRootAccepted(t *testing.T) {
	root := tempOutsideRepo(t)
	got, err := NewQualificationArtifactPaths(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Root == "" || got.HelperBinary == "" || got.ProductionBinary == "" {
		t.Fatalf("path contract returned empty fields: %+v", got)
	}
	for _, p := range []string{got.HelperBinary, got.ProductionBinary, got.HelperEvidence, got.ProductionEvidence, got.Metadata} {
		if !strings.HasPrefix(p, root) {
			t.Fatalf("path %q must be beneath root %q", p, root)
		}
	}
}

func TestQualificationArtifactPaths_RepositoryRootRejected(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewQualificationArtifactPaths(cwd); !errors.Is(err, ErrArtifactRootInvalid) {
		t.Fatalf("expected ErrArtifactRootInvalid for cwd, got %v", err)
	}
	sub := filepath.Join(cwd, "c48-subdir-test")
	if err := os.Mkdir(sub, 0o755); err == nil {
		defer os.RemoveAll(sub)
	}
	if _, err := NewQualificationArtifactPaths(sub); !errors.Is(err, ErrArtifactRootInvalid) {
		t.Fatalf("expected ErrArtifactRootInvalid for subdir, got %v", err)
	}
}

func TestQualificationArtifactPaths_ModuleRootRejected(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cur := cwd
	for cur != "/" && cur != "." {
		if _, statErr := os.Stat(filepath.Join(cur, ".git")); statErr == nil {
			break
		}
		cur = filepath.Dir(cur)
	}
	if cur == "/" || cur == "." {
		t.Skip("no .git ancestor in this environment")
	}
	if _, err := NewQualificationArtifactPaths(cur); !errors.Is(err, ErrArtifactRootInvalid) {
		t.Fatalf("expected ErrArtifactRootInvalid for module root %q, got %v", cur, err)
	}
}

func TestQualificationArtifactPaths_SymlinkIntoRepositoryRejected(t *testing.T) {
	root := tempOutsideRepo(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "into-repo")
	if err := os.Symlink(cwd, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := NewQualificationArtifactPaths(link); !errors.Is(err, ErrArtifactRootInvalid) {
		t.Fatalf("expected ErrArtifactRootInvalid for symlink into repository, got %v", err)
	}
}

func TestQualificationArtifactPaths_HelperProductionPathsDistinct(t *testing.T) {
	root := tempOutsideRepo(t)
	got, err := NewQualificationArtifactPaths(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.HelperBinary == got.ProductionBinary {
		t.Fatal("helper and production paths must be distinct")
	}
	if got.HelperEvidence == got.ProductionEvidence {
		t.Fatal("helper and production evidence paths must be distinct")
	}
}

func TestQualificationArtifactPaths_NonExistentRootRejected(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(cwd, "does-not-exist-"+filepath.Base(t.TempDir()))
	if _, err := NewQualificationArtifactPaths(root); !errors.Is(err, ErrArtifactRootInvalid) {
		t.Fatalf("expected ErrArtifactRootInvalid, got %v", err)
	}
}

func TestQualificationArtifactPaths_EmptyRootRejected(t *testing.T) {
	if _, err := NewQualificationArtifactPaths(""); !errors.Is(err, ErrArtifactRootInvalid) {
		t.Fatalf("expected ErrArtifactRootInvalid for empty root, got %v", err)
	}
}
