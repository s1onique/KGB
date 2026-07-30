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

		// Track enclosing function for call classification
		var enclosingFunc string

		// Inspect AST
		ast.Inspect(node, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.FuncDecl:
				// Track enclosing function
				enclosingFunc = node.Name.Name

			case *ast.FuncLit:
				// Track enclosing function for closures
				if node.Type != nil {
					enclosingFunc = "closure"
				}

			case *ast.CallExpr:
				// Handle both identifier calls and selector calls
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

				// Check for PollTargetAuthority calls
				if funcName == "PollTargetAuthority" {
					if isQualified {
						// Qualified call like pkg.PollTargetAuthority - external
						return true
					}
					// Direct call - classify by enclosing function
					switch enclosingFunc {
					case "runLab", "Run", "main":
						result.DirectRunnerPollCalls++
					case "RunCollectionLifecycle", "RunLifecycle", "lifecycle", "Lifecycle":
						result.LifecyclePollCalls++
					default:
						// Call in other context
						result.LifecyclePollCalls++
					}
				}

				// Check for http.Client.Get calls
				if funcName == "Get" && isQualified {
					if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
						if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "client" {
							result.ClientGetCallsInCapture++
						}
					}
				}

				// Check for direct Get calls (http.DefaultClient or similar)
				if funcName == "Get" && !isQualified {
					result.ClientGetCallsInCapture++
				}

			case *ast.SelectStmt:
				// Check for default case in select (non-blocking send)
				// Default case is *ast.CommClause with nil List
				for _, stmt := range node.Body.List {
					if c, ok := stmt.(*ast.CaseClause); ok && c.List == nil {
						result.DefaultPollSendCases++
					}
				}

			case *ast.Ident:
				// Check for PollFn field references
				if strings.Contains(node.Name, "PollFn") || strings.Contains(node.Name, "PollCallback") {
					result.GenericPollFnFields++
				}
			}

			return true
		})

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

	// Rule: No generic PollFn fields
	if inspection.GenericPollFnFields > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "generic_poll_callback_guard",
			Details:  "PollFn or PollCallback fields found",
			Expected: 0,
			Actual:   inspection.GenericPollFnFields,
		}
	}

	// Rule: No default select cases (non-blocking send)
	if inspection.DefaultPollSendCases > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "default_send_guard",
			Details:  "Default case found in select",
			Expected: 0,
			Actual:   inspection.DefaultPollSendCases,
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

	// Rule: No direct http.Client.Get calls
	if inspection.ClientGetCallsInCapture > 0 {
		return &ErrAuthorityGuardFailed{
			Guard:    "client_get_guard",
			Details:  "Direct http.Client.Get calls found",
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
