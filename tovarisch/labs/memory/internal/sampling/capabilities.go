// capabilities.go — Cgroup capability classification constants and proof structures
//
// Reference: kgb://doctrine/embedded-memory-frugality
//
// Sentinel errors (ErrNoCgroup2Mount, ErrNoUnifiedCgroup, ErrPathTraversal,
// ErrPermissionDenied, ErrParseFailure) are defined in procfs/identity.go.

package sampling

// CgroupCapability classifies the result of cgroup v2 resolution attempts.
type CgroupCapability string

// Capability outcomes for cgroup resolution:
const (
	// CgroupCapabilityAvailable means cgroup v2 path was successfully resolved.
	CgroupCapabilityAvailable CgroupCapability = "available"

	// CgroupCapabilityMountNamespaceMismatch means mount namespace differs between
	// controller and target process - container is isolated from cgroup visibility.
	CgroupCapabilityMountNamespaceMismatch CgroupCapability = "mount_namespace_mismatch"

	// CgroupCapabilityCgroupNamespaceMismatch means cgroup namespace differs between
	// controller and target process - container has different cgroup namespace.
	CgroupCapabilityCgroupNamespaceMismatch CgroupCapability = "cgroup_namespace_mismatch"

	// CgroupCapabilityNotMounted means cgroup2 is not mounted in this namespace.
	CgroupCapabilityNotMounted CgroupCapability = "not_mounted"

	// CgroupCapabilityPermissionDenied means insufficient permissions to read cgroup path.
	CgroupCapabilityPermissionDenied CgroupCapability = "permission_denied"

	// CgroupCapabilityParseFailure means cgroup file parsing failed.
	CgroupCapabilityParseFailure CgroupCapability = "parse_failure"

	// CgroupCapabilityPathAbsent means cgroup path does not exist.
	CgroupCapabilityPathAbsent CgroupCapability = "path_absent"

	// CgroupCapabilityNamespaceIdentityUnavail means namespace comparison could not
	// be performed due to read failures.
	CgroupCapabilityNamespaceIdentityUnavail CgroupCapability = "namespace_identity_unavailable"

	// CgroupCapabilityCgroupNotVisible means cgroup2 is not visible from this namespace.
	CgroupCapabilityCgroupNotVisible CgroupCapability = "cgroup_not_visible"

	// CgroupCapabilityNoUnifiedHierarchy means cgroup v2 unified hierarchy not found.
	CgroupCapabilityNoUnifiedHierarchy CgroupCapability = "no_unified_hierarchy"

	// CgroupCapabilityPathTraversal means path would escape mount root.
	CgroupCapabilityPathTraversal CgroupCapability = "path_traversal"
)

// String returns the string representation of a CgroupCapability.
func (c CgroupCapability) String() string {
	return string(c)
}

// NamespaceProof captures evidence for cgroup capability classification.
// This enables independent verification of classification decisions.
type NamespaceProof struct {
	// Target process namespace identities
	TargetMountNamespace   string `json:"target_mount_namespace,omitempty"`
	TargetCgroupNamespace string `json:"target_cgroup_namespace,omitempty"`

	// Controller process namespace identities
	ControllerMountNamespace   string `json:"controller_mount_namespace,omitempty"`
	ControllerCgroupNamespace string `json:"controller_cgroup_namespace,omitempty"`

	// Per-field read errors (for independent verification)
	TargetMountNamespaceErr   string `json:"target_mount_namespace_err,omitempty"`
	TargetCgroupNamespaceErr string `json:"target_cgroup_namespace_err,omitempty"`
	ControllerMountNamespaceErr   string `json:"controller_mount_namespace_err,omitempty"`
	ControllerCgroupNamespaceErr string `json:"controller_cgroup_namespace_err,omitempty"`

	// Namespace read error (combined)
	NamespaceReadError string `json:"namespace_read_error,omitempty"`

	// Human-readable reason for classification decision
	DecisionReason string `json:"decision_reason,omitempty"`
}

// CgroupCapabilityEvent records a cgroup capability classification with proof.
type CgroupCapabilityEvent struct {
	PID             int              `json:"pid"`
	Capability      CgroupCapability `json:"capability"`
	CgroupPath      string          `json:"cgroup_path,omitempty"`
	Error           string          `json:"error,omitempty"`
	ControllerPID   int             `json:"controller_pid"`
	Proof           *NamespaceProof  `json:"proof,omitempty"`
}
