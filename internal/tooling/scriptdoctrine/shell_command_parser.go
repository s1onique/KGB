package scriptdoctrine

import (
	"strings"
)

// countPythonInvocationsInLine returns the number of executable Python
// command sites in a single shell line.
//
// A "command site" is one top-level command after splitting the line by
// command-list separators (;, &&, ||, |, &). The metric counts Python
// invocations that would actually run, not Python mentioned in arguments,
// documentation, or lookups.
//
// Recursion handles command substitution $() and backticks, so a python
// invocation inside printf '%s' "$(python3 ...)" still counts.
func countPythonInvocationsInLine(line string) int {
	return countInCommandList(line)
}

func countInCommandList(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// Recurse into command substitution first; extracted text is parsed
	// independently and counted here.
	remaining, subCount := extractAndCountSubstitutions(s)
	total := subCount

	for _, piece := range splitShellList(remaining) {
		piece = strings.TrimSpace(piece)
		if piece == "" {
			continue
		}
		// Pure variable assignment lines never run (unless they contain
		// a substitution, which we already counted above).
		if isPureAssignment(piece) {
			continue
		}
		if invokesPython(piece) {
			total++
		}
	}
	return total
}

// extractAndCountSubstitutions removes every $(...) and backtick span
// from s, recursively counts invocations inside each span, and returns
// the stripped string plus the recursive total.
func extractAndCountSubstitutions(s string) (string, int) {
	total := 0

	// $( ... ) substitutions (with nested paren handling).
	for {
		start := strings.Index(s, "$(")
		if start < 0 {
			break
		}
		end, ok := findMatchingParen(s, start+2)
		if !ok {
			// Unbalanced - stop trying; the surrounding parser will
			// surface a more useful diagnostic on the file level.
			break
		}
		inner := s[start+2 : end]
		total += countInCommandList(inner)
		// Replace the substitution with a single space so downstream
		// splitter doesn't see unbalanced parens.
		s = s[:start] + " " + s[end+1:]
	}

	// ` ... ` backtick substitutions.
	for {
		start := strings.Index(s, "`")
		if start < 0 {
			break
		}
		end := -1
		for i := start + 1; i < len(s); i++ {
			if s[i] == '`' {
				end = i
				break
			}
		}
		if end < 0 {
			break
		}
		inner := s[start+1 : end]
		total += countInCommandList(inner)
		s = s[:start] + " " + s[end+1:]
	}

	return s, total
}

// findMatchingParen returns the index of the ')' matching the '(' at
// openIdx-1 (one position before the open paren content).
func findMatchingParen(s string, openIdx int) (int, bool) {
	depth := 1
	for i := openIdx; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return -1, false
}

// splitShellList splits a shell line by the top-level command-list
// separators (;, &&, ||, |, &) while respecting single/double quotes and
// paren / brace nesting. The result is the list of individual command
// operands, in order, without their separators. Each piece is trimmed of
// leading and trailing whitespace.
func splitShellList(s string) []string {
	var pieces []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	parenDepth := 0
	braceDepth := 0

	flush := func() {
		pieces = append(pieces, strings.TrimSpace(current.String()))
		current.Reset()
	}

	for i := 0; i < len(s); i++ {
		c := s[i]

		// Quote handling.
		if !inDouble && c == '\'' {
			inSingle = !inSingle
			current.WriteByte(c)
			continue
		}
		if !inSingle && c == '"' {
			inDouble = !inDouble
			current.WriteByte(c)
			continue
		}
		if inSingle || inDouble {
			current.WriteByte(c)
			continue
		}

		// Nesting depth.
		if c == '(' {
			parenDepth++
			current.WriteByte(c)
			continue
		}
		if c == ')' {
			if parenDepth > 0 {
				parenDepth--
			}
			current.WriteByte(c)
			continue
		}
		if c == '{' {
			braceDepth++
			current.WriteByte(c)
			continue
		}
		if c == '}' {
			if braceDepth > 0 {
				braceDepth--
			}
			current.WriteByte(c)
			continue
		}

		// Top-level separators only.
		if parenDepth > 0 || braceDepth > 0 {
			current.WriteByte(c)
			continue
		}

		switch c {
		case ';':
			flush()
		case '&':
			if i+1 < len(s) && s[i+1] == '&' {
				flush()
				i++ // consume second '&'
			} else {
				flush() // single '&' is backgrounding
			}
		case '|':
			if i+1 < len(s) && s[i+1] == '|' {
				flush()
				i++ // consume second '|'
			} else {
				flush() // single '|' is a pipe
			}
		default:
			current.WriteByte(c)
		}
	}

	if current.Len() > 0 {
		flush()
	}
	return pieces
}

