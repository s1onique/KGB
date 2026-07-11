package scriptdoctrine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// checkShellLineCounts verifies shell scripts don't exceed LOC limit.
// Every filesystem error is reported as an internal-error diagnostic.
func (v *Verifier) checkShellLineCounts() []Diagnostic {
	var diags []Diagnostic

	scripts, discoverDiags := v.discoverShellScripts()
	diags = append(diags, discoverDiags...)
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

		maxLOC := MaxShellLOC
		if v.Bootstrap {
			if baseline, exists := v.Baseline[rel]; exists {
				maxLOC = baseline.BaselineLOC
				if maxLOC == 0 {
					continue
				}
			}
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

	for path, entry := range v.Inventory {
		if entry.Language != "shell" || entry.Status == "migration-required" {
			continue
		}

		fullPath := filepath.Join(v.RepoRoot, path)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
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
// Returns the list of script paths plus any diagnostics that surfaced
// during the walk - the caller MUST propagate the diagnostics, never
// swallow them. This is the fail-closed contract: walk errors and read
// errors are surfaced to the user.
func (v *Verifier) discoverShellScripts() ([]string, []Diagnostic) {
	var scripts []string
	var diags []Diagnostic

	err := filepath.Walk(v.RepoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			diags = append(diags, Diagnostic{
				Check: "internal-error",
				Path:  path,
				Msg:   fmt.Sprintf("walking for shell scripts: %v", err),
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

		if filepath.Ext(path) == ".sh" || filepath.Ext(path) == ".bash" {
			scripts = append(scripts, rel)
			return nil
		}

		if info.Mode()&0111 != 0 {
			f, err := os.Open(path)
			if err != nil {
				diags = append(diags, Diagnostic{
					Check: "internal-error",
					Path:  rel,
					Msg:   fmt.Sprintf("opening file for shebang sniff: %v", err),
				})
				return nil
			}
			defer f.Close()

			buf := make([]byte, 128)
			n, err := f.Read(buf)
			if err != nil && n == 0 {
				diags = append(diags, Diagnostic{
					Check: "internal-error",
					Path:  rel,
					Msg:   fmt.Sprintf("reading file for shebang sniff: %v", err),
				})
				return nil
			}
			if n > 0 && string(buf[:n])[:min(n, 2)] == "#!" {
				scripts = append(scripts, rel)
			}
		}

		return nil
	})

	if err != nil {
		diags = append(diags, Diagnostic{
			Check: "internal-error",
			Path:  ".",
			Msg:   fmt.Sprintf("walk for shell scripts: %v", err),
		})
	}

	return scripts, diags
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
