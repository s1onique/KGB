package scriptdoctrine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// checkPythonFiles verifies no Python files exist (except in baseline).
// Every filesystem error (walk, stat) is treated as a hard failure.
func (v *Verifier) checkPythonFiles() []Diagnostic {
	var diags []Diagnostic

	err := filepath.Walk(v.RepoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
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

		if filepath.Ext(path) == ".py" || filepath.Ext(path) == ".pyw" {
			rel, _ := filepath.Rel(v.RepoRoot, path)
			if v.isLegacy(rel) {
				return nil
			}
			diags = append(diags, Diagnostic{
				Check: "python-file",
				Path:  rel,
				Msg:   "Python file exists",
			})
		}

		return nil
	})

	if err != nil {
		diags = append(diags, Diagnostic{
			Check: "internal-error",
			Path:  ".",
			Msg:   fmt.Sprintf("walking tree for python files: %v", err),
		})
	}

	return diags
}

// checkPythonShebangs verifies no Python shebangs exist in any file that
// looks like a script. Python shebang detection does NOT depend on the
// executable bit - any file (executable or not) whose first line is a
// Python shebang is reported.
//
// The scan walks every regular file in the repository (subject to the
// standard skip list) and reports those whose first line is a Python
// shebang. Each read or walk error is treated as a hard failure.
func (v *Verifier) checkPythonShebangs() []Diagnostic {
	var diags []Diagnostic

	err := filepath.Walk(v.RepoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
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
		if v.isLegacy(rel) {
			return nil
		}

		// Python shebang detection is independent of the executable bit.
		hasShebang, err := HasPythonShebang(path)
		if err != nil {
			diags = append(diags, Diagnostic{
				Check: "internal-error",
				Path:  rel,
				Msg:   fmt.Sprintf("reading file for shebang check: %v", err),
			})
			return nil
		}
		if hasShebang {
			diags = append(diags, Diagnostic{
				Check: "python-shebang",
				Path:  rel,
				Msg:   "Python shebang detected",
			})
		}

		return nil
	})

	if err != nil {
		diags = append(diags, Diagnostic{
			Check: "internal-error",
			Path:  ".",
			Msg:   fmt.Sprintf("walking tree for python shebangs: %v", err),
		})
	}

	return diags
}

// checkPythonInvocations verifies no Python invocations exist in Makefiles,
// CI workflows, Git hooks, or shell scripts. Every read error is reported
// as a hard failure.
func (v *Verifier) checkPythonInvocations() []Diagnostic {
	var diags []Diagnostic

	// Check root Makefile.
	makefilePath := filepath.Join(v.RepoRoot, "Makefile")
	data, err := os.ReadFile(makefilePath)
	if err != nil && !os.IsNotExist(err) {
		diags = append(diags, Diagnostic{
			Check: "internal-error",
			Path:  "Makefile",
			Msg:   fmt.Sprintf("reading Makefile: %v", err),
		})
	} else if err == nil {
		if hasPython, count := scanPython(makefilePath, data); hasPython {
			if !v.isLegacy("Makefile") {
				if count > 0 {
					diags = append(diags, Diagnostic{
						Check: "python-invocation",
						Path:  "Makefile",
						Msg:   fmt.Sprintf("Makefile invokes Python (%d site(s))", count),
					})
				} else {
					diags = append(diags, Diagnostic{
						Check: "internal-error",
						Path:  "Makefile",
						Msg:   "Makefile Python count could not be determined",
					})
				}
			}
		}
	}

	// Check nested Makefiles.
	v.walkMakefiles(v.RepoRoot, &diags)

	// Check CI workflows.
	v.walkCIWorkflows(v.RepoRoot, &diags)

	// Check Git hooks.
	v.walkGitHooks(v.RepoRoot, &diags)

	// Check all shell scripts.
	scripts, discoverDiags := v.discoverShellScripts()
	diags = append(diags, discoverDiags...)
	for _, rel := range scripts {
		fullPath := filepath.Join(v.RepoRoot, rel)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			diags = append(diags, Diagnostic{
				Check: "internal-error",
				Path:  rel,
				Msg:   fmt.Sprintf("reading script: %v", err),
			})
			continue
		}

		if hasPython, count := scanPython(fullPath, data); hasPython {
			if v.isLegacy(rel) {
				continue
			}
			if count > 0 {
				diags = append(diags, Diagnostic{
					Check: "python-invocation",
					Path:  rel,
					Msg:   fmt.Sprintf("Script invokes Python (%d site(s))", count),
				})
			} else {
				diags = append(diags, Diagnostic{
					Check: "internal-error",
					Path:  rel,
					Msg:   "script Python count could not be determined",
				})
			}
		}
	}

	return diags
}

