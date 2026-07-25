// Package evidence defines explicit provenance authorization policies.
package evidence

import "errors"

// ProvenanceCleanPolicy is the required policy selector for provenance
// collection. The non-qualifying option is available only to targeted tests.
type ProvenanceCleanPolicy string

const (
	ProvenanceRequireClean   ProvenanceCleanPolicy = "require_clean"
	ProvenanceIgnoreWorktree ProvenanceCleanPolicy = "ignore_worktree"
)

var ErrUnknownProvenanceCleanPolicy = errors.New("unknown provenance cleanliness policy; expected require_clean or ignore_worktree")

func ValidateProvenanceCleanPolicy(policy ProvenanceCleanPolicy) error {
	switch policy {
	case ProvenanceRequireClean, ProvenanceIgnoreWorktree:
		return nil
	default:
		return ErrUnknownProvenanceCleanPolicy
	}
}
