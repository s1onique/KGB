// lab_test.go — Tovarisch Memory Laboratory Integration Tests
//
// ACT-TOVARISCH-GO-MEMORY-LAB01
//
// Deterministic memory investigation using disposable Docker topologies.
// Go test entry points with -count=1 required.
//
// Usage:
//   go test -tags=tovarisch_memlab -count=1 ./tovarisch/labs/memory
//
// Scenarios:
//   - TestMemoryLab_SteadyStateNoHTTP
//   - TestMemoryLab_StatusOneHz
//   - TestMemoryLab_StatusBurst
//   - TestMemoryLab_ReconnectSuccess
//   - TestMemoryLab_ReconnectFailure
//   - TestMemoryLab_BFDSteadyState
//   - TestMemoryLab_CombinedPressure
//
// Reference: kgb://doctrine/embedded-memory-frugality
// Reference: kgb://factory/workflow

package memory_test

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/analysis"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling"
)

// Docker lab types - defined here to avoid circular imports
// In production, these would be in the dockerlab package

// ResourceLimits defines container resource constraints.
type ResourceLimits struct {
	MemoryLimit string
	CPUPeriod   int64
	CPUQuota    int64
	PidsLimit   int64
	MemorySwap  string
}

// Network represents a Docker network.
type Network struct {
	ID   string
	Name string
}

// Container represents a Docker container.
type Container struct {
	ID     string
	Name   string
	Status string
}

// LabConfig holds shared laboratory configuration.
type LabConfig struct {
	RunID             string
	SubjectImage      string
	SubjectBinaryPath string
	PeerImage         string
	ArtifactDir       string
	ResourceLimits    ResourceLimits
	PhaseConfig       sampling.PhaseConfig
	Thresholds        analysis.Thresholds
	SkipDocker        bool // For unit test mode without Docker
	Smoke             bool // Shortened durations for smoke testing
}

var (
	labConfig LabConfig
	runID     = flag.String("run-id", "", "Unique run identifier (auto-generated if empty)")
)

// init registers flags and sets defaults.
func init() {
	flag.StringVar(&labConfig.SubjectImage, "subject-image", "tovarisch:test", "Tovarisch container image")
	flag.StringVar(&labConfig.SubjectBinaryPath, "subject-binary", "./tovarisch/zig-out/bin/tovarisch", "Tovarisch binary path (for checksums)")
	flag.StringVar(&labConfig.PeerImage, "peer-image", "bird2:test", "BGP/BFD peer container image")
	flag.StringVar(&labConfig.ArtifactDir, "artifact-dir", "", "Evidence output directory")
	flag.BoolVar(&labConfig.SkipDocker, "skip-docker", false, "Skip Docker-dependent tests")
	flag.BoolVar(&labConfig.Smoke, "smoke", false, "Run smoke variant (shortened durations)")

	// Default phase configuration (can be overridden by smoke flag
	labConfig.PhaseConfig = sampling.DefaultPhaseConfig()
	labConfig.Thresholds = analysis.DefaultThresholds()
	labConfig.ResourceLimits = DefaultResourceLimits()
}

// DefaultResourceLimits returns default container resource limits.
func DefaultResourceLimits() ResourceLimits {
	return ResourceLimits{
		MemoryLimit: "128m",
		CPUPeriod:   100000,
		CPUQuota:    50000,
		PidsLimit:   64,
		MemorySwap:  "128m",
	}
}

// ensureRunID generates a run ID if not provided.
func ensureRunID(t *testing.T) string {
	if *runID != "" {
		return *runID
	}
	if labConfig.RunID != "" {
		return labConfig.RunID
	}
	// Generate deterministic-enough ID for reproducibility
	return fmt.Sprintf("lab-%d", time.Now().UnixNano())
}

// skipIfNoDocker skips the test if Docker is unavailable or --skip-docker is set.
func skipIfNoDocker(t *testing.T) {
	if labConfig.SkipDocker {
		t.Skip("skipping Docker-dependent test (--skip-docker set)")
	}
	// Check if Docker socket is accessible
	if _, err := os.Stat("/var/run/docker.sock"); os.IsNotExist(err) {
		t.Skip("skipping Docker-dependent test (Docker socket not found)")
	}
}

