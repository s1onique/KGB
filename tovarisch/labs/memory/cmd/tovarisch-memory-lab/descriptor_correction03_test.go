// descriptor_correction03_test.go — CORRECTION03 mandatory
// canary-image-provenance mutation matrix.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mutateDescriptorSII mutates the descriptor fixture's
// subject_image_identity block in the manifest and asserts
// the verifier emits a specific diagnostic.
func mutateDescriptorSII(t *testing.T, mutate func(*map[string]interface{}), expectDiagnostic string) {
	t.Helper()
	dst := t.TempDir()
	boundDir := copyFixture(t, dst, descriptorFixtureDir, descriptorFixtureRunID)
	rebindFixture(t, boundDir)

	manifestPath := filepath.Join(boundDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	sii, ok := m["subject_image_identity"].(map[string]interface{})
	if !ok {
		t.Fatalf("manifest.subject_image_identity is missing in fixture")
	}
	mutate(&sii)
	m["subject_image_identity"] = sii
	rewritten, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(manifestPath, rewritten, 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	recomputeChecksumsFor(t, boundDir)

	out, err := runDescriptorVerifier(t, dst)
	if err == nil {
		t.Fatalf("mutation did not cause verifier failure; output:\n%s", out)
	}
	if !strings.Contains(out, expectDiagnostic) {
		t.Errorf("mutation produced wrong diagnostic.\nexpected: %q\nactual:\n%s", expectDiagnostic, out)
	}
}

// TestDescriptorSII_MutatedContainerImageID rejects a
// mismatched container_image_id.
func TestDescriptorSII_MutatedContainerImageID(t *testing.T) {
	mutateDescriptorSII(t, func(sii *map[string]interface{}) {
		(*sii)["container_image_id"] = "sha256:" + strings.Repeat("1", 64)
	}, "subject_image_identity.container_image_id=")
}

// TestDescriptorSII_MutatedSourceCommit rejects a mismatched
// source_commit_oid.
func TestDescriptorSII_MutatedSourceCommit(t *testing.T) {
	mutateDescriptorSII(t, func(sii *map[string]interface{}) {
		(*sii)["source_commit_oid"] = strings.Repeat("1", 40)
	}, "subject_image_identity.source_commit_oid=")
}

// TestDescriptorSII_MutatedRepositoryTree rejects a
// mismatched repository_tree_oid.
func TestDescriptorSII_MutatedRepositoryTree(t *testing.T) {
	mutateDescriptorSII(t, func(sii *map[string]interface{}) {
		(*sii)["repository_tree_oid"] = strings.Repeat("1", 40)
	}, "subject_image_identity.repository_tree_oid=")
}

// TestDescriptorSII_MutatedCanarySourceSubtree rejects a
// mismatched canary_source_subtree_oid.
func TestDescriptorSII_MutatedCanarySourceSubtree(t *testing.T) {
	mutateDescriptorSII(t, func(sii *map[string]interface{}) {
		(*sii)["canary_source_subtree_oid"] = strings.Repeat("1", 40)
	}, "subject_image_identity.source_subtree_label=")
}

// TestDescriptorSII_MutatedPrebuildHash rejects a mismatched
// prebuild binary hash.
func TestDescriptorSII_MutatedPrebuildHash(t *testing.T) {
	mutateDescriptorSII(t, func(sii *map[string]interface{}) {
		(*sii)["prebuild_binary_sha256"] = "1" + strings.Repeat("0", 63)
	}, "subject_image_identity prebuild=")
}

// TestDescriptorSII_MutatedExtractedHash rejects a mismatched
// extracted binary hash (must equal prebuild).
func TestDescriptorSII_MutatedExtractedHash(t *testing.T) {
	mutateDescriptorSII(t, func(sii *map[string]interface{}) {
		(*sii)["extracted_image_binary_sha256"] = "2" + strings.Repeat("0", 63)
	}, "subject_image_identity prebuild=")
}

// TestDescriptorSII_MutatedRevisionLabel rejects a mismatched
// revision label.
func TestDescriptorSII_MutatedRevisionLabel(t *testing.T) {
	mutateDescriptorSII(t, func(sii *map[string]interface{}) {
		(*sii)["revision_label"] = strings.Repeat("1", 40)
	}, "subject_image_identity.revision_label=")
}

// TestDescriptorSII_MutatedRepositoryTreeLabel rejects a
// mismatched repository_tree_label.
func TestDescriptorSII_MutatedRepositoryTreeLabel(t *testing.T) {
	mutateDescriptorSII(t, func(sii *map[string]interface{}) {
		(*sii)["repository_tree_label"] = strings.Repeat("1", 40)
	}, "subject_image_identity.repository_tree_label=")
}

// TestDescriptorSII_MutatedSourceSubtreeLabel rejects a
// mismatched source_subtree_label.
func TestDescriptorSII_MutatedSourceSubtreeLabel(t *testing.T) {
	mutateDescriptorSII(t, func(sii *map[string]interface{}) {
		(*sii)["source_subtree_label"] = strings.Repeat("1", 40)
	}, "subject_image_identity.source_subtree_label=")
}

// TestDescriptorSII_MutatedBinaryHashLabel rejects a mismatched
// binary_sha256_label.
func TestDescriptorSII_MutatedBinaryHashLabel(t *testing.T) {
	mutateDescriptorSII(t, func(sii *map[string]interface{}) {
		(*sii)["binary_sha256_label"] = "3" + strings.Repeat("0", 63)
	}, "subject_image_identity prebuild=")
}

// TestDescriptorSII_MutatedRepoDigest rejects a
// repo_digest_status inconsistent with non-empty repo_digests.
func TestDescriptorSII_MutatedRepoDigest(t *testing.T) {
	mutateDescriptorSII(t, func(sii *map[string]interface{}) {
		(*sii)["repo_digests"] = []string{"kgb-tovarisch-canary@sha256:" + strings.Repeat("9", 64)}
	}, `subject_image_identity.repo_digest_status="unavailable_local_image" inconsistent`)
}

// TestDescriptorSII_MutatedRepoDigestStatus rejects a
// repo_digest_status that disagrees with repo_digests.
func TestDescriptorSII_MutatedRepoDigestStatus(t *testing.T) {
	mutateDescriptorSII(t, func(sii *map[string]interface{}) {
		(*sii)["repo_digests"] = []string{}
		(*sii)["repo_digest_status"] = "available"
	}, `subject_image_identity.repo_digest_status="available" inconsistent`)
}

// TestDescriptorSII_RemovedFromManifest requires the
// subject_image_identity block; removing it must fail.
func TestDescriptorSII_RemovedFromManifest(t *testing.T) {
	dst := t.TempDir()
	boundDir := copyFixture(t, dst, descriptorFixtureDir, descriptorFixtureRunID)
	rebindFixture(t, boundDir)

	manifestPath := filepath.Join(boundDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	delete(m, "subject_image_identity")
	rewritten, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(manifestPath, rewritten, 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	recomputeChecksumsFor(t, boundDir)

	out, err := runDescriptorVerifier(t, dst)
	if err == nil {
		t.Fatalf("mutation did not cause verifier failure; output:\n%s", out)
	}
	if !strings.Contains(out, "manifest.subject_image_identity is missing") {
		t.Errorf("expected missing-block diagnostic; got:\n%s", out)
	}
}

// TestDescriptorSII_AddedSidecarRejected rejects the old
// unchecked canary-image-provenance.json sidecar.
func TestDescriptorSII_AddedSidecarRejected(t *testing.T) {
	dst := t.TempDir()
	boundDir := copyFixture(t, dst, descriptorFixtureDir, descriptorFixtureRunID)
	rebindFixture(t, boundDir)

	manifestPath := filepath.Join(boundDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	delete(m, "subject_image_identity")
	rewritten, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(manifestPath, rewritten, 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	sidecarPath := filepath.Join(boundDir, "canary-image-provenance.json")
	if err := os.WriteFile(sidecarPath, []byte(`{"image_id":"sha256:00"}`), 0644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	recomputeChecksumsFor(t, boundDir)

	out, err := runDescriptorVerifier(t, dst)
	if err == nil {
		t.Fatalf("mutation did not cause verifier failure; output:\n%s", out)
	}
	if !strings.Contains(out, "canary-image-provenance.json is in the artifact directory") {
		t.Errorf("expected sidecar rejection diagnostic; got:\n%s", out)
	}
}

// TestDescriptorSII_UndeclaredProvenanceFileRejected rejects
// files not in the canonical inventory.
func TestDescriptorSII_UndeclaredProvenanceFileRejected(t *testing.T) {
	dst := t.TempDir()
	boundDir := copyFixture(t, dst, descriptorFixtureDir, descriptorFixtureRunID)
	rebindFixture(t, boundDir)
	extraPath := filepath.Join(boundDir, "unrecognized.json")
	if err := os.WriteFile(extraPath, []byte(`{"x":1}`), 0644); err != nil {
		t.Fatalf("write extra: %v", err)
	}
	recomputeChecksumsFor(t, boundDir)

	out, err := runDescriptorVerifier(t, dst)
	if err == nil {
		t.Fatalf("mutation did not cause verifier failure; output:\n%s", out)
	}
	if !strings.Contains(out, "unexpected file not in inventory") {
		t.Errorf("expected undeclared-file diagnostic; got:\n%s", out)
	}
}

// TestDescriptorSII_FieldChangedWithoutChecksumRepair detects
// a manifest field change without recomputing checksums.
func TestDescriptorSII_FieldChangedWithoutChecksumRepair(t *testing.T) {
	dst := t.TempDir()
	boundDir := copyFixture(t, dst, descriptorFixtureDir, descriptorFixtureRunID)
	rebindFixture(t, boundDir)

	manifestPath := filepath.Join(boundDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	sii := m["subject_image_identity"].(map[string]interface{})
	sii["image_id"] = "sha256:" + strings.Repeat("1", 64)
	m["subject_image_identity"] = sii
	rewritten, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(manifestPath, rewritten, 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	// Intentionally do NOT recompute checksums.

	out, err := runDescriptorVerifier(t, dst)
	if err == nil {
		t.Fatalf("mutation did not cause verifier failure; output:\n%s", out)
	}
	if !strings.Contains(out, "checksum mismatch") {
		t.Errorf("expected checksum-mismatch diagnostic; got:\n%s", out)
	}
}
