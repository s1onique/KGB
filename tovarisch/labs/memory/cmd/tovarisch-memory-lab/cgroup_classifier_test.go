// cgroup_classifier_test.go — Unit tests for cgroup capability classification
//
// Tests the fail-closed classification logic for namespace mismatch detection.

package main

import (
	"errors"
	"testing"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/procfs"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling"
)

// TestClassifyCgroupNamespaceMismatch tests mount namespace mismatch classification.
func TestClassifyCgroupNamespaceMismatch(t *testing.T) {
	// Create a mock error that wraps the expected sentinel
	mockErr := procfs.ErrNoCgroup2Mount

	// Simulate different namespace IDs for target and controller
	targetNS := &mockNamespaceInfo{
		mountNS:   "mnt:[4026531840]",
		cgroupNS:  "cgroup:[4026531835]",
		mountNSErr: nil,
		cgroupNSErr: nil,
	}
	controllerNS := &mockNamespaceInfo{
		mountNS:   "mnt:[4026531841]", // Different mount namespace
		cgroupNS:  "cgroup:[4026531835]",
		mountNSErr: nil,
		cgroupNSErr: nil,
	}

	capability, proof := testClassifyWithNS(mockErr, targetNS, controllerNS)

	if capability != sampling.CgroupCapabilityMountNamespaceMismatch {
		t.Errorf("expected mount_namespace_mismatch, got %s", capability)
	}
	if proof.DecisionReason != "mount_namespace_differ" {
		t.Errorf("expected mount_namespace_differ, got %s", proof.DecisionReason)
	}
	if proof.TargetMountNamespace != "mnt:[4026531840]" {
		t.Errorf("expected target mount ns mnt:[4026531840], got %s", proof.TargetMountNamespace)
	}
	if proof.ControllerMountNamespace != "mnt:[4026531841]" {
		t.Errorf("expected controller mount ns mnt:[4026531841], got %s", proof.ControllerMountNamespace)
	}
}

// TestClassifyCgroupNamespaceDifferent tests cgroup namespace mismatch classification.
func TestClassifyCgroupNamespaceDifferent(t *testing.T) {
	mockErr := procfs.ErrNoCgroup2Mount

	targetNS := &mockNamespaceInfo{
		mountNS:   "mnt:[4026531840]",
		cgroupNS:  "cgroup:[4026531835]",
		mountNSErr: nil,
		cgroupNSErr: nil,
	}
	controllerNS := &mockNamespaceInfo{
		mountNS:   "mnt:[4026531840]", // Same mount namespace
		cgroupNS:  "cgroup:[4026531836]", // Different cgroup namespace
		mountNSErr: nil,
		cgroupNSErr: nil,
	}

	capability, proof := testClassifyWithNS(mockErr, targetNS, controllerNS)

	if capability != sampling.CgroupCapabilityCgroupNamespaceMismatch {
		t.Errorf("expected cgroup_namespace_mismatch, got %s", capability)
	}
	if proof.DecisionReason != "cgroup_namespace_differ" {
		t.Errorf("expected cgroup_namespace_differ, got %s", proof.DecisionReason)
	}
}

// TestClassifyNamespaceIdentityUnavailable tests that unavailable identity returns namespace_identity_unavailable.
func TestClassifyNamespaceIdentityUnavailable(t *testing.T) {
	mockErr := procfs.ErrNoCgroup2Mount

	// Both identities unreadable
	targetNS := &mockNamespaceInfo{
		mountNS:   "",
		cgroupNS:  "",
		mountNSErr: errors.New("permission denied"),
		cgroupNSErr: errors.New("permission denied"),
	}
	controllerNS := &mockNamespaceInfo{
		mountNS:   "",
		cgroupNS:  "",
		mountNSErr: errors.New("permission denied"),
		cgroupNSErr: errors.New("permission denied"),
	}

	capability, proof := testClassifyWithNS(mockErr, targetNS, controllerNS)

	if capability != sampling.CgroupCapabilityNamespaceIdentityUnavail {
		t.Errorf("expected namespace_identity_unavailable, got %s", capability)
	}
	if proof.DecisionReason != "namespace_identity_unavailable" {
		t.Errorf("expected namespace_identity_unavailable, got %s", proof.DecisionReason)
	}
}

