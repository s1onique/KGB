package scriptdoctrine

import (
	"bytes"
	"regexp"
	"strconv"
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
	steps, err := extractYAMLSteps(data)
	if err != nil {
		return -1
	}
	total := 0
	for _, step := range steps {
		// R12 fail-closed: a dynamic GitHub Actions shell
		// substitution (`shell: ${{ matrix.shell }}`) cannot
		// be statically resolved to a python interpreter (or
		// any other executable), so the verifier must
		// surface this as a hard error rather than
		// silently green-light the file.
		if isDynamicShell(step.StepShell, step.JobDefaults, step.WorkflowShell) {
			return -1
		}
		if isPythonShell(step.StepShell, step.JobDefaults, step.WorkflowShell) {
			total++
			continue
		}
		if step.Run == "" {
			continue
		}
		count, perr := countPythonSitesInProgram(sanitizeGASubstitutions(step.Run))
		if perr != nil {
			return -1
		}
		total += count.Count
	}
	return total
}


// yamlLineColumnRx extracts the trailing "line N column M" pair from
// any error message produced by extractYAMLSteps. The regex is
// intentionally permissive: it matches both "line N column M" and
// "line N" so messages without a column component still surface a
// line number. classifyYAMLPythonRunBlocks uses this to recover
// line/column when wrapping a generic extractYAMLSteps error.
var yamlLineColumnRx = regexp.MustCompile(`line (\d+)(?: column (\d+))?`)

// classifyYAMLPythonRunBlocks is the typed twin of
// CountPythonInvocationsInYAMLRunBlocks. It returns the structured
// InvocationCount plus an error that, on a fail-closed surface,
// carries the original *ClassificationError so the verifier can call
// errors.As and populate Diagnostic.Line/Column/Msg.
//
// The error path preserves the AST node position: dynamic shell
// templates use step.Line/step.Column; malformed YAML extracts from
// the upstream error message via yamlLineColumnRx.
func classifyYAMLPythonRunBlocks(data []byte) (InvocationCount, error) {
	if data == nil {
		return ZeroCount, nil
	}
	steps, err := extractYAMLSteps(data)
	if err != nil {
		line, col := 0, 0
		if m := yamlLineColumnRx.FindStringSubmatch(err.Error()); len(m) >= 2 {
			line, _ = strconv.Atoi(m[1])
			if len(m) >= 3 && m[2] != "" {
				col, _ = strconv.Atoi(m[2])
			}
		}
		return ZeroCount, NewClassificationError("", line, col, "workflow YAML: "+err.Error())
	}
	total := 0
	for _, step := range steps {
		if isDynamicShell(step.StepShell, step.JobDefaults, step.WorkflowShell) {
			return ZeroCount, NewClassificationError(
				"", step.Line, step.Column, "dynamic workflow shell")
		}
		if isPythonShell(step.StepShell, step.JobDefaults, step.WorkflowShell) {
			total++
			continue
		}
		if step.Run == "" {
			continue
		}
		count, perr := countPythonSitesInProgram(sanitizeGASubstitutions(step.Run))
		if perr != nil {
			return ZeroCount, NewClassificationError(
				"", step.Line, step.Column, "malformed workflow run block: "+perr.Error())
		}
		total += count.Count
	}
	return InvocationCount{Count: total}, nil
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
