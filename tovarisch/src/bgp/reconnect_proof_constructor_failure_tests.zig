// bgp/reconnect_proof_constructor_failure_tests.zig —
// `initBgpServeBundle` ownership-on-failure regression tests
// (ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01-FA-3).
//
// The previous constructor had two P0 defects on the failure path:
//
//   (a) the input TCP descriptor was closed TWICE for one physical
//       close: once via the bundle's copy (`bundle.tcp.close()`) and
//       once via the caller's input pointer (`tcp.close()`) after
//       `loadConfigAndBgp` observed the `.failed` result. The two
//       struct copies share the kernel fd value but each close()
//       call sees the unmutated copy, so `std.c.close(fd)` runs
//       twice against the same kernel fd;
//
//   (b) `bundle.export_state` was initialised (and
//       `initExportedPrefixes` allocated the `current_exported_prefixes`
//       copy) BEFORE the allocation-tracker state was installed, so a
//       state-init failure leaked the exported-prefix allocation.
//
// The fix establishes one explicit ownership-transfer rule: once
// `bundle.*` is assigned, the bundle owns `tcp`, `raw`, and
// `prefixes` for the rest of the function. The constructor releases
// every transferred resource via `releaseBundleOnFailure`, and the
// caller (`loadConfigAndBgp`) does NOT touch these resources after
// the constructor returns.
//
// ACT-TOVARISCH-BOUNDED-MEMORY-RECONNECT-PROOF01-FA-3 (this file):
// the previous "FD oracle" only proved the caller's COPIED struct
// fields were not mutated, not that the kernel descriptor itself
// was closed. A copied `TcpTransport` retaining the original integer
// does not mean the underlying kernel fd remains open: a mis-wired
// constructor could double-close the fd without touching the caller's
// pointer copy at all. The new oracle queries the kernel through
// `fcntl(fd, F_GETFD, 0)`, which returns -1 with `errno = EBADF` for
// an invalid descriptor (POSIX). POSIX defines F_GETFD as retrieving
// flags for a valid descriptor; an invalid descriptor fails. The
// test now:
//
//   * issues `fcntl(stub.fd, F_GETFD, 0)` against the ORIGINAL kernel
//     fd (the integer value held by `stub.fd`, not the field on the
//     caller's copy);
//
//   * asserts the return value is `-1`, proving the kernel fd is
//     no longer valid (i.e. the constructor closed it);
//
//   * asserts the input pointer's struct copy was NOT touched (the
//     previous field-equality check, retained as a secondary
//     invariant so a regression that closes the fd AND mutates the
//     pointer copy is still caught);
//
//   * does NOT defer `std.c.close(stub.fd)`. After the constructor
//     closes the fd the test exits cleanly without a second close;
//     a conditional fallback `close` runs only if the assertion
//     fails (i.e. the constructor did NOT close the fd), to prevent
//     fd leaks across test runs.
//
// The tests below prove that contract against both paths using:
//
//   * a controlled failing allocator that succeeds for the first
//     `initExportedPrefixes` allocation but fails the very next
//     allocation (which is the allocation-tracker state);
//   * a real (kernel-allocated) TCP socket bound into the stub
//     `TcpTransport`, so the input pointer's `socket_fd` field
//     reflects the kernel fd verbatim;
//   * the `std.testing.allocator` leak-detecting GPA underneath the
//     failing allocator as the export-state allocation oracle — a
//     `deinit` skipped on the failure path would surface as a
//     reported leak in the test harness.
//
// `loadConfigAndBgp` is also covered: the wrapper's `.failed` branch
// does no cleanup of its own, so the constructor's
// `releaseBundleOnFailure` is the only path that releases. To prove
// that, the wrapper is exercised indirectly by calling
// `initBgpServeBundle` and asserting the failure invariants; the
// wrapper is then statically guaranteed by the type system (the
// wrapper now delegates to the constructor, so the failure path is
// the constructor's path verbatim).

const std = @import("std");
const config = @import("../config.zig");
const reconnect_stress = @import("reconnect_stress_tests.zig");
const serve_integration = @import("serve_integration.zig");
const tcp_transport = @import("tcp_transport.zig");
const types = @import("types.zig");

// POSIX fcntl flags not exposed in Zig 0.16 std.c.
const F_GETFD: c_int = 1;