// isPureAssignment reports whether the line is a plain variable assignment
// without any command substitution that would execute code.
func isPureAssignment(s string) bool {
	if !variableAssignRx.MatchString(s) {
		return false
	}
	return !strings.Contains(s, "$(") && !strings.Contains(s, "`")
}

// invokesPython reports whether the given (single) command operand runs
// Python. Lookups like `command -v python3` and `command --version` are
// NOT counted - they only inspect, they do not run.
func invokesPython(cmd string) bool {
	fields := shellFields(cmd)
	if len(fields) == 0 {
		return false
	}

	i := 0
	// Strip optional prefix commands.
	for i < len(fields) && isCommandPrefix(fields[i]) {
		// `command -flag ...` is a lookup, not an invocation. The
		// remainder of the line is arguments to the lookup, not a
		// separate command.
		if fields[i] == "command" && i+1 < len(fields) && strings.HasPrefix(fields[i+1], "-") {
			return false
		}
		i++
	}

	if i >= len(fields) {
		return false
	}
	return isPythonCommandWord(fields[i])
}

// isCommandPrefix reports whether the given word is one of the recognized
// command prefixes (sudo, env, /usr/bin/env, exec, command).
func isCommandPrefix(word string) bool {
	switch word {
	case "sudo", "env", "/usr/bin/env", "exec", "command":
		return true
	}
	return false
}

// isPythonCommandWord reports whether word, after stripping any leading
// path and trailing .py extension, is a Python interpreter or Python
// tool name.
func isPythonCommandWord(word string) bool {
	// Strip leading path.
	if idx := strings.LastIndex(word, "/"); idx >= 0 {
		word = word[idx+1:]
	}
	// Strip trailing ".py" extension (rare but legal: `python3.py`).
	if strings.HasSuffix(word, ".py") {
		word = strings.TrimSuffix(word, ".py")
	}
	switch word {
	case "python", "pip", "pytest":
		return true
	}
	// python3, python3.10, pip3, etc.
	if strings.HasPrefix(word, "python") && len(word) > len("python") {
		rest := word[len("python"):]
		if rest[0] >= '0' && rest[0] <= '9' {
			// Optional .N.M version suffix.
			for _, r := range rest {
				if r != '.' && (r < '0' || r > '9') {
					return false
				}
			}
			return true
		}
	}
	if strings.HasPrefix(word, "pip") && len(word) > len("pip") {
		rest := word[len("pip"):]
		if rest[0] >= '0' && rest[0] <= '9' {
			for _, r := range rest {
				if r < '0' || r > '9' {
					return false
				}
			}
			return true
		}
	}
	return false
}

// shellFields tokenizes a shell command into whitespace-separated words
// while respecting single and double quotes and backslash escapes.
func shellFields(s string) []string {
	var fields []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	hasContent := false

	flush := func() {
		if hasContent {
			fields = append(fields, current.String())
			current.Reset()
			hasContent = false
		}
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s):
			current.WriteByte(s[i+1])
			hasContent = true
			i++
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case !inSingle && !inDouble && (c == ' ' || c == '\t' || c == '\n'):
			flush()
		default:
			current.WriteByte(c)
			hasContent = true
		}
	}
	flush()
	return fields
}
