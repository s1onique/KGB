# ACT: KGB Artifact Writer Architecture Baseline - CORRECTION01

**Status:** PARTIAL — scanner unification required
**ACT ID:** ACT-KGB-ARTIFACT-WRITER-ARCHITECTURE-BASELINE01
**Correction:** ACT-KGB-ARTIFACT-WRITER-ARCHITECTURE-BASELINE01-CORRECTION01
**Date:** 2026-07-20
**Original Commit:** 6e37d2f (REVERTED)
**Baseline Commit:** 9d64d8e

## Resolution Summary

All P0 defects have been addressed:

1. **Go-based artifact-writer scanner** (`cmd/artifact-writer-scanner`) - detects actual bypass patterns
2. **Finding-level matching** - compares SHA-256 finding_id values
3. **Ratchet verifier** (`cmd/ratchet-verifier`) - fails on stale/unexpected findings
4. **Valid SHA-256 hashes** - 87/87 findings have valid 64-hex hashes

## Acceptance Checkpoint

**Historical checkpoint — predates sharded-baseline correction; requires Go 1.25.12 verification**

```
observed_findings=87
approved_legacy_findings=87
baseline_matches=87
unexpected_findings=0
stale_findings=0
scan_errors=0
package_load_errors=0
status=pass_baseline_equivalent
```

## P0 Defects (Previously Identified)

### 1. False-Green Ratchet (CRITICAL) - RESOLVED

The scanner now detects actual bypass patterns (os.WriteFile, ioutil.WriteFile, fmt.Fprintf to file handles):

### 2. File-Level Matching (CRITICAL)

`categorize_findings()` uses file-level comparison:

```python
if rel_path in baseline_files:
    baseline_matches.append(...)
```

Should compare finding_id, ast_fingerprint, callee, enclosing symbol, operation, destination expression.

### 3. Stale Findings Pass (CRITICAL)

```python
# STALE_LEGACY_FINDINGs are informational
```

Required: `stale > 0 => FAIL`

### 4. Placeholder Hashes

```text
finding_id with valid 64 hex:    5
finding_id with invalid length: 55
```

Baseline entries contain obvious placeholder sequences.

### 5. Non-Semantic Fingerprint

`compute_semantic_fingerprint()` hashes path + rule_id + explanation. Should use Go AST with type resolution.

### 6. ADR Incorrect - `replace` Cannot Bypass `internal`

```text
github.com/s1onique/KGB/uvb76/cmd/...      allowed
github.com/SPbNIX/KGB/tools/wg-netlink-lab forbidden  (replace does not help)
```

## Required Correction

### Architecture Decision

The ratchet must be implemented in **Go** using `go/ast` and `golang.org/x/tools/go/packages`:

```
Producer package (Go)           ->  Baseline (JSON)         ->  Verifier (Go)
detect artifact writers         ->  87 finding_id entries   ->  compare finding_id
```

The Python secret-pattern scanner is **not** the correct foundation for artifact-writer ratchet enforcement.

### Correct Baseline Generation

```bash
# Generate sharded baseline from actual AST analysis at commit 9d64d8e
# Uses worktree to keep current scanner implementation while scanning historical code
repo="$(git rev-parse --show-toplevel)"
source_parent="$(mktemp -d)"
source_tree="$source_parent/kgb-baseline-source"
scanner_bin="$source_parent/artifact-writer-scanner"

cleanup() {
  git -C "$repo" worktree remove --force "$source_tree" 2>/dev/null || true
  rm -rf "$source_parent"
}
trap cleanup EXIT

# Create detached worktree at historical commit
git -C "$repo" worktree add --detach "$source_tree" 9d64d8e

# Build current scanner (outside worktree to preserve scanner implementation)
go -C "$repo/cmd/artifact-writer-scanner" build -o "$scanner_bin" .

# Run scanner from within worktree so paths are relative to subject root
(
  cd "$source_tree"
  "$scanner_bin" \
    --directory=. \
    --format=sharded \
    --output-dir="$repo/scripts/uvb76_artifact_secret_hygiene/findings" \
    --commit="$(git rev-parse HEAD)"
)
```

