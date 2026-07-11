package scriptdoctrine

import (
	"bytes"
	"regexp"
	"strings"
)

// stripYAMLQuotes removes a single layer of YAML single or double
// quotes around a string. It is intentionally minimal: it does not
// unescape escape sequences, since the shell parser will treat the
// result as a literal anyway. Used to recover the value of a YAML
// quoted scalar like `run: "python3 x.py"`.
func stripYAMLQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return s
	}
	if s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	if s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
}

// shellTemplateRx captures the command part of a custom shell
// template like `bash --noprofile --norc -c {0}` or `python {0}`.
// The captured group is the command prefix that precedes the `{0}`
// invocation placeholder.
var shellTemplateRx = regexp.MustCompile(`^(\S+?)\s*\{0\}\s*$`)

// resolveShellTemplate returns the executable name of a custom
// shell template string, or empty if the string is the default
// template (no `{0}` placeholder). The verifier uses this to
// decide whether a step's `shell:` field means "run via python".
func resolveShellTemplate(tpl string) string {
	tpl = strings.TrimSpace(tpl)
	if tpl == "" {
		return ""
	}
	// The default shell is `bash --noprofile --norc -eo pipefail {0}`.
	// No `{0}` placeholder means the user did not customise.
	if !strings.Contains(tpl, "{0}") {
		return ""
	}
	m := shellTemplateRx.FindStringSubmatch(tpl)
	if m == nil {
		return ""
	}
	// First token of the prefix is the command.
	fields := strings.Fields(m[1])
	if len(fields) == 0 {
		return ""
	}
	return filepathBase(fields[0])
}

// filepathBase is a small wrapper around path/filepath.Base that
// avoids pulling the package import for this single use.
func filepathBase(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}

// CountPythonInvocationsInYAMLRunBlocks extracts every `run:` and
// `shell:` value from a YAML GitHub Actions workflow and counts
// python invocations across the sum of run blocks.
//
// GitHub Actions supports three forms of `run`:
//   - inline scalar:  `run: cmd`
//   - literal block:  `run: |\n  cmd`
//   - folded block:   `run: >\n  cmd`
//
// `shell: python` (or a custom shell template whose executable is
// python) is treated as a python invocation because GitHub
// Actions invokes the shell template's command with the run body
// as its argument.
//
// The extractor does NOT pull in a YAML library; it tracks the
// indentation of each `run:` / `shell:` key to delimit block
// scalars. Each extracted scalar is parsed as a complete shell
// program so any malformed run block surfaces as -1 (fail-closed).
//
// Returns -1 if data is nil OR any extracted run block cannot be
// parsed as a complete shell program.
func CountPythonInvocationsInYAMLRunBlocks(data []byte) int {
	if data == nil {
		return -1
	}
	blocks, shells := extractYAMLRunAndShell(data)
	total := 0
	for _, block := range blocks {
		n, err := countPythonSitesInProgram(sanitizeGASubstitutions(stripYAMLQuotes(block)))
		if err != nil {
			return -1
		}
		total += n
	}
	// For every step whose shell maps to a python interpreter,
	// add 1 invocation to the total: the run body is passed to
	// `python {0}` (or the equivalent) as the {0} substitution.
	for _, shell := range shells {
		exec := resolveShellTemplate(shell)
		if exec == "" {
			exec = shell
		}
		if isPythonCommandWord(exec) {
			total++
		}
	}
	return total
}

// extractYAMLRunBlocks walks the input lines and returns each
// `run:` value and each `shell:` template, supporting inline
// scalars (`run: cmd`), literal block scalars (`run: |` followed
// by indented lines), and folded block scalars (`run: >`). The
// list-item prefix `- ` is tolerated.
func extractYAMLRunBlocks(data []byte) []string {
	blocks, _ := extractYAMLRunAndShell(data)
	return blocks
}

// extractYAMLRunAndShell is the shared internal helper that powers
// both extractYAMLRunBlocks and CountPythonInvocationsInYAMLRunBlocks.
func extractYAMLRunAndShell(data []byte) (blocks []string, shells []string) {
	lines := strings.Split(string(data), "\n")
	var blockIndent int
	var blockKey string // "run" or "shell"
	var blockMarker byte
	var blockLines []string

	flushBlock := func() {
		if blockIndent < 0 {
			return
		}
		rendered := renderBlock(blockLines, blockMarker, blockIndent)
		switch blockKey {
		case "run":
			if rendered != "" {
				blocks = append(blocks, rendered)
			}
		case "shell":
			if rendered != "" {
				shells = append(shells, rendered)
			}
		}
		blockIndent = -1
		blockKey = ""
		blockMarker = 0
		blockLines = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")
		indent := leadingSpaces(line)

		if blockIndent >= 0 {
			if trimmed == "" {
				blockLines = append(blockLines, "")
				continue
			}
			if indent > blockIndent {
				blockLines = append(blockLines, line)
				continue
			}
			flushBlock()
		}

		key, after, ok := splitRunKeyExt(trimmed)
		if !ok || (key != "run" && key != "shell") {
			continue
		}

		value := strings.TrimSpace(after)
		switch {
		case value == "|" || value == "|-":
			blockIndent = indent
			blockKey = key
			blockMarker = '|'
			blockLines = nil
		case value == ">" || value == ">-":
			blockIndent = indent
			blockKey = key
			blockMarker = '>'
			blockLines = nil
		default:
			// Inline scalar (with or without trailing content).
			// Strip a single layer of YAML single or double
			// quotes so `shell: "python"` is classified as
			// `python` and not as the literal string `"python"`.
			if key == "run" {
				blocks = append(blocks, stripYAMLQuotes(value))
			} else {
				shells = append(shells, stripYAMLQuotes(value))
			}
		}
	}
	flushBlock()
	return blocks, shells
}

// splitRunKeyExt is the same as splitRunKey but returns any key
// (used here to recognise both `run:` and `shell:` headers).
func splitRunKeyExt(line string) (key string, after string, ok bool) {
	stripped := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(stripped, "- ") {
		stripped = strings.TrimPrefix(stripped, "- ")
		stripped = strings.TrimLeft(stripped, " \t")
	}
	// Recognise either "run:" or "shell:" as the prefix.
	for _, prefix := range []string{"shell:", "run:"} {
		if strings.HasPrefix(stripped, prefix) {
			rest := strings.TrimPrefix(stripped, prefix)
			rest = strings.TrimLeft(rest, " \t")
			return strings.TrimSuffix(prefix, ":"), rest, true
		}
	}
	return "", "", false
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

// renderBlock joins an accumulated block-scalar body into a single
// shell program, stripping the block's common leading indent so
// the parser sees clean shell from column 0. Folded scalars (`>`)
// per YAML spec join adjacent lines with a single space; literal
// scalars (`|`) preserve newlines. Returns an empty string when
// there is nothing meaningful to render.
func renderBlock(body []string, marker byte, headerIndent int) string {
	if len(body) == 0 {
		return ""
	}
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
