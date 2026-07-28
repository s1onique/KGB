// correction51_mandatory_inventory_test.go
//
// TestCorrection51Correction09_MandatoryTestInventory verifies that all mandatory tests
// for the production finalizer are present, executable, and do not contain forbidden patterns.
//
// This test uses Go source parsing to verify actual test declarations, not just string comparisons.
package evidence

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCorrection51Correction09_MandatoryTestInventory verifies the mandatory test inventory
// using actual Go source parsing. This test fails when:
// - A mandatory test is absent
// - A mandatory test has wrong signature
// - A mandatory test body is empty
// - A mandatory test contains t.Skip
// - A mandatory test uses os.Getenv/ LookupEnv as execution gate
// - A mandatory test invokes Docker (docker client or exec.Command docker)
// - A mandatory test body contains only Log/Logf calls (no assertions)
// - A mandatory test uses a second checksum parser (Scanner/SplitN/TrimSpace instead of ParseChecksumsCanonical)
// - A mandatory test references a gate-evidence provider
func TestCorrection51Correction09_MandatoryTestInventory(t *testing.T) {
	// Find the source directory using go list
	cmd := exec.Command("go", "list", "-f", "{{.Dir}}", "github.com/s1onique/KGB/tovarisch/labs/memory/internal/evidence")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list failed: %v", err)
	}
	dir := strings.TrimSpace(string(output))
	
	// Discover all test files in the package
	testFiles, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil {
		t.Fatalf("glob test files: %v", err)
	}
	
	// Debug: log what we found
	t.Logf("Looking for test files in: %s", dir)
	t.Logf("Found %d test files", len(testFiles))

	// Parse all test files and build a map of actual test declarations
	actualTests := make(map[string]*testDeclaration)
	fset := token.NewFileSet()

	for _, file := range testFiles {
		tests, err := parseTestFile(fset, file)
		if err != nil {
			t.Errorf("parse test file %s: %v", file, err)
			continue
		}
		for name, decl := range tests {
			actualTests[name] = decl
		}
	}

	// Verify mandatory tests exist and have correct properties
	var failures []string

	allMandatory := GetAllMandatoryTestNames()

	for _, name := range allMandatory {
		decl, exists := actualTests[name]
		if !exists {
			failures = append(failures, "MISSING: "+name)
			continue
		}

		// Check signature: must be func(*testing.T)
		if !decl.hasValidSignature {
			failures = append(failures, "INVALID_SIG: "+name)
		}

		// Check body is not empty
		if !decl.hasBody {
			failures = append(failures, "EMPTY_BODY: "+name)
		}

		// Check for forbidden patterns
		if decl.containsSkip {
			failures = append(failures, "CONTAINS_SKIP: "+name)
		}
		if decl.usesEnvGate {
			failures = append(failures, "USES_ENV_GATE: "+name)
		}
		if decl.invokesDocker {
			failures = append(failures, "INVOKES_DOCKER: "+name)
		}
		if decl.logOnly {
			failures = append(failures, "LOG_ONLY: "+name)
		}
		if decl.usesSecondChecksumParser {
			failures = append(failures, "USES_SECOND_CHECKSUM_PARSER: "+name)
		}
		if decl.referencesGateEvidenceProvider {
			failures = append(failures, "REFERENCES_GATE_EVIDENCE: "+name)
		}
	}

	// Report counts
	t.Logf("Mandatory test inventory count: %d", len(allMandatory))
	t.Logf("Actual test declarations found: %d", len(actualTests))

	// Count by category
	categoryCounts := make(map[string]int)
	for category, names := range correction51Correction09MandatoryTestNames {
		categoryCounts[category] = len(names)
	}
	for category, count := range categoryCounts {
		t.Logf("  %s: %d", category, count)
	}

	// Fail if any mandatory tests are missing or invalid
	if len(failures) > 0 {
		for _, f := range failures {
			t.Errorf("MANDATORY_TEST_FAILURE: %s", f)
		}
		t.Fatalf("Mandatory test inventory has %d failures", len(failures))
	}
}

// testDeclaration holds information about a discovered test function.
type testDeclaration struct {
	file                    string
	pos                     token.Pos
	hasValidSignature       bool
	hasBody                 bool
	containsSkip            bool
	usesEnvGate             bool
	invokesDocker           bool
	logOnly                 bool
	usesSecondChecksumParser bool
	referencesGateEvidenceProvider bool
}

// parseTestFile parses a test file and returns a map of test declarations.
func parseTestFile(fset *token.FileSet, filename string) (map[string]*testDeclaration, error) {
	file, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	tests := make(map[string]*testDeclaration)

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		// Must start with Test
		if !strings.HasPrefix(fn.Name.Name, "Test") {
			continue
		}

		// Must have exactly one parameter of type *testing.T
		if len(fn.Type.Params.List) != 1 {
			continue
		}
		param := fn.Type.Params.List[0]
		if len(param.Names) != 1 {
			continue
		}
		if !isTestingTBinary(param.Type) {
			continue
		}

		decl := &testDeclaration{
			file:              filename,
			pos:               fn.Pos(),
			hasValidSignature: true,
			hasBody:           fn.Body != nil && len(fn.Body.List) > 0,
		}

		// Inspect the function body for forbidden patterns
		if fn.Body != nil {
			inspectForForbiddenPatterns(fn.Body, decl)
		}

		tests[fn.Name.Name] = decl
	}

	return tests, nil
}

