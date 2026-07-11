// Package scriptdoctrine: typed *ClassificationError-aware Makefile
// extractor.
//
// This file was carved out of shell_make_extractors_r11.go when the
// typed helper pushed that file past the LLM-friendliness 450-line
// hard limit. The dispatcher in shell_analysis.go
// (CountPythonInvocationsForPathDetailed) routes Makefile inputs to
// CountPythonInvocationsInMakefileDetailed below.
package scriptdoctrine

import (
	"strings"
)

// CountPythonInvocationsInMakefileDetailed is the typed twin of
// CountPythonInvocationsInMakefile. It returns the structured
// InvocationCount that aggregates BOTH expansion-time `$(shell ...)`
// AND recipe-time (TAB-indented `python3 ...`) sites, plus an
// error. The error chain preserves *ClassificationError so the
// verifier's Diagnostic carries the original line/column on a
// fail-closed surface.
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
	masked := maskMakeExpansionSites(data)
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
