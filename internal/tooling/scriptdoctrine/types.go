// Package scriptdoctrine enforces Go-first tooling doctrine for the KGB repository.
//
// This package provides verification that all repository tooling follows:
//   - No Python files (except third-party/vendor)
//   - No Python shebangs
//   - No Python invocations in Makefiles, CI, or shell scripts
//   - Shell scripts ≤50 logical LOC (excluding blank/comment lines)
//   - All shell scripts listed in inventory
//   - No risky shell patterns (jq, polling, JSON parsing)
//
// Phase 1 (bootstrap) mode allows existing legacy violations with frozen baseline.
package scriptdoctrine

import (
	"fmt"
	"strconv"
)

// Required header columns (must match actual CSV order)
var requiredColumns = []string{"id", "path", "language", "logical_loc", "role", "public_interface", "target_go_command", "status", "notes"}

// Valid enum values
var validLanguages = map[string]bool{"shell": true, "python": true, "go": true, "other": true}
var validStatuses = map[string]bool{"migration-required": true, "migrated": true, "wrapper": true, "third-party-exempt": true}
var validRoles = map[string]bool{
	"verifier": true, "lab-orchestration": true, "ci-glue": true,
	"packaging": true, "bootstrap": true, "test": true,
}

// InventoryEntry represents a single script entry in the inventory CSV.
type InventoryEntry struct {
	ID         string
	Path       string
	Language   string
	LogicalLOC int
	Role       string
	Status     string
	Notes      string
}

// BaselineEntry represents a frozen baseline entry for bootstrap mode.
type BaselineEntry struct {
	Path                  string
	BaselineLOC           int
	PythonInvocationCount int
}

// InventoryLoadError represents errors loading the inventory.
type InventoryLoadError struct {
	Line    int
	Message string
}

func (e *InventoryLoadError) Error() string {
	return fmt.Sprintf("line %d: %s", e.Line, e.Message)
}

// InvocationCount is the structured return value of every internal
// python-invocation counter. The Count field is the number of
// invocation sites that the parser could prove statically. A zero
// Count combined with a nil error means "no python invocation was
// found"; a non-nil error means the parser cannot prove the input is
// python-free so the caller must fail closed.
//
// Public compatibility functions continue to return plain ints (-1
// indicating fail-closed); internal callers use this structured
// value so they can distinguish "matched but unknowable" from "not
// matched".
type InvocationCount struct {
	Count int
}

// String renders the count in diagnostic messages.
func (i InvocationCount) String() string {
	if i.Count == 1 {
		return "1 site"
	}
	return strconv.Itoa(i.Count) + " sites"
}

// HasSites reports whether the count is greater than zero.
func (i InvocationCount) HasSites() bool {
	return i.Count > 0
}

// ZeroCount is the canonical empty InvocationCount.
var ZeroCount = InvocationCount{Count: 0}

// ClassificationError is returned by internal parser boundaries
// when a Python execution surface cannot be classified statically.
// The Path / Line / Column fields identify the source location;
// Reason is a short human-readable explanation. Use the canonical
// fail-closed error if the rest of the system only needs the
// message.
type ClassificationError struct {
	Path   string
	Line   int
	Column int
	Reason string
}

func (e *ClassificationError) Error() string {
	return fmt.Sprintf("unable to classify Python execution at line %d, column %d: %s", e.Line, e.Column, e.Reason)
}

// NewClassificationError builds a ClassificationError with the
// canonical diagnostic fields populated.
func NewClassificationError(path string, line, column int, reason string) *ClassificationError {
	return &ClassificationError{
		Path:   path,
		Line:   line,
		Column: column,
		Reason: reason,
	}
}

// WorkflowRunStep projects one GitHub Actions run step into the
// values the verifier needs to compute its effective shell without
// re-walking the workflow tree.
//
// Shell precedence:
//
//	StepShell (step.shell:)
//	> JobDefaults (job.defaults.run.shell)
//	> WorkflowShell (workflow.defaults.run.shell)
//	> platform default (bash on Linux/macOS, pwsh on Windows)
type WorkflowRunStep struct {
	JobID         string
	StepIndex     int
	Run           string
	StepShell     string
	JobDefaults   string
	WorkflowShell string
	Line          int
	Column        int
}
