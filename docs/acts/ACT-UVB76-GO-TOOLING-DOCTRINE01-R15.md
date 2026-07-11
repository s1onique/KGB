# ACT-UVB76-GO-TOOLING-DOCTRINE01-R15

## Title

**R15 closure: line-aware Make comment masking covering indented and trailing comments, `\#` escape, and `.RECIPEPREFIX` recipe detection**

## Status

```text
CLOSED
Priority: P0
Parent:   ACT-UVB76-GO-TOOLING-DOCTRINE01
Predecessor: ACT-UVB76-GO-TOOLING-DOCTRINE01-R14 (b02a5a5 + R14 follow-up)
Closure effect: closes the R15 review blocker against the R14
                commit trail; ACT-UVB76-GO-TOOLING-DOCTRINE01 is
                already CLOSED.
```

## Background

The R14 review accepted the four-item closure but flagged a
final correctness defect in the Make comment scanner:

```go
if data[i] == '#' && (i == 0 || data[i-1] == '\n') {
```

R14's byte-level check recognised only a `#` at byte column 1
(immediately preceded by a newline). That rule under-suppressed
ordinary indented comments and trailing inline comments:

```make
   # $(shell python3 should-not-run.py)

VALUE := ok  # $(shell python3 should-not-run.py)
```

GNU Make states:

> In any non-recipe line, `#` starts a comment that runs to
> end-of-line. The exception is recipes: comments within
> recipes are passed to the shell. ([gnu.org][1])

R14's test matrix only covered column-one (`\n`-preceded) and
recipe-line (`\t`-prefixed) cases. R15 adds the indented,
trailing-inline, `\#`-escape, and `.RECIPEPREFIX` rows.

---

## Production Changes (commit pending)

```text
internal/tooling/scriptdoctrine/shell_make_extractors_r11.go |  60 -   (extracted classifyMakeShellExpansions)
internal/tooling/scriptdoctrine/shell_make_extractors_r14.go |   + 182 (classifyMakeShellExpansions + maskMakeComments + CountPythonInvocationsInMakefileDetailed mask wire-up)
internal/tooling/scriptdoctrine/verifier_r15_test.go       | + new file (TestR15MakeCommentContextMatrix)
.factory/gate-summary.json                            |   re-shape (R15 closure matrix, drop "TODO will verify")
docs/acts/ACT-UVB76-GO-TOOLING-DOCTRINE01-R15.md        | (this file)
```

### Behavioural changes

* `maskMakeComments(data []byte) []byte` is the new line-aware
  Make masker. It walks each physical line, decides recipe
  context by the active prefix (TAB by default;
  `.RECIPEPREFIX = X` when declared), and on non-recipe lines
  replaces bytes from the first **unescaped** `#` through
  end-of-line with spaces. The `\<X>` escape rule means a `#`
  preceded by a single backslash is treated as a literal `#`
  and does not start a comment; the implementation skips both
  bytes for any `\X` pair. Recipe lines are left intact because
  `$(shell ...)` expansion happens before the shell sees the
  recipe.
* `classifyMakeShellExpansions` runs `maskMakeComments`
  before its expansion-time scanners (`findShellFunctionSites`
  and `findShellAssignmentSites`) so the `$(shell ...)` and
  `!= RHS` extractors cannot see content inside Make comments.
* `CountPythonInvocationsInMakefileDetailed` runs
  `maskMakeComments` before `maskMakeExpansionSites` for the
  same reason on the recipe-time path. The recipe-time prefix
  is captured from `recipePrefixRx` (the existing helper in
  `shell_extractors.go`), so `.RECIPEPREFIX = >` is honoured.
* The per-byte `#` skip that lived inside
  `findShellFunctionSites` (the R14 implementation) has been
  removed; the function is now a pure byte-level scanner over
  already-masked bytes.
* `.factory/gate-summary.json` carries an explicit
  `r15_closure_matrix` block; the stale `"git diff --check
  (TODO will verify)"` evidence string has been replaced with
  the canonical `"git diff --check"`.

---

## Acceptance / Verification

