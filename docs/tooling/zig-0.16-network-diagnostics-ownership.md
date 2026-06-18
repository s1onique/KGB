# Zig 0.16 Network Diagnostics Ownership

This companion doc captures verified ownership lessons from `tovarisch` network diagnostics work: `tovarisch/src/status_network_diag.zig`, `tovarisch/src/net/ss_parser.zig`, and related parsers.

## Ownership Rules for Parsed Diagnostics

### Rule 1: One Explicit Owner

Every aggregate that owns allocated fields must expose a clear `deinit()` and tests must prove it frees nested allocations.

```zig
/// Network diagnostics aggregate.
/// All slices are allocator-owned; caller must call deinit().
pub const NetworkDiag = struct {
    started_at: []const u8,
    wireguard: ?WireguardDiagSection,
    interfaces: []InterfaceOutput,
    // ... more owned slices

    pub fn deinit(self: *NetworkDiag, allocator: std.mem.Allocator) void {
        allocator.free(self.started_at);
        for (self.interfaces) |iface| {
            allocator.free(iface.name);
            allocator.free(iface.operstate);
        }
        allocator.free(self.interfaces);
        // ... free all nested allocations
    }
};
```

### Rule 2: Avoid Mixed Ownership Fields

Do not mix borrowed literals and allocated strings in the same struct without clear documentation.

```zig
// AVOID: Mixed ownership - some fields are literals, others are allocated
pub const BadSocket = struct {
    name: []const u8,           // May be borrowed or allocated
    state: []const u8,          // May be borrowed or allocated
    local: ?[]const u8,         // Always allocated (nullable)
    remote: ?[]const u8,        // Always allocated (nullable)
};

// PREFERRED: Explicit ownership, nullable means allocated
pub const GoodSocket = struct {
    name: []const u8,           // Always allocated, never null
    state: []const u8,          // Always allocated, never null
    local: ?[]const u8,         // Nullable = allocated
    remote: ?[]const u8,        // Nullable = allocated
};
```

### Rule 3: No Fallback Literals for Owned Fields

Do not use `catch "0"` or `orelse "default"` when the field later participates in owned cleanup or semantic output.

```zig
// AVOID: Fallback literal creates mixed ownership problem
const state_str = try allocator.dupe(u8, socket.state) catch "ESTAB";

// PREFERRED: Explicit error handling, let caller decide on failure
const state_str = try allocator.dupe(u8, @tagName(socket.state));
```

### Rule 4: Explicit Nullable/Error Paths Over Fake Values

Prefer explicit nullable types or error paths over hiding parse/allocation failure behind fake values.

```zig
// PREFERRED: Nullable for optional data, error for parse failure
pub const TcpSocket = struct {
    local: ?[]const u8 = null,    // Optional = nullable
    rtt_ms: ?f64 = null,          // Optional = nullable
    // ...
};

// AVOID: Using fake values to hide optionality
pub const TcpSocketBad = struct {
    local: []const u8 = "N/A",   // Fake value hides missing data
    rtt_ms: f64 = -1,             // Magic number hides missing data
};
```

### Rule 5: Parser Modules Provide Matching Free Helpers

Parser modules should provide matching `free`/`deinit` helpers for whatever they allocate.

```zig
/// ss_parser.zig — Parser for `ss -tin` output
pub fn parseSsTinOutput(allocator, input, config) ParseError![]TcpSocket { ... }

/// Matching free helper for all allocated fields in TcpSocket[]
pub fn freeTcpSockets(allocator: std.mem.Allocator, sockets: []const TcpSocket) void {
    for (sockets) |socket| {
        if (socket.local) |l| allocator.free(l);
        if (socket.remote) |r| allocator.free(r);
        if (socket.process_name) |p| allocator.free(p);
    }
    allocator.free(sockets);
}
```

## Memory Pattern Summary

| Pattern | Owner | Lifetime | Free Pattern |
|---------|-------|----------|--------------|
| `[]const u8` (non-nullable) | Aggregate | Aggregate lifetime | `allocator.free()` in `deinit()` |
| `?[]const u8` (nullable) | Aggregate | Aggregate lifetime | `if (field) \|v\| allocator.free(v)` |
| String literal `"..."` | Static | Static | None |
| `[]const u8` (borrowed) | Caller | Caller lifetime | None (caller owns) |
| `?[]const u8` (borrowed nullable) | Caller | Caller lifetime | None (caller owns) |

## Confirmed Zig 0.16 Edge Cases

### `std.json.Stringify` Streaming API

Do not assume a high-level `std.json.stringify()` helper exists. Use the streaming API:

```zig
var jw = std.json.Stringify{ .writer = writer };
try jw.beginObject();
try jw.objectField("field_name");
try jw.write(value);
// ... more fields
try jw.endObject();
```

See `tovarisch/src/status.zig` for working example.

### Escaped Enum Variants for Reserved Words

