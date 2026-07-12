package producer

import (
	"path/filepath"
	"regexp"
	"strings"
)

// CompiledPattern is a compiled inventory path pattern.
//
// The pattern accepts exact paths, single-level glob segments (each
// segment containing at most one '*' that does not span slashes), and
// recursive glob segments ('**' standing alone). Forward slashes only.
type CompiledPattern struct {
	// Pattern is the normalized inventory path.
	Pattern string
	// Regex is anchored on both ends to a full repository-relative path.
	Regex *regexp.Regexp
	// Kind is "exact", "glob", or "recursive".
	Kind string
}

// CompileInventoryPattern compiles an inventory path pattern.
//
// Accepted forms:
//
//	uvb76/foo.json                                     — exact path
//	uvb76/cmd/*/main.go                                — single-level glob
//	uvb76/cmd/**/main.go, uvb76/cmd/**/*               — recursive glob
//
// Rejected forms:
//
//	absolute paths (starting with '/')
//	parent-directory escape ("..")
//	empty pattern
//	consecutive "**" segments
func CompileInventoryPattern(pattern string) (*CompiledPattern, error) {
	norm, err := normalizePath(pattern)
	if err != nil {
		return nil, err
	}
	segments := strings.Split(norm, "/")
	parts := make([]string, 0, len(segments))
	for i, seg := range segments {
		if seg == "**" {
			if i != len(segments)-1 && segments[i+1] == "**" {
				return nil, &PatternError{Pattern: pattern, Reason: "consecutive ** segments"}
			}
			parts = append(parts, ".*")
			continue
		}
		if strings.Contains(seg, "*") {
			parts = append(parts, translateSegment(seg))
			continue
		}
		parts = append(parts, regexp.QuoteMeta(seg))
	}
	body := strings.Join(parts, "/")
	regex := regexp.MustCompile("^" + body + "$")
	kind := "exact"
	if strings.Contains(pattern, "**") {
		kind = "recursive"
	} else if strings.Contains(pattern, "*") {
		kind = "glob"
	}
	return &CompiledPattern{Pattern: norm, Regex: regex, Kind: kind}, nil
}

// PatternError is returned by CompileInventoryPattern when a pattern is rejected.
type PatternError struct {
	Pattern string
	Reason  string
}

func (e *PatternError) Error() string {
	return "pattern " + e.Pattern + ": " + e.Reason
}

// normalizePath normalizes a repository-relative inventory path.
//
// Forward slashes only. No leading slash. No trailing slash unless path is
// exactly "/". No parent-directory escape. Empty rejected.
func normalizePath(p string) (string, error) {
	if p == "" {
		return "", &PatternError{Pattern: p, Reason: "empty pattern"}
	}
	s := strings.ReplaceAll(p, "\\", "/")
	for strings.HasPrefix(s, "./") {
		s = s[2:]
	}
	if strings.HasPrefix(s, "/") {
		return "", &PatternError{Pattern: p, Reason: "absolute path"}
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == "" || seg == "." {
			continue
		}
		if seg == ".." {
			return "", &PatternError{Pattern: p, Reason: "parent-directory escape"}
		}
	}
	for strings.Contains(s, "//") {
		s = strings.ReplaceAll(s, "//", "/")
	}
	for len(s) > 1 && strings.HasSuffix(s, "/") {
		s = s[:len(s)-1]
	}
	return s, nil
}

// translateSegment translates a single path segment to a regex.
// '*' anywhere in the segment matches non-slash chars only.
func translateSegment(seg string) string {
	var b strings.Builder
	for i := 0; i < len(seg); i++ {
		c := seg[i]
		if c == '*' {
			b.WriteString("[^/]*")
		} else {
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return b.String()
}

// PathMatchesPattern returns true when candidate matches pattern.
//
// The candidate must be normalized forward-slash repository-relative path.
func PathMatchesPattern(candidate, pattern string) bool {
	if candidate == "" || pattern == "" {
		return false
	}
	compiled, err := CompileInventoryPattern(pattern)
	if err != nil {
		return false
	}
	norm := strings.ReplaceAll(candidate, "\\", "/")
	for strings.HasPrefix(norm, "./") {
		norm = norm[2:]
	}
	return compiled.Regex.MatchString(norm)
}

// IsAbsolutePath reports whether p is an absolute filesystem path.
func IsAbsolutePath(p string) bool {
	if p == "" {
		return false
	}
	if p[0] == '/' || p[0] == '\\' {
		return true
	}
	if len(p) >= 3 && p[1] == ':' && (p[2] == '/' || p[2] == '\\') {
		return true
	}
	return false
}

// PathMatcher compiles a list of patterns once and exposes a single match API.
type PathMatcher struct {
	patterns []*CompiledPattern
}

// NewPathMatcher compiles the given patterns into a single matcher.
func NewPathMatcher(patterns []string) (*PathMatcher, error) {
	pm := &PathMatcher{}
	for _, p := range patterns {
		c, err := CompileInventoryPattern(p)
		if err != nil {
			return nil, err
		}
		pm.patterns = append(pm.patterns, c)
	}
	return pm, nil
}

// Match returns the first pattern that matches candidate, or empty.
func (pm *PathMatcher) Match(candidate string) string {
	norm := NormalizedPath(candidate)
	for _, c := range pm.patterns {
		if c.Regex.MatchString(norm) {
			return c.Pattern
		}
	}
	return ""
}

// AnyMatch returns true if any pattern matches candidate.
func (pm *PathMatcher) AnyMatch(candidate string) bool {
	return pm.Match(candidate) != ""
}

// GlobWalk enumerates files matching a single recursive glob pattern.
// Returns repository-relative paths. Used for diagnostics only.
func GlobWalk(repoRoot, pattern string) ([]string, error) {
	compiled, err := CompileInventoryPattern(pattern)
	if err != nil {
		return nil, err
	}
	absGlob := filepath.Join(repoRoot, compiled.Pattern)
	matches, err := filepath.Glob(absGlob)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if rel, err := filepath.Rel(repoRoot, m); err == nil {
			out = append(out, NormalizedPath(rel))
		}
	}
	return out, nil
}
