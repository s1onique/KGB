package scriptdoctrine

import (
	"regexp"
	"strings"
)

// sanitizeMakeVars rewrites GNU make's $(VAR) and ${VAR} expansion
// syntax into shell-compatible $VAR references. Make treats $(VAR)
// as a parenthetical expansion while bash rejects bare `(` in command
// position; converting to $VAR keeps the variable lookup shape
// readable for the parser while leaving python command words
// untouched. Function-call forms like $(call foo,bar) are replaced
// with a single placeholder char so the parser doesn't choke on the
// comma inside the parenthetical.
//
// The `$$` literal-dollar escape is preserved as a single `$` so
// `$$(python3 x.py)` becomes `$(python3 x.py)` after sanitisation
// rather than disappearing into a placeholder.
func sanitizeMakeVars(s string) string {
	if !strings.Contains(s, "$(") && !strings.Contains(s, "$$") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		// `$$` is a literal dollar after Make's escape pass.
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '$' {
			b.WriteByte('$')
			i += 2
			continue
		}
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '(' {
			// find matching ')'
			depth := 1
			j := i + 2
			for j < len(s) && depth > 0 {
				switch s[j] {
				case '(':
					depth++
				case ')':
					depth--
				}
				j++
			}
			if depth == 0 {
				inside := s[i+2 : j-1]
				// $(VAR) -> $VAR; $(call x,y) and $(shell …) -> 'X'.
				if strings.ContainsAny(inside, " ,$") ||
					strings.HasPrefix(strings.TrimSpace(inside), "shell ") ||
					strings.HasPrefix(strings.TrimSpace(inside), "shell\t") ||
					strings.TrimSpace(inside) == "shell" {
					b.WriteByte('X')
				} else {
					b.WriteByte('$')
					b.WriteString(inside)
				}
				i = j
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// recipePrefixRx matches `.RECIPEPREFIX = <char>` declarations in
// a Makefile so we can honour non-default recipe prefixes. The
// capture group is the single character that follows `=`.
var recipePrefixRx = regexp.MustCompile(`(?m)^\.RECIPEPREFIX\s*[:?]?=\s*(\S)`)

// makeVariableAssignmentRx matches `NAME := value` and `NAME = value`
// assignments so we can resolve simple Make variables when their
// value is a literal command word.
var makeVariableAssignmentRx = regexp.MustCompile(`(?m)^([A-Za-z_][A-Za-z0-9_]*)\s*[:?]?=\s*(.+?)\s*$`)

// extractMakeVariables returns a map of `NAME -> first value` from a
// Makefile. Multiple assignments to the same variable keep the first
// value. The map is used to resolve simple `$(VAR)` references whose
// value is a literal command word (e.g. `PYTHON := python3`).
func extractMakeVariables(data []byte) map[string]string {
	out := make(map[string]string)
	for _, m := range makeVariableAssignmentRx.FindAllSubmatch(data, -1) {
		name := string(m[1])
		if _, exists := out[name]; exists {
			continue
		}
		out[name] = string(m[2])
	}
	return out
}

// CountPythonInvocationsInMakefile composes three classifiers:
//  1. `$(shell ...)` Make function (expansion-time execution)
//  2. `VAR != RHS` shell-assignment (expansion-time execution)
//  3. TAB-indented recipes (recipe-time execution)
//
// The recipe classifier is fed a copy of the Makefile with
// `$(shell ...)` and `!= RHS` bodies masked so the same Python
// invocation cannot be counted twice. Any parse error from any
// surface propagates as -1 (fail-closed).
//
// Makefiles are not shell programs: variable assignments, includes,
// and conditionals are GNU make syntax. Recipe lines are normally
// TAB-indented, but `.RECIPEPREFIX = <char>` lets a Makefile change
// that character. We honour the declared prefix when present.
//
// Before parsing, the recipe text is sanitised: GNU make's
// `$$(VAR)` literal-dollar escape is preserved, `$(VAR)` and
// `${VAR}` expansions are rewritten as shell-compatible `$VAR`, and
// function-call forms (`$(call ...)`) are replaced with a single
// placeholder char so the parser doesn't choke on the comma inside
// the parenthetical. The placeholder is not a valid python command
// word, so a static analysis can never silently under-count.
//
// Returns -1 if data is nil OR the resulting shell program cannot
// be parsed (fail-closed: malformed recipes should not be silently
// green-lit).
func CountPythonInvocationsInMakefile(data []byte) int {
	if data == nil {
		return -1
	}
	// Expansion-time: $(shell ...) and `!=` shell assignments.
	expansionCount, err := classifyMakeShellExpansions(data)
	if err != nil {
		return -1
	}
	// Recipe-time: TAB-indented recipes over the masked data.
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
		return expansionCount.Count
	}
	recipeCount, err := countPythonSitesInProgram(sanitizeMakeVars(strings.Join(recipes, "\n")))
	if err != nil {
		return -1
	}
	return expansionCount.Count + recipeCount.Count
}

// stripMakePrefix returns the recipe body if line starts with the
// declared recipe prefix (TAB by default, or the single space +
// .RECIPEPREFIX character). The second return is true when the
// line is a recipe.
func stripMakePrefix(line, prefix string) (string, bool) {
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	body := line[len(prefix):]
	body = strings.TrimLeft(body, " \t")
	return body, true
}

// stripMakeSilentPrefix handles `@` and `-` make silent prefixes.
func stripMakeSilentPrefix(body string) (string, bool) {
	if body == "" {
		return "", false
	}
	switch body[0] {
	case '@', '-', '+':
		body = strings.TrimLeft(body[1:], " \t")
	}
	return body, true
}

// sameLineRecipeRx matches `target: ; cmd` and `target: cmd` forms.
var sameLineRecipeRx = regexp.MustCompile(`^[^:=#\t]+:[ \t]+;([ \t]*[^;].*)$`)

// extractSameLineRecipe parses a Makefile line that has recipe
// content on the same line as a target definition. Returns the
// recipe body and true when the line is a same-line recipe; the
// body is the text after the target's `: ` separator.
func extractSameLineRecipe(line string) (string, bool) {
	m := sameLineRecipeRx.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// resolveMakeVars substitutes `$(VAR)` for a known value when the
// value is a literal command word. Unknown variables are left alone
// (the recipe still parses via the shell walker). This lets us
// classify `PYTHON := python3` + `$(PYTHON) hidden.py` as one
// python invocation; variables whose value is dynamic
// (e.g. `:= $(shell ...)`) keep the placeholder and do not
// inflate the count.
func resolveMakeVars(s string, vars map[string]string) string {
	if len(vars) == 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '(' {
			depth := 1
			j := i + 2
			for j < len(s) && depth > 0 {
				switch s[j] {
				case '(':
					depth++
				case ')':
					depth--
				}
				j++
			}
			if depth == 0 {
				inside := s[i+2 : j-1]
				if v, ok := vars[inside]; ok && !strings.ContainsAny(v, " \t") {
					b.WriteString(v)
				} else {
					b.WriteString("$X")
				}
				i = j
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
