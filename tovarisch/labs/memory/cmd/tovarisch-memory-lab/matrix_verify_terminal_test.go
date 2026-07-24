// matrix_verify_terminal_test.go — Matrix Verify Command Terminal Tests
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03-P0-CLI-FIXTURE-CONVERGENCE01
//
// Tests the actual verify-matrix command execution against artifact-backed fixtures.
// P0-8: Uses test-only fixture from matrix_fixture_test.go

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence"
)

// =============================================================================
// PASS LINE MATCHING
// =============================================================================

const verifyMatrixPassLine = "All Checks: PASS"

// countTerminalPassLines counts exact PASS lines in output.
func countTerminalPassLines(output string) int {
	var count int
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == verifyMatrixPassLine {
			count++
		}
	}
	return count
}

// assertNoTerminalPass asserts no PASS line appears in output.
func assertNoTerminalPass(t *testing.T, stdout, stderr string) {
	t.Helper()

	if countTerminalPassLines(stdout) > 0 {
		t.Error("found PASS line in stdout")
	}
	if countTerminalPassLines(stderr) > 0 {
		t.Error("found PASS line in stderr")
	}
}

// assertTerminalPass asserts exactly one PASS line appears in output.
func assertTerminalPass(t *testing.T, stdout, stderr string) {
	t.Helper()

	passCount := countTerminalPassLines(stdout)
	if passCount == 0 {
		t.Error("no PASS line found in output")
	}
	if passCount > 1 {
		t.Errorf("expected exactly one PASS line, found %d", passCount)
	}

	if countTerminalPassLines(stderr) > 0 {
		t.Error("found PASS line in stderr (should only be in stdout)")
	}
}

// =============================================================================
// CANONICAL FIXTURE TESTS
// =============================================================================

// TestVerifyMatrixBundle_AcceptsCompleteCanonicalFixture proves VerifyMatrixBundle
// accepts a complete valid fixture.
func TestVerifyMatrixBundle_AcceptsCompleteCanonicalFixture(t *testing.T) {
	// P0-8: Use TempDir fixture - automatic cleanup
	fixture := writeValidMatrixBundleFixture(t)

	// Verify with real verifier
	result, err := VerifyMatrixBundle(fixture.rootDir, MatrixVerificationDeps{
		VerifyChildRun: verifyChildRunBundle,
	})
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}

	// Assert verification results
	if !result.AllChildrenVerified {
		t.Error("expected all children verified")
	}
	if !result.CleanupValid {
		t.Error("expected cleanup valid")
	}
	if !result.ReconstructedVerdict.MatrixValid {
		t.Error("expected matrix valid")
	}

	// Assert 3 scenarios verified
	if len(result.VerifiedRuns) != 3 {
		t.Errorf("expected 3 verified runs, got %d", len(result.VerifiedRuns))
	}

	// Assert 16 checks total
	if result.ReconstructedVerdict.ChecksTotal != 16 {
		t.Errorf("expected 16 checks total, got %d", result.ReconstructedVerdict.ChecksTotal)
	}

	// Assert all checks passed
	if result.ReconstructedVerdict.ChecksPassed != 16 {
		t.Errorf("expected 16 checks passed, got %d", result.ReconstructedVerdict.ChecksPassed)
	}
	if result.ReconstructedVerdict.ChecksFailed != 0 {
		t.Errorf("expected 0 checks failed, got %d", result.ReconstructedVerdict.ChecksFailed)
	}

	// Assert stored equals reconstructed
	diffs := CompareVerdicts(result.StoredVerdict, result.ReconstructedVerdict)
	if len(diffs) != 0 {
		t.Errorf("stored != reconstructed: %d differences", len(diffs))
	}

	// Assert cleanup identities match child identities
	for i, rec := range result.Cleanup.Runs {
		run := result.VerifiedRuns[i]
		if rec.Container.ID != run.ContainerID {
			t.Errorf("cleanup[%d] container ID mismatch", i)
		}
		if rec.Network.ID != run.NetworkID {
			t.Errorf("cleanup[%d] network ID mismatch", i)
		}
		if rec.Process.PID != run.SubjectPID {
			t.Errorf("cleanup[%d] PID mismatch", i)
		}
		if rec.Process.StartTime != run.SubjectStartTime {
			t.Errorf("cleanup[%d] start time mismatch", i)
		}
	}

	// Assert cleanup statuses are successful (gone)
	for i, rec := range result.Cleanup.Runs {
		if rec.Container.Status != "gone" {
			t.Errorf("cleanup[%d] container status: expected gone, got %s", i, rec.Container.Status)
		}
		if rec.Network.Status != "gone" {
			t.Errorf("cleanup[%d] network status: expected gone, got %s", i, rec.Network.Status)
		}
		if rec.Process.Status != "gone" && rec.Process.Status != "pid_reused" {
			t.Errorf("cleanup[%d] process status: expected gone/pid_reused, got %s", i, rec.Process.Status)
		}
	}

	t.Logf("fixture: %s, matrix_id: %s", fixture.rootDir, fixture.matrixID)
}

// =============================================================================
// SINGLE-CAUSE SEMANTIC MUTATION TESTS
// =============================================================================

// mutationTest describes a semantic mutation test case.
type mutationTest struct {
	name         string
	mutate       func(*matrixFixture, *testing.T)
	wantError    bool
	refreshChild bool // P0-7: True if mutation affects child artifacts requiring checksum refresh
}

