package producer

// ACT-UVB76-HULK05R4R1: ValidateCatalog accepts explicit inputs and
// performs canonical-source validation plus per-file/symbol/test checks.
//
// This replaces ValidateRegistry's reliance on package-level globals with
// a pure-functional API: callers pass surfaces + contracts explicitly and
// Get back a list of issues. Tests build local inputs and call this
// production validator end-to-end.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// CatalogValidationIssue is one catalog validation finding.
type CatalogValidationIssue struct {
	Severity  string
	Category  string
	SurfaceID string
	File      string
	Symbol    string
	Message   string
}

func (i CatalogValidationIssue) String() string {
	var b strings.Builder
	b.WriteString("[")
	b.WriteString(i.Severity)
	b.WriteString("/")
	b.WriteString(i.Category)
	if i.SurfaceID != "" {
		b.WriteString("]")
		b.WriteString(" surface=")
		b.WriteString(i.SurfaceID)
	} else {
		b.WriteString("]")
	}
	if i.File != "" {
		b.WriteString(" file=")
		b.WriteString(i.File)
	}
	if i.Symbol != "" {
		b.WriteString(" symbol=")
		b.WriteString(i.Symbol)
	}
	if i.Message != "" {
		b.WriteString(": ")
		b.WriteString(i.Message)
	}
	return b.String()
}

