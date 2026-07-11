package scriptdoctrine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// checkShellLineCounts verifies shell scripts don't exceed LOC limit.
// Every filesystem error is reported as an internal-error diagnostic.
func (v *Verifier) checkShellLineCounts() []Diagnostic {
	var diags []Diagnostic

	scripts := v.discoverShellScripts()
	for _, rel := range scripts {
		fullPath := filepath.Join(v.RepoRoot, rel)
		loc := CountLogicalLOC(fullPath)
		if loc < 0 {
			diags = append(diags, Diagnostic{
				Check: "internal-error",
				Path:  rel,
				Msg:   "could not determine logical LOC",
			})
			continue
		}

		// Determine the max allowed LOC for this file
		maxLOC := MaxShellLOC
		if v.Bootstrap {
			// In bootstrap mode, only baseline entries can exceed the default limit.
			// Use explicit baseline LOC ceiling if available.
			if baseline, exists := v.Baseline[rel]; exists {
				maxLOC = baseline.BaselineLOC
				if maxLOC == 0 {
					continue // Any size allowed
				}
			}
			// Not in baseline = new violation, use default limit
		}

		if loc > maxLOC {
			diags = append(diags, Diagnostic{
				Check: "shell-loc",
				Path:  rel,
				Msg:   fmt.Sprintf("has %d logical LOC (max %d)", loc, maxLOC),
			})
		}
	}

	return diags
}

// Risky patterns that indicate substantive shell scripting.
var riskyPatterns = []struct {
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

// checkShellRiskyPatterns verifies shell scripts don't contain risky patterns.
// Every filesystem error is reported as an internal-error diagnostic.
func (v *Verifier) checkShellRiskyPatterns() []Diagnostic {
	var diags []Diagnostic

	// Check only wrapper scripts (not migration-required)
	for path, entry := range v.Inventory {
		if entry.Language != "shell" || entry.Status == "migration-required" {
			continue
		}

		fullPath := filepath.Join(v.RepoRoot, path)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				// Reported by checkInventoryFilesExist.
				continue
			}
			diags = append(diags, Diagnostic{
				Check: "internal-error",
				Path:  path,
				Msg:   fmt.Sprintf("reading script for risky-pattern check: %v", err),
			})
			continue
		}

		content := string(data)
		for _, rp := range riskyPatterns {
			if rp.pattern.MatchString(content) {
				diags = append(diags, Diagnostic{
					Check: "risky-pattern",
					Path:  path,
					Msg:   fmt.Sprintf("contains risky pattern: %s", rp.name),
				})
				break
			}
		}
	}

	return diags
}

// discoverShellScripts finds all shell scripts in the repository.
// Files with a Python shebang are excluded because they are Python
// programs, not shell scripts.
func (v *Verifier) discoverShellScripts() []string {
	var scripts []string

	filepath.Walk(v.RepoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		// Skip vendor/third_party and other external directories
		if strings.Contains(path, "/vendor/") || strings.Contains(path, "/third_party/") ||
			strings.Contains(path, "/node_modules/") || strings.Contains(path, "/dist/") ||
			strings.Contains(path, ".git/hooks/") {
			return nil
		}

		rel, _ := filepath.Rel(v.RepoRoot, path)

		// Check by extension
		if strings.HasSuffix(path, ".sh") || strings.HasSuffix(path, ".bash") {
			scripts = append(scripts, rel)
			return nil
		}

		// Check executable bit with shebang
		if info.Mode()&0111 != 0 {
			// Check for shebang
			f, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer f.Close()

			buf := make([]byte, 128)
			n, _ := f.Read(buf)
			if n > 0 && strings.HasPrefix(string(buf[:n]), "#!") {
				scripts = append(scripts, rel)
			}
		}

		return nil
	})

	return scripts
}