// TestVerifyMatrixBundle_SemanticMutations proves semantic mutations fail
// with valid checksums regenerated.
func TestVerifyMatrixBundle_SemanticMutations(t *testing.T) {
	tests := []mutationTest{
		// Cleanup identity mismatches
		{"cleanup container ID mismatch", mutateCleanupContainerID, true, false},
		{"cleanup network ID mismatch", mutateCleanupNetworkID, true, false},
		{"cleanup PID mismatch", mutateCleanupPID, true, false},
		{"cleanup start time mismatch", mutateCleanupStartTime, true, false},
		{"cleanup RunID mismatch", mutateCleanupRunID, true, false},
		{"cleanup scenario mismatch", mutateCleanupScenario, true, false},
		{"cleanup index mismatch", mutateCleanupIndex, true, false},

		// Temporal binding failures
		{"cleanup timestamp before finished_at", mutateCleanupObservedAtBefore, true, false},

		// Cleanup status failures
		{"container status exists", mutateContainerStatusExists, true, false},
		{"container status unavailable", mutateContainerStatusUnavailable, true, false},
		{"network status exists", mutateNetworkStatusExists, true, false},
		{"network status unavailable", mutateNetworkStatusUnavailable, true, false},
		{"process status still_alive", mutateProcessStatusStillAlive, true, false},
		{"process status unavailable", mutateProcessStatusUnavailable, true, false},

		// P0-7: Child identity mutations require child checksum regeneration
		{"child container identity mutation", mutateChildContainerID, true, true},
		{"child network identity mutation", mutateChildNetworkID, true, true},
		{"child RunID mutation", mutateChildRunID, true, true},
		{"child scenario mutation", mutateChildScenario, true, true},

		// Verdict mutations
		{"stored classification mutation", mutateStoredClassification, true, false},
		{"stored matrix validity mutation", mutateStoredMatrixValid, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// P0-8: Use TempDir fixture - automatic cleanup
			fixture := writeValidMatrixBundleFixture(t)

			// Apply mutation
			tt.mutate(fixture, t)

			// P0-7: Regenerate checksums based on what was mutated
			if tt.refreshChild {
				// Child mutations require regenerating both child AND matrix checksums
				mustRegenerateAllChecksums(t, fixture)
			} else {
				// Matrix-level mutations only need matrix checksums
				// Re-write matrix artifacts and regenerate checksums
				if err := writeFixtureJSON(filepath.Join(fixture.rootDir, "matrix-cleanup.json"), fixture.cleanup); err != nil {
					t.Fatalf("write cleanup: %v", err)
				}
				if err := writeFixtureJSON(filepath.Join(fixture.rootDir, "matrix-verdict.json"), fixture.verdict); err != nil {
					t.Fatalf("write verdict: %v", err)
				}
				if err := regenerateMatrixChecksums(fixture.rootDir); err != nil {
					t.Fatalf("regenerate matrix checksums: %v", err)
				}
			}

			// Verify
			_, err := VerifyMatrixBundle(fixture.rootDir, MatrixVerificationDeps{
				VerifyChildRun: verifyChildRunBundle,
			})

			if tt.wantError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// =============================================================================
// MUTATION HELPERS (using matrixFixture)
// =============================================================================

func mutateCleanupContainerID(f *matrixFixture, t *testing.T) {
	f.cleanup.Runs[0].Container.ID = "mismatched-container-id"
}

func mutateCleanupNetworkID(f *matrixFixture, t *testing.T) {
	f.cleanup.Runs[0].Network.ID = "mismatched-network-id"
}

func mutateCleanupPID(f *matrixFixture, t *testing.T) {
	f.cleanup.Runs[0].Process.PID = 99999
}

func mutateCleanupStartTime(f *matrixFixture, t *testing.T) {
	f.cleanup.Runs[0].Process.StartTime = 999999
}

func mutateCleanupRunID(f *matrixFixture, t *testing.T) {
	f.cleanup.Runs[0].RunID = "mismatched-run-id"
}

func mutateCleanupScenario(f *matrixFixture, t *testing.T) {
	f.cleanup.Runs[0].Scenario = "canary-bounded"
}

func mutateCleanupIndex(f *matrixFixture, t *testing.T) {
	f.cleanup.Runs[0].Index = 99
}

func mutateCleanupObservedAtBefore(f *matrixFixture, t *testing.T) {
	f.cleanup.ObservedAt = f.manifest.FinishedAt.Add(-1 * time.Hour)
}

func mutateContainerStatusExists(f *matrixFixture, t *testing.T) {
	f.cleanup.Runs[0].Container.Status = "exists"
}

func mutateContainerStatusUnavailable(f *matrixFixture, t *testing.T) {
	f.cleanup.Runs[0].Container.Status = "unavailable"
}

func mutateNetworkStatusExists(f *matrixFixture, t *testing.T) {
	f.cleanup.Runs[0].Network.Status = "exists"
}

func mutateNetworkStatusUnavailable(f *matrixFixture, t *testing.T) {
	f.cleanup.Runs[0].Network.Status = "unavailable"
}

func mutateProcessStatusStillAlive(f *matrixFixture, t *testing.T) {
	f.cleanup.Runs[0].Process.Status = "still_alive"
}

func mutateProcessStatusUnavailable(f *matrixFixture, t *testing.T) {
	f.cleanup.Runs[0].Process.Status = "unavailable"
}

func mutateChildContainerID(f *matrixFixture, t *testing.T) {
	// Mutate container-inspect.json in first child
	runDir := f.runDirs[0]
	path := filepath.Join(runDir, "container-inspect.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read container inspect: %v", err)
	}
	var inspect map[string]string
	if err := json.Unmarshal(data, &inspect); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	inspect["Id"] = "mismatched-child-container"
	newData, err := json.MarshalIndent(inspect, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, newData, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func mutateChildNetworkID(f *matrixFixture, t *testing.T) {
	// Mutate network-identity.json in first child
	runDir := f.runDirs[0]
	path := filepath.Join(runDir, "network-identity.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read network identity: %v", err)
	}
	var netID NetworkIdentity
	if err := json.Unmarshal(data, &netID); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	netID.ID = "mismatched-child-network"
	newData, err := json.MarshalIndent(netID, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, newData, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func mutateChildRunID(f *matrixFixture, t *testing.T) {
	// Mutate manifest.json in first child
	runDir := f.runDirs[0]
	path := filepath.Join(runDir, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest evidence.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	manifest.RunID = "mismatched-child-runid"
	newData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, newData, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func mutateChildScenario(f *matrixFixture, t *testing.T) {
	// Mutate manifest.json in first child
	runDir := f.runDirs[0]
	path := filepath.Join(runDir, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest evidence.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	manifest.Scenario = "canary-bounded"
	newData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, newData, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func mutateStoredClassification(f *matrixFixture, t *testing.T) {
	// Mutate stored verdict
	if f.verdict.ScenarioResults == nil {
		t.Fatal("verdict has nil ScenarioResults")
	}
	f.verdict.ScenarioResults["canary-growing"].Overall = "wrong"
}

func mutateStoredMatrixValid(f *matrixFixture, t *testing.T) {
	// Set stored verdict invalid while reconstruction is valid
	f.verdict.MatrixValid = false
}

// =============================================================================
// EQUAL-INVALID TEST
// =============================================================================

// TestVerifyMatrixBundle_RejectsEqualInvalid proves equal-invalid verdicts fail.
// P0-6: An "equal-invalid" verdict has MatrixValid=false in both stored and reconstructed,
// but verification fails because equal-invalid is forbidden by policy.
// P0-6 FIX: Uses mustRegenerateAllChecksums to rewrite manifest to disk before verification.
func TestVerifyMatrixBundle_RejectsEqualInvalid(t *testing.T) {
	// P0-8: Use TempDir fixture - automatic cleanup
	fixture := writeValidMatrixBundleFixture(t)

	// Mutate child verdict to make reconstruction invalid
	descriptorIndex := 2 // canary-descriptor
	mutation := childArtifactMutation{
		runIndex: descriptorIndex,
		filename: "verdict.json",
		mutate: func(data []byte) ([]byte, error) {
			var verdict evidence.Verdict
			if err := json.Unmarshal(data, &verdict); err != nil {
				return nil, err
			}
			// Change to incompatible classification
			verdict.OverallClassification = analysis.ClassificationGrowing
			return json.MarshalIndent(verdict, "", "  ")
		},
	}
	mustApplyChildSemanticMutation(t, fixture, mutation)

	// P0-6 FIX: Rewrite manifest to disk with updated child checksums
	// mustApplyChildSemanticMutation updates fixture.manifest in memory,
	// but VerifyMatrixBundle reads from disk, so we must persist it.
	mustRegenerateAllChecksums(t, fixture)

	// Reconstruct with mutated child (from disk)
	verifiedRuns := buildVerifiedRunsFromFixture(fixture)
	reconstructedVerdict, err := ReconstructMatrixVerdict(fixture.manifest, verifiedRuns, fixture.cleanup)
	if err != nil {
		t.Fatalf("reconstruct: %v", err)
	}

	// P0-6 FIX: Assert reconstructed is invalid
	if reconstructedVerdict.MatrixValid {
		t.Fatal("reconstructed verdict should be invalid due to classification mismatch")
	}

	// Force stored verdict to match reconstructed (both invalid)
	fixture.verdict = reconstructedVerdict

	// Rewrite matrix verdict with invalid state
	verdictJSON, err := json.MarshalIndent(reconstructedVerdict, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixture.rootDir, "matrix-verdict.json"), verdictJSON, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// P0-6 FIX: Use full regeneration to update manifest and all checksums
	mustRegenerateAllChecksums(t, fixture)

	// Load stored verdict from disk and assert zero differences with reconstructed
	diskVerdictData, err := os.ReadFile(filepath.Join(fixture.rootDir, "matrix-verdict.json"))
	if err != nil {
		t.Fatalf("read stored verdict: %v", err)
	}
	var diskVerdict MatrixVerdict
	if err := json.Unmarshal(diskVerdictData, &diskVerdict); err != nil {
		t.Fatalf("unmarshal stored verdict: %v", err)
	}
	diffs := CompareVerdicts(&diskVerdict, reconstructedVerdict)
	if len(diffs) > 0 {
		t.Fatalf("stored and reconstructed should have zero differences:\n%s", FormatVerdictDiffs(diffs))
	}

	// P0-6 FIX: Assert stored verdict is also invalid
	if diskVerdict.MatrixValid {
		t.Fatal("stored verdict should be invalid")
	}

	// P0-6 FIX: Use authoritative VerifyChildRun dependency (not empty struct)
	// P0-6 FIX: Assert exact terminal reason, exclude earlier failures
	result, err := VerifyMatrixBundle(fixture.rootDir, MatrixVerificationDeps{
		VerifyChildRun: verifyChildRunBundle,
	})

	// Verify should fail (equal-invalid is forbidden)
	if err == nil {
		t.Fatal("expected error for equal-invalid verdict")
	}

	// P0-6 FIX: Assert the error is specifically about matrix invalidity, not earlier failures
	errMsg := err.Error()
	if !strings.Contains(errMsg, "invalid") {
		t.Fatalf("wrong rejection boundary: %v", err)
	}

	// P0-6 FIX: Exclude earlier failure boundaries
	earlierBoundaries := []string{
		"checksum mismatch",
		"missing checksum",
		"unexpected artifact",
		"child verification",
	}
	for _, boundary := range earlierBoundaries {
		if strings.Contains(errMsg, boundary) {
			t.Fatalf("failed before equal-invalid policy: %v", err)
		}
	}

	// P0-6 FIX: Verify result shows reconstructed invalid but stored also invalid (equal-invalid)
	if result != nil {
		if result.ReconstructedVerdict.MatrixValid {
			t.Error("reconstructed verdict should be invalid")
		}
		if result.StoredVerdict != nil && result.StoredVerdict.MatrixValid {
			t.Error("stored verdict should be invalid")
		}
	}
}

// =============================================================================
// CHECKSUM FAILURE TESTS
// =============================================================================

// TestVerifyMatrixBundle_RejectsChecksumMismatch proves checksum failures fail.
func TestVerifyMatrixBundle_RejectsChecksumMismatch(t *testing.T) {
	// P0-8: Use TempDir fixture - automatic cleanup
	fixture := writeValidMatrixBundleFixture(t)

	// Corrupt a file without updating checksums
	manifestPath := filepath.Join(fixture.rootDir, "matrix-manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	data = append(data, "corrupted"...)
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		t.Fatalf("write corrupted manifest: %v", err)
	}

	// Verify should fail
	_, err = VerifyMatrixBundle(fixture.rootDir, MatrixVerificationDeps{
		VerifyChildRun: verifyChildRunBundle,
	})
	if err == nil {
		t.Error("expected error for checksum mismatch")
	}
}

// =============================================================================
// FAIL-CLOSED REGENERATION TESTS
// =============================================================================

// TestRegenerateAllChecksums_RefreshesChildAndMatrixAuthority proves regeneration works.
// NOTE: This test uses container-inspect mutation which WILL cause cleanup binding failure.
// The test proves that regeneration succeeds even when the result fails verification.
// The key is that regeneration itself is fail-closed (no errors ignored).
func TestRegenerateAllChecksums_RefreshesChildAndMatrixAuthority(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Mutate a child artifact (container-inspect.json)
	mutateChildContainerID(fixture, t)

	// Regenerate checksums - this should succeed even though verification will fail
	mustRegenerateAllChecksums(t, fixture)

	// Verification SHOULD fail because container ID mismatch with cleanup
	// But the key point is that regeneration itself succeeded (fail-closed, no errors ignored)
	result, err := VerifyMatrixBundle(fixture.rootDir, MatrixVerificationDeps{
		VerifyChildRun: verifyChildRunBundle,
	})
	// We expect an error due to cleanup binding mismatch
	if err == nil {
		t.Error("expected verification to fail due to container ID mismatch")
	}
	// Result may be nil or partial
	if result != nil && result.ReconstructedVerdict.MatrixValid {
		t.Error("expected matrix invalid due to container ID mismatch")
	}
}

// TestRegenerateAllChecksums_StopsOnManifestMarshalFailure proves error propagation.
// P0-7 FIX: Uses fixtureFileOps with deterministic failing marshal.
func TestRegenerateAllChecksums_StopsOnManifestMarshalFailure(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Inject a failing marshal function
	failingOps := fixtureFileOps{
		readFile: os.ReadFile,
		writeFile: func(path string, data []byte, perm os.FileMode) error {
			// Only fail on manifest writes
			if strings.HasSuffix(path, "matrix-manifest.json") {
				return errors.New("injected marshal failure")
			}
			return os.WriteFile(path, data, perm)
		},
		marshal: func(v any, prefix, indent string) ([]byte, error) {
			return nil, errors.New("injected marshal failure")
		},
	}

	// Apply a mutation that would require manifest rewrite
	mutateChildContainerID(fixture, t)

	// Attempt regeneration with failing ops - should fail
	err := regenerateAllChecksumsWithOps(fixture, failingOps)
	if err == nil {
		t.Error("expected error when marshal fails")
	}
	if !strings.Contains(err.Error(), "marshal manifest") {
		t.Errorf("expected marshal error, got: %v", err)
	}
}

// P0-7: Test fail-closed when readFile fails during checksum computation.
func TestComputeChecksumsContentWithOps_FailsOnReadError(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Inject a failing readFile
	failingOps := fixtureFileOps{
		readFile: func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "manifest.json") {
				return nil, errors.New("injected read failure")
			}
			return os.ReadFile(path)
		},
		writeFile: os.WriteFile,
		marshal:   json.MarshalIndent,
	}

	runDir := fixture.runDirs[0]
	_, err := computeChecksumsContentWithOps(runDir, canonicalChildArtifactInventory, failingOps)
	if err == nil {
		t.Error("expected error when readFile fails")
	}
	if !strings.Contains(err.Error(), "read artifact") {
		t.Errorf("expected read error, got: %v", err)
	}
}

// P0-7: Test fail-closed when writeFile fails for child checksums.
func TestRegenerateAllChecksumsWithOps_FailsOnChildChecksumWrite(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Inject a failing writeFile for child checksums
	failingOps := fixtureFileOps{
		readFile: os.ReadFile,
		writeFile: func(path string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(path, "checksums.txt") && !strings.Contains(path, "matrix-checksums") {
				return errors.New("injected child checksum write failure")
			}
			return os.WriteFile(path, data, perm)
		},
		marshal: json.MarshalIndent,
	}

	err := regenerateAllChecksumsWithOps(fixture, failingOps)
	if err == nil {
		t.Error("expected error when child checksum write fails")
	}
	if !strings.Contains(err.Error(), "write child checksums") {
		t.Errorf("expected write error, got: %v", err)
	}
}

// P0-7: Test fail-closed when writeFile fails for matrix checksums.
func TestRegenerateAllChecksumsWithOps_FailsOnMatrixChecksumWrite(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Inject a failing writeFile for matrix checksums
	failingOps := fixtureFileOps{
		readFile: os.ReadFile,
		writeFile: func(path string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(path, "matrix-checksums.txt") {
				return errors.New("injected matrix checksum write failure")
			}
			return os.WriteFile(path, data, perm)
		},
		marshal: json.MarshalIndent,
	}

	err := regenerateAllChecksumsWithOps(fixture, failingOps)
	if err == nil {
		t.Error("expected error when matrix checksum write fails")
	}
	if !strings.Contains(err.Error(), "write matrix checksums") {
		t.Errorf("expected write error, got: %v", err)
	}
}

// P0-7: Test fail-closed when writeFile fails for manifest.json.
func TestRegenerateAllChecksumsWithOps_FailsOnManifestWrite(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Inject a failing writeFile for manifest
	failingOps := fixtureFileOps{
		readFile: os.ReadFile,
		writeFile: func(path string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(path, "matrix-manifest.json") {
				return errors.New("injected manifest write failure")
			}
			return os.WriteFile(path, data, perm)
		},
		marshal: json.MarshalIndent,
	}

	err := regenerateAllChecksumsWithOps(fixture, failingOps)
	if err == nil {
		t.Error("expected error when manifest write fails")
	}
	if !strings.Contains(err.Error(), "write manifest") {
		t.Errorf("expected write error, got: %v", err)
	}
}

// P0-7: Test fail-closed when writeFile fails for matrix-verdict.json.
func TestRegenerateAllChecksumsWithOps_FailsOnVerdictWrite(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Inject a failing writeFile for verdict
	failingOps := fixtureFileOps{
		readFile: os.ReadFile,
		writeFile: func(path string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(path, "matrix-verdict.json") {
				return errors.New("injected verdict write failure")
			}
			return os.WriteFile(path, data, perm)
		},
		marshal: json.MarshalIndent,
	}

	err := regenerateAllChecksumsWithOps(fixture, failingOps)
	if err == nil {
		t.Error("expected error when verdict write fails")
	}
	if !strings.Contains(err.Error(), "write verdict") {
		t.Errorf("expected write error, got: %v", err)
	}
}

// P0-7: Test first-failure-order proof for cleanup marshal failure.
// Verifies that child checksums and manifest marshal were called BEFORE cleanup marshal,
// and that NO later operations were called after the failure.
func TestRegenerateAllChecksumsWithOps_FirstFailureOrder_CleanupMarshal(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Track call order
	var calls []string
	failingOps := fixtureFileOps{
		readFile: os.ReadFile,
		writeFile: func(path string, data []byte, perm os.FileMode) error {
			calls = append(calls, "write:"+filepath.Base(path))
			return os.WriteFile(path, data, perm)
		},
		marshal: func(v any, prefix, indent string) ([]byte, error) {
			typeName := fmt.Sprintf("%T", v)
			calls = append(calls, "marshal:"+typeName)
			// Fail ONLY on cleanup marshal - %T returns *main.MatrixCleanupEvidence for pointers
			if strings.Contains(typeName, "MatrixCleanup") {
				return nil, errors.New("injected cleanup marshal failure")
			}
			return json.MarshalIndent(v, prefix, indent)
		},
	}

	err := regenerateAllChecksumsWithOps(fixture, failingOps)
	if err == nil {
		t.Fatal("expected error when cleanup marshal fails")
	}
	if !strings.Contains(err.Error(), "marshal cleanup") {
		t.Errorf("expected marshal cleanup error, got: %v", err)
	}

	// P0-7: Verify call order - child checksums must come BEFORE cleanup marshal
	// Expected: write:checksums.txt (run 0), marshal:*main.MatrixManifest, marshal:*main.MatrixCleanup (FAILS here)
	hasChildChecksumWrite := false
	hasManifestMarshal := false
	hasCleanupMarshal := false
	cleanupMarshalIndex := -1

	for i, call := range calls {
		if call == "write:checksums.txt" {
			hasChildChecksumWrite = true
		}
		if strings.Contains(call, "MatrixManifest") {
			hasManifestMarshal = true
		}
		if strings.Contains(call, "MatrixCleanup") {
			hasCleanupMarshal = true
			cleanupMarshalIndex = i
		}
	}

	if !hasChildChecksumWrite {
		t.Error("child checksum write should have been called before cleanup marshal failure")
	}
	if !hasManifestMarshal {
		t.Error("manifest marshal should have been called before cleanup marshal failure")
	}
	if !hasCleanupMarshal {
		t.Fatal("cleanup marshal should have been called (and failed)")
	}
	if cleanupMarshalIndex <= 0 {
		t.Error("cleanup marshal should have been called after earlier operations")
	}

	// P0-7 STRENGTHENED: Verify NO later operations were called
	// After cleanup marshal fails, these should NOT appear:
	// - write:matrix-cleanup.json
	// - marshal:*MatrixVerdict
	// - write:matrix-verdict.json
	// - write:matrix-checksums.txt
	for _, call := range calls {
		if call == "write:matrix-cleanup.json" {
			t.Error("cleanup write should NOT be called after cleanup marshal failure")
		}
		if strings.Contains(call, "MatrixVerdict") {
			t.Error("verdict marshal should NOT be called after cleanup marshal failure")
		}
		if call == "write:matrix-verdict.json" {
			t.Error("verdict write should NOT be called after cleanup marshal failure")
		}
		if call == "write:matrix-checksums.txt" {
			t.Error("matrix checksums write should NOT be called after cleanup marshal failure")
		}
	}
}

// P0-7: Test first-failure-order proof for verdict marshal failure.
// Verifies cleanup marshal was called BEFORE verdict marshal,
// and that NO later operations were called after the failure.
func TestRegenerateAllChecksumsWithOps_FirstFailureOrder_VerdictMarshal(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Track call order
	var calls []string
	failingOps := fixtureFileOps{
		readFile: os.ReadFile,
		writeFile: func(path string, data []byte, perm os.FileMode) error {
			calls = append(calls, "write:"+filepath.Base(path))
			return os.WriteFile(path, data, perm)
		},
		marshal: func(v any, prefix, indent string) ([]byte, error) {
			typeName := fmt.Sprintf("%T", v)
			calls = append(calls, "marshal:"+typeName)
			// Fail ONLY on verdict marshal - %T returns *main.MatrixVerdict for pointers
			if strings.Contains(typeName, "MatrixVerdict") {
				return nil, errors.New("injected verdict marshal failure")
			}
			return json.MarshalIndent(v, prefix, indent)
		},
	}

	err := regenerateAllChecksumsWithOps(fixture, failingOps)
	if err == nil {
		t.Fatal("expected error when verdict marshal fails")
	}
	if !strings.Contains(err.Error(), "marshal verdict") {
		t.Errorf("expected marshal verdict error, got: %v", err)
	}

	// P0-7: Verify call order
	hasCleanupMarshal := false
	hasVerdictMarshal := false
	verdictMarshalIndex := -1

	for i, call := range calls {
		if strings.Contains(call, "MatrixCleanup") {
			hasCleanupMarshal = true
		}
		if strings.Contains(call, "MatrixVerdict") {
			hasVerdictMarshal = true
			verdictMarshalIndex = i
		}
	}

	if !hasCleanupMarshal {
		t.Error("cleanup marshal should have been called before verdict marshal failure")
	}
	if !hasVerdictMarshal {
		t.Fatal("verdict marshal should have been called (and failed)")
	}
	if verdictMarshalIndex <= 0 {
		t.Error("verdict marshal should have been called after cleanup marshal")
	}

	// P0-7 STRENGTHENED: Verify NO later operations were called
	// After verdict marshal fails, these should NOT appear:
	// - write:matrix-verdict.json
	// - write:matrix-checksums.txt
	for _, call := range calls {
		if call == "write:matrix-verdict.json" {
			t.Error("verdict write should NOT be called after verdict marshal failure")
		}
		if call == "write:matrix-checksums.txt" {
			t.Error("matrix checksums write should NOT be called after verdict marshal failure")
		}
	}
}

// P0-7: Test first-failure-order proof for cleanup write failure.
// Verifies earlier operations complete before cleanup write fails.
// NOTE: When cleanup write fails, verdict marshal is NOT called (we fail before reaching it).
func TestRegenerateAllChecksumsWithOps_FirstFailureOrder_CleanupWrite(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Track call order
	var calls []string
	failingOps := fixtureFileOps{
		readFile: os.ReadFile,
		writeFile: func(path string, data []byte, perm os.FileMode) error {
			calls = append(calls, "write:"+filepath.Base(path))
			// Fail ONLY on cleanup write
			if filepath.Base(path) == "matrix-cleanup.json" {
				return errors.New("injected cleanup write failure")
			}
			return os.WriteFile(path, data, perm)
		},
		marshal: func(v any, prefix, indent string) ([]byte, error) {
			calls = append(calls, "marshal:"+fmt.Sprintf("%T", v))
			return json.MarshalIndent(v, prefix, indent)
		},
	}

	err := regenerateAllChecksumsWithOps(fixture, failingOps)
	if err == nil {
		t.Fatal("expected error when cleanup write fails")
	}
	if !strings.Contains(err.Error(), "write cleanup") {
		t.Errorf("expected write cleanup error, got: %v", err)
	}

	// P0-7: Verify cleanup marshal was called before cleanup write fails
	hasCleanupMarshal := false
	hasManifestMarshal := false
	hasCleanupWrite := false
	cleanupMarshalIndex := -1

	for i, call := range calls {
		if strings.Contains(call, "MatrixManifest") {
			hasManifestMarshal = true
		}
		if strings.Contains(call, "MatrixCleanup") {
			hasCleanupMarshal = true
			cleanupMarshalIndex = i
		}
		if call == "write:matrix-cleanup.json" {
			hasCleanupWrite = true
		}
	}

	if !hasManifestMarshal {
		t.Error("manifest marshal should have been called before cleanup write failure")
	}
	if !hasCleanupMarshal {
		t.Fatal("cleanup marshal should have been called before cleanup write fails")
	}
	if !hasCleanupWrite {
		t.Fatal("cleanup write should have been called (and failed)")
	}
	if cleanupMarshalIndex <= 0 {
		t.Error("cleanup marshal should have been called after manifest marshal")
	}
	// NOTE: Verdict marshal is NOT called when cleanup write fails - we fail before reaching it
	hasVerdictMarshal := false
	for _, call := range calls {
		if strings.Contains(call, "MatrixVerdict") {
			hasVerdictMarshal = true
			break
		}
	}
	if hasVerdictMarshal {
		t.Error("verdict marshal should NOT be called when cleanup write fails (fail-closed)")
	}
}

// P0-7: Test fail-closed when checksum file write fails due to permission error simulation.
func TestRegenerateAllChecksumsWithOps_FailsOnPermissionDenied(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Simulate permission denied error wrapped in PathError
	failingOps := fixtureFileOps{
		readFile: os.ReadFile,
		writeFile: func(path string, data []byte, perm os.FileMode) error {
			return &os.PathError{
				Op:   "write",
				Path: path,
				Err:  errors.New("permission denied"),
			}
		},
		marshal: json.MarshalIndent,
	}

	err := regenerateAllChecksumsWithOps(fixture, failingOps)
	if err == nil {
		t.Fatal("expected error when permission denied")
	}
	// Assert the error wraps the PathError boundary
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("expected PathError wrapper, got: %T", err)
	}
	if !strings.Contains(pathErr.Err.Error(), "permission denied") {
		t.Errorf("expected 'permission denied' in PathError.Err, got: %v", pathErr.Err)
	}
}

// P0-7: Test fail-closed when samples.csv read fails during child checksum computation.
func TestComputeChecksumsContentWithOps_FailsOnSamplesRead(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Fail on samples.csv read
	failingOps := fixtureFileOps{
		readFile: func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "samples.csv") {
				return nil, errors.New("injected read failure for samples")
			}
			return os.ReadFile(path)
		},
		writeFile: os.WriteFile,
		marshal:   json.MarshalIndent,
	}

	runDir := fixture.runDirs[0]
	_, err := computeChecksumsContentWithOps(runDir, canonicalChildArtifactInventory, failingOps)
	if err == nil {
		t.Fatal("expected error when samples.csv read fails")
	}
	if !strings.Contains(err.Error(), "read artifact") {
		t.Errorf("expected read error, got: %v", err)
	}
}

// P0-7: Test fail-closed when network-identity.json read fails.
func TestComputeChecksumsContentWithOps_FailsOnNetworkIdentityRead(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Fail on network-identity.json read
	failingOps := fixtureFileOps{
		readFile: func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "network-identity.json") {
				return nil, errors.New("injected network-identity read failure")
			}
			return os.ReadFile(path)
		},
		writeFile: os.WriteFile,
		marshal:   json.MarshalIndent,
	}

	runDir := fixture.runDirs[0]
	_, err := computeChecksumsContentWithOps(runDir, canonicalChildArtifactInventory, failingOps)
	if err == nil {
		t.Error("expected error when network-identity read fails")
	}
}

// P0-7: Test fail-closed when container-inspect.json read fails.
func TestComputeChecksumsContentWithOps_FailsOnContainerInspectRead(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Fail on container-inspect.json read
	failingOps := fixtureFileOps{
		readFile: func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "container-inspect.json") {
				return nil, errors.New("injected container-inspect read failure")
			}
			return os.ReadFile(path)
		},
		writeFile: os.WriteFile,
		marshal:   json.MarshalIndent,
	}

	runDir := fixture.runDirs[0]
	_, err := computeChecksumsContentWithOps(runDir, canonicalChildArtifactInventory, failingOps)
	if err == nil {
		t.Error("expected error when container-inspect read fails")
	}
}

