// R14/R15 closure: typed *ClassificationError-aware Makefile
// extractor with line-aware comment masking.
//
// This file was carved out of shell_make_extractors_r11.go when
// the typed helper (R14) and the line-aware comment masker (R15)
// pushed that file past the LLM-friendliness 450-line hard
// limit. The dispatcher in shell_analysis.go
// (CountPythonInvocationsForPathDetailed) routes Makefile
// inputs to CountPythonInvocationsInMakefileDetailed below.
package scriptdoctrine

import (
	"strings"
)

// CountPythonInvocationsInMakefileDetailed is the typed twin of
// CountPythonInvocationsInMakefile. It returns the structured
// InvocationCount that aggregates BOTH expansion-time
// `$(shell ...)` AND recipe-time (TAB-indented `python3 ...`)
// sites, plus an error. The error chain preserves
// *ClassificationError so the verifier's Diagnostic carries the
// original line/column on a fail-closed surface.
//
// R14 closure: the previous dispatcher in
// CountPythonInvocationsForPathDetailed dispatched to
// classifyMakeShellExpansions alone, which undercounted Makefile
// sites by the recipe-time delta. The bootstrap-baseline gate
// reads this API directly, so an undercount would surface as
// spurious baseline-python-invocation-changed diagnostics. The
// new function aggregates the recipe-time pass after the
// expansion-time pass so the total matches the int-API contract
// callers expect.
func CountPythonInvocationsInMakefileDetailed(data []byte) (InvocationCount, error) {
	if data == nil {
		return ZeroCount, nil
	}
	expansion, err := classifyMakeShellExpansions(data)
	if err != nil {
		return ZeroCount, err
	}
	prefix := "\t"
	if m := recipePrefixRx.FindSubmatch(data); m != nil {
		prefix = string(m[1])
	}
	cleaned := maskMakeComments(data)
	masked := maskMakeExpansionSites(cleaned)
	vars := extractMakeVariables(masked)

	var recipes []string
	for _, line := range strings.Split(string(masked), "\n") {
		body, ok := stripMakePrefix(line, prefix)
		if !ok {
			continue
		}
		body, ok = stripMakeSilentPrefix(body)
		if !ok {
			continue
		}
		body = resolveMakeVars(body, vars)
		recipes = append(recipes, body)
	}
	for _, line := range strings.Split(string(masked), "\n") {
		if body, ok := extractSameLineRecipe(line); ok {
			body = resolveMakeVars(body, vars)
			recipes = append(recipes, body)
		}
	}
	if len(recipes) == 0 {
		return expansion, nil
	}
	recipeCount, err := countPythonSitesInProgram(sanitizeMakeVars(strings.Join(recipes, "\n")))
	if err != nil {
		return ZeroCount, wrapShellParseError("Makefile", err)
	}
	return InvocationCount{Count: expansion.Count + recipeCount.Count}, nil
}

// classifyMakeShellExpansions scans a Makefile for `$(shell ...)`
// and `!= RHS` execution sites, classifies each as a complete
// shell program, and returns the total Python invocation count.
// A non-nil error means one site was dynamic or malformed: the
// caller must surface an internal-error diagnostic.
//
// R15: the input is first run through maskMakeComments so
// ordinary Make comments (column-one or indented, `\#`-escaped
// or trailing inline) cannot inflate the Python count. Recipe
// lines (TAB or `.RECIPEPREFIX` prefix) are left intact.
func classifyMakeShellExpansions(data []byte) (InvocationCount, error) {
	cleaned := maskMakeComments(data)
	if bad := findUnbalancedMakeParens(cleaned); len(bad) > 0 {
		return ZeroCount, NewClassificationError(
			"", lineOf(cleaned, bad[0]), 1, "unbalanced GNU Make shell function")
	}
	functions := findShellFunctionSites(cleaned)
	assignments := findShellAssignmentSites(cleaned)
	// Vars are extracted from the un-masked data because a
	// `PYTHON := python3` never lives inside a comment or an
	// `$(shell ...)` body, so the mask cannot hide a variable.
	vars := extractMakeVariables(data)
	resolve := func(s string) string { return resolveMakeVars(s, vars) }
	total := 0
	for _, fn := range functions {
		if fn.InnerStart >= fn.InnerEnd {
			continue
		}
		script := strings.TrimSpace(string(cleaned[fn.InnerStart:fn.InnerEnd]))
		if script == "" {
			continue
		}
		if countUnresolvedMakeRefsInCommand(script, vars) > 0 {
			return ZeroCount, NewClassificationError(
				"", int(lineOf(cleaned, fn.Start)), 1, "dynamic GNU Make shell command")
		}
		script = preProcessMakeShell(resolve(script))
		count, err := countPythonSitesInProgram(script)
		if err != nil {
			return ZeroCount, err
		}
		total += count.Count
	}
	for _, as := range assignments {
		if as.RHSStart >= as.RHSEnd {
			continue
		}
		rhs := strings.TrimSpace(string(cleaned[as.RHSStart:as.RHSEnd]))
		if rhs == "" {
			continue
		}
		if countUnresolvedMakeRefsInCommand(rhs, vars) > 0 {
			return ZeroCount, NewClassificationError(
				"", int(lineOf(cleaned, as.RHSStart)), 1, "dynamic GNU Make shell command")
		}
		rhs = preProcessMakeShell(resolve(rhs))
		count, err := countPythonSitesInProgram(rhs)
		if err != nil {
			return ZeroCount, err
		}
		total += count.Count
	}
	return InvocationCount{Count: total}, nil
}
