# Shell Containment Doctrine

**Principle:** Shell is an acceptable _wrapper_, not an acceptable _brain_.

KGB must not accumulate Unix shell as program logic. Shell may remain as thin launcher/privilege/CI glue, but new semantic orchestration must be written in Go or Python.

## Rationale

Recent work has exposed Shell-hostile responsibilities repeatedly falling into shell:

- JSON artifact validation
- jq boolean handling
- API polling decisions
- Release asset barriers
- netns phase orchestration
- Lab evidence parsing
- Retry/cooldown state machines

These should be typed, testable program logic — not Bash state machines.

## Allowed Shell Use

Shell is appropriate when it is truly a _wrapper_:

| Pattern | Example |
|---------|---------|
| Thin launcher | `#!/bin/sh` execs a typed binary |
| Privilege wrapper | `sudo`, `nsenter`, `capsh` invocation |
| Env/bootstrap wrapper | Setting env vars before exec |
| CI glue | Test orchestration that invokes binaries |
| Tiny compatibility shim | Minimal portability layer for known-stable commands |
| Typed runner exec | `exec go run ./cmd/foo` or `exec python -m foo` |

**Definition of thin wrapper:** A script that:
1. Does not exceed ~50 lines
2. Contains no control flow beyond simple error propagation
3. Delegates all decision-making to typed code
4. Uses no risky tokens (see below)

## Banned-by-Default Shell Use

These are anti-patterns that should be replaced with typed code:

| Anti-Pattern | Risk |
|--------------|------|
| JSON parsing/serialization | `jq`, `python -c json`, `grep` on JSON |
| API polling decisions | `while`/`until` loops with `sleep` |
| Phase orchestration | State machines, trap-heavy cleanup for phases |
| Retry/cooldown loops | Exponential backoff, timeout logic |
| Release publishing | `gh release` decisions, asset checks |
| Artifact schema validation | Any JSON Schema validation in shell |
| Structured data transformations | Any `jq` transformation beyond extraction |
| Boolean logic via exit codes | Complex decision trees in shell |

## Default Replacement Language

| Domain | Language | Rationale |
|--------|----------|-----------|
| Product semantics | Go | Type safety, deployment alignment |
| Lab orchestration | Go | Integration with tovarisch |
| UVB-76/tovarisch | Go | Production path |
| Repo verifiers | Python | Flexibility, JSON handling |
| Docs/reporting | Python | String processing, templating |
| Release validation | Python | Rich CLI, gh API wrapper |
| Static policy checks | Python | Regex, file analysis |

Shell remains as _only_ a wrapper around these.

## Risk Tokens

The following tokens indicate non-wrapper shell that must be migrated or grandfathered:

```
jq                          # JSON parsing beyond trivial
curl + grep/awk/sed/jq      # API response parsing
while + sleep               # Polling loops
until + sleep               # Polling loops
for ... in sleep            # Retry loops
gh release create/upload/edit  # Release decisions
trap.*cleanup.*exit         # Complex cleanup
python.*json\..*parse       # JSON in shell
grep.*\{.*\}.*json          # Regex on JSON
```

## Grandfathering

The canonical inventory is `docs/generated/shell_inventory.csv`. The Markdown (`shell_inventory.md`) is generated from it for reporting.

Existing scripts may be listed in the CSV with:
- `grandfathered_needs_owner`: risky script with identified migration owner
- `owner`: named owner or `TBD` only for initial bootstrap (`notes=Bootstrap inventory`)

**New grandfathering after this ACT requires a named owner.**

Grandfathered scripts may continue to exist but should be migrated opportunistically.

## New Script Policy

**New shell scripts must either:**

1. Pass the thin-wrapper profile (≤50 lines, no risk tokens)
2. Carry explicit annotations in the script header:
   ```bash
   # ShellJustification: <reason why shell is appropriate>
   # ShellRole: <launcher|wrapper|ci-glue|bootstrap>
   # MigrationPlan: <optional: when/if this will be migrated>
   ```

**New scripts with risky tokens MUST:**
- Be explicitly justified with ShellJustification, ShellRole, and MigrationPlan headers
- Have a named migration owner (not TBD)
- Be listed in `docs/generated/shell_inventory.csv` with disposition

## Top Migration Candidates

Based on current repository analysis, priority migration targets:

1. **JSON artifact validation** → Python/Go verifier
2. **Lab polling loops** → Go lab harness
3. **BGP/BFD state machines** → Go library
4. **Cooldown helpers** → Go or Python timer library
5. **UVB-76 capture helpers** → Python or Go

See `docs/generated/shell_inventory.md` for full inventory.

## Doctrine Index Entry

This document is part of KGB core doctrine. See also:

- [factory.md](./factory.md) — Factory workflow
- [privacy.md](./privacy.md) — Data handling principles
- [tiny-leafs.md](./tiny-leafs.md) — Leaf constraints
