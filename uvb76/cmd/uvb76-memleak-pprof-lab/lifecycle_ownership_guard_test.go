package main

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// inspectLifecycleOwnership parses the source and returns the lifecycle ownership result.
func inspectLifecycleOwnership(filename string, source []byte) (lifecycleOwnershipResult, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, source, parser.ParseComments)
	if err != nil {
		return lifecycleOwnershipResult{}, err
	}

	var (
		result                     lifecycleOwnershipResult
		deferredCancelCallsInDefer int
		cancelCallsInAll           int
	)

	// Check for slices import
	for _, imp := range node.Imports {
		if imp.Path != nil && filepath.Base(imp.Path.Value) == `"slices"` {
			result.ImportsSlices = true
			break
		}
	}

	// Walk the AST to find the runLab function and analyze it
	var runLabFunc *ast.FuncDecl
	for _, decl := range node.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "runLab" {
			runLabFunc = fn
			result.RunLabFound = true
			break
		}
	}

	if runLabFunc == nil {
		return result, nil
	}

	ast.Inspect(runLabFunc.Body, func(n ast.Node) bool {
		// Track deferred cancel calls using DeferStmt
		if deferStmt, ok := n.(*ast.DeferStmt); ok {
			if call, ok := deferStmt.Call.Fun.(*ast.Ident); ok {
				if call.Name == "collectionCancel" {
					result.DeferredCancelCalls++
					deferredCancelCallsInDefer++
				}
			}
		}

		// Handle CallExpr
		if call, ok := n.(*ast.CallExpr); ok {
			// Check for CollectAndSnapshot calls
			if ident, ok := call.Fun.(*ast.Ident); ok {
				if ident.Name == "CollectAndSnapshot" {
					result.CollectAndSnapshotCalls++
					// Extract the WaitGroup identifier name from the second argument
					if len(call.Args) >= 2 {
						if unary, ok := call.Args[1].(*ast.UnaryExpr); ok {
							if ident, ok := unary.X.(*ast.Ident); ok {
								result.WaitGroupIdentifier = ident.Name
							}
						}
					}
				}
				// Count all collectionCancel calls
				if ident.Name == "collectionCancel" {
					cancelCallsInAll++
				}
			}

			// Check for method calls (SelectorExpr)
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				// Check for wg.Wait() calls
				if sel.Sel.Name == "Wait" {
					if recvIdent, ok := sel.X.(*ast.Ident); ok {
						if result.WaitGroupIdentifier != "" && recvIdent.Name == result.WaitGroupIdentifier {
							result.DirectWaitCalls++
						}
					}
				}
				// Check for slices.Clone calls
				if sel.Sel.Name == "Clone" {
					if recvIdent, ok := sel.X.(*ast.Ident); ok && recvIdent.Name == "slices" {
						result.DirectCloneCalls++
					}
				}
			}
		}

		return true
	})

	// Ordinary calls = all calls - deferred calls
	result.OrdinaryCancelCalls = cancelCallsInAll - deferredCancelCallsInDefer

	return result, nil
}

// TestLifecycleOwnershipGuard_ProductionUsesCollectAndSnapshot verifies that runLab uses
// the CollectAndSnapshot helper exactly once and does not contain inline lifecycle logic.
func TestLifecycleOwnershipGuard_ProductionUsesCollectAndSnapshot(t *testing.T) {
	runnerPath := getRunnerPath(t)
	src := readRunnerSource(t, runnerPath)

	result, err := inspectLifecycleOwnership(runnerPath, src)
	if err != nil {
		t.Fatalf("inspectLifecycleOwnership failed: %v", err)
	}

	// Assert ownership rules
	if result.CollectAndSnapshotCalls != 1 {
		t.Errorf("runLab must call CollectAndSnapshot exactly once: got %d", result.CollectAndSnapshotCalls)
	}

	if result.DeferredCancelCalls != 1 {
		t.Errorf("runLab must have exactly one deferred collectionCancel: got %d", result.DeferredCancelCalls)
	}

	if result.OrdinaryCancelCalls != 0 {
		t.Errorf("runLab must not have ordinary collectionCancel calls: got %d", result.OrdinaryCancelCalls)
	}

	if result.DirectWaitCalls != 0 {
		t.Errorf("runLab must not call wg.Wait directly: got %d", result.DirectWaitCalls)
	}

	if result.DirectCloneCalls != 0 {
		t.Errorf("runLab must not call slices.Clone directly: got %d", result.DirectCloneCalls)
	}

	if result.ImportsSlices {
		t.Error("runner.go must not import slices package")
	}

	t.Logf("Lifecycle ownership verified: CollectAndSnapshot=%d, deferredCancel=%d, ordinaryCancel=%d, directWait=%d, directClone=%d, slicesImport=%v",
		result.CollectAndSnapshotCalls, result.DeferredCancelCalls, result.OrdinaryCancelCalls, result.DirectWaitCalls, result.DirectCloneCalls, result.ImportsSlices)
}

