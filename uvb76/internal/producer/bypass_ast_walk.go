package producer

import "go/ast"

// enclosingFuncDecl walks the AST iteratively and returns the innermost
// FuncDecl that contains the given target node.
func enclosingFuncDecl(f *ast.File, target ast.Node) *ast.FuncDecl {
	if target == nil {
		return nil
	}
	parents := map[ast.Node]ast.Node{}
	seen := map[ast.Node]bool{}
	stack := []ast.Node{f}
	for len(stack) > 0 {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if top == nil || seen[top] {
			continue
		}
		seen[top] = true
		for _, child := range nodeChildrenForWalk(top) {
			if child == nil || seen[child] {
				continue
			}
			parents[child] = top
			stack = append(stack, child)
		}
	}
	p := parents[target]
	for p != nil {
		if fd, ok := p.(*ast.FuncDecl); ok {
			return fd
		}
		p = parents[p]
	}
	return nil
}

// nodeChildrenForWalk returns the immediate children of n for the AST node
// types we encounter during the bypass scan.
func nodeChildrenForWalk(n ast.Node) []ast.Node {
	switch v := n.(type) {
	case *ast.File:
		out := make([]ast.Node, 0, len(v.Decls))
		for _, d := range v.Decls {
			if d != nil {
				out = append(out, d)
			}
		}
		return out
	case *ast.FuncDecl:
		out := []ast.Node{}
		if v.Recv != nil {
			out = append(out, v.Recv)
		}
		if v.Type != nil {
			out = append(out, v.Type)
		}
		if v.Body != nil {
			out = append(out, v.Body)
		}
		return out
	case *ast.FuncType:
		if v.Params != nil {
			out := []ast.Node{v.Params}
			if v.Results != nil {
				out = append(out, v.Results)
			}
			return out
		}
		if v.Results != nil {
			return []ast.Node{v.Results}
		}
		return nil
	case *ast.FieldList:
		out := make([]ast.Node, 0, len(v.List))
		for _, f := range v.List {
			if f != nil {
				out = append(out, f)
			}
		}
		return out
	case *ast.Field:
		if v.Type != nil {
			return []ast.Node{v.Type}
		}
		return nil
	case *ast.BlockStmt:
		out := make([]ast.Node, 0, len(v.List))
		for _, s := range v.List {
			if s != nil {
				out = append(out, s)
			}
		}
		return out
	case *ast.ReturnStmt:
		out := make([]ast.Node, 0, len(v.Results))
		for _, r := range v.Results {
			if r != nil {
				out = append(out, r)
			}
		}
		return out
	case *ast.IfStmt:
		out := []ast.Node{}
		if v.Init != nil {
			out = append(out, v.Init)
		}
		if v.Cond != nil {
			out = append(out, v.Cond)
		}
		if v.Body != nil {
			out = append(out, v.Body)
		}
		if v.Else != nil {
			out = append(out, v.Else)
		}
		return out
	case *ast.ForStmt:
		out := []ast.Node{}
		if v.Init != nil {
			out = append(out, v.Init)
		}
		if v.Cond != nil {
			out = append(out, v.Cond)
		}
		if v.Post != nil {
			out = append(out, v.Post)
		}
		if v.Body != nil {
			out = append(out, v.Body)
		}
		return out
	case *ast.RangeStmt:
		out := []ast.Node{}
		if v.Key != nil {
			out = append(out, v.Key)
		}
		if v.Value != nil {
			out = append(out, v.Value)
		}
		if v.X != nil {
			out = append(out, v.X)
		}
		if v.Body != nil {
			out = append(out, v.Body)
		}
		return out
	case *ast.ExprStmt:
		if v.X != nil {
			return []ast.Node{v.X}
		}
		return nil
	case *ast.AssignStmt:
		out := make([]ast.Node, 0, len(v.Lhs)+len(v.Rhs))
		for _, e := range v.Lhs {
			if e != nil {
				out = append(out, e)
			}
		}
		for _, e := range v.Rhs {
			if e != nil {
				out = append(out, e)
			}
		}
		return out
	case *ast.DeclStmt:
		if v.Decl != nil {
			return []ast.Node{v.Decl}
		}
		return nil
	case *ast.GenDecl:
		out := make([]ast.Node, 0, len(v.Specs))
		for _, s := range v.Specs {
			if s != nil {
				out = append(out, s)
			}
		}
		return out
	case *ast.ValueSpec:
		out := []ast.Node{}
		for _, n := range v.Names {
			if n != nil {
				out = append(out, n)
			}
		}
		if v.Type != nil {
			out = append(out, v.Type)
		}
		for _, v := range v.Values {
			if v != nil {
				out = append(out, v)
			}
		}
		return out
	case *ast.CallExpr:
		out := []ast.Node{v.Fun}
		for _, a := range v.Args {
			if a != nil {
				out = append(out, a)
			}
		}
		return out
	case *ast.SelectorExpr:
		if v.X != nil {
			return []ast.Node{v.X}
		}
		return nil
	case *ast.BinaryExpr:
		if v.X != nil && v.Y != nil {
			return []ast.Node{v.X, v.Y}
		}
		return nil
	case *ast.UnaryExpr:
		if v.X != nil {
			return []ast.Node{v.X}
		}
		return nil
	}
	return nil
}
