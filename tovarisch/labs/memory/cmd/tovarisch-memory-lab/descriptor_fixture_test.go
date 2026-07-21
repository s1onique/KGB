// descriptor_fixture_test.go — Hermetic fixture copy + positive
// baseline for the canary-descriptor scenario.
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-DESCRIPTOR-QUALIFICATION01 §13:
//
//   - TestDescriptorPositiveBaseline_CopiedFixtureVerifies
//   - TestDescriptorPositiveBaseline_InventoryVerifies
//   - TestDescriptorPositiveBaseline_ExactStateDelta
//   - TestDescriptorPositiveBaseline_ResourceClassification
//   - TestDescriptorPositiveBaseline_MemoryStable
//
// All positive tests copy the committed fixture into a per-test
// temp dir, rebind the live-inode fields to the freshly built
// verifier, and assert the canonical descriptor verdict.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence"
)

// descriptorFixtureDir is the committed test data fixture used by
// every descriptor negative test. Missing this directory is a
// hard test failure (not a skip) per ACT §5.2.
const descriptorFixtureDir = "testdata/descriptor-valid"

// descriptorFixtureRunID is the run_id recorded inside the
// committed descriptor fixture's manifest.json. All tests copy the
// fixture into a temp dir with this exact name and pass this exact
// run_id to the verifier.
const descriptorFixtureRunID = "lab-canary-descriptor-placeholder"

// requireDescriptorFixture is the descriptor-specific entry point
// for the scenario-agnostic fixture requirement. A missing fixture
// is a hard test failure (not a skip) per ACT §5.2.
func requireDescriptorFixture(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(descriptorFixtureDir)
	if err != nil {
		t.Fatalf("absolute descriptor fixture path: %v", err)
	}
	manifest := filepath.Join(abs, "manifest.json")
	if _, err := os.Stat(manifest); err != nil {
		t.Fatalf("committed descriptor fixture missing at %s: %v "+
			"(ACT requires a committed fixture; no .factory fallback)",
			abs, err)
	}
	return abs
}

// rebindDescriptorFixture is a thin wrapper that uses the
// descriptor-specific run_id constant.
func rebindDescriptorFixture(t *testing.T, boundDir string) string {
	return rebindFixture(t, boundDir)
}

// readDescriptorManifest parses the manifest.json of a bound
// descriptor fixture copy.
func readDescriptorManifest(t *testing.T, boundDir string) *evidence.Manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(boundDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read descriptor manifest: %v", err)
	}
	var m evidence.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse descriptor manifest: %v", err)
	}
	return &m
}

// runDescriptorVerifier invokes the freshly built verifier against
// the descriptor fixture copy.
func runDescriptorVerifier(t *testing.T, artifactsDir string) (string, error) {
	return runVerifierForRunID(t, artifactsDir, descriptorFixtureRunID)
}

// TestDescriptorPositiveBaseline_CopiedFixtureVerifies copies the
// committed descriptor fixture into t.TempDir(), rebinds the
// live-inode fields to the freshly built verifier, and requires
// exit code 0 with the canonical descriptor verdict.
func TestDescriptorPositiveBaseline_CopiedFixtureVerifies(t *testing.T) {
	src := requireDescriptorFixture(t)
	scenarioFixtureFilesExist(t, src)

	dst := t.TempDir()
	boundDir := copyFixture(t, dst, descriptorFixtureDir, descriptorFixtureRunID)
	rebindDescriptorFixture(t, boundDir)

	out, err := runDescriptorVerifier(t, dst)
	if err != nil {
		t.Fatalf("positive baseline: verifier rejected copied fixture:\n%s", out)
	}
	// Sanity: the verifier output reports resource_growth classification.
	if !strings.Contains(out, "Overall: resource_growth") {
		t.Errorf("positive baseline: expected Overall: resource_growth, got:\n%s", out)
	}
	if !strings.Contains(out, "ScenarioValid: true") {
		t.Errorf("positive baseline: expected ScenarioValid: true, got:\n%s", out)
	}
	if !strings.Contains(out, "CanariesValid: true") {
		t.Errorf("positive baseline: expected CanariesValid: true, got:\n%s", out)
	}
}

