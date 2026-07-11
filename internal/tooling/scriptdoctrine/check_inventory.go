package scriptdoctrine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// checkInventoryCoverage verifies all shell scripts are in inventory.
func (v *Verifier) checkInventoryCoverage() []Diagnostic {
	var diags []Diagnostic

	allScripts := v.discoverShellScripts()
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

		// Check LOC drift for shell scripts (skip in bootstrap mode for migration-required)
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

		// If file is supposed to be removed (status=migrated), it must not exist.
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
// classification. The blanket `/packaging/` exemption has been removed:
// third-party packaging artifacts must use explicit allowlists rather
// than a directory-wide exemption.
func (v *Verifier) checkUnclassifiedExecutables() []Diagnostic {
	var diags []Diagnostic

	// In bootstrap mode, skip all checks except truly new files
	skipCheck := v.Bootstrap

	// Find all executables without known extensions
	filepath.Walk(v.RepoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			diags = append(diags, Diagnostic{
				Check: "internal-error",
				Path:  path,
				Msg:   fmt.Sprintf("walking for executables: %v", err),
			})
			return nil
		}
		if info.IsDir() {
			return nil
		}

		// Skip vendor/third_party and other external directories
		if strings.Contains(path, "/vendor/") || strings.Contains(path, "/third_party/") ||
			strings.Contains(path, "/node_modules/") || strings.Contains(path, "/dist/") ||
			strings.Contains(path, "/zig-out/") || strings.Contains(path, ".git/hooks/") {
			return nil
		}

		rel, _ := filepath.Rel(v.RepoRoot, path)

		// In bootstrap mode, only flag files NOT in inventory
		if skipCheck {
			if _, exists := v.Inventory[rel]; exists {
				return nil
			}
		}

		// Skip if has known extension
		if strings.HasSuffix(path, ".sh") || strings.HasSuffix(path, ".bash") ||
			strings.HasSuffix(path, ".py") || strings.HasSuffix(path, ".go") {
			return nil
		}

		// Check executable bit
		if info.Mode()&0111 == 0 {
			return nil
		}

		// Skip compiled binaries
		if isBinaryFile(path) {
			return nil
		}

		// Check if it's in inventory
		if _, exists := v.Inventory[rel]; exists {
			return nil
		}

		// Unclassified executable
		diags = append(diags, Diagnostic{
			Check: "unclassified-executable",
			Path:  rel,
			Msg:   "executable not in inventory with classification",
		})

		return nil
	})

	return diags
}

// isBinaryFile checks if a file is a binary (contains null bytes).
// Returns false on any read error so the caller can decide what to do.
func isBinaryFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	if n == 0 {
		return false
	}

	// Check for null bytes (common in binaries)
	for i := 0; i < n && i < 512; i++ {
		if buf[i] == 0 {
			return true
		}
	}

	return false
}