// TestLifecycleOwnershipGuard_RejectsMissingHelperCall verifies the guard detects
// when CollectAndSnapshot is not called.
func TestLifecycleOwnershipGuard_RejectsMissingHelperCall(t *testing.T) {
	badSource := `package main

func runLab() {
    // No CollectAndSnapshot call - just defer without the helper
}
`
	result, err := inspectLifecycleOwnership("test.go", []byte(badSource))
	if err != nil {
		t.Fatalf("inspectLifecycleOwnership failed: %v", err)
	}

	if result.CollectAndSnapshotCalls != 0 {
		t.Errorf("guard should detect missing CollectAndSnapshot: got %d", result.CollectAndSnapshotCalls)
	}
}

// TestLifecycleOwnershipGuard_RejectsDuplicateHelperCall verifies the guard rejects
// source with duplicate CollectAndSnapshot calls.
func TestLifecycleOwnershipGuard_RejectsDuplicateHelperCall(t *testing.T) {
	badSource := `package main

func runLab() {
    var mu sync.Mutex
    var wg sync.WaitGroup
    input := &CollectorInput{
        TovarischSamples: &[]ProcessSample{},
        UVB76Samples: &[]ProcessSample{},
        CollectorErrors: &[]string{},
        SamplesMu: &mu,
    }

    // First call
    CollectAndSnapshot(collectionCancel, &wg, input)
    // Duplicate call
    CollectAndSnapshot(collectionCancel, &wg, input)
}
`
	result, err := inspectLifecycleOwnership("test.go", []byte(badSource))
	if err != nil {
		t.Fatalf("inspectLifecycleOwnership failed: %v", err)
	}

	if result.CollectAndSnapshotCalls != 2 {
		t.Errorf("guard should reject duplicate CollectAndSnapshot: got %d", result.CollectAndSnapshotCalls)
	}
}

// TestLifecycleOwnershipGuard_RejectsOrdinaryCancel verifies the guard rejects
// source with ordinary (non-deferred) collectionCancel calls.
func TestLifecycleOwnershipGuard_RejectsOrdinaryCancel(t *testing.T) {
	badSource := `package main

func runLab() {
    defer collectionCancel()
    // Ordinary cancel call - forbidden!
    collectionCancel()
}
`
	result, err := inspectLifecycleOwnership("test.go", []byte(badSource))
	if err != nil {
		t.Fatalf("inspectLifecycleOwnership failed: %v", err)
	}

	if result.OrdinaryCancelCalls != 1 {
		t.Errorf("guard should reject ordinary collectionCancel calls: got %d", result.OrdinaryCancelCalls)
	}
}

// TestLifecycleOwnershipGuard_RejectsDirectWait verifies the guard rejects
// source with direct wg.Wait() calls.
func TestLifecycleOwnershipGuard_RejectsDirectWait(t *testing.T) {
	badSource := `package main

func runLab() {
    var mu sync.Mutex
    var wg sync.WaitGroup
    input := &CollectorInput{
        TovarischSamples: &[]ProcessSample{},
        UVB76Samples: &[]ProcessSample{},
        CollectorErrors: &[]string{},
        SamplesMu: &mu,
    }

    CollectAndSnapshot(collectionCancel, &wg, input)
    // Direct wg.Wait() - forbidden!
    wg.Wait()
}
`
	result, err := inspectLifecycleOwnership("test.go", []byte(badSource))
	if err != nil {
		t.Fatalf("inspectLifecycleOwnership failed: %v", err)
	}

	if result.DirectWaitCalls != 1 {
		t.Errorf("guard should reject direct wg.Wait() calls: got %d", result.DirectWaitCalls)
	}
}

