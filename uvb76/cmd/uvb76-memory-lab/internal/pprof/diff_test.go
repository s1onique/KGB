package pprof

import (
	"testing"
)

func TestBuildDiffArgv(t *testing.T) {
	argv := BuildDiffArgv("heap-t000.pb.gz", "heap-t060.pb.gz", DiffProfileHeap, "top", "inuse_space")

	// Check expected structure
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

	// Check for -top
	foundTop := false
	for _, arg := range argv {
		if arg == "-top" {
			foundTop = true
			break
		}
	}
	if !foundTop {
		t.Errorf("Expected -top in argv: %v", argv)
	}

	// Check for -inuse_space
	foundSample := false
	for _, arg := range argv {
		if arg == "-inuse_space" {
			foundSample = true
			break
		}
	}
	if !foundSample {
		t.Errorf("Expected -inuse_space in argv: %v", argv)
	}

	// Check for -diff_base
	foundDiffBase := false
	for _, arg := range argv {
		if arg == "-diff_base=heap-t000.pb.gz" {
			foundDiffBase = true
			break
		}
	}
	if !foundDiffBase {
		t.Errorf("Expected -diff_base=heap-t000.pb.gz in argv: %v", argv)
	}

	// Check last arg is target profile
	lastIdx := len(argv) - 1
	if argv[lastIdx] != "heap-t060.pb.gz" {
		t.Errorf("Expected last argv to be target profile, got %s", argv[lastIdx])
	}
}

func TestBuildHeapDiffArgv(t *testing.T) {
	argv := BuildHeapDiffArgv("heap-t000.pb.gz", "heap-t060.pb.gz")

	// Verify structure
	if argv[0] != "go" || argv[1] != "tool" || argv[2] != "pprof" {
		t.Errorf("Unexpected argv structure: %v", argv)
	}

	// Should contain the profile paths
	foundTarget := false
	for _, arg := range argv {
		// Base profile is in -diff_base flag, not as separate arg
		if arg == "-diff_base=heap-t000.pb.gz" {
			continue // base profile found in diff_base
		}
		if arg == "heap-t060.pb.gz" {
			foundTarget = true
		}
	}
	if !foundTarget {
		t.Errorf("Missing target profile in argv: %v", argv)
	}
}

func TestBuildAllocsDiffArgv(t *testing.T) {
	argv := BuildAllocsDiffArgv("allocs-t000.pb.gz", "allocs-t060.pb.gz")

	if argv[0] != "go" {
		t.Errorf("Expected go, got %s", argv[0])
	}
}

func TestBuildTopInuseObjectsDiffArgv(t *testing.T) {
	argv := BuildTopInuseObjectsDiffArgv("heap-t000.pb.gz", "heap-t060.pb.gz", DiffProfileHeap)

	foundObjects := false
	for _, arg := range argv {
		if arg == "-inuse_objects" {
			foundObjects = true
			break
		}
	}
	if !foundObjects {
		t.Errorf("Expected -inuse_objects in argv: %v", argv)
	}
}

func TestVerifyProfilePaths(t *testing.T) {
	// Valid paths
	err := VerifyProfilePaths("heap-t000.pb.gz", "heap-t060.pb.gz")
	if err != nil {
		t.Errorf("Expected no error for valid paths, got %v", err)
	}

	// Empty base
	err = VerifyProfilePaths("", "heap-t060.pb.gz")
	if err == nil {
		t.Error("Expected error for empty base")
	}

	// Empty target
	err = VerifyProfilePaths("heap-t000.pb.gz", "")
	if err == nil {
		t.Error("Expected error for empty target")
	}

	// Path with directory
	err = VerifyProfilePaths("/tmp/heap-t000.pb.gz", "heap-t060.pb.gz")
	if err == nil {
		t.Error("Expected error for path with directory")
	}
}

func TestDiffProfileType(t *testing.T) {
	if DiffProfileHeap != "heap" {
		t.Errorf("Expected DiffProfileHeap=heap, got %s", DiffProfileHeap)
	}
	if DiffProfileAllocs != "allocs" {
		t.Errorf("Expected DiffProfileAllocs=allocs, got %s", DiffProfileAllocs)
	}
}