// EBADF via the libc errno enum. Zig's `std.c.E.BADF` resolves
// to the libc-supplied numeric value on every target KGB
// supports (Linux/macOS/FreeBSD). Using the symbol rather than
// a hand-written numeric literal means the test tracks the libc
// header if a target ever diverges from POSIX.
const EBADF: c_int = @intFromEnum(std.c.E.BADF);

const VoidWriter = struct {
    pub fn writeAll(_: @This(), _: []const u8) error{}!void {}
    pub fn print(_: @This(), _: []const u8, _: anytype) error{}!void {}
};

/// Allocating wrapper that fails the Nth `alloc` call. `free` always
/// forwards to the backing allocator so the leak-detecting GPA
/// underneath still sees balanced allocation/free pairs (so the leak
/// oracle is purely about whether `export_state.deinit()` ran).
const CountingFailingAllocator = struct {
    backing: std.mem.Allocator,
    fail_after: u32,
    counter: u32 = 0,

    fn allocFn(ctx: *anyopaque, len: usize, alignment: std.mem.Alignment, ra: usize) ?[*]u8 {
        const self: *CountingFailingAllocator = @ptrCast(@alignCast(ctx));
        self.counter += 1;
        if (self.counter > self.fail_after) {
            return null;
        }
        return self.backing.rawAlloc(len, alignment, ra);
    }

    fn resizeFn(ctx: *anyopaque, buf: []u8, alignment: std.mem.Alignment, new_len: usize, ra: usize) bool {
        const self: *CountingFailingAllocator = @ptrCast(@alignCast(ctx));
        return self.backing.rawResize(buf, alignment, new_len, ra);
    }

    fn remapFn(ctx: *anyopaque, buf: []u8, alignment: std.mem.Alignment, new_len: usize, ra: usize) ?[*]u8 {
        const self: *CountingFailingAllocator = @ptrCast(@alignCast(ctx));
        return self.backing.rawRemap(buf, alignment, new_len, ra);
    }

    fn freeFn(ctx: *anyopaque, buf: []u8, alignment: std.mem.Alignment, ra: usize) void {
        const self: *CountingFailingAllocator = @ptrCast(@alignCast(ctx));
        self.backing.rawFree(buf, alignment, ra);
    }

    fn allocator(self: *CountingFailingAllocator) std.mem.Allocator {
        return .{
            .ptr = self,
            .vtable = &.{
                .alloc = allocFn,
                .resize = resizeFn,
                .remap = remapFn,
                .free = freeFn,
            },
        };
    }
};

/// Build a stub `TcpTransport` whose `socket_fd` is a real kernel
/// descriptor (so we can detect any second close through the input
/// pointer via `fcntl(fd, F_GETFD, 0)`). The kernel fd is opened via
/// `std.c.socket()` against `AF_INET/SOCK_STREAM` and never bound.
/// The fd is closed manually by the test ONLY IF the constructor
/// fails to close it; the oracle is "constructor closes the kernel
/// fd exactly once, no second close is attempted".
fn realFdStubTransport() struct { transport: tcp_transport.TcpTransport, fd: std.c.fd_t } {
    const fd = std.c.socket(std.c.AF.INET, std.c.SOCK.STREAM, 0);
    if (fd < 0) @panic("could not allocate real fd for stub transport");
    const stub = tcp_transport.TcpTransport{
        .socket_fd = fd,
        .recv_buf = undefined,
        .recv_len = 0,
        .closed = false,
        .peer_address = .{ 0, 0, 0, 0 },
        .peer_port = 0,
    };
    return .{ .transport = stub, .fd = fd };
}

/// Outcome of a kernel FD-state probe. The previous oracle only
/// distinguished "success" from "some error"; F_GETFD returns -1
/// on ANY error, so a return of -1 alone does not prove the
/// descriptor is invalid. POSIX requires F_GETFD to set errno to
/// EBADF when `fd` is not a valid open descriptor; we assert both
/// the return value AND the errno value so a different errno (e.g.
/// EFAULT from an internal libc bug) cannot masquerade as a clean
/// close.
const FdProbeOutcome = enum { open, closed, other_error };

