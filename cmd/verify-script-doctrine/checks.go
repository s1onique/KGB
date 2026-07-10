// verify-script-doctrine checks repository scripts against the script doctrine.
//
// All verification checks.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	maxLogicalLOC = 50
)

var (
	// Patterns that indicate substantive shell scripting
	riskyPatterns = []struct {
		pattern *regexp.Regexp
		name    string
	}{
		{regexp.MustCompile(`\bjq\b`), "jq usage"},
		{regexp.MustCompile(`curl.*\|.*grep`), "curl pipe to grep"},
		{regexp.MustCompile(`while.*sleep`), "polling loop"},
		{regexp.MustCompile(`until.*sleep`), "polling loop"},
		{regexp.MustCompile(`for.*in.*sleep`), "retry loop"},
		{regexp.MustCompile(`gh release (create|upload|edit)`), "release decisions"},
		{regexp.MustCompile(`trap.*cleanup.*exit`), "complex cleanup"},
		{regexp.MustCompile(`python.*json.*parse`), "JSON in shell"},
		{regexp.MustCompile(`grep.*\{.*\}.*json`), "regex on JSON"},
	}

	// Python shebang patterns
	pythonShebang = regexp.MustCompile(`^#!.*(?:python|python3|pip|pytest)`)

	// Python invocation patterns
	pythonInvocation = regexp.MustCompile(`\b(python|python3|pip|pip3|pytest)\b`)
)

// isMigrationRequired checks if a path is marked as migration-required in the inventory.
func isMigrationRequired(path string, inventory map[string]*InventoryEntry) bool {
	if entry, ok := inventory[path]; ok {
		return entry.Status == "migration-required"
	}
	return false
}

// checkPythonFiles verifies no repository-owned Python files exist.
func checkPythonFiles(repoRoot string, inventory map[string]*InventoryEntry) []string {
	var errors []string

	err := filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip vendor and third_party
		if strings.Contains(path, "/vendor/") || strings.Contains(path, "/third_party/") || strings.Contains(path, "/__pycache__/") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			return nil
		}

		if strings.HasSuffix(path, ".py") || strings.HasSuffix(path, ".pyw") {
			rel, _ := filepath.Rel(repoRoot, path)
			// In bootstrap mode, Python files marked as migration-required are allowed
			if bootstrapMode && isMigrationRequired(rel, inventory) {
				return nil
			}
			errors = append(errors, fmt.Sprintf("Python file exists: %s", rel))
		}

		return nil
	})

	if err != nil {
		errors = append(errors, fmt.Sprintf("error walking tree: %v", err))
	}

	return errors
}

// checkPythonShebangs verifies no Python shebangs are present.
func checkPythonShebangs(repoRoot string, inventory map[string]*InventoryEntry) []string {
	var errors []string

	scriptsDir := filepath.Join(repoRoot, "scripts")
	entries, err := os.ReadDir(scriptsDir)
	if err != nil {
		return []string{fmt.Sprintf("cannot read scripts dir: %v", err)}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sh") && !strings.HasSuffix(name, ".bash") {
			continue
		}

		path := filepath.Join(scriptsDir, name)
		rel := filepath.Join("scripts", name)

		// In bootstrap mode, shell scripts marked as migration-required are allowed
		if bootstrapMode && isMigrationRequired(rel, inventory) {
			continue
		}

		f, err := os.Open(path)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		if scanner.Scan() {
			line := scanner.Text()
			if pythonShebang.MatchString(line) {
				errors = append(errors, fmt.Sprintf("Python shebang in %s: %s", name, line))
			}
		}
		f.Close()
	}

	return errors
}

// checkPythonInvocations verifies no Python invocations in Makefiles or shell scripts.
func checkPythonInvocations(repoRoot string, inventory map[string]*InventoryEntry) []string {
	var errors []string

	// Check Makefile
	// In bootstrap mode, Makefile Python invocations are expected (migration-required)
	makefilePath := filepath.Join(repoRoot, "Makefile")
	if data, err := os.ReadFile(makefilePath); err == nil {
		if pythonInvocation.Match(data) && !bootstrapMode {
			errors = append(errors, "Makefile invokes Python")
		}
	}

	// Check shell scripts
	scriptsDir := filepath.Join(repoRoot, "scripts")
	err := filepath.Walk(scriptsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".sh") && !strings.HasSuffix(path, ".bash") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		if pythonInvocation.Match(data) {
			rel, _ := filepath.Rel(repoRoot, path)
			// In bootstrap mode, migration-required scripts are allowed
			if bootstrapMode && isMigrationRequired(rel, inventory) {
				return nil
			}
			errors = append(errors, fmt.Sprintf("%s invokes Python", rel))
		}

		return nil
	})

	if err != nil {
		errors = append(errors, fmt.Sprintf("error checking Python invocations: %v", err))
	}

	return errors
}

