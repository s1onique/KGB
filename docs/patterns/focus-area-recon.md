# Focus-Area Recon Pattern

This pattern prevents agent swarms from duplicating the same search path. It is for broad investigations where the answer space is too large for one linear pass, but not yet structured enough for parallel implementation.

## Pattern

Use this sequence:

1. **Recon**

   * Perform one short exploratory pass.
   * Identify the major surfaces, files, modules, workflows, doctrines, or risk classes involved.
   * Do not assign multiple agents yet.

2. **Partition**

   * Split the search space into non-overlapping focus areas.
   * Each focus area must have a clear boundary and a concrete question.
   * Prefer boundaries such as subsystem, file family, workflow phase, doctrine category, or evidence type.

3. **Parallel work**

   * Assign one agent per focus area.
   * Each agent reports only within its assigned boundary.
   * Findings outside the boundary should be recorded as handoff notes, not chased immediately.

4. **Dedupe**

   * Merge overlapping findings.
   * Collapse duplicate symptoms into one root issue when evidence supports it.
   * Preserve dissent when agents disagree about severity, cause, or scope.

5. **Synthesis**

   * Produce one final answer, patch plan, or review report.
   * State which focus areas were covered.
   * State which areas remain uncovered or require a follow-up ACT.

## Relationship to impact-scan doctrine

Impact scans identify what a change can affect. Focus-area recon decides how to divide that affected space before scaling agent work.

Use impact-scan output as the input to partitioning. For example:

* If impact scan finds frontend, backend, CI, and docs effects, those can become separate focus areas.
* If impact scan finds security, runtime, UX, and test effects, those can become separate review lenses.
* If impact scan finds several workflows, partition by workflow instead of by file path when workflow ownership is clearer.

Focus-area recon is not a replacement for impact scanning. It is the coordination step that makes the scan actionable across multiple agents.

## Rejection rule

Do not scale agents horizontally before partitioning the search space.

Reject plans that say "run several agents on this" without first defining:

* the recon result,
* the focus-area boundaries,
* the question each agent must answer,
* the dedupe/synthesis owner.

Unpartitioned parallelism creates duplicated findings, contradictory patches, noisy reviews, and weak accountability.

## Acceptable same-area overlap

Intentional overlap is allowed only when the overlap has a purpose, such as:

* verifier vs finder separation,
* adversarial review of a risky area,
* independent reproduction of a severe finding.

The overlap must be declared before the work starts.

## Close-report expectations

A close report using this pattern should include:

* recon summary,
* partition list,
* per-focus-area result,
* deduped findings,
* synthesis decision,
* uncovered areas or follow-up ACTs.

---

## Wiring notes

* If `docs/doctrine/impact-scan.md` is added later, add a "Related patterns" section linking here.
* If `docs/patterns/README.md` is added as a pattern index, add this pattern to it.
