// clean_policy.go — Explicit provenance-cleanliness policy.
//
// CORRECTION48 P0-4: the legacy binary `RequireClean bool` is
// promoted into a typed policy with three explicit states:
//
//   * ProvenanceRequireClean — the closure producer MUST run from
//     a clean checkout. Tracked modifications, staged
//     modifications, untracked files, HEAD/build-info revisions
//     mismatches, source-tree mismatches, and embedded
//     `vcs.modified=true` are all hard errors.
//
//   * ProvenanceIgnoreWorktree — the hermetic helper test path
//     is allowed to run from a dirty worktree. The collector
//     still records the dirty state but sets
//     QualifyingObservation=false, so the resulting
//     ControllerProvenance can never authorize `pass=true`.
//
//   * An unknown policy string is rejected at the call site.
//     There is no implicit fallback; the producer must choose
//     explicitly.
//
// The two live production paths (the tovarisch-memory-lab CLI
// and the compiled helper `go test -c` artifact) MUST both
// encode `ProvenanceRequireClean`. Any rollback to
// `ProvenanceIgnoreWorktree` for a live run is a stop condition.

package evidence

import "errors"

// ProvenanceCleanPolicy is the typed provenance-cleanliness
// selector. The constant values are the only acceptable inputs;
// any other string is rejected by ValidateProvenanceCleanPolicy.
type ProvenanceCleanPolicy string

const (
	// ProvenanceRequireClean forbids dirty working trees, dirty
	// HEAD, embedded `vcs.modified=true`, and revision
	// mismatches. This is the canonical closure policy.
	ProvenanceRequireClean ProvenanceCleanPolicy = "require_clean"
	// ProvenanceIgnoreWorktree allows dirty working trees and
	// dirty HEAD but records the dirt and forces
	// QualifyingObservation=false so the collector output can
	// never certify `pass=true`. It is reserved for targeted
	// hermetic tests that intentionally run in a dirty worktree.
	ProvenanceIgnoreWorktree ProvenanceCleanPolicy = "ignore_worktree"
)

// ErrUnknownProvenanceCleanPolicy indicates the producer passed
// a CleanPolicy value that is not in the allowed set.
var ErrUnknownProvenanceCleanPolicy = errors.New("unknown provenance cleanliness policy; expected require_clean or ignore_worktree")

// ValidateProvenanceCleanPolicy ensures the policy is one of
// the recognized values. An empty input is rejected — the
// producer MUST pass an explicit policy.
func ValidateProvenanceCleanPolicy(p ProvenanceCleanPolicy) error {
	switch p {
	case ProvenanceRequireClean, ProvenanceIgnoreWorktree:
		return nil
	case "":
		return ErrUnknownProvenanceCleanPolicy
	default:
		return ErrUnknownProvenanceCleanPolicy
	}
}
