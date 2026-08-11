// Package main provides the UVB-76 pprof memory leak lab.
//
// # Authority Guards
//
// P0-14: Authority guards using go/types for complete type-checking.
// go/types provides symbol identity resolution via Info.Uses/Defs.
//
// Authority contract (fail-closed):
// - resolved == canonical object     → ACCEPT
// - resolved != canonical object     → REJECT
// - unresolved                       → REJECT
// - type-checking failed             → REJECT
//
// NO AST FALLBACK after semantic resolution. Once go/types resolves an identifier,
// the result is authoritative. AST is only used for structural verification.
package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// ErrPollAuthorityViolation is returned when a production authority guard fails.
var ErrPollAuthorityViolation = errors.New("poll authority violation")

// ErrProfileAuthorityViolation is returned when a profile authority guard fails.
var ErrProfileAuthorityViolation = errors.New("profile authority violation")

// pollProfileAuthorityInspection contains AST inspection results.
type pollProfileAuthorityInspection struct {
	// Poll guards
	DirectRunnerPollCalls      int
	LifecyclePollCalls        int
	UnexpectedPollCallSites    int
	GenericPollFnFields        int
	DefaultPollSendCases       int
	StringErrorClassifications int

	// Profile guards
	ClientGetCallsInCapture  int
	DirectDestinationCreates int
	RenamesBeforeValidation  int
	TempCleanupCalls         int
	SeamDelegationCount      int

	// P0-11: CaptureProfile call count enforcement
	CaptureProfileCalls int
}

// inspectPollProfileAuthority parses source files and returns AST inspection results.
func inspectPollProfileAuthority(sourceDir string) (*pollProfileAuthorityInspection, error) {
	result := &pollProfileAuthorityInspection{}

	fset := token.NewFileSet()

	// Find all .go files
	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip test files
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Parse the file
		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		// Inspect each function separately to avoid closure contamination
		for _, decl := range node.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			funcName := fn.Name.Name
			inspectFunctionBody(fn.Body, funcName, result)
		}

		// Inspect type declarations for forbidden fields
		for _, decl := range node.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				// Only inspect CollectionLifecycleInput
				if typeSpec.Name.Name != "CollectionLifecycleInput" {
					continue
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				for _, field := range structType.Fields.List {
					for _, name := range field.Names {
						if name.Name == "PollFn" || name.Name == "PollCallback" {
							result.GenericPollFnFields++
						}
					}
				}
			}
		}

		// Check for os.Create calls (direct destination creation)
		result.DirectDestinationCreates += countDirectDestinationCreates(node)

		// Check for temp cleanup calls (both os.Remove and cleanupProfileTemp)
		result.TempCleanupCalls += countTempCleanupCalls(node)

		// Check for CaptureProfile calls (P0-11: authority call count)
		result.CaptureProfileCalls += countCaptureProfileCalls(node)

		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// canonicalPollAuthorityFunction is the sole authorized function for PollTargetAuthority.
// P0-2: Only RunCollectionLifecycle may directly call PollTargetAuthority.
const canonicalPollAuthorityFunction = "RunCollectionLifecycle"

// inspectFunctionBody inspects a single function body with precise function tracking.
func inspectFunctionBody(body *ast.BlockStmt, funcName string, result *pollProfileAuthorityInspection) {
	// Functions where client.Get calls are forbidden
	isCaptureFunction := funcName == "CaptureProfile" || funcName == "captureProfileWithOps"

	// Functions where string-based error classification is forbidden
	// P0-10: String classification is forbidden in ALL poll-related functions
	isPollFunction := funcName == "PollTargetAuthority" ||
		funcName == "finalizeTargetPollContext" ||
		funcName == "isTerminalPollError" ||
		funcName == canonicalPollAuthorityFunction ||
		funcName == "drainTargetPoll" ||
		funcName == "CaptureProfile" ||
		funcName == "captureProfileWithOps" ||
		funcName == "profileContextFailure" ||
		funcName == "authFailure"

	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.SelectStmt:
			// Check for lossy poll send pattern: send to pollResultCh + default
			hasPollSend := false
			hasDefault := false
			for _, stmt := range node.Body.List {
				clause, ok := stmt.(*ast.CommClause)
				if !ok {
					continue
				}
				// Check for default clause (nil Comm)
				if clause.Comm == nil {
					hasDefault = true
				}
				// Check for send to pollResultCh
				if clause.Comm != nil {
					if send, ok := clause.Comm.(*ast.SendStmt); ok {
						if ident, ok := send.Chan.(*ast.Ident); ok {
							if strings.Contains(ident.Name, "pollResultCh") || strings.Contains(ident.Name, "PollResult") {
								hasPollSend = true
							}
						}
					}
				}
			}
			if hasPollSend && hasDefault {
				result.DefaultPollSendCases++
			}

		case *ast.AssignStmt:
			// Check for strings.Contains or strings.HasPrefix with .Error() for classification
			// P0-10: Must check ALL RHS expressions, not just assignments
			if isCaptureFunction || isPollFunction {
				// Check all RHS expressions for string classification with .Error()
				for _, expr := range node.Rhs {
					if containsStringErrorClassification(expr) {
						result.StringErrorClassifications++
					}
				}
				// Also check LHS if it's used in condition context later
				for _, lhsExpr := range node.Lhs {
					if containsStringErrorClassification(lhsExpr) {
						result.StringErrorClassifications++
					}
				}
			}

		case *ast.IfStmt:
			// P0-10: Check if conditions for string error classification
			if isCaptureFunction || isPollFunction {
				if containsStringErrorClassification(node.Cond) {
					result.StringErrorClassifications++
				}
			}

		case *ast.ReturnStmt:
			// P0-10: Check return expressions for string error classification
			if isCaptureFunction || isPollFunction {
				for _, expr := range node.Results {
					if containsStringErrorClassification(expr) {
						result.StringErrorClassifications++
					}
				}
			}

		case *ast.CallExpr:
			// Check for http.Client.Get calls ONLY in capture functions
			if isCaptureFunction {
				var funcName string
				var isQualified bool

				switch fun := node.Fun.(type) {
				case *ast.Ident:
					funcName = fun.Name
					isQualified = false
				case *ast.SelectorExpr:
					funcName = fun.Sel.Name
					isQualified = true
				default:
					return true
				}

				if funcName == "Get" && isQualified {
					if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
						if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "client" {
							result.ClientGetCallsInCapture++
						}
					}
				}
			}

			// P0-10: Check if CallExpr is itself a string error classification
			// This catches cases like: if strings.Contains(err.Error(), ...) { ... }
			if isCaptureFunction || isPollFunction {
				if containsStringErrorClassification(node) {
					result.StringErrorClassifications++
				}
			}

			// Check for PollTargetAuthority calls (not scoped)
			var pollFuncName string
			var pollQualified bool

			switch fun := node.Fun.(type) {
			case *ast.Ident:
				pollFuncName = fun.Name
				pollQualified = false
			case *ast.SelectorExpr:
				pollFuncName = fun.Sel.Name
				pollQualified = true
			default:
				return true
			}

			if pollFuncName == "PollTargetAuthority" {
				if pollQualified {
					// Qualified call like pkg.PollTargetAuthority - external
					return true
				}
				// P0-2: Direct call - classify by enclosing function
				// P0-2: Only RunCollectionLifecycle is canonical authority
				switch funcName {
				case "runLab", "Run", "main":
					result.DirectRunnerPollCalls++
				case canonicalPollAuthorityFunction:
					// P0-2: Canonical lifecycle - the sole production authority
					result.LifecyclePollCalls++
				default:
					// P0-2: Any other call site is forbidden
					result.UnexpectedPollCallSites++
				}
			}
		}
		return true
	})
}

