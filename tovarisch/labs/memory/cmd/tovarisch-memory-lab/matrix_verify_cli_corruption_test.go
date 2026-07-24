// matrix_verify_cli_corruption_test.go — CLI Corruption Matrix Tests
//
// ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION03-P0-CLI-CORRUPTION-MATRIX01
//
// Tests the actual verify-matrix command execution against checksum and JSON-envelope corruptions.
// Each test crosses the production command-line boundary with real subprocess execution.
//
// P0-CLI-CORRUPTION-MATRIX01 owns:
// - child_checksum: growing, bounded, descriptor (3)
// - root_checksum: manifest, cleanup, verdict (3)
// - unknown_fields: child, root (2)
// - trailing_documents: child, root (2)
// Total: 10 cases

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
)

// =============================================================================
// CLI TEST HELPERS
// =============================================================================

// runCLIVerifyMatrix builds and runs the verify-matrix CLI with the given matrix directory.
func runCLIVerifyMatrix(t *testing.T, matrixDir string) (int, string, string) {
	t.Helper()

	pkgDir := "."
	binPath := filepath.Join(t.TempDir(), "tovarisch-memory-lab")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, ".")
	cmd.Dir = pkgDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build failed: %v", err)
	}

	verifyCmd := exec.CommandContext(ctx, binPath, "verify-matrix", "--matrix-dir", matrixDir)
	stdout, stderr := &strings.Builder{}, &strings.Builder{}
	verifyCmd.Stdout, verifyCmd.Stderr = stdout, stderr
	err := verifyCmd.Run()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("CLI execution error: %v", err)
		}
	}

	return exitCode, stdout.String(), stderr.String()
}

// =============================================================================
// STALE CHILD CHECKSUM TESTS
// =============================================================================

// TestVerifyMatrixCLI_RejectsStaleGrowingChildChecksum proves CLI rejects stale growing child checksum.
func TestVerifyMatrixCLI_RejectsStaleGrowingChildChecksum(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Corrupt growing child verdict without updating checksums
	growingRunDir := fixture.runDirs[0] // index 0 = canary-growing
	verdictPath := filepath.Join(growingRunDir, "verdict.json")
	data, err := os.ReadFile(verdictPath)
	if err != nil {
		t.Fatalf("read verdict: %v", err)
	}
	// Mutate the file content
	data = append(data, "STALE"...)
	if err := os.WriteFile(verdictPath, data, 0644); err != nil {
		t.Fatalf("write corrupted verdict: %v", err)
	}
	// DO NOT regenerate checksums - this is the corruption

	exitCode, stdout, stderr := runCLIVerifyMatrix(t, fixture.rootDir)

	if exitCode == 0 {
		t.Error("expected nonzero exit for stale growing child checksum")
	}
	assertNoTerminalPass(t, stdout, stderr)
	assertErrorIdentifiesArtifact(t, stdout+stderr, "growing")
	assertErrorIdentifiesFailureClass(t, stdout+stderr, "checksum")
}

// TestVerifyMatrixCLI_RejectsStaleBoundedChildChecksum proves CLI rejects stale bounded child checksum.
func TestVerifyMatrixCLI_RejectsStaleBoundedChildChecksum(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Corrupt bounded child verdict without updating checksums
	boundedRunDir := fixture.runDirs[1] // index 1 = canary-bounded
	verdictPath := filepath.Join(boundedRunDir, "verdict.json")
	data, err := os.ReadFile(verdictPath)
	if err != nil {
		t.Fatalf("read verdict: %v", err)
	}
	// Mutate the file content
	data = append(data, "STALE"...)
	if err := os.WriteFile(verdictPath, data, 0644); err != nil {
		t.Fatalf("write corrupted verdict: %v", err)
	}
	// DO NOT regenerate checksums - this is the corruption

	exitCode, stdout, stderr := runCLIVerifyMatrix(t, fixture.rootDir)

	if exitCode == 0 {
		t.Error("expected nonzero exit for stale bounded child checksum")
	}
	assertNoTerminalPass(t, stdout, stderr)
	assertErrorIdentifiesArtifact(t, stdout+stderr, "bounded")
	assertErrorIdentifiesFailureClass(t, stdout+stderr, "checksum")
}

