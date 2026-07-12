package artifactio

import (
	"errors"
	"fmt"
)

// Error is the typed error returned by the artifactio boundary.
// Safe diagnostics only — never includes the underlying secret bytes.
type Error struct {
	// SurfaceID identifies the producer contract for diagnostics.
	SurfaceID string

	// Destination is the final-path destination of the failed write.
	// Only the repository-relative path or hash is ever recorded;
	// the final user-controlled component is fine because that path
	// is part of the producer contract.
	Destination string

	// Sanitizer identifies the typed sanitizer that was applied.
	Sanitizer string

	// RuleID is the registry rule identifier (when applicable).
	RuleID string

	// FieldPath is the safe redacted field path within a structured
	// artifact (when applicable). Diagnostic format only.
	FieldPath string

	// FailureCategory classifies the failure for machine consumption.
	// One of: "policy_invalid", "input_too_large", "output_too_large",
	//         "serialize", "sanitize", "post_validate", "atomic_publish",
	//         "io", "permission".
	FailureCategory string

	// Wrapped is the underlying error without secret values.
	Wrapped error
}

// Error implements the standard error interface.
func (e *Error) Error() string {
	cat := e.FailureCategory
	if cat == "" {
		cat = "unspecified"
	}
	switch {
	case e.RuleID != "" && e.FieldPath != "":
		return fmt.Sprintf("artifactio: surface=%s dest=%s sanitizer=%s category=%s rule=%s field=%s: %v",
			e.SurfaceID, e.Destination, e.Sanitizer, cat, e.RuleID, e.FieldPath, e.Wrapped)
	case e.RuleID != "":
		return fmt.Sprintf("artifactio: surface=%s dest=%s sanitizer=%s category=%s rule=%s: %v",
			e.SurfaceID, e.Destination, e.Sanitizer, cat, e.RuleID, e.Wrapped)
	default:
		return fmt.Sprintf("artifactio: surface=%s dest=%s sanitizer=%s category=%s: %v",
			e.SurfaceID, e.Destination, e.Sanitizer, cat, e.Wrapped)
	}
}

// Unwrap exposes the underlying error for errors.Is / errors.As.
func (e *Error) Unwrap() error { return e.Wrapped }

// Is reports whether the target is an artifactio error.
func (e *Error) Is(target error) bool {
	return target == ErrArtifactIO
}

// ErrArtifactIO is the sentinel error for the artifactio boundary.
var ErrArtifactIO = errors.New("artifactio boundary error")
