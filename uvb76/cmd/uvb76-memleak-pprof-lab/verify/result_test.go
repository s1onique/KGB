// Package verify provides tests for the artifact verifier.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVerify_ResultJSON_OkFalseFails(t *testing.T) {
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
	os.WriteFile(filepath.Join(tmpDir, "manifest.json"), data, 0644)

	// Create verdict.json
	verdict := Verdict{Summary: "test", RSSGrowthBytes: 0, Reasons: []string{}}
	os.WriteFile(filepath.Join(tmpDir, "verdict.json"), mustMarshal(verdict), 0644)

	// Create result.json with ok=false
	result := map[string]interface{}{
		"ok":                   false,
		"uvb76_started":        true,
		"pprof_reachable":      true,
		"tovarisch_reachable":  true,
		"collector_succeeded":  true,
		"pprof_diff_succeeded": true,
		"manifest_valid":       true,
		"verdict_valid":        true,
	}
	os.WriteFile(filepath.Join(tmpDir, "result.json"), mustMarshal(result), 0644)

	errors := verify(tmpDir)

	foundOkError := false
	for _, err := range errors {
		if err == "result.json: ok field is false" {
			foundOkError = true
		}
	}
	if !foundOkError {
		t.Error("Expected error for result.json with ok=false")
	}
}

func TestVerify_ResultJSON_MissingCriticalFieldFails(t *testing.T) {
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
	os.WriteFile(filepath.Join(tmpDir, "manifest.json"), data, 0644)

	// Create verdict.json
	verdict := Verdict{Summary: "test", RSSGrowthBytes: 0, Reasons: []string{}}
	os.WriteFile(filepath.Join(tmpDir, "verdict.json"), mustMarshal(verdict), 0644)

	// Create result.json with tovarisch_reachable=false
	result := map[string]interface{}{
		"ok":                   true,
		"uvb76_started":        true,
		"pprof_reachable":      true,
		"tovarisch_reachable":  false, // This should fail
		"collector_succeeded":  true,
		"pprof_diff_succeeded": true,
		"manifest_valid":       true,
		"verdict_valid":        true,
	}
	os.WriteFile(filepath.Join(tmpDir, "result.json"), mustMarshal(result), 0644)

	errors := verify(tmpDir)

	foundTovarischError := false
	for _, err := range errors {
		if err == "result.json: tovarisch_reachable is false" {
			foundTovarischError = true
		}
	}
	if !foundTovarischError {
		t.Error("Expected error for result.json with tovarisch_reachable=false")
	}
}

func TestVerify_MissingGoroutineDumpFails(t *testing.T) {
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
	os.WriteFile(filepath.Join(tmpDir, "manifest.json"), data, 0644)

	// Create verdict.json
	verdict := Verdict{Summary: "test", RSSGrowthBytes: 0, Reasons: []string{}}
	os.WriteFile(filepath.Join(tmpDir, "verdict.json"), mustMarshal(verdict), 0644)

	// Create result.json
	result := map[string]interface{}{
		"ok":                   true,
		"uvb76_started":        true,
		"pprof_reachable":      true,
		"tovarisch_reachable":  true,
		"collector_succeeded":  true,
		"pprof_diff_succeeded": true,
		"manifest_valid":       true,
		"verdict_valid":        true,
	}
	os.WriteFile(filepath.Join(tmpDir, "result.json"), mustMarshal(result), 0644)

	// Missing goroutine dumps should cause errors
	errors := verify(tmpDir)

	foundBaselineError := false
	foundFinalError := false
	for _, err := range errors {
		if err == "required file missing: goroutine-t000.txt" {
			foundBaselineError = true
		}
		if err == "required file missing: goroutine-t600.txt" {
			foundFinalError = true
		}
	}
	if !foundBaselineError {
		t.Error("Expected error for missing goroutine-t000.txt")
	}
	if !foundFinalError {
		t.Error("Expected error for missing goroutine-t600.txt")
	}
}

func TestVerify_ResultJSON_PProfNotReachableFails(t *testing.T) {
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
	os.WriteFile(filepath.Join(tmpDir, "manifest.json"), data, 0644)

	// Create verdict.json
	verdict := Verdict{Summary: "test", RSSGrowthBytes: 0, Reasons: []string{}}
	os.WriteFile(filepath.Join(tmpDir, "verdict.json"), mustMarshal(verdict), 0644)

	// Create result.json with pprof_reachable=false
	result := map[string]interface{}{
		"ok":                   true,
		"uvb76_started":        true,
		"pprof_reachable":      false, // This should fail
		"tovarisch_reachable":  true,
		"collector_succeeded":  true,
		"pprof_diff_succeeded": true,
		"manifest_valid":       true,
		"verdict_valid":        true,
	}
	os.WriteFile(filepath.Join(tmpDir, "result.json"), mustMarshal(result), 0644)

	errors := verify(tmpDir)

	foundPProfError := false
	for _, err := range errors {
		if err == "result.json: pprof_reachable is false" {
			foundPProfError = true
		}
	}
	if !foundPProfError {
		t.Error("Expected error for result.json with pprof_reachable=false")
	}
}

func TestVerify_ResultJSON_PProfDiffFailedFails(t *testing.T) {
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
	os.WriteFile(filepath.Join(tmpDir, "manifest.json"), data, 0644)

	// Create verdict.json
	verdict := Verdict{Summary: "test", RSSGrowthBytes: 0, Reasons: []string{}}
	os.WriteFile(filepath.Join(tmpDir, "verdict.json"), mustMarshal(verdict), 0644)

	// Create result.json with pprof_diff_succeeded=false
	result := map[string]interface{}{
		"ok":                   true,
		"uvb76_started":        true,
		"pprof_reachable":      true,
		"tovarisch_reachable":  true,
		"collector_succeeded":  true,
		"pprof_diff_succeeded": false, // This should fail
		"manifest_valid":       true,
		"verdict_valid":        true,
	}
	os.WriteFile(filepath.Join(tmpDir, "result.json"), mustMarshal(result), 0644)

	errors := verify(tmpDir)

	foundDiffError := false
	for _, err := range errors {
		if err == "result.json: pprof_diff_succeeded is false" {
			foundDiffError = true
		}
	}
	if !foundDiffError {
		t.Error("Expected error for result.json with pprof_diff_succeeded=false")
	}
}

func mustMarshal(v interface{}) []byte {
	data, _ := json.Marshal(v)
	return data
}
