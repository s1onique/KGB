package scriptdoctrine

import (
	"strings"
)

// GNU Make expansion-time execution sites (R11.4/R12/R13).
// `$(shell X)` and `VAR != RHS` are scanned; the recipe parser
// receives a masked copy so a single Python invocation counts
// once.

// shellFunctionRange represents a Make expansion-time execution
// site found in the source bytes. Start/End are the bytes of the
// whole `$(shell ...)`; InnerStart/InnerEnd delimit the command
// argument (i.e. the bytes inside the parens, minus the function
// name `shell`).
type shellFunctionRange struct {
	Start, End int
	InnerStart int
	InnerEnd   int
	Line       int
}

// shellAssignmentRange represents a Make `VAR != RHS` site.
type shellAssignmentRange struct{ RHSStart, RHSEnd, Line int }

// findShellFunctionSites scans a Makefile for balanced
// `$(shell ...)` expansions and returns them in source order.
// The returned slice is sorted by Start offset; callers use the
// offsets to mask the original bytes with a placeholder.
func findShellFunctionSites(data []byte) []shellFunctionRange {
	var out []shellFunctionRange
	i := 0
	for i < len(data) {
		// Skip GNU Make comments: `#` at start of a logical
		// line. TAB/space-prefixed `#` is recipe text and
		// must still expand `$(shell ...)` (R14).
		if data[i] == '#' && (i == 0 || data[i-1] == '\n') {
			for i < len(data) && data[i] != '\n' {
				i++
			}
			continue
		}
		// Skip $$ literal-dollar escapes.
		if i+1 < len(data) && data[i] == '$' && data[i+1] == '$' {
			i += 2
			continue
		}
		// Match $( ... ) with possible whitespace before the
		// function name `shell`.
		if i+1 < len(data) && data[i] == '$' && data[i+1] == '(' {
			depth := 1
			j := i + 2
			for j < len(data) && depth > 0 {
				switch data[j] {
				case '(':
					depth++
				case ')':
					depth--
				}
				j++
			}
			if depth != 0 {
				break
			}
			inner := string(data[i+2 : j-1])
			trim := strings.TrimLeft(inner, " \t")
			if trim == "shell" {
				out = append(out, shellFunctionRange{
					Start: i, End: j,
					InnerStart: j - 1, InnerEnd: j - 1,
					Line: lineOf(data, i),
				})
				i = j
				continue
			}
			if strings.HasPrefix(trim, "shell") && len(trim) > len("shell") {
				next := trim[len("shell")]
				if next == ' ' || next == '\t' {
					rest := strings.TrimLeft(trim[len("shell"):], " \t")
					innerStart := j - 1 - len(rest)
					if innerStart < i+2 {
						innerStart = i + 2
					}
					out = append(out, shellFunctionRange{
						Start: i, End: j,
						InnerStart: innerStart, InnerEnd: j - 1,
						Line: lineOf(data, i),
					})
					i = j
					continue
				}
			}
			// Not a $(shell ... ) expansion; skip past the `$`.
			i++
			continue
		}
		i++
	}
	return out
}

// findShellAssignmentSites scans a Makefile for `NAME != RHS`
// lines and returns the RHS offsets. Continuation lines and
// recipe lines (TAB-prefixed) are NOT shell assignments; only
// assignment statements at the top level are recognised.
func findShellAssignmentSites(data []byte) []shellAssignmentRange {
	var out []shellAssignmentRange
	for offset := 0; offset < len(data); {
		nl := -1
		for k := offset; k < len(data); k++ {
			if data[k] == '\n' {
				nl = k
				break
			}
		}
		var line []byte
		var nextOffset int
		if nl < 0 {
			line = data[offset:]
			nextOffset = len(data)
		} else {
			line = data[offset:nl]
			nextOffset = nl + 1
		}
		if len(line) > 0 && line[0] != '\t' {
			trimmed := strings.TrimLeft(string(line), " \t")
			if trimmed == "" || trimmed[0] == '#' {
				offset = nextOffset
				continue
			}
			if trimmed != "" {
				k := 0
				for k < len(trimmed) {
					if k+1 < len(trimmed) && trimmed[k] == '!' && trimmed[k+1] == '=' {
						rhs := strings.TrimLeft(trimmed[k+2:], " \t")
						leadingGap := len(trimmed[k+2:]) - len(rhs)
						rhsStart := offset + len(line) - len(trimmed) + k + 2 + leadingGap
						rhsEnd := offset + len(line)
						out = append(out, shellAssignmentRange{
							RHSStart: rhsStart,
							RHSEnd:   rhsEnd,
							Line:     lineOf(data, offset),
						})
						break
					}
					if k+1 < len(trimmed) && trimmed[k] == '$' && trimmed[k+1] == '(' {
						depth := 1
						kk := k + 2
						for kk < len(trimmed) && depth > 0 {
							switch trimmed[kk] {
							case '(':
								depth++
							case ')':
								depth--
							}
							kk++
						}
						k = kk
						continue
					}
					k++
				}
			}
		}
		offset = nextOffset
	}
	return out
}

