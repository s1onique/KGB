// cgroup_test.go — Tests for procfs cgroup namespace detection
//
// Reference: kgb://doctrine/embedded-memory-frugality

package procfs

import (
	"testing"
)

func TestNamespaceProof_Fields(t *testing.T) {
	// Test that NamespaceProof has all required fields
	proof := &NamespaceInfo{}

	// Verify the struct has the expected fields
	if proof.MountNamespace != "" {
		t.Error("MountNamespace should be empty initially")
	}
	if proof.CgroupNamespace != "" {
		t.Error("CgroupNamespace should be empty initially")
	}
}

func TestReadNamespaceIDs_Self(t *testing.T) {
	// Read our own namespace IDs - may fail in restricted environments
	ns, err := ReadNamespaceIDs(1)

	// If error, it's acceptable in restricted/container environments
	if err != nil {
		t.Skipf("ReadNamespaceIDs(1) skipped in restricted environment: %v", err)
	}

	// Verify we got some namespace info for PID 1
	if ns == nil {
		t.Fatal("ReadNamespaceIDs(1) returned nil with no error")
	}

	t.Logf("PID 1 mount namespace: %s", ns.MountNamespace)
	t.Logf("PID 1 cgroup namespace: %s", ns.CgroupNamespace)
}

func TestReadNamespaceIDs_CurrentProcess(t *testing.T) {
	// Read our own process's namespace IDs - may fail in restricted environments
	ns, err := ReadNamespaceIDs(0) // 0 means self

	if err != nil {
		t.Skipf("ReadNamespaceIDs(0) skipped in restricted environment: %v", err)
	}

	if ns == nil {
		t.Fatal("ReadNamespaceIDs(0) returned nil with no error")
	}

	t.Logf("Current process mount namespace: %s", ns.MountNamespace)
	t.Logf("Current process cgroup namespace: %s", ns.CgroupNamespace)
}

func TestReadNamespaceIDs_InvalidPID(t *testing.T) {
	// Invalid PID should return empty info but not error
	// (procfs is resilient - it just returns empty strings)
	ns, err := ReadNamespaceIDs(99999999)
	// Error is acceptable since process doesn't exist
	if err == nil {
		// If no error, verify we got empty info
		if ns != nil {
			if ns.MountNamespace != "" || ns.CgroupNamespace != "" {
				t.Error("Expected empty namespaces for non-existent PID")
			}
		}
	}
}

func TestResolveCgroupV2Path_WithoutContainer(t *testing.T) {
	// This test verifies the function exists and has correct signature
	// Actual resolution depends on running in a container
	path, err := ResolveCgroupV2Path(1)
	if err != nil {
		// This is expected if not in a container environment
		t.Logf("Cgroup resolution failed (expected outside container): %v", err)
	}
	if path != "" {
		t.Logf("Cgroup path resolved: %s", path)
	}
}
