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
// shebang. The error return is mandatory: a non-nil error means the file
// could not be read (because it does not exist, is a directory, or is
// otherwise inaccessible) and the caller must NOT treat that as "not a
// Python shebang". Fail-closed contract.
func HasPythonShebang(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	buf := make([]byte, 256)
	n, err := f.Read(buf)
	// EOF on a zero-byte read is fine - the file is empty, no shebang.
	if err != nil && n == 0 {
		if err == io.EOF {
			return false, nil
		}
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	return pythonShebangRx.Match(buf[:n]), nil
}

// variableAssignRx matches shell / Makefile variable assignments at line
// start (identifier followed by =).
var variableAssignRx = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\s*=`)

// yamlRunKeyRx matches the leading "run:" key in YAML GitHub Actions
// steps. The "run" keyword is not a shell command; stripping it lets the
// rest of the line be parsed as a normal command list.
var yamlRunKeyRx = regexp.MustCompile(`(?m)^\s*run:\s*`)

// CountPythonInvocations returns the number of executable Python command
// sites in data. The metric counts command sites (not lines) - each
// distinct command node after parsing command lists, command
// substitutions, and pipelines counts independently.
//
// Before per-line parsing, any leading `run:` key in YAML workflow files
// is stripped so the rest of the line is treated as a normal shell
// command.
//
// Comments (after stripping inline trailing comments that are not inside
// quotes) do not count. Pure variable assignments without command
// substitution do not count. Output commands such as `echo python3 ...`
// do not count because Python appears as data, not as the command word.
// `command -v python3` and `command --version` do not count because they
// are lookups, not invocations.
//
// Returns:
//
//	-1 if data is nil.
//	N >= 0 otherwise.
func CountPythonInvocations(data []byte) int {
	if data == nil {
		return -1
	}
	// Strip YAML run: keys so the value is treated as a shell command.
	stripped := yamlRunKeyRx.ReplaceAll(data, nil)
	return countPythonInvocationsFromReader(bytes.NewReader(stripped))
}

// CountPythonInvocationsFromFile is identical to CountPythonInvocations but
// reads from a file path. Returns -1 on any open error.
func CountPythonInvocationsFromFile(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return -1
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return -1
	}
	return CountPythonInvocations(data)
}

func countPythonInvocationsFromReader(r io.Reader) int {
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
				continue
			}
			// Non-shebang first line: count normally and fall through.
		}
		// After a Python shebang we have stopped counting - the rest of
		// the file is Python source, not shell command invocations.
		if sawPythonShebang {
			continue
		}
		count += countPythonInvocationsInLine(line)
	}
	return count
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

// IsBinaryFile reports whether path is a binary file (contains null bytes
// in the first 512 bytes). Returns the read error so callers can fail
// closed on filesystem failures.
func IsBinaryFile(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		if err == io.EOF {
			return false, nil
		}
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	for i := 0; i < n && i < 512; i++ {
		if buf[i] == 0 {
			return true, nil
		}
	}
	return false, nil
}

// IsExcludedPath reports whether path falls under a directory that the
// script doctrine verification must skip entirely: vendor trees,
// dependency caches, build output, git internals. These trees contain
// files that look executable but are not first-party tooling.
func IsExcludedPath(path string) bool {
	excludes := []string{
		"/vendor/",
		"/third_party/",
		"/node_modules/",
		"/.zig-cache/",
		"/zig-cache/",
		"/zig-out/",
		"/__pycache__/",
		".git/hooks/",
		".git/objects/",
		".git/refs/",
		"/artifacts/",
		"/dist/",
	}
	for _, ex := range excludes {
		if strings.Contains(path, ex) {
			return true
		}
	}
	return false
}

// Sanity-check the package-level invariants at startup so that a regression
// in the python invocation detection fails loudly rather than silently
// undercount.
func init() {
	if !pythonShebangRx.MatchString("#!/usr/bin/env python3") {
		panic("scriptdoctrine: pythonShebangRx no longer matches Python shebang")
	}
	if !invokesPython("python3 script.py") {
		panic("scriptdoctrine: invokesPython no longer matches direct python3 call")
	}
	if !invokesPython("\tpython3 script.py") {
		panic("scriptdoctrine: invokesPython no longer matches tab-indented recipe")
	}
	if !invokesPython("/usr/bin/python3 script.py") {
		panic("scriptdoctrine: invokesPython no longer matches absolute path")
	}
	if !invokesPython("python3 a.py; python3 b.py") {
		panic("scriptdoctrine: invokesPython no longer matches chained commands")
	}
	if invokesPython("# python3 script.py") {
		panic("scriptdoctrine: invokesPython counted a comment as invocation")
	}
	if invokesPython("echo \"python3 script.py\"") {
		panic("scriptdoctrine: invokesPython counted an echo argument as invocation")
	}
	if invokesPython("PY=python3") {
		panic("scriptdoctrine: invokesPython counted a variable assignment as invocation")
	}
	if invokesPython("command -v python3") {
		panic("scriptdoctrine: invokesPython counted command -v as invocation")
	}
	// Force-import "fmt" so package compile checks remain stable when
	// other files in this package stop using it.
	_ = fmt.Sprintf
}
