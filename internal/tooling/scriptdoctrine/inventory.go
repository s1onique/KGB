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

// Re-export types and functions from split modules for backwards compatibility.
// The actual implementations are in:
//   - types.go: Type definitions
//   - load_inventory.go: Inventory CSV loading
//   - load_baseline.go: Baseline CSV loading
//   - shell_analysis.go: LOC counting and Python detection
