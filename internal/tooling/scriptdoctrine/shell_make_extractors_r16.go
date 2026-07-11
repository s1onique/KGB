// R16 closure: a Make lexical state machine that replaces the R14/R15
// "global recipe prefix + physical-line masking" model with a
// sequential walker over logical lines (joined by backslash-newline
// continuations), tracking:
//
//   - active recipe prefix byte (TAB by default; updated each time
//     a `.RECIPEPREFIX = X` directive is processed; reset to TAB
//     when the directive is empty, i.e. `.RECIPEPREFIX =` with
//     nothing after the operator);
//   - rule context (true after a target-definition line, false after
//     a blank line or after a directive / assignment).
//
// Only NON-recipe logical lines have their unescaped-`#`-through-EOL
// bytes rewritten with spaces. Recipe lines are left intact because
// GNU Make expands `$(shell ...)` and the recipe-prefix-body before
// the shell sees them; suppressing those bytes would hide a
// legitimate execution site. `\X` is treated as a literal-X escape
// pair so `\#` is a literal `#` and does NOT start a comment.
//
// The classifier runs in priority order:
//
//  1. Byte-0 == active recipe prefix  ⇒ recipe line (no masking).
//  2. Blank logical line                  ⇒ reset rule context.
//  3. `.RECIPEPREFIX = X` directive       ⇒ update prefix, reset
//     rule context.
//  4. Comment-only line (first sig byte   ⇒ keep rule context, mask
//     is unescaped `#`)                      the comment body.
//  5. Other directive (first byte `.`)    ⇒ reset rule context, mask
//     trailing comment.
//  6. Assignment or non-target body       ⇒ reset rule context, mask
//     trailing comment.
//
// Rule 1 (byte-0 prefix) is what GNU Make actually requires for a
// line to participate in recipe context; the more nuanced case
// (a prefix-looking top-level directive such as `include` after
// `.RECIPEPREFIX = i`) is handled by the fact that `include` does
// not start with the active prefix `i` in the byte-zero position.
package scriptdoctrine

import "bytes"

// makeLexState is the per-file mutable state used by
// maskMakeComments. Each field's value at logical-line N reflects
// the directives and target lines that precede it.
type makeLexState struct {
	prefix byte // active recipe prefix; '\t' default
	inRule bool // inside a target's recipe block
}

// newMakeLexState returns the makeLexState that matches GNU Make's
// startup conditions: the recipe prefix is TAB and we are not in
// a rule block yet.
func newMakeLexState() makeLexState {
	return makeLexState{prefix: '\t', inRule: false}
}

// maskMakeComments is the R16 entry point. It walks each LOGICAL
// line (joined by `\<newline>` continuations), updates the
// makeLexState per the GNU Make semantics described at the top of
// this file, and rewrites bytes from the first unescaped `#` to
// EOL with spaces ONLY when the line is not in recipe context.
// Recipe lines are left intact so their `$(shell ...)` expansion-time
// calls survive.
func maskMakeComments(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	out := make([]byte, len(data))
	copy(out, data)
	state := newMakeLexState()
	i := 0
	for i < len(out) {
		lineStart := i
		// Walk to end-of-LINE, honouring `\<newline>` continuations
		// that join physical lines into one logical line.
		eol := lineStart
		for eol < len(out) {
			if out[eol] == '\n' {
				break
			}
			if eol+1 < len(out) && out[eol] == '\\' && out[eol+1] == '\n' {
				eol += 2
				continue
			}
			eol++
		}
		processMakeLogicalLine(out, lineStart, eol, &state)
		i = eol + 1
	}
	return out
}

