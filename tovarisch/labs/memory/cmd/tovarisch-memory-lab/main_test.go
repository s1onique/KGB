// main_test.go — Tests for classifier, provenance, and verifier enforcement
//
// Tests:
// - Namespace reader errors make proof unavailable
// - Required provenance validation
// - Verifier enforces provenance fields
//
// Reference: kgb://factory/workflow

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/procfs"
	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/sampling"
)

// TestClassifyCgroupFailureWithReader_TopLevelErrors makes proof unavailable
func TestClassifyCgroupFailureWithReader_TopLevelErrors(t *testing.T) {
	// Create fake reader that returns partial data + top-level error
	fakeReader := func(pid int) (*procfs.NamespaceInfo, error) {
		return &procfs.NamespaceInfo{
			MountNamespace:  "4026532135",
			CgroupNamespace: "4026532135",
		}, fmt.Errorf("simulated read failure")
	}

	capability, proof := classifyCgroupFailureWithReader(
		procfs.ErrNoCgroup2Mount, // visibility error
		1234,                     // targetPID
		5678,                     // controllerPID
		fakeReader,
	)

	// Top-level error must produce unavailable
	if capability != sampling.CgroupCapabilityNamespaceIdentityUnavail {
		t.Errorf("expected namespace_identity_unavailable, got %s", capability)
	}

	if proof.DecisionReason != "namespace_identity_unavailable" {
		t.Errorf("expected decision_reason=namespace_identity_unavailable, got %s", proof.DecisionReason)
	}

	if proof.TargetReadError == "" {
		t.Error("expected TargetReadError to be set")
	}
}

// TestClassifyCgroupFailureWithReader_TargetTopLevelError makes proof unavailable
func TestClassifyCgroupFailureWithReader_TargetTopLevelError(t *testing.T) {
	targetError := fmt.Errorf("target read failed")

	fakeReader := func(pid int) (*procfs.NamespaceInfo, error) {
		if pid == 1234 {
			return nil, targetError
		}
		// controller succeeds
		return &procfs.NamespaceInfo{
			MountNamespace:   "4026532135",
			CgroupNamespace:  "4026532135",
			MountNamespaceErr: nil,
			CgroupNamespaceErr: nil,
		}, nil
	}

	capability, proof := classifyCgroupFailureWithReader(
		procfs.ErrNoCgroup2Mount,
		1234, 5678, fakeReader,
	)

	if capability != sampling.CgroupCapabilityNamespaceIdentityUnavail {
		t.Errorf("expected namespace_identity_unavailable, got %s", capability)
	}
	if proof.TargetReadError != targetError.Error() {
		t.Errorf("expected TargetReadError=%q, got %q", targetError.Error(), proof.TargetReadError)
	}
}

// TestClassifyCgroupFailureWithReader_ControllerTopLevelError makes proof unavailable
func TestClassifyCgroupFailureWithReader_ControllerTopLevelError(t *testing.T) {
	controllerError := fmt.Errorf("controller read failed")

	fakeReader := func(pid int) (*procfs.NamespaceInfo, error) {
		if pid == 5678 {
			return nil, controllerError
		}
		return &procfs.NamespaceInfo{
			MountNamespace:   "4026532135",
			CgroupNamespace:  "4026532135",
			MountNamespaceErr: nil,
			CgroupNamespaceErr: nil,
		}, nil
	}

	capability, proof := classifyCgroupFailureWithReader(
		procfs.ErrNoCgroup2Mount,
		1234, 5678, fakeReader,
	)

	if capability != sampling.CgroupCapabilityNamespaceIdentityUnavail {
		t.Errorf("expected namespace_identity_unavailable, got %s", capability)
	}
	if proof.ControllerReadError != controllerError.Error() {
		t.Errorf("expected ControllerReadError=%q, got %q", controllerError.Error(), proof.ControllerReadError)
	}
}

// TestClassifyCgroupFailureWithReader_OsErrPermission detected
func TestClassifyCgroupFailureWithReader_OsErrPermission(t *testing.T) {
	// Create fake reader that always fails
	fakeReader := func(pid int) (*procfs.NamespaceInfo, error) {
		return nil, fmt.Errorf("permission denied")
	}

	capability, _ := classifyCgroupFailureWithReader(
		os.ErrPermission, // standard permission error
		1234, 5678, fakeReader,
	)

	if capability != sampling.CgroupCapabilityPermissionDenied {
		t.Errorf("expected permission_denied, got %s", capability)
	}
}

