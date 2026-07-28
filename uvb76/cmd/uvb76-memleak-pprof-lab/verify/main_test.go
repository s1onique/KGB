// Package verify provides tests for the artifact verifier.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyCSVHeader(t *testing.T) {
	// Create temp dir
	tmpDir, err := os.MkdirTemp("", "verifier-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test valid header
	validCSV := filepath.Join(tmpDir, "valid.csv")
	os.WriteFile(validCSV, []byte("elapsed_seconds,pid,rss_bytes,vsz_bytes,threads,fd_count\n"), 0644)

	if err := verifyCSVHeader(validCSV, "elapsed_seconds,pid,rss_bytes,vsz_bytes,threads,fd_count"); err != nil {
		t.Errorf("Expected valid header to pass: %v", err)
	}

	// Test invalid header
	invalidCSV := filepath.Join(tmpDir, "invalid.csv")
	os.WriteFile(invalidCSV, []byte("wrong,header\n"), 0644)

	if err := verifyCSVHeader(invalidCSV, "elapsed_seconds,pid,rss_bytes,vsz_bytes,threads,fd_count"); err == nil {
		t.Error("Expected invalid header to fail")
	}

	// Test empty file
	emptyCSV := filepath.Join(tmpDir, "empty.csv")
	os.WriteFile(emptyCSV, []byte(""), 0644)

	if err := verifyCSVHeader(emptyCSV, "elapsed_seconds,pid,rss_bytes,vsz_bytes,threads,fd_count"); err == nil {
		t.Error("Expected empty file to fail")
	}
}

func TestAllowedClassifications(t *testing.T) {
	valid := []string{
		"suspected_go_heap_retention",
		"rss_growth_heap_stable",
		"goroutine_growth",
		"no_material_growth",
		"inconclusive",
	}

	for _, c := range valid {
		if !allowedClassifications[c] {
			t.Errorf("Classification %q should be allowed", c)
		}
	}

	invalid := []string{
		"invalid_classification",
		"memory_leak",
		"",
	}

	for _, c := range invalid {
		if allowedClassifications[c] {
			t.Errorf("Classification %q should not be allowed", c)
		}
	}
}

func TestVerdictJSON(t *testing.T) {
	v := Verdict{
		Summary:        "test summary",
		RSSGrowthBytes: 1024,
		Reasons:        []string{"reason1"},
	}

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Failed to marshal verdict: %v", err)
	}

	var decoded Verdict
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal verdict: %v", err)
	}

	if decoded.Summary != v.Summary {
		t.Errorf("Summary mismatch: got %q, want %q", decoded.Summary, v.Summary)
	}
	if decoded.RSSGrowthBytes != v.RSSGrowthBytes {
		t.Errorf("RSSGrowthBytes mismatch: got %d, want %d", decoded.RSSGrowthBytes, v.RSSGrowthBytes)
	}
}

func TestVerify_MissingFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "verifier-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	errors := verify(tmpDir)

	// Should have errors for missing files
	if len(errors) == 0 {
		t.Error("Expected errors for missing files")
	}

	foundManifestError := false
	for _, err := range errors {
		if err == "required file missing: manifest.json" {
			foundManifestError = true
		}
	}
	if !foundManifestError {
		t.Error("Expected error for missing manifest.json")
	}
}

func TestVerify_EmptyFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "verifier-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create empty manifest.json
	manifestPath := filepath.Join(tmpDir, "manifest.json")
	os.WriteFile(manifestPath, []byte(""), 0644)

	errors := verify(tmpDir)

	foundEmptyError := false
	for _, err := range errors {
		if err == "required file is empty: manifest.json" {
			foundEmptyError = true
		}
	}
	if !foundEmptyError {
		t.Error("Expected error for empty manifest.json")
	}
}

