// runtime/allocation_tracker_connector_probe.zig — minimal
// connector-side types for the bounded-memory instrumentation.
//
// The authoritative handle accounting lives inside `ReconnectMemoryState`
// (see `allocation_tracker_internal.zig`). External code never sees
// probe or ledger structs; the only observable handle-side type is the
// full `{ptr, release_fn}` release token returned by a connector's
// `acquire`.
//
// Connectors produce a `ReconnectHandle` (a `{ptr, release_fn}` pair).
// The state's `adoptHandle(state, handle)` records the full token as
// the active handle; the state's `releaseHandle(state, handle)`
// verifies that BOTH `handle.ptr` AND `handle.release_fn` match the
// recorded token, then invokes `release_fn(handle.ptr)`, and only
// then records the release. `release_fn` itself MUST NOT mutate any
// state-bound oracle counters — it may only perform local cleanup
// (closing a held test resource, clearing an in-flight flag, etc.).
//
// Sibling files in this directory carry the remaining instrumentation:
//   * `allocation_tracker.zig`                 — public re-exports.
//   * `allocation_tracker_internal.zig`        — opaque state + accessors
//                                                + `init/deinit` +
//                                                `adoptHandle/releaseHandle` +
//                                                `finishReconnectBoundary`.
//   * `allocation_tracker_tracking_allocator.zig`
//                                              — producer-side (private).
//
// Identity token note: the state's identity check now covers both
// `ptr` and `release_fn`. A forged release with a matching `ptr` but
// a different `release_fn` is rejected as `WrongHandle` without
// invoking either physical callback.

const std = @import("std");

/// Physical-release callback installed by a connector's `acquire` step.
/// The state invokes this exactly once per `releaseHandle` call, AFTER
/// the active-handle identity check (BOTH `ptr` and `release_fn`) has
/// passed. The callback MUST NOT mutate any state-bound oracle counters
/// — it may only perform local cleanup tied to the connector's own
/// resources.
pub const reconnect_handle_release_fn = *const fn (handle: *anyopaque) void;

/// Opaque release token produced by a connector's `acquire` step.
///
/// The state's `adoptHandle(state, handle)` records the full token
/// (both `ptr` and `release_fn`) as the active handle; the state's
/// `releaseHandle(state, handle)` verifies that BOTH `ptr` AND
/// `release_fn` match the recorded token, invokes
/// `release_fn(handle.ptr)`, and only then clears the active record
/// and increments the release counter.
///
/// A forged release that supplies a different `release_fn` (even with
/// the matching `ptr`) is rejected as `error.WrongHandle` without
/// invoking either physical callback; the state cannot be tricked into
/// executing a different cleanup path or recording a release for it.
///
/// Callers MUST NOT invoke `release_fn` directly. Always dispose of a
/// handle through `releaseHandle(state, handle)` so the atomic boundary
/// (see `finishReconnectBoundary`) observes a balanced acquire/release
/// history.
pub const ReconnectHandle = struct {
    ptr: *anyopaque,
    release_fn: reconnect_handle_release_fn,
};