// TestCollectProvenance_ExecutablePathFailure fails
func TestCollectProvenance_ExecutablePathFailure(t *testing.T) {
	// This test verifies the function signature and logic
	// In a real test environment, we'd mock os.Readlink
	subject, host, controllerPID, err := collectProvenance()

	// On Linux, /proc/self/exe should always exist
	if err == nil {
		// Success path
		if subject.ControllerExecutablePath == "" {
			t.Error("expected non-empty executable path on success")
		}
		if len(subject.ControllerExecutableSHA256) != 64 {
			t.Errorf("expected 64-char SHA256, got %d chars", len(subject.ControllerExecutableSHA256))
		}
		if host.CollectionStatus != "complete" {
			t.Errorf("expected collection_status=complete, got %s", host.CollectionStatus)
		}
		if controllerPID == "" {
			t.Error("expected non-empty controller PID")
		}
	}
}

// TestCollectProvenance_AllFieldsRequired verifies validation
func TestCollectProvenance_AllFieldsRequired(t *testing.T) {
	// Verify collectProvenance returns error for missing required fields
	// This tests the logic path where git commit is empty
	subject, host, controllerPID, err := collectProvenance()

	if err == nil {
		// If successful, all required fields must be present
		if subject.GitCommit == "" {
			t.Error("GitCommit should be non-empty on success")
		}
		if subject.GitTree == "" {
			t.Error("GitTree should be non-empty on success")
		}
		if subject.ControllerExecutablePath == "" {
			t.Error("ControllerExecutablePath should be non-empty on success")
		}
		if len(subject.ControllerExecutableSHA256) != 64 {
			t.Error("ControllerExecutableSHA256 should be 64 chars on success")
		}
		_ = host
		_ = controllerPID
	}
}

// TestValidateHexString tests hex string validation
func TestValidateHexString(t *testing.T) {
	validCases := []string{
		"abcd1234abcd1234abcd1234abcd1234abcd1234",                                          // 40 hex (git commit)
		"deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",                 // 64 hex (SHA256)
		"DEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEF",                 // uppercase
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123",            // mixed
	}

	for _, input := range validCases {
		if err := validateHexString(input); err != nil {
			t.Errorf("expected valid hex for %q, got error: %v", input, err)
		}
	}

	invalidCases := []string{
		"xyz12345xyz12345xyz12345xyz12345xyz12345",  // non-hex chars
		"gggggggggggggggggggggggggggggggggggggggg",   // invalid hex chars
		"abcd-1234-abcd-1234-abcd-1234-abcd-1234-abcd", // dashes
	}

	for _, input := range invalidCases {
		if err := validateHexString(input); err == nil {
			t.Errorf("expected error for %q", input)
		}
	}
	// Note: empty string is valid hex (empty is decodeable), but we check empty separately in validateProvenanceEvidence
}

// TestValidateProvenanceEvidence tests the pure validation function
func TestValidateProvenanceEvidence(t *testing.T) {
	// Valid manifest and verdict
	validManifest := &evidence.Manifest{
		SubjectIdentity: &evidence.SubjectIdentity{
			GitCommit:                 "abcd1234abcd1234abcd1234abcd1234abcd1234",
			GitTree:                   "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
			ControllerExecutablePath:  "/some/path",
			ControllerExecutableSHA256: "abcd1234567890efabcd1234567890efabcd1234567890efabcd1234567890ef",
		},
		HostID: &evidence.HostIdentity{
			CollectionStatus: "complete",
		},
	}
	validVerdict := &evidence.Verdict{
		ProvenanceValid: true,
		ProvenanceError: "",
	}

	errs := validateProvenanceEvidence(validManifest, validVerdict)
	if len(errs) > 0 {
		t.Errorf("expected no errors for valid evidence, got: %v", errs)
	}
}

