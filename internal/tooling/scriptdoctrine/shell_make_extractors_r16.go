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
//     a blank line was previously used, but R17 keeps inRule on
//     blanks and comments because they are allowed among recipe
//     lines).
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
//  1. Recipe line.        inRule AND byte-0 == active prefix.
//  2. Blank line.         PRESERVE inRule (no reset).
//  3. `.RECIPEPREFIX = X`  update prefix; reset rule context.
//  4. Comment-only line.  keep rule context; mask body.
//  5. Other directive.    reset rule context; mask.
//  6. Assignment / target.  reset or set inRule; mask.
//
// R17: Rule 1 requires BOTH a recipe prefix at byte 0 AND active
// rule context (state.inRule == true). This matches GNU Make: a
// prefix-starting line is a recipe only when it appears after a
// target definition and before a rule-context-reset statement
// (assignment, new target, directive, end-of-file). Blank and
// comment-only lines PRESERVE inRule so a recipe block can span
// multiple physical lines interleaved with blanks and comments.
package scriptdoctrine

import "bytes"

// makeLexState is the per-file mutable state used by
// maskMakeComments and the recipe-time walker. Each field's value
// at logical-line N reflects the directives and target lines that
// precede it.
type makeLexState struct {
	prefix byte // active recipe prefix; '\t' default
	inRule bool // inside a rule's recipe block
}

// newMakeLexState returns the makeLexState that matches GNU Make's
// startup conditions: the recipe prefix is TAB and we are not in
// a rule block yet.
func newMakeLexState() makeLexState {
	return makeLexState{prefix: '\t', inRule: false}
}

// lineKind is the classification returned by processMakeLogicalLine.
// The recipe-time walker uses the same kind to decide which lines
// are recipes to count.
type lineKind int

const (
	kindUnknown lineKind = iota
	kindRecipe
	kindDirective
	kindAssignment
	kindTarget
	kindBlank
	kindComment
)

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

// processMakeLogicalLine classifies one logical line and updates
// the lexer state. It returns the classification of the line so
// the recipe-time walker (and any future caller) can use the same
// authoritative state transitions the masker uses.
func processMakeLogicalLine(out []byte, lineStart, eol int, state *makeLexState) lineKind {
	// Rule 1 — recipe line. Requires BOTH:
	//   (a) we are currently inside a rule's recipe block
	//       (inRule == true), AND
	//   (b) the logical line's first byte equals the active
	//       recipe prefix.
	if state.inRule && lineStart < eol && out[lineStart] == state.prefix {
		return kindRecipe
	}

	// Trim leading whitespace (spaces and tabs).
	sigStart := lineStart
	for sigStart < eol && (out[sigStart] == ' ' || out[sigStart] == '\t') {
		sigStart++
	}

	// Rule 2 — blank line. Preserve rule context.
	if sigStart == eol {
		return kindBlank
	}

	// Rule 3 — `.RECIPEPREFIX = X` directive.
	if eol-sigStart >= 13 && bytes.Equal(out[sigStart:sigStart+13], []byte(".RECIPEPREFIX")) {
		processRecpePrefix(out, sigStart+13, eol, state)
		state.inRule = false
		return kindDirective
	}

	// Rule 4 — comment-only line. `\#` is a literal `#`.
	first := out[sigStart]
	if first == '\\' && sigStart+1 < eol && out[sigStart+1] == '#' {
		// Fall through.
	} else if first == '#' {
		maskCommentInMakeLine(out, sigStart, eol)
		return kindComment
	}

	// Rule 5 — top-level target OR directive.
	if first == '.' {
		// Dot-prefixed lines can be EITHER directives (`.PHONY`,
		// `.EXPORT_ALL_VARIABLES`) OR target rules (`.PHONY: all`
		// is itself a target; `.hidden: source` is a normal target
		// whose name starts with a dot). Distinguish via
		// isMakeTargetLine. R18 fix.
		if isMakeTargetLine(out, lineStart, eol) {
			state.inRule = true
			maskCommentInMakeLine(out, sigStart, eol)
			return kindTarget
		}
		state.inRule = false
		maskCommentInMakeLine(out, sigStart, eol)
		return kindDirective
	}

	// Rule 6 — assignment vs target vs other top-level.
	targetLike := isMakeTargetLine(out, lineStart, eol)
	assignmentLike := isMakeAssignmentLine(out, lineStart, eol)
	switch {
	case assignmentLike && !targetLike:
		state.inRule = false
		maskCommentInMakeLine(out, sigStart, eol)
		return kindAssignment
	case targetLike:
		state.inRule = true
		maskCommentInMakeLine(out, sigStart, eol)
		return kindTarget
	default:
		// Other top-level body (include / vpath / define / etc.).
		state.inRule = false
		maskCommentInMakeLine(out, sigStart, eol)
		return kindUnknown
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
// the trailing `:` of a target rule). Coarse heuristic.
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

// isMakeTargetLine reports whether the logical line is a Make
// rule of the form `targets: prereqs` (single-colon), `targets:: prereqs`
// (double-colon), `targets &: prereqs` or `targets &:: prereqs`.
//
// The R18 change lifts the original R14 "ends in `:`" heuristic:
// modern rule lines routinely include prerequisites, so the rule
// separator may be followed by `dep1 dep2 ...`. The classifier still
// rejects lines that are assignments (an unescaped `=` somewhere
// in the body) or that contain `:=` (which would already have been
// consumed as part of `=` scanning).
func isMakeTargetLine(out []byte, lineStart, eol int) bool {
	k := lineStart
	for k < eol {
		if k+1 < eol && out[k] == '\\' {
			k += 2
			continue
		}
		isSep := false
		switch out[k] {
		case ':':
			// Single `:` separator. Skip if it is part of `:=`.
			if k+1 < eol && out[k+1] == '=' {
				k += 2
				continue
			}
			isSep = true
		case '&':
			// `&:` or `&::`.
			if k+1 < eol && out[k+1] == ':' {
				isSep = true
			}
		}
		if isSep {
			// A rule separator BEFORE any unescaped `=` means
			// the line is a target (not an assignment).
			isAssign := false
			for j := lineStart; j < k; j++ {
				if j+1 < eol && out[j] == '\\' {
					j++
					continue
				}
				if out[j] == '=' {
					isAssign = true
					break
				}
			}
			if !isAssign {
				return true
			}
		}
		k++
	}
	return false
}

// maskCommentInMakeLine rewrites bytes from the first unescaped
// `#` to EOL with spaces. The `\X` escape rule means a `#`
// preceded by a single backslash is a literal `#` and is consumed
// (both bytes skipped) so the masking does NOT trip on it.
func maskCommentInMakeLine(out []byte, lineStart, eol int) {
	k := lineStart
	for k < eol {
		if k+1 < eol && out[k] == '\\' {
			// `\#` → literal `#` (escape pair). Skip both bytes.
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