// TestDescriptorPositiveBaseline_InventoryVerifies asserts that
// the committed descriptor fixture's checksums.txt matches the
// actual SHA-256 of each canonical artifact. The
// controller_executable_sha256 in the committed fixture is a
// placeholder; the live-runtime binding is covered by
// TestDescriptorPositiveBaseline_CopiedFixtureVerifies.
func TestDescriptorPositiveBaseline_InventoryVerifies(t *testing.T) {
	src := requireDescriptorFixture(t)
	scenarioFixtureFilesExist(t, src)

	checksumPath := filepath.Join(src, "checksums.txt")
	raw, err := os.ReadFile(checksumPath)
	if err != nil {
		t.Fatalf("read checksums.txt: %v", err)
	}
	lines := strings.Split(string(raw), "\n")
	checked := 0
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			t.Fatalf("malformed checksums.txt line: %q", line)
		}
		stored := strings.TrimSpace(parts[0])
		path := strings.TrimSpace(parts[1])
		if path == "checksums.txt" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		sum := sha256.Sum256(data)
		got := hex.EncodeToString(sum[:])
		if got != stored {
			t.Errorf("checksum mismatch for %s: stored=%s got=%s", path, stored, got)
		}
		checked++
	}
	if checked < 9 {
		t.Errorf("expected at least 9 artifact checksums (excluding checksums.txt), found %d", checked)
	}
}

// TestDescriptorPositiveBaseline_ExactStateDelta reads the fixture's
// initial and final canary states and asserts the descriptor
// invariant: operation_count_delta == 100 and fd_count_delta == 200.
func TestDescriptorPositiveBaseline_ExactStateDelta(t *testing.T) {
	src := requireDescriptorFixture(t)

	readState := func(name string) *CanaryState {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var s CanaryState
		if err := json.Unmarshal(data, &s); err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		return &s
	}

	initial := readState("initial-canary-state.json")
	final := readState("final-canary-state.json")

	opDelta := final.OperationCount - initial.OperationCount
	if opDelta != 100 {
		t.Errorf("operation_count_delta=%d, want 100", opDelta)
	}
	fdDelta := final.FDCount - initial.FDCount
	if fdDelta != 200 {
		t.Errorf("fd_count_delta=%d, want 200", fdDelta)
	}
	if initial.Mode != "descriptor" {
		t.Errorf("initial mode=%q, want descriptor", initial.Mode)
	}
	if final.Mode != "descriptor" {
		t.Errorf("final mode=%q, want descriptor", final.Mode)
	}
	if final.RetainedBlocks != 0 || final.RetainedBytes != 0 {
		t.Errorf("descriptor must not retain memory: blocks=%d bytes=%d",
			final.RetainedBlocks, final.RetainedBytes)
	}
}

// TestDescriptorPositiveBaseline_ResourceClassification asserts the
// stored verdict's resource_classification is exactly
// `resource_growth` and the overall_classification equals
// `resource_growth`.
func TestDescriptorPositiveBaseline_ResourceClassification(t *testing.T) {
	src := requireDescriptorFixture(t)

	data, err := os.ReadFile(filepath.Join(src, "verdict.json"))
	if err != nil {
		t.Fatalf("read verdict.json: %v", err)
	}
	var verdict evidence.Verdict
	if err := json.Unmarshal(data, &verdict); err != nil {
		t.Fatalf("parse verdict.json: %v", err)
	}

	if verdict.OverallClassification != "resource_growth" {
		t.Errorf("overall_classification=%q, want resource_growth", verdict.OverallClassification)
	}
	if verdict.ResourceClassification != "resource_growth" {
		t.Errorf("resource_classification=%q, want resource_growth", verdict.ResourceClassification)
	}
	if verdict.Scenario != "canary-descriptor" {
		t.Errorf("verdict scenario=%q, want canary-descriptor", verdict.Scenario)
	}
}

// TestDescriptorPositiveBaseline_MemoryStable asserts the stored
// verdict's memory_classification is `stable` and that no signal
// with IsPrimary=true carries the `growing` classification. This
// proves the descriptor scenario is not falsely classified as
// memory-growth.
func TestDescriptorPositiveBaseline_MemoryStable(t *testing.T) {
	src := requireDescriptorFixture(t)

	data, err := os.ReadFile(filepath.Join(src, "verdict.json"))
	if err != nil {
		t.Fatalf("read verdict.json: %v", err)
	}
	var verdict evidence.Verdict
	if err := json.Unmarshal(data, &verdict); err != nil {
		t.Fatalf("parse verdict.json: %v", err)
	}

	if verdict.MemoryClassification != "stable" {
		t.Errorf("memory_classification=%q, want stable", verdict.MemoryClassification)
	}
	for _, sig := range verdict.SignalSummaries {
		if sig.IsPrimary && sig.Classification == "growing" {
			t.Errorf("primary signal %q is growing in descriptor scenario (false memory-growth verdict)", sig.Name)
		}
	}
	// Sanity: the descriptor leak must be reported, not the absence
	// of a memory leak.
	if verdict.OverallClassification == "stable" {
		t.Errorf("overall=stable, want resource_growth (descriptor leak must show)")
	}
}

// formatFDDeltaDiag is a small helper used by descriptor state
// negative tests to assert the verifier's exact error path. It
// matches the production verifier's diagnostic format.
func formatFDDeltaDiag(actual, expected int) string {
	return fmt.Sprintf("descriptor: fd_delta=%d != expected=%d", actual, expected)
}