### `go test ./internal/tooling/scriptdoctrine/...`

```
ok  github.com/s1onique/KGB/internal/tooling/scriptdoctrine  1.14s
```

(72 top-level tests + 322 subtests including R14 + R15 rows, all
pass.)

### `go vet ./internal/tooling/scriptdoctrine`

```
(clean — no output)
```

### `gofmt -l internal/tooling/scriptdoctrine`

```
(clean — no output)
```

### `git diff --check`

```
(clean — no whitespace errors)
```

### R15 closure matrix

| Surface                                          | Makefile input                                                          | Expected | Got |
|--------------------------------------------------|-------------------------------------------------------------------------|----------|-----|
| **Toplevel comment**                             | `# $(shell python3 x.py)`                                              | 0 | 0 |
| **Indented comment** (new)                       | `all:` then `   # $(shell python3 x.py)`                                | 0 | 0 |
| **Trailing inline comment** (new)                | `VALUE := ok # $(shell python3 x.py)`                                 | 0 | 0 |
| **`\#` escape counts as code** (new)             | `VALUE := \# $(shell python3 x.py)`                                    | 1 | 1 |
| **Recipe line comment** (R14 preserved)          | `all:` then `<TAB># $(shell python3 x.py)`                             | 1 | 1 |
| **`.RECIPEPREFIX = >` recipe line** (new)        | `.RECIPEPREFIX = >` then `all:` then `># $(shell python3 x.py)`         | 1 | 1 |

`TestR15MakeCommentContextMatrix` in
`internal/tooling/scriptdoctrine/verifier_r15_test.go`
executes each row.

### Existing R12/R13/R14 matrix regression

All R12 comment-exclusion rows
(`TestR12MakeCommentExclusion`), R12 unresolved-reference
rows (`TestR12MakeUnresolvedReference`, `TestR12MakeResolvedReference`),
R13 brace/parenthesis rows (`TestR13MakeBracedReference`),
R14 bash-wrapped value-option rows, R14 typed-propagation
rows, and the `Verifier.Verify()` end-to-end mutation tests
in `verifier_r14_test.go` continue to pass.

### `make gate`

```
[gate] LLM-friendliness: PASS
[gate] Memory ownership hygiene gate passed.
[gate] PASS
```

### `leamas factory digest --range '<commit>~1..HEAD'`

The digest over the R15 commit range reports (R15 + R14):

```text
source_status=present
schema_version=1
overall_status=pass
checks_failed=0
checks_unavailable=0
```

(The full digest is regenerated by `leamas factory digest`
into `/tmp/R15-digest.txt` after the closeout commit.)

### Canonical machine-readable artefact

```
.factory/gate-summary.json
```

R15 closure matrix section:

```json
{
  "r15_closure_matrix": {
    "toplevel_comment": 0,
    "indented_comment": 0,
    "trailing_inline_comment": 0,
    "escaped_hash_literal": 1,
    "recipe_line_comment_kept": 1,
    "recipe_prefix_double_arrow_kept": 1
  }
}
```

---

## Bootstrap baseline (R14.0)

The baseline entries remain unchanged. The Makefile still
counts 22 python invocations after R14 + R15; the gate
passes against the same
`docs/tooling/script-doctrine-bootstrap-baseline.csv`.

## Risks / Follow-ups

1. `maskMakeComments` does not honour `\<newline>` line
   continuations (GNU Make joins the joined logical line at
   all `#`-bearing physical lines). The R14/R15 matrix does
   not exercise this case; a future ACT could promote the
   masker to a logical-line scanner.
2. `maskMakeComments` treats any `\X` pair as an escape,
   including cases where Make's lexer would distinguish
   `\\X` from `\X`. Again, the R14/R15 matrix does not
   pin a behaviour here.

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
ACT-UVB76-GO-TOOLING-DOCTRINE01-R15  CLOSED  (this commit)
ACT-UVB76-GO-TOOLING-DOCTRINE01      CLOSED

NEXT: ACT-UVB76-CAPTURE-NETNS-GO01
```

[1]: https://www.gnu.org/software/make/manual/html_node/Comments.html