// scanPython returns whether content has any Python invocation and the
// unique site count. The dispatch is path-aware: Makefiles and
// `.github/workflows/*.yml` files go through their dedicated
// extractors; everything else goes through the whole-file AST
// parse. Any path that the chosen extractor cannot parse (a
// malformed recipe block, a malformed `run:` block, etc.) yields
// (true, 0) so the caller can surface an internal-error diagnostic.
func scanPython(path string, data []byte) (bool, int) {
	count := CountPythonInvocationsForPath(path, data)
	if count < 0 {
		return true, 0
	}
	return count > 0, count
}

func (v *Verifier) walkMakefiles(root string, diags *[]Diagnostic) {
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			*diags = append(*diags, Diagnostic{
				Check: "internal-error",
				Path:  path,
				Msg:   fmt.Sprintf("walking for Makefiles: %v", err),
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

		name := info.Name()
		if name != "Makefile" && filepath.Ext(name) != ".mk" {
			return nil
		}

		rel, _ := filepath.Rel(v.RepoRoot, path)
		if v.isLegacy(rel) {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			*diags = append(*diags, Diagnostic{
				Check: "internal-error",
				Path:  rel,
				Msg:   fmt.Sprintf("reading Makefile: %v", err),
			})
			return nil
		}

		if hasPython, count := scanPython(path, data); hasPython {
			if count > 0 {
				*diags = append(*diags, Diagnostic{
					Check: "python-invocation",
					Path:  rel,
					Msg:   fmt.Sprintf("Makefile invokes Python (%d site(s))", count),
				})
			} else {
				*diags = append(*diags, Diagnostic{
					Check: "internal-error",
					Path:  rel,
					Msg:   "Makefile Python count could not be determined",
				})
			}
		}

		return nil
	})
	// Fail closed: a top-level walk error (e.g. permission denied on the
	// repo root) must surface as an internal-error diagnostic, not be
	// silently swallowed.
	if walkErr != nil {
		*diags = append(*diags, Diagnostic{
			Check: "internal-error",
			Path:  ".",
			Msg:   fmt.Sprintf("walk for Makefiles: %v", walkErr),
		})
	}
}

func (v *Verifier) walkCIWorkflows(root string, diags *[]Diagnostic) {
	workflowsDir := filepath.Join(root, ".github", "workflows")
	entries, err := os.ReadDir(workflowsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		*diags = append(*diags, Diagnostic{
			Check: "internal-error",
			Path:  ".github/workflows",
			Msg:   fmt.Sprintf("reading workflows dir: %v", err),
		})
		return
	}

	for _, entry := range entries {
if entry.IsDir() || strings.HasSuffix(entry.Name(), ".sample") {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".yml" && filepath.Ext(name) != ".yaml" {
			continue
		}

		rel := filepath.Join(".github", "workflows", name)
		if v.isLegacy(rel) {
			continue
		}

		fullPath := filepath.Join(workflowsDir, name)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			*diags = append(*diags, Diagnostic{
				Check: "internal-error",
				Path:  rel,
				Msg:   fmt.Sprintf("reading workflow: %v", err),
			})
			continue
		}

		if hasPython, count := scanPython(fullPath, data); hasPython {
			if count > 0 {
				*diags = append(*diags, Diagnostic{
					Check: "python-invocation",
					Path:  rel,
					Msg:   fmt.Sprintf("CI workflow invokes Python (%d site(s))", count),
				})
			} else {
				*diags = append(*diags, Diagnostic{
					Check: "internal-error",
					Path:  rel,
					Msg:   "workflow Python count could not be determined",
				})
			}
		}
	}
}

func (v *Verifier) walkGitHooks(root string, diags *[]Diagnostic) {
	hooksDir := filepath.Join(root, ".git", "hooks")
	entries, err := os.ReadDir(hooksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		*diags = append(*diags, Diagnostic{
			Check: "internal-error",
			Path:  ".git/hooks",
			Msg:   fmt.Sprintf("reading git hooks dir: %v", err),
		})
		return
	}

	for _, entry := range entries {
if entry.IsDir() || strings.HasSuffix(entry.Name(), ".sample") {
			continue
		}

		rel := filepath.Join(".git", "hooks", entry.Name())
		if v.isLegacy(rel) {
			continue
		}

		fullPath := filepath.Join(hooksDir, entry.Name())
		data, err := os.ReadFile(fullPath)
		if err != nil {
			*diags = append(*diags, Diagnostic{
				Check: "internal-error",
				Path:  rel,
				Msg:   fmt.Sprintf("reading git hook: %v", err),
			})
			continue
		}

		if hasPython, count := scanPython(fullPath, data); hasPython {
			if count > 0 {
				*diags = append(*diags, Diagnostic{
					Check: "python-invocation",
					Path:  rel,
					Msg:   fmt.Sprintf("Git hook invokes Python (%d site(s))", count),
				})
			} else {
				*diags = append(*diags, Diagnostic{
					Check: "internal-error",
					Path:  rel,
					Msg:   "git hook Python count could not be determined",
				})
			}
		}
	}
}