// classifyMakeShellExpansions scans a Makefile for `20 20 101 12 61 79 80 81 98 701 33 100 204 250 395 398 399 400shell ...)`
// and `!= RHS` execution sites, classifies each as a complete
// shell program, and returns the total Python invocation count.
// A non-nil error means one site was dynamic or malformed: the
// caller must surface an internal-error diagnostic.
func classifyMakeShellExpansions(data []byte) (InvocationCount, error) {
	// R11.4 fail-closed: an unbalanced `20 20 101 12 61 79 80 81 98 701 33 100 204 250 395 398 399 400` is a hard error.
	if bad := findUnbalancedMakeParens(data); len(bad) > 0 {
		return ZeroCount, NewClassificationError(
			"", lineOf(data, bad[0]), 1, "unbalanced GNU Make shell function")
	}
	functions := findShellFunctionSites(data)
	assignments := findShellAssignmentSites(data)
	// R11.4 Make-variable resolution: PYTHON := python3 + body
	// `20 20 101 12 61 79 80 81 98 701 33 100 204 250 395 398 399 400shell 20 20 101 12 61 79 80 81 98 701 33 100 204 250 395 398 399 400PYTHON) x.py)` resolves to a single python
	// invocation. We only resolve simple `NAME := value`
	// assignments whose value is a single shell-safe word.
	vars := extractMakeVariables(data)
	resolve := func(s string) string { return resolveMakeVars(s, vars) }
	total := 0
	for _, fn := range functions {
		if fn.InnerStart >= fn.InnerEnd {
			continue
		}
		script := strings.TrimSpace(string(data[fn.InnerStart:fn.InnerEnd]))
		if script == "" {
			continue
		}
		if countUnresolvedMakeRefsInCommand(script, vars) > 0 {
			return ZeroCount, NewClassificationError(
				"", int(lineOf(data, fn.Start)), 1, "dynamic GNU Make shell command")
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
		rhs := strings.TrimSpace(string(data[as.RHSStart:as.RHSEnd]))
		if rhs == "" {
			continue
		}
		if countUnresolvedMakeRefsInCommand(rhs, vars) > 0 {
			return ZeroCount, NewClassificationError(
				"", int(lineOf(data, as.RHSStart)), 1, "dynamic GNU Make shell command")
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

// maskMakeExpansionSites replaces the contents of `$(shell ...)`
// expansions and `!= RHS` bodies with whitespace so the recipe
// scanner cannot double-count a site that the expansion-time
// classifier already saw. The placeholder char is ` ` (space);
// the recipe walker ignores spaces within a Make variable
// substitution anyway.
func maskMakeExpansionSites(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	functions := findShellFunctionSites(data)
	assignments := findShellAssignmentSites(data)
	type mask struct{ start, end int }
	masks := make([]mask, 0, len(functions)+len(assignments))
	for _, fn := range functions {
		masks = append(masks, mask{fn.Start, fn.End})
	}
	for _, as := range assignments {
		masks = append(masks, mask{as.RHSStart, as.RHSEnd})
	}
	if len(masks) == 0 {
		return data
	}
	// Sort by start offset (descending) so we mutate from the
	// end without invalidating later offsets.
	for i := 0; i < len(masks); i++ {
		for j := i + 1; j < len(masks); j++ {
			if masks[j].start < masks[i].start {
				masks[i], masks[j] = masks[j], masks[i]
			}
		}
	}
	out := append([]byte(nil), data...)
	for _, m := range masks {
		start := m.start
		end := m.end
		if start < 0 {
			start = 0
		}
		if end > len(out) {
			end = len(out)
		}
		for k := start; k < end; k++ {
			if k >= len(out) {
				break
			}
			out[k] = ' '
		}
	}
	return out
}

// lineOf returns the 1-based line number of the byte at offset in
// data. Used to surface precise diagnostics for the expansion-time
// sites.
func lineOf(data []byte, offset int) int {
	if offset < 0 {
		return 1
	}
	if offset > len(data) {
		offset = len(data)
	}
	line := 1
	for i := 0; i < offset; i++ {
		if data[i] == '\n' {
			line++
		}
	}
	return line
}

// preProcessMakeShell applies GNU Make expansion semantics to a
// script body so the result is statically classifiable by
// mvdan.cc/sh. Make expands `$$` -> `$` and `$(VAR)` -> value
// before invoking the shell; substituting those references with
// a numeric placeholder here produces a fragment that parses as
// a shell program and contains the same Python calls (if any).
//
// We do not attempt to honour specific Make variable values; the
// script's *invocation* of a Python interpreter is what policy
// cares about, and the identifier substitutions cannot introduce
// a new invocation site. Numeric placeholders keep the shell
// arithmetic parser happy for inputs like
// `echo $(( $(ATTRIBUTE) / 60 ))`.
func preProcessMakeShell(script string) string {
	if !strings.Contains(script, "$") {
		return script
	}
	var b strings.Builder
	b.Grow(len(script))
	i := 0
	for i < len(script) {
		if i+1 < len(script) && script[i] == '$' && script[i+1] == '$' {
			b.WriteByte('$')
			i += 2
			continue
		}
		if i+1 < len(script) && script[i] == '$' && script[i+1] == '(' {
			depth := 1
			j := i + 2
			for j < len(script) && depth > 0 {
				switch script[j] {
				case '(':
					depth++
				case ')':
					depth--
				}
				j++
			}
			if depth == 0 {
				b.WriteString("1")
				i = j
				continue
			}
		}
		b.WriteByte(script[i])
		i++
	}
	return b.String()
}

// findUnbalancedMakeParens scans data for `$(` tokens whose
// matching `)` is missing, and returns the byte offset of each
// offending `$(`. The expansion-time detector uses this to
// surface a hard error rather than silently counting the partial
// site as zero.
func findUnbalancedMakeParens(data []byte) []int {
	var bad []int
	depth := 0
	i := 0
	for i < len(data) {
		if i+1 < len(data) && data[i] == '$' && data[i+1] == '$' {
			i += 2
			continue
		}
		if depth == 0 && i+1 < len(data) && data[i] == '$' && data[i+1] == '(' {
			// Possibly an unbalanced $(...); look ahead for
			// the matching ). If we never find it, this is a
			// candidate.
			depth = 1
			j := i + 2
			for j < len(data) && depth > 0 {
				switch data[j] {
				case '(':
					if depth > 0 {
						depth++
					}
				case ')':
					depth--
				}
				j++
			}
			if depth != 0 {
				bad = append(bad, i)
			}
			depth = 0
			i = j
			continue
		}
		i++
	}
	return bad
}

// countUnresolvedMakeRefsInCommand counts `$(VAR)` and
// `${VAR}` references that are NOT pre-recognised Make
// function calls (`shell`). R12/R13 fail-closed gate.
func countUnresolvedMakeRefsInCommand(s string, vars map[string]string) int {
	unresolved := 0
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '$' && s[i+1] == '$' {
			i += 2
			continue
		}
		if s[i] != '$' || i+1 >= len(s) {
			i++
			continue
		}
		openCh := byte(0)
		closeCh := byte(0)
		switch s[i+1] {
		case '(':
			openCh, closeCh = '(', ')'
		case '{':
			openCh, closeCh = '{', '}'
		default:
			i++
			continue
		}
		depth := 1
		j := i + 2
		for j < len(s) && depth > 0 {
			if s[j] == openCh {
				depth++
			} else if s[j] == closeCh {
				depth--
			}
			j++
		}
		if depth == 0 {
			trimmed := strings.TrimSpace(s[i+2 : j-1])
			if trimmed == "shell" || strings.HasPrefix(trimmed, "shell ") {
				i = j
				continue
			}
			if _, ok := vars[trimmed]; !ok {
				unresolved++
			}
			i = j
			continue
		}
		i++
	}
	return unresolved
}
