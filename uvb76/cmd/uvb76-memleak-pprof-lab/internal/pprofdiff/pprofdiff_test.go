// Package pprofdiff provides tests for pprof diff execution.
package pprofdiff

import (
	"testing"
)

func TestBuildArgv_DiffBase(t *testing.T) {
	cfg := ReportConfig{
		Name:          "test-diff.txt",
		BaseProfile:   "heap-t000.pb.gz",
		TargetProfile: "heap-t600.pb.gz",
		OutputType:    "top",
		SampleType:    "inuse_space",
		DiffBase:      true,
	}

	argv, err := BuildArgv(cfg)
	if err != nil {
		t.Fatalf("BuildArgv failed: %v", err)
	}

	// Verify structure
	if len(argv) < 5 {
		t.Fatalf("argv too short: %v", argv)
	}
	if argv[0] != "go" {
		t.Errorf("Expected argv[0]=go, got %s", argv[0])
	}
	if argv[1] != "tool" {
		t.Errorf("Expected argv[1]=tool, got %s", argv[1])
	}
	if argv[2] != "pprof" {
		t.Errorf("Expected argv[2]=pprof, got %s", argv[2])
	}
	if argv[3] != "-top" {
		t.Errorf("Expected argv[3]=-top, got %s", argv[3])
	}
	if argv[4] != "-inuse_space" {
		t.Errorf("Expected argv[4]=-inuse_space, got %s", argv[4])
	}
	if argv[5] != "-diff_base=heap-t000.pb.gz" {
		t.Errorf("Expected argv[5]=-diff_base=..., got %s", argv[5])
	}
	if argv[6] != "heap-t600.pb.gz" {
		t.Errorf("Expected argv[6]=heap-t600.pb.gz, got %s", argv[6])
	}
}

func TestBuildArgv_NoDiffBase(t *testing.T) {
	cfg := ReportConfig{
		Name:          "test-final.txt",
		BaseProfile:   "",
		TargetProfile: "allocs-t600.pb.gz",
		OutputType:    "top",
		SampleType:    "alloc_space",
		DiffBase:      false,
	}

	argv, err := BuildArgv(cfg)
	if err != nil {
		t.Fatalf("BuildArgv failed: %v", err)
	}

	// Should not include -diff_base
	foundDiffBase := false
	for _, arg := range argv {
		if arg == "-diff_base=heap-t000.pb.gz" {
			foundDiffBase = true
		}
	}
	if foundDiffBase {
		t.Error("Should not include -diff_base for non-diff report")
	}
}

func TestBuildArgv_MissingTarget(t *testing.T) {
	cfg := ReportConfig{
		Name:          "test.txt",
		BaseProfile:   "heap-t000.pb.gz",
		TargetProfile: "",
		OutputType:    "top",
		SampleType:    "inuse_space",
		DiffBase:      true,
	}

	_, err := BuildArgv(cfg)
	if err == nil {
		t.Error("Expected error for missing target profile")
	}
}

func TestBuildArgv_MissingBaseForDiff(t *testing.T) {
	cfg := ReportConfig{
		Name:          "test.txt",
		BaseProfile:   "",
		TargetProfile: "heap-t600.pb.gz",
		OutputType:    "top",
		SampleType:    "inuse_space",
		DiffBase:      true,
	}

	_, err := BuildArgv(cfg)
	if err == nil {
		t.Error("Expected error for missing base profile in diff report")
	}
}

func TestContainsMissingFileError(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"no such file or directory", true},
		{"cannot open file", true},
		{"something else", false},
		{"", false},
	}

	for _, tc := range tests {
		got := containsMissingFileError(tc.input)
		if got != tc.expected {
			t.Errorf("containsMissingFileError(%q) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}
