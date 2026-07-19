// Package allocationtrackerimports enforces the private runtime package boundary.
package allocationtrackerimports

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	ImportToken          = "@import"
	SourceScopePathspec  = ":(glob)tovarisch/src/**/*.zig"
	TrustedRuntimePrefix = "tovarisch/src/runtime/"
)

var privateSiblingBasenames = []string{
	"allocation_tracker_internal.zig",
	"allocation_tracker_destroy.zig",
	"allocation_tracker_tracking_allocator.zig",
	"allocation_tracker_snapshots.zig",
	"allocation_tracker_connector_probe.zig",
}

// Deliberately strict: escaped literals and every expression form fail closed.
var approvedLiteralImport = regexp.MustCompile(
	`^@import\s*\(\s*"([^"\\\r\n]+)"\s*,?\s*\)`,
)

// Finding is one import-boundary policy violation.
type Finding struct {
	Path   string
	Line   int
	Reason string
	Source string
}

func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", f.Path, f.Line, f.Reason, f.Source)
}

// FindRepoRoot resolves the repository root through Git.
func FindRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %w: %s", err, output)
	}
	return strings.TrimSpace(string(output)), nil
}

// VerifyRepo scans distinct cached and untracked inventories from the real tree.
func VerifyRepo(repoRoot string) ([]Finding, error) {
	cached, err := gitList(
		repoRoot, "--cached", "--", SourceScopePathspec,
	)
	if err != nil {
		return nil, err
	}
	others, err := gitList(
		repoRoot, "--others", "--exclude-standard", "--", SourceScopePathspec,
	)
	if err != nil {
		return nil, err
	}

	inventory := append(
		filterTrustedRuntime(cached),
		filterTrustedRuntime(others)...,
	)
	return Scan(repoRoot, inventory)
}

func gitList(repoRoot string, args ...string) ([]string, error) {
	allArgs := append([]string{"ls-files", "-z"}, args...)
	cmd := exec.Command("git", allArgs...)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf(
			"git %s failed: %w: %s", strings.Join(allArgs, " "), err, output,
		)
	}

	parts := bytes.Split(output, []byte{0})
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) != 0 {
			paths = append(paths, string(part))
		}
	}
	return paths, nil
}

func filterTrustedRuntime(paths []string) []string {
	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		if strings.HasPrefix(filepath.ToSlash(path), TrustedRuntimePrefix) {
			continue
		}
		filtered = append(filtered, path)
	}
	return filtered
}

// Scan rejects every unapproved source-level import and every literal private import.
func Scan(repoRoot string, inventory []string) ([]Finding, error) {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("resolving repository root: %w", err)
	}
	// Canonicalize the root too: macOS temporary paths commonly map
	// `/var` to `/private/var`, and importers are symlink-evaluated below.
	root = canonicalPath(root)
	privatePaths := canonicalPrivatePaths(root)
	findings := make([]Finding, 0)

	for _, relPath := range inventory {
		absolute, err := resolveImporter(root, relPath)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(absolute)
		if err != nil {
			return nil, fmt.Errorf("scanner could not stat %s: %w", relPath, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("scanner inventory entry is not a file: %s", relPath)
		}
		contents, err := os.ReadFile(absolute)
		if err != nil {
			return nil, fmt.Errorf("scanner could not read %s: %w", relPath, err)
		}

		for _, offset := range codeImportOffsets(contents) {
			line, source := sourceLocation(contents, offset)
			match := approvedLiteralImport.FindSubmatch(contents[offset:])
			if match == nil {
				findings = append(findings, Finding{
					Path: relPath, Line: line, Source: source,
					Reason: "non-literal or unapproved @import syntax rejected",
				})
				continue
			}

			resolved, inside, err := resolveTarget(root, absolute, string(match[1]))
			if err != nil {
				return nil, fmt.Errorf("resolving import in %s: %w", relPath, err)
			}
			if inside {
				if _, private := privatePaths[resolved]; private {
					findings = append(findings, Finding{
						Path: relPath, Line: line, Source: source,
						Reason: "external import of private allocation_tracker sibling",
					})
				}
			}
		}
	}
	return findings, nil
}

func canonicalPrivatePaths(root string) map[string]struct{} {
	paths := make(map[string]struct{}, len(privateSiblingBasenames))
	for _, basename := range privateSiblingBasenames {
		path := filepath.Join(root, "tovarisch", "src", "runtime", basename)
		paths[canonicalPath(path)] = struct{}{}
	}
	return paths
}

func resolveImporter(root, relPath string) (string, error) {
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("scanner importer must be repository-relative: %s", relPath)
	}
	absolute := canonicalPath(filepath.Join(root, filepath.FromSlash(relPath)))
	if !pathInside(root, absolute) {
		return "", fmt.Errorf("scanner importer escapes repository: %s", relPath)
	}
	return absolute, nil
}

func resolveTarget(root, importer, target string) (string, bool, error) {
	if target == "" || filepath.IsAbs(target) {
		return "", false, nil
	}
	absolute := canonicalPath(filepath.Join(filepath.Dir(importer), filepath.FromSlash(target)))
	return absolute, pathInside(root, absolute), nil
}

func canonicalPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if evaluated, evalErr := filepath.EvalSymlinks(absolute); evalErr == nil {
		return filepath.Clean(evaluated)
	}
	return filepath.Clean(absolute)
}

func pathInside(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func codeImportOffsets(contents []byte) []int {
	offsets := make([]int, 0)
	for i := 0; i < len(contents); {
		if bytes.HasPrefix(contents[i:], []byte("//")) {
			i = skipLine(contents, i+2)
			continue
		}
		if multilineStringLineStart(contents, i) {
			i = skipLine(contents, i+2)
			continue
		}
		if contents[i] == '"' || contents[i] == '\'' {
			i = skipQuoted(contents, i, contents[i])
			continue
		}
		if bytes.HasPrefix(contents[i:], []byte(ImportToken)) {
			end := i + len(ImportToken)
			if end == len(contents) || !identifierContinue(contents[end]) {
				offsets = append(offsets, i)
				i = end
				continue
			}
		}
		i++
	}
	return offsets
}

func skipLine(contents []byte, offset int) int {
	if next := bytes.IndexByte(contents[offset:], '\n'); next >= 0 {
		return offset + next + 1
	}
	return len(contents)
}

func skipQuoted(contents []byte, offset int, quote byte) int {
	for i := offset + 1; i < len(contents); i++ {
		switch contents[i] {
		case '\\':
			i++
		case quote:
			return i + 1
		case '\n':
			return i + 1
		}
	}
	return len(contents)
}

func multilineStringLineStart(contents []byte, offset int) bool {
	if offset+1 >= len(contents) || contents[offset] != '\\' || contents[offset+1] != '\\' {
		return false
	}
	lineStart := bytes.LastIndexByte(contents[:offset], '\n') + 1
	return len(bytes.TrimSpace(contents[lineStart:offset])) == 0
}

func identifierContinue(ch byte) bool {
	return ch == '_' || ch >= '0' && ch <= '9' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z'
}

func sourceLocation(contents []byte, offset int) (int, string) {
	line := bytes.Count(contents[:offset], []byte{'\n'}) + 1
	start := bytes.LastIndexByte(contents[:offset], '\n') + 1
	end := len(contents)
	if next := bytes.IndexByte(contents[offset:], '\n'); next >= 0 {
		end = offset + next
	}
	return line, string(contents[start:end])
}