// processMakeLogicalLine classifies one logical line and updates the
// lexer state. The masker is intentionally conservative: it avoids
// masking bytes that could belong to a recipe or to a directive
// that is in effect for subsequent lines.
func processMakeLogicalLine(out []byte, lineStart, eol int, state *makeLexState) {
	// Rule 1 — recipe line (byte 0 == active prefix).
	if lineStart < eol && out[lineStart] == state.prefix {
		state.inRule = true
		return
	}

	// Trim leading whitespace (spaces and tabs) so we can look
	// at the line's first significant byte for further checks.
	sigStart := lineStart
	for sigStart < eol && (out[sigStart] == ' ' || out[sigStart] == '\t') {
		sigStart++
	}

	// Rule 2 — blank line.
	if sigStart == eol {
		state.inRule = false
		return
	}

	// Rule 3 — `.RECIPEPREFIX = X` directive.
	if eol-sigStart >= 13 && bytes.Equal(out[sigStart:sigStart+13], []byte(".RECIPEPREFIX")) {
		processRecpePrefix(out, sigStart+13, eol, state)
		state.inRule = false
		return
	}

	// Rule 4 — comment-only line (first significant byte is
	// unescaped `#`). The R14/R15 matrix says these counts are
	// 0. Keep rule context so an intervening comment does not
	// end a recipe block.
	first := out[sigStart]
	if first == '\\' && sigStart+1 < eol && out[sigStart+1] == '#' {
		// \# literal: line's first content is a literal `#`,
		// which is NOT a comment-only marker. Fall through.
	} else if first == '#' {
		maskCommentInMakeLine(out, sigStart, eol)
		return
	}

	// Rule 5 — other directive (first byte `.`).
	if first == '.' {
		state.inRule = false
		maskCommentInMakeLine(out, sigStart, eol)
		return
	}

	// Rule 6 — assignment vs target vs other top-level.
	targetLike := isMakeTargetLine(out, lineStart, eol)
	assignmentLike := isMakeAssignmentLine(out, lineStart, eol)
	switch {
	case assignmentLike && !targetLike:
		state.inRule = false
		maskCommentInMakeLine(out, sigStart, eol)
	case targetLike:
		state.inRule = true
		maskCommentInMakeLine(out, sigStart, eol)
	default:
		// Other top-level body (include / vpath / define / etc.).
		state.inRule = false
		maskCommentInMakeLine(out, sigStart, eol)
	}
}

// processRecpePrefix parses the assignment `.RECIPEPREFIX =
// value` (or `:=` / `?=` / `=` variants) starting at the byte
// right after `.RECIPEPREFIX` and updates state.prefix. An empty
// value resets the prefix to TAB per GNU Make behaviour.
func processRecpePrefix(out []byte, pos, eol int, state *makeLexState) {
	k := pos
	for k < eol && (out[k] == ' ' || out[k] == '\t') {
		k++
	}
	if k+1 < eol && out[k] == ':' && out[k+1] == '=' {
		k += 2
	} else if k+1 < eol && out[k] == '?' && out[k+1] == '=' {
		k += 2
	} else if k < eol && out[k] == '=' {
		k++
	} else {
		return
	}
	for k < eol && (out[k] == ' ' || out[k] == '\t') {
		k++
	}
	if k >= eol {
		state.prefix = '\t'
		return
	}
	state.prefix = out[k]
}

// isMakeAssignmentLine reports whether the logical line is a Make
// variable assignment (the byte `=` appears somewhere other than
// the trailing `:` of a target rule). Coarse heuristic; the R16
// matrix does not pin a finer behaviour.
func isMakeAssignmentLine(out []byte, lineStart, eol int) bool {
	k := lineStart
	for k < eol {
		if k+1 < eol && out[k] == '\\' {
			k += 2
			continue
		}
		if out[k] == '=' {
			return true
		}
		k++
	}
	return false
}

// isMakeTargetLine reports whether the logical line looks like a
// target rule: the last non-whitespace byte is `:` (without an
// immediate trailing `=`, which would mark `:=`) AND there is no
// `=` in the body that would have made it an assignment.
func isMakeTargetLine(out []byte, lineStart, eol int) bool {
	k := eol - 1
	for k >= lineStart && (out[k] == ' ' || out[k] == '\t') {
		k--
	}
	if k < lineStart || out[k] != ':' {
		return false
	}
	if k+1 < eol && out[k+1] == '=' {
		return false
	}
	for j := lineStart; j < k; j++ {
		if j+1 < eol && out[j] == '\\' {
			j++
			continue
		}
		if out[j] == '=' {
			return false
		}
	}
	return true
}

// maskCommentInMakeLine rewrites bytes from the first unescaped
// `#` to EOL with spaces. The `\X` escape rule means a `#`
// preceded by a single backslash is a literal `#` and is consumed
// (both bytes skipped) so the masking does NOT trip on it.
func maskCommentInMakeLine(out []byte, lineStart, eol int) {
	k := lineStart
	for k < eol {
		if k+1 < eol && out[k] == '\\' {
			k += 2
			continue
		}
		if out[k] == '#' {
			for m := k; m < eol; m++ {
				out[m] = ' '
			}
			return
		}
		k++
	}
}
