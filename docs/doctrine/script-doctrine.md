# Script Doctrine

**Principle:** Shell and Python are forbidden. Go is the only permissible language for repository tooling.

## Summary

KGB adopts a strict tooling boundary:

1. **Python is prohibited** throughout the repository
2. **Shell is permitted only as thin wrappers** (≤50 logical LOC)
3. **Go is the default language** for all substantive tooling

## Why Python is Forbidden

Python tooling creates maintenance burden:
- Multiple Python versions across machines
- Implicit dependencies (virtualenv, pip packages)
- Testing infrastructure separate from Go
- No unified tooling experience

Python scripts in this repository must be migrated to Go.

## Why Shell is Restricted

Shell is acceptable as a thin launcher but not as a brain:

| Allowed Shell | Forbidden Shell |
|---------------|----------------|
| `exec go run ./cmd/foo` | JSON parsing |
| `chmod +x` before exec | jq transformations |
| Path resolution | API polling |
| Environment setup | State machines |
| Privilege wrappers (sudo, nsenter) | Retry/cooldown logic |

### Thin Wrapper Definition

A shell script must contain **no more than 50 logical lines** (excluding blank lines and comment-only lines) to qualify as an allowed wrapper.

Allowed responsibilities:
- Enable strict mode
- Determine repository root
- Normalize arguments/environment
- Perform preflight checks
- Invoke typed binary via `exec`
- Preserve public interfaces

Forbidden responsibilities:
- Orchestration
- JSON parsing
- State machines
- Retry loops
- Artifact validation
- Report generation
- Network workflows
- Test assertions
- Business logic

## Go Tooling Layout

Substantive tooling must be implemented as reusable Go packages:

```text
cmd/
  verify-script-doctrine/
  uvb76-capture-netns-lab/
  memory-lab/

internal/
  tooling/
    scriptdoctrine/
    capturelab/
    packaging/
    verification/
```

Commands delegate to testable packages; `main` is only the entry point.

## Inventory

All scripts are tracked in `docs/tooling/script-inventory.csv`.

Columns:
- `id`: Unique identifier
- `path`: Relative path from repo root
- `language`: `shell`, `python`, or `go`
- `logical_loc`: Lines of code (shell only; excludes blank/comment)
- `role`: wrapper, lab-orchestration, verifier, ci-glue, migrated
- `public_interface`: Make target or CLI command
- `target_go_command`: Future Go command (if applicable)
- `status`: wrapper, migration-required, migrated, third-party-exempt
- `notes`: Additional context

## Status Definitions

| Status | Meaning |
|--------|---------|
| `wrapper` | Approved thin wrapper (≤50 logical LOC) |
| `migration-required` | Must be migrated to Go |
| `migrated` | Successfully ported to Go |
| `third-party-exempt` | External code (vendor, third_party) |

## Enforcement

The Go verifier `go run ./cmd/verify-script-doctrine` fails when:

1. A repository-owned Python file exists
2. A Python shebang is present (`#!/usr/bin/env python3`)
3. Makefiles, CI files, or shell scripts invoke Python
4. A shell script exceeds 50 logical lines
5. A shell script is absent from the inventory
6. An inventory entry references a nonexistent file
7. A migrated script is still present at its legacy path
8. A shell wrapper contains known substantive patterns
9. A new script language is introduced without classification

## Migration Phases

### Phase 1: Establish Boundary
- Add this doctrine
- Add complete inventory
- Implement Go verifier
- Integrate into make gate
- Allow `migration-required` entries temporarily

### Phase 2: Port Oversized Shell Scripts
- Port in descending order of complexity
- Preserve public interfaces (Make targets)
- Shell remains only as path resolution + exec

### Phase 3: Remove Python
- Implement Go equivalent for each Python script
- Delete Python only after Go is working
- Do not create shell translations

### Phase 4: Close Migration Window
- Make all Python findings unconditional failures
- Make oversized shell scripts unconditional failures
- Prohibit new `migration-required` entries
- Verify repository works without Python installed

## Acceptance Criteria

```bash
go test ./...
go run ./cmd/verify-script-doctrine
make verify-script-doctrine
make gate
```

```bash
find . -type f -name '*.py' \
  -not -path './vendor/*' \
  -not -path './third_party/*'
```

Returns no repository-owned files.

## Doctrine Index

This document is part of KGB core doctrine. See also:

- [shell-containment.md](./shell-containment.md) — Original shell restriction policy
- [factory.md](./factory.md) — Factory workflow
- [privacy.md](./privacy.md) — Data handling principles
- [tiny-leafs.md](./tiny-leafs.md) — Leaf constraints
- [native-owned-critical-paths.md](./native-owned-critical-paths.md) — Native code preference
