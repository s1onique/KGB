package scriptdoctrine

import (
	"fmt"
	"os"
	"path/filepath"
)

// checkInventoryCoverage verifies all shell scripts are in inventory.
func (v *Verifier) checkInventoryCoverage() []Diagnostic {
	var diags []Diagnostic

	allScripts, discoverDiags := v.discoverShellScripts()
	diags = append(diags, discoverDiags...)
	listedScripts := make(map[string]bool)
	for path := range v.Inventory {
		listedScripts[path] = true
	}

	for _, rel := range allScripts {
		if !listedScripts[rel] && !v.isInBaseline(rel) {
			diags = append(diags, Diagnostic{
				Check: "missing-inventory",
				Path:  rel,
				Msg:   "shell script not in inventory",
			})
		}
	}

	return diags
}

// checkInventoryFilesExist verifies inventory entries reference existing
// files. LOC drift is also reported here for shell scripts.
func (v *Verifier) checkInventoryFilesExist() []Diagnostic {
	var diags []Diagnostic

	for path, entry := range v.Inventory {
		fullPath := filepath.Join(v.RepoRoot, path)
		info, err := os.Stat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				diags = append(diags, Diagnostic{
					Check: "stale-inventory",
					Path:  path,
					Msg:   "inventory entry references nonexistent file",
				})
				continue
			}
			diags = append(diags, Diagnostic{
				Check: "internal-error",
				Path:  path,
				Msg:   fmt.Sprintf("stat inventory entry: %v", err),
			})
			continue
		}

		if entry.Language == "shell" && entry.Status != "migration-required" {
			actualLOC := CountLogicalLOC(fullPath)
			if actualLOC < 0 {
				diags = append(diags, Diagnostic{
					Check: "internal-error",
					Path:  path,
					Msg:   "could not determine logical LOC",
				})
			} else if actualLOC != entry.LogicalLOC {
				diags = append(diags, Diagnostic{
					Check: "loc-drift",
					Path:  path,
					Msg:   fmt.Sprintf("inventory has %d LOC, actual is %d", entry.LogicalLOC, actualLOC),
				})
			}
		}

		if entry.Status == "migrated" && info != nil {
			diags = append(diags, Diagnostic{
				Check: "migrated-exists",
				Path:  path,
				Msg:   "script marked migrated but still exists",
			})
		}
	}

	return diags
}

// checkMigratedScripts verifies migrated scripts are not still present.
// This check is now folded into checkInventoryFilesExist (one stat per
// entry) but is kept as a separate verifier hook for clarity.
func (v *Verifier) checkMigratedScripts() []Diagnostic {
	return nil
}

// checkUnclassifiedExecutables verifies executables have proper
// classification.
func (v *Verifier) checkUnclassifiedExecutables() []Diagnostic {
	var diags []Diagnostic

	skipCheck := v.Bootstrap

	walkErr := filepath.Walk(v.RepoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			diags = append(diags, Diagnostic{
				Check: "internal-error",
				Path:  path,
				Msg:   fmt.Sprintf("walking for executables: %v", err),
			})
			return nil
		}
		if IsExcludedPath(path) {
			if info != nil && info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}

		rel, _ := filepath.Rel(v.RepoRoot, path)

		if skipCheck {
			if _, exists := v.Inventory[rel]; exists {
				return nil
			}
		}

		ext := filepath.Ext(path)
		if ext == ".sh" || ext == ".bash" || ext == ".py" || ext == ".go" {
			return nil
		}

		if info.Mode()&0111 == 0 {
			return nil
		}

		isBin, err := IsBinaryFile(path)
		if err != nil {
			diags = append(diags, Diagnostic{
				Check: "internal-error",
				Path:  rel,
				Msg:   fmt.Sprintf("reading file for binary check: %v", err),
			})
			return nil
		}
		if isBin {
			return nil
		}

		if _, exists := v.Inventory[rel]; exists {
			return nil
		}

		diags = append(diags, Diagnostic{
			Check: "unclassified-executable",
			Path:  rel,
			Msg:   "executable not in inventory with classification",
		})

		return nil
	})

	if walkErr != nil {
		diags = append(diags, Diagnostic{
			Check: "internal-error",
			Path:  ".",
			Msg:   fmt.Sprintf("walk for executables: %v", walkErr),
		})
	}

	return diags
}