func TestVerify_InvalidSchemaVersion(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "verifier-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create manifest with wrong schema version
	manifest := map[string]interface{}{
		"schema_version": 2,
	}
	data, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(tmpDir, "manifest.json")
	os.WriteFile(manifestPath, data, 0644)

	errors := verify(tmpDir)

	foundSchemaError := false
	for _, err := range errors {
		if err == "manifest.json: expected schema_version=1, got 2" {
			foundSchemaError = true
		}
	}
	if !foundSchemaError {
		t.Error("Expected error for wrong schema version")
	}
}

func TestVerify_WrongClassification(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "verifier-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create manifest with invalid classification
	manifest := map[string]interface{}{
		"schema_version": 1,
		"classification": "invalid_class",
	}
	data, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(tmpDir, "manifest.json")
	os.WriteFile(manifestPath, data, 0644)

	errors := verify(tmpDir)

	foundClassError := false
	for _, err := range errors {
		if err == "unknown classification: invalid_class" {
			foundClassError = true
		}
	}
	if !foundClassError {
		t.Error("Expected error for invalid classification")
	}
}

func TestVerify_EmptyDiffReportFails(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "verifier-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create minimal valid manifest
	manifest := map[string]interface{}{
		"schema_version":   1,
		"classification":   "no_material_growth",
		"duration_seconds": 600,
	}
	data, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(tmpDir, "manifest.json")
	os.WriteFile(manifestPath, data, 0644)

	// Create verdict.json
	verdict := Verdict{
		Summary:        "test",
		RSSGrowthBytes: 0,
		Reasons:        []string{},
	}
	verdictData, _ := json.Marshal(verdict)
	os.WriteFile(filepath.Join(tmpDir, "verdict.json"), verdictData, 0644)

	// Create empty diff report
	emptyReport := filepath.Join(tmpDir, "heap-diff-inuse-space.txt")
	os.WriteFile(emptyReport, []byte(""), 0644)

	errors := verify(tmpDir)

	foundEmptyError := false
	for _, err := range errors {
		if err == "pprof diff report is empty: heap-diff-inuse-space.txt" {
			foundEmptyError = true
		}
	}
	if !foundEmptyError {
		t.Error("Expected error for empty diff report")
	}
}

func TestVerify_MissingTovarischLogFails(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "verifier-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create minimal valid manifest
	manifest := map[string]interface{}{
		"schema_version":   1,
		"classification":   "no_material_growth",
		"duration_seconds": 600,
	}
	data, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(tmpDir, "manifest.json")
	os.WriteFile(manifestPath, data, 0644)

	// Create verdict.json
	verdict := Verdict{
		Summary:        "test",
		RSSGrowthBytes: 0,
		Reasons:        []string{},
	}
	verdictData, _ := json.Marshal(verdict)
	os.WriteFile(filepath.Join(tmpDir, "verdict.json"), verdictData, 0644)

	errors := verify(tmpDir)

	foundMissingError := false
	for _, err := range errors {
		if err == "tovarisch.log missing (required for default fake tovarisch mode)" {
			foundMissingError = true
		}
	}
	if !foundMissingError {
		t.Error("Expected error for missing tovarisch.log")
	}
}

func TestVerify_EmptyTovarischLogFails(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "verifier-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create minimal valid manifest
	manifest := map[string]interface{}{
		"schema_version":   1,
		"classification":   "no_material_growth",
		"duration_seconds": 600,
	}
	data, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(tmpDir, "manifest.json")
	os.WriteFile(manifestPath, data, 0644)

	// Create verdict.json
	verdict := Verdict{
		Summary:        "test",
		RSSGrowthBytes: 0,
		Reasons:        []string{},
	}
	verdictData, _ := json.Marshal(verdict)
	os.WriteFile(filepath.Join(tmpDir, "verdict.json"), verdictData, 0644)

	// Create empty tovarisch.log
	os.WriteFile(filepath.Join(tmpDir, "tovarisch.log"), []byte(""), 0644)

	errors := verify(tmpDir)

	foundEmptyError := false
	for _, err := range errors {
		if err == "tovarisch.log is empty" {
			foundEmptyError = true
		}
	}
	if !foundEmptyError {
		t.Error("Expected error for empty tovarisch.log")
	}
}
