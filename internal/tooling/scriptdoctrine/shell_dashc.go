package scriptdoctrine

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// bash -c / sh -c script body classification.
//
// GitHub Actions and many CI tools ship shell payloads through
// `bash -c SCRIPT` (or `sh -c SCRIPT`). The scriptdoctrine parser
// must recursively classify the SCRIPT body so a python invocation
// inside it cannot escape detection.
//
// `bash` accepts GNU-style single-character option clusters:
// `-c`, `-ec`, `-ce`, `-xec`, `-ceeu` are all equivalent to
// `-c` followed by the next argument as the command string.
// Clusters such as `-xcSCRIPT` (where the script follows `c`
// inline) are NOT a valid bash invocation; bash always expects a
// separate argument. The verifier therefore:
//
//   1. recognises `c` anywhere inside a valid short-option cluster
//      (`-X…c…` where every character is a single-letter flag),
//   2. consumes the FOLLOWING non-option token as the command
//      string,
//   3. parses the command string as a complete shell program and
//      aggregates the count.
//
// Dynamic command strings — `$VAR`, `$(...)`, `\`...\`` — fail
// closed: a dynamic payload means we cannot prove the body is
// python-free, so the verifier surfaces a ClassificationError.

// countShellDashCScript detects bash/sh -c invocations and
// classifies the command string. Three return-value semantics:
//
//	not a bash/sh -c invocation:
//	    count=N/A (callers should not read), matched=false, err=nil
//
//	literal bash/sh -c payload:
//	    count=N, matched=true, err=nil
//
//	dynamic bash/sh -c payload or malformed literal payload:
//	    count=0, matched=true, err=ClassificationError
//
// The triple value lets pythonInvocationSite distinguish
// "this call matched a -c analysis surface but we cannot classify
// it" from "this call did not match -c at all" without resorting
// to sentinel-only error reporting.
func countShellDashCScript(call *syntax.CallExpr) (int, bool, error) {
	if call == nil || len(call.Args) < 2 {
		return 0, false, nil
	}
	first := call.Args[0].Lit()
	if first == "" || !isShellInDir(first) {
		return 0, false, nil
	}
	// Walk the args. We only treat this as a `-c` invocation
	// when we have actually SEEN a `-c` flag (or a short-option
	// cluster containing `c`) at any earlier position. A bare
	// `bash "$VAR"` or `bash /path/to/script arg1 arg2` is just
	// a script-execution call and is not a `-c` invocation.
	sawDashC := false
	for i := 1; i < len(call.Args); i++ {
		argLit := call.Args[i].Lit()
		switch {
		case argLit == "":
			// Dynamic argument. This is only meaningful as the
			// command string when we already saw `-c`; in that
			// case the dynamic payload fails closed.
			if sawDashC {
				return 0, true, NewClassificationError(
					"", int(call.Pos().Line()), columnFromCallArg(call, i),
					"dynamic bash -c command string")
			}
			// Positional dynamic args after `bash <script>`
			// have no script-flag interpretation; fall through
			// to the next positional check.
			return 0, false, nil
		case argLit == "--":
			// End of options. With no `-c` seen, this is a
			// script-execution call, not `-c`.
			if !sawDashC {
				return 0, false, nil
			}
			return 0, true, NewClassificationError(
				"", int(call.Pos().Line()), columnFromCallArg(call, i),
				"malformed bash -c invocation: missing command string")
		case isShortOptionCluster(argLit) && strings.ContainsRune(argLit[1:], 'c'):
			sawDashC = true
			scriptIdx := i + 1
			if scriptIdx >= len(call.Args) {
				return 0, true, NewClassificationError(
					"", int(call.Pos().Line()), columnFromCallArg(call, i),
					"malformed bash -c invocation: missing command string")
			}
			return classifyScriptWord(call, scriptIdx)
		case strings.HasPrefix(argLit, "-") && !sawDashC:
			// Long-form options other than `-c` are tolerated but
			// do not trigger the `-c` analysis by themselves.
			continue
		default:
			// First non-option positional argument. If we have
			// not yet seen a `-c` flag, this is just a script
			// path (e.g. `bash /path/to/script.sh`).
			return 0, false, nil
		}
	}
	return 0, false, nil
}

// classifyScriptWord recurses into the command-string argument at
// idx. The recursion uses countPythonSitesInProgram (a complete
// shell program parse) so the inner script benefits from the
// same fail-closed contract as a top-level shell file.
func classifyScriptWord(call *syntax.CallExpr, idx int) (int, bool, error) {
	if idx >= len(call.Args) {
		return 0, true, NewClassificationError(
			"", int(call.Pos().Line()), columnFromCallArg(call, idx-1),
			"malformed bash -c invocation: missing command string")
	}
	w := call.Args[idx]
	script, ok := literalScriptValue(w)
	if !ok {
		return 0, true, NewClassificationError(
			"", int(call.Pos().Line()), columnFromCallArg(call, idx),
			"dynamic bash -c command string")
	}
	count, err := countPythonSitesInProgram(script)
	if err != nil {
		return 0, true, NewClassificationError(
			"", int(call.Pos().Line()), columnFromCallArg(call, idx),
			"malformed nested shell command: "+err.Error())
	}
	return count.Count, true, nil
}

// isShortOptionCluster reports whether s is a valid short-option
// cluster (one or more single-letter flags prefixed by a single
// dash) and that it does NOT contain a long-form flag like
// `--xxx`. The empty string, `--`, `-`, and any string containing
// whitespace are rejected.
func isShortOptionCluster(s string) bool {
	if len(s) < 2 || s[0] != '-' || s[1] == '-' {
		return false
	}
	for _, r := range s[1:] {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

// literalScriptValue returns the inner text of a bash -c command
// string. The second return is false when the Word embeds any
// expansion (parameter expansion, command substitution, arithmetic
// expansion, process substitution, or backticks). SglQuoted and
// DblQuoted forms with only literal Lit children are accepted.
//
// A literal double-quoted string with a parameter expansion is
// dynamic even though some literal text is present; bash will
// substitute the expansion at run time. The verifier therefore
// fails closed in that case, returning ("", false).
func literalScriptValue(w *syntax.Word) (string, bool) {
	if w == nil {
		return "", false
	}
	if len(w.Parts) == 0 {
		return "", false
	}
	switch p := w.Parts[0].(type) {
	case *syntax.SglQuoted:
		return p.Value, true
	case *syntax.DblQuoted:
		return joinWordParts(p.Parts)
	}
	return joinWordParts(w.Parts)
}

// joinWordParts concatenates the literal text of a slice of
// WordParts. Returns ("", false) if any part embeds a parameter
// expansion, command substitution, arithmetic expansion or
// process substitution that we cannot reconstruct statically.
func joinWordParts(parts []syntax.WordPart) (string, bool) {
	var b strings.Builder
	for _, p := range parts {
		switch v := p.(type) {
		case *syntax.Lit:
			b.WriteString(v.Value)
		default:
			return "", false
		}
	}
	return b.String(), true
}

// columnFromCallArg returns an approximate 1-based column for an
// argument inside a CallExpr. We use the arg index plus the
// call's base position offset; the column is informational only
// (the value never appears in user-facing paths; it only feeds
// internal diagnostic counters and test assertions).
func columnFromCallArg(call *syntax.CallExpr, argIndex int) int {
	if call == nil || argIndex <= 0 {
		return 1
	}
	return 1 + argIndex
}
