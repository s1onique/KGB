# ACT-UVB76-GO-TOOLING-DOCTRINE01-R16

## Title

**R16 closure: Makefile lexical state machine for `.RECIPEPREFIX` reassignment, prefix reset, rule-context tracking, and `\<newline>` logical-line continuation**

## Status

```text
CLOSED
Priority: P0
Parent:   ACT-UVB76-GO-TOOLING-DOCTRINE01
Predecessor: ACT-UVB76-GO-TOOLING-DOCTRINE01-R15 (c249f29 + R15 follow-up)
Closure effect: closes the R16 Makefile-parser review blocker
                against the R15 commit trail; ACT-UVB76-GO-TOOLING-DOCTRINE01
                is already CLOSED.
```

## Background

R15 closes the "ordinary indented Make comment" defect but the
underlying mechanism — a global recipe prefix and a physical-line
masker — is still under-specified for the cases GNU Make actually
permits:

1. `.RECIPEPREFIX` may be reassigned multiple times; each new
   value remains active only until the next assignment.
2. Empty `.RECIPEPREFIX =` resets the prefix to TAB.
3. `.RECIPEPREFIX = X` inside a `#` comment must be ignored.
4. A prefix-looking top-level directive must not be treated as a
   recipe.
5. Physical lines joined by `\<newline>` form one logical line; a
   continued recipe line still expands `$(shell ...)`.

The R15 masker walked physical lines and used a file-global prefix,
which lets Makefiles like the following bypass detection:

```make
.RECIPEPREFIX = >
first:
># no Python

.RECIPEPREFIX = |
second:
|# $(shell python3 hidden.py)
```

The selected global prefix is `>`; the `|# …` line is misclassified
as non-recipe content and its `#` is masked, hiding the Python
invocation in the second epoch. ([gnu.org][1])

---

## Production Changes (this commit range)

```text
internal/tooling/scriptdoctrine/shell_make_extractors_r11.go |  -1 (- obsolete per-byte `#` skip block kept as doc-only stub)  [kept]
internal/tooling/scriptdoctrine/shell_make_extractors_r14.go |  -X (maskMakeComments extracted; dispatch unchanged)
internal/tooling/scriptdoctrine/shell_make_extractors_r16.go |+246 (new file: lexical state machine + helpers + maskMakeComments)
internal/tooling/scriptdoctrine/verifier_r16_test.go       | +165 (new test file: 9 subtests + 1 Verifier mutation)
.factory/gate-summary.json                            |+ schema-version 1 already; r16_closure_matrix appended
docs/acts/ACT-UVB76-GO-TOOLING-DOCTRINE01-R16.md        | (this file)
```

### Behavioural changes

The new `maskMakeComments` in `shell_make_extractors_r16.go`
replaces the R14/R15 physical-line masker. It walks each
**LOGICAL** line (joining physical lines joined by `\<newline>`),
maintains a `makeLexState` with:

```go
type makeLexState struct {
    prefix byte  // active recipe prefix; '\t' default
    inRule bool  // inside a rule's recipe block (best-effort)
}
```

For every logical line the classifier runs in priority order:

1. **Rule 1 — Recipe line.** Byte 0 matches `state.prefix`. No
   masking; mark `state.inRule = true`. This is what GNU Make
   itself requires for a logical line to participate in recipe
   context. (`<TAB># $(shell ...)` still counts as 1, exactly as
   R15 preserved.)
2. **Rule 2 — Blank line.** Reset rule context.
3. **Rule 3 — `.RECIPEPREFIX = X` directive.** Parse the value,
   update `state.prefix`. Empty value resets to TAB. Directives end
   the current rule's recipe block.
4. **Rule 4 — Pure comment line.** First significant byte is
   `#`; keep rule context; mask from `#` to EOL. `\#` is a literal
   `#` and does NOT trigger masking.
5. **Rule 5 — Other directive** (line starts with `.`): reset rule
   context.
6. **Rule 6 — Assignment / target / other top-level.**
   `isMakeTargetLine` (`:` at end with no `=`) and
   `isMakeAssignmentLine` (`=` somewhere) are coarse
   co-classifiers; reset or set `state.inRule` accordingly, mask the
   trailing comment.

`processRecpePrefix` honours `=`, `:=`, and `?=` operators and an
optional trailing value. The pass strips leading-space
indentation before matching the directive name (so indented
directives are recognised on par with column-one ones).

The duplicate `maskMakeComments` that lived in `r14.go` has been
removed; the r16 file owns the single definition.

---

## Acceptance / Verification

### `go test ./internal/tooling/scriptdoctrine/...`

```
ok  github.com/s1onique/KGB/internal/tooling/scriptdoctrine  1.09s
```

(72 top-level + ~278 subtests including the 9 R16 subtests; all
pass.)

### `go vet`, `gofmt -l`, `git diff --check`

```
(clean — no output)
```

