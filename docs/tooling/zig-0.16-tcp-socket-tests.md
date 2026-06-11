# TCP Socket Testing in Zig Tests

This document captures lessons learned from debugging a BGP TCP transport hang in Linux CI where raw `accept()` blocked indefinitely.

When writing tests that use TCP sockets, follow these rules to prevent CI hangs.

## Rule 1: No Raw Blocking `accept()` or `recv()` in Tests

Raw `accept()` and `recv()` calls block indefinitely if no connection arrives or no data is sent. In CI environments (especially Linux), this causes test suite hangs that are difficult to debug.

**Wrong:**
```zig
// NEVER do this in tests - blocks forever if no connection arrives
const client_fd = std.c.accept(listen_fd, null, null);
```

**Right:** Use bounded accept with `poll()` before calling `accept()`:
```zig
// Poll for incoming connection with timeout first
var poll_fd: [1]std.c.pollfd = .{
    .{ .fd = listen_fd, .events = 0x001, .revents = 0 }, // POLLIN
};
const poll_result = std.c.poll(&poll_fd, 1, timeout_ms);
if (poll_result < 0) return error.AcceptFailed;
if (poll_result == 0) return error.AcceptTimeout;

// Now accept - will not block since poll indicated data is ready
const client_fd = std.c.accept(listen_fd, null, null);
```

## Rule 2: Use Bounded `poll()` Before Socket Receive

Do not call `recv()` as the readiness probe. On a non-blocking socket it may return EAGAIN; on a blocking socket it may hang. Poll first, then recv.

**Wrong:**
```zig
// Do not call recv() as readiness probe
const received = std.c.recv(client_fd, @ptrCast(&buf), buf.len, 0);
```

**Right:**
```zig
var poll_fd: [1]std.c.pollfd = .{
    .{ .fd = client_fd, .events = 0x001, .revents = 0 }, // POLLIN
};
const poll_result = std.c.poll(&poll_fd, 1, timeout_ms);
try std.testing.expect(poll_result > 0);
try std.testing.expect((poll_fd[0].revents & 0x001) != 0); // POLLIN
const received = std.c.recv(client_fd, @ptrCast(&buf), buf.len, 0);
```

## Rule 3: Use Compile/Run Split for CI Test Observability

When debugging TCP transport issues in CI, `zig build test-bgp-tcp` combines compile and run in one step. This hides useful test output during failure triage.

**Better approach:** Compile the test binary separately, then run it directly:

```make
# Compile without running
make install-test-bgp-tcp

# Run the compiled binary directly
./tovarisch/zig-out/bin/tovarisch-test-bgp-tcp
```

This approach:
- Separates compile errors from runtime errors
- Gives clearer logs during CI failure triage
- Allows running the same binary with different timeouts

**When to use:** TCP transport tests, any test suite that previously caused CI hangs, or any test that produces output useful for debugging runtime behavior.

## Working Example

See `tovarisch/src/bgp/tcp_transport_tests.zig` for a working implementation that:
- Uses `acceptConnectionBounded()` helper with poll before accept
- Uses bounded poll before recv
- Tests bounded connect behavior