When an enum variant name collides with a Zig keyword, escape it with `@"..."`:

```zig
pub const NetworkDiagStatus = enum {
    ok,
    warning,
    @"error",      // 'error' is reserved
    unavailable,
    disabled,
};
```

Use as `.@"error"` in comparisons.

### Encoder Byte Counts Before Flush

When encoding protocol messages, assign byte counts to send-buffer length fields before flushing:

```zig
const encoded_len = encodeKeepalive(encoded_buf[0..], tx_stats) catch 0;
tx_stats.send_buf_len = encoded_len;  // Assign BEFORE flush
try stream.writeAll(encoded_buf[0..tx_stats.send_buf_len]);
```

### `std.mem.copyForwards()` for Overlapping Buffers

Use `std.mem.copyForwards()` for buffer compaction (overlapping ranges). Never use `@memcpy` on overlapping ranges:

```zig
// FORBIDDEN: Zig 0.16 panics with @memcpy arguments alias
// @memcpy(buf[0..n], buf[10..10+n]);

// CORRECT: Handles overlap correctly
std.mem.copyForwards(u8, buf[0..n], buf[10..10+n]);
```

See [`zig-memory-copy-safety.md`](./zig-memory-copy-safety.md) for full documentation.

### Compile-Time OS Branching for Linux Modules

Use compile-time OS branching for Linux-only modules. Avoid treating imported modules as optionals:

```zig
fn wallClockMs() i64 {
    if (comptime @import("builtin").os.tag == .linux and
        @hasDecl(std.os.linux, "clock_gettime")) {
        var ts: std.os.linux.timespec = undefined;
        if (std.os.linux.clock_gettime(@enumFromInt(0), &ts) < 0) return 0;
        return ts.tv_sec * 1000 + @divTrunc(ts.tv_nsec, 1_000_000);
    }
    return 1718700000000;  // Fallback for non-Linux
}
```

### Linux Watcher Abstractions Match Public API

Wrapper functions should match actual public API, not imagined methods:

```zig
// CORRECT: Matches actual inotify API
pub fn inotify_init() c_int {
    return std.c.inotify_init();
}

// WRONG: Assumed API that doesn't exist
// pub fn initInotify() !i32 { ... }
```

### BufPrint Aliasing

Chained `bufPrint()` calls with same buffer cause aliasing panics:

```zig
// FORBIDDEN: Same buffer aliases
const parent = try std.fmt.bufPrint(&buf, "{s}/{s}", .{ base, iface });
const child = try std.fmt.bufPrint(&buf, "{s}/statistics", .{parent}); // PANIC

// CORRECT: Distinct buffers
var parent_buf: [4096]u8 = undefined;
var child_buf: [4096]u8 = undefined;
const parent = try std.fmt.bufPrint(&parent_buf, "{s}/{s}", .{ base, iface });
const child = try std.fmt.bufPrint(&child_buf, "{s}/statistics", .{parent});
```

## Manifesto Axioms as Reviewer Guidance

Recent work should improve by making ownership, lifetimes, fallbacks, and public APIs mechanically obvious. The following Axiom guidance applies to code review:

**Axiom 1 (Repo-Local Project Memory):** Field lessons from Zig 0.16 work belong in `docs/tooling/zig-0.16-observations.md`. ACT outcomes must leave breadcrumbs.

**Axiom 2 (Cold-Resume Checkpoint):** Close reports must include files changed, verification output, and next exact step.

**Do NOT add or modify Manifesto axioms.** This section provides guidance on how existing axioms apply to recent work.

## Related Code

- `tovarisch/src/status_network_diag.zig` — Complete `NetworkDiag.deinit()` implementation
- `tovarisch/src/net/ss_parser.zig` — `TcpSocket` ownership and `freeTcpSockets()`
- `tovarisch/src/net/wg_dump_parser.zig` — Redaction helpers that return owned strings
- `tovarisch/src/status_ownership_tests.zig` — Tests proving `deinit()` correctness

## Narrow Field Manual Verifier

A narrow verifier exists to validate field manual sections exist and catch known dangerous patterns in network diagnostics code.

### What It Checks

1. **Field manual sections exist:**
   - `docs/tooling/zig-0.16-field-manual.md` must contain reference to network diagnostics ownership
   - Must contain reference to confirmed edge cases

2. **Dangerous patterns in `tovarisch/src/net`:**
   - `catch "0"` or `catch ""` in ownership-sensitive code (mixed ownership trap)
   - `@memcpy` without `MemoryCopySafety` annotation (overlap panic trap)

### Running the Verifier

```bash
./scripts/verify_field_manual_network_diag.sh
```

### Wire-in

The verifier is optional and not wired into `make gate` by default. Run manually when updating network diagnostics code.

### Self-Test

```bash
./scripts/verify_field_manual_network_diag.sh --self-test
```

This validates the verifier itself against known fixtures.