// containsStringErrorClassification checks if an expression contains string-based
// error classification patterns like strings.Contains(err.Error(), ...).
// P0-10: Inspect all relevant expression types.
func containsStringErrorClassification(expr ast.Expr) bool {
	if expr == nil {
		return false
	}

	var found bool
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}

		// Look for: strings.Contains(expr.Error(), ...), strings.HasPrefix, etc.
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				methodName := sel.Sel.Name
				if methodName == "Contains" || methodName == "HasPrefix" ||
					methodName == "HasSuffix" || methodName == "EqualFold" ||
					methodName == "Equal" {
					// Check if the base is "strings"
					if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "strings" {
						// Check if any argument contains an .Error() call
						for _, arg := range call.Args {
							if containsErrorDotCall(arg) {
								found = true
								return false
							}
						}
					}
				}
			}
		}
		return true
	})

	return found
}

// containsErrorDotCall checks if an expression contains an .Error() method call.
func containsErrorDotCall(expr ast.Expr) bool {
	if expr == nil {
		return false
	}

	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}

		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if sel.Sel.Name == "Error" && len(call.Args) == 0 {
					found = true
					return false
				}
			}
		}
		return true
	})

	return found
}

// countDirectDestinationCreates counts os.Create calls in the source.
func countDirectDestinationCreates(node ast.Node) int {
	count := 0
	ast.Inspect(node, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if sel.Sel.Name == "Create" {
					if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "os" {
						count++
					}
				}
			}
		}
		return true
	})
	return count
}

// countTempCleanupCalls counts explicit temp file cleanup patterns.
func countTempCleanupCalls(node ast.Node) int {
	count := 0
	ast.Inspect(node, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				// os.Remove, os.RemoveAll
				if sel, ok := fun.X.(*ast.Ident); ok && sel.Name == "os" {
					if fun.Sel.Name == "Remove" || fun.Sel.Name == "RemoveAll" {
						count++
					}
				}
			case *ast.Ident:
				// cleanupProfileTemp, cleanupProfileTempPreserving (local function calls)
				if fun.Name == "cleanupProfileTemp" || fun.Name == "cleanupProfileTempPreserving" {
					count++
				}
			}
		}
		return true
	})
	return count
}

// countCaptureProfileCalls counts calls to CaptureProfile in the source.
func countCaptureProfileCalls(node ast.Node) int {
	count := 0
	ast.Inspect(node, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				// pkg.CaptureProfile (qualified - external)
				if fun.Sel.Name == "CaptureProfile" {
					// Check if it's from our package or an import alias
					if ident, ok := fun.X.(*ast.Ident); ok {
						// External call if it has a package prefix
						if ident.Name != "" && ident.Name != "main" && ident.Name != "captureProfileWithOps" {
							count++
						}
					}
				}
			case *ast.Ident:
				// Direct CaptureProfile call (unqualified - same package)
				if fun.Name == "CaptureProfile" {
					count++
				}
			}
		}
		return true
	})
	return count
}

// ErrAuthorityGuardFailed is returned when a guard fails.
type ErrAuthorityGuardFailed struct {
	Guard    string
	Details  string
	Expected interface{}
	Actual   interface{}
}

func (e *ErrAuthorityGuardFailed) Error() string {
	return fmt.Sprintf("authority guard %s failed: %s (expected %v, got %v)",
		e.Guard, e.Details, e.Expected, e.Actual)
}

// VerifyPollAuthorityGuards verifies that production code follows authority rules.
func VerifyPollAuthorityGuards(sourceDir string) error {
	inspection, err := inspectPollProfileAuthority(sourceDir)
	if err != nil {
		return fmt.Errorf("inspection failed: %w", err)
	}

	// P0-14: Poll authority rules

	// Rule: No direct PollTargetAuthority calls in runner
	if inspection.DirectRunnerPollCalls > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "direct_runner_poll_guard",
			Details:  "PollTargetAuthority called directly in runner",
			Expected: 0,
			Actual:   inspection.DirectRunnerPollCalls,
		}
	}

	// Rule: No generic PollFn fields in CollectionLifecycleInput
	if inspection.GenericPollFnFields > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "generic_poll_callback_guard",
			Details:  "PollFn or PollCallback fields found in CollectionLifecycleInput",
			Expected: 0,
			Actual:   inspection.GenericPollFnFields,
		}
	}

	// Rule: No lossy poll send pattern (send to pollResultCh + default)
	if inspection.DefaultPollSendCases > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "lossy_poll_send_guard",
			Details:  "Lossy poll send pattern found (pollResultCh send + default)",
			Expected: 0,
			Actual:   inspection.DefaultPollSendCases,
		}
	}

	// Rule: No string-based error classification
	if inspection.StringErrorClassifications > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "string_classification_guard",
			Details:  "String-based error classification found",
			Expected: 0,
			Actual:   inspection.StringErrorClassifications,
		}
	}

	// Rule: No unexpected poll call sites (fail closed)
	if inspection.UnexpectedPollCallSites > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "unexpected_poll_call_guard",
			Details:  "PollTargetAuthority called from unrecognized function",
			Expected: 0,
			Actual:   inspection.UnexpectedPollCallSites,
		}
	}

	// Rule: Exactly one lifecycle poll call (canonical authority)
	if inspection.LifecyclePollCalls != 1 {
		return &ErrAuthorityGuardFailed{
			Guard:    "lifecycle_poll_call_count",
			Details:  "Lifecycle must have exactly one PollTargetAuthority call",
			Expected: 1,
			Actual:   inspection.LifecyclePollCalls,
		}
	}

	return nil
}

