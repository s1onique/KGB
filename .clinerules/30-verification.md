# Verification Rules

## Quality Gate

Run `make gate` before claiming any ACT is complete.

The gate enforces:
- Required docs exist and are non-empty
- No forbidden generic naming (e.g., "kgb-agent")
- Privacy doctrine present
- Zig package structure valid
- Zig 0.16 field manual content verified
- Zig build/test/status when Zig is available

## Zig-Specific Verification

When touching Zig code, also run:

```bash
make tovarisch-build
make tovarisch-test
make tovarisch-status
```

Include exact output of these commands in your final response.

## Completion Checklist

Before marking an ACT done:

- [ ] `make gate` passes
- [ ] All modified/new files listed
- [ ] Test output included
- [ ] Status command output included (if applicable)
- [ ] Zig observations documented (if any)
