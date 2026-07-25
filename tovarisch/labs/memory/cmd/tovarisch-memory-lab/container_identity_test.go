// container_identity_test.go — Canonical Container Identity Tests
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03

package main

import (
	"os"
	"testing"
)

func TestProjectContainerIdentity_ExtractsFromInspect(t *testing.T) {
	inspectData := []byte(`[{
		"Id": "c6636cb0590ed65a30ae1682af98b50e5ae8ba5b784a9abaac1f8ccb6413e3bf",
		"Name": "/tovarisch-subject-lab-canary-bounded-1784624046",
		"Created": "2026-07-21T08:54:06.92141871Z",
		"State": {"Running": true}
	}]`)

	identity, err := ProjectContainerIdentity(inspectData)
	if err != nil {
		t.Fatalf("ProjectContainerIdentity failed: %v", err)
	}

	if identity.SchemaVersion != "1.0.0" {
		t.Errorf("expected schema_version 1.0.0, got %q", identity.SchemaVersion)
	}
	if identity.ID != "c6636cb0590ed65a30ae1682af98b50e5ae8ba5b784a9abaac1f8ccb6413e3bf" {
		t.Errorf("unexpected ID: %q", identity.ID)
	}
	if identity.Name != "tovarisch-subject-lab-canary-bounded-1784624046" {
		t.Errorf("unexpected Name: %q", identity.Name)
	}
}