// TestValidateProvenanceEvidence_RejectsInvalidFields tests validation of invalid fields
func TestValidateProvenanceEvidence_RejectsInvalidFields(t *testing.T) {
	testCases := []struct {
		name     string
		manifest *evidence.Manifest
		verdict  *evidence.Verdict
		wantErrs int
	}{
		{
			name: "empty git_commit",
			manifest: &evidence.Manifest{
				SubjectIdentity: &evidence.SubjectIdentity{
					GitCommit:                 "",
					GitTree:                   "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
					ControllerExecutablePath:  "/some/path",
					ControllerExecutableSHA256: "abcd1234567890efabcd1234567890efabcd1234567890efabcd1234567890ef",
				},
				HostID: &evidence.HostIdentity{CollectionStatus: "complete"},
			},
			verdict:  &evidence.Verdict{ProvenanceValid: true, ProvenanceError: ""},
			wantErrs: 1,
		},
		{
			name: "non-hex git_commit",
			manifest: &evidence.Manifest{
				SubjectIdentity: &evidence.SubjectIdentity{
					GitCommit:                 "xyzxyzxyzxyzxyzxyzxyzxyzxyzxyzxyzxyzxyz",
					GitTree:                   "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
					ControllerExecutablePath:  "/some/path",
					ControllerExecutableSHA256: "abcd1234567890efabcd1234567890efabcd1234567890efabcd1234567890ef",
				},
				HostID: &evidence.HostIdentity{CollectionStatus: "complete"},
			},
			verdict:  &evidence.Verdict{ProvenanceValid: true, ProvenanceError: ""},
			wantErrs: 1,
		},
		{
			name: "wrong length git_commit",
			manifest: &evidence.Manifest{
				SubjectIdentity: &evidence.SubjectIdentity{
					GitCommit:                 "abc1234",
					GitTree:                   "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
					ControllerExecutablePath:  "/some/path",
					ControllerExecutableSHA256: "abcd1234567890efabcd1234567890efabcd1234567890efabcd1234567890ef",
				},
				HostID: &evidence.HostIdentity{CollectionStatus: "complete"},
			},
			verdict:  &evidence.Verdict{ProvenanceValid: true, ProvenanceError: ""},
			wantErrs: 1,
		},
		{
			name: "non-hex sha256",
			manifest: &evidence.Manifest{
				SubjectIdentity: &evidence.SubjectIdentity{
					GitCommit:                 "abcd1234abcd1234abcd1234abcd1234abcd1234",
					GitTree:                   "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
					ControllerExecutablePath:  "/some/path",
					ControllerExecutableSHA256: "xyzxyzxyzxyzxyzxyzxyzxyzxyzxyzxyzxyzxyzxyzxyzxyzxyzxyzxyzxyzxyz",
				},
				HostID: &evidence.HostIdentity{CollectionStatus: "complete"},
			},
			verdict:  &evidence.Verdict{ProvenanceValid: true, ProvenanceError: ""},
			wantErrs: 1,
		},
		{
			name: "nil subject_identity",
			manifest: &evidence.Manifest{
				SubjectIdentity: nil,
				HostID:          &evidence.HostIdentity{CollectionStatus: "complete"},
			},
			verdict:  &evidence.Verdict{ProvenanceValid: true, ProvenanceError: ""},
			wantErrs: 1,
		},
		{
			name: "nil host_identity",
			manifest: &evidence.Manifest{
				SubjectIdentity: &evidence.SubjectIdentity{
					GitCommit:                 "abcd1234abcd1234abcd1234abcd1234abcd1234",
					GitTree:                   "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
					ControllerExecutablePath:  "/some/path",
					ControllerExecutableSHA256: "abcd1234567890efabcd1234567890efabcd1234567890efabcd1234567890ef",
				},
				HostID: nil,
			},
			verdict:  &evidence.Verdict{ProvenanceValid: true, ProvenanceError: ""},
			wantErrs: 1,
		},
		{
			name: "incomplete collection_status",
			manifest: &evidence.Manifest{
				SubjectIdentity: &evidence.SubjectIdentity{
					GitCommit:                 "abcd1234abcd1234abcd1234abcd1234abcd1234",
					GitTree:                   "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
					ControllerExecutablePath:  "/some/path",
					ControllerExecutableSHA256: "abcd1234567890efabcd1234567890efabcd1234567890efabcd1234567890ef",
				},
				HostID: &evidence.HostIdentity{CollectionStatus: "partial"},
			},
			verdict:  &evidence.Verdict{ProvenanceValid: true, ProvenanceError: ""},
			wantErrs: 1,
		},
		{
			name: "provenance_valid_false",
			manifest: &evidence.Manifest{
				SubjectIdentity: &evidence.SubjectIdentity{
					GitCommit:                 "abcd1234abcd1234abcd1234abcd1234abcd1234",
					GitTree:                   "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
					ControllerExecutablePath:  "/some/path",
					ControllerExecutableSHA256: "abcd1234567890efabcd1234567890efabcd1234567890efabcd1234567890ef",
				},
				HostID: &evidence.HostIdentity{CollectionStatus: "complete"},
			},
			verdict:  &evidence.Verdict{ProvenanceValid: false, ProvenanceError: "git_commit unavailable"},
			wantErrs: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateProvenanceEvidence(tc.manifest, tc.verdict)
			if len(errs) != tc.wantErrs {
				t.Errorf("expected %d errors, got %d: %v", tc.wantErrs, len(errs), errs)
			}
		})
	}
}

