# Cline/MiniMax Bootstrap

## Before Editing

1. Read `AGENTS.md` — canonical agent contract for this repo.
2. Read the current epic under `docs/epics/`.
3. Read relevant doctrine docs under `docs/doctrine/` (see `ai-native-code-discipline-axioms.md` for axiom mappings).
4. Read `docs/doctrine/native-owned-critical-paths.md` — native code preference for critical paths, anti-NIH clause.
5. Read `docs/doctrine/embedded-memory-frugality.md` — memory footprint contracts, allocation ownership, leak discipline.

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
