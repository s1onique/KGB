package scriptdoctrine

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
)

// SortDiagnostics sorts violations deterministically by check type then path.
func SortDiagnostics(diags []Diagnostic) {
	sort.Slice(diags, func(i, j int) bool {
		if diags[i].Check != diags[j].Check {
			return diags[i].Check < diags[j].Check
		}
		return diags[i].Path < diags[j].Path
	})
}

// CountLogicalLOC counts non-blank, non-comment lines in a shell script.
//
// Returns:
//
//	-1 if the file could not be opened or read.
//	 0 for an empty file or one with only comments/shebang.
//	N >= 1 for the count of non-blank, non-comment, non-shebang lines.
func CountLogicalLOC(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return -1
	}
	defer f.Close()

	count := 0
	isFirstLine := true
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if isFirstLine && strings.HasPrefix(line, "#!") {
			isFirstLine = false
			continue
		}
		isFirstLine = false

		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		count++
	}

	if err := scanner.Err(); err != nil {
		return -1
	}
	return count
}

// Python shebang pattern. A Python shebang starts with #! and points at a
// Python interpreter (or pip/pytest wrappers).
var pythonShebangRx = regexp.MustCompile(`^#!.*\b(?:python|python3|pip|pytest)\b`)

// HasPythonShebang reports whether the file at path starts with a Python
// shebang. Returns false on any read error.
func HasPythonShebang(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, 256)
	n, _ := f.Read(buf)
	if n == 0 {
		return false
	}
	return pythonShebangRx.Match(buf[:n])
}

// pureOutputCmdRx matches lines whose first command word is a known
// non-executing output command. Such lines print/transform text and only
// reference Python as data, not as an executable.
var pureOutputCmdRx = regexp.MustCompile(`^(echo|printf|cat|sed|awk|grep|dd|cut|tr|head|tail)\b`)

// variableAssignRx matches shell / Makefile variable assignments at line
// start (identifier followed by =). These assign values; they do not run.
// Lines whose value is a command substitution ($(...), ${...}, or backticks)
// are NOT variable assignments - they execute - and must be excluded here.
var variableAssignRx = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\s*=`)

// hasCmdSubstitutionRx detects a command substitution anywhere in the line.
var hasCmdSubstitutionRx = regexp.MustCompile(`\$\(|\$\{|` + "`" + `.*` + "`" + `|` + "`")

// pythonInvocationLineRx matches a Python invocation in command position.
// The leading alternation enforces a real command boundary: start of line,
// whitespace, semicolon, logical operators, pipe, command substitution, or
// a slash (used by absolute paths like /usr/bin/python3).
// An optional prefix allows for /usr/bin/env, sudo, env, exec, or command.
var pythonInvocationLineRx = regexp.MustCompile(
	`(?:^|[/\s;&|]+|(?:\$\(|\$\{|` + "`" + `))` +
		`(?:/usr/bin/env\s+)?` +
		`(?:sudo\s+|env\s+|exec\s+|command\s+)?` +
		`(?:python\d?(?:\.\d+)?|pip\d?|pytest)\b`,
)

// CountPythonInvocations returns the number of unique Python command sites in
// data. A "command site" is one non-comment, non-output-command,
// non-variable-assignment line that actually invokes Python. Each line
// contributes at most one count, so multiple patterns matching the same line
// never inflate the total.
//
// Line categories that contribute 0:
//   - comment-only lines (trimmed line starts with #, after stripping any
//     inline trailing comment that is not inside single or double quotes)
//   - lines whose first command word is echo/printf/cat/sed/awk/grep/dd/cut/tr
//     (Python appears only as printed data, not as an executable)
//   - pure variable assignments (VAR=value) where Python appears only as a
//     value, never inside a command substitution
//   - blank lines
//
// The first line is treated specially: if it is a Python shebang it counts
// as exactly one invocation. The body of the script is then assumed to be
// Python source (not shell), so further Python token matches in subsequent
// lines are NOT counted as invocations - they would be Python language
// syntax, not shell commands invoking Python.
//
// Returns -1 if data is nil.
func CountPythonInvocations(data []byte) int {
	if data == nil {
		return -1
	}
	return countPythonInvocations(bytes.NewReader(data))
}

// CountPythonInvocationsFromFile is identical to CountPythonInvocations but
// reads from a file path. Returns -1 on any open error.
func CountPythonInvocationsFromFile(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return -1
	}
	defer f.Close()
	return countPythonInvocations(f)
}

func countPythonInvocations(r io.Reader) int {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	count := 0
	firstLine := true
	sawPythonShebang := false
	for scanner.Scan() {
		line := scanner.Text()
		if firstLine {
			firstLine = false
			if pythonShebangRx.MatchString(line) {
				count++
				sawPythonShebang = true
			} else if isExecutablePythonLine(line) {
				count++
			}
			continue
		}
		// After a Python shebang we have stopped counting - the rest of
		// the file is Python source, not shell command invocations.
		if sawPythonShebang {
			continue
		}
		if isExecutablePythonLine(line) {
			count++
		}
	}
	return count
}

// isExecutablePythonLine returns true if line contains a Python invocation
// that would actually be executed (not a comment, output argument, or value).
func isExecutablePythonLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	// Strip an inline trailing comment (not inside quotes) so that
	// `cmd ... # python3` does not count.
	if idx := indexUnquotedHash(trimmed); idx >= 0 {
		trimmed = strings.TrimSpace(trimmed[:idx])
		if trimmed == "" {
			return false
		}
	}
	// Comment-only line.
	if strings.HasPrefix(trimmed, "#") {
		return false
	}
	// Output commands: Python mentioned only as printed data.
	if pureOutputCmdRx.MatchString(trimmed) {
		return false
	}
	// Variable assignments are values, not invocations - unless the value
	// itself runs a command (e.g. X=$(python3 ...)).
	if variableAssignRx.MatchString(trimmed) && !hasCmdSubstitutionRx.MatchString(trimmed) {
		return false
	}
	// Otherwise: require a Python token at a real command boundary.
	return pythonInvocationLineRx.MatchString(trimmed)
}

