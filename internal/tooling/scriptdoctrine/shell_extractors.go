package scriptdoctrine

import (
	"bytes"
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
func sanitizeMakeVars(s string) string {
	if !strings.Contains(s, "$(") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
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
				// $(VAR) -> $VAR; $(call x,y) -> 'X'.
				if strings.ContainsAny(inside, " ,$") {
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

// CountPythonInvocationsInMakefile extracts the TAB-indented recipe
// lines of a Makefile (and `.mk` includes) and counts python
// invocations over the joined recipes.
//
// Makefiles are not shell programs: variable assignments, includes,
// and conditionals are GNU make syntax. Only TAB-indented lines are
// shell commands (one shell invocation per line in vanilla make;
// .ONESHELL: redirects treat the entire recipe as one shell call).
// We join the recipe lines with newlines and feed the result to the
// mvdan.cc/sh parser, so any heredoc body, $(...) expansion, or
// pipeline that crosses two TAB-indented lines is parsed as part of
// the same shell program.
//
// Before parsing, the recipe text is sanitised: GNU make's $(VAR)
// and $(call ...) expansions use parentheses that mvdan.cc/sh
// interprets as subshell syntax (which would cause spurious parse
// errors). The sanitiser rewrites variable lookups to bare $VAR and
// replaces function calls with a single placeholder char so they
// cannot trigger a parser error.
//
// Returns -1 if data is nil OR the resulting shell program cannot
// be parsed (fail-closed: malformed recipes should not be silently
// green-lit).
func CountPythonInvocationsInMakefile(data []byte) int {
	if data == nil {
		return -1
	}
	var recipes []string
	for _, line := range strings.Split(string(data), "\n") {
		// GNU make requires TAB (not spaces) for recipes.
		if !strings.HasPrefix(line, "\t") {
			continue
		}
		// Strip the TAB plus an optional GNU make silent prefix
		// `@` and any leading spaces. mvdan.cc/sh rejects `@`
		// at command position so the silent prefix must be
		// removed before parsing.
		body := strings.TrimLeft(line[1:], " \t")
		if strings.HasPrefix(body, "@") {
			body = strings.TrimLeft(body[1:], " \t")
		} else if strings.HasPrefix(body, "-") {
			// `-@cmd` (errors ignored) is also a Make silent
			// prefix.
			body = strings.TrimLeft(body[1:], " \t")
		}
		recipes = append(recipes, body)
	}
	if len(recipes) == 0 {
		return 0
	}
	n, err := countPythonSitesInProgram(sanitizeMakeVars(strings.Join(recipes, "\n")))
	if err != nil {
		return -1
	}
	return n
}

// CountPythonInvocationsInYAMLRunBlocks extracts every `run:` value
// from a YAML GitHub Actions workflow and counts python invocations
// across the sum of run blocks.
//
// GitHub Actions supports three forms of `run`:
//   - inline scalar:  `run: cmd`
//   - literal block:  `run: |\n  cmd`
//   - folded block:   `run: >\n  cmd`
//
// The extractor does NOT pull in a YAML library; it tracks the
// indentation of each `run:` key to delimit block scalars. Each
// extracted scalar is parsed as a complete shell program so any
// malformed run block surfaces as -1 (fail-closed).
//
// Before parsing each block we sanitise GitHub-Actions-only
// constructs that are not valid bash (the `${{ ... }}` template
// substitution, with leading `$` followed by `{` `{` is an empty
// parameter name). The substitution is replaced with `$X` (a
// valid parameter reference to a never-set variable). The
// placeholder cannot itself be a python command word, so the
// substitution cannot inflate the count.
//
// Returns -1 if data is nil OR any extracted run block cannot be
// parsed as a complete shell program.
func CountPythonInvocationsInYAMLRunBlocks(data []byte) int {
	if data == nil {
		return -1
	}
	blocks := extractYAMLRunBlocks(data)
	total := 0
	for _, block := range blocks {
		n, err := countPythonSitesInProgram(sanitizeGASubstitutions(block))
		if err != nil {
			return -1
		}
		total += n
	}
	return total
}

// sanitizeGASubstitutions replaces every GitHub-Actions template
// substitution `${{ ... }}` with a benign parameter reference.
// bash rejects `${{` because `${` followed by `{` looks like an
// empty parameter name; GitHub-Actions ships the `}}` to delimit
// the substitution. Replacing the whole `${{ ... }}` with `$X`
// (a never-set variable) leaves a syntactically valid shell
// fragment that does not change any python-command count.
func sanitizeGASubstitutions(s string) string {
	if !strings.Contains(s, "${{") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if i+3 < len(s) && s[i] == '$' && s[i+1] == '{' && s[i+2] == '{' {
			// find the matching "}}"
			j := i + 3
			depth := 1
			for j < len(s)-1 {
				if s[j] == '}' && s[j+1] == '}' {
					depth--
					if depth == 0 {
						break
					}
				}
				if s[j] == '{' && j+1 < len(s) && s[j+1] == '{' {
					depth++
				}
				j++
			}
			if depth == 0 {
				b.WriteString("$X")
				i = j + 2
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// extractYAMLRunBlocks walks the input lines and returns each `run:`
// value, supporting inline scalars (`run: cmd`), literal block
// scalars (`run: |` followed by indented lines), and folded block
// scalars (`run: >`). The list-item prefix `- ` is tolerated.
func extractYAMLRunBlocks(data []byte) []string {
	lines := strings.Split(string(data), "\n")
	var blocks []string
	var blockIndent int    // indent of the run: header line; -1 means no active block
	var blockMarker byte   // '|' or '>'
	var blockLines []string // accumulated body lines

	flushBlock := func() {
		if blockIndent < 0 {
			return
		}
		if rendered := renderBlock(blockLines, blockMarker, blockIndent); rendered != "" {
			blocks = append(blocks, rendered)
		}
		blockIndent = -1
		blockMarker = 0
		blockLines = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")
		indent := leadingSpaces(line)

		// Inside a block scalar: accumulate while indented deeper
		// than the header. A blank or shallower line closes the
		// block.
		if blockIndent >= 0 {
			if trimmed == "" {
				blockLines = append(blockLines, "")
				continue
			}
			if indent > blockIndent {
				blockLines = append(blockLines, line)
				continue
			}
			// Block ends here.
			flushBlock()
		}

		// Look for the `run:` key.
		key, after, ok := splitRunKey(trimmed)
		if !ok || key != "run" {
			continue
		}

		value := strings.TrimSpace(after)
		switch {
		case value == "|" || value == "|-":
			blockIndent = indent
			blockMarker = '|'
			blockLines = nil
		case value == ">" || value == ">-":
			blockIndent = indent
			blockMarker = '>'
			blockLines = nil
		default:
			// Inline scalar (with or without trailing content).
			blocks = append(blocks, value)
		}
	}
	flushBlock()
	return blocks
}

// splitRunKey parses a line for a `run:` YAML key (with optional
// list-item `- ` prefix) and returns the trimmed value tail.
func splitRunKey(line string) (key string, after string, ok bool) {
	stripped := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(stripped, "- ") {
		stripped = strings.TrimPrefix(stripped, "- ")
		stripped = strings.TrimLeft(stripped, " \t")
	}
	if !strings.HasPrefix(stripped, "run:") {
		return "", "", false
	}
	rest := strings.TrimPrefix(stripped, "run:")
	rest = strings.TrimLeft(rest, " \t")
	return "run", rest, true
}

// leadingSpaces returns the number of leading whitespace characters
// in s. Tabs are counted as one whitespace unit (sufficient for the
// YAML indentation patterns we observe in CI workflows).
func leadingSpaces(s string) int {
	for i, r := range s {
		if r != ' ' && r != '\t' {
			return i
		}
	}
	return len(s)
}

// renderBlock joins an accumulated `run:` block-scalar body into a
// single shell program, stripping the block's common leading indent
// so the parser sees clean shell from column 0. Folded scalars (`>`)
// per YAML spec join adjacent lines with a single space; literal
// scalars (`|`) preserve newlines. Returns an empty string when
// there is nothing meaningful to render.
func renderBlock(body []string, marker byte, headerIndent int) string {
	if len(body) == 0 {
		return ""
	}
	// Find the smallest indent among non-blank body lines so we
	// can strip it uniformly.
	minIndent := -1
	for _, l := range body {
		if strings.TrimSpace(l) == "" {
			continue
		}
		ind := leadingSpaces(l)
		if minIndent == -1 || ind < minIndent {
			minIndent = ind
		}
	}
	if minIndent < 0 || minIndent < headerIndent+2 {
		return ""
	}
	var b bytes.Buffer
	for i, l := range body {
		if i > 0 {
			if marker == '>' {
				b.WriteByte(' ')
			} else {
				b.WriteByte('\n')
			}
		}
		if len(l) >= minIndent {
			b.WriteString(l[minIndent:])
		} else {
			b.WriteString(l)
		}
	}
	return b.String()
}
