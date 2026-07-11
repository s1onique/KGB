# ACT-UVB76-GO-TOOLING-DOCTRINE01-R17

## Title

**R17 closure: rule-context ordering — recipe-line detection now requires `state.inRule`, blank lines preserve rule context, and the recipe-time walker reuses the R16 state machine**

## Status

```text
CLOSED
Priority: P0
Parent:   ACT-UVB76-GO-TOOLING-DOCTRINE01
Predecessor: ACT-UVB76-GO-TOOLING-DOCTRINE01-R16 (aa79c56 + R16 follow-up)
Closure effect: closes the R17 rule-context review blocker
                against the R16 commit trail; ACT-UVB76-GO-TOOLING-DOCTRINE01
                is already CLOSED.
```

## Background

R16's recipe-line detection fired unconditionally on a byte-0
prefix match, ignoring `state.inRule`. The correct GNU Make
grammar requires recipe-line classification to also be inside
an active rule block. Without this gate, a `.RECIPEPREFIX = X`
directive that itself starts with the OLD prefix (e.g. switching
from `.` to `>` via a `VAR = x` between) keeps the prefix at the
old value and silently masks the second-epoch recipe. ([gnu.org][1])

A related class of false positives appeared with the R15 line-aware
masker when blank lines were treated as a rule-context reset:
GNU Make actually allows blank and comment-only lines among
recipe lines, and the rule context spans them.

---

## Production Changes (this commit range)

```text
internal/tooling/scriptdoctrine/shell_make_extractors_r11.go        | (unchanged)
internal/tooling/scriptdoctrine/shell_make_extractors_r14.go        | ~+12 (recipe-walker reuses R16 state machine; same-line recipe support; int wrapper delegating to detailed)
internal/tooling/scriptdoctrine/shell_make_extractors_r16.go        | ~+50 (LineKind return; processMakeLogicalLine Rule 1+2+5+6 fix)
internal/tooling/scriptdoctrine/shell_extractors.go                 | ~-46 (CountPythonInvocationsInMakefile replaced with thin shim)
internal/tooling/scriptdoctrine/shell_command_parser_r10_test.go    | ~ 1 (R10 prefix-without-target test updated to R17 semantics)
internal/tooling/scriptdoctrine/verifier_r16_test.go               | 0 (R17 tests added in prior R16 commit)
.factory/gate-summary.json                                  |  r17_closure_matrix block + refreshed timestamp
docs/acts/ACT-UVB76-GO-TOOLING-DOCTRINE01-R17.md              | (this file)
```

### Behavioural changes

The R16 state machine in `shell_make_extractors_r16.go` was
extended in two ways:

1. `processMakeLogicalLine` now returns a `lineKind` value so
   the recipe-time walker (and any future caller) can use the
   same authoritative state transitions the masker uses.
2. Rule 1 (recipe line) now requires BOTH `state.inRule == true`
   AND `byte 0 == active prefix`. Rule 2 (blank line) PRESERVES
   `state.inRule` instead of resetting it. Rule 4 (comment-only)
   already preserved it; the rest of the rules are unchanged.

The recipe-time walker in `CountPythonInvocationsInMakefileDetailed`
now:

- iterates logical lines exactly the way `maskMakeComments` does;
- calls `processMakeLogicalLine` per line and uses the returned
  `lineKind` to decide whether to count;
- additionally handles same-line recipes (`target: ; cmd`) by
  calling `extractSameLineRecipe` and adding the body to
  `recipes`.

`CountPythonInvocationsInMakefile` is now a thin shim over the
typed detailed helper.

---

## Acceptance / Verification

### `go test ./internal/tooling/scriptdoctrine/...`

```
ok  github.com/s1onique/KGB/internal/tooling/scriptdoctrine  0.84s
```

(72 top-level + 350+ subtests including the 4 R17 rows; all pass.)

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

### R17 closure matrix

| Surface                                                            | Makefile input                                                                                                                                            | Expected | Got |
|--------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------|----------|-----|
| **Prefix mid-epoch reset** (REVIEW BLOCKER)                        | `.RECIPEPREFIX = .\nfirst:\n.echo first\n\nVAR = x\n.RECIPEPREFIX = >\n\nsecond:\n># $(shell python3 hidden.py)\n`                                                  | 1 | 1 |
| **Blank line preserves rule context**                              | `all:\n\n\t# $(shell python3 hidden.py)\n`                                                                                                             | 1 | 1 |
| **Comment-only line preserves rule context**                       | `all:\n\t# this is a Makefile-level comment\n\t# $(shell python3 hidden.py)\n`                                                                            | 1 | 1 |
| **Prefix-looking directive outside rule context**                   | `.RECIPEPREFIX = .\n.PHONY: all\nfirst:\n.echo first\n`                                                                                          | 0 | 0 |

The `TestR17VerifierRuleContextOrdering` mutation test additionally
pins the end-to-end `Verifier.Verify()` path: the prefix-mid-epoch
attack surface writes a multi-epoch Makefile to disk, the verifier
reads it through `Verify()`, and the test asserts that exactly one
`python-invocation` diagnostic surfaces for `Makefile`.

### `leamas factory digest --range 'b02a5a5..HEAD'`

```text
source_status=present
schema_version=1
overall_status=pass
checks_failed=0
checks_unavailable=0
```

### Existing R12 / R13 / R14 / R15 / R16 regression

All R12 / R13 / R14 / R15 / R16 rows continue to pass (the
R10 prefix-without-target row was updated to insert a target
line so the recipe prefix has a rule context — the new R17
ordering rejects prefix-starting lines outside a rule).

---

## Bootstrap baseline (R16.0)

The baseline entries remain unchanged. The Makefile still
counts 22 python invocations after R14 + R15 + R16 + R17.

## Risks / Follow-ups

1. The R17 mask + walker correctly handle the prefix-mid-epoch
   bypass and blank/comment rule-context preservation. The
   same-line recipe support relies on the existing
   `extractSameLineRecipe` helper; if a future Makefile
   extension adds another syntax (e.g. `target: cmd|cmd2`) the
   helper will need an update.
2. Rule 1 is now strictly inRule AND prefix. A prefix-starting
   line at the top of a Makefile (with no target above) is now
   classified as a top-level statement, not a recipe. The R7/R10
   tests that exercised this corner case were updated; if a
   future Makefile in the wild depends on the old lenient
   behaviour, a follow-up could add an opt-in escape hatch.

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
ACT-UVB76-GO-TOOLING-DOCTRINE01-R16  CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R17  CLOSED  (this commit)
ACT-UVB76-GO-TOOLING-DOCTRINE01      CLOSED

NEXT: ACT-UVB76-CAPTURE-NETNS-GO01
```

[1]: https://www.gnu.org/software/make/manual/html_node/Recipe-Syntax.html
[2]: https://www.gnu.org/software/make/manual/html_node/Parsing-Makefiles.html