// indexUnquotedHash returns the index of the first '#' that is not inside
// single or double quotes. Returns -1 if no such hash exists.
func indexUnquotedHash(s string) int {
	inSingle := false
	inDouble := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return i
			}
		}
	}
	return -1
}

// HasPythonInvocation reports whether file content contains at least one
// executable Python command site.
func HasPythonInvocation(data []byte) bool {
	return CountPythonInvocations(data) > 0
}

// Sanity-check the package-level invariants at startup so that a regression
// in the python invocation regex fails loudly rather than silently undercount.
func init() {
	if !pythonShebangRx.MatchString("#!/usr/bin/env python3") {
		panic("scriptdoctrine: pythonShebangRx no longer matches Python shebang")
	}
	if !pythonInvocationLineRx.MatchString("python3 script.py") {
		panic("scriptdoctrine: pythonInvocationLineRx no longer matches direct python3 call")
	}
	if !pythonInvocationLineRx.MatchString("\tpython3 script.py") {
		panic("scriptdoctrine: pythonInvocationLineRx no longer matches tab-indented recipe")
	}
	if !pythonInvocationLineRx.MatchString("/usr/bin/python3 script.py") {
		panic("scriptdoctrine: pythonInvocationLineRx no longer matches absolute path")
	}
	if isExecutablePythonLine("# python3 script.py") {
		panic("scriptdoctrine: comment line counted as invocation")
	}
	if isExecutablePythonLine("echo \"python3 script.py\"") {
		panic("scriptdoctrine: echo line counted as invocation")
	}
	if isExecutablePythonLine("PY=python3") {
		panic("scriptdoctrine: variable assignment counted as invocation")
	}
	if !isExecutablePythonLine("X=$(python3 -c 'print(1)')") {
		panic("scriptdoctrine: command substitution not counted as invocation")
	}
	// Force-import "fmt" so package compile checks remain stable when
	// other files in this package stop using it.
	_ = fmt.Sprintf
}
