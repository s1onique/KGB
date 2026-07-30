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
	GenericPollFnFields        int
	DefaultPollSendCases       int
	StringErrorClassifications int

	// Profile guards
	ClientGetCallsInCapture  int
	DirectDestinationCreates int
	RenamesBeforeValidation  int
	TempCleanupCalls         int
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

// inspectFunctionBody inspects a single function body with precise function tracking.
func inspectFunctionBody(body *ast.BlockStmt, funcName string, result *pollProfileAuthorityInspection) {
	// Functions where client.Get calls are forbidden
	isCaptureFunction := funcName == "CaptureProfile" || funcName == "captureProfileWithOps"
	// Functions where string-based error classification is forbidden
	isLifecycleFunction := funcName == "RunCollectionLifecycle" || funcName == "RunLifecycle" || funcName == "lifecycle" || funcName == "Lifecycle"

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
			if isCaptureFunction || isLifecycleFunction {
				for _, expr := range node.Rhs {
					if call, ok := expr.(*ast.CallExpr); ok {
						if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
							if sel.Sel.Name == "Contains" || sel.Sel.Name == "HasPrefix" || sel.Sel.Name == "HasSuffix" || sel.Sel.Name == "Equal" {
								if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "strings" {
									result.StringErrorClassifications++
								}
							}
						}
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
				// Direct call - classify by enclosing function
				switch funcName {
				case "runLab", "Run", "main":
					result.DirectRunnerPollCalls++
				case "RunCollectionLifecycle", "RunLifecycle", "lifecycle", "Lifecycle":
					result.LifecyclePollCalls++
				default:
					result.LifecyclePollCalls++
				}
			}
		}
		return true
	})
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
