package scriptdoctrine

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// SortDiagnostics sorts violations deterministically by check type
// then path then line then column. The line/column tie-breaker keeps
// ordering stable when a single path emits multiple diagnostics
// from different execution sites (e.g., multiple Make $(shell ...)
// expansions in one Makefile).
func SortDiagnostics(diags []Diagnostic) {
	sort.Slice(diags, func(i, j int) bool {
		if diags[i].Check != diags[j].Check {
			return diags[i].Check < diags[j].Check
		}
		if diags[i].Path != diags[j].Path {
			return diags[i].Path < diags[j].Path
		}
		if diags[i].Line != diags[j].Line {
			return diags[i].Line < diags[j].Line
		}
		return diags[i].Column < diags[j].Column
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

// CountPythonInvocations parses the byte slice as a single complete
// shell program with mvdan.cc/sh/v3/syntax and counts the python
// command sites inside it.
//
// Returns -1 if data is nil OR the program cannot be parsed. The
// fail-closed contract is intentional: malformed shell might host
// a python invocation that we cannot see, and the caller must
// surface an internal-error diagnostic rather than silently green-
// light the script.
//
// A leading `#!python3` shebang counts as one invocation and the
// rest of the file is treated as Python source (zero further
// invocations).
//
// Output commands such as `echo python3 ...` never count (Python
// appears as data, not as the command word). Lookups such as
// `command -v python3` and `command --version python3` never count.
// Pure variable assignments without command substitution never
// count.
func CountPythonInvocations(data []byte) int {
	if data == nil {
		return -1
	}
	// Honour a leading python shebang.
	if bytes.HasPrefix(data, []byte("#!")) {
		firstLine := data
		if nl := bytes.IndexByte(data, '\n'); nl >= 0 {
			firstLine = data[:nl]
		}
		if pythonShebangRx.Match(firstLine) {
			return 1
		}
	}
	count, err := countPythonSitesInProgram(string(data))
	if err != nil {
		return -1
	}
	return count.Count
}

// CountPythonInvocationsForPath dispatches to the appropriate
// extractor based on the file's path:
//
//   - `Makefile` or `*.mk` -> CountPythonInvocationsInMakefile
//     (TAB-indented recipes only, then shell parse)
//   - `.github/workflows/*.yml`/`*.yaml` -> CountPythonInvocationsInYAMLRunBlocks
//     (each `run:` value is shell-parsed independently)
//   - anything else -> CountPythonInvocations
//     (whole-file shell parse)
//
// The path may be either absolute or repository-relative; the
// extractor selection matches on the suffix of `.github/workflows/`
// and the file extension so both forms work.
//
// Returns -1 if data is nil OR the chosen extractor reports a
// parse error (fail-closed per extractor).
func CountPythonInvocationsForPath(path string, data []byte) int {
	if data == nil {
		return -1
	}
	base := filepath.Base(path)
	if base == "Makefile" || strings.HasSuffix(base, ".mk") {
		return CountPythonInvocationsInMakefile(data)
	}
	if (strings.HasPrefix(path, ".github/workflows/") ||
		strings.Contains(path, "/.github/workflows/")) &&
		(strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml")) {
		return CountPythonInvocationsInYAMLRunBlocks(data)
	}
	return CountPythonInvocations(data)
}

// CountPythonInvocationsForPathDetailed is the structured twin of
// CountPythonInvocationsForPath. It dispatches to the extractor
// that matches path and returns the structured InvocationCount plus
// an error.
//
// The error preserves the original *ClassificationError type so the
// verifier can call errors.As and reach the line/column fields
// directly. Bare extractor failures (mvdan.cc/sh parse errors,
// malformed YAML) are wrapped as *ClassificationError anchored at
// the parser's reported position; non-positioned failures use
// (line=0, column=0) so the caller can still distinguish
// "fail-closed with line/column" from "fail-closed without".
//
// Returning (ZeroCount, nil) for nil data is intentional: missing
// data is a programming error at the call site (the verifier layer
// reads from disk), not a classification problem to surface.
func CountPythonInvocationsForPathDetailed(path string, data []byte) (InvocationCount, error) {
	if data == nil {
		return ZeroCount, nil
	}
	base := filepath.Base(path)
	switch {
	case base == "Makefile" || strings.HasSuffix(base, ".mk"):
		return CountPythonInvocationsInMakefileDetailed(data)
	case isWorkflowYAMLPath(path):
		return classifyYAMLPythonRunBlocks(data)
	}
	count, err := countPythonSitesInProgram(string(data))
	if err != nil {
		// If the underlying walker already produced a typed
		// *ClassificationError (bash -c dynamic payload, etc.),
		// propagate it as-is so the verifier's errors.As can
		// recover the original line/column. We only wrap bare
		// mvdan.cc/sh parse failures (no caller context).
		var ce *ClassificationError
		if errors.As(err, &ce) {
			return ZeroCount, err
		}
		return ZeroCount, wrapShellParseError(filepath.Base(path), err)
	}
	return count, nil
}

// isWorkflowYAMLPath reports whether path lives under
// .github/workflows/ and ends in .yml/.yaml. The check tolerates
// both repo-relative ("workflows/foo.yml") and absolute
// ("/repo/.github/workflows/foo.yml") forms.
func isWorkflowYAMLPath(path string) bool {
	if !(strings.HasPrefix(path, ".github/workflows/") ||
		strings.Contains(path, "/.github/workflows/")) {
		return false
	}
	base := filepath.Base(path)
	return strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml")
}

// wrapShellParseError converts an mvdan.cc/sh parse error into a
// *ClassificationError carrying the parser's reported Line/Column.
// When the underlying error is not a *syntax.ParseError the wrapper
// falls back to (line=0, column=0) so callers can still detect
// "fail-closed" via errors.As(err, &*ClassificationError) without
// relying on line/column being populated.
func wrapShellParseError(path string, err error) error {
	var pe *syntax.ParseError
	if errors.As(err, &pe) {
		return NewClassificationError(
			path,
			int(pe.Pos.Line()),
			int(pe.Pos.Col()),
			"malformed shell program: "+err.Error(),
		)
	}
	return NewClassificationError(
		path, 0, 0,
		"shell classification failed: "+err.Error(),
	)
}

// CountPythonInvocationsFromFile reads path and counts python
// invocations using the extractor that matches its path.
// Returns -1 on any open or read error, or on parse failure.
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
	return CountPythonInvocationsForPath(path, data)
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