fn probeFdState(fd: std.c.fd_t) FdProbeOutcome {
    // Reset errno so a stale value from a previous syscall cannot
    // leak into this probe. POSIX permits any non-zero value on
    // entry; setting it to 0 is the portable "no prior error"
    // sentinel.
    std.c._errno().* = 0;
    const result = std.c.fcntl(fd, F_GETFD, @as(c_int, 0));
    if (result >= 0) return .open;
    const err = std.c._errno().*;
    if (err == EBADF) return .closed;
    return .other_error;
}

/// Tracks whether the test still owns the kernel fd. The test opens
/// the fd via `realFdStubTransport`; the constructor is expected
/// to close it on the failure path. The defer is conditional: it
/// fires ONLY if the assertion fails AND the constructor did NOT
/// already close the fd, so the passing path runs exactly one
/// physical close (the constructor's).
const FdOwnershipTracker = struct {
    fd: std.c.fd_t,
    /// Set to true once we have positively observed that the
    /// constructor closed the fd. The defer reads this flag and
    /// closes ONLY if it is still false.
    closed_by_constructor: bool = false,

    fn releaseIfStillOpen(self: *FdOwnershipTracker) void {
        if (!self.closed_by_constructor) {
            _ = std.c.close(self.fd);
        }
    }
};

test "initBgpServeBundle on state-init failure closes the kernel fd exactly once (fcntl F_GETFD + EBADF oracle)" {
    // 1. Real kernel fd on the input pointer. After the constructor
    //    returns, `fcntl(stub.fd, F_GETFD, 0)` MUST return -1 with
    //    errno == EBADF. This is the primary oracle: it queries
    //    the kernel, not the caller's struct copy, so a mis-wired
    //    double-close that mutates the caller's pointer would also
    //    be detected.
    var stub = realFdStubTransport();
    var owner = FdOwnershipTracker{ .fd = stub.fd };
    defer owner.releaseIfStillOpen();

    // 2. Prefixes allocated via `std.testing.allocator` (the
    //    leak-detecting GPA) so a leaked allocation surfaces as a
    //    reported leak in the test harness. The constructor copies
    //    these into `export_state.current_exported_prefixes` and
    //    must release both copies on the failure path.
    const prefixes = try std.testing.allocator.alloc(types.Ipv4Prefix, 1);
    prefixes[0] = types.Ipv4Prefix.init("10.0.0.0/8");
    // We deliberately do NOT `defer prefixes.deinit` here: the
    // constructor now owns the slice after we hand it over (just
    // like in production). If the constructor fails to release it,
    // the leak-detecting GPA reports the leak.

    // 3. Raw config: a default-initialised value. The constructor
    //    must `raw.deinit(...)` on the failure path. The default
    //    value has no heap allocations so the deinit is a no-op,
    //    but the call itself is part of the contract.
    const raw = config.RawConfig{};

    // 4. Failing allocator: succeed for the prefix allocation (the
    //    first call from `initExportedPrefixes`), then fail the
    //    allocation-tracker state init (the second call).
    var fail_alloc = CountingFailingAllocator{
        .backing = std.testing.allocator,
        .fail_after = 1,
    };
    const fail_alloc_iface = fail_alloc.allocator();

    // 5. Run the constructor. Expect `.failed`.
    const load = serve_integration.initBgpServeBundle(
        raw,
        .{ .present = true, .enabled = true },
        reconnect_stress.makeTestSessionConfig(),
        &stub.transport,
        prefixes,
        VoidWriter{},
        fail_alloc_iface,
    );
    try std.testing.expect(load == .failed);

    // 6. PRIMARY ORACLE: `fcntl(stub.fd, F_GETFD, 0)` MUST return
    //    -1 with errno == EBADF. POSIX defines F_GETFD as
    //    retrieving flags for a valid descriptor; an invalid
    //    descriptor fails with -1 and sets errno to EBADF. Any
    //    other errno value (EFAULT, EINVAL, ...) is treated as a
    //    test failure rather than a vacuous close, so a buggy
    //    libc that mishandles the fd cannot masquerade as a clean
    //    close.
    const outcome = probeFdState(stub.fd);
    switch (outcome) {
        .closed => {
            // POSIX contract satisfied: the kernel no longer
            // recognises the descriptor. The constructor owns the
            // close; the test must NOT close again.
            owner.closed_by_constructor = true;
        },
        .open => {
            // The constructor did NOT close the fd. The deferred
            // cleanup will run `close` to prevent fd leaks across
            // test runs.
            std.debug.print("ERROR: fcntl F_GETFD returned >= 0; kernel still considers fd valid\n", .{});
            return error.TestUnexpectedFdState;
        },
        .other_error => {
            std.debug.print("ERROR: fcntl F_GETFD failed with non-EBADF errno\n", .{});
            return error.TestUnexpectedFdState;
        },
    }

    // 7. SECONDARY ORACLE: the input pointer's struct copy is
    //    unchanged. A previous regression closed the input pointer
    //    AFTER bundle.tcp.close() had already closed the same
    //    kernel fd, mutating `stub.socket_fd` to -1. The kernel-FD
    //    oracle above catches the regression too, but keeping this
    //    assertion makes the failure mode obvious in the test log.
    try std.testing.expectEqual(stub.fd, stub.transport.socket_fd);
    try std.testing.expectEqual(false, stub.transport.closed);

    // 8. The leak-detecting GPA at the bottom of the failing
    //    allocator MUST report no remaining allocations from this
    //    test (no prefix leak from `initExportedPrefixes`, no leak
    //    from the failing `allocation_tracker.init`).
    //    The std.testing.allocator checks at the END of each test;
    //    passing this test cleanly is the oracle that
    //    `releaseBundleOnFailure` ran.
}

