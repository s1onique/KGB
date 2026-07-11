package scriptdoctrine

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// ============================================================================
// AST walker
// ============================================================================
//
// The scriptdoctrine verifier counts executable Python command sites in
// shell-shaped inputs. The canonical implementation walks a parsed
// mvdan.cc/sh/v3 syntax tree and aggregates site counts into an
// InvocationCount value (or surfaces an error when the walker meets a
// dynamic execution surface that cannot be proven Python-free).
//
// This file holds the AST walker plus the most commonly used
// classification helpers (isPythonCommandWord, isCommandPrefixWord,
// isShellInDir, literalValueOfWord). The wrapper-handling helpers
// for sudo / env / command live in shell_wrappers.go; the bash -c
// detection lives in shell_dashc.go so each policy concern owns a small
// focused file.

// countPythonSitesInProgram parses program as a complete shell
// program and counts the number of executable Python command sites
// inside it. The structured InvocationCount return value lets
// internal callers distinguish "matched but unknowable" (a
// non-nil error) from "not matched" (a zero count with a nil
// error).
//
// Any parse error, malformed embedded script, dynamic bash -c
// payload, or other unclassifiable surface produces a non-nil
// error. The public compatibility shim CountPythonInvocations
// converts that error into -1 for callers that have not migrated
// to the typed API.
func countPythonSitesInProgram(program string) (InvocationCount, error) {
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(program), "inline")
	if err != nil {
		return ZeroCount, err
	}
	count := 0
	walkErr := error(nil)
	syntax.Walk(file, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}
		v, isPython, err := pythonInvocationSite(call)
		if err != nil {
			walkErr = err
			return false
		}
		if isPython {
			count += v
		}
		return true
	})
	if walkErr != nil {
		return ZeroCount, walkErr
	}
	return InvocationCount{Count: count}, nil
}

// pythonInvocationSite reports whether call is a python command
// invocation site. Returns (count, true, nil) when the call counts
// as one invocation, (0, false, nil) otherwise (data reference,
// lookup, assignment without further invocation, etc.), and
// (0, true, err) when the call matched an analysis surface but the
// surface is dynamic and cannot be classified statically.
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
func pythonInvocationSite(call *syntax.CallExpr) (int, bool, error) {
	if call == nil || len(call.Args) == 0 {
		return 0, false, nil
	}

	// bash -c '<script>' and sh -c '<script>'. The python
	// invocation is in the script, not in the outer CallExpr.
	if n, matched, err := countShellDashCScript(call); matched {
		if err != nil {
			return 0, true, err
		}
		return n, true, nil
	}

	// env / sudo wrappers. Skip their flags and (for env) any
	// NAME=VALUE assignments before the real command. After the
	// wrappers are stripped, the residual command may itself be a
	// bash -c invocation (R11.2 + R11.3 closure matrix); a wrapper
	// around `bash -c 'python3 x.py'` MUST still classify as one
	// python invocation. Strip prefixes AFTER wrappers so the
	// `command --` end-of-options marker can be disambiguated on
	// the residual command (e.g. `env FOO=bar command -v python3`).
	args := stripWrapperArgs(call.Args)

	// Recognised command prefixes (command, exec, plus the
	// wrappers we may not have fully stripped above). Also
	// disambiguates `command -v` (lookup) from `command --`
	// (end-of-options).
	args, lookup := stripCommandPrefixes(args)
	if lookup {
		return 0, false, nil
	}

	if len(args) == 0 {
		return 0, false, nil
	}

	// Re-check for `bash -c`/`sh -c` after wrappers and prefixes
	// have been normalised. `sudo bash -c 'python3 x.py'` would have
	// failed the direct check above because args[0] was `sudo`;
	// the residual first arg is now `bash` or `sh`.
	if n, matched, err := countShellDashCScriptWrapped(call, args); matched {
		if err != nil {
			return 0, true, err
		}
		return n, true, nil
	}

	// The first remaining Word is the command. Lit() returns ""
	// when the word embeds any expansion; we cannot identify the
	// literal command name in that case so we bail rather than
	// mis-identify.
	first := args[0].Lit()
	if first == "" {
		return 0, false, nil
	}
	if isPythonCommandWord(first) {
		return 1, true, nil
	}
	return 0, false, nil
}

// isShellInDir reports whether the literal word names a POSIX
// shell. It matches `sh` and `bash` (and `/bin/sh`, `/usr/bin/sh`,
// etc. via a path-stripping suffix match).
func isShellInDir(word string) bool {
	if idx := strings.LastIndex(word, "/"); idx >= 0 {
		word = word[idx+1:]
	}
	return word == "sh" || word == "bash"
}

// isCommandPrefixWord reports whether word, as the literal first token
// of a command, is a recognised command-line prefix that we strip
// before checking for python. Empty words (i.e. expanded) are not
// prefixes. Referenced from shell_wrappers.go via stripCommandPrefixes.
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