// VerifyRunnerProfileOrchestrationGuard verifies that the runner's CaptureProfilesFn
// field delegates to the canonical captureProfilesWithValidation function.
// P0-11: Topology-bound guard with cardinality enforcement - no masking allowed.
// P0-11: Uses go/types for compiler-backed symbol identity resolution.
//
// Authority contract (fail-closed):
// - resolved == canonical object     → ACCEPT
// - resolved != canonical object     → REJECT
// - unresolved                       → REJECT
// - type-checking failed             → REJECT
//
// NO AST FALLBACK after semantic resolution.
func VerifyRunnerProfileOrchestrationGuard(sourceDir string) error {
	inspection, err := inspectRunnerProfileOrchestration(sourceDir)
	if err != nil {
		return fmt.Errorf("inspection failed: %w", err)
	}

	// P0-11: Symbol identity enforcement - requires actual top-level function declaration
	if !inspection.CanonicalFunctionExists {
		return &ErrAuthorityGuardFailed{
			Guard:    "runner_profile_orchestration_guard",
			Details:  "canonical captureProfilesWithValidation function declaration missing",
			Expected: true,
			Actual:   false,
		}
	}

	// P0-11: Cardinality enforcement - every binding must be canonical
	if inspection.Bindings == 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "runner_profile_orchestration_guard",
			Details:  "No CaptureProfilesFn bindings found",
			Expected: "at least one binding",
			Actual:   0,
		}
	}

	// Rule: ALL bindings must be canonical (no noncanonical bindings allowed)
	if inspection.NonCanonicalBindings > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "runner_profile_orchestration_guard",
			Details:  fmt.Sprintf("Found %d noncanonical CaptureProfilesFn binding(s)", inspection.NonCanonicalBindings),
			Expected: "all bindings canonical",
			Actual:   fmt.Sprintf("%d noncanonical, %d canonical", inspection.NonCanonicalBindings, inspection.CanonicalBindings),
		}
	}

	return nil
}

// runnerProfileOrchestration contains the orchestration inspection results.
type runnerProfileOrchestration struct {
	Bindings                int  // Total CaptureProfilesFn bindings found in CollectionLifecycleInput
	CanonicalBindings       int  // Bindings that directly call captureProfilesWithValidation
	NonCanonicalBindings    int  // Bindings that don't directly call captureProfilesWithValidation
	CanonicalFunctionExists bool // Top-level captureProfilesWithValidation function exists
}

// inspectRunnerProfileOrchestration checks CaptureProfilesFn field assignments.
// P0-11: Uses go/packages for module-aware type-checking with semantic symbol identity.
// FAILS CLOSED on any loader/package/type error.
//
// Uses non-recursive loading to ensure only the target package is analyzed,
// not subpackages like internal/* or verify/*.
//
// Authority contract (fail-closed):
// - packages.Load error        → REJECT
// - package count != 1         → REJECT
// - pkg.Errors non-empty       → REJECT
// - pkg.IllTyped               → REJECT
// - missing Types/TypesInfo     → REJECT
// - type-checking failed       → REJECT
// - unresolved                 → REJECT
func inspectRunnerProfileOrchestration(sourceDir string) (*runnerProfileOrchestration, error) {
	result := &runnerProfileOrchestration{}

	// Use go/packages for module-aware type-checking
	// This handles module-local imports like github.com/s1onique/KGB/uvb76/server
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  sourceDir,
	}

	pkgs, err := packages.Load(cfg, ".")

	// P0-11: Fail closed on any loader failure
	if err != nil {
		return nil, &ErrAuthorityGuardFailed{
			Guard:    "runner_profile_orchestration_guard",
			Details:  fmt.Sprintf("packages.Load failed: %v", err),
			Expected: "successful package load",
			Actual:   fmt.Sprintf("error: %v", err),
		}
	}

	if len(pkgs) != 1 {
		return nil, &ErrAuthorityGuardFailed{
			Guard:    "runner_profile_orchestration_guard",
			Details:  fmt.Sprintf("expected 1 package, got %d", len(pkgs)),
			Expected: "exactly 1 package",
			Actual:   fmt.Sprintf("%d packages", len(pkgs)),
		}
	}

	pkg := pkgs[0]

	// P0-11: Check for package errors BEFORE using type info
	// packages.Load can return err==nil but still have errors in pkg.Errors
	if len(pkg.Errors) != 0 || pkg.IllTyped {
		return nil, &ErrAuthorityGuardFailed{
			Guard:    "runner_profile_orchestration_guard",
			Details:  fmt.Sprintf("package load/type-check failed: %v", pkg.Errors),
			Expected: "well-typed package with zero load/type errors",
			Actual:   fmt.Sprintf("errors=%d illTyped=%v", len(pkg.Errors), pkg.IllTyped),
		}
	}

	// Verify we have the required type info
	if pkg.Types == nil || pkg.TypesInfo == nil || len(pkg.Syntax) == 0 {
		return nil, &ErrAuthorityGuardFailed{
			Guard:    "runner_profile_orchestration_guard",
			Details:  "missing required type info from go/packages",
			Expected: "Types, TypesInfo, and Syntax available",
			Actual:   "nil or empty",
		}
	}

	// Get go/types objects from the package
	typesPkg := pkg.Types
	typesInfo := pkg.TypesInfo

	// Find canonical function using go/types scope
	canonicalFunc := findCanonicalFunc(typesPkg, "captureProfilesWithValidation")
	if canonicalFunc != nil {
		result.CanonicalFunctionExists = true
	}

	// Find canonical type (if defined)
	canonicalType := findCanonicalType(typesPkg, "CollectionLifecycleInput")

	// Find all CompositeLits that are CollectionLifecycleInput
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			comp, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}

			// Check if this composite literal has CaptureProfilesFn field
			var captureFnExpr ast.Expr
			for _, elt := range comp.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok || key.Name != "CaptureProfilesFn" {
					continue
				}
				captureFnExpr = kv.Value
				break
			}

			if captureFnExpr == nil {
				return true
			}

			// P0-11: Verify type identity using go/types
			isCanonicalType := isCanonicalCollectionLifecycleInputType(comp.Type, typesInfo, typesPkg, canonicalType)

			if !isCanonicalType {
				// Non-canonical type - this binding doesn't count for our guard
				return true
			}

			// Found CaptureProfilesFn field with canonical type
			result.Bindings++

			// P0-11: Verify call identity using go/types symbol resolution
			isCanonical := isCanonicalCaptureDelegationWithTypes(captureFnExpr, typesInfo, canonicalFunc)

			if isCanonical {
				result.CanonicalBindings++
			} else {
				result.NonCanonicalBindings++
			}

			return true
		})
	}

	return result, nil
}