// TestVerifyMatrixCLI_RejectsStaleDescriptorChildChecksum proves CLI rejects stale descriptor child checksum.
func TestVerifyMatrixCLI_RejectsStaleDescriptorChildChecksum(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Corrupt descriptor child verdict without updating checksums
	descriptorRunDir := fixture.runDirs[2] // index 2 = canary-descriptor
	verdictPath := filepath.Join(descriptorRunDir, "verdict.json")
	data, err := os.ReadFile(verdictPath)
	if err != nil {
		t.Fatalf("read verdict: %v", err)
	}
	// Mutate the file content
	data = append(data, "STALE"...)
	if err := os.WriteFile(verdictPath, data, 0644); err != nil {
		t.Fatalf("write corrupted verdict: %v", err)
	}
	// DO NOT regenerate checksums - this is the corruption

	exitCode, stdout, stderr := runCLIVerifyMatrix(t, fixture.rootDir)

	if exitCode == 0 {
		t.Error("expected nonzero exit for stale descriptor child checksum")
	}
	assertNoTerminalPass(t, stdout, stderr)
	assertErrorIdentifiesArtifact(t, stdout+stderr, "descriptor")
	assertErrorIdentifiesFailureClass(t, stdout+stderr, "checksum")
}

// =============================================================================
// STALE ROOT CHECKSUM TESTS
// =============================================================================

// TestVerifyMatrixCLI_RejectsStaleManifestChecksum proves CLI rejects stale matrix manifest checksum.
func TestVerifyMatrixCLI_RejectsStaleManifestChecksum(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Corrupt matrix-manifest.json without updating its checksum
	manifestPath := filepath.Join(fixture.rootDir, "matrix-manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	// Mutate the file content
	data = append(data, "STALE"...)
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		t.Fatalf("write corrupted manifest: %v", err)
	}
	// DO NOT regenerate checksums - this is the corruption

	exitCode, stdout, stderr := runCLIVerifyMatrix(t, fixture.rootDir)

	if exitCode == 0 {
		t.Error("expected nonzero exit for stale manifest checksum")
	}
	assertNoTerminalPass(t, stdout, stderr)
	assertErrorIdentifiesArtifact(t, stdout+stderr, "manifest")
	assertErrorIdentifiesFailureClass(t, stdout+stderr, "checksum")
}

// TestVerifyMatrixCLI_RejectsStaleCleanupChecksum proves CLI rejects stale matrix cleanup checksum.
func TestVerifyMatrixCLI_RejectsStaleCleanupChecksum(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Corrupt matrix-cleanup.json without updating its checksum
	cleanupPath := filepath.Join(fixture.rootDir, "matrix-cleanup.json")
	data, err := os.ReadFile(cleanupPath)
	if err != nil {
		t.Fatalf("read cleanup: %v", err)
	}
	// Mutate the file content
	data = append(data, "STALE"...)
	if err := os.WriteFile(cleanupPath, data, 0644); err != nil {
		t.Fatalf("write corrupted cleanup: %v", err)
	}
	// DO NOT regenerate checksums - this is the corruption

	exitCode, stdout, stderr := runCLIVerifyMatrix(t, fixture.rootDir)

	if exitCode == 0 {
		t.Error("expected nonzero exit for stale cleanup checksum")
	}
	assertNoTerminalPass(t, stdout, stderr)
	assertErrorIdentifiesArtifact(t, stdout+stderr, "cleanup")
	assertErrorIdentifiesFailureClass(t, stdout+stderr, "checksum")
}