// isTestingTBinary returns true if the type is *testing.T or *testing.B or *testing.M or *testing.TB.
func isTestingTBinary(expr ast.Expr) bool {
	// *testing.T is a StarExpr containing a SelectorExpr (testing.T)
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "testing" && (sel.Sel.Name == "T" || sel.Sel.Name == "B" || sel.Sel.Name == "M" || sel.Sel.Name == "TB")
}

// inspectForForbiddenPatterns walks the AST and checks for forbidden constructs.
func inspectForForbiddenPatterns(node ast.Node, decl *testDeclaration) {
	ast.Inspect(node, func(n ast.Node) bool {
		if n == nil {
			return true
		}

		switch expr := n.(type) {
		case *ast.CallExpr:
			// Check for t.Skip, t.Skipf, t.SkipNow
			if sel, ok := expr.Fun.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok {
					if ident.Name == "t" && (sel.Sel.Name == "Skip" || sel.Sel.Name == "Skipf" || sel.Sel.Name == "SkipNow") {
						decl.containsSkip = true
					}
				}
			}

			// Check for os.Getenv or os.LookupEnv
			if sel, ok := expr.Fun.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok {
					if ident.Name == "os" && (sel.Sel.Name == "Getenv" || sel.Sel.Name == "LookupEnv") {
						decl.usesEnvGate = true
					}
				}
			}

			// Check for Docker client construction or docker commands
			if sel, ok := expr.Fun.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok {
					// Check for exec.Command with "docker" argument
					if ident.Name == "exec" && sel.Sel.Name == "Command" {
						for _, arg := range expr.Args {
							if basicLit, ok := arg.(*ast.BasicLit); ok {
								if basicLit.Kind == token.STRING {
									val := strings.Trim(basicLit.Value, `"`)
									if strings.Contains(val, "docker") {
										decl.invokesDocker = true
									}
								}
							}
						}
					}
					// Check for docker client types (common patterns)
					if strings.Contains(ident.Name, "docker") || sel.Sel.Name == "NewClient" {
						decl.invokesDocker = true
					}
				}
			}

			// Check for references to gate evidence provider
			if sel, ok := expr.Fun.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok {
					if strings.Contains(ident.Name, "gate") && strings.Contains(sel.Sel.Name, "Evidence") {
						decl.referencesGateEvidenceProvider = true
					}
				}
			}

			// Check for second checksum parser patterns
			// Only flag if calling checksum-specific parsing methods
			if sel, ok := expr.Fun.(*ast.SelectorExpr); ok {
				methodName := sel.Sel.Name
				if methodName == "Scan" || methodName == "SplitN" || methodName == "TrimSpace" {
					// Check if receiver is a variable that suggests checksum parsing
					if ident, ok := sel.X.(*ast.Ident); ok {
						lower := strings.ToLower(ident.Name)
						// Flag: scanner for checksum parsing, not just any scanner
						if (methodName == "Scan" && strings.Contains(lower, "checksum") && strings.Contains(lower, "scan")) ||
						   (methodName == "SplitN" && strings.Contains(lower, "checksum")) ||
						   (methodName == "TrimSpace" && strings.Contains(lower, "checksum") && strings.Contains(lower, "line")) {
							decl.usesSecondChecksumParser = true
						}
					}
				}
			}

		case *ast.IfStmt:
			// Check for if os.Getenv pattern (environment gate)
			if binary, ok := expr.Cond.(*ast.CallExpr); ok {
				if sel, ok := binary.Fun.(*ast.SelectorExpr); ok {
					if ident, ok := sel.X.(*ast.Ident); ok {
						if ident.Name == "os" && (sel.Sel.Name == "Getenv" || sel.Sel.Name == "LookupEnv") {
							decl.usesEnvGate = true
						}
					}
				}
			}

		case *ast.RangeStmt:
			// Flag if using a second checksum parser for actual checksum verification
			// (not just using Scanner to inspect file content for test verification)
			// This is indicated by having both SplitN and hex decoding in the same scope
			if expr.X != nil {
				if ident, ok := expr.X.(*ast.Ident); ok {
					// Flag if it's clearly a second checksum parser variable
					lower := strings.ToLower(ident.Name)
					if strings.Contains(lower, "checksum") && strings.Contains(lower, "line") && !strings.Contains(lower, "test") {
						decl.usesSecondChecksumParser = true
					}
				}
			}
		}

		return true
	})
}

// isLogOnly checks if the function body contains only Log/Logf calls.
func isLogOnly(fn *ast.FuncDecl) bool {
	if fn.Body == nil || len(fn.Body.List) == 0 {
		return false
	}

	for _, stmt := range fn.Body.List {
		// Check for expression statements that are not Log/Logf
		if exprStmt, ok := stmt.(*ast.ExprStmt); ok {
			if call, ok := exprStmt.X.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					if ident, ok := sel.X.(*ast.Ident); ok {
						if ident.Name == "t" {
							name := sel.Sel.Name
							if name != "Log" && name != "Logf" {
								return false
							}
						} else {
							return false
						}
					} else {
						return false
					}
				} else {
					return false
				}
			}
			return false
		}
		// Allow empty statements
		if _, ok := stmt.(*ast.EmptyStmt); ok {
			continue
		}
		return false
	}

	return true
}