// ValidateCatalog validates the catalog against canonical rules plus
// file/symbol/test existence and reference checks for ACTIVE contracts.
//
// Required drift failures:
//
//   - missing surface (empty id/path/producer)
//   - unknown surface status / sensitivity / binary / persistence policy
//   - duplicate surface id
//   - path mismatch (path must compile as a valid inventory pattern)
//   - producer mismatch (empty producer)
//   - sanitizer mismatch (HIGH ACTIVE requires typed sanitizer)
//   - sensitivity mismatch
//   - committed_allowed type
//   - status mismatch (ACTIVE requires writer_files unless exact_hash_fixture)
//   - policy mismatch
//   - missing writer file
//   - missing test file
//   - invented writer symbol (symbol not declared in any writer file)
//   - symbol located in undeclared file
//   - writer file with no registered surface
//   - active contract with no verified persistence point
//
// The validator never mutates inputs.
func ValidateCatalog(
	catalog *CanonicalCatalog,
	options ValidationOptions,
) []CatalogValidationIssue {
	if catalog == nil {
		return []CatalogValidationIssue{{
			Severity: "error", Category: "catalog",
			Message: "nil catalog",
		}}
	}
	var issues []CatalogValidationIssue

	// canonical-source validation.
	for _, msg := range catalog.ValidateCanonical() {
		issues = append(issues, CatalogValidationIssue{
			Severity:  "error",
			Category:  classifyCatalogError(msg),
			SurfaceID: surfaceIDFromCanonicalMessage(msg),
			Message:   msg,
		})
	}

	contracts, _ := catalog.ProjectContracts()
	contractBySurface := map[string]*ProducerContract{}
	for _, c := range contracts {
		contractBySurface[c.SurfaceID] = c
	}

	for _, s := range catalog.Surfaces {
		if s.Status != StatusActive {
			continue
		}
		if _, ok := contractBySurface[s.ID]; !ok {
			issues = append(issues, CatalogValidationIssue{
				Severity: "error", Category: "contract",
				SurfaceID: s.ID,
				Message:   "ACTIVE surface has no projected contract",
			})
			continue
		}

		// legacy_bypass surfaces have raw os.* writes; we only require
		// writer files to exist (real current state). The bypass detector
		// will produce findings for them.
		if s.EnforcementState == EnforcementStateLegacyBypass {
			if options.CheckWriterFilesExist {
				for _, wf := range s.WriterFiles {
					if !repoFileExists(options.RepoRoot, wf) {
						issues = append(issues, CatalogValidationIssue{
							Severity: "error", Category: "writer_file",
							SurfaceID: s.ID, File: wf,
							Message: "missing writer file",
						})
					}
				}
			}
			continue
		}

		// migrated surfaces (or any non-legacy state) enforce strict
		// writer-symbol resolution and test-file references.
		if options.CheckWriterFilesExist {
			for _, wf := range s.WriterFiles {
				if !repoFileExists(options.RepoRoot, wf) {
					issues = append(issues, CatalogValidationIssue{
						Severity: "error", Category: "writer_file",
						SurfaceID: s.ID, File: wf,
						Message: "missing writer file",
					})
				}
			}
		}

		// Writer symbol resolution.
		if options.CheckWriterSymbolsExist && options.CheckWriterFilesExist {
			fileToSymbols := map[string]map[string]bool{}
			for _, wf := range s.WriterFiles {
				syms, err := collectDeclaredFuncs(options.RepoRoot, wf)
				if err != nil {
					continue
				}
				fileToSymbols[NormalizedPath(wf)] = syms
			}
			for _, sym := range s.WriterSymbols {
				_, found := locateDeclaredSymbol(fileToSymbols, sym)
				if !found {
					issues = append(issues, CatalogValidationIssue{
						Severity: "error", Category: "writer_symbol",
						SurfaceID: s.ID, Symbol: sym,
						Message: "invented writer symbol",
					})
				}
			}

			// Each declared WriterFile should contain at least one declared
			// writer symbol or be a shared helper file. We warn (not error)
			// because some producer files have multiple writers.
			for wf, syms := range fileToSymbols {
				if len(syms) == 0 {
					issues = append(issues, CatalogValidationIssue{
						Severity: "warning", Category: "writer_file",
						SurfaceID: s.ID, File: wf,
						Message: "writer file declares no functions",
					})
					continue
				}
				if !symbolSetContainsAny(syms, s.WriterSymbols) {
					issues = append(issues, CatalogValidationIssue{
						Severity: "warning", Category: "writer_file",
						SurfaceID: s.ID, File: wf,
						Message: "writer file does not contain a declared writer symbol",
					})
				}
			}
		}

		// Test file existence and content reference.
		if options.CheckTestFilesExist {
			for _, tf := range s.TestFiles {
				if !repoFileExists(options.RepoRoot, tf) {
					issues = append(issues, CatalogValidationIssue{
						Severity: "error", Category: "test_file",
						SurfaceID: s.ID, File: tf,
						Message: "missing test file",
					})
					continue
				}
				if !testFileReferences(options.RepoRoot, tf, s.WriterSymbols, s.ID) {
					issues = append(issues, CatalogValidationIssue{
						Severity: "warning", Category: "test_file",
						SurfaceID: s.ID, File: tf,
						Message: "test file does not reference producer boundary or writer symbol",
					})
				}
			}
		}

		// ACTIVE without verified persistence point.
		if len(s.WriterSymbols) == 0 {
			issues = append(issues, CatalogValidationIssue{
				Severity: "error", Category: "writer_symbol",
				SurfaceID: s.ID,
				Message:   "active contract with no verified persistence point",
			})
		}
	}

	return issues
}

// classifyCatalogError maps a ValidateCanonical error message to a drift-failure
// category.
func classifyCatalogError(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "missing surface"):
		return "missing_surface"
	case strings.Contains(lower, "duplicate surface"):
		return "duplicate_surface"
	case strings.Contains(lower, "unknown surface status"):
		return "status_mismatch"
	case strings.Contains(lower, "sensitivity mismatch"):
		return "sensitivity_mismatch"
	case strings.Contains(lower, "policy mismatch"):
		return "policy_mismatch"
	case strings.Contains(lower, "path mismatch"):
		return "path_mismatch"
	case strings.Contains(lower, "sanitizer mismatch"):
		return "sanitizer_mismatch"
	case strings.Contains(lower, "producer mismatch"):
		return "producer_mismatch"
	case strings.Contains(lower, "status mismatch"):
		return "status_mismatch"
	}
	return "catalog"
}

