// Package pprof provides pprof-related utilities for the memory lab.
package pprof

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

// DiffConfig holds configuration for pprof diff generation.
type DiffConfig struct {
	// BaseProfile is the baseline profile path
	BaseProfile string
	// TargetProfile is the comparison profile path
	TargetProfile string
	// OutputType specifies the output type (top, list, svg, etc.)
	OutputType string
	// SampleType specifies what to measure (inuse_space, inuse_objects, etc.)
	SampleType string
}

// DiffProfileType defines the type of profile to diff.
type DiffProfileType string

const (
	DiffProfileHeap   DiffProfileType = "heap"
	DiffProfileAllocs DiffProfileType = "allocs"
)

// BuildDiffArgv constructs the argv for `go tool pprof -diff_base`.
// This is a pure function that returns argv, not a shell string.
func BuildDiffArgv(baseProfile, targetProfile string, profileType DiffProfileType, outputType, sampleType string) []string {
	// Normalize paths
	base := baseProfile
	target := targetProfile

	argv := []string{"go", "tool", "pprof"}

	// Add output type if specified
	if outputType != "" {
		argv = append(argv, "-"+outputType)
	}

	// Add sample type
	if sampleType != "" {
		argv = append(argv, "-"+sampleType)
	}

	// Add diff_base flag
	argv = append(argv, "-diff_base="+base)

	// Add target profile
	argv = append(argv, target)

	return argv
}

// BuildTopDiffArgv returns argv for a top-style diff comparison.
func BuildTopDiffArgv(baseProfile, targetProfile string, profileType DiffProfileType) []string {
	return BuildDiffArgv(baseProfile, targetProfile, profileType, "top", "inuse_space")
}

// BuildTopInuseObjectsDiffArgv returns argv for a top-style diff with inuse_objects.
func BuildTopInuseObjectsDiffArgv(baseProfile, targetProfile string, profileType DiffProfileType) []string {
	return BuildDiffArgv(baseProfile, targetProfile, profileType, "top", "inuse_objects")
}

// BuildHeapDiffArgv returns argv for a heap profile diff.
func BuildHeapDiffArgv(baseProfile, targetProfile string) []string {
	return BuildTopDiffArgv(baseProfile, targetProfile, DiffProfileHeap)
}

// BuildHeapInuseObjectsDiffArgv returns argv for a heap profile diff with inuse_objects.
func BuildHeapInuseObjectsDiffArgv(baseProfile, targetProfile string) []string {
	return BuildTopInuseObjectsDiffArgv(baseProfile, targetProfile, DiffProfileHeap)
}

// BuildAllocsDiffArgv returns argv for an allocs profile diff.
func BuildAllocsDiffArgv(baseProfile, targetProfile string) []string {
	return BuildTopDiffArgv(baseProfile, targetProfile, DiffProfileAllocs)
}

// RunDiff executes `go tool pprof -diff_base` with the given argv.
// This requires `go` to be in PATH and is not executed in this ACT.
func RunDiff(argv []string) (*exec.Cmd, error) {
	if len(argv) < 4 {
		return nil, fmt.Errorf("argv too short: %v", argv)
	}
	if argv[0] != "go" || argv[1] != "tool" || argv[2] != "pprof" {
		return nil, fmt.Errorf("invalid pprof argv: %v", argv)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	return cmd, nil
}

// VerifyProfilePaths checks that the profile paths exist.
func VerifyProfilePaths(baseProfile, targetProfile string) error {
	if baseProfile == "" {
		return fmt.Errorf("base profile path is empty")
	}
	if targetProfile == "" {
		return fmt.Errorf("target profile path is empty")
	}

	// Check paths are relative and safe
	baseName := filepath.Base(baseProfile)
	targetName := filepath.Base(targetProfile)

	if baseName != baseProfile || targetName != targetProfile {
		return fmt.Errorf("profile paths must be simple filenames, not paths")
	}

	return nil
}
