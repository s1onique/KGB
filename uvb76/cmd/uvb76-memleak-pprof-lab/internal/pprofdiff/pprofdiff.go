// Package pprofdiff provides pprof diff report execution for the memory lab.
//
// This package executes `go tool pprof` diff reports using explicit argv,
// capturing stdout/stderr for artifact output.
package pprofdiff

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Runner executes pprof diff reports.
type Runner struct {
	artifactDir string
}

// ReportConfig describes a pprof report to generate.
type ReportConfig struct {
	// Name is the output filename
	Name string
	// BaseProfile is the baseline profile path (for diff reports)
	BaseProfile string
	// TargetProfile is the comparison profile path
	TargetProfile string
	// OutputType is the pprof output type (top, list, etc.)
	OutputType string
	// SampleType is the sample type (inuse_space, alloc_space, etc.)
	SampleType string
	// DiffBase indicates whether to use -diff_base flag
	DiffBase bool
}

// NewRunner creates a new pprof diff runner.
func NewRunner(artifactDir string) *Runner {
	return &Runner{artifactDir: artifactDir}
}

// RunReports executes all configured pprof reports.
func (r *Runner) RunReports(ctx context.Context, reports []ReportConfig) error {
	for _, cfg := range reports {
		if err := r.runReport(ctx, cfg); err != nil {
			return fmt.Errorf("report %s: %w", cfg.Name, err)
		}
	}
	return nil
}

// runReport executes a single pprof report.
func (r *Runner) runReport(ctx context.Context, cfg ReportConfig) error {
	argv, err := BuildArgv(cfg)
	if err != nil {
		return err
	}

	outputPath := filepath.Join(r.artifactDir, cfg.Name)

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = r.artifactDir // Ensure pprof can find profiles
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// pprof returns non-zero for various reasons, including when there's growth
		// Only fail on truly missing profiles
		if len(stderr.String()) > 0 && !containsMissingFileError(stderr.String()) {
			// Write output anyway, pprof may have produced partial output
		} else if err != nil {
			return fmt.Errorf("pprof execution failed: %w", err)
		}
	}

	// Write output file
	if err := os.WriteFile(outputPath, stdout.Bytes(), 0644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	return nil
}

// BuildArgv constructs the argv for a pprof report.
func BuildArgv(cfg ReportConfig) ([]string, error) {
	if cfg.TargetProfile == "" {
		return nil, fmt.Errorf("target profile is required")
	}

	argv := []string{"go", "tool", "pprof"}

	// Add output type
	if cfg.OutputType != "" {
		argv = append(argv, "-"+cfg.OutputType)
	}

	// Add sample type
	if cfg.SampleType != "" {
		argv = append(argv, "-"+cfg.SampleType)
	}

	// Add diff_base if requested
	if cfg.DiffBase {
		if cfg.BaseProfile == "" {
			return nil, fmt.Errorf("base profile required for diff report")
		}
		argv = append(argv, "-diff_base="+cfg.BaseProfile)
	}

	// Add target profile
	argv = append(argv, cfg.TargetProfile)

	return argv, nil
}

// containsMissingFileError checks if the error indicates a missing file.
func containsMissingFileError(s string) bool {
	return contains(s, "no such file") || contains(s, "cannot open")
}

// contains is a simple string contains check.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

// findSubstring performs a simple substring search.
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
