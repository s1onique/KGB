# ACT-UVB76-GO-TOOLING-DOCTRINE01-R18

## Title

**R18 closure: explicit rule-separator recognition — Make rules with prerequisites and dot-prefixed targets are now classified as targets, not directives**

## Status

```text
CLOSED
Priority: P0
Parent:   ACT-UVB76-GO-TOOLING-DOCTRINE01
Predecessor: ACT-UVB76-GO-TOOLING-DOCTRINE01-R17 (37c5dd6 + R17 follow-up)
Closure effect: closes the R18 explicit-rule-recognition review
                blocker against the R17 commit trail; ACT-UVB76-
                GO-TOOLING-DOCTRINE01 is already CLOSED.
```

## Background

The R17 mask+walker correctly required `state.inRule` for
recipe-line classification but still misclassified two valid
Makefile shapes:

1. **Ordinary rules with prerequisites.** `isMakeTargetLine()`
   required the *last* non-whitespace byte to be `:`. The
   documented GNU Make rule syntax is
   `targets : prerequisites`; rules with `prereq1 prereq2`
   after the colon end with the last prereq's last byte, not
   `:`. ([gnu.org][1])

2. **Dot-prefixed target rules.** Rule 5 unconditionally treated
   any line starting with `.` as a directive. But `.PHONY: all`
   is itself a target rule (and so is `.hidden: source`, a
   hidden-file target). ([gnu.org][2])

The R18 fix lifts both heuristics without re-introducing the
R17 review-blocker (recipe line requires rule context).

---

## Production Changes (this commit range)

```text
internal/tooling/scriptdoctrine/shell_make_extractors_r16.go | isMakeTargetLine rewritten; Rule 5 distinguishes dot-prefixed targets from directives
internal/tooling/scriptdoctrine/verifier_r18_test.go     | new (6 subtests + 1 Verifier mutation)
.factory/gate-summary.json                            |  r18_closure_matrix block + refreshed timestamp
docs/acts/ACT-UVB76-GO-TOOLING-DOCTRINE01-R18.md        | (this file)
```

### Behavioural changes

`isMakeTargetLine` was rewritten to walk the line looking for a
rule separator (`:`, `::`, `&:`, `&::`) anywhere before any
unescaped `=`. The R14 "ends-in-`:`" heuristic is gone; the
classifier now matches modern rule-with-prerequisites syntax.
`:=` is recognised as part of an assignment and skipped.

Rule 5 in `processMakeLogicalLine` was updated: a dot-prefixed
line that satisfies `isMakeTargetLine` is now classified as
`kindTarget` (not `kindDirective`). Pure directives like
`.PHONY` or `.EXPORT_ALL_VARIABLES` keep the directive branch.

The recipe-time walker continues to use the R17 rule-context
state machine, so all R12–R17 regression rows pass.

---

## Acceptance / Verification

### `go test ./internal/tooling/scriptdoctrine/...`

```
ok  github.com/s1onique/KGB/internal/tooling/scriptdoctrine  1.36s
```

(72 top-level + 364 subtests including the 6 R18 rows; all pass.)

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

### R18 closure matrix

| Surface                                                       | Makefile input                                                                          | Expected | Got |
|---------------------------------------------------------------|-----------------------------------------------------------------------------------------|----------|-----|
| **Prerequisite rule**                                          | `all: dependency\n\tpython3 hidden.py\n`                                              | 1 | 1 |
| **Order-only prerequisite**                                    | `all: \| order-only\n\tpython3 hidden.py\n`                                            | 1 | 1 |
| **Dot-prefixed target**                                        | `.RECIPEPREFIX = .\n.hidden: source\n.python3 hidden.py\n`                            | 1 | 1 |
| **`.PHONY: all` is itself a target**                           | `.PHONY: all\nall: dependency\n\tpython3 hidden.py\n`                                  | 1 | 1 |
| **Pattern rule**                                              | `%.o: %.c\n\tpython3 generator.py\n`                                                  | 1 | 1 |
| **Double-colon rule**                                         | `target:: dependency\n\tpython3 hidden.py\n`                                         | 1 | 1 |

`TestR18VerifierPrereqRuleBypass` covers the end-to-end
`Verifier.Verify()` path: an `all: dependency` Makefile written
to disk produces exactly one `python-invocation` diagnostic
for `Makefile`.

### `leamas factory digest --range '37c5dd6..HEAD'`

```text
source_status=present
schema_version=1
overall_status=pass
checks_failed=0
checks_unavailable=0
```

### Existing R12 / R13 / R14 / R15 / R16 / R17 regression

All prior rows continue to pass.

---

## Bootstrap baseline (R17.0)

The baseline entries remain unchanged. The Makefile still
counts 22 python invocations after R14 + R15 + R16 + R17 + R18.

## Risks / Follow-ups

1. `isMakeTargetLine` still does not parse every GNU Make rule
   construct. The remaining TODO is the `targets : prereqs : recipe`
   inline form (recipe on the same line as prereqs). The
   Makefile in this repo does not exercise it; a follow-up
   could add that pattern alongside the same-line recipe.
2. The Rule 5 dot-prefix branch checks `isMakeTargetLine` on the
   whole logical line. A line like `.PHONY:` (target with no
   prerequisites, ending in `:`) is also caught because the
   rule separator is still inside the line. Lines that are
   only `.NAME` (no `:` at all) fall through to `kindDirective`,
   matching GNU Make semantics.

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
ACT-UVB76-GO-TOOLING-DOCTRINE01-R17  CLOSED
ACT-UVB76-GO-TOOLING-DOCTRINE01-R18  CLOSED  (this commit)
ACT-UVB76-GO-TOOLING-DOCTRINE01      CLOSED

NEXT: ACT-UVB76-CAPTURE-NETNS-GO01
```

[1]: https://www.gnu.org/software/make/manual/html_node/Rule-Syntax.html
[2]: https://www.gnu.org/software/make/manual/html_node/Phony-Targets.html
