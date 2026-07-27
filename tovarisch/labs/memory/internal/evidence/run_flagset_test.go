// run_flagset_test.go — Tests for NewRunFlagSet.
//
// P0-7: Required tests for the production FlagSet constructor.
//
// Reference: ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION51-CORRECTION03
package evidence

import (
	"bytes"
	"flag"
	"os"
	"strings"
	"testing"
)

// TestRunFlagSet_NilOutputHandled verifies nil output is handled gracefully.
func TestRunFlagSet_NilOutputHandled(t *testing.T) {
	fs, v := NewRunFlagSet(nil)

	if fs == nil {
		t.Fatal("FlagSet is nil")
	}
	if v == nil {
		t.Fatal("RunFlagValues is nil")
	}

	// Should be able to parse without panic
	err := fs.Parse([]string{"--scenario=canary-growing", "--artifacts-dir=/tmp/test"})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if v.Scenario != "canary-growing" {
		t.Errorf("Scenario: got %q", v.Scenario)
	}
}

// TestRunFlagSet_RequiredFlagsPresent verifies all required flags exist.
func TestRunFlagSet_RequiredFlagsPresent(t *testing.T) {
	fs, v := NewRunFlagSet(os.Stderr)

	// Verify all flag pointers are non-nil
	if fs == nil {
		t.Fatal("FlagSet is nil")
	}
	if v == nil {
		t.Fatal("RunFlagValues is nil")
	}

	// Parse empty args - flag.Parse won't fail, but ValidateRunFlags will
	err := fs.Parse([]string{})
	if err != nil {
		t.Fatalf("flag.Parse should not fail: %v", err)
	}

	// Validation should catch missing required flags
	if err := ValidateRunFlags(v); err == nil {
		t.Fatal("expected validation error for missing required flags")
	}

	// Verify flag names exist
	flagNames := []string{
		"scenario",
		"duration",
		"artifacts-dir",
		"v",
		"container-image",
		"canary-port",
		"canary-build-metadata",
		"repository-root",
	}

	for _, name := range flagNames {
		f := fs.Lookup(name)
		if f == nil {
			t.Errorf("flag --%s not found", name)
		}
	}
}

// TestRunFlagSet_NoEvidenceBypassFlags verifies no bypass flags exist.
func TestRunFlagSet_NoEvidenceBypassFlags(t *testing.T) {
	fs, _ := NewRunFlagSet(os.Stderr)

	// Forbidden bypass flags that must NOT exist
	forbiddenFlags := []string{
		"verify",
		"no-verify",
		"capture-provenance",
		"skip-evidence",
		"evidence-enabled",
		"disable-evidence",
		"no-evidence",
		"skip-qualified",
	}

	for _, name := range forbiddenFlags {
		f := fs.Lookup(name)
		if f != nil {
			t.Errorf("forbidden bypass flag --%s must not exist", name)
		}
	}
}

// TestRunFlagSet_StableDefaults verifies stable default values.
func TestRunFlagSet_StableDefaults(t *testing.T) {
	fs, v := NewRunFlagSet(os.Stderr)

	// Parse with no args - flag.Parse won't fail (validation is separate)
	_ = fs.Parse([]string{})

	// Verify stable defaults
	if v.Duration != 60 {
		t.Errorf("Duration default: got %d, want 60", v.Duration)
	}
	if v.ContainerImage != "kgb-tovarisch-canary:latest" {
		t.Errorf("ContainerImage default: got %q, want %q", v.ContainerImage, "kgb-tovarisch-canary:latest")
	}
	if v.CanaryPort != 8080 {
		t.Errorf("CanaryPort default: got %d, want 8080", v.CanaryPort)
	}
	if v.Verbose != false {
		t.Errorf("Verbose default: got %v, want false", v.Verbose)
	}
}

// TestRunFlagSet_NoDuplicateFlags verifies no duplicate flag definitions.
func TestRunFlagSet_NoDuplicateFlags(t *testing.T) {
	fs, _ := NewRunFlagSet(os.Stderr)

	// Collect all flag names
	names := make(map[string]int)
	fs.VisitAll(func(f *flag.Flag) {
		names[f.Name]++
	})

	// Check for duplicates
	for name, count := range names {
		if count > 1 {
			t.Errorf("duplicate flag: --%s appears %d times", name, count)
		}
	}
}

