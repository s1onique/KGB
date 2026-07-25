// cgroup_classifier_test.go — Unit tests for cgroup capability classification
//
// Tests the fail-closed classification logic for namespace mismatch detection.

package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/procfs"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling"
)

// mockNamespaceInfo mirrors procfs.NamespaceInfo for test fixtures.
type mockNamespaceInfo struct {
	mountNS     string
	cgroupNS    string
	mountNSErr  error
	cgroupNSErr error
}

// fakeNamespaceReader creates a namespaceReader from a map of mock responses.
func fakeNamespaceReader(responses map[int]*mockNamespaceInfo) namespaceReader {
	return func(pid int) (*procfs.NamespaceInfo, error) {
		if mock, ok := responses[pid]; ok && mock != nil {
			ns := &procfs.NamespaceInfo{
				MountNamespace:     mock.mountNS,
				CgroupNamespace:    mock.cgroupNS,
				MountNamespaceErr:  mock.mountNSErr,
				CgroupNamespaceErr: mock.cgroupNSErr,
			}
			return ns, nil
		}
		// Default: not found
		return nil, errors.New("process not found")
	}
}

func TestClassifyNilError(t *testing.T) {
	cap, proof := classifyCgroupFailureWithNamespace(nil, 1, 2)
	if cap != sampling.CgroupCapabilityAvailable {
		t.Errorf("expected available, got %s", cap)
	}
	if proof != nil {
		t.Errorf("expected nil proof, got %+v", proof)
	}
}

func TestClassifyErrNoCgroup2Mount_MountMismatch(t *testing.T) {
	// Mount namespaces differ → mount_namespace_mismatch
	readNS := fakeNamespaceReader(map[int]*mockNamespaceInfo{
		100: {mountNS: "mnt:[4026531840]", cgroupNS: "cgroup:[4026531835]"},
		1:   {mountNS: "mnt:[4026531841]", cgroupNS: "cgroup:[4026531835]"}, // different mount
	})

	cap, proof := classifyCgroupFailureWithReader(procfs.ErrNoCgroup2Mount, 100, 1, readNS)
	if cap != sampling.CgroupCapabilityMountNamespaceMismatch {
		t.Errorf("expected mount_namespace_mismatch, got %s", cap)
	}
	if proof.DecisionReason != "mount_namespace_differ" {
		t.Errorf("expected mount_namespace_differ, got %s", proof.DecisionReason)
	}
	if proof.TargetMountNamespace != "mnt:[4026531840]" {
		t.Errorf("expected target mnt:[4026531840], got %s", proof.TargetMountNamespace)
	}
	if proof.ControllerMountNamespace != "mnt:[4026531841]" {
		t.Errorf("expected controller mnt:[4026531841], got %s", proof.ControllerMountNamespace)
	}
}

func TestClassifyErrNoCgroup2Mount_CgroupMismatch(t *testing.T) {
	// Mount namespaces equal, cgroup namespaces differ → cgroup_namespace_mismatch
	readNS := fakeNamespaceReader(map[int]*mockNamespaceInfo{
		100: {mountNS: "mnt:[4026531840]", cgroupNS: "cgroup:[4026531835]"},
		1:   {mountNS: "mnt:[4026531840]", cgroupNS: "cgroup:[4026531836]"}, // different cgroup
	})

	cap, proof := classifyCgroupFailureWithReader(procfs.ErrNoCgroup2Mount, 100, 1, readNS)
	if cap != sampling.CgroupCapabilityCgroupNamespaceMismatch {
		t.Errorf("expected cgroup_namespace_mismatch, got %s", cap)
	}
	if proof.DecisionReason != "cgroup_namespace_differ" {
		t.Errorf("expected cgroup_namespace_differ, got %s", proof.DecisionReason)
	}
}