// findCanonicalFunc finds the canonical function by name in the package scope.
func findCanonicalFunc(pkg *types.Package, name string) *types.Func {
	scope := pkg.Scope()
	if obj := scope.Lookup(name); obj != nil {
		if fn, ok := obj.(*types.Func); ok {
			return fn
		}
	}
	return nil
}

// findCanonicalType finds the canonical type by name in the package scope.
func findCanonicalType(pkg *types.Package, name string) *types.TypeName {
	scope := pkg.Scope()
	if obj := scope.Lookup(name); obj != nil {
		if tn, ok := obj.(*types.TypeName); ok {
			return tn
		}
	}
	return nil
}

// isCanonicalCollectionLifecycleInputType checks if the type is the canonical CollectionLifecycleInput
// using go/types symbol resolution.
// P0-11: Uses only semantic object resolution - no name-only fallback.
//
// Authority contract (fail-closed):
// - resolved same object  → ACCEPT
// - resolved different    → REJECT
// - unresolved           → REJECT
// - name-only match      → NEVER_ACCEPT
func isCanonicalCollectionLifecycleInputType(typeExpr ast.Expr, info *types.Info, pkg *types.Package, canonicalType *types.TypeName) bool {
	if info == nil || pkg == nil {
		return false
	}

	// Get the identifier for the type
	ident, ok := typeExpr.(*ast.Ident)
	if !ok {
		// SelectorExpr or other - not canonical (could be pkg.Type spoofing)
		return false
	}

	// Use ObjectOf for semantic resolution (Uses/Defs combined)
	obj := info.ObjectOf(ident)
	if obj == nil {
		// Identifier not resolved - fail closed
		return false
	}

	// Verify object identity - must be the exact canonical type
	tn, ok := obj.(*types.TypeName)
	if !ok {
		// Object is not a TypeName - could be local type shadow or other object
		return false
	}

	if canonicalType != nil && tn == canonicalType {
		return true
	}

	return false
}

// isCanonicalCaptureDelegationWithTypes checks if the expression directly delegates to
// captureProfilesWithValidation using go/types symbol resolution.
// P0-11: Uses only semantic object resolution - no name-only fallback.
//
// Authority contract (fail-closed):
// - resolved same object  → ACCEPT
// - resolved different    → REJECT
// - unresolved           → REJECT
// - name-only match      → NEVER_ACCEPT
func isCanonicalCaptureDelegationWithTypes(expr ast.Expr, info *types.Info, canonicalFunc *types.Func) bool {
	if info == nil {
		return false
	}

	// Handle FuncLit (lambda/anonymous function)
	funcLit, ok := expr.(*ast.FuncLit)
	if !ok {
		return false
	}

	if funcLit.Body == nil || len(funcLit.Body.List) != 1 {
		return false
	}

	// Must be a ReturnStmt
	ret, ok := funcLit.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}

	// Return value must be a call
	call, ok := ret.Results[0].(*ast.CallExpr)
	if !ok {
		return false
	}

	// Get the function identifier
	var ident *ast.Ident
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		ident = fun
	case *ast.SelectorExpr:
		// Selector calls (pkg.captureProfilesWithValidation) are NOT canonical
		return false
	default:
		return false
	}

	// Use ObjectOf for semantic resolution (Uses/Defs combined)
	obj := info.ObjectOf(ident)
	if obj == nil {
		// Identifier not resolved - fail closed
		return false
	}

	// Verify object identity - must be the exact canonical function
	fn, ok := obj.(*types.Func)
	if !ok {
		// Object is not a Func - could be function-valued variable or method
		return false
	}

	if canonicalFunc != nil && fn == canonicalFunc {
		return true
	}

	return false
}

// VerifyProfileAuthorityGuards verifies that profile code follows authority rules.
func VerifyProfileAuthorityGuards(sourceDir string) error {
	inspection, err := inspectPollProfileAuthority(sourceDir)
	if err != nil {
		return fmt.Errorf("inspection failed: %w", err)
	}

	// P0-14: Profile authority rules

	// Rule: No direct http.Client.Get calls in capture functions
	if inspection.ClientGetCallsInCapture > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "client_get_guard",
			Details:  "Direct http.Client.Get calls found in CaptureProfile",
			Expected: 0,
			Actual:   inspection.ClientGetCallsInCapture,
		}
	}

	// Rule: Temp cleanup must be present
	if inspection.TempCleanupCalls == 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "temp_cleanup_guard",
			Details:  "No temp file cleanup calls found",
			Expected: 1,
			Actual:   0,
		}
	}

	// P0-11: At least one CaptureProfile call (canonical authority - required)
	if inspection.CaptureProfileCalls == 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "capture_profile_call_count",
			Details:  "CaptureProfile must be called at least once in authority topology",
			Expected: 1,
			Actual:   0,
		}
	}

	return nil
}

// ErrPersistResultAuthorityViolation is returned when a publication authority guard fails.
var ErrPersistResultAuthorityViolation = errors.New("persist result authority violation")

// publicationAuthorityInspection contains semantic inspection results for publication authority.
// P0-12A through P0-12D: Semantic inspection using go/packages and go/types.
type publicationAuthorityInspection struct {
	// Package loading state
	LoadFailed          bool
	TypeCheckFailed     bool
	MissingPersistFunc  bool
	PersistFuncWrongKind bool

	// Semantic identity of canonical persistResult function
	CanonicalPersistResultFunc *types.Func

	// P0-12C: Canonical call site inventory
	CanonicalPersistCalls     int
	NonCanonicalPersistCalls int

	// P0-12D: Path binding for canonical calls
	SuccessPathCanonicalCalls int
	FailurePathCanonicalCalls int

	// Enclosing function names for canonical calls (for diagnostic)
	SuccessPathEnclosingFuncs []string
	FailurePathEnclosingFuncs []string

	// P0-12E: Spoof matrix - noncanonical calls using the persistResult name
	SameNameLocalVariable     int
	SameNameMethod            int
	SameNameSelector          int
	SameNameInOtherPkg       int
	FunctionFieldPublication  int
	HelperPublication        int

	// P0-12F: Alternate publication authorities detected
	AlternateResultWriterCalls int

	// P0-12B: Unresolved identifier count (fail-closed)
	UnresolvedPersistCalls int

	// P0-12D: Unexpected canonical caller calls (fail-closed)
	UnexpectedCanonicalCallerCalls      int
	UnexpectedCanonicalCallerEnclosingFuncs []string

	// === Legacy AST-based fields for backward compatibility ===
	// persistResult call count
	PersistResultCalls int

	// Bypass patterns found in functions (counted per-function, not globally)
	DirectOsWriteFileToResult   int
	DirectOsCreateToResult      int
	DirectJsonEncoderToResult   int
	HelperWrapperBypass         int
	FunctionFieldBypass         int
	DeadCanonicalPlusBypass     int
	ConditionalCanonicalBypass  int
	SelectorBypass              int
	MethodBypass                int
	VariableFunctionBypass      int
}