This separates:
- **Generator identity:** current corrected scanner (built once)
- **Subject identity:** tree at `9d64d8e`
- **Execution context:** scanner runs inside worktree with `.` as root (paths stay repository-relative)
- **Output location:** current correction checkout

### Baseline Sharding

To comply with LLM-friendliness line limits (500-line hard limit), the baseline is split into shards:

```
scripts/uvb76_artifact_secret_hygiene/findings/
├── manifest.json                    # Lists all shards
├── capture-netns-lab-artifacts.jsonl
├── capture-netns-polling-artifacts.jsonl
├── icmp-ping-soak-artifacts.jsonl
├── latency-crash-lab-artifacts.jsonl
├── makefile-composition-artifacts.jsonl
├── memleak-pprof-lab-artifacts.jsonl
├── memory-lab-artifacts.jsonl
├── memory-lab-evidence-artifact.jsonl
├── memory-lab-evidence-attribution.jsonl
├── memory-lab-evidence-main.jsonl
├── memory-lab-evidence-tls_config.jsonl
├── targets-crash-lab-artifacts.jsonl
├── tcp-diag-telemetry-lab-artifacts.jsonl
└── wg-netlink-lab-evidence.jsonl
```

**Shard format:** JSON Lines (one JSON object per line) for compactness.

**Loader:** Go shard loader in `internal/artifactwriterbaseline` provides:
- Deterministic merge (sorted by surface_id, file, line, finding_id)
- Duplicate detection
- Path traversal validation
- Schema version enforcement (ratchet-sharded-v1)
- Strict JSON decoding (DisallowUnknownFields)

### Required Mutation Tests

| Mutation | Expected Result |
|----------|-----------------|
| new bypass in new file | FAIL unexpected=1 |
| new bypass in same legacy file | FAIL unexpected=1 |
| changed approved call | FAIL stale=1 unexpected=1 |
| removed approved call | FAIL stale=1 |
| package load failure | FAIL |
| malformed baseline | FAIL |

## Deferred P1 Hardening (Successor Work)

The following hardening items were identified but deferred to allow this correction to close:

1. **EOF validation on JSON streams** — require a second `Decode` call to return `io.EOF` for manifest and JSONL shards
2. **Pre-flight validation** — validate all findings and derived shard names before modifying the destination
3. **Memory-lab-evidence collision resistance** — replace basename-only shard subdivision with collision-resistant canonical identity
4. **Atomic writes** — generate into fresh temporary directory, validate with `LoadAll`, then replace destination
5. **Stale shard cleanup** — remove or reject `.jsonl` files not listed in new manifest

## Blocker for ACT-UVB76-RETAINED-ARTIFACT-MIGRATION-WAVE01

Do not start migration ACT until CORRECTION01 passes Go 1.25.12 verification.

## 🔴 SCANNER UNIFICATION BLOCKER (P0)

The current implementation has two materially different scanners:

| Scanner | Finding Count | Notes |
|---------|---------------|-------|
| `uvb76/internal/producer.BypassDetector` | 60 | Used by `uvb76-artifact-writer-verify` |
| `cmd/artifact-writer-scanner` | 87 | Used by `ratchet-verifier` (includes unknown surfaces and cmd tools) |

### Architecture Constraint

Go's `internal` visibility rule prevents `cmd/artifact-writer-scanner` from importing `uvb76/internal/producer`.

Correct unification path:
```
uvb76/internal/producer          (detector logic)
        ↑
uvb76/cmd/uvb76-artifact-writer-scan  (new scanner command inside uvb76 module)
        ↑ binary/report contract
cmd/ratchet-verifier              (invokes scanner, compares to baseline)
```

