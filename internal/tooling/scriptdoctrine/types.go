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

import "fmt"

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