// TestLifecycleOwnershipGuard_RejectsDirectClone verifies the guard rejects
// source with slices.Clone() calls.
func TestLifecycleOwnershipGuard_RejectsDirectClone(t *testing.T) {
	badSource := `package main

func runLab() {
    var mu sync.Mutex
    var wg sync.WaitGroup
    input := &CollectorInput{
        TovarischSamples: &[]ProcessSample{},
        UVB76Samples: &[]ProcessSample{},
        CollectorErrors: &[]string{},
        SamplesMu: &mu,
    }

    CollectAndSnapshot(collectionCancel, &wg, input)
    // Direct slices.Clone() - forbidden!
    _ = slices.Clone([]int{})
}
`
	result, err := inspectLifecycleOwnership("test.go", []byte(badSource))
	if err != nil {
		t.Fatalf("inspectLifecycleOwnership failed: %v", err)
	}

	if result.DirectCloneCalls != 1 {
		t.Errorf("guard should reject slices.Clone() calls: got %d", result.DirectCloneCalls)
	}
}

// TestLifecycleOwnershipGuard_RejectsSlicesImport verifies the guard rejects
// source that imports slices.
func TestLifecycleOwnershipGuard_RejectsSlicesImport(t *testing.T) {
	badSource := `package main

import "slices"

func runLab() {
    var mu sync.Mutex
    var wg sync.WaitGroup
    input := &CollectorInput{
        TovarischSamples: &[]ProcessSample{},
        UVB76Samples: &[]ProcessSample{},
        CollectorErrors: &[]string{},
        SamplesMu: &mu,
    }

    CollectAndSnapshot(collectionCancel, &wg, input)
}
`
	result, err := inspectLifecycleOwnership("test.go", []byte(badSource))
	if err != nil {
		t.Fatalf("inspectLifecycleOwnership failed: %v", err)
	}

	if !result.ImportsSlices {
		t.Error("guard should reject slices import")
	}
}

// TestLifecycleOwnershipGuard_TracksRenamedWaitGroup verifies the guard
// correctly tracks WaitGroup even when renamed.
func TestLifecycleOwnershipGuard_TracksRenamedWaitGroup(t *testing.T) {
	goodSource := `package main

func runLab() {
    var mu sync.Mutex
    var myWaitGroup sync.WaitGroup // Renamed WaitGroup
    input := &CollectorInput{
        TovarischSamples: &[]ProcessSample{},
        UVB76Samples: &[]ProcessSample{},
        CollectorErrors: &[]string{},
        SamplesMu: &mu,
    }

    CollectAndSnapshot(collectionCancel, &myWaitGroup, input)
}
`
	result, err := inspectLifecycleOwnership("test.go", []byte(goodSource))
	if err != nil {
		t.Fatalf("inspectLifecycleOwnership failed: %v", err)
	}

	if result.WaitGroupIdentifier != "myWaitGroup" {
		t.Errorf("guard should track WaitGroup identifier: got %s", result.WaitGroupIdentifier)
	}

	// Add a direct wait on myWaitGroup - should be detected
	badSource := `package main

func runLab() {
    var mu sync.Mutex
    var myWaitGroup sync.WaitGroup // Renamed WaitGroup
    input := &CollectorInput{
        TovarischSamples: &[]ProcessSample{},
        UVB76Samples: &[]ProcessSample{},
        CollectorErrors: &[]string{},
        SamplesMu: &mu,
    }

    CollectAndSnapshot(collectionCancel, &myWaitGroup, input)
    myWaitGroup.Wait() // Direct wait on renamed WaitGroup - forbidden!
}
`
	badResult, err := inspectLifecycleOwnership("test.go", []byte(badSource))
	if err != nil {
		t.Fatalf("inspectLifecycleOwnership failed: %v", err)
	}

	if badResult.DirectWaitCalls != 1 {
		t.Errorf("guard should reject direct myWaitGroup.Wait() calls: got %d", badResult.DirectWaitCalls)
	}
}

// TestLifecycleOwnershipEnforcer_MissingRunLab verifies that missing runLab function
// returns a distinct error that can be detected.
func TestLifecycleOwnershipEnforcer_MissingRunLab(t *testing.T) {
	badSource := `package main

func otherFunc() {
    // No runLab function at all
}
`
	result, err := inspectLifecycleOwnership("test.go", []byte(badSource))
	if err != nil {
		t.Fatalf("inspectLifecycleOwnership failed: %v", err)
	}

	// Result should have RunLabFound=false when runLab is absent
	if result.RunLabFound {
		t.Error("expected RunLabFound=false for missing runLab")
	}

	// Result should have zero values when runLab is absent
	if result.CollectAndSnapshotCalls != 0 {
		t.Errorf("expected 0 CollectAndSnapshotCalls for missing runLab: got %d", result.CollectAndSnapshotCalls)
	}

	// Enforcing function should detect missing runLab
	err = validateLifecycleOwnership(result)
	if err == nil {
		t.Error("expected error for missing runLab")
	}
	if !errors.Is(err, ErrMissingRunLab) {
		t.Errorf("expected ErrMissingRunLab: got %v", err)
	}
	// Negative assertion: missing runLab should NOT produce ErrMissingCollectAndSnapshot
	if errors.Is(err, ErrMissingCollectAndSnapshot) {
		t.Error("missing runLab must not produce ErrMissingCollectAndSnapshot")
	}
}

