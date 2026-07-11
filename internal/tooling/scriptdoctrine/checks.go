package scriptdoctrine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Diagnostic represents a single doctrine violation.
type Diagnostic struct {
	Check string
	Path  string
	Msg   string
}

// MaxShellLOC is the maximum allowed logical LOC for shell scripts.
const MaxShellLOC = 50

// Verifier performs doctrine verification.
type Verifier struct {
	RepoRoot  string
	Bootstrap bool
	Baseline  map[string]*BaselineEntry // Frozen legacy baseline
	Inventory map[string]*InventoryEntry
}

// NewVerifier creates a new verifier instance.
func NewVerifier(repoRoot string, bootstrap bool) *Verifier {
	return &Verifier{
		RepoRoot:  repoRoot,
		Bootstrap: bootstrap,
		Baseline:  make(map[string]*BaselineEntry),
		Inventory: make(map[string]*InventoryEntry),
	}
}

// SetBaseline sets the frozen legacy baseline for bootstrap mode.
func (v *Verifier) SetBaseline(baseline map[string]*BaselineEntry) {
	v.Baseline = baseline
}

// SetInventory sets the script inventory.
func (v *Verifier) SetInventory(inventory map[string]*InventoryEntry) {
	v.Inventory = inventory
}

// Verify runs all doctrine checks and returns violations.
func (v *Verifier) Verify() []Diagnostic {
	var diags []Diagnostic

	// Check 0: Bootstrap baseline enforcement.
	if v.Bootstrap {
		diags = append(diags, v.CheckBootstrapBaseline()...)
		diags = append(diags, v.checkBaselineEnforcement()...)
	}

	// Check 1: No Python files.
	diags = append(diags, v.checkPythonFiles()...)

	// Check 2: No Python shebangs (any file, regardless of executable bit).
	diags = append(diags, v.checkPythonShebangs()...)

	// Check 3: No Python invocations in Makefiles, CI, shell.
	diags = append(diags, v.checkPythonInvocations()...)

	// Check 4: Shell script line counts.
	diags = append(diags, v.checkShellLineCounts()...)

	// Check 5: All scripts in inventory.
	diags = append(diags, v.checkInventoryCoverage()...)

	// Check 6: Inventory entries reference existing files.
	diags = append(diags, v.checkInventoryFilesExist()...)

	// Check 7: Migrated scripts are not still present (folded into 6).
	diags = append(diags, v.checkMigratedScripts()...)

	// Check 8: Shell wrappers don't contain risky patterns.
	diags = append(diags, v.checkShellRiskyPatterns()...)

	// Check 9: Script language classification (unclassified executables).
	diags = append(diags, v.checkUnclassifiedExecutables()...)

	// Sort for deterministic output.
	SortDiagnostics(diags)

	return diags
}

// isInBaseline checks if a path is in the frozen legacy baseline.
func (v *Verifier) isInBaseline(path string) bool {
	_, exists := v.Baseline[path]
	return exists
}

// isLegacy checks if a path is a legacy violation.
// Only paths in the frozen baseline are considered legacy.
// Inventory-based legacy detection is NOT authoritative - the baseline is.
func (v *Verifier) isLegacy(path string) bool {
	_, exists := v.Baseline[path]
	return exists
}

// CheckBootstrapBaseline checks that bootstrap mode has a proper frozen baseline.
// Returns diagnostics for any entries marked migration-required but not in baseline.
func (v *Verifier) CheckBootstrapBaseline() []Diagnostic {
	var diags []Diagnostic

	if !v.Bootstrap {
		return diags
	}

	// Check that all migration-required entries are in baseline
	for path, entry := range v.Inventory {
		if entry.Status == "migration-required" {
			if _, exists := v.Baseline[path]; !exists {
				diags = append(diags, Diagnostic{
					Check: "bootstrap-missing-baseline",
					Path:  path,
					Msg:   "migration-required entry missing from baseline",
				})
			}
		}
	}

	return diags
}

// checkBaselineEnforcement verifies baseline entries don't exceed their
// frozen limits. Every filesystem, read, or scanner error is reported as
// an internal-error diagnostic - verification fails closed.
func (v *Verifier) checkBaselineEnforcement() []Diagnostic {
	var diags []Diagnostic

	if !v.Bootstrap {
		return diags
	}

	// Check each baseline entry against current state
	for path, baseline := range v.Baseline {
		fullPath := filepath.Join(v.RepoRoot, path)

		// Stat first; any non-IsNotExist error is a hard failure.
		info, err := os.Stat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				diags = append(diags, Diagnostic{
					Check: "stale-bootstrap-baseline",
					Path:  path,
					Msg:   "baseline entry refers to non-existent file",
				})
				continue
			}
			diags = append(diags, Diagnostic{
				Check: "internal-error",
				Path:  path,
				Msg:   fmt.Sprintf("stat baseline entry: %v", err),
			})
			continue
		}
		if !info.Mode().IsRegular() {
			diags = append(diags, Diagnostic{
				Check: "internal-error",
				Path:  path,
				Msg:   fmt.Sprintf("baseline entry is not a regular file: mode %v", info.Mode()),
			})
			continue
		}

		// Check LOC ceiling.
		loc := CountLogicalLOC(fullPath)
		if loc < 0 {
			diags = append(diags, Diagnostic{
				Check: "internal-error",
				Path:  path,
				Msg:   "could not determine logical LOC for baseline entry",
			})
		} else if baseline.BaselineLOC > 0 && loc > baseline.BaselineLOC {
			diags = append(diags, Diagnostic{
				Check: "baseline-loc-exceeded",
				Path:  path,
				Msg:   fmt.Sprintf("has %d LOC (baseline ceiling: %d)", loc, baseline.BaselineLOC),
			})
		}

		// Check Python invocation count.
		// For .py files, they ARE Python files (migration-required, not counted as invocations).
		if strings.HasSuffix(path, ".py") || strings.HasSuffix(path, ".pyw") {
			continue
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			diags = append(diags, Diagnostic{
				Check: "internal-error",
				Path:  path,
				Msg:   fmt.Sprintf("reading baseline entry: %v", err),
			})
			continue
		}

		actualCount := CountPythonInvocationsForPath(path, data)
		if actualCount < 0 {
			diags = append(diags, Diagnostic{
				Check: "internal-error",
				Path:  path,
				Msg:   "could not determine Python invocation count for baseline entry",
			})
			continue
		}
		if actualCount != baseline.PythonInvocationCount {
			diags = append(diags, Diagnostic{
				Check: "baseline-python-invocation-changed",
				Path:  path,
				Msg:   fmt.Sprintf("Python invocation count: baseline=%d, current=%d", baseline.PythonInvocationCount, actualCount),
			})
		}
	}

	return diags
}
