package scriptdoctrine

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// countPythonInvocationsInLine returns the number of executable Python
// command sites in a single shell line.
//
// A "command site" is one node in the parsed shell program that would
// invoke a command. Lines are parsed with mvdan.cc/sh/v3/syntax so the
// full bash grammar is honoured: simple CallExpr, branches of pipe /
// boolean BinaryCmd, body statements inside compound commands
// (if/while/for/case/subshell/block), and command substitutions inside
// any Word (e.g. printf '%s' "$(python3 -c 'print(1)')").
//
// Command-line prefix words (sudo, env, /usr/bin/env, exec, command)
// are stripped before deciding whether the first remaining word is a
// Python interpreter. The `command -flag ...` form is recognised as a
// lookup, never as an invocation.
//
// Returns -1 if the line cannot be parsed - fail-closed: malformed
// syntax might contain python that we cannot reliably see, so the
// caller must surface an internal-error diagnostic rather than
// silently treating the count as zero.
func countPythonInvocationsInLine(line string) int {
	n, err := countPythonSitesInLine(line)
	if err != nil {
		return -1
	}
	return n
}

func countPythonSitesInLine(line string) (int, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return 0, nil
	}

	// LangBash is the zero value of LangVariant. We deliberately do NOT
	// pass RecoverErrors - the parser must surface syntax errors so the
	// caller can fail closed.
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(line), "inline")
	if err != nil {
		return 0, err
	}

	w := &pythonWalker{}
	for _, stmt := range file.Stmts {
		w.walkStmt(stmt)
	}
	return w.count, nil
}

// pythonWalker counts python invocations in a parsed shell program.
//
// The walker is deliberately conservative: when a Word contains any
// expansion (e.g. cmd "$(...)" where the first part is a Lit and the
// second is a CmdSubst) Word.Lit() returns "". In that case we cannot
// identify the literal command name, so we skip counting it instead
// of guessing - but we still recurse into the substitution, which is
// where embedded python lives.
type pythonWalker struct{ count int }

func (w *pythonWalker) walkStmt(stmt *syntax.Stmt) {
	if stmt == nil {
		return
	}
	// Heredocs and file-name redirections can themselves embed
	// $(...) and backtick substitutions, so we walk them.
	for _, r := range stmt.Redirs {
		w.walkRedirect(r)
	}
	w.walkCommand(stmt.Cmd)
}

func (w *pythonWalker) walkRedirect(r *syntax.Redirect) {
	if r == nil {
		return
	}
	if r.Word != nil {
		w.walkWord(r.Word)
	}
	if r.Hdoc != nil {
		// Heredoc bodies are unexpanded Words on the AST; $() inside
		// still produces CmdSubst parts and must be scanned.
		w.walkWord(r.Hdoc)
	}
}

func (w *pythonWalker) walkCommand(cmd syntax.Command) {
	if cmd == nil {
		return
	}
	switch c := cmd.(type) {
	case *syntax.CallExpr:
		w.walkCall(c)
	case *syntax.BinaryCmd:
		// &&, ||, |, |&
		w.walkStmt(c.X)
		w.walkStmt(c.Y)
	case *syntax.IfClause:
		for _, s := range c.Cond {
			w.walkStmt(s)
		}
		for _, s := range c.Then {
			w.walkStmt(s)
		}
		if c.Else != nil {
			w.walkCommand(c.Else)
		}
	case *syntax.WhileClause:
		for _, s := range c.Cond {
			w.walkStmt(s)
		}
		for _, s := range c.Do {
			w.walkStmt(s)
		}
	case *syntax.ForClause:
		// Loop variable and iterable cannot invoke commands.
		for _, s := range c.Do {
			w.walkStmt(s)
		}
	case *syntax.CaseClause:
		for _, item := range c.Items {
			for _, s := range item.Stmts {
				w.walkStmt(s)
			}
		}
	case *syntax.Subshell:
		for _, s := range c.Stmts {
			w.walkStmt(s)
		}
	case *syntax.Block:
		for _, s := range c.Stmts {
			w.walkStmt(s)
		}
		// DeclClause / FuncDecl / ArithmCmd / ParenArithm /
		// BinaryArithm / LetClause do not invoke external commands
		// that we care about.
	}
}

func (w *pythonWalker) walkCall(call *syntax.CallExpr) {
	if call == nil {
		return
	}

	// Prefix assignments live on the CallExpr even when Args is empty
	// (e.g. `X=`python3 ...`` - bare assignment with a substitution
	// value). Their Value word can still embed $(...) or backtick
	// invocations, so walk it before deciding the call is a no-op.
	for _, as := range call.Assigns {
		if as != nil && as.Value != nil {
			w.walkWord(as.Value)
		}
	}

	if len(call.Args) == 0 {
		// Pure assignment (x=1 y=2 …) - not an invocation.
		return
	}

	// Strip recognised command prefixes. `command -flag ...` is a
	// lookup and does not count.
	args := call.Args
	for len(args) > 0 {
		lit := args[0].Lit()
		if !isCommandPrefixWord(lit) {
			break
		}
		if lit == "command" && len(args) >= 2 && strings.HasPrefix(args[1].Lit(), "-") {
			return
		}
		args = args[1:]
	}
	if len(args) == 0 {
		return
	}

	// The first remaining Word is the command. Lit() returns "" when
	// the word embeds any expansion; bail rather than mis-identify
	// the command name.
	first := args[0].Lit()
	if first == "" {
		// But the word's substitutions may still host python.
		for _, a := range args {
			w.walkWord(a)
		}
		return
	}
	if isPythonCommandWord(first) {
		w.count++
	}

	// Walk the rest of the words for embedded $(...) and backticks.
	for _, a := range args[1:] {
		w.walkWord(a)
	}
}

func (w *pythonWalker) walkWord(word *syntax.Word) {
	if word == nil {
		return
	}
	for _, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.CmdSubst:
			for _, s := range p.Stmts {
				w.walkStmt(s)
			}
		case *syntax.DblQuoted:
			for _, inner := range p.Parts {
				if cs, ok := inner.(*syntax.CmdSubst); ok {
					for _, s := range cs.Stmts {
						w.walkStmt(s)
					}
				}
			}
		case *syntax.ProcSubst:
			// Process substitutions <(cmd) and >(cmd) run the
			// inner statements; treat the same as a subshell.
			for _, s := range p.Stmts {
				w.walkStmt(s)
			}
		}
	}
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
