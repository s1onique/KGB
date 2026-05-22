# Cline/MiniMax Bootstrap

## Before Editing

1. Read `AGENTS.md` — canonical agent contract for this repo.
2. Read the current epic under `docs/epics/`.
3. Read relevant doctrine docs under `docs/doctrine/`.

## If Touching Zig

Before editing any Zig code, read:

- `docs/tooling/zig-0.16-field-manual.md`
- `docs/tooling/zig-0.16-observations.md`
- `tovarisch/src/main.zig`
- `tovarisch/src/cli.zig`
- `tovarisch/src/status.zig`

## End of Every ACT

Always include:

- **files changed**: exact list of files modified
- **tests/gate run**: output of `make gate` and any Zig tests
- **Zig observations**: if you hit any Zig 0.16-specific issues, document them