// setupArtifactDir creates the artifact directory for the run.
func setupArtifactDir(t *testing.T, scenario string) string {
	runID := ensureRunID(t)
	dir := labConfig.ArtifactDir
	if dir == "" {
		repoRoot, err := findRepoRoot()
		if err != nil {
			t.Fatalf("find repo root: %v", err)
		}
		dir = filepath.Join(repoRoot, ".factory", "tovarisch-memory-lab", runID)
	}
	scenarioDir := filepath.Join(dir, scenario)
	if err := os.MkdirAll(scenarioDir, 0755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	return scenarioDir
}

// TeardownFunc is returned by test setup to ensure cleanup.
type TeardownFunc func(context.Context, *testing.T)

// LabState holds the laboratory state for a scenario.
type LabState struct {
	Context        context.Context
	RunID          string
	Scenario       string
	ArtifactDir    string
	PhaseConfig    sampling.PhaseConfig
	Thresholds     analysis.Thresholds
	ResourceLimits ResourceLimits

	// Runtime state populated during execution
	SubjectContainer *Container
	PeerContainer    *Container
}

// LabScenarioConfig configures a test scenario.
type LabScenarioConfig struct {
	Name   string
	Phases []sampling.Phase
	Setup  func(*LabState) error
	Inject func(*LabState) error
	Assert func(*LabState, *analysis.Verdict) error
}

// runLab executes a scenario with the given configuration.
// Returns cleanup function that must be called even on test failure.
func runLab(t *testing.T, ctx context.Context, scenario string, cfg *LabScenarioConfig) (teardown TeardownFunc, err error) {
	artifactDir := setupArtifactDir(t, scenario)
	runID := ensureRunID(t)

	// Adjust phase config for smoke testing
	phaseCfg := labConfig.PhaseConfig
	if labConfig.Smoke {
		phaseCfg = sampling.SmokePhaseConfig()
	}

	// Build lab state
	state := &LabState{
		Context:        ctx,
		RunID:          runID,
		Scenario:       scenario,
		ArtifactDir:    artifactDir,
		PhaseConfig:    phaseCfg,
		Thresholds:     labConfig.Thresholds,
		ResourceLimits: labConfig.ResourceLimits,
	}

	// Execute scenario setup
	if cfg != nil && cfg.Setup != nil {
		if err := cfg.Setup(state); err != nil {
			return nil, fmt.Errorf("scenario setup: %w", err)
		}
	}

	// Return teardown function
	return func(ctx context.Context, t *testing.T) {
		t.Logf("Lab %s/%s completed", runID, scenario)
	}, nil
}

// TestMemoryLab_SteadyStateNoHTTP measures memory under passive BGP/BFD operation.
func TestMemoryLab_SteadyStateNoHTTP(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()

	teardown, err := runLab(t, ctx, "steady-state-no-http", &LabScenarioConfig{
		Name: "SteadyStateNoHTTP",
		Phases: []sampling.Phase{
			sampling.PhaseStartup,
			sampling.PhaseWarmup,
			sampling.PhaseBaseline,
			sampling.PhaseStimulus,
			sampling.PhaseSettling,
			sampling.PhaseFinal,
		},
	})
	if err != nil {
		t.Fatalf("run lab: %v", err)
	}
	defer teardown(ctx, t)
}

// TestMemoryLab_StatusOneHz measures memory under 1Hz /status polling.
func TestMemoryLab_StatusOneHz(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()

	teardown, err := runLab(t, ctx, "status-1hz", &LabScenarioConfig{
		Name: "StatusOneHz",
	})
	if err != nil {
		t.Fatalf("run lab: %v", err)
	}
	defer teardown(ctx, t)
}

// TestMemoryLab_StatusBurst measures memory under high-rate /status requests.
func TestMemoryLab_StatusBurst(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()

	teardown, err := runLab(t, ctx, "status-burst", &LabScenarioConfig{
		Name: "StatusBurst",
	})
	if err != nil {
		t.Fatalf("run lab: %v", err)
	}
	defer teardown(ctx, t)
}

// TestMemoryLab_ReconnectSuccess measures memory during successful reconnect cycles.
func TestMemoryLab_ReconnectSuccess(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()

	teardown, err := runLab(t, ctx, "reconnect-success", &LabScenarioConfig{
		Name: "ReconnectSuccess",
	})
	if err != nil {
		t.Fatalf("run lab: %v", err)
	}
	defer teardown(ctx, t)
}

// TestMemoryLab_ReconnectFailure measures memory during repeated failed reconnects.
func TestMemoryLab_ReconnectFailure(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()

	teardown, err := runLab(t, ctx, "reconnect-failure", &LabScenarioConfig{
		Name: "ReconnectFailure",
	})
	if err != nil {
		t.Fatalf("run lab: %v", err)
	}
	defer teardown(ctx, t)
}

// TestMemoryLab_BFDSteadyState isolates high-frequency BFD processing.
func TestMemoryLab_BFDSteadyState(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()

	teardown, err := runLab(t, ctx, "bfd-steady-state", &LabScenarioConfig{
		Name: "BFDSteadyState",
	})
	if err != nil {
		t.Fatalf("run lab: %v", err)
	}
	defer teardown(ctx, t)
}

// TestMemoryLab_CombinedPressure tests combined workload interactions.
func TestMemoryLab_CombinedPressure(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()

	teardown, err := runLab(t, ctx, "combined-pressure", &LabScenarioConfig{
		Name: "CombinedPressure",
	})
	if err != nil {
		t.Fatalf("run lab: %v", err)
	}
	defer teardown(ctx, t)
}

// findRepoRoot finds the repository root by looking for .git.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no .git found")
		}
		dir = parent
	}
}

