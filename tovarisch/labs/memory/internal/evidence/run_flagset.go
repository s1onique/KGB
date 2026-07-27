// run_flagset.go — Production FlagSet constructor and tests.
//
// P0-7: Extract real production FlagSet as a testable constructor.
// runCommand and tests must consume this exact constructor.
//
// Reference: ACT-TOVARISCH-GO-MEMORY-LAB01-CANARY-MATRIX-QUALIFICATION01-CORRECTION51-CORRECTION03
package evidence

import (
	"flag"
	"fmt"
	"io"
)

// RunFlagValues contains the parsed flag values for a production run.
type RunFlagValues struct {
	Scenario            string
	Duration            int
	ArtifactsDir        string
	Verbose             bool
	ContainerImage      string
	CanaryPort          int
	CanaryBuildMetadata string
	RepositoryRoot      string
}

// NewRunFlagSet creates a FlagSet with the production flags and returns
// the FlagSet and a pointer to the parsed values. The caller must call
// fs.Parse(args) before accessing the values.
//
// Nil output is handled gracefully: flags that fail will write to os.Stderr
// instead if output is nil.
//
// Forbidden flags (evidence bypass):
//   - verify, no-verify
//   - capture-provenance
//   - skip-evidence, evidence-enabled, disable-evidence, no-evidence
//   - skip-qualified
//
// Required flags:
//   - scenario: one of canary-growing, canary-bounded, canary-descriptor
//   - artifacts-dir: directory for output artifacts
func NewRunFlagSet(output io.Writer) (*flag.FlagSet, *RunFlagValues) {
	// Handle nil output gracefully
	if output == nil {
		output = io.Discard
	}

	fs := flag.NewFlagSet("memory-lab run", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.Usage = func() {
		fmt.Fprintf(output, "Usage: %s run [options]\n\nOptions:\n", fs.Name())
		fs.PrintDefaults()
	}

	v := &RunFlagValues{}

	fs.StringVar(&v.Scenario, "scenario", "", "Scenario (required): canary-growing, canary-bounded, canary-descriptor")
	fs.IntVar(&v.Duration, "duration", 60, "Duration in seconds")
	fs.StringVar(&v.ArtifactsDir, "artifacts-dir", "", "Artifacts directory (required)")
	fs.BoolVar(&v.Verbose, "v", false, "Verbose output")
	fs.StringVar(&v.ContainerImage, "container-image", "kgb-tovarisch-canary:latest", "Container image")
	fs.IntVar(&v.CanaryPort, "canary-port", 8080, "Canary HTTP port")
	fs.StringVar(&v.CanaryBuildMetadata, "canary-build-metadata", "", "Absolute or relative path to canary image build metadata JSON")
	fs.StringVar(&v.RepositoryRoot, "repository-root", "", "Repository root for the compatibility metadata fallback")

	return fs, v
}

// isAllowedScenario returns true if the scenario is valid.
func isAllowedScenario(scenario string) bool {
	switch scenario {
	case "canary-growing", "canary-bounded", "canary-descriptor":
		return true
	default:
		return false
	}
}

// ValidateRunFlags validates the parsed flag values.
func ValidateRunFlags(v *RunFlagValues) error {
	if v == nil {
		return fmt.Errorf("RunFlagValues is nil")
	}
	if v.Scenario == "" {
		return fmt.Errorf("--scenario is required")
	}
	if !isAllowedScenario(v.Scenario) {
		return fmt.Errorf("invalid scenario %q: allowed: canary-growing, canary-bounded, canary-descriptor", v.Scenario)
	}
	if v.ArtifactsDir == "" {
		return fmt.Errorf("--artifacts-dir is required")
	}
	if v.Duration < 10 {
		return fmt.Errorf("duration must be >= 10 seconds")
	}
	return nil
}
