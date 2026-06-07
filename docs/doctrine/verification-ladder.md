# Verification Ladder Doctrine

Factory verification is layered. Higher tiers add confidence, but they do not replace lower executable checks.

## T0: Build, typecheck, lint

The change must pass the project's normal executable gates: build, typecheck, lint, formatting checks, and unit tests required by the touched area.

T0 proves the patch is mechanically acceptable. It does not prove the original failure was fixed.

## T1: Original evidence neutralized

The evidence that motivated the ACT must be re-run or directly checked.

Examples:
- The failing command now passes.
- The suspicious log line no longer appears.
- The broken UI state is no longer reproducible.
- The exact reviewer complaint is addressed.

T1 prevents "green but irrelevant" patches.

## T2: Regression suite preserved

The change must preserve or add regression coverage for the behavior being fixed.

Examples:
- Existing tests still pass.
- A failing test is added before or alongside the fix.
- A contract fixture captures the corrected behavior.
- A verifier script checks the invariant.

T2 prevents the same bug from returning silently.

## T3: Adversarial variant search

The agent or reviewer must look for nearby ways the fix could be bypassed.

Examples:
- Similar inputs with different casing, spacing, paths, labels, or timing.
- Adjacent code paths using the same helper.
- Negative cases that should not match.
- Injection, traversal, stale-cache, malformed-payload, or partial-failure variants where relevant.

T3 is not an invitation to broaden scope. It is a focused search around the changed behavior.

## T4: Advisory maintainability review

The reviewer should assess whether the patch is understandable, minimal, and maintainable.

Examples:
- Is the diff surgical?
- Is naming clear?
- Is the abstraction justified?
- Are docs and tests aligned with the behavior?
- Is there unnecessary cleanup or unrelated churn?

T4 is advisory. It cannot replace T0–T3 executable checks.

## Non-substitution rule

Advisory review cannot replace executable verification.

A patch that "looks right" but lacks required build, test, regression, or original-evidence verification is not complete.

## Human owns merge

Agents may propose, implement, test, and review changes, but the human owns the merge decision.

The human must decide whether the evidence is sufficient, whether residual risk is acceptable, and whether the patch belongs in the project history.