func TestClassifyErrNoCgroup2Mount_BothUnavailable(t *testing.T) {
	// Both identities unreadable → namespace_identity_unavailable
	readNS := fakeNamespaceReader(map[int]*mockNamespaceInfo{
		100: {mountNS: "", cgroupNS: "", mountNSErr: errors.New("permission denied"), cgroupNSErr: errors.New("permission denied")},
		1:   {mountNS: "", cgroupNS: "", mountNSErr: errors.New("permission denied"), cgroupNSErr: errors.New("permission denied")},
	})

	cap, proof := classifyCgroupFailureWithReader(procfs.ErrNoCgroup2Mount, 100, 1, readNS)
	if cap != sampling.CgroupCapabilityNamespaceIdentityUnavail {
		t.Errorf("expected namespace_identity_unavailable, got %s", cap)
	}
	if proof.DecisionReason != "namespace_identity_unavailable" {
		t.Errorf("expected namespace_identity_unavailable, got %s", proof.DecisionReason)
	}
}

func TestClassifyErrNoCgroup2Mount_MountUnavailableCgroupEqual(t *testing.T) {
	// Mount unavailable, cgroup equal → namespace_identity_unavailable (fail-closed)
	readNS := fakeNamespaceReader(map[int]*mockNamespaceInfo{
		100: {mountNS: "", cgroupNS: "cgroup:[4026531835]", mountNSErr: errors.New("permission denied")},
		1:   {mountNS: "", cgroupNS: "cgroup:[4026531835]", mountNSErr: errors.New("permission denied")},
	})

	cap, _ := classifyCgroupFailureWithReader(procfs.ErrNoCgroup2Mount, 100, 1, readNS)
	if cap != sampling.CgroupCapabilityNamespaceIdentityUnavail {
		t.Errorf("expected namespace_identity_unavailable when mount unavailable, got %s", cap)
	}
}

func TestClassifyErrNoCgroup2Mount_MountEqualCgroupUnavailable(t *testing.T) {
	// Mount equal, cgroup unavailable → namespace_identity_unavailable (fail-closed)
	readNS := fakeNamespaceReader(map[int]*mockNamespaceInfo{
		100: {mountNS: "mnt:[4026531840]", cgroupNS: "", cgroupNSErr: errors.New("permission denied")},
		1:   {mountNS: "mnt:[4026531840]", cgroupNS: "", cgroupNSErr: errors.New("permission denied")},
	})

	cap, _ := classifyCgroupFailureWithReader(procfs.ErrNoCgroup2Mount, 100, 1, readNS)
	if cap != sampling.CgroupCapabilityNamespaceIdentityUnavail {
		t.Errorf("expected namespace_identity_unavailable when cgroup unavailable, got %s", cap)
	}
}

func TestClassifyErrNoCgroup2Mount_NamespacesEqual(t *testing.T) {
	// Both namespaces equal → not_mounted
	readNS := fakeNamespaceReader(map[int]*mockNamespaceInfo{
		100: {mountNS: "mnt:[4026531840]", cgroupNS: "cgroup:[4026531835]"},
		1:   {mountNS: "mnt:[4026531840]", cgroupNS: "cgroup:[4026531835]"},
	})

	cap, proof := classifyCgroupFailureWithReader(procfs.ErrNoCgroup2Mount, 100, 1, readNS)
	if cap != sampling.CgroupCapabilityNotMounted {
		t.Errorf("expected not_mounted, got %s", cap)
	}
	if proof.DecisionReason != "namespaces_equal_cgroup_not_visible" {
		t.Errorf("expected namespaces_equal_cgroup_not_visible, got %s", proof.DecisionReason)
	}
}

func TestClassifyErrNoUnifiedCgroup(t *testing.T) {
	// ErrNoUnifiedCgroup: namespaces equal → not_mounted
	readNS := fakeNamespaceReader(map[int]*mockNamespaceInfo{
		100: {mountNS: "mnt:[4026531840]", cgroupNS: "cgroup:[4026531835]"},
		1:   {mountNS: "mnt:[4026531840]", cgroupNS: "cgroup:[4026531835]"},
	})

	cap, _ := classifyCgroupFailureWithReader(procfs.ErrNoUnifiedCgroup, 100, 1, readNS)
	if cap != sampling.CgroupCapabilityNotMounted {
		t.Errorf("expected not_mounted, got %s", cap)
	}
}

