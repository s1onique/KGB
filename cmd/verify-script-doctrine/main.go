// verify-script-doctrine checks repository scripts against the script doctrine.
//
// It verifies:
//  1. No repository-owned Python files exist
//  2. No Python shebangs are present
//  3. Makefiles, CI files, or shell scripts don't invoke Python
//  4. Shell scripts don't exceed 50 logical lines (excluding blank/comment)
//  5. All shell scripts are listed in the inventory
//  6. Inventory entries reference existing files
//  7. Migrated scripts are not still present
//  8. Shell wrappers don't contain known substantive patterns
//  9. No new script languages are introduced without classification
//
// During Phase 1 (bootstrap), Python files and oversized shell scripts
// that are marked as "migration-required" in the inventory are allowed
// as a temporary exception until migration is complete.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

var (
	// bootstrapMode allows migration-required entries during Phase 1
	bootstrapMode bool
)

func init() {
	flag.BoolVar(&bootstrapMode, "bootstrap", false, "Allow migration-required entries (Phase 1 bootstrap mode)")
}

func main() {
	flag.Parse()

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("finding repo root: %w", err)
	}

	// Load inventory
	inventoryPath := filepath.Join(repoRoot, "docs", "tooling", "script-inventory.csv")
	inventory, err := loadInventory(inventoryPath)
	if err != nil {
		return fmt.Errorf("loading inventory: %w", err)
	}

	var errors []string

	// Check 1: No repository-owned Python files
	pyErrors := checkPythonFiles(repoRoot, inventory)
	errors = append(errors, pyErrors...)

	// Check 2: No Python shebangs
	shebangErrors := checkPythonShebangs(repoRoot, inventory)
	errors = append(errors, shebangErrors...)

	// Check 3: No Python invocations in Makefiles or shell scripts
	pyInvokeErrors := checkPythonInvocations(repoRoot, inventory)
	errors = append(errors, pyInvokeErrors...)

	// Check 4: Shell script line counts
	locErrors := checkShellLineCounts(repoRoot, inventory)
	errors = append(errors, locErrors...)

	// Check 5: All shell scripts are in inventory
	inventoryErrors := checkInventoryCoverage(repoRoot, inventory)
	errors = append(errors, inventoryErrors...)

	// Check 6: Inventory entries reference existing files
	existErrors := checkInventoryFilesExist(repoRoot, inventory)
	errors = append(errors, existErrors...)

	// Check 7: Migrated scripts are not still present
	migratedErrors := checkMigratedScripts(repoRoot, inventory)
	errors = append(errors, migratedErrors...)

	// Check 8: Shell wrappers don't contain risky patterns
	riskyErrors := checkShellRiskyPatterns(repoRoot, inventory)
	errors = append(errors, riskyErrors...)

	// Report results
	if len(errors) > 0 {
		fmt.Println("Script doctrine violations found:")
		for _, e := range errors {
			fmt.Printf("  - %s\n", e)
		}
		return fmt.Errorf("%d violation(s) found", len(errors))
	}

	fmt.Println("Script doctrine verification passed")
	return nil
}