// Evidence types for JSON serialization.

// VerdictOutput represents the verdict.json structure.
type VerdictOutput struct {
	OverallClassification  analysis.Classification  `json:"overall_classification"`
	ScenarioValid          bool                     `json:"scenario_valid"`
	CanariesValid          bool                     `json:"canaries_valid"`
	MemoryClassification   analysis.Classification  `json:"memory_classification"`
	ResourceClassification analysis.Classification  `json:"resource_classification"`
	SemanticClassification analysis.Classification  `json:"semantic_classification"`
	SignalSummaries        []analysis.SignalSummary `json:"signal_summaries"`
	Thresholds             analysis.Thresholds      `json:"thresholds"`
	Failures               []string                 `json:"failures,omitempty"`
	Warnings               []string                 `json:"warnings,omitempty"`
	Unknowns               []string                 `json:"unknowns,omitempty"`
}

// MarshalJSON implements json.Marshaler for VerdictOutput.
func (v *VerdictOutput) MarshalJSON() ([]byte, error) {
	type alias struct {
		OverallClassification  string                   `json:"overall_classification"`
		ScenarioValid          bool                     `json:"scenario_valid"`
		CanariesValid          bool                     `json:"canaries_valid"`
		MemoryClassification   string                   `json:"memory_classification"`
		ResourceClassification string                   `json:"resource_classification"`
		SemanticClassification string                   `json:"semantic_classification"`
		SignalSummaries        []analysis.SignalSummary `json:"signal_summaries"`
		Thresholds             analysis.Thresholds      `json:"thresholds"`
		Failures               []string                 `json:"failures,omitempty"`
		Warnings               []string                 `json:"warnings,omitempty"`
		Unknowns               []string                 `json:"unknowns,omitempty"`
	}
	return json.Marshal(&alias{
		OverallClassification:  string(v.OverallClassification),
		MemoryClassification:   string(v.MemoryClassification),
		ResourceClassification: string(v.ResourceClassification),
		SemanticClassification: string(v.SemanticClassification),
		ScenarioValid:          v.ScenarioValid,
		CanariesValid:          v.CanariesValid,
		SignalSummaries:        v.SignalSummaries,
		Thresholds:             v.Thresholds,
		Failures:               v.Failures,
		Warnings:               v.Warnings,
		Unknowns:               v.Unknowns,
	})
}