func TestProjectContainerIdentity_RejectsEmpty(t *testing.T) {
	_, err := ProjectContainerIdentity([]byte{})
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestProjectContainerIdentity_RejectsInvalidJSON(t *testing.T) {
	_, err := ProjectContainerIdentity([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestProjectContainerIdentity_RejectsEmptyArray(t *testing.T) {
	_, err := ProjectContainerIdentity([]byte("[]"))
	if err == nil {
		t.Error("expected error for empty array")
	}
}

func TestParseContainerIdentity_Valid(t *testing.T) {
	data := []byte(`{
		"schema_version": "1.0.0",
		"id": "c6636cb0590ed65a30ae1682af98b50e5ae8ba5b784a9abaac1f8ccb6413e3bf",
		"name": "tovarisch-subject"
	}`)

	identity, err := ParseContainerIdentity(data)
	if err != nil {
		t.Fatalf("ParseContainerIdentity failed: %v", err)
	}

	if identity.SchemaVersion != "1.0.0" {
		t.Errorf("expected schema_version 1.0.0, got %q", identity.SchemaVersion)
	}
	if identity.ID != "c6636cb0590ed65a30ae1682af98b50e5ae8ba5b784a9abaac1f8ccb6413e3bf" {
		t.Errorf("unexpected ID: %q", identity.ID)
	}
}

func TestParseContainerIdentity_RejectsUnknownFields(t *testing.T) {
	data := []byte(`{
		"schema_version": "1.0.0",
		"id": "c6636cb0590ed65a30ae1682af98b50e5ae8ba5b784a9abaac1f8ccb6413e3bf",
		"name": "test",
		"extra_field": "should cause error"
	}`)

	_, err := ParseContainerIdentity(data)
	if err == nil {
		t.Error("expected error for unknown field")
	}
}

func TestParseContainerIdentity_RejectsMissingID(t *testing.T) {
	data := []byte(`{
		"schema_version": "1.0.0",
		"name": "test"
	}`)

	_, err := ParseContainerIdentity(data)
	if err == nil {
		t.Error("expected error for missing ID")
	}
}

func TestParseContainerIdentity_RejectsUnsupportedSchema(t *testing.T) {
	data := []byte(`{
		"schema_version": "99.99.99",
		"id": "c6636cb0590ed",
		"name": "test"
	}`)

	_, err := ParseContainerIdentity(data)
	if err == nil {
		t.Error("expected error for unsupported schema version")
	}
}

func TestWriteAndReadContainerIdentity(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "container-identity-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	containerID := "c6636cb0590ed65a30ae1682af98b50e5ae8ba5b784a9abaac1f8ccb6413e3bf"
	containerName := "tovarisch-subject"

	// Write
	err = WriteContainerIdentity(tmpDir, containerID, containerName)
	if err != nil {
		t.Fatalf("WriteContainerIdentity failed: %v", err)
	}

	// Read back
	identity, err := ReadContainerIdentity(tmpDir)
	if err != nil {
		t.Fatalf("ReadContainerIdentity failed: %v", err)
	}

	if identity.ID != containerID {
		t.Errorf("ID mismatch: expected %q, got %q", containerID, identity.ID)
	}
	if identity.Name != containerName {
		t.Errorf("Name mismatch: expected %q, got %q", containerName, identity.Name)
	}
}

func TestCanonicalChildArtifactInventory_HasNineEntries(t *testing.T) {
	if len(CanonicalChildArtifactInventory) != 9 {
		t.Errorf("expected 9 entries in canonical inventory, got %d", len(CanonicalChildArtifactInventory))
	}

	// Verify expected entries
	expected := map[string]bool{
		"container-identity.json":   true,
		"events.jsonl":              true,
		"final-canary-state.json":   true,
		"initial-canary-state.json": true,
		"manifest.json":             true,
		"network-identity.json":     true,
		"samples.csv":               true,
		"verdict.json":              true,
		"workload-result.json":      true,
	}

	for _, name := range CanonicalChildArtifactInventory {
		if !expected[name] {
			t.Errorf("unexpected entry in canonical inventory: %s", name)
		}
	}
}

func TestValidateArtifactInventory_Valid(t *testing.T) {
	checksumPaths := []string{
		"container-identity.json",
		"events.jsonl",
		"final-canary-state.json",
		"initial-canary-state.json",
		"manifest.json",
		"network-identity.json",
		"samples.csv",
		"verdict.json",
		"workload-result.json",
	}

	err := ValidateArtifactInventory(checksumPaths)
	if err != nil {
		t.Errorf("unexpected error for valid inventory: %v", err)
	}
}

func TestValidateArtifactInventory_RejectsMissing(t *testing.T) {
	checksumPaths := []string{
		"container-identity.json",
		"events.jsonl",
		"final-canary-state.json",
		// Missing several entries
		"manifest.json",
		"samples.csv",
		"verdict.json",
		"workload-result.json",
	}

	err := ValidateArtifactInventory(checksumPaths)
	if err == nil {
		t.Error("expected error for missing entries")
	}
}

func TestValidateArtifactInventory_RejectsExtra(t *testing.T) {
	checksumPaths := []string{
		"container-identity.json",
		"events.jsonl",
		"final-canary-state.json",
		"initial-canary-state.json",
		"manifest.json",
		"network-identity.json",
		"samples.csv",
		"verdict.json",
		"workload-result.json",
		"extra-file.json", // Extra entry
	}

	err := ValidateArtifactInventory(checksumPaths)
	if err == nil {
		t.Error("expected error for extra entries")
	}
}

func TestValidateArtifactInventory_RejectsDuplicate(t *testing.T) {
	checksumPaths := []string{
		"container-identity.json",
		"events.jsonl",
		"final-canary-state.json",
		"initial-canary-state.json",
		"manifest.json",
		"network-identity.json",
		"samples.csv",
		"verdict.json",
		"workload-result.json",
		"manifest.json", // Duplicate
	}

	err := ValidateArtifactInventory(checksumPaths)
	if err == nil {
		t.Error("expected error for duplicate entries")
	}
}

func TestValidateArtifactInventory_RejectsEmpty(t *testing.T) {
	err := ValidateArtifactInventory([]string{""})
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestValidateArtifactInventory_RejectsAbsolutePath(t *testing.T) {
	checksumPaths := []string{
		"container-identity.json",
		"events.jsonl",
		"final-canary-state.json",
		"initial-canary-state.json",
		"manifest.json",
		"network-identity.json",
		"samples.csv",
		"verdict.json",
		"/absolute/path/workload-result.json",
	}

	err := ValidateArtifactInventory(checksumPaths)
	if err == nil {
		t.Error("expected error for absolute path")
	}
}

func TestValidateArtifactInventory_RejectsParentTraversal(t *testing.T) {
	checksumPaths := []string{
		"container-identity.json",
		"events.jsonl",
		"final-canary-state.json",
		"initial-canary-state.json",
		"manifest.json",
		"network-identity.json",
		"samples.csv",
		"verdict.json",
		"../workload-result.json",
	}

	err := ValidateArtifactInventory(checksumPaths)
	if err == nil {
		t.Error("expected error for parent traversal")
	}
}