### Required Actions

1. **Create scanner command inside `uvb76` module** — `uvb76/cmd/uvb76-artifact-writer-scan` that directly invokes `producer.BypassDetector`
2. **Remove duplicate detection logic** — Retire the independent `cmd/artifact-writer-scanner` implementation
3. **Bind findings to surfaces** — Every finding must bind to exactly one active catalog surface:
   - `finding ∈ exactly 1 surface` → include in ratchet comparison
   - `finding ∉ any surface` → `UNBOUND_FINDING`, fail gate (NEVER silently filter)
   - `finding ∈ multiple surfaces` → `AMBIGUOUS_SURFACE_BINDING`, fail gate
4. **Add scanner self-exclusion** — Scanner report artifacts are excluded from scanning
5. **Prove determinism** — Repeated scanner runs produce byte-identical output
6. **Regenerate baseline** — From unified scanner output
7. **Wire ratchet gate** — After scanner unification is complete

### Required Invariant

```
authoritative_count = count emitted by the unified scanner
  after console exclusions, surface binding, and scanner self-exclusions

authoritative semantic findings and multiplicities
    == ratchet observed findings and multiplicities    (PASS)
    == approved baseline findings and multiplicities   (PASS)
```

Do NOT hard-code `60` into acceptance criteria — the count depends on the unified scanner's analysis.

### Malformed JSON Fixture

The `testdata/fail_malformed_json/` directory intentionally contains malformed JSON for negative testing. The file must remain malformed; the secret-hygiene scanner must correctly classify this as a test fixture.

## Recommended Successor ACT

```
ACT-KGB-ARTIFACT-WRITER-SCANNER-AUTHORITY-CONVERGENCE01
```

Implementation sequence:
1. Add parity tests comparing internal detector and new scanner command against same fixtures
2. Create `uvb76/cmd/uvb76-artifact-writer-scan`
3. Remove duplicate detection from `cmd/artifact-writer-scanner`
4. Bind every finding to exactly one active catalog surface
5. Fail on unbound, ambiguous, parse, and scan errors
6. Add exact scanner-report self-exclusions
7. Prove raw-report determinism
8. Regenerate baseline from unified scanner
9. Wire `hulk-uvb76-artifact-producer-gate` to ratchet verifier
10. Add mutation test: new bypass → non-zero exit

## Implementation Notes

### Go Scanner Structure

```go
// cmd/artifact-writer-scanner/main.go
// Detects artifact writer patterns:
// - os.WriteFile (no artifactio)
// - ioutil.WriteFile (no artifactio)
// - os.Create + Write (no artifactio)
// - http.DetectContentType writes (no artifactio)
//
// Outputs ratchet-baseline JSON with finding_id (SHA-256 of normalized AST)
```

### Baseline Schema

```json
{
  "schema_version": "ratchet-v1",
  "baseline_commit": "9d64d8e...",
  "generator": "artifact-writer-scanner",
  "findings": [
    {
      "finding_id": "sha256:<64-hex>",
      "surface_id": "capture-netns-lab-artifacts",
      "file": "uvb76/cmd/uvb76-capture-netns-lab/internal/lab/artifacts.go",
      "operation": "os.WriteFile",
      "destination_expression": "artifactsPath",
      "enclosing_symbol": "WriteArtifacts",
      "ast_fingerprint": "sha256:<64-hex>",
      "justification": "Legacy bypass - needs migration to artifactio.WriteRedactedJSON",
      "owner": "uvb76-team",
      "successor_act": "ACT-UVB76-RETAINED-ARTIFACT-MIGRATION-WAVE01"
    }
  ]
}
```

### Topology Correction

- `uvb76/internal/artifactio` → retained for uvb76-prefixed modules
- `wg-netlink-lab` → requires module normalization + non-internal API, or external-tool adapter ACT
- Do NOT claim `replace` bypasses `internal` visibility
