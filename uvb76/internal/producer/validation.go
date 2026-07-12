package producer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// ValidationIssue is a single validation finding with severity and category.
type ValidationIssue struct {
	Severity string // "error" or "warning"
	Category string // short identifier ("contract", "inventory", "path", "binary", etc.)
	Surface  string // surface ID when applicable
	Message  string
}

// String formats the issue for diagnostics.
func (v ValidationIssue) String() string {
	if v.Surface != "" {
		return fmt.Sprintf("[%s/%s] surface=%s: %s", v.Severity, v.Category, v.Surface, v.Message)
	}
	return fmt.Sprintf("[%s/%s] %s", v.Severity, v.Category, v.Message)
}

// HasErrors returns true if any issue is severity=error.
func HasErrors(issues []ValidationIssue) bool {
	for _, i := range issues {
		if i.Severity == "error" {
			return true
		}
	}
	return false
}

// ValidateRegistry performs full validation of the registry:
//
//   - contract structural validation,
//   - inventory<->contract parity (1:1 by SurfaceID),
//   - closed-vocabulary checks (status, policies, binary policy),
//   - ACTIVE requires writer files + writer symbols + test file,
//   - PROSPECTIVE/PERSISTENCE_PROSPECTIVE_NO_WRITER forbids active writer claims,
//   - STATIC forbids runtime writer claims,
//   - committed_allowed enforcement for tracked files,
//   - binary policy asserts on inventory surface classification.
func ValidateRegistry(reg *Registry) []ValidationIssue {
	var issues []ValidationIssue

	// Track seen IDs for duplicate detection.
	seenContract := make(map[string]bool)
	for _, c := range reg.Contracts() {
		if seenContract[c.SurfaceID] {
			issues = append(issues, ValidationIssue{
				Severity: "error", Category: "contract",
				Surface:  c.SurfaceID,
				Message:  "duplicate surface_id",
			})
		}
		seenContract[c.SurfaceID] = true

		if err := c.Validate(); err != nil {
			issues = append(issues, ValidationIssue{
				Severity: "error", Category: "contract",
				Surface:  c.SurfaceID,
				Message:  err.Error(),
			})
		}

		// Contract<->inventory parity.
		s, ok := reg.SurfaceByID(c.SurfaceID)
		if !ok {
			issues = append(issues, ValidationIssue{
				Severity: "error", Category: "parity",
				Surface:  c.SurfaceID,
				Message:  "contract without inventory surface",
			})
			continue
		}

		// ACTIVE high-sensitivity requires a non-"none" sanitizer.
		// Binary profile surfaces (binary_policy=exact_hash_fixture) are exempt:
		// they preserve exact bytes and rely on hash equality instead of redaction.
		if c.Status == StatusActive &&
			s.Sensitivity == "high" &&
			c.BinaryPolicy != BinaryExactHashFixture {
			if c.Sanitizer == "" || c.Sanitizer == "none" {
				issues = append(issues, ValidationIssue{
					Severity: "error", Category: "sanitizer",
					Surface:  c.SurfaceID,
					Message:  "ACTIVE HIGH-sensitivity surface requires a typed sanitizer",
				})
			}
		}
		if c.Status == StatusActive && len(c.TestFiles) == 0 {
			issues = append(issues, ValidationIssue{
				Severity: "error", Category: "test",
				Surface:  c.SurfaceID,
				Message:  "ACTIVE surface must have at least one test file",
			})
		}
		if c.Status == StatusStatic && c.Sanitizer == "" {
			issues = append(issues, ValidationIssue{
				Severity: "error", Category: "sanitizer",
				Surface:  c.SurfaceID,
				Message:  "STATIC surface requires an explicit sanitizer declaration",
			})
		}
		if c.Status == StatusProspective &&
			c.PersistencePolicy != PersistenceProspectiveNoWriter {
			issues = append(issues, ValidationIssue{
				Severity: "error", Category: "persistence",
				Surface:  c.SurfaceID,
				Message:  "PROSPECTIVE surface requires persistence_policy=prospective_no_writer",
			})
		}
		if c.Status == StatusStatic && len(c.WriterFiles) > 0 {
			issues = append(issues, ValidationIssue{
				Severity: "error", Category: "writer",
				Surface:  c.SurfaceID,
				Message:  "STATIC surface cannot claim runtime writer files",
			})
		}
		if c.Status == StatusProspective &&
			(len(c.WriterFiles) > 0 || len(c.WriterSymbols) > 0) {
			issues = append(issues, ValidationIssue{
				Severity: "error", Category: "writer",
				Surface:  c.SurfaceID,
				Message:  "PROSPECTIVE surface cannot claim runtime writers",
			})
		}
	}

	// Inventory parity.
	for _, s := range reg.Surfaces() {
		c := reg.ContractByID(s.ID)
		if c == nil {
			issues = append(issues, ValidationIssue{
				Severity: "error", Category: "parity",
				Surface:  s.ID,
				Message:  "inventory surface without producer contract",
			})
			continue
		}
		if _, err := CompileInventoryPattern(s.Path); err != nil {
			issues = append(issues, ValidationIssue{
				Severity: "error", Category: "path",
				Surface:  s.ID,
				Message:  fmt.Sprintf("invalid inventory path pattern: %v", err),
			})
		}
	}

	return issues
}

// CheckInventoryCommittedAllowed walks the repository and reports every
// file matched by an inventory surface whose CommittedAllowed is false.
//
// Used by the gate to enforce the "prospective surface must not have
// committed artifacts" invariant.
func CheckInventoryCommittedAllowed(reg *Registry, repoRoot string) []ValidationIssue {
	var issues []ValidationIssue
	for _, s := range reg.Surfaces() {
		if s.CommittedAllowed {
			continue
		}
		matches, err := walkSurface(s.Path, repoRoot)
		if err != nil {
			issues = append(issues, ValidationIssue{
				Severity: "warning", Category: "inventory",
				Surface:  s.ID,
				Message:  fmt.Sprintf("pattern walk failed: %v", err),
			})
			continue
		}
		for _, m := range matches {
			issues = append(issues, ValidationIssue{
				Severity: "error", Category: "committed_allowed",
				Surface:  s.ID,
				Message:  fmt.Sprintf("matched path %s has committed_allowed=false", m),
			})
		}
	}
	return issues
}

// walkSurface enumerates repo-relative files matching the surface path pattern.
// Only implemented for exact paths and recursive globs (no single-level
// globs since we walk the tree).
func walkSurface(pattern, repoRoot string) ([]string, error) {
	compiled, err := CompileInventoryPattern(pattern)
	if err != nil {
		return nil, err
	}
	if compiled.Kind == "exact" {
		abs := filepath.Join(repoRoot, pattern)
		if st, err := os.Stat(abs); err == nil && !st.IsDir() {
			return []string{NormalizedPath(pattern)}, nil
		}
		return nil, nil
	}
	var out []string
	err = filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "zig-cache" ||
				name == "zig-out" || name == ".zig-cache" || name == "dist" || name == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return nil
		}
		rel = NormalizedPath(rel)
		if compiled.Regex.MatchString(rel) {
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}