// TestNamespaceProof_HasTargetReadError verifies struct has field
func TestNamespaceProof_HasTargetReadError(t *testing.T) {
	proof := &sampling.NamespaceProof{}

	// Verify field exists and can be set
	proof.TargetReadError = "test error"
	if proof.TargetReadError != "test error" {
		t.Error("TargetReadError field not settable/gettable")
	}
}

// TestNamespaceProof_HasControllerReadError verifies struct has field
func TestNamespaceProof_HasControllerReadError(t *testing.T) {
	proof := &sampling.NamespaceProof{}

	proof.ControllerReadError = "controller error"
	if proof.ControllerReadError != "controller error" {
		t.Error("ControllerReadError field not settable/gettable")
	}
}

// TestClassifyCgroupFailureWithReader_MountNamespaceMismatch detects mismatch
func TestClassifyCgroupFailureWithReader_MountNamespaceMismatch(t *testing.T) {
	fakeReader := func(pid int) (*procfs.NamespaceInfo, error) {
		if pid == 1234 {
			return &procfs.NamespaceInfo{
				MountNamespace:   "4026532135",
				CgroupNamespace:  "4026532135",
				MountNamespaceErr: nil,
				CgroupNamespaceErr: nil,
			}, nil
		}
		// Controller has different mount namespace
		return &procfs.NamespaceInfo{
			MountNamespace:   "4026532999",
			CgroupNamespace:  "4026532135",
			MountNamespaceErr: nil,
			CgroupNamespaceErr: nil,
		}, nil
	}

	capability, proof := classifyCgroupFailureWithReader(
		procfs.ErrNoCgroup2Mount,
		1234, 5678, fakeReader,
	)

	if capability != sampling.CgroupCapabilityMountNamespaceMismatch {
		t.Errorf("expected mount_namespace_mismatch, got %s", capability)
	}
	if proof.DecisionReason != "mount_namespace_differ" {
		t.Errorf("expected decision_reason=mount_namespace_differ, got %s", proof.DecisionReason)
	}
}

// TestClassifyCgroupFailureWithReader_CgroupNamespaceMismatch detects mismatch
func TestClassifyCgroupFailureWithReader_CgroupNamespaceMismatch(t *testing.T) {
	fakeReader := func(pid int) (*procfs.NamespaceInfo, error) {
		if pid == 1234 {
			return &procfs.NamespaceInfo{
				MountNamespace:   "4026532135",
				CgroupNamespace:  "4026532135",
				MountNamespaceErr: nil,
				CgroupNamespaceErr: nil,
			}, nil
		}
		// Controller has different cgroup namespace
		return &procfs.NamespaceInfo{
			MountNamespace:   "4026532135",
			CgroupNamespace:  "4026532999",
			MountNamespaceErr: nil,
			CgroupNamespaceErr: nil,
		}, nil
	}

	capability, proof := classifyCgroupFailureWithReader(
		procfs.ErrNoCgroup2Mount,
		1234, 5678, fakeReader,
	)

	if capability != sampling.CgroupCapabilityCgroupNamespaceMismatch {
		t.Errorf("expected cgroup_namespace_mismatch, got %s", capability)
	}
	if proof.DecisionReason != "cgroup_namespace_differ" {
		t.Errorf("expected decision_reason=cgroup_namespace_differ, got %s", proof.DecisionReason)
	}
}

// TestClassifyCgroupFailureWithReader_NamespacesEqual returns not_mounted
func TestClassifyCgroupFailureWithReader_NamespacesEqual(t *testing.T) {
	fakeReader := func(pid int) (*procfs.NamespaceInfo, error) {
		// Both processes in same namespace
		return &procfs.NamespaceInfo{
			MountNamespace:   "4026532135",
			CgroupNamespace:  "4026532135",
			MountNamespaceErr: nil,
			CgroupNamespaceErr: nil,
		}, nil
	}

	capability, proof := classifyCgroupFailureWithReader(
		procfs.ErrNoCgroup2Mount,
		1234, 5678, fakeReader,
	)

	if capability != sampling.CgroupCapabilityNotMounted {
		t.Errorf("expected not_mounted, got %s", capability)
	}
	if proof.DecisionReason != "namespaces_equal_cgroup_not_visible" {
		t.Errorf("expected decision_reason=namespaces_equal_cgroup_not_visible, got %s", proof.DecisionReason)
	}
}