// TestVerifyMatrixCLI_RejectsStaleVerdictChecksum proves CLI rejects stale matrix verdict checksum.
func TestVerifyMatrixCLI_RejectsStaleVerdictChecksum(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Corrupt matrix-verdict.json without updating its checksum
	verdictPath := filepath.Join(fixture.rootDir, "matrix-verdict.json")
	data, err := os.ReadFile(verdictPath)
	if err != nil {
		t.Fatalf("read verdict: %v", err)
	}
	// Mutate the file content
	data = append(data, "STALE"...)
	if err := os.WriteFile(verdictPath, data, 0644); err != nil {
		t.Fatalf("write corrupted verdict: %v", err)
	}
	// DO NOT regenerate checksums - this is the corruption

	exitCode, stdout, stderr := runCLIVerifyMatrix(t, fixture.rootDir)

	if exitCode == 0 {
		t.Error("expected nonzero exit for stale verdict checksum")
	}
	assertNoTerminalPass(t, stdout, stderr)
	assertErrorIdentifiesArtifact(t, stdout+stderr, "verdict")
	assertErrorIdentifiesFailureClass(t, stdout+stderr, "checksum")
}

// =============================================================================
// UNKNOWN FIELD TESTS
// =============================================================================

// TestVerifyMatrixCLI_RejectsUnknownFieldInChildArtifact proves CLI rejects unknown fields in child artifacts.
func TestVerifyMatrixCLI_RejectsUnknownFieldInChildArtifact(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Add unknown field to growing child verdict
	growingRunDir := fixture.runDirs[0]
	verdictPath := filepath.Join(growingRunDir, "verdict.json")
	data, err := os.ReadFile(verdictPath)
	if err != nil {
		t.Fatalf("read verdict: %v", err)
	}

	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Add unknown field
	v["unknown_field_forbidden"] = "this should not be here"
	newData, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(verdictPath, newData, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Regenerate checksums to reach the strict decoder
	mustRegenerateAllChecksumsCLI(t, fixture)

	exitCode, stdout, stderr := runCLIVerifyMatrix(t, fixture.rootDir)

	if exitCode == 0 {
		t.Error("expected nonzero exit for unknown field in child artifact")
	}
	assertNoTerminalPass(t, stdout, stderr)
	assertErrorIdentifiesArtifact(t, stdout+stderr, "growing")
	assertErrorIdentifiesFailureClass(t, stdout+stderr, "unknown")
}

// TestVerifyMatrixCLI_RejectsUnknownFieldInRootArtifact proves CLI rejects unknown fields in root artifacts.
func TestVerifyMatrixCLI_RejectsUnknownFieldInRootArtifact(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Add unknown field to matrix manifest by directly modifying JSON text
	// DO NOT use json.Unmarshal/Marshal which would normalize the field
	manifestPath := filepath.Join(fixture.rootDir, "matrix-manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	// Find the closing brace and add the unknown field before it
	// This preserves the JSON structure and adds the forbidden field
	dataStr := string(data)
	// Insert before the final closing brace
	lastBrace := len(dataStr) - 1
	for lastBrace > 0 && dataStr[lastBrace] != '}' {
		lastBrace--
	}
	if lastBrace == 0 {
		t.Fatal("could not find closing brace in manifest")
	}

	// Add unknown field
	unknownField := `,"forbidden_unknown_root_field":"this should not be here"`
	newData := []byte(dataStr[:lastBrace] + unknownField + dataStr[lastBrace:])
	if err := os.WriteFile(manifestPath, newData, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Manually regenerate matrix checksums (NOT the JSON files)
	matrixChecksumContent, err := computeMatrixChecksumsContent(fixture.rootDir)
	if err != nil {
		t.Fatalf("compute matrix checksums: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixture.rootDir, "matrix-checksums.txt"), []byte(matrixChecksumContent), 0644); err != nil {
		t.Fatalf("write matrix checksums: %v", err)
	}

	exitCode, stdout, stderr := runCLIVerifyMatrix(t, fixture.rootDir)

	if exitCode == 0 {
		t.Error("expected nonzero exit for unknown field in root artifact")
	}
	assertNoTerminalPass(t, stdout, stderr)
	assertErrorIdentifiesArtifact(t, stdout+stderr, "manifest")
	assertErrorIdentifiesFailureClass(t, stdout+stderr, "unknown")
}

// =============================================================================
// TRAILING DOCUMENT TESTS
// =============================================================================

// TestVerifyMatrixCLI_RejectsTrailingDocumentInChildArtifact proves CLI rejects trailing documents in child artifacts.
func TestVerifyMatrixCLI_RejectsTrailingDocumentInChildArtifact(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Append trailing document to growing child verdict
	growingRunDir := fixture.runDirs[0]
	verdictPath := filepath.Join(growingRunDir, "verdict.json")
	data, err := os.ReadFile(verdictPath)
	if err != nil {
		t.Fatalf("read verdict: %v", err)
	}
	// Append a second valid JSON document
	data = append(data, []byte("\n{\"trailing\":\"document\"}")...)
	if err := os.WriteFile(verdictPath, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Regenerate checksums to reach the strict decoder
	mustRegenerateAllChecksumsCLI(t, fixture)

	exitCode, stdout, stderr := runCLIVerifyMatrix(t, fixture.rootDir)

	if exitCode == 0 {
		t.Error("expected nonzero exit for trailing document in child artifact")
	}
	assertNoTerminalPass(t, stdout, stderr)
	assertErrorIdentifiesArtifact(t, stdout+stderr, "growing")
	assertErrorIdentifiesFailureClass(t, stdout+stderr, "trailing")
}

// TestVerifyMatrixCLI_RejectsTrailingDocumentInRootArtifact proves CLI rejects trailing documents in root artifacts.
func TestVerifyMatrixCLI_RejectsTrailingDocumentInRootArtifact(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	// Append trailing document to matrix cleanup by directly modifying the file
	// DO NOT use json.Unmarshal/Marshal which would remove trailing content
	cleanupPath := filepath.Join(fixture.rootDir, "matrix-cleanup.json")
	data, err := os.ReadFile(cleanupPath)
	if err != nil {
		t.Fatalf("read cleanup: %v", err)
	}
	// Append a second valid JSON document
	data = append(data, []byte("\n{\"trailing\":\"document\"}")...)
	if err := os.WriteFile(cleanupPath, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Manually regenerate matrix checksums (NOT the JSON files)
	matrixChecksumContent, err := computeMatrixChecksumsContent(fixture.rootDir)
	if err != nil {
		t.Fatalf("compute matrix checksums: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixture.rootDir, "matrix-checksums.txt"), []byte(matrixChecksumContent), 0644); err != nil {
		t.Fatalf("write matrix checksums: %v", err)
	}

	exitCode, stdout, stderr := runCLIVerifyMatrix(t, fixture.rootDir)

	if exitCode == 0 {
		t.Error("expected nonzero exit for trailing document in root artifact")
	}
	assertNoTerminalPass(t, stdout, stderr)
	assertErrorIdentifiesArtifact(t, stdout+stderr, "cleanup")
	assertErrorIdentifiesFailureClass(t, stdout+stderr, "trailing")
}

// =============================================================================
// CLEAN CONTROL TEST
// =============================================================================

// TestVerifyMatrixCLI_AcceptsCleanFixture proves clean fixture passes through CLI.
func TestVerifyMatrixCLI_AcceptsCleanFixture(t *testing.T) {
	fixture := writeValidMatrixBundleFixture(t)

	exitCode, stdout, stderr := runCLIVerifyMatrix(t, fixture.rootDir)

	if exitCode != 0 {
		t.Errorf("expected zero exit for clean fixture, got %d\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	assertTerminalPass(t, stdout, stderr)
}

// =============================================================================
// ASSERTION HELPERS
// =============================================================================

// assertErrorIdentifiesArtifact asserts the error output identifies the affected artifact.
func assertErrorIdentifiesArtifact(t *testing.T, output string, artifact string) {
	t.Helper()
	artifactLower := strings.ToLower(artifact)
	if !strings.Contains(output, artifactLower) {
		t.Errorf("expected error to identify %q artifact, got: %s", artifactLower, output)
	}
}

// assertErrorIdentifiesFailureClass asserts the error output identifies the failure class.
func assertErrorIdentifiesFailureClass(t *testing.T, output string, failureClass string) {
	t.Helper()
	// Check for failure class keywords
	keywords := map[string][]string{
		"checksum": {"checksum", "hash", "mismatch"},
		"unknown":  {"unknown", "field", "disallow"},
		"trailing": {"trailing", "second", "document"},
	}

	found := false
	for _, kw := range keywords[failureClass] {
		if strings.Contains(strings.ToLower(output), kw) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error to identify failure class %q, got: %s", failureClass, output)
	}
}

// =============================================================================
// CHECKSUM REGENERATION HELPERS (for CLI tests)
// =============================================================================

// mustRegenerateAllChecksumsCLI regenerates all checksums or fails the test.
// This is used to reach the strict JSON decoder after file mutations.
func mustRegenerateAllChecksumsCLI(t *testing.T, fixture *matrixFixture) {
	t.Helper()
	if err := regenerateChecksumsForCLI(fixture); err != nil {
		t.Fatalf("regenerate checksums: %v", err)
	}
}

// regenerateChecksumsForCLI regenerates all checksums from fixture state.
// This updates child checksums and matrix checksums.
func regenerateChecksumsForCLI(fixture *matrixFixture) error {
	// Step 1: Regenerate child checksums for all runs
	for i, runDir := range fixture.runDirs {
		checksumContent, err := computeChildChecksumsContent(runDir)
		if err != nil {
			return fmt.Errorf("compute child checksums for %s: %w", fixtureRunIDs[i], err)
		}
		if err := os.WriteFile(filepath.Join(runDir, "checksums.txt"), []byte(checksumContent), 0644); err != nil {
			return fmt.Errorf("write child checksums for %s: %w", fixtureRunIDs[i], err)
		}

		// Update manifest's checksums_sha256 for this run
		h := sha256.Sum256([]byte(checksumContent))
		checksumHash := hex.EncodeToString(h[:])
		fixture.manifest.Runs[i].ChecksumsSHA256 = checksumHash
	}

	// Step 2: Rewrite matrix manifest
	manifestJSON, err := json.MarshalIndent(fixture.manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(fixture.rootDir, "matrix-manifest.json"), manifestJSON, 0644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	// Step 3: Rewrite matrix cleanup
	cleanupJSON, err := json.MarshalIndent(fixture.cleanup, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cleanup: %w", err)
	}
	if err := os.WriteFile(filepath.Join(fixture.rootDir, "matrix-cleanup.json"), cleanupJSON, 0644); err != nil {
		return fmt.Errorf("write cleanup: %w", err)
	}

	// Step 4: Rewrite matrix verdict
	verdictJSON, err := json.MarshalIndent(fixture.verdict, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal verdict: %w", err)
	}
	if err := os.WriteFile(filepath.Join(fixture.rootDir, "matrix-verdict.json"), verdictJSON, 0644); err != nil {
		return fmt.Errorf("write verdict: %w", err)
	}

	// Step 5: Regenerate matrix checksums
	checksumContent, err := computeMatrixChecksumsContent(fixture.rootDir)
	if err != nil {
		return fmt.Errorf("compute matrix checksums: %w", err)
	}
	if err := os.WriteFile(filepath.Join(fixture.rootDir, "matrix-checksums.txt"), []byte(checksumContent), 0644); err != nil {
		return fmt.Errorf("write matrix checksums: %w", err)
	}

	return nil
}