// TestLifecycleOwnershipEnforcer_GoodSource returns nil.
func TestLifecycleOwnershipEnforcer_GoodSource(t *testing.T) {
	goodSource := `package main

func runLab() {
    var mu sync.Mutex
    var wg sync.WaitGroup
    input := &CollectorInput{
        TovarischSamples: &[]ProcessSample{},
        UVB76Samples: &[]ProcessSample{},
        CollectorErrors: &[]string{},
        SamplesMu: &mu,
    }

    CollectAndSnapshot(collectionCancel, &wg, input)
}
`
	result, err := inspectLifecycleOwnership("test.go", []byte(goodSource))
	if err != nil {
		t.Fatalf("inspectLifecycleOwnership failed: %v", err)
	}

	err = validateLifecycleOwnership(result)
	if err != nil {
		t.Errorf("good source should not violate: %v", err)
	}
}

// TestLifecycleOwnershipEnforcer_RejectsMissingHelper returns typed error.
func TestLifecycleOwnershipEnforcer_RejectsMissingHelper(t *testing.T) {
	badSource := `package main

func runLab() {
    // No CollectAndSnapshot call
}
`
	result, err := inspectLifecycleOwnership("test.go", []byte(badSource))
	if err != nil {
		t.Fatalf("inspectLifecycleOwnership failed: %v", err)
	}

	// runLab exists but CollectAndSnapshot is not called
	if !result.RunLabFound {
		t.Error("expected RunLabFound=true for missing helper")
	}

	err = validateLifecycleOwnership(result)
	if err == nil {
		t.Error("expected error for missing helper")
	}
	if !errors.Is(err, ErrMissingCollectAndSnapshot) {
		t.Errorf("expected ErrMissingCollectAndSnapshot: got %v", err)
	}
	if !errors.Is(err, ErrLifecycleOwnershipViolation) {
		t.Errorf("expected ErrLifecycleOwnershipViolation: got %v", err)
	}
	// Negative assertion: present runLab with missing helper should NOT produce ErrMissingRunLab
	if errors.Is(err, ErrMissingRunLab) {
		t.Error("present runLab with missing helper must not produce ErrMissingRunLab")
	}
}

// TestLifecycleOwnershipEnforcer_RejectsDuplicateHelper returns typed error.
func TestLifecycleOwnershipEnforcer_RejectsDuplicateHelper(t *testing.T) {
	badSource := `package main

func runLab() {
    var mu sync.Mutex
    var wg sync.WaitGroup
    input := &CollectorInput{
        TovarischSamples: &[]ProcessSample{},
        UVB76Samples: &[]ProcessSample{},
        CollectorErrors: &[]string{},
        SamplesMu: &mu,
    }

    CollectAndSnapshot(collectionCancel, &wg, input)
    CollectAndSnapshot(collectionCancel, &wg, input)
}
`
	result, err := inspectLifecycleOwnership("test.go", []byte(badSource))
	if err != nil {
		t.Fatalf("inspectLifecycleOwnership failed: %v", err)
	}

	err = validateLifecycleOwnership(result)
	if err == nil {
		t.Error("expected error for duplicate helper")
	}
	if !errors.Is(err, ErrDuplicateCollectAndSnapshot) {
		t.Errorf("expected ErrDuplicateCollectAndSnapshot: got %v", err)
	}
}

// TestLifecycleOwnershipEnforcer_RejectsOrdinaryCancel tests the enforcer directly without source parsing.
// This proves that validateLifecycleOwnership converts OrdinaryCancelCalls == 1 into ErrOrdinaryCancel.
func TestLifecycleOwnershipEnforcer_RejectsOrdinaryCancel(t *testing.T) {
	result := lifecycleOwnershipResult{
		RunLabFound:             true,
		CollectAndSnapshotCalls: 1,
		OrdinaryCancelCalls:     1,
	}

	err := validateLifecycleOwnership(result)
	if err == nil {
		t.Fatal("expected lifecycle ownership violation")
	}
	if !errors.Is(err, ErrLifecycleOwnershipViolation) {
		t.Fatalf("missing ErrLifecycleOwnershipViolation: %v", err)
	}
	if !errors.Is(err, ErrOrdinaryCancel) {
		t.Fatalf("missing ErrOrdinaryCancel: %v", err)
	}
}

