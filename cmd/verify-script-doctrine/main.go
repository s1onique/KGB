// verify-script-doctrine checks repository scripts against the script doctrine.
//
// Entry point only - delegates to internal/tooling/scriptdoctrine package.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/s1onique/KGB/internal/tooling/scriptdoctrine"
)

func main() {
	bootstrap := flag.Bool("bootstrap", false, "Enable bootstrap mode (allows legacy violations)")
	flag.Parse()

	if err := run(*bootstrap); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func run(bootstrap bool) error {
	repoRoot, err := scriptdoctrine.FindRepoRoot()
	if err != nil {
		return fmt.Errorf("finding repo root: %w", err)
	}

	// Load inventory relative to repo root
	inventoryPath := filepath.Join(repoRoot, "docs", "tooling", "script-inventory.csv")
	inventory, err := scriptdoctrine.LoadInventory(inventoryPath)
	if err != nil {
		return fmt.Errorf("loading inventory: %w", err)
	}

	// Create verifier
	verifier := scriptdoctrine.NewVerifier(repoRoot, bootstrap)
	verifier.SetInventory(inventory)

	// Load baseline from independent baseline file if bootstrap mode
	if bootstrap {
		baselinePath := filepath.Join(repoRoot, "docs", "tooling", "script-doctrine-bootstrap-baseline.csv")
		baseline, err := scriptdoctrine.LoadBaseline(baselinePath)
		if err != nil {
			return fmt.Errorf("loading bootstrap baseline: %w", err)
		}
		verifier.SetBaseline(baseline)
	}

	// Run verification
	diags := verifier.Verify()
	scriptdoctrine.SortDiagnostics(diags)

	// Report results
	if len(diags) > 0 {
		fmt.Println("Script doctrine violations found:")
		for _, d := range diags {
			fmt.Printf("  [%s] %s: %s\n", d.Check, d.Path, d.Msg)
		}
		return fmt.Errorf("%d violation(s) found", len(diags))
	}

	fmt.Println("Script doctrine verification passed")
	return nil
}