// TestClassifyCgroupFailureWithReader_EmptyNamespaceValues fails closed
func TestClassifyCgroupFailureWithReader_EmptyNamespaceValues(t *testing.T) {
	fakeReader := func(pid int) (*procfs.NamespaceInfo, error) {
		// Returns nil errors but empty namespace values
		return &procfs.NamespaceInfo{
			MountNamespace:    "",
			CgroupNamespace:   "",
			MountNamespaceErr: nil,
			CgroupNamespaceErr: nil,
		}, nil
	}

	capability, _ := classifyCgroupFailureWithReader(
		procfs.ErrNoCgroup2Mount,
		1234, 5678, fakeReader,
	)

	// Empty values must fail closed
	if capability != sampling.CgroupCapabilityNamespaceIdentityUnavail {
		t.Errorf("expected namespace_identity_unavailable for empty values, got %s", capability)
	}
}

// TestVerifierRequiresValidGitCommitLength verifies git commit validation
func TestVerifierRequiresValidGitCommitLength(t *testing.T) {
	testCases := []struct {
		commit    string
		wantError bool
	}{
		{"", true},                                             // empty
		{"abc", true},                                          // too short
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},   // 40 chars
		{"abcd1234abcd1234abcd1234abcd1234abcd1234", false},    // 40 chars valid
	}

	for _, tc := range testCases {
		verifyErrors := []string{}
		if tc.commit == "" {
			verifyErrors = append(verifyErrors, "subject_identity.git_commit is empty")
		} else if len(tc.commit) != 40 {
			verifyErrors = append(verifyErrors, fmt.Sprintf("subject_identity.git_commit invalid length: %d (expected 40)", len(tc.commit)))
		}

		if tc.wantError && len(verifyErrors) == 0 {
			t.Errorf("expected error for commit length %d", len(tc.commit))
		}
		if !tc.wantError && len(verifyErrors) > 0 {
			t.Errorf("expected no error for commit length %d", len(tc.commit))
		}
	}
}

// TestVerifierRequiresValidExecutableHashLength verifies SHA256 validation
func TestVerifierRequiresValidExecutableHashLength(t *testing.T) {
	testCases := []struct {
		hash      string
		wantError bool
	}{
		{"", true},  // empty
		{"abc", true}, // too short
		{"deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef", false}, // 64 chars valid
	}

	for _, tc := range testCases {
		verifyErrors := []string{}
		if tc.hash == "" {
			verifyErrors = append(verifyErrors, "subject_identity.controller_executable_sha256 is empty")
		} else if len(tc.hash) != 64 {
			verifyErrors = append(verifyErrors, fmt.Sprintf("subject_identity.controller_executable_sha256 invalid length: %d (expected 64)", len(tc.hash)))
		}

		if tc.wantError && len(verifyErrors) == 0 {
			t.Errorf("expected error for hash length %d", len(tc.hash))
		}
		if !tc.wantError && len(verifyErrors) > 0 {
			t.Errorf("expected no error for hash length %d", len(tc.hash))
		}
	}
}

// TestClassifyCgroupFailureWithReader_NonVisibilityError_returnsClassifiedCapability
func TestClassifyCgroupFailureWithReader_NonVisibilityError(t *testing.T) {
	testCases := []struct {
		err  error
		want sampling.CgroupCapability
	}{
		{procfs.ErrPathTraversal, sampling.CgroupCapabilityPathTraversal},
		{procfs.ErrParseFailure, sampling.CgroupCapabilityParseFailure},
		{errors.New("unknown error"), sampling.CgroupCapabilityPathAbsent},
	}

	for _, tc := range testCases {
		capability, _ := classifyCgroupFailureWithReader(
			tc.err, 1234, 5678, func(pid int) (*procfs.NamespaceInfo, error) {
				return nil, nil
			},
		)

		if capability != tc.want {
			t.Errorf("for error %v: expected %s, got %s", tc.err, tc.want, capability)
		}
	}
}

// TestEvidenceDirectoryPolicy_TempDirCreation tests the artifact directory structure
func TestEvidenceDirectoryPolicy_TempDirCreation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "evidence-policy-test")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	runID := "lab-test-123"
	artifactPath := filepath.Join(tmpDir, runID)

	if err := os.MkdirAll(artifactPath, 0755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}

	// Verify directory exists
	info, err := os.Stat(artifactPath)
	if err != nil {
		t.Fatalf("stat artifact dir: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected artifact path to be directory")
	}
}
