// Package latencyringownership provides a static analyzer that enforces
// latency ring ownership boundaries in the UVB-76 codebase.
//
// This analyzer protects the FP01 domain boundary by rejecting raw
// LatencyTracker.GetRecentSamples use and public []state.LatencySample
// exposure outside the state owner package, while allowing immutable
// SampleWindow APIs, tests, generated code, and unrelated types.
package latencyringownership

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
	"go/types"
)

// OwnerPackagePath is the canonical path of the latency ring owner package.
const OwnerPackagePath = "github.com/s1onique/KGB/uvb76/state"

// Analyzer enforces latency ring ownership boundaries.
var Analyzer = &analysis.Analyzer{
	Name:     "latencyringownership",
	Doc:      "reports raw latency ring/sample access outside approved UVB-76 state boundaries",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// run is the main analysis function.
func run(pass *analysis.Pass) (interface{}, error) {
	// Skip if this is the allowed owner package
	if isAllowedPackage(pass) {
		return nil, nil
	}

	inspector := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Pre-compute set of generated files to skip
	generatedFiles := generatedFilePositions(pass)

	// Filter for relevant node types
	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
		(*ast.FuncDecl)(nil),
		(*ast.FuncType)(nil),
	}

	inspector.Preorder(nodeFilter, func(node ast.Node) {
		pos := node.Pos()

		// Skip test files - the ACT specifies tests should not be flagged
		if isTestFile(pass, pos) {
			return
		}

		// Skip generated files
		if isGeneratedFilePos(pass, pos, generatedFiles) {
			return
		}

		switch n := node.(type) {
		case *ast.CallExpr:
			checkCallExpr(pass, n)
		case *ast.FuncDecl:
			checkFuncDecl(pass, n)
		case *ast.FuncType:
			// Only check standalone FuncType nodes (interface method signatures)
			// FuncDecl embeds FuncType internally, so we skip FuncType inside FuncDecl
			if !isFuncTypeInFuncDecl(pass, n) {
				checkFuncType(pass, n)
			}
		}
	})

	return nil, nil
}

// generatedFileRange maps each generated file to its [Pos, End) range.
type generatedFileRange struct {
	start token.Pos
	end   token.Pos
}

// generatedFilePositions returns ranges of generated files.
func generatedFilePositions(pass *analysis.Pass) []generatedFileRange {
	var ranges []generatedFileRange
	for _, file := range pass.Files {
		if isGeneratedFile(file) {
			ranges = append(ranges, generatedFileRange{
				start: file.Pos(),
				end:   file.End(),
			})
		}
	}
	return ranges
}

// isGeneratedFilePos returns true if the given position is within a generated file.
func isGeneratedFilePos(pass *analysis.Pass, pos token.Pos, generatedRanges []generatedFileRange) bool {
	for _, r := range generatedRanges {
		if r.start <= pos && pos < r.end {
			return true
		}
	}
	return false
}

// checkCallExpr checks for disallowed GetRecentSamples calls.
func checkCallExpr(pass *analysis.Pass, call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}

	// Check if this is a method call
	fn, ok := pass.TypesInfo.Uses[sel.Sel]
	if !ok {
		return
	}

	method, ok := fn.(*types.Func)
	if !ok {
		return
	}

	sig := method.Type().(*types.Signature)
	recv := sig.Recv()
	if recv == nil {
		return
	}

	// Check if this is GetRecentSamples on LatencyTracker
	if method.Name() == "GetRecentSamples" && isStateLatencyTracker(recv.Type()) {
		pass.Reportf(call.Pos(), "use LatencyTracker.GetSampleWindow or Manager.Get{HTTP,ICMP}SampleWindow for analysis; raw GetRecentSamples is owned by uvb76/state")
	}
}

// checkFuncType checks interface method signatures for disallowed return types.
func checkFuncType(pass *analysis.Pass, ft *ast.FuncType) {
	if ft.Results == nil {
		return
	}

	// Check each return type
	for _, result := range ft.Results.List {
		tv := pass.TypesInfo.TypeOf(result.Type)
		if tv != nil && isSliceOfStateLatencySample(tv) {
			pass.Reportf(result.Type.Pos(), "interface method return type []state.LatencySample not allowed outside uvb76/state; use domain.SampleWindow instead")
			return
		}
	}
}

// checkFuncDecl checks for disallowed return types.
func checkFuncDecl(pass *analysis.Pass, fn *ast.FuncDecl) {
	if fn.Type.Results == nil {
		return
	}

	// Check each return type
	for _, result := range fn.Type.Results.List {
		tv := pass.TypesInfo.TypeOf(result.Type)
		if tv != nil && isSliceOfStateLatencySample(tv) {
			pass.Reportf(fn.Pos(), "do not expose []state.LatencySample outside uvb76/state; expose domain.SampleWindow or API DTOs instead")
			return
		}
	}
}

// isAllowedPackage returns true if the package being analyzed is the owner package.
func isAllowedPackage(pass *analysis.Pass) bool {
	return pass.Pkg != nil && pass.Pkg.Path() == OwnerPackagePath
}

// isTestFile returns true if the file is a test file.
func isTestFile(pass *analysis.Pass, pos token.Pos) bool {
	filename := pass.Fset.Position(pos).Filename
	return strings.HasSuffix(filename, "_test.go")
}

// isGeneratedFile returns true if the file has a standard generated-code header.
func isGeneratedFile(file *ast.File) bool {
	for _, group := range file.Comments {
		for _, comment := range group.List {
			text := comment.Text
			if strings.Contains(text, "Code generated") && strings.Contains(text, "DO NOT EDIT") {
				return true
			}
		}
	}
	return false
}

// isFuncTypeInFuncDecl returns true if the FuncType is part of a FuncDecl.
// This helps avoid duplicate reporting when FuncType is embedded in FuncDecl.
func isFuncTypeInFuncDecl(pass *analysis.Pass, ft *ast.FuncType) bool {
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Type == ft {
				return true
			}
		}
	}
	return false
}

// isStateLatencyTracker returns true if the type is state.LatencyTracker.
func isStateLatencyTracker(t types.Type) bool {
	t = deref(t)
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	if obj == nil || obj.Name() != "LatencyTracker" || obj.Pkg() == nil {
		return false
	}
	return obj.Pkg().Path() == OwnerPackagePath
}

// isSliceOfStateLatencySample returns true if the type is []state.LatencySample.
func isSliceOfStateLatencySample(t types.Type) bool {
	slice, ok := t.(*types.Slice)
	if !ok {
		return false
	}
	named, ok := slice.Elem().(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	if obj == nil || obj.Name() != "LatencySample" || obj.Pkg() == nil {
		return false
	}
	return obj.Pkg().Path() == OwnerPackagePath
}

// deref returns the dereferenced type (handles *T).
func deref(t types.Type) types.Type {
	if ptr, ok := t.(*types.Pointer); ok {
		return ptr.Elem()
	}
	return t
}
