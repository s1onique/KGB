# Zig 0.16 Import Observations

## 2026-07-19 — Comptime signature does not imply expression operands

- **Context:** Hardening the allocation-tracker encapsulation gate against
  concatenated and identifier-based `@import` arguments.
- **Symptom:** Zig 0.16.0 rejects
  `@import("runtime/" ++ "allocation_tracker_internal.zig")` with
  `error: @import operand must be a string literal`; an identifier operand
  is rejected for the same reason.
- **Wrong assumption:** The documented builtin type
  `@import(comptime target: []const u8) anytype` was interpreted as allowing
  every comptime-known string expression.
- **Working fix:** Do not rely on compiler rejection as the policy boundary.
  The repository scanner lexically locates every real `@import` outside the
  trusted runtime package, accepts only one plain literal shape, and fails
  closed on concatenation, identifiers, escaped literals, or unparsed forms.
- **Files affected:**
  `internal/tooling/allocationtrackerimports/scanner.go`,
  `internal/tooling/allocationtrackerimports/selftest.go`.
- **Promote to field manual?** Yes — this distinction is reusable, but the
  curated field manual was not changed in this ACT.