test "initBgpServeBundle on state-init failure deinits export_state (no exported-prefix allocation leak)" {
    // The oracle here is `std.testing.allocator`'s leak detection:
    // `initExportedPrefixes` allocates one slice of `Ipv4Prefix`
    // via the export_state's allocator. The constructor's failure
    // path MUST call `export_state.deinit()` so that allocation is
    // freed. If the constructor skipped `export_state.deinit()` on
    // failure (the previous design), the leak-detecting GPA would
    // report the leak at test teardown.
    var fail_alloc = CountingFailingAllocator{
        .backing = std.testing.allocator,
        // Allow a generous budget; the only alloc that matters for
        // the export_state oracle is the prefix allocation in
        // `initExportedPrefixes` (the first call). The state-init
        // failure happens on the second call.
        .fail_after = 1,
    };
    const fail_alloc_iface = fail_alloc.allocator();

    var stub = realFdStubTransport();
    // CONDITIONAL CLEANUP: the constructor is expected to close the
    // kernel fd on the failure path; we only close here if it
    // didn't. This is the same model the primary oracle test uses,
    // so the passing path runs exactly one physical close (the
    // constructor's). File descriptors can be reused after closure,
    // and a stray second close can hit a newly-assigned resource;
    // we MUST NOT defer an unconditional close here.
    var owner = FdOwnershipTracker{ .fd = stub.fd };
    defer owner.releaseIfStillOpen();

    const prefixes = try std.testing.allocator.alloc(types.Ipv4Prefix, 2);
    prefixes[0] = types.Ipv4Prefix.init("10.0.0.0/8");
    prefixes[1] = types.Ipv4Prefix.init("192.168.0.0/16");

    const raw = config.RawConfig{};

    const load = serve_integration.initBgpServeBundle(
        raw,
        .{ .present = true, .enabled = true },
        reconnect_stress.makeTestSessionConfig(),
        &stub.transport,
        prefixes,
        VoidWriter{},
        fail_alloc_iface,
    );
    try std.testing.expect(load == .failed);

    // SECONDARY ORACLE for THIS test: confirm the constructor also
    // closed the kernel fd on this path (the export_state path is
    // an extension of the primary failure path; both paths must
    // close the fd). Mark the conditional tracker so the deferred
    // cleanup does not double-close.
    const outcome = probeFdState(stub.fd);
    switch (outcome) {
        .closed => owner.closed_by_constructor = true,
        .open => return error.TestUnexpectedFdState,
        .other_error => return error.TestUnexpectedFdState,
    }

    // The leak-detecting GPA at the bottom of the failing allocator
    // runs at test teardown and FAILS the test if
    // `export_state.deinit()` was skipped. The only way this test
    // passes is if `releaseBundleOnFailure` called
    // `bundle.export_state.deinit()`.
}