// TestLifecycleOwnershipEnforcer_RejectsDirectWait tests the enforcer directly without source parsing.
func TestLifecycleOwnershipEnforcer_RejectsDirectWait(t *testing.T) {
	result := lifecycleOwnershipResult{
		RunLabFound:             true,
		CollectAndSnapshotCalls: 1,
		DirectWaitCalls:         1,
	}

	err := validateLifecycleOwnership(result)
	if err == nil {
		t.Fatal("expected lifecycle ownership violation")
	}
	if !errors.Is(err, ErrLifecycleOwnershipViolation) {
		t.Fatalf("missing ErrLifecycleOwnershipViolation: %v", err)
	}
	if !errors.Is(err, ErrDirectWait) {
		t.Fatalf("missing ErrDirectWait: %v", err)
	}
}

// TestLifecycleOwnershipEnforcer_RejectsDirectClone tests the enforcer directly without source parsing.
func TestLifecycleOwnershipEnforcer_RejectsDirectClone(t *testing.T) {
	badSource := `package main

func runLab() {
    var mu sync.Mutex
    var wg sync.WaitGroup
    input := &CollectorInput{
        TovarischSamples: &[]ProcessSample{},
        UVB76Samples: &[]ProcessSample{},
        CollectorErrors: &[]string{},
        SamplesMu: &mu,
    }

    CollectAndSnapshot(collectionCancel, &wg, input)
    _ = slices.Clone([]int{}) // Direct clone - forbidden!
}
`
	result, err := inspectLifecycleOwnership("test.go", []byte(badSource))
	if err != nil {
		t.Fatalf("inspectLifecycleOwnership failed: %v", err)
	}

	err = validateLifecycleOwnership(result)
	if err == nil {
		t.Error("expected error for direct clone")
	}
	if !errors.Is(err, ErrDirectClone) {
		t.Errorf("expected ErrDirectClone: got %v", err)
	}
}

// TestLifecycleOwnershipEnforcer_RejectsSlicesImport returns typed error.
func TestLifecycleOwnershipEnforcer_RejectsSlicesImport(t *testing.T) {
	badSource := `package main

import "slices"

func runLab() {
    var mu sync.Mutex
    var wg sync.WaitGroup
    input := &CollectorInput{
        TovarischSamples: &[]ProcessSample{},
        UVB76Samples: &[]ProcessSample{},
        CollectorErrors: &[]string{},
        SamplesMu: &mu,
    }

    CollectAndSnapshot(collectionCancel, &wg, input)
}
`
	result, err := inspectLifecycleOwnership("test.go", []byte(badSource))
	if err != nil {
		t.Fatalf("inspectLifecycleOwnership failed: %v", err)
	}

	err = validateLifecycleOwnership(result)
	if err == nil {
		t.Error("expected error for slices import")
	}
	if !errors.Is(err, ErrSlicesImport) {
		t.Errorf("expected ErrSlicesImport: got %v", err)
	}
}

// TestLifecycleOwnershipEnforcer_ProductionSource verifies the actual runner.go passes.
func TestLifecycleOwnershipEnforcer_ProductionSource(t *testing.T) {
	runnerPath := getRunnerPath(t)
	src := readRunnerSource(t, runnerPath)

	result, err := inspectLifecycleOwnership(runnerPath, src)
	if err != nil {
		t.Fatalf("inspectLifecycleOwnership failed: %v", err)
	}

	err = validateLifecycleOwnership(result)
	if err != nil {
		t.Errorf("runner.go must pass ownership validation: %v", err)
	}
}

// Helper functions

func getRunnerPath(t *testing.T) string {
	_, thisFile, _, _ := runtime.Caller(0)
	thisDir := filepath.Dir(thisFile)
	return filepath.Join(thisDir, "runner.go")
}

func readRunnerSource(t *testing.T, runnerPath string) []byte {
	src, err := os.ReadFile(runnerPath)
	if err != nil {
		t.Fatalf("failed to read runner.go: %v", err)
	}
	return src
}
