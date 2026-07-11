package scriptdoctrine

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/syntax"
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
// sites in data.
//
// The byte slice is first parsed as a single shell program with
// mvdan.cc/sh/v3/syntax - this is what lets heredocs, multi-line
// for/while bodies, and pipelines with continuation lines survive a
// single CountPythonInvocations call. If the mvdan.cc/sh parser
// rejects the input as malformed shell (which happens for the
// non-shell structure inside Makefiles and YAML files), the
// implementation falls back to the per-line scanner used by R5/R6
// to keep the legacy contract intact. Real shell scripts whose only
// structural oddity is a heredoc, multi-line compound command, or
// other R7-recognised construct still succeed on the AST path.
//
// Before parsing, any leading `run:` key in YAML workflow files
// is stripped so the rest of the line is treated as a normal shell
// command.
//
// A shebang `#!python3` on the very first line is counted as a
// python invocation; the rest of the file after a python shebang is
// treated as Python source (not shell) and contributes zero
// additional invocations. Other shebangs (bash, sh, etc.) are
// ignored and parsing falls through to the rest of the file.
//
// Output commands such as `echo python3 ...` never count because
// Python appears as data, not the command word. Lookups such as
// `command -v python3` and `command --version python3` never count.
// Pure variable assignments without command substitution never
// count.
//
// Returns:
//
//	-1 if data is nil OR both the AST and per-line paths fail. The
//	  caller MUST surface an internal-error diagnostic rather than
//	  treating the script as python-free.
//	N >= 0 otherwise.
func CountPythonInvocations(data []byte) int {
	if data == nil {
		return -1
	}
	stripped := yamlRunKeyRx.ReplaceAll(data, nil)

	// Honour a leading python shebang - that single invocation is
	// the only thing the script ever runs (the rest is python
	// source). Looking at the bytes avoids constructing a parser
	// just to ignore its output.
	if bytes.HasPrefix(stripped, []byte("#!")) {
		firstLine := stripped
		if nl := bytes.IndexByte(stripped, '\n'); nl >= 0 {
			firstLine = stripped[:nl]
		}
		if pythonShebangRx.Match(firstLine) {
			return 1
		}
	}

	// First attempt: full AST parse. This is the R7-correct path
	// for genuine shell scripts and exercises the new
	// heredoc/compound/prefix visitor.
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(bytes.NewReader(stripped), "inline")
	if err == nil {
		w := &pythonWalker{}
		for _, stmt := range file.Stmts {
			w.walkStmt(stmt)
		}
		return w.count
	}

	// Fall back to per-line scanning. This preserves the
	// R5/R6 behaviour for files whose surrounding structure
	// (Makefile variables, YAML `key: value`, GitHub Actions
	// `${{ }}` interpolation) makes the byte slice unparseable
	// as a single shell program. Heredocs spanning multiple
	// lines will be missed on this path; documented as a known
	// trade-off in docs/acts/ACT-UVB76-GO-TOOLING-DOCTRINE01-R7.md.
	//
	// We trust the per-line count unconditionally on this path:
	// the failure of the AST already tells us the file is not a
	// pure shell program, so escalating 0 -> -1 would only
	// produce spurious internal-error diagnostics for files
	// (e.g. Makefiles) that are not shell at all.
	return countPythonInvocationsFromReader(bytes.NewReader(stripped))
}

// CountPythonInvocationsFromFile is identical to CountPythonInvocations but
// reads from a file path. Returns -1 on any open error or scan error
// (fail-closed).
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

// countPythonInvocationsFromReader is the per-line variant kept for
// callers (and tests) that need to attribute invocations to a
// specific source line.
//
// Behaviour:
//   - scanner.Err() is honoured; -1 is returned on any IO or
//     buffer-oversize failure.
//   - A leading `#!python...` shebang counts as one invocation and
//     the rest of the file is treated as python source (zero
//     further invocations).
//   - GitHub-Actions-style `${{ ... }}` substitutions inside lines
//     are replaced with a placeholder so the per-line parser can
//     still produce a valid parse tree. The placeholder is not a
//     Python command word, so it cannot inflate the count.
//   - A line that the per-line parser cannot parse (returns -1) is
//     skipped rather than accumulated as a negative value. This
//     keeps the legacy R6 contract intact for the Makefile/YAML
//     surface where some lines are noise that the full-file parser
//     correctly rejects.
//   - If every non-blank, non-shebang line in the input was
//     rejected by the per-line parser, the input is treated as
//     completely unparseable shell and the function returns -1
//     (fail-closed). This distinguishes a Makefile whose recipe
//     lines all parse cleanly (return 0) from a shell file whose
//     every line is malformed (return -1).
//
// Returns the sum of python invocations across all parseable lines,
// or -1 if the underlying scanner failed OR every line was
// unparseable.
func countPythonInvocationsFromReader(r io.Reader) int {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	count := 0
	firstLine := true
	sawPythonShebang := false
	candidateLines := 0
	parseableLines := 0
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
		candidateLines++
		n := countPythonInvocationsInLine(line)
		if n < 0 {
			// Single-line parse failure: skip rather than accumulate
			// a negative value. See the function comment above.
			continue
		}
		parseableLines++
		count += n
	}
	// Fail closed on scanner errors (e.g. line too long for buffer, IO
	// failure). Returning the partial count would silently under-report
	// when malformed or oversized input truncates the scan; -1 lets the
	// caller emit an internal-error diagnostic instead.
	if err := scanner.Err(); err != nil {
		return -1
	}
	// Fail closed when every candidate line was rejected by the
	// per-line parser - this is the malformed-shell signal that we
	// cannot verify as "no python here".
	if candidateLines > 0 && parseableLines == 0 {
		return -1
	}
	return count
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
