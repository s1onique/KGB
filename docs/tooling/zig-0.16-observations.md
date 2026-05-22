# Zig 0.16 Observations

Recording field notes from Zig 0.16 experiments. Confidence varies; do not promote to field manual until verified with a minimal reproducer.

---

## 2026-05-22 — ArrayList initialization uncertainty in Zig 0.16

- **Context:** Attempted to use `std.ArrayList(u8)` in tests/status construction.
- **Symptom:** Expected older examples around `ArrayList.init(allocator)` did not work as-is.
- **Failed assumption:** Online examples for `std.ArrayList` are reliable for Zig 0.16.
- **Working fix:** Avoided allocation for the static v0 status payload and used a compile-time constant string.
- **Candidate doctrine:** Do not introduce dynamic allocation for status JSON until we have a local Zig 0.16-compatible buffer/list pattern.
- **Confidence:** medium; needs a minimal reproducer before promotion to the field manual.

---

## 2026-05-22 — Process entry point confirmed working

- `pub fn main(init: std.process.Init) !void` works as documented.
- `init.arena.allocator()` provides the arena allocator.
- `init.minimal.args.toSlice(arena)` works for argument extraction.
- `Io.File.Writer.init(.stdout(), init.io, buf[0..])` works for stdout/stderr writers.
- Flush stdout before `std.process.exit(...)`.

**Confidence:** high; these patterns are proven in `tovarisch/src/main.zig`.

---

## 2026-05-22 — Build system patterns confirmed

- `.root_module = b.createModule(...)` is the correct pattern in Zig 0.16.
- Module options like `.test_files` are NOT valid in `b.createModule()` options for `addTest`.
- Tests are discovered from all source files in the module; explicit test files not required.

**Confidence:** high; verified with compiler errors and successful builds.

---

## 2026-05-22 — Io.Dir API not directly usable in status.zig

- **Context:** Attempted to use `std.fs.cwd()` and `std.Io.Dir.cwd()` to check directory existence.
- **Symptom:** `std.fs.cwd()` does not exist in Zig 0.16; `std.Io.Dir.cwd()` exists but requires an `Io` context for operations.
- **Failed assumptions:**
  1. `std.fs.cwd()` was assumed to be available (old API pattern)
  2. `std.process.cwd()` was assumed to exist (doesn't exist in Zig 0.16)
  3. `std.Io.Dir.openDir(path, io, options)` requires 3 arguments including `Io` context
  4. `std.options.debug_io` doesn't exist as a simple field access
  5. Static global initialization in `const x = fn()` causes segfault at runtime for certain function calls
- **Working fix:** For v0 ACT 2, implemented `state_dir` check as a placeholder that always returns `warn`. The actual directory checking will be implemented once the `Io.Dir` API is fully understood.
- **Files affected:** `tovarisch/src/status.zig`
- **Promote to field manual:** No — this is a known limitation, not a recommended pattern. The placeholder approach is temporary.
- **Recommendation:** Investigate `std.fs.Dir.stat()` or another simpler API for directory existence checking. The old `std.fs.cwd().stat(path)` pattern is invalid in Zig 0.16.

**Confidence:** high; verified with compiler errors and successful builds.