func TestClassifyErrPathTraversal(t *testing.T) {
	// Path traversal error → path_traversal
	readNS := fakeNamespaceReader(map[int]*mockNamespaceInfo{
		100: {mountNS: "mnt:[4026531840]", cgroupNS: "cgroup:[4026531835]"},
		1:   {mountNS: "mnt:[4026531840]", cgroupNS: "cgroup:[4026531835]"},
	})

	cap, proof := classifyCgroupFailureWithReader(procfs.ErrPathTraversal, 100, 1, readNS)
	if cap != sampling.CgroupCapabilityPathTraversal {
		t.Errorf("expected path_traversal, got %s", cap)
	}
	if proof.DecisionReason != "path_traversal" {
		t.Errorf("expected path_traversal, got %s", proof.DecisionReason)
	}
}

func TestClassifyErrPermissionDenied(t *testing.T) {
	// Permission denied error → permission_denied
	readNS := fakeNamespaceReader(map[int]*mockNamespaceInfo{
		100: {mountNS: "mnt:[4026531840]", cgroupNS: "cgroup:[4026531835]"},
		1:   {mountNS: "mnt:[4026531840]", cgroupNS: "cgroup:[4026531835]"},
	})

	cap, proof := classifyCgroupFailureWithReader(procfs.ErrPermissionDenied, 100, 1, readNS)
	if cap != sampling.CgroupCapabilityPermissionDenied {
		t.Errorf("expected permission_denied, got %s", cap)
	}
	if proof.DecisionReason != "permission_denied" {
		t.Errorf("expected permission_denied, got %s", proof.DecisionReason)
	}
}

func TestClassifyErrParseFailure(t *testing.T) {
	// Parse failure error → parse_failure
	readNS := fakeNamespaceReader(map[int]*mockNamespaceInfo{
		100: {mountNS: "mnt:[4026531840]", cgroupNS: "cgroup:[4026531835]"},
		1:   {mountNS: "mnt:[4026531840]", cgroupNS: "cgroup:[4026531835]"},
	})

	cap, proof := classifyCgroupFailureWithReader(procfs.ErrParseFailure, 100, 1, readNS)
	if cap != sampling.CgroupCapabilityParseFailure {
		t.Errorf("expected parse_failure, got %s", cap)
	}
	if proof.DecisionReason != "parse_failure" {
		t.Errorf("expected parse_failure, got %s", proof.DecisionReason)
	}
}

func TestClassifyWrappedErrNoCgroup2Mount(t *testing.T) {
	// Wrapped error detected via errors.Is, but namespace comparison overwrites:
	// Equal namespaces → not_mounted (semantic meaning takes precedence)
	readNS := fakeNamespaceReader(map[int]*mockNamespaceInfo{
		100: {mountNS: "mnt:[4026531840]", cgroupNS: "cgroup:[4026531835]"},
		1:   {mountNS: "mnt:[4026531840]", cgroupNS: "cgroup:[4026531835]"},
	})

	wrappedErr := fmt.Errorf("context: %w", procfs.ErrNoCgroup2Mount)
	cap, _ := classifyCgroupFailureWithReader(wrappedErr, 100, 1, readNS)
	if cap != sampling.CgroupCapabilityNotMounted {
		t.Errorf("expected not_mounted (namespace equality takes precedence), got %s", cap)
	}
}

func TestClassifyWrappedErrPermissionDenied(t *testing.T) {
	// Wrapped permission error should be detected via errors.Is
	readNS := fakeNamespaceReader(map[int]*mockNamespaceInfo{
		100: {mountNS: "mnt:[4026531840]", cgroupNS: "cgroup:[4026531835]"},
		1:   {mountNS: "mnt:[4026531840]", cgroupNS: "cgroup:[4026531835]"},
	})

	wrappedErr := fmt.Errorf("failed: %w", procfs.ErrPermissionDenied)
	cap, _ := classifyCgroupFailureWithReader(wrappedErr, 100, 1, readNS)
	if cap != sampling.CgroupCapabilityPermissionDenied {
		t.Errorf("expected permission_denied for wrapped ErrPermissionDenied, got %s", cap)
	}
}