// TestClassifyMountUnavailableCgroupEqual tests mount unavailable with cgroup equal.
func TestClassifyMountUnavailableCgroupEqual(t *testing.T) {
	mockErr := procfs.ErrNoCgroup2Mount

	targetNS := &mockNamespaceInfo{
		mountNS:   "",
		cgroupNS:  "cgroup:[4026531835]",
		mountNSErr: errors.New("permission denied"),
		cgroupNSErr: nil,
	}
	controllerNS := &mockNamespaceInfo{
		mountNS:   "",
		cgroupNS:  "cgroup:[4026531835]", // Same cgroup namespace
		mountNSErr: errors.New("permission denied"),
		cgroupNSErr: nil,
	}

	capability, _ := testClassifyWithNS(mockErr, targetNS, controllerNS)

	// Fail-closed: mount unavailable, cannot complete comparison
	if capability != sampling.CgroupCapabilityNamespaceIdentityUnavail {
		t.Errorf("expected namespace_identity_unavailable when mount unavailable, got %s", capability)
	}
}

// TestClassifyMountEqualCgroupUnavailable tests mount equal with cgroup unavailable.
func TestClassifyMountEqualCgroupUnavailable(t *testing.T) {
	mockErr := procfs.ErrNoCgroup2Mount

	targetNS := &mockNamespaceInfo{
		mountNS:   "mnt:[4026531840]",
		cgroupNS:  "",
		mountNSErr: nil,
		cgroupNSErr: errors.New("permission denied"),
	}
	controllerNS := &mockNamespaceInfo{
		mountNS:   "mnt:[4026531840]", // Same mount namespace
		cgroupNS:  "",
		mountNSErr: nil,
		cgroupNSErr: errors.New("permission denied"),
	}

	capability, _ := testClassifyWithNS(mockErr, targetNS, controllerNS)

	// Fail-closed: cgroup unavailable, cannot complete comparison
	if capability != sampling.CgroupCapabilityNamespaceIdentityUnavail {
		t.Errorf("expected namespace_identity_unavailable when cgroup unavailable, got %s", capability)
	}
}

// TestClassifyNamespacesEqual tests that equal namespaces return not_mounted.
func TestClassifyNamespacesEqual(t *testing.T) {
	mockErr := procfs.ErrNoCgroup2Mount

	targetNS := &mockNamespaceInfo{
		mountNS:   "mnt:[4026531840]",
		cgroupNS:  "cgroup:[4026531835]",
		mountNSErr: nil,
		cgroupNSErr: nil,
	}
	controllerNS := &mockNamespaceInfo{
		mountNS:   "mnt:[4026531840]", // Same mount namespace
		cgroupNS:  "cgroup:[4026531835]", // Same cgroup namespace
		mountNSErr: nil,
		cgroupNSErr: nil,
	}

	capability, proof := testClassifyWithNS(mockErr, targetNS, controllerNS)

	if capability != sampling.CgroupCapabilityNotMounted {
		t.Errorf("expected not_mounted, got %s", capability)
	}
	if proof.DecisionReason != "namespaces_equal_cgroup_not_visible" {
		t.Errorf("expected namespaces_equal_cgroup_not_visible, got %s", proof.DecisionReason)
	}
}

// TestClassifyPermissionDenied tests permission denied classification.
func TestClassifyPermissionDenied(t *testing.T) {
	mockErr := errors.New("permission denied: /proc/123/cgroup")

	targetNS := &mockNamespaceInfo{
		mountNS:   "mnt:[4026531840]",
		cgroupNS:  "cgroup:[4026531835]",
		mountNSErr: nil,
		cgroupNSErr: nil,
	}
	controllerNS := &mockNamespaceInfo{
		mountNS:   "mnt:[4026531840]",
		cgroupNS:  "cgroup:[4026531835]",
		mountNSErr: nil,
		cgroupNSErr: nil,
	}

	capability, _ := testClassifyWithNS(mockErr, targetNS, controllerNS)

	if capability != sampling.CgroupCapabilityPermissionDenied {
		t.Errorf("expected permission_denied, got %s", capability)
	}
}

// TestClassifyNoUnifiedHierarchy tests no unified hierarchy classification.
func TestClassifyNoUnifiedHierarchy(t *testing.T) {
	mockErr := procfs.ErrNoUnifiedCgroup

	targetNS := &mockNamespaceInfo{
		mountNS:   "mnt:[4026531840]",
		cgroupNS:  "cgroup:[4026531835]",
		mountNSErr: nil,
		cgroupNSErr: nil,
	}
	controllerNS := &mockNamespaceInfo{
		mountNS:   "mnt:[4026531840]",
		cgroupNS:  "cgroup:[4026531835]",
		mountNSErr: nil,
		cgroupNSErr: nil,
	}

	capability, _ := testClassifyWithNS(mockErr, targetNS, controllerNS)

	// Namespaces equal, cgroup not visible in this namespace
	if capability != sampling.CgroupCapabilityNotMounted {
		t.Errorf("expected not_mounted, got %s", capability)
	}
}