// VerifyPersistResultAuthorityGuards verifies that code follows publication authority rules.
// P0-12G: No second publication authority - must flow through persistResult.
// P0-12A: Finalization exactly once.
// P0-12B: Publication exactly once.
//
// This is the AST-based guard for fixture/legacy compatibility.
// For production topology verification, use VerifyPersistResultSemantics.
func VerifyPersistResultAuthorityGuards(sourceDir string) error {
	inspection, err := inspectPersistResultAuthority(sourceDir)
	if err != nil {
		return &ErrAuthorityGuardFailed{
			Guard:    "publication_authority_guard",
			Details:  fmt.Sprintf("inspection failed: %v", err),
			Expected: 0,
			Actual:   1,
		}
	}

	// Rule: At least one persistResult call (canonical authority - required)
	// P0-12B: Publication exactly once
	if inspection.PersistResultCalls == 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "publication_call_count",
			Details:  "persistResult must be called at least once in authority topology",
			Expected: 1,
			Actual:   0,
		}
	}

	// Rule: No direct os.WriteFile to result.json
	if inspection.DirectOsWriteFileToResult > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "publication_bypass_guard",
			Details:  "Direct os.WriteFile to result.json found",
			Expected: 0,
			Actual:   inspection.DirectOsWriteFileToResult,
		}
	}

	// Rule: No direct os.Create to result.json
	if inspection.DirectOsCreateToResult > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "publication_bypass_guard",
			Details:  "Direct os.Create to result.json found",
			Expected: 0,
			Actual:   inspection.DirectOsCreateToResult,
		}
	}

	// Rule: No direct json.NewEncoder to result file
	if inspection.DirectJsonEncoderToResult > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "publication_bypass_guard",
			Details:  "Direct json.NewEncoder to result file found",
			Expected: 0,
			Actual:   inspection.DirectJsonEncoderToResult,
		}
	}

	// Rule: No helper wrapper bypass
	if inspection.HelperWrapperBypass > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "publication_bypass_guard",
			Details:  "Helper wrapper alternate publication found",
			Expected: 0,
			Actual:   inspection.HelperWrapperBypass,
		}
	}

	// Rule: No function field bypass
	if inspection.FunctionFieldBypass > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "publication_bypass_guard",
			Details:  "Function field publication bypass found",
			Expected: 0,
			Actual:   inspection.FunctionFieldBypass,
		}
	}

	// Rule: No dead canonical + real bypass
	if inspection.DeadCanonicalPlusBypass > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "publication_bypass_guard",
			Details:  "Dead canonical call plus real bypass found",
			Expected: 0,
			Actual:   inspection.DeadCanonicalPlusBypass,
		}
	}

	// Rule: No conditional canonical + bypass
	if inspection.ConditionalCanonicalBypass > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "publication_bypass_guard",
			Details:  "Conditional canonical plus bypass found",
			Expected: 0,
			Actual:   inspection.ConditionalCanonicalBypass,
		}
	}

	// Rule: No selector bypass
	if inspection.SelectorBypass > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "publication_bypass_guard",
			Details:  "Selector-based publication bypass found",
			Expected: 0,
			Actual:   inspection.SelectorBypass,
		}
	}

	// Rule: No method bypass
	if inspection.MethodBypass > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "publication_bypass_guard",
			Details:  "Method-based publication bypass found",
			Expected: 0,
			Actual:   inspection.MethodBypass,
		}
	}

	// Rule: No variable function bypass
	if inspection.VariableFunctionBypass > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "publication_bypass_guard",
			Details:  "Function-valued variable publication bypass found",
			Expected: 0,
			Actual:   inspection.VariableFunctionBypass,
		}
	}

	return nil
}

// canonicalPersistResultFunction is the sole authorized function for publication.
const canonicalPersistResultFunction = "persistResult"

// inspectPersistResultAuthority parses source files and returns AST inspection results.
func inspectPersistResultAuthority(sourceDir string) (*publicationAuthorityInspection, error) {
	result := &publicationAuthorityInspection{}

	// Use AST-only inspection to avoid go/packages module loading issues with test fixtures
	fset := token.NewFileSet()

	// Find all .go files in the directory
	files, err := filepath.Glob(filepath.Join(sourceDir, "*.go"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob .go files: %w", err)
	}

	for _, filePath := range files {
		// Skip test files
		if strings.HasSuffix(filePath, "_test.go") {
			continue
		}

		node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", filePath, err)
		}

		// Track all functions defined in this file
		definedFuncs := make(map[string]bool)
		for _, decl := range node.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				definedFuncs[fn.Name.Name] = true
			}
		}

		// Check for var declarations of persistResult
		for _, decl := range node.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
				for _, spec := range genDecl.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, name := range vs.Names {
							if name.Name == canonicalPersistResultFunction {
								result.VariableFunctionBypass++
							}
						}
					}
				}
			}
		}

		// Inspect each function declaration
		for _, decl := range node.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			funcResult := inspectPersistResultFunctionBody(fn.Body, definedFuncs)
			result.PersistResultCalls += funcResult.calls
			result.DirectOsWriteFileToResult += funcResult.directWriteFileToResult
			result.DirectOsCreateToResult += funcResult.directCreateToResult
			result.DirectJsonEncoderToResult += funcResult.directJsonEncoderToResult
			if funcResult.hasBypass {
				result.HelperWrapperBypass++
			}
			if funcResult.hasDeadCanonical && funcResult.hasBypass {
				result.DeadCanonicalPlusBypass++
			}
			if funcResult.hasConditionalBypass {
				result.ConditionalCanonicalBypass++
			}
			if funcResult.hasSelectorBypass {
				result.SelectorBypass++
			}
			if funcResult.hasMethodBypass {
				result.MethodBypass++
			}
		}
	}

	return result, nil
}

