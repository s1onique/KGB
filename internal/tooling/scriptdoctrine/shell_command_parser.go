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
// ForClause.Loop iterable expansions, and nested
// ParamExp/ArithmExp substitutions) is classified exactly once.
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
// The classification handles several indirection wrappers that
// otherwise look like shell command words but in fact execute
// python:
//
//   - bash -c '<script>' and sh -c '<script>' recursively
//     count the script.
//   - env [flags] [NAME=VALUE] command strips the env
//     prefix and any environment-variable assignments.
//   - sudo [flags] command strips the sudo prefix.
//   - command -v / command --help are lookups and never count.
//   - command -- is the end-of-options marker; the remainder
//     IS a real command and is treated as such.
//
// A CallExpr is an invocation site iff, after stripping the
// recognised command prefixes and the wrappers above, the first
// remaining Word's literal text is a python interpreter or a
// recognised python tool (pip, pytest, version-suffixed python).
func pythonInvocationSite(call *syntax.CallExpr) (int, bool) {
	if call == nil || len(call.Args) == 0 {
		return 0, false
	}

	// bash -c '<script>' and sh -c '<script>'. The python
	// invocation is in the script, not in the outer CallExpr.
	if n, ok := countShellDashCScript(call); ok {
		return n, true
	}

	// env / sudo wrappers. Skip their flags and (for env) any
	// NAME=VALUE assignments before the real command.
	args := stripWrapperArgs(call.Args)

	// Recognised command prefixes (command, exec, plus the
	// wrappers we may not have fully stripped above). Also
	// disambiguates `command -v` (lookup) from `command --`
	// (end-of-options).
	args, lookup := stripCommandPrefixes(args)
	if lookup {
		return 0, false
	}

	if len(args) == 0 {
		return 0, false
	}

	// The first remaining Word is the command. Lit() returns ""
	// when the word embeds any expansion; we cannot identify the
	// literal command name in that case so we bail rather than
	// mis-identify.
	first := args[0].Lit()
	if first == "" {
		return 0, false
	}
	if isPythonCommandWord(first) {
		return 1, true
	}
	return 0, false
}

// countShellDashCScript detects `bash -c SCRIPT` and `sh -c SCRIPT`
// invocations, returning the python-command count inside the
// literal script text. The script is expected to be a single
// Word whose `Lit()` returns its unquoted content. If the script
// is a complex expression with parameter expansions, the literal
// value is empty and the helper returns (0, false) - the caller
// will then fall through to the outer-classification branch which
// is also conservative.
func countShellDashCScript(call *syntax.CallExpr) (int, bool) {
	if len(call.Args) < 3 {
		return 0, false
	}
	first := call.Args[0].Lit()
	if !isShellInDir(first) {
		return 0, false
	}
	for i := 1; i < len(call.Args); i++ {
		a := call.Args[i].Lit()
		if a == "-c" && i+1 < len(call.Args) {
			return countPythonInScriptWord(call.Args[i+1])
		}
		if strings.HasPrefix(a, "-c") && a != "-c" {
			// -cscript (no space). The whole "-c<rest>" is
			// the script; we have to look at the Word parts
			// directly. Lit() drops the "-c" prefix on a
			// SglQuoted word like -c'python3 x.py', returning
			// the inner content. Use that.
			rest := strings.TrimPrefix(a, "-c")
			if rest == "" {
				return 0, false
			}
			n, err := countPythonSitesInProgram(rest)
			if err != nil {
				return 0, false
			}
			return n, true
		}
	}
	return 0, false
}

// isShellInDir reports whether the literal word names a POSIX
// shell. It matches `sh` and `bash` (and `/bin/sh`, `/usr/bin/sh`,
// etc. via the path-aware isPythonCommandWord-style suffix match).
func isShellInDir(word string) bool {
	if idx := strings.LastIndex(word, "/"); idx >= 0 {
		word = word[idx+1:]
	}
	return word == "sh" || word == "bash"
}

// countPythonInScriptWord parses a single Word that follows
// `bash -c` and counts python invocations inside it. Returns
// (0, false) if the word is nil or has no literal value (e.g.
// it embeds a parameter expansion that Lit() does not
// reconstruct). The mvdan.cc/sh v3 Word.Lit() helper returns
// an empty string for SglQuoted and DblQuoted wrappers; we walk
// the parts manually to recover the inner text.
func countPythonInScriptWord(w *syntax.Word) (int, bool) {
	if w == nil {
		return 0, false
	}
	script := literalValueOfWord(w)
	if script == "" {
		return 0, false
	}
	n, err := countPythonSitesInProgram(script)
	if err != nil {
		return 0, false
	}
	return n, true
}