// TestClassifyNilError tests nil error returns available.
func TestClassifyNilError(t *testing.T) {
	capability, proof := classifyCgroupFailureWithNamespace(nil, 1, 2)

	if capability != sampling.CgroupCapabilityAvailable {
		t.Errorf("expected available, got %s", capability)
	}
	if proof != nil {
		t.Errorf("expected nil proof for available, got %+v", proof)
	}
}

// mockNamespaceInfo implements namespace reading for testing.
type mockNamespaceInfo struct {
	mountNS      string
	cgroupNS     string
	mountNSErr  error
	cgroupNSErr error
}

// testClassifyWithNS is a test helper that simulates namespace classification.
func testClassifyWithNS(err error, targetNS, controllerNS *mockNamespaceInfo) (sampling.CgroupCapability, *sampling.NamespaceProof) {
	proof := &sampling.NamespaceProof{}

	if err == nil {
		return sampling.CgroupCapabilityAvailable, nil
	}

	// Classify by error type
	var capability sampling.CgroupCapability
	switch {
	case errors.Is(err, procfs.ErrNoCgroup2Mount):
		capability = sampling.CgroupCapabilityCgroupNotVisible
	case errors.Is(err, procfs.ErrNoUnifiedCgroup):
		capability = sampling.CgroupCapabilityNoUnifiedHierarchy
	case errors.Is(err, procfs.ErrPathTraversal):
		capability = sampling.CgroupCapabilityPathTraversal
	default:
		errStr := err.Error()
		if contains(errStr, "permission denied") || contains(errStr, "permission") {
			capability = sampling.CgroupCapabilityPermissionDenied
		} else if contains(errStr, "parse") {
			capability = sampling.CgroupCapabilityParseFailure
		} else {
			capability = sampling.CgroupCapabilityPathAbsent
		}
	}

	// Populate proof
	if targetNS != nil {
		proof.TargetMountNamespace = targetNS.mountNS
		proof.TargetCgroupNamespace = targetNS.cgroupNS
		if targetNS.mountNSErr != nil {
			proof.TargetMountNamespaceErr = targetNS.mountNSErr.Error()
		}
		if targetNS.cgroupNSErr != nil {
			proof.TargetCgroupNamespaceErr = targetNS.cgroupNSErr.Error()
		}
	}
	if controllerNS != nil {
		proof.ControllerMountNamespace = controllerNS.mountNS
		proof.ControllerCgroupNamespace = controllerNS.cgroupNS
		if controllerNS.mountNSErr != nil {
			proof.ControllerMountNamespaceErr = controllerNS.mountNSErr.Error()
		}
		if controllerNS.cgroupNSErr != nil {
			proof.ControllerCgroupNamespaceErr = controllerNS.cgroupNSErr.Error()
		}
	}

	// For cgroup visibility errors, attempt namespace comparison
	if capability == sampling.CgroupCapabilityCgroupNotVisible ||
		capability == sampling.CgroupCapabilityNoUnifiedHierarchy {

		canProveMount := targetNS != nil && targetNS.mountNSErr == nil &&
			controllerNS != nil && controllerNS.mountNSErr == nil
		canProveCgroup := targetNS != nil && targetNS.cgroupNSErr == nil &&
			controllerNS != nil && controllerNS.cgroupNSErr == nil

		// Check mount namespace mismatch
		if canProveMount &&
			proof.TargetMountNamespace != "" && proof.ControllerMountNamespace != "" &&
			proof.TargetMountNamespace != proof.ControllerMountNamespace {
			proof.DecisionReason = "mount_namespace_differ"
			return sampling.CgroupCapabilityMountNamespaceMismatch, proof
		}

		// Check cgroup namespace mismatch
		if canProveCgroup &&
			proof.TargetCgroupNamespace != "" && proof.ControllerCgroupNamespace != "" &&
			proof.TargetCgroupNamespace != proof.ControllerCgroupNamespace {
			proof.DecisionReason = "cgroup_namespace_differ"
			return sampling.CgroupCapabilityCgroupNamespaceMismatch, proof
		}

		// Fail-closed: if we needed to prove identity but couldn't
		if !canProveMount || !canProveCgroup {
			proof.DecisionReason = "namespace_identity_unavailable"
			return sampling.CgroupCapabilityNamespaceIdentityUnavail, proof
		}

		// Both identities proven equal
		proof.DecisionReason = "namespaces_equal_cgroup_not_visible"
		return sampling.CgroupCapabilityNotMounted, proof
	}

	proof.DecisionReason = capability.String()
	return capability, proof
}