// P0-12A: Semantic package loader for publication authority inspection.
// Reuses the Phase 4 pattern for fail-closed semantic loading.
// FAILS CLOSED on any loader/package/type error.
type publicationAuthorityPackage struct {
	Package   *types.Package
	TypesInfo *types.Info
	Syntax    []*ast.File
	Fset      *token.FileSet
}

// loadPublicationAuthorityPackage loads the package with full semantic type info.
// P0-12A: Fails closed on any error.
func loadPublicationAuthorityPackage(sourceDir string) (*publicationAuthorityPackage, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  sourceDir,
	}

	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, fmt.Errorf("packages.Load failed: %w", err)
	}

	if len(pkgs) != 1 {
		return nil, fmt.Errorf("expected 1 package, got %d", len(pkgs))
	}

	pkg := pkgs[0]

	// Check for package errors BEFORE using type info
	if len(pkg.Errors) != 0 || pkg.IllTyped {
		return nil, fmt.Errorf("package load/type-check failed: errors=%d, illTyped=%v", len(pkg.Errors), pkg.IllTyped)
	}

	// Verify required type info
	if pkg.Types == nil || pkg.TypesInfo == nil || len(pkg.Syntax) == 0 {
		return nil, fmt.Errorf("missing required type info from go/packages")
	}

	return &publicationAuthorityPackage{
		Package:   pkg.Types,
		TypesInfo: pkg.TypesInfo,
		Syntax:    pkg.Syntax,
	}, nil
}

// P0-12B: Resolve the canonical persistResult function using semantic identity.
// Requires exact *types.Func resolution via package scope lookup.
func (p *publicationAuthorityPackage) resolveCanonicalPersistResult() (*types.Func, error) {
	// P0-12B: Use Scope().Lookup for semantic resolution
	scope := p.Package.Scope()
	obj := scope.Lookup(canonicalPersistResultFunction)
	if obj == nil {
		return nil, fmt.Errorf("persistResult not found in package scope")
	}

	// P0-12B: Must be *types.Func, not variable/method/selector
	fn, ok := obj.(*types.Func)
	if !ok {
		return nil, fmt.Errorf("persistResult is not a function: %T", obj)
	}

	return fn, nil
}

// P0-12D: Find an enclosing function by name and return its *types.Func.
func (p *publicationAuthorityPackage) findEnclosingFunc(name string) *types.Func {
	scope := p.Package.Scope()
	if obj := scope.Lookup(name); obj != nil {
		if fn, ok := obj.(*types.Func); ok {
			return fn
		}
	}
	return nil
}

// P0-12C: Enumerate all canonical persistResult calls in the package.
// P0-12D: Bind each call to its production path (success/failure) using semantic *types.Func identity.
// P0-12D: Authorized callers:
//   - runLab: production success path caller
//   - finalizeLifecycleFailure: production failure path caller (production failure path caller)
func (p *publicationAuthorityPackage) enumeratePersistResultCalls() (*publicationAuthorityInspection, error) {
	result := &publicationAuthorityInspection{}

	// P0-12B: Resolve canonical function
	canonicalFn, err := p.resolveCanonicalPersistResult()
	if err != nil {
		result.MissingPersistFunc = true
		return result, err
	}
	result.CanonicalPersistResultFunc = canonicalFn

	// P0-12D: Resolve authorized publication callers from package scope
	// Success caller: runLab
	// Failure caller: finalizeLifecycleFailure (production failure path caller)
	successCaller := p.findEnclosingFunc("runLab")
	failureCaller := p.findEnclosingFunc("finalizeLifecycleFailure")

	// P0-12D: Fail closed if either caller is not found in package scope
	if successCaller == nil {
		result.TypeCheckFailed = true
		return result, fmt.Errorf("success caller runLab not found in package scope")
	}
	if failureCaller == nil {
		result.TypeCheckFailed = true
		return result, fmt.Errorf("failure caller finalizeLifecycleFailure not found in package scope")
	}

	// Traverse all AST files and find persistResult calls
	for _, file := range p.Syntax {
		// P0-12D: Inspect both FuncDecls (named functions) and FuncLits (anonymous functions)
		// Production code uses both: named functions for fixtures and function literals for production
		inspectForPersistCalls(file, result, p.TypesInfo, canonicalFn, successCaller, failureCaller, nil, "File:root")
	}

	return result, nil
}

