# [Closed for doctrine/schema substrate / Open for real enforcement] Epic: Embedded Memory Frugality and Leak Discipline

## Goal

Create a strict memory-management doctrine and first enforcement layer for KGB Go/Zig embedded daemons. Memory footprint and allocation behavior must become explicit product contracts, with automated gates and hermetic labs for leak detection and footprint optimization.

## Status

**This epic is CLOSED for doctrine/schema substrate. Real enforcement is still OPEN.**

Completed:
- Embedded memory doctrine added and wired into bootstrap/agent docs.
- Memory budget YAML schema added for tovarisch and UVB-76.
- Memory lab artifact schema and verifier added.
- Schema fixtures validate.
- Fast gate now checks memory budget/artifact schema.

Not yet complete:
- Real hardware memory baselines are not populated.
- Go hot-path allocation verifier is future.
- Zig allocator ownership tests are future.
- Real hermetic memory labs are future.
- Budget files are mostly `baseline_required`, so current gate validates shape, not footprint.

## Board

| ID | Work Item | Status |
|----|-----------|--------|
| mem-0001 | Add `docs/doctrine/embedded-memory-frugality.md` | done |
| mem-0002 | Create memory budget files for tovarisch | done |
| mem-0003 | Create memory budget files for uvb76 | done |
| mem-0004 | Add memory budget verifier script | done |
| mem-0005 | Add memory lab artifact schema and verifier | done |
| mem-0006 | Add tovarisch memory lab fixture | done |
| mem-0007 | Add uvb76 memory lab fixture | done |
| mem-0008 | Wire memory gates into quality_gate.sh | done |
| mem-0009 | Wire memory gates into Makefile | done |
| mem-0010 | Wire into AGENTS.md and .clinerules | done |
| mem-0011 | Register doctrine in doctrine index | done |
| mem-0012 | Add Go hot-path allocation verifier | future |
| mem-0013 | Add Zig allocator ownership tests | future |
| mem-0014 | Run memory labs on real hardware | future |

## Acceptance

- [x] Doctrine exists and is registered in `docs/doctrine/README.md`
- [x] Memory budget files exist at `docs/memory/budgets/` and validate
- [x] Memory lab artifact schema documented
- [x] Memory lab artifact verifier exists and passes self-tests
- [x] At least one tovarisch memory lab artifact fixture verified
- [x] At least one uvb76 memory lab artifact fixture verified
- [x] Fast local gate includes memory budget and artifact checks
- [x] Memory ownership hygiene gate runs in gate
- [x] New doctrine wired into AGENTS.md and .clinerules
- [ ] Go hot-path allocation verifier (future)
- [ ] Zig allocator ownership tests (future)
- [ ] Real memory lab execution (future)

## Files Changed

### New Files

- `docs/doctrine/embedded-memory-frugality.md` — Core memory doctrine
- `docs/labs/memory-lab-artifact-schema.md` — Lab artifact schema documentation
- `docs/memory/budgets/tovarisch-memory-budget.yaml` — Tovarisch RSS/PSS budgets
- `docs/memory/budgets/uvb76-memory-budget.yaml` — UVB-76 RSS/PSS budgets
- `docs/memory/fixtures/tovarisch-status-json-memory-lab.json` — Tovarisch fixture
- `docs/memory/fixtures/uvb76-status-api-memory-lab.json` — UVB-76 fixture
- `scripts/verify_memory_budgets.py` — Budget YAML verifier
- `scripts/verify_memory_lab_artifact.py` — Lab artifact verifier

### Modified Files

- `docs/doctrine/README.md` — Added embedded-memory-frugality.md to index
- `AGENTS.md` — Added doctrine to mandatory reads
- `.clinerules/00-bootstrap.md` — Added doctrine to bootstrap reads
- `Makefile` — Added memory gate targets
- `scripts/quality_gate.sh` — Added memory budget and artifact checks

## Verification Output

```
=== Memory Budget Verifier ===

A. Checking required budget files...
  Checking: tovarisch-memory-budget.yaml
    OK: Valid YAML with correct schema
    Baseline required: 24/27 (89%)
  Checking: uvb76-memory-budget.yaml
    OK: Valid YAML with correct schema
    Baseline required: 28/31 (90%)

B. Checking budget file schema consistency...
    tovarisch-memory-budget.yaml: linux/arm64 hot_paths = ['status_render', 'status_json_render', 'bgp_status_render', 'bfd_status_render', 'config_parse']
    uvb76-memory-budget.yaml: linux/arm64 hot_paths = ['status_api', 'diagnostic_capture', 'icmp_probe', 'tcp_quality_collector', 'route_collector', 'spike_diagnostics']

VERIFICATION PASSED
Memory budget files are valid.

=== Memory Lab Artifact Verifier ===

Validating: docs/memory/fixtures/tovarisch-status-json-memory-lab.json
  OK: Valid artifact
Validating: docs/memory/fixtures/uvb76-status-api-memory-lab.json
  OK: Valid artifact

VERIFICATION PASSED
```

## Next Steps

1. Add Go hot-path allocation budget verifier scanning for benchmark results
2. Add Zig allocator ownership tests using `std.testing.allocator`
3. Execute real memory labs on target hardware to populate baseline_required values
4. Create hermetic memory lab scripts for CI execution
