# LLM-Friendliness Doctrine

KGB source, docs, tests, and fixtures must stay aggressively small and readable for humans and LLMs.

## Core Rule

**Large files are architecture smell.** The default response to file growth is decomposition, not tolerance.

This repo is a teaching/sample Factory project. Code that cannot be read in one sitting cannot be reviewed, maintained, or used as a teaching example.

## Why This Matters

- LLMs have context windows; large files get truncated or ignored
- Humans doing code review lose track of large files
- Small files encourage single-responsibility design
- Teaching materials require clear, focused examples
- Decomposition forces explicit thinking about boundaries

## Size Limits

| File class             | Soft warning | Hard fail | Notes                                |
| ---------------------- | -----------: | --------: | ------------------------------------ |
| Source files           |    300 lines | 450 lines | Go/Zig/Python/etc.                   |
| Markdown docs          |    250 lines | 400 lines | Epic docs can be allowlisted briefly |
| Tests                  |    350 lines | 550 lines | Split tests by behavior             |
| JSON/YAML/TOML         |    250 lines | 500 lines | Configs need special scrutiny       |
| Generated/vendor files |      ignored |   ignored | Must be explicitly excluded           |

### Byte-Size Hard Limits

| File class              | Hard fail |
| ----------------------- | --------: |
| Normal source/doc files |    32 KiB |
| Fixtures/configs        |    64 KiB |
| Generated/vendor files  |   ignored |

## Decomposition Over Allowlisting

When a file exceeds limits:

1. **Decompose first.** Split by responsibility, not convenience.
2. **Allowlisting is last resort.** If decomposition is not feasible, add an explicit exception to `.llm-friendly-ignore` with a documented reason.
3. **Document exceptions.** Every ignore pattern must include why it's necessary.

## Ignore Policy

Generated/vendor files must be **explicitly ignored** via `.llm-friendly-ignore`. Examples:

- `vendor/**` — Go dependencies
- `node_modules/**` — JS dependencies
- `*.pb.go` — Protocol buffer generated files
- `tovarisch/.zig-cache/**` — Zig build cache
- `tovarisch/.ziggy/**` — Ziggy TypeScript bindings

**Do not allow broad ignores** like `docs/**` or `src/**` — these defeat the purpose.

## Gate Behavior

The check script fails on:

- Source/doc/test files above max line count
- Source/doc/test files above max byte count
- Newly added huge fixtures without justification
- Generated/vendor files not explicitly ignored

Output format:

```
[FAIL] internal/foo/big_service.go has 612 lines; hard limit is 450.
Split by responsibility before continuing.
```

## Exceptions

To add an exception:

1. Add the pattern to `.llm-friendly-ignore`
2. Include a comment explaining why this file/module is exempt
3. Set a review date or milestone for re-evaluation
4. Prefer "defer decomposition to epic X" over permanent exceptions