// P0-7: Test fail-closed when matrix-root artifact (matrix-cleanup.json) read fails during
// matrix checksum computation. This proves the seam handles matrix-level artifact reads.
func TestComputeMatrixChecksumsContentWithOps_FailsOnMatrixCleanupRead(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Fail on matrix-cleanup.json read during matrix checksum computation
	failingOps := fixtureFileOps{
		readFile: func(path string) ([]byte, error) {
			if strings.HasSuffix(path, "matrix-cleanup.json") {
				return nil, errors.New("injected matrix-cleanup read failure")
			}
			return os.ReadFile(path)
		},
		writeFile: os.WriteFile,
		marshal:   json.MarshalIndent,
	}

	_, err := computeMatrixChecksumsContentWithOps(fixture.rootDir, canonicalMatrixArtifactInventory[:], failingOps)
	if err == nil {
		t.Fatal("expected error when matrix-cleanup read fails during matrix checksum computation")
	}
	if !strings.Contains(err.Error(), "read artifact") {
		t.Errorf("expected read error, got: %v", err)
	}
}

// P0-7 FIX: Regenerate with injected file operations.
// All file reads and writes go through ops for complete testability.
func regenerateAllChecksumsWithOps(fixture *matrixFixture, ops fixtureFileOps) error {
	// Step 1: Regenerate child checksums for all runs using ops
	for i, runDir := range fixture.runDirs {
		// P0-7 FIX: Use ops-injected checksum computation
		childChecksumContent, err := computeChildChecksumsContentWithOps(runDir, canonicalChildArtifactInventory, ops)
		if err != nil {
			return fmt.Errorf("compute child checksums for %s: %w", fixtureRunIDs[i], err)
		}

		// P0-7 FIX: Write child checksums using ops
		if err := ops.writeFile(filepath.Join(runDir, "checksums.txt"), []byte(childChecksumContent), 0644); err != nil {
			return fmt.Errorf("write child checksums for %s: %w", fixtureRunIDs[i], err)
		}

		// Step 2: Update manifest's checksums_sha256 for this run
		checksumHash := sha256.Sum256([]byte(childChecksumContent))
		childDigest := hex.EncodeToString(checksumHash[:])
		if err := updateDeclaredChildChecksum(fixture.manifest, i, childDigest); err != nil {
			return fmt.Errorf("update child checksum digest: %w", err)
		}
	}

	// Step 3: Rewrite matrix manifest with injected ops
	manifestJSON, err := ops.marshal(fixture.manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := ops.writeFile(filepath.Join(fixture.rootDir, "matrix-manifest.json"), manifestJSON, 0644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	// Step 4: Rewrite matrix cleanup with injected ops
	cleanupJSON, err := ops.marshal(fixture.cleanup, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cleanup: %w", err)
	}
	if err := ops.writeFile(filepath.Join(fixture.rootDir, "matrix-cleanup.json"), cleanupJSON, 0644); err != nil {
		return fmt.Errorf("write cleanup: %w", err)
	}

	// Step 5: Rewrite matrix verdict with injected ops
	verdictJSON, err := ops.marshal(fixture.verdict, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal verdict: %w", err)
	}
	if err := ops.writeFile(filepath.Join(fixture.rootDir, "matrix-verdict.json"), verdictJSON, 0644); err != nil {
		return fmt.Errorf("write verdict: %w", err)
	}

	// Step 6: Regenerate matrix checksums using ops
	// P0-7 FIX: Use ops-injected matrix checksum computation
	matrixChecksumContent, err := computeMatrixChecksumsContentWithOps(fixture.rootDir, canonicalMatrixArtifactInventory[:], ops)
	if err != nil {
		return fmt.Errorf("compute matrix checksums: %w", err)
	}
	if err := ops.writeFile(filepath.Join(fixture.rootDir, "matrix-checksums.txt"), []byte(matrixChecksumContent), 0644); err != nil {
		return fmt.Errorf("write matrix checksums: %w", err)
	}

	return nil
}

// P0-7: Test that checksum regeneration fails when file is missing.
func TestComputeChecksumsContent_FailsOnMissingFile(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Delete a child artifact file
	runDir := fixture.runDirs[0]
	manifestPath := filepath.Join(runDir, "manifest.json")
	if err := os.Remove(manifestPath); err != nil {
		t.Fatalf("remove manifest: %v", err)
	}

	// Compute checksums should fail
	_, err := computeChildChecksumsContent(runDir)
	if err == nil {
		t.Error("expected error when artifact is missing")
	}
}

// P0-7: Test that manifest checksums_sha256 is updated on disk after regeneration.
func TestRegenerateAllChecksums_UpdatesManifestOnDisk(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Mutate a child artifact
	mutateChildContainerID(fixture, t)

	// Regenerate checksums
	mustRegenerateAllChecksums(t, fixture)

	// Load manifest from disk and verify checksums_sha256 is updated
	manifestData, err := os.ReadFile(filepath.Join(fixture.rootDir, "matrix-manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var diskManifest MatrixManifest
	if err := json.Unmarshal(manifestData, &diskManifest); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// All runs should have non-empty checksums_sha256
	for i, run := range diskManifest.Runs {
		if run.ChecksumsSHA256 == "" {
			t.Errorf("run[%d] checksums_sha256 is empty", i)
		}
	}
}

// P0-5: Test actual CLI execution of verify-matrix command.
// Fixed: Uses correct package directory, CommandContext, and --matrix-dir flag.
func TestVerifyMatrixCommand_CLIExecution(t *testing.T) {
	// Build the CLI binary from the current package directory
	// Tests run from: /home/kgb/Projects/KGB/tovarisch/labs/memory/cmd/tovarisch-memory-lab
	// Use "." to build from current directory
	pkgDir := "."
	binPath := filepath.Join(t.TempDir(), "tovarisch-memory-lab")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, ".")
	cmd.Dir = pkgDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build failed: %v", err)
	}

	// Run verify-matrix with --matrix-dir flag
	fixture := writeValidMatrixBundleFixture(t)
	verifyCmd := exec.CommandContext(ctx, binPath, "verify-matrix", "--matrix-dir", fixture.rootDir)
	stdout, stderr := &strings.Builder{}, &strings.Builder{}
	verifyCmd.Stdout, verifyCmd.Stderr = stdout, stderr
	err := verifyCmd.Run()

	// Should succeed
	if err != nil {
		t.Errorf("verify-matrix failed: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	// Should emit PASS line - P0-5 FIX: Use combined assertion for both stdout and stderr
	output := stdout.String()
	assertTerminalPass(t, output, stderr.String())
}

// P0-5: Test CLI fails on equal-invalid fixture.
// P0-5 FIX: Uses correct --matrix-dir flag, captures output, checks ExitError.
func TestVerifyMatrixCommand_CLIRejectsEqualInvalid(t *testing.T) {
	// Build the CLI binary from the current package directory
	pkgDir := "."
	binPath := filepath.Join(t.TempDir(), "tovarisch-memory-lab")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, ".")
	cmd.Dir = pkgDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build failed: %v", err)
	}

	// Create equal-invalid fixture
	fixture := createEqualInvalidFixture(t)

	// P0-5 FIX: Use correct --matrix-dir flag and capture output
	verifyCmd := exec.CommandContext(ctx, binPath, "verify-matrix", "--matrix-dir", fixture.rootDir)
	var stdout, stderr strings.Builder
	verifyCmd.Stdout, verifyCmd.Stderr = &stdout, &stderr
	err := verifyCmd.Run()

	// Should fail with non-zero exit
	if err == nil {
		t.Fatal("expected nonzero exit")
	}

	// Verify it's an ExitError, not a timeout
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("CLI infrastructure failure: %v", err)
	}

	// Verify it wasn't a timeout
	if ctx.Err() != nil {
		t.Fatalf("CLI exceeded timeout: %v", ctx.Err())
	}

	// P0-5 FIX: Assert no PASS line in output
	assertNoTerminalPass(t, stdout.String(), stderr.String())
}

// P0-5: Test CLI fails when container status is "exists".
// P0-5 FIX: Uses actual CLI execution with file-based mutation.
func TestVerifyMatrixCommand_CLIRejectsContainerStatusExists(t *testing.T) {
	pkgDir := "."
	binPath := filepath.Join(t.TempDir(), "tovarisch-memory-lab")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, ".")
	cmd.Dir = pkgDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build failed: %v", err)
	}

	// Create valid fixture
	fixture := writeValidMatrixBundleFixture(t)

	// Mutate cleanup to have container status "exists" (should be "gone")
	fixture.cleanup.Runs[0].Container.Status = "exists"
	cleanupJSON, err := json.MarshalIndent(fixture.cleanup, "", "  ")
	if err != nil {
		t.Fatalf("marshal cleanup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixture.rootDir, "matrix-cleanup.json"), cleanupJSON, 0644); err != nil {
		t.Fatalf("write cleanup: %v", err)
	}

	// Regenerate matrix checksums
	if err := regenerateMatrixChecksums(fixture.rootDir); err != nil {
		t.Fatalf("regenerate checksums: %v", err)
	}

	// Run verify-matrix CLI
	verifyCmd := exec.CommandContext(ctx, binPath, "verify-matrix", "--matrix-dir", fixture.rootDir)
	var stdout, stderr strings.Builder
	verifyCmd.Stdout, verifyCmd.Stderr = &stdout, &stderr
	err = verifyCmd.Run()

	// Should fail
	if err == nil {
		t.Fatal("expected nonzero exit")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("CLI infrastructure failure: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("CLI exceeded timeout: %v", ctx.Err())
	}

	// No PASS line
	assertNoTerminalPass(t, stdout.String(), stderr.String())

	// P0-5 FIX: Assert field-specific diagnostic
	output := stdout.String() + stderr.String()
	assertCLIRejectedFor(t, output, "container", "exists")
}

// P0-5: Test CLI fails when network status is "exists".
// P0-5 FIX: Uses actual CLI execution with file-based mutation.
func TestVerifyMatrixCommand_CLIRejectsNetworkStatusExists(t *testing.T) {
	pkgDir := "."
	binPath := filepath.Join(t.TempDir(), "tovarisch-memory-lab")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, ".")
	cmd.Dir = pkgDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build failed: %v", err)
	}

	// Create valid fixture
	fixture := writeValidMatrixBundleFixture(t)

	// Mutate cleanup to have network status "exists" (should be "gone")
	fixture.cleanup.Runs[0].Network.Status = "exists"
	cleanupJSON, err := json.MarshalIndent(fixture.cleanup, "", "  ")
	if err != nil {
		t.Fatalf("marshal cleanup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixture.rootDir, "matrix-cleanup.json"), cleanupJSON, 0644); err != nil {
		t.Fatalf("write cleanup: %v", err)
	}

	// Regenerate matrix checksums
	if err := regenerateMatrixChecksums(fixture.rootDir); err != nil {
		t.Fatalf("regenerate checksums: %v", err)
	}

	// Run verify-matrix CLI
	verifyCmd := exec.CommandContext(ctx, binPath, "verify-matrix", "--matrix-dir", fixture.rootDir)
	var stdout, stderr strings.Builder
	verifyCmd.Stdout, verifyCmd.Stderr = &stdout, &stderr
	err = verifyCmd.Run()

	// Should fail
	if err == nil {
		t.Fatal("expected nonzero exit")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("CLI infrastructure failure: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("CLI exceeded timeout: %v", ctx.Err())
	}

	// No PASS line
	assertNoTerminalPass(t, stdout.String(), stderr.String())

	// P0-5 FIX: Assert field-specific diagnostic
	output := stdout.String() + stderr.String()
	assertCLIRejectedFor(t, output, "network", "exists")
}

// P0-5: Test CLI fails when process status is "still_alive".
// P0-5 FIX: Uses actual CLI execution with file-based mutation.
func TestVerifyMatrixCommand_CLIRejectsProcessStatusStillAlive(t *testing.T) {
	pkgDir := "."
	binPath := filepath.Join(t.TempDir(), "tovarisch-memory-lab")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, ".")
	cmd.Dir = pkgDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build failed: %v", err)
	}

	// Create valid fixture
	fixture := writeValidMatrixBundleFixture(t)

	// Mutate cleanup to have process status "still_alive" (should be "gone" or "pid_reused")
	fixture.cleanup.Runs[0].Process.Status = "still_alive"
	cleanupJSON, err := json.MarshalIndent(fixture.cleanup, "", "  ")
	if err != nil {
		t.Fatalf("marshal cleanup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixture.rootDir, "matrix-cleanup.json"), cleanupJSON, 0644); err != nil {
		t.Fatalf("write cleanup: %v", err)
	}

	// Regenerate matrix checksums
	if err := regenerateMatrixChecksums(fixture.rootDir); err != nil {
		t.Fatalf("regenerate checksums: %v", err)
	}

	// Run verify-matrix CLI
	verifyCmd := exec.CommandContext(ctx, binPath, "verify-matrix", "--matrix-dir", fixture.rootDir)
	var stdout, stderr strings.Builder
	verifyCmd.Stdout, verifyCmd.Stderr = &stdout, &stderr
	err = verifyCmd.Run()

	// Should fail
	if err == nil {
		t.Fatal("expected nonzero exit")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("CLI infrastructure failure: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("CLI exceeded timeout: %v", ctx.Err())
	}

	// No PASS line
	assertNoTerminalPass(t, stdout.String(), stderr.String())

	// P0-5 FIX: Assert field-specific diagnostic
	output := stdout.String() + stderr.String()
	assertCLIRejectedFor(t, output, "process", "still_alive")
}

// =============================================================================
// CLI UNAVAILABLE STATUS TESTS (table-driven)
// =============================================================================

// cliUnavailableTest describes a CLI unavailable-status test case.
type cliUnavailableTest struct {
	name           string
	runIndex       int
	field          string
	setUnavailable func(*MatrixCleanupEvidence, int)
}

// TestVerifyMatrixCommand_CLIRejectsUnavailableStatuses proves CLI rejects unavailable statuses.
// P0-5: Table-driven tests for unavailable statuses to reduce duplication.
func TestVerifyMatrixCommand_CLIRejectsUnavailableStatuses(t *testing.T) {
	tests := []cliUnavailableTest{
		{
			name:     "container unavailable",
			runIndex: 0,
			field:    "container",
			setUnavailable: func(c *MatrixCleanupEvidence, i int) {
				c.Runs[i].Container.Status = "unavailable"
			},
		},
		{
			name:     "network unavailable",
			runIndex: 0,
			field:    "network",
			setUnavailable: func(c *MatrixCleanupEvidence, i int) {
				c.Runs[i].Network.Status = "unavailable"
			},
		},
		{
			name:     "process unavailable",
			runIndex: 0,
			field:    "process",
			setUnavailable: func(c *MatrixCleanupEvidence, i int) {
				c.Runs[i].Process.Status = "unavailable"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkgDir := "."
			binPath := filepath.Join(t.TempDir(), "tovarisch-memory-lab")

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, ".")
			cmd.Dir = pkgDir
			if err := cmd.Run(); err != nil {
				t.Fatalf("go build failed: %v", err)
			}

			// Create valid fixture
			fixture := writeValidMatrixBundleFixture(t)

			// Set field to unavailable
			tt.setUnavailable(fixture.cleanup, tt.runIndex)
			cleanupJSON, err := json.MarshalIndent(fixture.cleanup, "", "  ")
			if err != nil {
				t.Fatalf("marshal cleanup: %v", err)
			}
			if err := os.WriteFile(filepath.Join(fixture.rootDir, "matrix-cleanup.json"), cleanupJSON, 0644); err != nil {
				t.Fatalf("write cleanup: %v", err)
			}

			// Regenerate matrix checksums
			if err := regenerateMatrixChecksums(fixture.rootDir); err != nil {
				t.Fatalf("regenerate checksums: %v", err)
			}

			// Run verify-matrix CLI
			verifyCmd := exec.CommandContext(ctx, binPath, "verify-matrix", "--matrix-dir", fixture.rootDir)
			var stdout, stderr strings.Builder
			verifyCmd.Stdout, verifyCmd.Stderr = &stdout, &stderr
			err = verifyCmd.Run()

			// Should fail
			if err == nil {
				t.Fatal("expected nonzero exit")
			}
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("CLI infrastructure failure: %v", err)
			}
			if ctx.Err() != nil {
				t.Fatalf("CLI exceeded timeout: %v", ctx.Err())
			}

			// No PASS line
			assertNoTerminalPass(t, stdout.String(), stderr.String())

			// P0-5: Assert field-specific diagnostic
			output := stdout.String() + stderr.String()
			assertCLIRejectedFor(t, output, tt.field, "unavailable")
		})
	}
}

