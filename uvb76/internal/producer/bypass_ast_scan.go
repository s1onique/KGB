package producer

import (
	"fmt"
	"go/ast"
	"go/token"
)

// scanFile walks the AST and emits findings.
//
// Symbol scoping:
//   - The detector tracks the enclosing *ast.FuncDecl for each call site.
//   - For each call, only the bindings whose WriterSymbol matches the
//     enclosing symbol are attached to the finding.
//   - When the contract declares the entire file as dedicated, the file's
//     full binding set is attached regardless of enclosing symbol.
//   - Constructor-only calls (bufio.NewWriter, json.NewEncoder) by themselves
//     are NOT findings; a follow-up write call inside the same FuncDecl
//     becomes the finding.
func (d *BypassDetector) scanFile(fset *token.FileSet, f *ast.File, relFile string, bindings []ProducerBinding) []BypassFinding {
	var findings []BypassFinding

	// Construct the "fully dedicated" predicate: when every binding for this
	// file shares the same enclosing file scope, the file is dedicated.
	// For shared files we cannot attach every binding to every finding.
	fileFullyDedicated := isFileFullyDedicated(bindings)

	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		funName := callName(call)
		if !d.isBypassingCall(funName) {
			return true
		}
		if funName == "os.OpenFile" {
			if !hasWriteFlagArg(call) {
				return true
			}
		}
		// Constructor-only calls are suppressed unless a follow-up write
		// occurs in the same FuncDecl.
		if isConstructionOnlyCall(funName) {
			return true
		}
		enclosing := enclosingFuncDecl(f, call)
		applicable := applicableBindingsForSymbol(bindings, enclosing, fileFullyDedicated)
		if len(applicable) == 0 {
			return true
		}
		dest, reason := d.destinationFromCall(call, funName)
		pos := fset.Position(call.Pos())
		sids := make([]string, 0, len(applicable))
		writerSym := ""
		var policy PersistencePolicy
		for _, b := range applicable {
			sids = append(sids, b.SurfaceID)
			if writerSym == "" {
				writerSym = b.WriterSymbol
			}
			if policy == "" {
				policy = b.PersistencePolicy
			}
		}
		suggestion := fmt.Sprintf(
			"replace %s at line %d with %s.WriteRedacted* for surface(s) %v (persistence=%s)",
			funName, pos.Line, d.cfg.RequiredAPI, sids, policy,
		)
		findings = append(findings, BypassFinding{
			File:              relFile,
			Line:              pos.Line,
			Column:            pos.Column,
			CallName:          funName,
			SurfaceIDs:        sids,
			WriterSymbol:      writerSym,
			RequiredAPI:       d.cfg.RequiredAPI,
			Suggestion:        suggestion,
			Destination:       dest,
			DestinationReason: reason,
		})
		return true
	})
	return findings
}

// isFileFullyDedicated returns true when every binding for the file shares
// a single writer symbol and the file does not appear as a shared helper.
// This is a conservative default: if every binding's writer symbols are
// present in the same file (only one writer per file), the file is
// considered dedicated to that one writer.
func isFileFullyDedicated(bindings []ProducerBinding) bool {
	if len(bindings) == 0 {
		return false
	}
	first := bindings[0].WriterSymbol
	for _, b := range bindings[1:] {
		if b.WriterSymbol != first {
			return false
		}
	}
	return true
}

// applicableBindingsForSymbol filters the bindings for the file to those
// whose WriterSymbol matches the enclosing FuncDecl name. When the file is
// fully dedicated, every binding is returned as-is.
func applicableBindingsForSymbol(bindings []ProducerBinding, fn *ast.FuncDecl, fileFullyDedicated bool) []ProducerBinding {
	if fileFullyDedicated {
		return bindings
	}
	if fn == nil {
		// No enclosing function: the call lives at file scope. We do NOT
		// attach any binding because we cannot prove which writer it belongs
		// to.
		return nil
	}
	name := funcDeclName(fn)
	if name == "" {
		return nil
	}
	var out []ProducerBinding
	for _, b := range bindings {
		if b.WriterSymbol == name {
			out = append(out, b)
		}
	}
	return out
}

// astChildren returns the immediate AST children of n. Used by
// enclosingFuncDecl to do an explicit iterative walk that does not
// recurse through ast.Inspect (which would visit descendants, not just
// immediate children, and cause exponential blow-up when we push every
// visited descendant onto our walker stack).
func astChildren(n ast.Node) []ast.Node {
	if n == nil {
		return nil
	}
	var out []ast.Node
	switch v := n.(type) {
	case *ast.File:
		// skip Docs (Go 1.25 removed File.Docs field)
		for _, decl := range v.Decls {
			out = append(out, decl)
		}
	case *ast.FuncDecl:
		out = append(out, v.Recv, v.Type, v.Body)
	case *ast.BlockStmt:
		for _, stmt := range v.List {
			out = append(out, stmt)
		}
	case *ast.IfStmt:
		out = append(out, v.Init, v.Cond, v.Body, v.Else)
	case *ast.ForStmt:
		out = append(out, v.Init, v.Cond, v.Post, v.Body)
	case *ast.RangeStmt:
		out = append(out, v.Key, v.Value, v.X, v.Body)
	case *ast.ExprStmt:
		out = append(out, v.X)
	case *ast.AssignStmt:
		for _, e := range v.Lhs {
			out = append(out, e)
		}
		for _, e := range v.Rhs {
			out = append(out, e)
		}
	case *ast.DeclStmt:
		out = append(out, v.Decl)
	case *ast.GenDecl:
		for _, sp := range v.Specs {
			out = append(out, sp)
		}
	case *ast.ValueSpec:
		for _, e := range v.Values {
			out = append(out, e)
		}
		out = append(out, v.Type)
	case *ast.CallExpr:
		out = append(out, v.Fun)
		for _, a := range v.Args {
			out = append(out, a)
		}
	case *ast.SelectorExpr:
		out = append(out, v.X)
	case *ast.BinaryExpr:
		out = append(out, v.X, v.Y)
	case *ast.UnaryExpr:
		out = append(out, v.X)
	case *ast.StarExpr:
		out = append(out, v.X)
	case *ast.CompositeLit:
		out = append(out, v.Type)
		for _, e := range v.Elts {
			out = append(out, e)
		}
	case *ast.KeyValueExpr:
		out = append(out, v.Key, v.Value)
	case *ast.FuncLit:
		out = append(out, v.Type, v.Body)
	}
	return out
}

func funcDeclName(fn *ast.FuncDecl) string {
	if fn == nil {
		return ""
	}
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		switch r := fn.Recv.List[0].Type.(type) {
		case *ast.StarExpr:
			if id, ok := r.X.(*ast.Ident); ok {
				return id.Name + "." + fn.Name.Name
			}
		case *ast.Ident:
			return r.Name + "." + fn.Name.Name
		}
	}
	return fn.Name.Name
}