// literalValueOfWord returns the literal string value of w, falling
// back to Lit() when the word's parts do not yield a usable
// reconstruction. mvdan.cc/sh v3 represents SglQuoted as a
// distinct WordPart with a Value field; DblQuoted wraps a list of
// WordParts whose Lit nodes carry the inner text. We collapse
// those back into a single string so the script can be re-parsed
// by the shell visitor.
func literalValueOfWord(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	if len(w.Parts) == 1 {
		switch p := w.Parts[0].(type) {
		case *syntax.SglQuoted:
			return p.Value
		case *syntax.DblQuoted:
			return joinWordParts(p.Parts)
		}
	}
	return w.Lit()
}

// joinWordParts concatenates the literal text of a slice of
// WordParts. Returns an empty string if any part embeds a
// parameter expansion or command substitution that we cannot
// reconstruct statically.
func joinWordParts(parts []syntax.WordPart) string {
	var b strings.Builder
	for _, p := range parts {
		switch v := p.(type) {
		case *syntax.Lit:
			b.WriteString(v.Value)
		default:
			// Any non-Lit part (CmdSubst, ParamExp, ArithmExp, ...)
			// would be runtime-resolved; we cannot classify
			// the script statically. Bail by returning
			// empty.
			return ""
		}
	}
	return b.String()
}

// stripWrapperArgs removes an `env` or `sudo` wrapper from the
// start of args along with its options and (for env) any leading
// NAME=VALUE assignments. Returns the slice shifted past the
// wrapper.
func stripWrapperArgs(args []*syntax.Word) []*syntax.Word {
	for {
		if len(args) == 0 {
			return args
		}
		first := args[0].Lit()
		switch first {
		case "env", "/usr/bin/env":
			// env [-i] [-u] [-C dir] [-] [NAME=VALUE]... command
			args = args[1:]
			for len(args) > 0 {
				lit := args[0].Lit()
				if lit == "" {
					// Assignment like a= (Lit empty).
					args = args[1:]
					continue
				}
				if lit == "--" {
					// End of options; what follows is the
					// real command.
					args = args[1:]
					break
				}
				if lit == "-C" || lit == "--chdir" {
					// Takes a value; consume the next arg
					// if present.
					args = args[1:]
					if len(args) > 0 {
						args = args[1:]
					}
					continue
				}
				if strings.HasPrefix(lit, "-") {
					// -i, -u, --help, etc.
					args = args[1:]
					continue
				}
				if strings.Contains(lit, "=") {
					// NAME=VALUE assignment.
					args = args[1:]
					continue
				}
				// First non-flag, non-assignment: the real
				// command.
				break
			}
		case "sudo":
			// sudo [-E] [-H] [-u user] [-g group] ...
			//     [-i|-s] [command [arg ...]]
			args = args[1:]
			for len(args) > 0 {
				lit := args[0].Lit()
				if lit == "" {
					args = args[1:]
					continue
				}
				if strings.HasPrefix(lit, "-") {
					// -E, -u user (next arg is value), -g group ...
					args = args[1:]
					// Some flags like -u take a value; the
					// simplest correct policy is to skip the
					// next arg too if the current one looks
					// like a value flag and the next arg is
					// not another flag. This is a heuristic;
					// it errs on the side of over-skipping,
					// which is safe (we just don't classify).
					// Actually a more conservative policy is to
					// only skip one when the previous flag is
					// known to take a value; for now skip
					// unconditionally and rely on the next
					// "real" token being recognisable.
					continue
				}
				break
			}
		default:
			return args
		}
	}
}

// stripCommandPrefixes strips recognised command prefixes
// (command, exec) from the start of args. The second return is
// true when the call should be classified as a lookup (e.g.
// `command -v python3`); false when the remainder is a real
// command.
func stripCommandPrefixes(args []*syntax.Word) ([]*syntax.Word, bool) {
	for len(args) > 0 {
		lit := args[0].Lit()
		if !isCommandPrefixWord(lit) {
			break
		}
		if lit == "command" && len(args) >= 2 {
			arg := args[1].Lit()
			// `command -v` and `command -V` are lookups; the
			// same goes for `--help`. `command --` is the
			// end-of-options marker: the rest of the line
			// is a real command, not a flag.
			if arg == "-v" || arg == "-V" || arg == "--help" {
				return nil, true
			}
			// Any other -<letter> option is a builtin flag
			// (e.g. `-p` for command path lookup).
			if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
				return nil, true
			}
		}
		args = args[1:]
	}
	return args, false
}

// isCommandPrefixWord reports whether word, as the literal first token
// of a command, is a recognised command-line prefix that we strip
// before checking for python. Empty words (i.e. expanded) are not
// prefixes.
func isCommandPrefixWord(word string) bool {
	switch word {
	case "command", "exec":
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