// =============================================================================
// PASS LINE CONTRACT TESTS
// =============================================================================

// TestVerifyMatrixCommand_ValidFixtureEmitsPASS proves valid fixture emits PASS.
// P0-5: This is a string matching test, not CLI execution.
func TestVerifyMatrixCommand_ValidFixtureEmitsPASS(t *testing.T) {
	output := "All Checks: PASS\n"

	count := countTerminalPassLines(output)
	if count != 1 {
		t.Errorf("expected 1 PASS line, got %d", count)
	}
}

// TestVerifyMatrixCommand_FailureEmitsNoPASS proves failures emit no PASS.
// P0-5: This is a string matching test, not CLI execution.
func TestVerifyMatrixCommand_FailureEmitsNoPASS(t *testing.T) {
	output := "ERROR: cleanup PID mismatch detected\n"

	count := countTerminalPassLines(output)
	if count != 0 {
		t.Errorf("expected 0 PASS lines, got %d", count)
	}
}

// assertCLIRejectedFor asserts the CLI failed for the specific reason.
// P0-5: Verifies the failure is at the expected boundary, not an unrelated error.
func assertCLIRejectedFor(t *testing.T, output string, field, expected string) {
	t.Helper()
	// Check for the specific field and expected value in output
	expectedLower := strings.ToLower(expected)
	fieldLower := strings.ToLower(field)
	if !strings.Contains(output, fieldLower) {
		t.Errorf("expected output to contain %q, got: %s", fieldLower, output)
	}
	if !strings.Contains(output, expectedLower) {
		t.Errorf("expected output to contain %q for %s, got: %s", expectedLower, fieldLower, output)
	}
}
