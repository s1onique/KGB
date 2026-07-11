package scriptdoctrine

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// countPythonInvocationsInLine returns the number of executable Python
// command sites in a single shell line.
//
// The line is parsed as a complete shell program with
// mvdan.cc/sh/v3/syntax, then walked depth-first with `syntax.Walk`
// so every `*syntax.CallExpr` (including those reached via
// FuncDecl bodies, TimeClause targets, CoprocClause pipes,
// ForClause.Loop iterable expansions, ParamExp / ArithmExp
// substitutions, heredoc bodies, and process / command
// substitutions) is classified exactly once.
//
// Command-line prefix words (sudo, env, /usr/bin/env, exec,
// command) are stripped before deciding whether the first remaining
// word is a Python interpreter. The `command -flag ...` form is
// recognised as a lookup, never as an invocation.
//
// Returns -1 if the line cannot be parsed - fail-closed: malformed
// syntax might host python we cannot reliably see, so the caller
// must surface an internal-error diagnostic rather than silently
// treating the line as python-free.
func countPythonInvocationsInLine(line string) int {
	if strings.TrimSpace(line) == "" {
		return 0
	}
	n, err := countPythonSitesInProgram(line)
	if err != nil {
		return -1
	}
	return n
}

// countPythonSitesInProgram parses program as a complete shell
// program and counts the number of executable Python command sites
// inside it. Any parse error returns a non-nil error so the caller
// can fail closed.
func countPythonSitesInProgram(program string) (int, error) {
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(program), "inline")
	if err != nil {
		return 0, err
	}
	count := 0
	syntax.Walk(file, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}
		if v, isPython := pythonInvocationSite(call); isPython {
			count += v
		}
		return true
	})
	return count, nil
}

// pythonInvocationSite reports whether call is a python command
// invocation site. Returns (count, true) when the call counts as
// one invocation, (0, false) otherwise (data reference, lookup,
// assignment without further invocation, etc.).
//
// A CallExpr is an invocation site iff, after stripping the
// recognised command prefixes (sudo, env, /usr/bin/env, exec,
// command) the first remaining Word's literal text is a python
// interpreter or a recognised python tool (pip, pytest,
// version-suffixed python).
//
// `command -flag ...` is recognised as a lookup and never counts,
// even though it is a CallExpr.
func pythonInvocationSite(call *syntax.CallExpr) (int, bool) {
	if call == nil || len(call.Args) == 0 {
		return 0, false
	}

	// Strip recognised command prefixes.
	args := call.Args
	for len(args) > 0 {
		lit := args[0].Lit()
		if !isCommandPrefixWord(lit) {
			break
		}
		if lit == "command" && len(args) >= 2 && strings.HasPrefix(args[1].Lit(), "-") {
			return 0, false
		}
		args = args[1:]
	}
	if len(args) == 0 {
		return 0, false
	}

	// The first remaining Word is the command. Lit() returns "" when
	// the word embeds any expansion; we cannot identify the literal
	// command name in that case so we bail rather than mis-identify.
	first := args[0].Lit()
	if first == "" {
		return 0, false
	}
	if isPythonCommandWord(first) {
		return 1, true
	}
	return 0, false
}

// isCommandPrefixWord reports whether word, as the literal first token
// of a command, is a recognised command-line prefix that we strip
// before checking for python. Empty words (i.e. expanded) are not
// prefixes.
func isCommandPrefixWord(word string) bool {
	switch word {
	case "sudo", "env", "/usr/bin/env", "exec", "command":
		return true
	}
	return false
}

// isPythonCommandWord reports whether word, after stripping any
// leading path and a trailing .py extension, is a python interpreter or
// a recognised python tool name (pip, pytest).
func isPythonCommandWord(word string) bool {
	if idx := strings.LastIndex(word, "/"); idx >= 0 {
		word = word[idx+1:]
	}
	if strings.HasSuffix(word, ".py") {
		word = strings.TrimSuffix(word, ".py")
	}
	switch word {
	case "python", "pip", "pytest":
		return true
	}
	// python3, python3.10, pip3, pip3.10 …
	if strings.HasPrefix(word, "python") && len(word) > len("python") {
		rest := word[len("python"):]
		if rest[0] >= '0' && rest[0] <= '9' {
			for _, r := range rest {
				if r != '.' && (r < '0' || r > '9') {
					return false
				}
			}
			return true
		}
	}
	if strings.HasPrefix(word, "pip") && len(word) > len("pip") {
		rest := word[len("pip"):]
		if rest[0] >= '0' && rest[0] <= '9' {
			for _, r := range rest {
				if r < '0' || r > '9' {
					return false
				}
			}
			return true
		}
	}
	return false
}