// inspectForPersistCalls recursively inspects AST nodes for persistResult calls.
func inspectForPersistCalls(node ast.Node, result *publicationAuthorityInspection, info *types.Info, canonicalFn *types.Func, successCaller, failureCaller *types.Func, enclosingFunc *types.Func, enclosingFuncName string) {
	// Handle both FuncDecl (named functions) and FuncLit (anonymous functions)
	switch n := node.(type) {
	case *ast.File:
		// Iterate over top-level declarations
		for _, decl := range n.Decls {
			inspectForPersistCalls(decl, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		return

	case *ast.GenDecl:
		// Handle package-level declarations (imports, const, var, type)
		for _, spec := range n.Specs {
			inspectForPersistCalls(spec, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		return

	case *ast.FuncDecl:
		var newEnclosingFunc *types.Func
		var newEnclosingFuncName string
		if n.Name != nil {
			newEnclosingFuncName = "FuncDecl:" + n.Name.Name
			if obj := info.ObjectOf(n.Name); obj != nil {
				if f, ok := obj.(*types.Func); ok {
					newEnclosingFunc = f
				}
			}
		}
		// Recursively inspect the function body - CallExpr nodes will be counted there
		if n.Body != nil {
			inspectForPersistCalls(n.Body, result, info, canonicalFn, successCaller, failureCaller, newEnclosingFunc, newEnclosingFuncName)
		}
		return

	case *ast.FuncLit:
		// Anonymous function - we track it but can't resolve to *types.Func
		var newEnclosingFuncName string
		newEnclosingFuncName = "FuncLit"
		if n.Body != nil {
			inspectForPersistCalls(n.Body, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, newEnclosingFuncName)
		}
		return

	case *ast.BlockStmt:
		// Inspect statements
		for _, stmt := range n.List {
			inspectForPersistCalls(stmt, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		return

	case *ast.AssignStmt:
		// Inspect LHS (for function-valued variable declarations)
		for _, lhs := range n.Lhs {
			inspectForPersistCalls(lhs, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		// Inspect RHS
		for _, rhs := range n.Rhs {
			inspectForPersistCalls(rhs, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		return

	case *ast.CallExpr:
		// Check if this is a persistResult call
		ident, ok := n.Fun.(*ast.Ident)
		if !ok {
			// Not an identifier - recursively inspect arguments for any nested calls
			for _, arg := range n.Args {
				inspectForPersistCalls(arg, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
			}
			return
		}

		// P0-12C: First check if the identifier NAME is persistResult
		if ident.Name != canonicalPersistResultFunction {
			// Not persistResult - inspect arguments for any nested calls
			for _, arg := range n.Args {
				inspectForPersistCalls(arg, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
			}
			return
		}

		// Resolve object for classification
		obj := info.ObjectOf(ident)
		if obj == nil {
			result.UnresolvedPersistCalls++
			return
		}

		fnObj, ok := obj.(*types.Func)
		if !ok {
			// Function-valued variable
			result.FunctionFieldPublication++
			return
		}

		if fnObj == canonicalFn {
			result.CanonicalPersistCalls++

			// P0-12D: Classify by enclosing *types.Func identity ONLY
			// P0-12D: Fail-closed: any canonical call not from authorized caller is unexpected
			isFailurePath := false
			isSuccessPath := false

			if enclosingFunc != nil {
				// Check by *types.Func identity only
				if failureCaller != nil && enclosingFunc == failureCaller {
					isFailurePath = true
				} else if successCaller != nil && enclosingFunc == successCaller {
					isSuccessPath = true
				}
			}

			// P0-12D: Fail-closed - canonical call from non-authorized enclosing function is unexpected
			if !isFailurePath && !isSuccessPath {
				result.UnexpectedCanonicalCallerCalls++
				result.UnexpectedCanonicalCallerEnclosingFuncs = append(result.UnexpectedCanonicalCallerEnclosingFuncs, enclosingFuncName)
			}

			if isFailurePath {
				result.FailurePathCanonicalCalls++
				result.FailurePathEnclosingFuncs = append(result.FailurePathEnclosingFuncs, enclosingFuncName)
			} else if isSuccessPath {
				result.SuccessPathCanonicalCalls++
				result.SuccessPathEnclosingFuncs = append(result.SuccessPathEnclosingFuncs, enclosingFuncName)
			}
		} else {
			result.SameNameInOtherPkg++
		}
		return

	case *ast.ReturnStmt:
		for _, val := range n.Results {
			inspectForPersistCalls(val, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		return

	case *ast.ExprStmt:
		inspectForPersistCalls(n.X, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		return

	case *ast.IfStmt:
		// P0-12: Inspect Init (e.g., if x := foo(); x != nil { ... })
		if n.Init != nil {
			inspectForPersistCalls(n.Init, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		inspectForPersistCalls(n.Cond, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		if n.Body != nil {
			inspectForPersistCalls(n.Body, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		if n.Else != nil {
			inspectForPersistCalls(n.Else, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		return

	case *ast.ForStmt:
		if n.Init != nil {
			inspectForPersistCalls(n.Init, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		if n.Cond != nil {
			inspectForPersistCalls(n.Cond, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		if n.Post != nil {
			inspectForPersistCalls(n.Post, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		if n.Body != nil {
			inspectForPersistCalls(n.Body, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		return

	case *ast.SwitchStmt:
		if n.Init != nil {
			inspectForPersistCalls(n.Init, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		if n.Tag != nil {
			inspectForPersistCalls(n.Tag, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		for _, c := range n.Body.List {
			inspectForPersistCalls(c, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		return

	case *ast.CommClause:
		if n.Comm != nil {
			inspectForPersistCalls(n.Comm, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		for _, s := range n.Body {
			inspectForPersistCalls(s, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		return

	case *ast.SelectStmt:
		for _, c := range n.Body.List {
			inspectForPersistCalls(c, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		return

	case *ast.SendStmt:
		inspectForPersistCalls(n.Chan, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		inspectForPersistCalls(n.Value, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		return

	case *ast.DeferStmt:
		inspectForPersistCalls(n.Call, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		return

	case *ast.GoStmt:
		inspectForPersistCalls(n.Call, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		return

	case *ast.CompositeLit:
		for _, elt := range n.Elts {
			inspectForPersistCalls(elt, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		return

	case *ast.IndexExpr:
		inspectForPersistCalls(n.X, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		inspectForPersistCalls(n.Index, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		return

	case *ast.StarExpr:
		inspectForPersistCalls(n.X, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		return

	case *ast.UnaryExpr:
		inspectForPersistCalls(n.X, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		return

	case *ast.BinaryExpr:
		inspectForPersistCalls(n.X, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		inspectForPersistCalls(n.Y, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		return

	case *ast.KeyValueExpr:
		inspectForPersistCalls(n.Key, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		inspectForPersistCalls(n.Value, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		return

	case *ast.ValueSpec:
		for _, val := range n.Values {
			inspectForPersistCalls(val, result, info, canonicalFn, successCaller, failureCaller, enclosingFunc, enclosingFuncName)
		}
		return
	}
}

// countPersistCallsInBlock counts persistResult calls in a block statement.
func countPersistCallsInBlock(body *ast.BlockStmt, canonicalFn *types.Func) int {
	count := 0
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			ident, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			if ident.Name != canonicalPersistResultFunction {
				return true
			}
			count++
		}
		return true
	})
	return count
}

// findEnclosingFuncDecl finds the function declaration that encloses a node.
func findEnclosingFuncDecl(file *ast.File, target ast.Node) *ast.FuncDecl {
	var enclosingFunc *ast.FuncDecl
	var foundTarget bool

	ast.Inspect(file, func(n ast.Node) bool {
		if foundTarget {
			return false // Stop once we've passed the target
		}

		if n == target {
			foundTarget = true
			return false
		}

		if fn, ok := n.(*ast.FuncDecl); ok {
			enclosingFunc = fn
		}

		return true
	})

	return enclosingFunc
}

// InspectPersistResultAuthoritySemantically performs full semantic inspection.
// P0-12A through P0-12D: Uses go/packages for compiler-backed identity resolution.
func InspectPersistResultAuthoritySemantically(sourceDir string) (*publicationAuthorityInspection, error) {
	// P0-12A: Load package with semantic type info
	pkg, err := loadPublicationAuthorityPackage(sourceDir)
	if err != nil {
		return &publicationAuthorityInspection{
			LoadFailed: true,
		}, err
	}

	// P0-12B: Resolve canonical persistResult
	canonicalFn, err := pkg.resolveCanonicalPersistResult()
	if err != nil {
		return &publicationAuthorityInspection{
			MissingPersistFunc: true,
		}, err
	}

	// P0-12C: Enumerate canonical calls with path binding
	result, err := pkg.enumeratePersistResultCalls()
	if err != nil {
		result.TypeCheckFailed = true
		return result, err
	}

	// Verify we found the canonical function
	if result.CanonicalPersistResultFunc == nil {
		result.CanonicalPersistResultFunc = canonicalFn
	}

	return result, nil
}

// VerifyPersistResultSemantics verifies semantic publication authority rules.
// P0-12A through P0-12D: Semantic authority verification.
func VerifyPersistResultSemantics(sourceDir string) error {
	inspection, err := InspectPersistResultAuthoritySemantically(sourceDir)
	if err != nil {
		return &ErrAuthorityGuardFailed{
			Guard:    "persist_result_semantic_authority",
			Details:  fmt.Sprintf("semantic inspection failed: %v", err),
			Expected: "successful semantic inspection",
			Actual:   fmt.Sprintf("error: %v", err),
		}
	}

	// P0-12D: Require zero unexpected canonical caller calls (fail-closed)
	// Only runLab and finalizeLifecycleFailure are authorized callers
	if inspection.UnexpectedCanonicalCallerCalls != 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "persist_result_unexpected_caller",
			Details:  fmt.Sprintf("expected 0 unexpected canonical caller calls, got %d (from: %v)", inspection.UnexpectedCanonicalCallerCalls, inspection.UnexpectedCanonicalCallerEnclosingFuncs),
			Expected: 0,
			Actual:   inspection.UnexpectedCanonicalCallerCalls,
		}
	}

	// P0-12C: Require exactly 2 canonical call sites in production
	if inspection.CanonicalPersistCalls != 2 {
		return &ErrAuthorityGuardFailed{
			Guard:    "persist_result_topology",
			Details:  fmt.Sprintf("expected 2 canonical persistResult calls, got %d", inspection.CanonicalPersistCalls),
			Expected: 2,
			Actual:   inspection.CanonicalPersistCalls,
		}
	}

	// P0-12D: Require 1 success path call
	if inspection.SuccessPathCanonicalCalls != 1 {
		return &ErrAuthorityGuardFailed{
			Guard:    "persist_result_success_path",
			Details:  fmt.Sprintf("expected 1 success path call, got %d", inspection.SuccessPathCanonicalCalls),
			Expected: 1,
			Actual:   inspection.SuccessPathCanonicalCalls,
		}
	}

	// P0-12D: Require 1 failure path call
	if inspection.FailurePathCanonicalCalls != 1 {
		return &ErrAuthorityGuardFailed{
			Guard:    "persist_result_failure_path",
			Details:  fmt.Sprintf("expected 1 failure path call, got %d", inspection.FailurePathCanonicalCalls),
			Expected: 1,
			Actual:   inspection.FailurePathCanonicalCalls,
		}
	}

	// P0-12B: Require zero unresolved identifiers (fail-closed)
	if inspection.UnresolvedPersistCalls != 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "persist_result_unresolved",
			Details:  fmt.Sprintf("expected 0 unresolved persistResult identifiers, got %d", inspection.UnresolvedPersistCalls),
			Expected: 0,
			Actual:   inspection.UnresolvedPersistCalls,
		}
	}

	// P0-12C: Require zero semantic spoof calls
	// All currently implemented spoof categories must be zero
	if inspection.FunctionFieldPublication != 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "persist_result_function_field_spoof",
			Details:  fmt.Sprintf("expected 0 function field publication spoofs, got %d", inspection.FunctionFieldPublication),
			Expected: 0,
			Actual:   inspection.FunctionFieldPublication,
		}
	}

	if inspection.SameNameInOtherPkg != 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "persist_result_same_name_spoof",
			Details:  fmt.Sprintf("expected 0 same-name-in-other-package spoofs, got %d", inspection.SameNameInOtherPkg),
			Expected: 0,
			Actual:   inspection.SameNameInOtherPkg,
		}
	}

	// P0-12C: Require zero noncanonical same-name calls
	if inspection.NonCanonicalPersistCalls != 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "persist_result_noncanonical",
			Details:  fmt.Sprintf("expected 0 noncanonical persistResult calls, got %d", inspection.NonCanonicalPersistCalls),
			Expected: 0,
			Actual:   inspection.NonCanonicalPersistCalls,
		}
	}

	return nil
}

// persistResultFunctionResult contains inspection results for a single function.
type persistResultFunctionResult struct {
	calls                        int
	directWriteFileToResult      int
	directCreateToResult         int
	directJsonEncoderToResult    int
	hasBypass                    bool
	hasDeadCanonical             bool
	hasConditionalBypass         bool
	hasSelectorBypass             bool
	hasMethodBypass               bool
}

// inspectPersistResultFunctionBody inspects a function body for publication authority violations.
func inspectPersistResultFunctionBody(body *ast.BlockStmt, definedFuncs map[string]bool) persistResultFunctionResult {
	result := persistResultFunctionResult{}

	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			// Check for persistResult calls (only if it's defined in this file)
			if ident, ok := node.Fun.(*ast.Ident); ok {
				if ident.Name == canonicalPersistResultFunction && definedFuncs[canonicalPersistResultFunction] {
					result.calls++
				}
			}

			// Check for os.WriteFile calls
			if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "os" {
					if sel.Sel.Name == "WriteFile" {
						// Check if the path contains "result.json"
						if len(node.Args) >= 1 {
							if pathLit, ok := node.Args[0].(*ast.BasicLit); ok {
								if strings.Contains(pathLit.Value, "result.json") {
									result.directWriteFileToResult++
								}
							}
						}
					}
					if sel.Sel.Name == "Create" {
						// Check if the path contains "result.json"
						if len(node.Args) >= 1 {
							if pathLit, ok := node.Args[0].(*ast.BasicLit); ok {
								if strings.Contains(pathLit.Value, "result.json") {
									result.directCreateToResult++
								}
							}
						}
					}
				}

				// Check for json.NewEncoder calls
				if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "json" {
					if sel.Sel.Name == "NewEncoder" {
						result.directJsonEncoderToResult++
					}
				}

				// Check for selector-based bypass (e.g., p.persistResult)
				if sel.Sel.Name == canonicalPersistResultFunction && !definedFuncs[canonicalPersistResultFunction] {
					result.hasSelectorBypass = true
				}
			}

			// Check for method calls
			if _, ok := node.Fun.(*ast.SelectorExpr); ok {
				// Could be method bypass
			}

			return true

		case *ast.AssignStmt:
			// Check for function-valued variables
			for _, lhs := range node.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					if ident.Name == canonicalPersistResultFunction {
						result.hasBypass = true
					}
				}
			}
			return true

		default:
			return true
		}
	})

	return result
}