// countLogicalLOC counts non-blank, non-comment lines in a shell script.
func countLogicalLOC(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	count := 0
	inBlockComment := false
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if line == "" {
			continue
		}

		// Handle block comments
		if strings.HasPrefix(line, "<<") {
			inBlockComment = true
			continue
		}
		if inBlockComment {
			if strings.Contains(line, "EOF") || strings.Contains(line, "'") || strings.Contains(line, "\"") {
				inBlockComment = false
			}
			continue
		}

		// Skip comment-only lines
		if strings.HasPrefix(line, "#") {
			continue
		}

		count++
	}

	return count
}

// checkShellLineCounts verifies shell scripts don't exceed 50 logical LOC.
func checkShellLineCounts(repoRoot string, inventory map[string]*InventoryEntry) []string {
	var errors []string

	scriptsDir := filepath.Join(repoRoot, "scripts")
	err := filepath.Walk(scriptsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".sh") && !strings.HasSuffix(path, ".bash") {
			return nil
		}

		loc := countLogicalLOC(path)
		if loc > maxLogicalLOC {
			rel, _ := filepath.Rel(repoRoot, path)
			// In bootstrap mode, migration-required scripts are allowed
			if bootstrapMode && isMigrationRequired(rel, inventory) {
				return nil
			}
			errors = append(errors, fmt.Sprintf("%s has %d logical LOC (max %d)", rel, loc, maxLogicalLOC))
		}

		return nil
	})

	if err != nil {
		errors = append(errors, fmt.Sprintf("error checking line counts: %v", err))
	}

	return errors
}

// checkInventoryCoverage verifies all shell scripts are listed in the inventory.
func checkInventoryCoverage(repoRoot string, inventory map[string]*InventoryEntry) []string {
	var errors []string
	listedScripts := make(map[string]bool)

	// Get listed scripts from inventory
	for path := range inventory {
		listedScripts[path] = true
	}

	// Check scripts directory
	scriptsDir := filepath.Join(repoRoot, "scripts")
	err := filepath.Walk(scriptsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".sh") && !strings.HasSuffix(path, ".bash") {
			return nil
		}

		rel, _ := filepath.Rel(repoRoot, path)
		if !listedScripts[rel] {
			errors = append(errors, fmt.Sprintf("shell script not in inventory: %s", rel))
		}

		return nil
	})

	if err != nil {
		errors = append(errors, fmt.Sprintf("error checking inventory coverage: %v", err))
	}

	return errors
}

// checkInventoryFilesExist verifies inventory entries reference existing files.
func checkInventoryFilesExist(repoRoot string, inventory map[string]*InventoryEntry) []string {
	var errors []string

	for path, entry := range inventory {
		fullPath := filepath.Join(repoRoot, path)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			errors = append(errors, fmt.Sprintf("inventory entry references nonexistent file: %s", path))
		}
		// Check logical LOC for shell scripts
		if entry.Language == "shell" && entry.LogicalLOC > maxLogicalLOC && entry.Status != "migration-required" {
			errors = append(errors, fmt.Sprintf("%s exceeds max LOC (%d > %d) but status is not migration-required", path, entry.LogicalLOC, maxLogicalLOC))
		}
	}

	return errors
}

// checkMigratedScripts verifies migrated scripts are not still present.
func checkMigratedScripts(repoRoot string, inventory map[string]*InventoryEntry) []string {
	var errors []string

	for path, entry := range inventory {
		if entry.Status == "migrated" {
			fullPath := filepath.Join(repoRoot, path)
			if _, err := os.Stat(fullPath); err == nil {
				errors = append(errors, fmt.Sprintf("script marked migrated but still exists: %s", path))
			}
		}
	}

	return errors
}

// checkShellRiskyPatterns verifies shell wrappers don't contain risky patterns.
func checkShellRiskyPatterns(repoRoot string, inventory map[string]*InventoryEntry) []string {
	var errors []string

	for path, entry := range inventory {
		// Only check shell scripts that are wrappers
		if entry.Language != "shell" {
			continue
		}

		// In bootstrap mode, skip migration-required scripts
		if bootstrapMode && entry.Status == "migration-required" {
			continue
		}

		fullPath := filepath.Join(repoRoot, path)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		content := string(data)
		for _, rp := range riskyPatterns {
			if rp.pattern.MatchString(content) {
				errors = append(errors, fmt.Sprintf("%s contains risky pattern: %s", path, rp.name))
				break
			}
		}
	}

	return errors
}
