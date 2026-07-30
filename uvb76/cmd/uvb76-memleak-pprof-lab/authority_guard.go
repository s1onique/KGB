// Package main provides the UVB-76 pprof memory leak lab.
//
// # AST-Based Authority Guards
//
// P0-14: Syntax-aware production authority guards using go/ast.
// Guards parse production source and fail if forbidden structures appear.
package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// ErrPollAuthorityViolation is returned when a production authority guard fails.
var ErrPollAuthorityViolation = errors.New("poll authority violation")

// ErrProfileAuthorityViolation is returned when a profile authority guard fails.
var ErrProfileAuthorityViolation = errors.New("profile authority violation")

// pollProfileAuthorityInspection contains AST inspection results.
type pollProfileAuthorityInspection struct {
	// Poll guards
	DirectRunnerPollCalls      int
	LifecyclePollCalls         int
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

// ErrAuthorityGuardFailed is returned when a guard fails.
type ErrAuthorityGuardFailed struct {
	Guard    string
	Details  string
	Expected int
	Actual   int
}

func (e *ErrAuthorityGuardFailed) Error() string {
	return fmt.Sprintf("authority guard %s failed: %s (expected %d, got %d)",
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

	return nil
}
