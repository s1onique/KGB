package producer

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// callName returns the fully-qualified name of a call expression.
//
// For a method call like `f.Write(...)` we return both `f.Write` (with
// receiver) and `Write` (method name without receiver). The detector
// matches against both forms so that shared helper variables like
// `f := os.Create(...)` followed by `f.Write(...)` are caught without
// requiring a literal `File.Write` reference in the AST.
func callName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		x := selectorRoot(fun)
		if x == nil {
			return fun.Sel.Name
		}
		if id, ok := x.(*ast.Ident); ok {
			// Receiver-style method call: use the method name as the
			// canonical call name so the detector can match against the
			// common "File.Write" / "File.WriteString" patterns.
			if id.Name != "" && isLikelyFileReceiver(id.Name) {
				return "File." + fun.Sel.Name
			}
			return id.Name + "." + fun.Sel.Name
		}
		return fun.Sel.Name
	}
	return ""
}

// isLikelyFileReceiver reports whether the receiver identifier name looks
// like a file handle, encoder, or buffered writer (so the call is treated
// as a File.*, json.NewEncoder.Encode, or bufio.NewWriter.* method).
//
// We deliberately accept common short names. This is a heuristic: false
// positives produce extra findings that are reviewed, false negatives
// miss bypass calls. We err on the side of detection.
func isLikelyFileReceiver(name string) bool {
	switch name {
	case "f", "file", "fh", "out", "fp", "w":
		return true
	case "enc", "encoder", "dec", "decoder":
		return true
	case "bw", "buf", "wr", "writer":
		return true
	}
	return false
}

func selectorRoot(e ast.Expr) ast.Expr {
	switch v := e.(type) {
	case *ast.SelectorExpr:
		return selectorRoot(v.X)
	default:
		return e
	}
}

// isBypassingCall reports whether funName is a known bypassing write call.
//
// The detector flags the call site, but only EMITS a finding when the
// enclosing FuncDecl matches one of the file's bindings (or when the
// contract declares the file as fully dedicated).
func (d *BypassDetector) isBypassingCall(funName string) bool {
	switch funName {
	case "os.WriteFile", "ioutil.WriteFile",
		"os.Create", "os.CreateTemp",
		"os.OpenFile", "os.Rename",
		"json.NewEncoder.Encode",
		"fmt.Fprint", "fmt.Fprintf", "fmt.Fprintln",
		"File.Write", "File.WriteString",
		"io.Copy", "bufio.NewWriter":
		return true
	}
	return false
}

// isConstructionOnlyCall reports a call that constructs a buffered writer
// or encoder but does not by itself write to disk. We use this to avoid
// flagging the constructor in isolation; a follow-up write call inside the
// same FuncDecl is the actual persistence point.
func isConstructionOnlyCall(funName string) bool {
	switch funName {
	case "bufio.NewWriter",
		"json.NewEncoder":
		return true
	}
	return false
}

// destinationFromCall attempts to resolve the destination of the call. It
// returns (resolved, reason) where resolved is "" when resolution failed.
func (d *BypassDetector) destinationFromCall(call *ast.CallExpr, fn string) (string, string) {
	switch fn {
	case "os.WriteFile", "ioutil.WriteFile":
		if len(call.Args) >= 1 {
			if s, ok := stringLiteralValue(call.Args[0]); ok {
				return s, "literal"
			}
			if path := resolveToStringFromJoinedExpr(call.Args[0]); path != "" {
				return path, "filepath.Join"
			}
		}
	case "os.Create", "os.CreateTemp":
		if len(call.Args) >= 1 {
			if s, ok := stringLiteralValue(call.Args[0]); ok {
				return s, "literal"
			}
		}
	case "os.OpenFile":
		if len(call.Args) >= 1 {
			if s, ok := stringLiteralValue(call.Args[0]); ok {
				return s, "literal"
			}
		}
	case "File.Write", "File.WriteString":
		// No destination string; receiver is the file handle.
		return "file-receiver", "method-receiver"
	}
	return "", "unknown"
}

func stringLiteralValue(expr ast.Expr) (string, bool) {
	bl, ok := expr.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	s, err := unquoteString(bl.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

func unquoteString(s string) (string, error) {
	if len(s) < 2 {
		return "", fmt.Errorf("literal too short")
	}
	if s[0] != '"' || s[len(s)-1] != '"' {
		return "", fmt.Errorf("not a quoted string")
	}
	return s[1 : len(s)-1], nil
}

// resolveToStringFromJoinedExpr looks for filepath.Join(...) calls with
// literal or variable parts.
func resolveToStringFromJoinedExpr(expr ast.Expr) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}
	if callName(call) != "filepath.Join" {
		return ""
	}
	var parts []string
	for _, a := range call.Args {
		if s, ok := stringLiteralValue(a); ok {
			parts = append(parts, s)
			continue
		}
		if id, ok := a.(*ast.Ident); ok {
			parts = append(parts, id.Name)
		}
	}
	return strings.Join(parts, "/")
}

// hasWriteFlagArg reports whether os.OpenFile is called with a write flag.
func hasWriteFlagArg(call *ast.CallExpr) bool {
	for _, a := range call.Args {
		bl, ok := a.(*ast.BinaryExpr)
		if !ok || bl.Op != token.OR {
			continue
		}
		if hasWriteFlag(bl) {
			return true
		}
	}
	return false
}

// hasWriteFlag walks a binary OR expression searching for write-flag identifiers.
// Both direct identifiers (O_WRONLY) and selector-style flags (os.O_WRONLY)
// are accepted because os.OpenFile callers frequently use the qualified form.
func hasWriteFlag(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.BinaryExpr:
		return hasWriteFlag(v.X) || hasWriteFlag(v.Y)
	case *ast.Ident:
		return isWriteFlagIdent(v.Name)
	case *ast.SelectorExpr:
		return isWriteFlagIdent(v.Sel.Name)
	}
	return false
}

// isWriteFlagIdent reports whether an identifier name is one of the standard
// os package write-mode flags (case-insensitive substring match).
//
// The shortcut aliases (wr/rd/cr/tr/ap) match common local-const aliases
// declared in tests and helper code. Production code uses os.O_* constants
// directly which the substring check already accepts.
func isWriteFlagIdent(name string) bool {
	switch strings.ToLower(name) {
	case "wr", "write", "wronly":
		return true
	case "rd", "read":
		return true
	case "cr", "create":
		return true
	case "tr", "trunc":
		return true
	case "ap", "append":
		return true
	}
	name = strings.ToUpper(name)
	return strings.Contains(name, "O_WRONLY") ||
		strings.Contains(name, "O_CREATE") ||
		strings.Contains(name, "O_TRUNC") ||
		strings.Contains(name, "O_APPEND") ||
		strings.Contains(name, "O_RDWR")
}
