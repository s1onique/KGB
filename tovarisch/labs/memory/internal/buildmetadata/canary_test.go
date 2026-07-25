package buildmetadata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validMetadata() CanaryImageBuild {
	return CanaryImageBuild{SchemaVersion: SchemaVersion, SourceCommit: strings.Repeat("a", 40), SourceTree: strings.Repeat("b", 40), CanarySourceTree: strings.Repeat("c", 40), RequestedReference: "kgb-tovarisch-canary:correction46-S46", EngineImageID: "sha256:" + strings.Repeat("d", 64), RepoDigests: []string{}, BuildKitManifestDigest: "sha256:" + strings.Repeat("e", 64), CanaryBinarySHA256: strings.Repeat("f", 64), CanaryVCSRevision: strings.Repeat("a", 40)}
}
func TestCanaryBuildMetadata_CurrentSourceOnly(t *testing.T) {
	m := validMetadata()
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	m.CanaryVCSRevision = strings.Repeat("9", 40)
	if err := m.Validate(); err == nil {
		t.Fatal("stale VCS accepted")
	}
}
func TestCanaryBuildMetadata_EngineImageIDExact(t *testing.T) {
	m := validMetadata()
	m.EngineImageID = m.RequestedReference
	if err := m.Validate(); err == nil {
		t.Fatal("tag accepted as engine ID")
	}
}
func TestCanaryBuildMetadata_RejectsStaleExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata.json")
	stale := validMetadata()
	stale.SourceCommit = strings.Repeat("1", 40)
	stale.CanaryVCSRevision = stale.SourceCommit
	stale.EngineImageID = "sha256:" + strings.Repeat("2", 64)
	if err := WriteAtomic(path, stale); err != nil {
		t.Fatal(err)
	}
	current := validMetadata()
	if err := WriteAtomic(path, current); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceCommit == stale.SourceCommit || got.EngineImageID == stale.EngineImageID {
		t.Fatal("stale identity survived replacement")
	}
}
func TestCanaryBuildMetadata_DistinguishesIndexAndEngineID(t *testing.T) {
	m := validMetadata()
	m.BuildKitIndexDigest = "sha256:" + strings.Repeat("1", 64)
	if m.EngineImageID == m.BuildKitIndexDigest {
		t.Fatal("identity classes conflated")
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
}
func TestCanaryBuildMetadata_AtomicReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata.json")
	if err := WriteAtomic(path, validMetadata()); err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".canary-image-build-*.tmp"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary files=%v err=%v", matches, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatal("metadata lacks final newline")
	}
}