// surfaceIDFromCanonicalMessage extracts the surface id from a ValidateCanonical
// error message when one is present.
func surfaceIDFromCanonicalMessage(msg string) string {
	for _, prefix := range []string{"missing surface: empty path (", "missing surface: empty producer (", "duplicate surface: "} {
		if strings.HasPrefix(msg, prefix) {
			id := strings.TrimPrefix(msg, prefix)
			if idx := strings.Index(id, ")"); idx > 0 {
				id = id[:idx]
			}
			return id
		}
	}
	for _, prefix := range []string{
		"unknown surface status: ",
		"sensitivity mismatch: ",
		"path mismatch: ",
		"sanitizer mismatch: ",
		"policy mismatch: ",
		"status mismatch: ",
		"missing surface justification: ",
		"producer mismatch: ",
	} {
		if strings.HasPrefix(msg, prefix) {
			rest := strings.TrimPrefix(msg, prefix)
			if idx := strings.Index(rest, " "); idx > 0 {
				return rest[:idx]
			}
			return rest
		}
	}
	return ""
}

// repoFileExists reports whether rel exists under options.RepoRoot.
func repoFileExists(repoRoot, rel string) bool {
	if repoRoot == "" || rel == "" {
		return false
	}
	abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
	info, err := os.Stat(abs)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// collectDeclaredFuncs parses a Go file and returns the set of declared
// function and method names.
func collectDeclaredFuncs(repoRoot, rel string) (map[string]bool, error) {
	abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
	srcBytes, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, rel, srcBytes, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, d := range f.Decls {
		v, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if v.Recv != nil && len(v.Recv.List) > 0 {
			switch r := v.Recv.List[0].Type.(type) {
			case *ast.StarExpr:
				if id, ok := r.X.(*ast.Ident); ok {
					out[id.Name+"."+v.Name.Name] = true
				}
			case *ast.Ident:
				out[r.Name+"."+v.Name.Name] = true
			}
			continue
		}
		out[v.Name.Name] = true
	}
	return out, nil
}

// locateDeclaredSymbol reports whether sym is declared in any of the files.
func locateDeclaredSymbol(fileToSymbols map[string]map[string]bool, sym string) (string, bool) {
	if sym == "" {
		return "", false
	}
	for f, syms := range fileToSymbols {
		if syms[sym] {
			return f, true
		}
	}
	return "", false
}

// symbolSetContainsAny reports whether the file's symbol set contains any of
// the wanted symbols.
func symbolSetContainsAny(have map[string]bool, want []string) bool {
	for _, w := range want {
		if w == "" {
			continue
		}
		if have[w] {
			return true
		}
	}
	return false
}

// testFileReferences reports whether the test file at rel references one of
// the writer symbols or the surface id (allowing either matcher).
func testFileReferences(repoRoot, rel string, writerSymbols []string, surfaceID string) bool {
	if repoRoot == "" || rel == "" {
		return false
	}
	abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
	body, err := os.ReadFile(abs)
	if err != nil {
		return false
	}
	if strings.HasSuffix(rel, "_test.go") {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, rel, body, parser.SkipObjectResolution)
		if err == nil {
			for _, d := range f.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				if mentionsSymbol(fn.Body, writerSymbols, surfaceID) {
					return true
				}
			}
		}
	}
	for _, sym := range writerSymbols {
		if sym != "" && bytesContains(body, sym) {
			return true
		}
	}
	if surfaceID != "" && bytesContains(body, surfaceID) {
		return true
	}
	return false
}

// mentionsSymbol walks an AST expression looking for any of the wanted names
// or the surface id as an identifier.
func mentionsSymbol(expr ast.Node, wanted []string, surfaceID string) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		for _, w := range wanted {
			if id.Name == w {
				found = true
				return false
			}
		}
		if surfaceID != "" && id.Name == surfaceID {
			found = true
			return false
		}
		return true
	})
	return found
}

func bytesContains(haystack []byte, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	return strings.Contains(string(haystack), needle)
}