// TestRunFlagSet_ValidScenarioAccepted verifies valid scenarios are accepted.
func TestRunFlagSet_ValidScenarioAccepted(t *testing.T) {
	for _, scenario := range []string{"canary-growing", "canary-bounded", "canary-descriptor"} {
		fs, v := NewRunFlagSet(os.Stderr)
		err := fs.Parse([]string{
			"--scenario=" + scenario,
			"--artifacts-dir=/tmp/test",
			"--duration=30",
		})
		if err != nil {
			t.Errorf("scenario %q: unexpected error: %v", scenario, err)
		}
		if v.Scenario != scenario {
			t.Errorf("scenario %q: got %q", scenario, v.Scenario)
		}
	}
}

// TestRunFlagSet_InvalidScenarioRejected verifies invalid scenarios are rejected.
func TestRunFlagSet_InvalidScenarioRejected(t *testing.T) {
	fs, v := NewRunFlagSet(os.Stderr)
	err := fs.Parse([]string{
		"--scenario=invalid-scenario",
		"--artifacts-dir=/tmp/test",
	})
	if err != nil {
		t.Fatalf("flag.Parse should not fail for invalid scenario: %v", err)
	}
	// Validation should catch it
	if err := ValidateRunFlags(v); err == nil {
		t.Error("expected validation error for invalid scenario")
	}
}

// TestRunFlagSet_MissingArtifactsDirRejected verifies missing artifacts-dir is rejected.
func TestRunFlagSet_MissingArtifactsDirRejected(t *testing.T) {
	fs, v := NewRunFlagSet(os.Stderr)
	err := fs.Parse([]string{
		"--scenario=canary-growing",
	})
	if err != nil {
		t.Fatalf("flag.Parse should not fail for missing artifacts-dir: %v", err)
	}
	// Validation should catch it
	if err := ValidateRunFlags(v); err == nil {
		t.Error("expected validation error for missing artifacts-dir")
	}
}

// TestRunFlagSet_DurationMinimumEnforced verifies duration minimum is enforced.
func TestRunFlagSet_DurationMinimumEnforced(t *testing.T) {
	fs, v := NewRunFlagSet(os.Stderr)
	err := fs.Parse([]string{
		"--scenario=canary-growing",
		"--artifacts-dir=/tmp/test",
		"--duration=5", // below minimum
	})
	if err != nil {
		t.Fatal("parse should not fail for duration below minimum (validation is separate)")
	}

	// Validation should catch it
	if err := ValidateRunFlags(v); err == nil {
		t.Error("expected validation error for duration < 10")
	}
}

// TestRunFlagSet_AllFlagsParsed verifies all flags can be parsed.
func TestRunFlagSet_AllFlagsParsed(t *testing.T) {
	fs, v := NewRunFlagSet(os.Stderr)
	err := fs.Parse([]string{
		"--scenario=canary-bounded",
		"--artifacts-dir=/tmp/artifacts",
		"--duration=120",
		"--v",
		"--container-image=test:tag",
		"--canary-port=9090",
		"--canary-build-metadata=/path/to/metadata.json",
		"--repository-root=/repo",
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if v.Scenario != "canary-bounded" {
		t.Errorf("Scenario: got %q", v.Scenario)
	}
	if v.ArtifactsDir != "/tmp/artifacts" {
		t.Errorf("ArtifactsDir: got %q", v.ArtifactsDir)
	}
	if v.Duration != 120 {
		t.Errorf("Duration: got %d", v.Duration)
	}
	if !v.Verbose {
		t.Error("Verbose: got false, want true")
	}
	if v.ContainerImage != "test:tag" {
		t.Errorf("ContainerImage: got %q", v.ContainerImage)
	}
	if v.CanaryPort != 9090 {
		t.Errorf("CanaryPort: got %d", v.CanaryPort)
	}
	if v.CanaryBuildMetadata != "/path/to/metadata.json" {
		t.Errorf("CanaryBuildMetadata: got %q", v.CanaryBuildMetadata)
	}
	if v.RepositoryRoot != "/repo" {
		t.Errorf("RepositoryRoot: got %q", v.RepositoryRoot)
	}
}

// TestRunFlagSet_UsageOutput verifies usage is set correctly.
func TestRunFlagSet_UsageOutput(t *testing.T) {
	var buf bytes.Buffer
	fs, _ := NewRunFlagSet(&buf)

	// Usage should be callable
	fs.Usage()

	output := buf.String()
	if output == "" {
		t.Fatal("usage output is empty")
	}

	// Exact first-line assertion: "Usage: memory-lab run [options]"
	firstLine := strings.SplitN(output, "\n", 2)[0]
	expected := "Usage: memory-lab run [options]"
	if firstLine != expected {
		t.Errorf("usage first line: got %q, want %q", firstLine, expected)
	}

	// Verify "Options:" header is present
	if !bytes.Contains(buf.Bytes(), []byte("Options:")) {
		t.Error("usage output missing 'Options:'")
	}
}