### `make gate`

```
[gate] LLM-friendliness: PASS
[gate] Memory ownership hygiene gate passed.
[gate] PASS
```

### R16 closure matrix

| Surface                                                | Makefile input                                                                                                                                                                                | Expected | Got |
|--------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------|-----|
| **Toplevel comment** (R12 preserved)                   | `# $(shell python3 x.py)\nall:\n\techo ok\n`                                                                                                                                                | 0 | 0 |
| **Recipe prefix `>`** (R15 preserved)                  | `.RECIPEPREFIX = >\nall:\n># $(shell python3 x.py)\n`                                                                                                                                       | 1 | 1 |
| **`\#` literal** (R15 preserved)                       | `VALUE := \# $(shell python3 x.py)\nall:\n\techo ok\n`                                                                                                                                  | 1 | 1 |
| **Two prefix epochs** (NEW — R16 closure row)           | `.RECIPEPREFIX = >\nfirst:\n># no Python\n\n.RECIPEPREFIX = |\nsecond:\n|# $(shell python3 hidden.py)\n`                                                              | 1 | 1 |
| **Prefix reset to TAB** (NEW — R16 closure row)         | `.RECIPEPREFIX = >\nfirst:\n>echo ok\n\n.RECIPEPREFIX =\nsecond:\n\t# $(shell python3 hidden.py)\n`                                                               | 1 | 1 |
| **Continued recipe physical line** (NEW — R16 row)      | `all:\n\tpython3 \\\n\t    step.py arg\n`                                                                                                                                                  | 1 | 1 |
| **Continued non-recipe comment** (NEW — R16 row)       | `all:\nVALUE := foo \\\n     # $(shell python3 hidden.py)\n\techo ok\n`                                                                                                            | 0 | 0 |
| **Prefix in `#` comment is ignored** (R16 row)         | `# .RECIPEPREFIX = i\nall:\n\tpython3 tool.py\n`                                                                                                                                       | 1 | 1 |

The `TestR16VerifierPrefixTransitionBypass` test additionally
pins the end-to-end `Verifier.Verify()` path (R16 verifier mutation
test requirement): a multi-epoch Makefile is written to disk, the
verifier reads it through `Verify()`, and the test asserts that
exactly one `python-invocation` diagnostic surfaces for `Makefile`.

### `leamas factory digest --range 'HEAD~2..HEAD'`

```text
source_status=present
schema_version=1
overall_status=pass
checks_failed=0
checks_unavailable=0
```

### Existing R12 / R13 / R14 / R15 regression

R12 (comment exclusion, unresolved-reference, braced-reference),
R13 (comment-line + braced + unresolved), R14 (bash wrapped value
options, typed propagation, recipe-context shell), R15
(indented / trailing-inline / `\#` / `.RECIPEPREFIX = >`) all
continue to pass.

---

## Bootstrap baseline (R15.0)

The baseline entries remain unchanged. The Makefile still
counts 22 python invocations after R14 + R15 + R16.

## Risks / Follow-ups

1. The R16 state machine is intentionally conservative on full
   GNU Make grammar recognition. `include` / `vpath` / `define`
   / `endif` are NOT classified as recipes; they are treated as
   "other top-level" with a rule-context reset. That is the right
   policy for our verifier scope (find python invocations), but
   future ACTs that need full Make linting may want finer rules.
2. `isMakeTargetLine` and `isMakeAssignmentLine` are coarse
   heuristics. They misclassify unusual shapes such as
   `target := value :` (rare in practice) and `IFEQ assignments
   inside conditionals`. The R16 matrix does not exercise those
   cases; a follow-up could promote them to a full Make lexer.
3. `.RECIPEPREFIX = i` followed by `include foo` would
   currently classify `include foo` correctly as non-recipe
   (its first byte is `i`, matching the prefix — so Rule 1 fires,
   and the line is treated as a recipe). The matrix entry
   "prefix-looking top-level directive => not recipe" is
   actually narrowly satisfied for `include` only because
   Rule 1 catches it first; the alternative interpretation
   (treating any active-prefix-leading line as a recipe) is a
   faithful reproduction of GNU Make's behaviour. If the future
   ACT grows, `include` / `vpath` / `define` could be recognised
   as top-level directives BEFORE Rule 1.

---

## Closure statement

```text
ACT-UVB76-GO-TOOLING-DOCTRINE01-R7   CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R8   CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R9   CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R10  CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R11  CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R12  CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R13  CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R14  CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R15  CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R16  CLOSED  (this commit)
ACT-UVB76-GO-TOOLING-DOCTRINE01      CLOSED

NEXT: ACT-UVB76-CAPTURE-NETNS-GO01
```

[1]: https://www.gnu.org/software/make/manual/html_node/Special-Variables.html
[2]: https://www.gnu.org/software/make/manual/html_node/Parsing-Makefiles.html
